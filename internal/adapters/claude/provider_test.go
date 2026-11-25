package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aef-framework/aef/internal/integration"
)

func TestResumeUsesClaudeResumeFlag(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	script := filepath.Join(t.TempDir(), "claude-stub")
	body := "#!/bin/sh\ncase \"$1\" in\n--version) printf '%s\\n' '2.1.314' ;;\n*) printf '%s\\n' \"$@\" > \"$AEF_TEST_ARGS\"; printf '%s\\n' '{\"result\":\"ok\",\"session_id\":\"sess-1\"}' ;;\nesac\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEF_TEST_ARGS", argsPath)
	provider := New()
	provider.Binary = script
	workspace := t.TempDir()
	if _, err := provider.Run(context.Background(), integration.RunRequest{
		TaskID: "chat-1", Skill: "provider-chat", Prompt: "first", Workspace: workspace,
		Timeout: time.Second, Persistent: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Run(context.Background(), integration.RunRequest{
		TaskID: "chat-2", Skill: "provider-chat", Prompt: "follow up", Workspace: workspace,
		Timeout: time.Second, SessionID: "sess-1", Persistent: true,
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	for _, want := range []string{"--resume", "sess-1", "--bare", "follow up"} {
		if !strings.Contains(args, want) {
			t.Errorf("claude arguments missing %q: %q", want, args)
		}
	}
	if strings.Contains(args, "Bash") {
		t.Errorf("claude read-only invocation unexpectedly allowed a shell tool: %q", args)
	}
}

func TestWriteUsesFileToolsWithoutShell(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	settingsCopy := filepath.Join(t.TempDir(), "settings.json")
	script := filepath.Join(t.TempDir(), "claude-stub")
	body := "#!/bin/sh\ncase \"$1\" in\n--version) printf '%s\\n' '2.1.314' ;;\n*) prev=\"\"; for arg in \"$@\"; do if [ \"$prev\" = \"--settings\" ]; then cp \"$arg\" \"$AEF_TEST_SETTINGS\"; fi; prev=\"$arg\"; done; printf '%s\\n' \"$@\" > \"$AEF_TEST_ARGS\"; printf '%s\\n' '{\"result\":\"ok\",\"session_id\":\"sess-2\"}' ;;\nesac\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEF_TEST_ARGS", argsPath)
	t.Setenv("AEF_TEST_SETTINGS", settingsCopy)
	provider := New()
	provider.Binary = script
	if _, err := provider.Run(context.Background(), integration.RunRequest{
		TaskID: "implement", Skill: "backend-development", Prompt: "edit files", Workspace: t.TempDir(),
		Timeout: time.Second, Write: true,
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	if !strings.Contains(args, "Read,Write,Edit") || strings.Contains(args, "Bash") {
		t.Fatalf("claude write arguments were not constrained to file tools: %q", args)
	}
	if !strings.Contains(args, "--settings") {
		t.Fatalf("claude write invocation did not pass confinement settings: %q", args)
	}
	settings, err := os.ReadFile(settingsCopy)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"blockReadsOutsideWorkingDirectories", "blockWritesOutsideWorkingDirectories"} {
		if !strings.Contains(string(settings), key) {
			t.Fatalf("confinement settings missing %q: %q", key, settings)
		}
	}
}

func TestWriteRejectsOldClaudeVersion(t *testing.T) {
	script := filepath.Join(t.TempDir(), "claude-stub")
	body := "#!/bin/sh\nprintf '%s\\n' '2.1.313'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	provider := New()
	provider.Binary = script
	_, err := provider.Run(context.Background(), integration.RunRequest{
		TaskID: "implement", Skill: "backend-development", Prompt: "edit files", Workspace: t.TempDir(),
		Timeout: time.Second, Write: true,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot confine file writes") {
		t.Fatalf("old Claude version was not rejected: %v", err)
	}
}
