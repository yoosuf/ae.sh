// Package gemini is a fail-closed extension point for a future Gemini adapter.
// It implements the provider-neutral port without invoking a binary, making a
// network call, or reading credentials. Live Gemini invocation remains planned /
// not implemented.
package gemini

import (
	"context"
	"errors"

	"github.com/yoosuf/ae.sh/internal/integration"
)

// ErrNotImplemented is returned for every invocation. It exists so callers can
// distinguish a configured extension point from an unknown provider name.
var ErrNotImplemented = errors.New("Gemini adapter is a local extension point only; live invocation is not implemented")

// Provider reserves the Gemini adapter boundary for future optional adapters.
// No field is exported because there is currently no safe live invocation path.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "gemini" }

// Capabilities are intentionally empty: this extension point cannot execute a
// task, cancel a session, observe usage, or select a model.
func (p *Provider) Capabilities() integration.Capabilities {
	return integration.Capabilities{}
}

func (p *Provider) Run(context.Context, integration.RunRequest) (integration.RunResult, error) {
	return integration.RunResult{}, ErrNotImplemented
}

func (p *Provider) Cancel(context.Context, string) error { return ErrNotImplemented }
