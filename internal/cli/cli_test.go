package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
	"github.com/yoosuf/ae.sh/internal/framework"
)

func createMinimalFrameworkRoot(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(framework.Dir(root, framework.Skills), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestWorktreePathStaysInsideManagedDirectory(t *testing.T) {
	root := t.TempDir()
	path, err := worktreePath(root, "run-abc123")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".aef", "worktrees", "run-abc123")
	if path != want {
		t.Fatalf("unexpected worktree path %q, want %q", path, want)
	}
	for _, id := range []string{"", "../escape", "run/../escape", "run with space"} {
		if _, err := worktreePath(root, id); err == nil {
			t.Errorf("run id %q was accepted", id)
		}
	}
}

func TestValidateWorkspaceRejectsUnmanagedPaths(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, ".aef", "worktrees")
	inside := filepath.Join(base, "run-1")
	outside := filepath.Join(root, "user-owned")
	for _, dir := range []string{inside, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	want, err := filepath.EvalSymlinks(inside)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := validateWorkspace(root, inside)
	if err != nil || resolved != want {
		t.Fatalf("managed workspace was rejected: %q, %v", resolved, err)
	}
	if _, err := validateWorkspace(root, outside); err == nil {
		t.Fatal("unmanaged workspace was accepted")
	}
}

func TestValidateManagedWorktreeRejectsPlainDirectory(t *testing.T) {
	root := t.TempDir()
	if _, err := runGit(context.Background(), root, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".aef", "worktrees", "run-plain")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := validateManagedWorktree(context.Background(), root, path); err == nil {
		t.Fatal("a plain directory under .aef/worktrees was accepted as a managed worktree")
	}
}

func TestLoadProfileRejectsUnsafeNamesAndGates(t *testing.T) {
	root := t.TempDir()
	dir := framework.Dir(root, framework.Profiles, "custom")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("schemaVersion: \"1.0\"\nid: custom\nversion: \"1.0.0\"\nchecks:\n  - gate: \"rm -rf /\"\n    command: \"echo unsafe\"\n")
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadProfile(root, "custom"); err == nil {
		t.Fatal("profile with a non-identifier gate was accepted")
	}
	if _, err := config.LoadProfile(root, "../custom"); err == nil {
		t.Fatal("profile with a traversal name was accepted")
	}
}

func TestAuthoredEvaluationFixturesPassStructuralChecks(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	passed, total, problems, err := checkSkillFixtures(root)
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 || passed != total {
		t.Fatalf("structural checks passed %d/%d: %v", passed, total, problems)
	}
	ok, cases, problems, err := checkSecurityFixture(root)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || cases == 0 {
		t.Fatalf("security fixture check failed: ok=%t cases=%d problems=%v", ok, cases, problems)
	}
}

func TestBuildTraceReportMapsLifecycleStagesAndHonestGaps(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	run := execution.Run{
		ID: "run-trace-1", WorkflowID: "feature-development", WorkflowVersion: "1.0.0",
		IssueKey: "PROJ-101", IssuePath: "fixture", State: "blocked", Status: "blocked",
		Risk: "low", Provider: "fake", BaseSHA: "base", CandidateSHA: "candidate",
		PlanDigest: "sha256:plan", CurrentTask: 4, Profiles: []string{"go"},
		Actor: "tester", TerminalReason: "missing required evidence: unit-tests",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", Version: 1,
	}
	store, err := sqliteadapter.Open(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	manager := evidence.New(filepath.Join(dir, "evidence"), filepath.Join(dir, "objects"))
	seq := 0
	add := func(kind, subject string, data map[string]any, exit *int) {
		t.Helper()
		record, err := manager.Write(ctx, run.ID, seq, kind, subject, "test", data, exit, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AppendEvidence(ctx, execution.EvidenceRecord{RunID: run.ID, JSON: string(raw)}); err != nil {
			t.Fatal(err)
		}
		seq++
	}
	zero := 0
	add("plan", "Normalized plan", map[string]any{"issue": "PROJ-101", "taskOrder": []string{"intake"}}, nil)
	add("risk", "Task task classify-risk", map[string]any{"gate": "architecture", "skill": "risk-assessment"}, nil)
	add("test", "Verify build", map[string]any{"gate": "build", "command": "go build ./...", "passed": true, "simulated": false}, &zero)
	add("review", "Task task review", map[string]any{"verified": false, "findings": []any{}}, nil)
	add("gate", "Gate merge", map[string]any{"gate": "merge", "decision": "approved"}, nil)
	if err := store.AppendTransition(ctx, execution.Transition{
		RunID: run.ID, From: "new", To: "analyzing", TaskID: "requirements-intake", Timestamp: run.CreatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	for _, gate := range []string{"architecture", "merge"} {
		if err := store.SaveApproval(ctx, execution.ApprovalRecord{
			RunID: run.ID, Gate: gate, Approver: "reviewer-1", Role: "reviewer",
			IdentitySource: "local-manual", ArtifactDigest: run.PlanDigest,
			CandidateSHA: run.CandidateSHA, Timestamp: run.CreatedAt,
		}); err != nil {
			t.Fatal(err)
		}
	}

	report, err := buildTraceReport(ctx, store, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Stages) != 7 || len(report.EvidenceChain) != 5 || len(report.Approvals) != 2 {
		t.Fatalf("unexpected trace shape: stages=%d chain=%d approvals=%d", len(report.Stages), len(report.EvidenceChain), len(report.Approvals))
	}
	stages := map[string]traceStage{}
	for _, stage := range report.Stages {
		stages[stage.ID] = stage
	}
	for _, id := range []string{"requirements", "architecture", "testing", "review", "release"} {
		if stages[id].Status == "missing" {
			t.Errorf("stage %s was not mapped from evidence", id)
		}
	}
	if stages["feedback"].Status != "missing" {
		t.Errorf("feedback stage should remain missing, got %s", stages["feedback"].Status)
	}
	if len(stages["requirements"].Transitions) == 0 {
		t.Error("requirements transition was not mapped")
	}
	markdown := traceMarkdown(report)
	if !strings.Contains(markdown, "| Operational feedback | missing |") {
		t.Error("markdown report did not show the missing feedback stage")
	}
}

func TestInitAcceptsExpandedProfilesAndRejectsUnknown(t *testing.T) {
	root := t.TempDir()
	createMinimalFrameworkRoot(t, root)
	var stdout, stderr bytes.Buffer
	app := App{
		Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"init", "--profile", "java,python,laravel,typescript,all"},
	}
	if code := app.Run(context.Background()); code != ExitOK {
		t.Fatalf("init exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	cfg, err := config.Load(filepath.Join(root, "aef.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 5 {
		t.Fatalf("unexpected profiles: %v", cfg.Profiles)
	}
	for _, name := range []string{"java", "python", "laravel", "typescript", "all"} {
		found := false
		for _, selected := range cfg.Profiles {
			if selected == name {
				found = true
			}
		}
		if !found {
			t.Errorf("profile %q was not persisted", name)
		}
	}

	badRoot := t.TempDir()
	var badOut, badErr bytes.Buffer
	badApp := App{
		Stdout: &badOut, Stderr: &badErr, Root: badRoot, JSON: true,
		Args: []string{"init", "--profile", "unknown-stack"},
	}
	if code := badApp.Run(context.Background()); code != ExitUsage {
		t.Fatalf("unknown profile exited %d, want %d: %s", code, ExitUsage, badOut.String())
	}
	if _, err := os.Stat(filepath.Join(badRoot, "aef.yaml")); !os.IsNotExist(err) {
		t.Fatalf("config was written after a profile validation error: %v", err)
	}
}

func TestInitRejectsRootsWithoutCanonicalFrameworkAssets(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	app := App{
		Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"init", "--profile", "go"},
	}
	if code := app.Run(context.Background()); code != ExitValidation {
		t.Fatalf("init exited %d for a non-framework root: %s %s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "aef.yaml")); !os.IsNotExist(err) {
		t.Fatalf("init wrote configuration for a non-framework root: %v", err)
	}
}

func TestRunEmitsSingleJSONErrorEnvelopeForUsageErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Args: []string{"trace"}, JSON: true}
	if code := app.Run(context.Background()); code != ExitUsage {
		t.Fatalf("unexpected exit code %d", code)
	}
	decoder := json.NewDecoder(&stdout)
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("stdout was not a JSON error envelope: %v", err)
	}
	if decoder.More() {
		t.Fatal("usage error emitted more than one JSON object")
	}
	if payload["exitCode"] != float64(ExitUsage) {
		t.Fatalf("unexpected envelope: %+v", payload)
	}
	if strings.TrimSpace(payload["error"].(string)) == "" {
		t.Fatal("error envelope has no message")
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON mode wrote to stderr: %s", stderr.String())
	}
}

func TestRunRejectsUnsafeLiveAndWriteFlags(t *testing.T) {
	root, fixture := initAutopilotRepo(t, false)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "fake-writes",
			args: []string{"feature-development", "--issue", fixture, "--provider", "fake", "--allow-writes"},
			want: "--allow-writes",
		},
		{
			name: "live-without-workspace",
			args: []string{"feature-development", "--issue", fixture, "--provider", "claude", "--confirm-live"},
			want: "--workspace",
		},
		{
			name: "live-without-confirmation",
			args: []string{"feature-development", "--issue", fixture, "--provider", "claude"},
			want: "--confirm-live",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
				Args: append([]string{"run"}, tc.args...)}
			code := app.Run(context.Background())
			if code != ExitPolicy {
				t.Fatalf("expected policy exit, got %d: %s %s", code, stdout.String(), stderr.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(fmt.Sprint(payload["error"]), tc.want) {
				t.Fatalf("policy detail missing %q: %+v", tc.want, payload)
			}
		})
	}
}
