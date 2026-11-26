package cli

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/schema"
	"github.com/yoosuf/ae.sh/internal/util"
)

const (
	evidenceSignatureFile = "signature.json"
	maxSigningKeyBytes    = 8 << 10
)

type evidenceSignature struct {
	SchemaVersion string `json:"schemaVersion"`
	RunID         string `json:"runID"`
	Algorithm     string `json:"algorithm"`
	KeyID         string `json:"keyID"`
	PublicKey     string `json:"publicKey"`
	BundleDigest  string `json:"bundleDigest"`
	RecordCount   int    `json:"recordCount"`
	Signature     string `json:"signature"`
	Note          string `json:"note"`
}

type evidenceBundle struct {
	SchemaVersion string            `json:"schemaVersion"`
	RecordCount   int               `json:"recordCount"`
	Records       []evidence.Record `json:"records"`
}

func evidenceBundleDigest(records []evidence.Record) (string, []byte) {
	lines := make([]string, 0, len(records))
	for _, record := range records {
		lines = append(lines, fmt.Sprintf("%d:%s:%s:%s", record.Sequence, record.RecordID,
			record.DataDigest, record.PreviousDigest))
	}
	canonical := []byte(strings.Join(lines, "\n"))
	return util.Digest(canonical), canonical
}

func publicKeyID(public ed25519.PublicKey) string {
	return util.Digest([]byte(public))
}

func loadSigningKey(keyFile, keyEnv string) (ed25519.PrivateKey, error) {
	if (keyFile == "") == (keyEnv == "") {
		return nil, invalid("signing requires exactly one of --key-file or --key-env")
	}
	path := keyFile
	if keyEnv != "" {
		if !config.IsEnvName(keyEnv) {
			return nil, invalid("invalid signing key environment variable name")
		}
		path = os.Getenv(keyEnv)
		if path == "" {
			return nil, invalid("signing key environment variable %s is not set", keyEnv)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, invalid("signing key file: %v", err)
	}
	if len(raw) > maxSigningKeyBytes {
		return nil, invalid("signing key file exceeds %d bytes", maxSigningKeyBytes)
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, invalid("signing key must be a PEM-encoded PKCS#8 private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, invalid("signing key is not a valid PKCS#8 key: %v", err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, invalid("signing key must be Ed25519")
	}
	return key, nil
}

func signEvidenceBundle(dir string, records []evidence.Record, key ed25519.PrivateKey) (evidenceSignature, error) {
	if len(records) == 0 {
		return evidenceSignature{}, invalid("an empty evidence bundle cannot be signed")
	}
	digest, canonical := evidenceBundleDigest(records)
	public := key.Public().(ed25519.PublicKey)
	signature := ed25519.Sign(key, canonical)
	result := evidenceSignature{
		SchemaVersion: "1.0",
		RunID:         records[0].RunID,
		Algorithm:     "Ed25519",
		KeyID:         publicKeyID(public),
		PublicKey:     base64.StdEncoding.EncodeToString(public),
		BundleDigest:  digest,
		RecordCount:   len(records),
		Signature:     base64.StdEncoding.EncodeToString(signature),
		Note:          "Local authenticity only: no trusted timestamp, transparency log, or external attestation.",
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return evidenceSignature{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, evidenceSignatureFile), append(raw, '\n'), 0o600); err != nil {
		return evidenceSignature{}, err
	}
	return result, nil
}

func loadVerificationKey(keyFile string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, invalid("verification key file: %v", err)
	}
	if len(raw) > maxSigningKeyBytes {
		return nil, invalid("verification key file exceeds %d bytes", maxSigningKeyBytes)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, invalid("verification key must be PEM-encoded")
	}
	switch block.Type {
	case "PUBLIC KEY":
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, invalid("verification key is not a valid PKIX public key: %v", err)
		}
		key, ok := parsed.(ed25519.PublicKey)
		if !ok {
			return nil, invalid("verification key must be Ed25519")
		}
		return key, nil
	case "PRIVATE KEY":
		key, err := loadSigningKey(keyFile, "")
		if err != nil {
			return nil, err
		}
		return key.Public().(ed25519.PublicKey), nil
	default:
		return nil, invalid("verification key must be a PEM public or PKCS#8 private key")
	}
}

func verifyEvidenceBundle(dir, runID, keyFile string) (map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "evidence.json"))
	if err != nil {
		return nil, invalid("evidence bundle: %v", err)
	}
	var bundle evidenceBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, invalid("evidence bundle is not valid JSON: %v", err)
	}
	if bundle.SchemaVersion != "1.0" || bundle.RecordCount != len(bundle.Records) || len(bundle.Records) == 0 {
		return nil, invalid("evidence bundle has an unsupported shape")
	}
	sigRaw, err := os.ReadFile(filepath.Join(dir, evidenceSignatureFile))
	if err != nil {
		return nil, invalid("signature sidecar: %v", err)
	}
	var signature evidenceSignature
	if err := json.Unmarshal(sigRaw, &signature); err != nil {
		return nil, invalid("signature sidecar is not valid JSON: %v", err)
	}
	if signature.SchemaVersion != "1.0" || signature.Algorithm != "Ed25519" ||
		signature.RunID != runID || signature.RecordCount != len(bundle.Records) {
		return nil, invalid("signature does not match the requested run")
	}
	for index, record := range bundle.Records {
		if record.RunID != runID || record.Sequence != index {
			return nil, invalid("evidence sequence or run id is inconsistent at record %d", index)
		}
		if index > 0 && record.PreviousDigest != bundle.Records[index-1].DataDigest {
			return nil, invalid("evidence chain is broken at record %d", index)
		}
		generic, err := util.JSONValue(record)
		if err != nil || schema.Validate("evidence", generic) != nil {
			return nil, invalid("evidence record %d failed schema validation", index)
		}
		rawData, err := json.Marshal(record.Data)
		if err != nil {
			return nil, invalid("evidence record %d cannot be encoded", index)
		}
		if util.Digest(rawData) != record.DataDigest {
			return nil, invalid("evidence record %d data digest mismatch", index)
		}
		if util.SecretShapeCount(string(rawData)) > 0 {
			return nil, invalid("evidence record %d contains a credential-shaped value", index)
		}
	}
	digest, canonical := evidenceBundleDigest(bundle.Records)
	if digest != signature.BundleDigest {
		return nil, invalid("bundle digest does not match the signature sidecar")
	}
	public, err := base64.StdEncoding.DecodeString(signature.PublicKey)
	if err != nil || len(public) != ed25519.PublicKeySize {
		return nil, invalid("signature sidecar has an invalid public key")
	}
	sig, err := base64.StdEncoding.DecodeString(signature.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, invalid("signature sidecar has an invalid signature")
	}
	keySource := "bundle"
	if keyFile != "" {
		derived, err := loadVerificationKey(keyFile)
		if err != nil {
			return nil, err
		}
		if publicKeyID(derived) != signature.KeyID || string(derived) != string(public) {
			return nil, invalid("signature key does not match the supplied key file")
		}
		keySource = "key-file"
	}
	if signature.KeyID != publicKeyID(public) || !ed25519.Verify(public, canonical, sig) {
		return nil, invalid("evidence bundle signature is invalid")
	}
	return map[string]any{
		"runID": runID, "records": len(bundle.Records), "bundleDigest": digest,
		"keyID": signature.KeyID, "keySource": keySource, "verified": true,
		"status": "passed",
		"note":   "Local signature and chain verification only; not an external attestation.",
	}, nil
}
