package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoosuf/ae.sh/internal/execution"
)

func testRun(id string) execution.Run {
	return execution.Run{
		ID: id, WorkflowID: "test-workflow", WorkflowVersion: "1.0.0",
		IssueKey: "TEST-1", IssuePath: "fixture", State: "queued", Status: "running",
		Risk: "low", Provider: "fake", BaseSHA: "base", CandidateSHA: "candidate",
		PlanDigest: "sha256:plan", Workspace: ".aef/worktrees/run-1", CurrentTask: 0, Profiles: []string{"go"},
		Actor: "tester", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", Version: 1,
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "aef.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	run := testRun("run-1")
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStep(ctx, execution.Transition{
		RunID: run.ID, From: "queued", To: "analyzing", TaskID: "intake", Timestamp: run.CreatedAt,
	}, &execution.EvidenceRecord{RunID: run.ID, JSON: `{"recordID":"ev-1"}`}); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"author", "reviewer"} {
		err := store.SaveApproval(ctx, execution.ApprovalRecord{
			RunID: run.ID, Gate: "architecture", Approver: role + "-1", Role: role,
			IdentitySource: "local-manual", ArtifactDigest: run.PlanDigest,
			CandidateSHA: run.CandidateSHA, ReviewDigest: "sha256:review", Timestamp: run.CreatedAt,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Enqueue(ctx, "notification", `{"kind":"test"}`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := reopened.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 1 || loaded.State != "queued" || len(loaded.Profiles) != 1 || loaded.Workspace != ".aef/worktrees/run-1" {
		t.Fatalf("run did not survive restart: %+v", loaded)
	}
	if transitions, err := reopened.Transitions(ctx, run.ID); err != nil || len(transitions) != 1 || transitions[0].To != "analyzing" {
		t.Fatalf("transitions did not survive restart: %+v, %v", transitions, err)
	}
	if records, err := reopened.Evidence(ctx, run.ID); err != nil || len(records) != 1 {
		t.Fatalf("evidence did not survive restart: %+v, %v", records, err)
	}
	if approvals, err := reopened.Approvals(ctx, run.ID, "architecture"); err != nil || len(approvals) != 2 {
		t.Fatalf("approvals did not survive restart: %+v, %v", approvals, err)
	} else {
		for _, approval := range approvals {
			if approval.ReviewDigest != "sha256:review" {
				t.Fatalf("review digest did not survive restart: %+v", approval)
			}
		}
	}
	if outbox, err := reopened.Outbox(ctx, ""); err != nil || len(outbox) != 1 {
		t.Fatalf("outbox did not survive restart: %+v, %v", outbox, err)
	}
}

func TestUpdateRunEnforcesOptimisticConcurrency(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "aef.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run := testRun("run-2")
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	run.State = "analyzing"
	if err := store.UpdateRun(ctx, run, 99); err == nil {
		t.Fatal("stale version must be rejected")
	} else {
		var conflict execution.ErrConflict
		if !errors.As(err, &conflict) {
			t.Fatalf("unexpected conflict error type: %T", err)
		}
	}
	if err := store.UpdateRun(ctx, run, 1); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetRun(ctx, run.ID)
	if err != nil || loaded.Version != 2 || loaded.State != "analyzing" {
		t.Fatalf("unexpected run after update: %+v, %v", loaded, err)
	}
}

func TestBindWorkspaceDoesNotRepointRun(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "aef.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run := testRun("run-bind")
	run.Workspace = ""
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	workspace := ".aef/worktrees/run-bind"
	if err := store.BindWorkspace(ctx, run.ID, workspace); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetRun(ctx, run.ID)
	if err != nil || loaded.Workspace != workspace || loaded.Version != 2 {
		t.Fatalf("workspace was not bound: %+v, %v", loaded, err)
	}
	if err := store.BindWorkspace(ctx, run.ID, ".aef/worktrees/other"); err == nil {
		t.Fatal("run workspace was silently re-pointed")
	} else {
		var conflict execution.ErrConflict
		if !errors.As(err, &conflict) {
			t.Fatalf("unexpected conflict error type: %T", err)
		}
	}
	if err := store.BindWorkspace(ctx, run.ID, workspace); err != nil {
		t.Fatalf("idempotent bind failed: %v", err)
	}
	loaded, err = store.GetRun(ctx, run.ID)
	if err != nil || loaded.Version != 2 {
		t.Fatalf("idempotent bind changed the run version: %+v, %v", loaded, err)
	}
}

func TestLegacyApprovalsAreMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE approvals (
  run_id TEXT NOT NULL,
  gate TEXT NOT NULL,
  approver TEXT NOT NULL,
  identity_source TEXT NOT NULL,
  artifact_digest TEXT NOT NULL,
  candidate_sha TEXT NOT NULL,
  timestamp TEXT NOT NULL,
  PRIMARY KEY (run_id, gate)
)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`INSERT INTO approvals VALUES (?,?,?,?,?,?,?)`,
		"run-legacy", "architecture", "reviewer-1", "local-manual", "sha256:plan", "candidate", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	approvals, err := store.Approvals(context.Background(), "run-legacy", "architecture")
	if err != nil || len(approvals) != 1 {
		t.Fatalf("legacy approval was not migrated: %+v, %v", approvals, err)
	}
	raw, err := json.Marshal(approvals[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"role":"local-manual"`) {
		t.Errorf("migrated approval has no role: %s", raw)
	}
	if approvals[0].ReviewDigest != "" {
		t.Errorf("migrated approval should have an empty review digest: %s", raw)
	}
}

func TestLegacyRunsGainWorkspaceColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-runs.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE runs (
  id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, workflow_version TEXT NOT NULL,
  issue_key TEXT NOT NULL, issue_path TEXT NOT NULL, state TEXT NOT NULL, status TEXT NOT NULL,
  risk TEXT NOT NULL, provider TEXT NOT NULL, base_sha TEXT NOT NULL, candidate_sha TEXT NOT NULL,
  plan_digest TEXT NOT NULL, current_task INTEGER NOT NULL, remediation_attempts INTEGER NOT NULL,
  profiles TEXT NOT NULL, actor TEXT NOT NULL, terminal_reason TEXT NOT NULL,
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL, version INTEGER NOT NULL
)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run := testRun("run-workspace-migration")
	run.Workspace = ".aef/worktrees/run-workspace-migration"
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run after migration: %v", err)
	}
	loaded, err := store.GetRun(context.Background(), run.ID)
	if err != nil || loaded.Workspace != run.Workspace {
		t.Fatalf("workspace was not migrated: %+v, %v", loaded, err)
	}
}
