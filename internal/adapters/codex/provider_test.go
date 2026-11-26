package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoosuf/ae.sh/internal/integration"
)

func TestSanitizeShortRedactsCredentialShapes(t *testing.T) {
	secret := "sk-" + strings.Repeat("a", 20)
	got := sanitizeShort("provider failed with " + secret)
	if strings.Contains(got, secret) || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("stderr was not redacted: %q", got)
	}
}

func TestResumeUsesCodexExecResumeSubcommand(t *testing.T) {
	dir := t.TempDir()
	firstArgs := filepath.Join(dir, "first-args.txt")
	secondArgs := filepath.Join(dir, "second-args.txt")
	script := filepath.Join(dir, "codex-stub")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$AE_TEST_ARGS\"\nprintf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"sess-codex\"}'\nprintf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"ok\"}}'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	provider := New()
	provider.Binary = script
	workspace := t.TempDir()
	t.Setenv("AE_TEST_ARGS", firstArgs)
	first, err := provider.Run(context.Background(), integration.RunRequest{
		TaskID: "chat-1", Skill: "provider-chat", Prompt: "first", Workspace: workspace,
		Timeout: time.Second, Persistent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID != "sess-codex" {
		t.Fatalf("unexpected first session id %q", first.SessionID)
	}
	t.Setenv("AE_TEST_ARGS", secondArgs)
	if _, err := provider.Run(context.Background(), integration.RunRequest{
		TaskID: "chat-2", Skill: "provider-chat", Prompt: "follow up", Workspace: workspace,
		Timeout: time.Second, SessionID: first.SessionID, Persistent: true,
	}); err != nil {
		t.Fatal(err)
	}
	firstRaw, err := os.ReadFile(firstArgs)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := os.ReadFile(secondArgs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(firstRaw), "--ephemeral") || !strings.Contains(string(firstRaw), "exec") {
		t.Errorf("persistent first turn used wrong arguments: %q", firstRaw)
	}
	for _, want := range []string{"exec", "resume", "sess-codex", "follow up"} {
		if !strings.Contains(string(secondRaw), want) {
			t.Errorf("codex arguments missing %q: %q", want, secondRaw)
		}
	}
	if !strings.Contains(string(firstRaw), "read-only") {
		t.Errorf("persistent first turn was not read-only: %q", firstRaw)
	}
}

func TestRequestedModelOverridesProviderModel(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	script := filepath.Join(t.TempDir(), "codex-stub")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$AE_TEST_ARGS\"\nprintf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"ok\"},\"model\":\"gpt-6.2-sol\"}'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AE_TEST_ARGS", argsPath)
	provider := New()
	provider.Binary = script
	provider.Model = "gpt-6-luna"
	result, err := provider.Run(context.Background(), integration.RunRequest{
		TaskID: "review", Skill: "code-review", Prompt: "review", Workspace: t.TempDir(),
		Timeout: time.Second, RequestedModel: "gpt-6.2-sol",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	if !strings.Contains(args, "--model\ngpt-6.2-sol\n") {
		t.Fatalf("requested model was not passed to Codex: %q", args)
	}
	if strings.Contains(args, "--model\ngpt-6-luna\n") {
		t.Fatalf("provider model overrode requested model: %q", args)
	}
	if result.RequestedModel != "gpt-6.2-sol" || result.Model != "gpt-6.2-sol" {
		t.Fatalf("model evidence mismatch: requested %q, actual %q", result.RequestedModel, result.Model)
	}
}

func TestWriteUsesWorkspaceWriteSandbox(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	script := filepath.Join(t.TempDir(), "codex-stub")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$AE_TEST_ARGS\"\nprintf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"ok\"}}'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AE_TEST_ARGS", argsPath)
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
	if !strings.Contains(string(raw), "workspace-write") {
		t.Fatalf("codex write invocation did not use workspace-write: %q", raw)
	}
}
