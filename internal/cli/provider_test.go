package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yoosuf/ae.sh/internal/adapters/gemini"
	"github.com/yoosuf/ae.sh/internal/integration"
)

func TestNewProviderGeminiIsFailClosed(t *testing.T) {
	provider, err := newProvider("gemini")
	if err != nil {
		t.Fatalf("gemini extension point was not registered: %v", err)
	}
	if _, ok := provider.(*gemini.Provider); !ok {
		t.Fatalf("unexpected Gemini provider type %T", provider)
	}
	_, err = provider.Run(context.Background(), integration.RunRequest{
		TaskID: "task-1", Skill: "code-review", Prompt: "review", Workspace: t.TempDir(),
	})
	if !errors.Is(err, gemini.ErrNotImplemented) {
		t.Fatalf("Gemini invocation did not fail closed: %v", err)
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("error does not explain the missing live adapter: %v", err)
	}
}
