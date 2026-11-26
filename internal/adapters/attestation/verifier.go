// Package attestation reserves the externally verified review boundary. It is
// a fail-closed extension point: no verifier, credential read, network call, or
// verified review claim is implemented. Provider text remains untrusted data.
package attestation

import (
	"context"
	"errors"
	"fmt"

	"github.com/yoosuf/ae.sh/internal/config"
)

// ErrNotImplemented is returned for every verification attempt. It lets callers
// distinguish a configured extension point from an unknown review mode.
var ErrNotImplemented = errors.New("externally verified review is configured but not implemented; provider text remains untrusted")

// Result is the future typed outcome of an independent attestation check. It
// exists only to reserve the port; Verified is always false today.
type Result struct {
	Verified bool   `json:"verified"`
	Issuer   string `json:"issuer,omitempty"`
	Digest   string `json:"digest,omitempty"`
}

// Verifier is a placeholder for a future independent review verifier.
type Verifier struct{}

// New accepts only the local review boundary. External-attestation
// configuration is validated by config.Load but is deliberately refused here.
func New(cfg config.ReviewIntegration) (*Verifier, error) {
	switch cfg.Mode {
	case "", "local":
		return &Verifier{}, nil
	case "external-attestation":
		return nil, ErrNotImplemented
	default:
		return nil, fmt.Errorf("unsupported integrations.review.mode %q", cfg.Mode)
	}
}

// Verify always fails closed. It does not inspect the supplied bytes, read a
// verifier path, contact a service, or mark any review as independently verified.
func (v *Verifier) Verify(context.Context, []byte) (Result, error) {
	return Result{}, ErrNotImplemented
}
