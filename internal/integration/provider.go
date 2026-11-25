// Package integration defines the provider-neutral agent interface. Provider
// SDKs and CLIs are never referenced from core packages.
package integration

import (
	"context"
	"time"
)

// Capabilities describes what a provider supports.
type Capabilities struct {
	StructuredOutput bool
	Cancellation     bool
	Usage            bool
	SessionMetadata  bool
	Live             bool
	ModelSelection   bool
}

// RunRequest is one typed task invocation. Prompt text is sanitized by the
// engine before it reaches a provider.
type RunRequest struct {
	RunID      string
	TaskID     string
	Skill      string
	Agent      string
	Prompt     string
	Workspace  string
	Timeout    time.Duration
	SessionID  string // resume an existing provider conversation when supported
	Persistent bool   // keep the session resumable; used by `ae chat`
	// RoutingProfile is a provider-neutral intent from framework/routing.
	RoutingProfile string
	// RequestedModel is the local config mapping for RoutingProfile. Adapters
	// must not ignore it when they advertise ModelSelection.
	RequestedModel string
	// Write permits file edits in Workspace. It is false unless the caller
	// explicitly opts in and the workspace is an AE-managed worktree.
	Write bool
}

// Usage records provider usage. Unknown usage is never reported as zero.
type Usage struct {
	Known        bool     `json:"known"`
	InputTokens  *int     `json:"inputTokens,omitempty"`
	OutputTokens *int     `json:"outputTokens,omitempty"`
	CostUSD      *float64 `json:"costUSD,omitempty"`
}

// RunResult is the typed result of a provider invocation.
type RunResult struct {
	Output         map[string]any `json:"output"`
	ExitStatus     *int           `json:"exitStatus"`
	Usage          Usage          `json:"usage"`
	SessionID      string         `json:"sessionId,omitempty"`
	RequestedModel string         `json:"requestedModel,omitempty"`
	Model          string         `json:"model,omitempty"` // actual model when the adapter can observe it
	Simulated      bool           `json:"simulated"`
}

// Provider is the port implemented by Claude, Codex and fake adapters.
type Provider interface {
	Name() string
	Capabilities() Capabilities
	Run(ctx context.Context, request RunRequest) (RunResult, error)
	Cancel(ctx context.Context, sessionID string) error
}
