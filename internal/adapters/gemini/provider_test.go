package gemini

import (
	"context"
	"errors"
	"testing"

	"github.com/yoosuf/ae.sh/internal/integration"
)

func TestProviderFailsClosedWithoutInvocation(t *testing.T) {
	provider := New()
	if provider.Name() != "gemini" {
		t.Fatalf("unexpected provider name %q", provider.Name())
	}
	if provider.Capabilities() != (integration.Capabilities{}) {
		t.Fatalf("extension point advertised live capabilities: %+v", provider.Capabilities())
	}
	if _, err := provider.Run(context.Background(), integration.RunRequest{
		TaskID: "task-1", Skill: "code-review", Prompt: "review", Workspace: t.TempDir(),
	}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Run did not fail closed: %v", err)
	}
	if err := provider.Cancel(context.Background(), "session-1"); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Cancel did not fail closed: %v", err)
	}
}
