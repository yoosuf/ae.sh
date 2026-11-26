package identity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yoosuf/ae.sh/internal/config"
)

func TestFederatedModeFailsClosedWithoutReadingCredentials(t *testing.T) {
	const tokenValue = "synthetic-identity-token-value"
	t.Setenv("AE_IDENTITY_TOKEN", tokenValue)
	cfg := config.IdentityIntegration{
		Mode:       "federated",
		Issuer:     "https://id.example.com/realms/ae",
		TokenEnv:   "AE_IDENTITY_TOKEN",
		Audience:   "ae-cli",
		ActorClaim: "preferred_username",
	}
	verifier, err := New(cfg)
	if verifier != nil || !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("federated identity did not fail closed: verifier=%v err=%v", verifier, err)
	}
	if strings.Contains(err.Error(), tokenValue) {
		t.Fatal("error leaked the configured token value")
	}
}

func TestLocalManualBoundaryVerifierAlsoFailsClosed(t *testing.T) {
	verifier, err := New(config.IdentityIntegration{Mode: "local-manual"})
	if err != nil || verifier == nil {
		t.Fatalf("local boundary was not registered: %v", err)
	}
	result, err := verifier.Verify(context.Background(), "reviewer")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Verify did not fail closed: %v", err)
	}
	if result.Verified || result.Approver != "" || result.IdentitySource != "" {
		t.Fatalf("Verify produced an identity claim: %+v", result)
	}
}

func TestNewRejectsUnknownMode(t *testing.T) {
	if _, err := New(config.IdentityIntegration{Mode: "oidc"}); err == nil {
		t.Fatal("unknown identity mode was accepted")
	}
}
