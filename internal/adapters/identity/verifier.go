// Package identity reserves the federated approver identity boundary. It is a
// fail-closed extension point: no verifier, token read, network call, or
// approval claim is implemented. Local approvals remain local-manual.
package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/yoosuf/ae.sh/internal/config"
)

// ErrNotImplemented is returned for every verification attempt. It lets callers
// distinguish a configured extension point from an unknown identity mode.
var ErrNotImplemented = errors.New("federated approver identity is configured but not implemented; approvals remain local-manual")

// Result is the future typed outcome of an external identity check. It exists
// only to reserve the port; Verified is always false today.
type Result struct {
	Approver       string `json:"approver"`
	IdentitySource string `json:"identitySource"`
	Verified       bool   `json:"verified"`
}

// Verifier is a placeholder for a future federated identity verifier.
type Verifier struct{}

// New accepts only the local-manual boundary. Federated configuration is
// validated by config.Load but is deliberately refused here.
func New(cfg config.IdentityIntegration) (*Verifier, error) {
	switch cfg.Mode {
	case "", "local-manual":
		return &Verifier{}, nil
	case "federated":
		return nil, ErrNotImplemented
	default:
		return nil, fmt.Errorf("unsupported integrations.identity.mode %q", cfg.Mode)
	}
}

// Verify always fails closed. It does not read a token, contact an issuer, or
// produce an approver identity that could satisfy an organization gate.
func (v *Verifier) Verify(context.Context, string) (Result, error) {
	return Result{}, ErrNotImplemented
}
