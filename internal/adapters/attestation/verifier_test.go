package attestation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yoosuf/ae.sh/internal/config"
)

func TestExternalAttestationFailsClosedWithoutReadingCredentials(t *testing.T) {
	const verifierValue = "synthetic-verifier-path-value"
	t.Setenv("AE_REVIEW_VERIFIER", verifierValue)
	cfg := config.ReviewIntegration{
		Mode:        "external-attestation",
		Issuer:      "https://attest.example.com",
		VerifierEnv: "AE_REVIEW_VERIFIER",
	}
	verifier, err := New(cfg)
	if verifier != nil || !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("external attestation did not fail closed: verifier=%v err=%v", verifier, err)
	}
	if strings.Contains(err.Error(), verifierValue) {
		t.Fatal("error leaked the configured verifier value")
	}
}

func TestLocalBoundaryVerifierAlsoFailsClosed(t *testing.T) {
	verifier, err := New(config.ReviewIntegration{Mode: "local"})
	if err != nil || verifier == nil {
		t.Fatalf("local boundary was not registered: %v", err)
	}
	result, err := verifier.Verify(context.Background(), []byte(`{"findings":[]}`))
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Verify did not fail closed: %v", err)
	}
	if result.Verified || result.Issuer != "" || result.Digest != "" {
		t.Fatalf("Verify produced a verification claim: %+v", result)
	}
}

func TestNewRejectsUnknownMode(t *testing.T) {
	if _, err := New(config.ReviewIntegration{Mode: "signed"}); err == nil {
		t.Fatal("unknown review mode was accepted")
	}
}
