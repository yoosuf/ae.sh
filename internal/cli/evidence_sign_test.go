package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
)

func writeSigningKeyPair(t *testing.T, dir string) (string, string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	privatePath := filepath.Join(dir, "signing-key.pem")
	publicPath := filepath.Join(dir, "verification-key.pem")
	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return privatePath, publicPath
}

func signedEvidenceRecords(t *testing.T) []evidence.Record {
	t.Helper()
	ctx := context.Background()
	manager := evidence.New(t.TempDir(), t.TempDir())
	first, err := manager.Build(ctx, "run-sign-1", 0, "note", "first", "tester", map[string]any{"step": "one"}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Build(ctx, "run-sign-1", 1, "note", "second", "tester", map[string]any{"step": "two"}, nil, nil, first.DataDigest)
	if err != nil {
		t.Fatal(err)
	}
	return []evidence.Record{first, second}
}

func TestEvidenceBundleSignVerifyAndTamperDetection(t *testing.T) {
	dir := t.TempDir()
	records := signedEvidenceRecords(t)
	if err := evidence.ExportJSON(records, filepath.Join(dir, "evidence.json")); err != nil {
		t.Fatal(err)
	}
	privatePath, publicPath := writeSigningKeyPair(t, dir)
	key, err := loadSigningKey(privatePath, "")
	if err != nil {
		t.Fatal(err)
	}
	signature, err := signEvidenceBundle(dir, records, key)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := verifyEvidenceBundle(dir, "run-sign-1", publicPath)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}
	if verified["verified"] != true || verified["bundleDigest"] != signature.BundleDigest || verified["keySource"] != "key-file" {
		t.Fatalf("unexpected verification result: %+v", verified)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), `"step": "one"`, `"step": "changed"`, 1)
	if tampered == string(raw) {
		t.Fatal("test fixture did not contain the expected evidence value")
	}
	if err := os.WriteFile(filepath.Join(dir, "evidence.json"), []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyEvidenceBundle(dir, "run-sign-1", ""); err == nil {
		t.Fatal("tampered evidence bundle was accepted")
	}
}

func TestEvidenceVerificationFailsClosedOnMissingOrWrongKey(t *testing.T) {
	dir := t.TempDir()
	records := signedEvidenceRecords(t)
	if err := evidence.ExportJSON(records, filepath.Join(dir, "evidence.json")); err != nil {
		t.Fatal(err)
	}
	privatePath, _ := writeSigningKeyPair(t, dir)
	key, err := loadSigningKey(privatePath, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := signEvidenceBundle(dir, records, key); err != nil {
		t.Fatal(err)
	}
	otherDir := t.TempDir()
	_, wrongPublic := writeSigningKeyPair(t, otherDir)
	if _, err := verifyEvidenceBundle(dir, "run-sign-1", wrongPublic); err == nil {
		t.Fatal("verification accepted an unrelated key")
	}
	if _, err := loadSigningKey("", "NOT_A_VALID_NAME"); err == nil {
		t.Fatal("invalid environment variable name was accepted")
	}
	if _, err := loadSigningKey("", "AE_MISSING_SIGNING_KEY_PATH"); err == nil {
		t.Fatal("missing key environment variable was accepted")
	}
}

func initEvidenceTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	copyCanonicalRepo(t, mustRepoRoot(t), root)
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true, Args: []string{"init", "--profile", "go"}}
	if code := app.Run(context.Background()); code != ExitOK {
		t.Fatalf("init failed with %d: %s %s", code, stdout.String(), stderr.String())
	}
	return root
}

func appendTestEvidence(t *testing.T, root, runID string) {
	t.Helper()
	configPath, err := config.Find(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run := execution.Run{
		ID: runID, WorkflowID: "feature-development", WorkflowVersion: "1.0.0",
		IssueKey: "PROJ-101", State: "awaiting_approval", Status: "awaiting-approval",
		Risk: "low", Provider: "fake", Profiles: []string{"go"}, Actor: "tester",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", Version: 1,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if _, err := appendRunEvidence(context.Background(), cfg, store, runID, "note", "local test", "tester", map[string]any{"step": "recorded"}, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func runEvidenceCLI(t *testing.T, root string, args ...string) (int, map[string]any) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true, Args: append([]string{"evidence"}, args...)}
	code := app.Run(context.Background())
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("parse evidence output (%d): %v: %s %s", code, err, stdout.String(), stderr.String())
	}
	return code, payload
}

func TestEvidenceCLIExportRejectsUnknownOrUnsafeRunID(t *testing.T) {
	root := initEvidenceTestRepo(t)
	code, payload := runEvidenceCLI(t, root, "export", "run-does-not-exist")
	if code != ExitError {
		t.Fatalf("export of an unknown run succeeded: %d %+v", code, payload)
	}
	code, payload = runEvidenceCLI(t, root, "export", "../escape")
	if code != ExitValidation {
		t.Fatalf("traversal run id was accepted: %d %+v", code, payload)
	}
}

func TestEvidenceCLIExportSignAndVerify(t *testing.T) {
	root := initEvidenceTestRepo(t)
	appendTestEvidence(t, root, "run-cli-sign")
	privatePath, publicPath := writeSigningKeyPair(t, root)
	bundleDir := filepath.Join(root, "signed-export")
	code, exported := runEvidenceCLI(t, root, "export", "run-cli-sign", "--format", "json", "--out", bundleDir,
		"--sign", "--key-file", privatePath)
	if code != ExitOK || exported["signed"] != true || exported["records"] != float64(1) {
		t.Fatalf("signed export failed: %d %+v", code, exported)
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "signature.json")); err != nil {
		t.Fatal(err)
	}
	code, verified := runEvidenceCLI(t, root, "verify", "run-cli-sign", "--bundle", bundleDir, "--key-file", publicPath)
	if code != ExitOK || verified["verified"] != true || verified["records"] != float64(1) {
		t.Fatalf("signed verification failed: %d %+v", code, verified)
	}
	code, _ = runEvidenceCLI(t, root, "export", "run-cli-sign", "--format", "markdown", "--out", bundleDir,
		"--sign", "--key-file", privatePath)
	if code != ExitValidation {
		t.Fatalf("markdown-only signing was accepted with exit %d", code)
	}
}
