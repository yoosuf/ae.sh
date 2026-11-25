package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
)

func initChatRepo(t *testing.T, root string) config.Config {
	t.Helper()
	createMinimalFrameworkRoot(t, root)
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"init", "--profile", "go"}}
	if code := app.Run(context.Background()); code != ExitOK {
		t.Fatalf("init exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	cfg, err := config.Load(filepath.Join(root, "aef.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func createChatRun(t *testing.T, cfg config.Config, runID string) {
	t.Helper()
	ctx := context.Background()
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.CreateRun(ctx, execution.Run{
		ID: runID, WorkflowID: "local-flow", WorkflowVersion: "1.0.0",
		IssueKey: "TEST-1", State: "queued", Status: "running", Risk: "low",
		Provider: "fake", Actor: "tester", CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z", Version: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestChatFakeScriptPreservesSessionAndWritesEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := initChatRepo(t, root)
	const runID = "run-chat-1"
	createChatRun(t, cfg, runID)
	script := filepath.Join(root, "chat.json")
	content := `{"messages":["first question","token sk-FAKEabcdefghijklmnop please"]}`
	if err := os.WriteFile(script, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"chat", "--provider", "fake", "--script", script, "--run", runID, "--timeout", "10s"}}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("chat exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	var result chatResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Provider != "fake" || result.SessionID == "" || len(result.Turns) != 2 {
		t.Fatalf("unexpected chat result: %+v", result)
	}
	if result.Turns[0].SessionID != result.SessionID || result.Turns[1].SessionID != result.SessionID {
		t.Errorf("session id was not preserved across turns: %+v", result.Turns)
	}
	if strings.Contains(result.Turns[1].Prompt, "sk-FAKEabcdefghijklmnop") {
		t.Errorf("prompt credential shape was not redacted: %q", result.Turns[1].Prompt)
	}
	for _, turn := range result.Turns {
		if !turn.Simulated || turn.Response == "" || turn.EvidenceRecordID == "" {
			t.Errorf("turn was not simulated or evidenced: %+v", turn)
		}
	}

	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records, err := store.Evidence(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected two chat evidence records, got %d", len(records))
	}
	var first, second evidence.Record
	if err := json.Unmarshal([]byte(records[0].JSON), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(records[1].JSON), &second); err != nil {
		t.Fatal(err)
	}
	if first.Kind != "provider" || second.Kind != "provider" {
		t.Fatalf("unexpected evidence kinds: %q, %q", first.Kind, second.Kind)
	}
	if second.PreviousDigest != first.DataDigest {
		t.Error("chat evidence chain is not linked")
	}
	raw, err := json.Marshal(second.Data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-FAKEabcdefghijklmnop") {
		t.Error("chat evidence contains an unredacted credential shape")
	}
}

func TestChatLiveProviderRequiresExplicitConfirmation(t *testing.T) {
	root := t.TempDir()
	initChatRepo(t, root)
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"chat", "--provider", "claude", "--message", "hello"}}
	if code := app.Run(context.Background()); code != ExitPolicy {
		t.Fatalf("live chat exited %d, want %d: %s", code, ExitPolicy, stdout.String())
	}
}
