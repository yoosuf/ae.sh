// Package fake provides a deterministic, offline provider used by tests and
// the documented offline demo. It performs no shell execution and no network I/O.
package fake

import (
	"context"
	"fmt"
	"sort"

	"github.com/aef-framework/aef/internal/integration"
)

// Provider is a deterministic fake agent provider.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "fake" }

func (p *Provider) Capabilities() integration.Capabilities {
	return integration.Capabilities{StructuredOutput: true, Cancellation: true, Usage: false, SessionMetadata: true, Live: false}
}

func (p *Provider) Run(_ context.Context, request integration.RunRequest) (integration.RunResult, error) {
	if request.TaskID == "" || request.Skill == "" {
		return integration.RunResult{}, fmt.Errorf("fake provider requires taskId and skill")
	}
	exit := 0
	sessionID := request.SessionID
	if sessionID == "" && request.Persistent {
		sessionID = "fake-session-" + request.RunID
		if sessionID == "fake-session-" {
			sessionID = "fake-session-" + request.TaskID
		}
	}
	return integration.RunResult{
		Output: map[string]any{
			"simulated": true,
			"taskId":    request.TaskID,
			"skill":     request.Skill,
			"agent":     request.Agent,
			"result":    fmt.Sprintf("Simulated response to %q", request.Prompt),
			"summary":   fmt.Sprintf("Simulated %s task completed by fake provider", request.Skill),
			"artifacts": sortedArtifacts(request.Skill),
		},
		ExitStatus: &exit,
		Usage:      integration.Usage{Known: false},
		SessionID:  sessionID,
		Simulated:  true,
	}, nil
}

func (p *Provider) Cancel(_ context.Context, _ string) error { return nil }

func sortedArtifacts(skill string) []string {
	artifacts := []string{skill + ".result.json", "task.evidence.json"}
	sort.Strings(artifacts)
	return artifacts
}
