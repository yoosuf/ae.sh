package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aef-framework/aef/internal/config"
	"github.com/aef-framework/aef/internal/framework"
)

const autopilotFixture = `{
  "schemaVersion": "1.0",
  "key": "AUTO-1",
  "summary": "Return the configured greeting",
  "description": "A local-only low-risk fixture used to exercise the autonomous loop without network calls or credentials.",
  "acceptanceCriteria": [
    "The greeting is deterministic.",
    "No credential or customer data is used.",
    "Local checks are recorded as evidence."
  ],
  "changedPaths": ["internal/greeting/greeting.go"],
  "semantics": ["business-logic"],
  "dependencies": [],
  "links": [],
  "baseSHA": "0fedcba98765432",
  "source": "fixture"
}`

const fastProfile = `schemaVersion: "1.0"
id: fast
version: "1.0.0"
language: Go
checks:
  - gate: format
    command: "true"
  - gate: lint
    command: "true"
  - gate: build
    command: "true"
  - gate: unit-tests
    command: "true"
  - gate: dependency-audit
    command: "true"
evidenceCommands:
  - "true"
`

func initAutopilotRepo(t *testing.T, withGit bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	source, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	copyCanonicalRepo(t, source, root)

	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"init"}}
	if code := app.Run(context.Background()); code != ExitOK {
		t.Fatalf("init exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	profileDir := framework.Dir(root, framework.Profiles, "fast")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "profile.yaml"), []byte(fastProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, config.FileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profiles = []string{"fast"}
	if err := config.Write(cfgPath, cfg, true); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(root, "examples", "autopilot", "issue-AUTO-1.json")
	if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixturePath, []byte(autopilotFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	if !withGit {
		return root, fixturePath
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("add", "-A")
	run("-c", "user.name=AEF", "-c", "user.email=aef@example.invalid", "commit", "-qm", "fixture", "--no-gpg-sign")
	return root, fixturePath
}

func copyCanonicalRepo(t *testing.T, source, root string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		top := strings.SplitN(rel, string(os.PathSeparator), 2)[0]
		switch top {
		case ".git", ".aef", "dist":
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == config.FileName {
			return nil
		}
		target := filepath.Join(root, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func runAutopilot(t *testing.T, root string, args ...string) (int, map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: append([]string{"autopilot"}, args...)}
	code := app.Run(context.Background())
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("parse autopilot output: %v: %s", err, stdout.String())
	}
	return code, payload, stderr.String()
}

func TestAutopilotDryRunWritesNoState(t *testing.T) {
	root, fixture := initAutopilotRepo(t, false)
	code, payload, stderr := runAutopilot(t, root, "--issue", fixture, "--dry-run")
	if code != ExitOK {
		t.Fatalf("dry run exited %d: %v %s", code, payload, stderr)
	}
	if payload["status"] != "dry-run" {
		t.Fatalf("unexpected dry-run status: %+v", payload)
	}
	if payload["runId"] != nil {
		t.Fatalf("dry run created a run: %+v", payload)
	}
	steps, ok := payload["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("dry run had no planned steps: %+v", payload)
	}
	if _, err := os.Stat(filepath.Join(root, ".aef", "state.db")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote state: %v", err)
	}
}

func TestAutopilotRejectsLiveProviderWithoutConfirmation(t *testing.T) {
	root, fixture := initAutopilotRepo(t, false)
	code, payload, _ := runAutopilot(t, root, "--issue", fixture, "--provider", "claude")
	if code != ExitPolicy {
		t.Fatalf("expected policy exit, got %d: %+v", code, payload)
	}
	if !strings.Contains(fmt.Sprint(payload["error"]), "--confirm-live") {
		t.Fatalf("live confirmation was not reported: %+v", payload)
	}
}

func TestAutopilotRejectsGitHubWithoutConfirmation(t *testing.T) {
	root, fixture := initAutopilotRepo(t, false)
	code, payload, _ := runAutopilot(t, root, "--issue", fixture, "--github-pr", "12")
	if code != ExitPolicy {
		t.Fatalf("expected policy exit, got %d: %+v", code, payload)
	}
}

func TestAutopilotStopsAtHumanGate(t *testing.T) {
	root, fixture := initAutopilotRepo(t, true)
	code, payload, stderr := runAutopilot(t, root, "--issue", fixture)
	if code != ExitOK {
		t.Fatalf("autopilot exited %d: %+v %s", code, payload, stderr)
	}
	if payload["status"] != "awaiting_approval" || payload["runId"] == nil {
		t.Fatalf("autopilot did not stop at the human gate: %+v", payload)
	}
	runState, ok := payload["run"].(map[string]any)
	if !ok || runState["workspace"] == "" || runState["workspace"] == nil {
		t.Fatalf("run was not bound to a managed worktree before provider tasks: %+v", payload)
	}
	if _, err := os.Stat(filepath.Join(root, ".aef", "worktrees")); err != nil {
		t.Fatalf("managed worktree was not prepared before provider tasks: %v", err)
	}
}

func TestCollectWorkspaceDiffIncludesUntrackedFiles(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	if _, err := runGit(ctx, workspace, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	secret := "sk-FAKEabcdefghijklmnop"
	body := "package main\nvar token = \"" + secret + "\"\n"
	if err := os.WriteFile(filepath.Join(workspace, "new.go"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	diff, err := collectWorkspaceDiff(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+var token = \""+secret+"\"") {
		t.Fatalf("untracked file was omitted from the review diff: %q", diff)
	}
	if err := os.Symlink(filepath.Join(workspace, "new.go"), filepath.Join(workspace, "link.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := collectWorkspaceDiff(ctx, workspace); err == nil {
		t.Fatal("an untracked symlink was accepted for diff review")
	}
}

func TestAutopilotRejectsSelfApproval(t *testing.T) {
	root, fixture := initAutopilotRepo(t, true)
	code, payload, _ := runAutopilot(t, root,
		"--issue", fixture, "--approver", "local-user", "--confirm-gates")
	if code != ExitPolicy {
		t.Fatalf("expected policy exit, got %d: %+v", code, payload)
	}
	if !strings.Contains(fmt.Sprint(payload["error"]), "self-approval") {
		t.Fatalf("self-approval was not reported: %+v", payload)
	}
}

func TestAutopilotLocalFakeFlowWithDelegatedApproval(t *testing.T) {
	root, fixture := initAutopilotRepo(t, true)
	code, payload, stderr := runAutopilot(t, root,
		"--issue", fixture, "--provider", "fake",
		"--approver", "reviewer-1", "--confirm-gates")
	if code != ExitOK {
		t.Fatalf("autopilot exited %d: %+v %s", code, payload, stderr)
	}
	if payload["status"] != "completed" {
		t.Fatalf("autopilot did not complete: %+v", payload)
	}
	run, ok := payload["run"].(map[string]any)
	if !ok || run["state"] != "completed" {
		t.Fatalf("run did not reach completed state: %+v", payload)
	}
	steps, ok := payload["steps"].([]any)
	if !ok || len(steps) < 6 {
		t.Fatalf("autopilot steps were not recorded: %+v", payload)
	}
	workspace := filepath.Join(root, ".aef", "worktrees", fmt.Sprint(run["id"]))
	if _, err := os.Stat(workspace); err != nil {
		t.Fatalf("managed worktree was not used: %v", err)
	}
}
