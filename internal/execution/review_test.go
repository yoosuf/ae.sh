package execution_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
)

func TestCollectReviewEvidenceReconstructsResolutionAfterRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := sqliteadapter.Open(filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run := execution.Run{
		ID: "run-review-1", WorkflowID: "local-flow", WorkflowVersion: "1.0.0",
		IssueKey: "TEST-1", State: "awaiting_merge", Status: "running", Risk: "low",
		CandidateSHA: "offline-abc123", Actor: "tester", Version: 1,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	manager := evidence.New(filepath.Join(root, ".aef", "evidence"), filepath.Join(root, ".aef", "objects"))
	reviewRecord, err := manager.Write(ctx, run.ID, 0, "review", "Local diff pattern scan", "aef-review", map[string]any{
		"candidateSHA": run.CandidateSHA,
		"findings": []any{map[string]any{
			"id": "rev-0123456789abcdef", "ruleId": "credential-literal", "severity": "high",
		}},
	}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := appendEvidenceJSON(ctx, store, run.ID, reviewRecord); err != nil {
		t.Fatal(err)
	}
	state, err := execution.CollectReviewEvidence(ctx, store, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Findings) != 1 || len(state.Unresolved) != 1 || state.Unresolved[0] != "rev-0123456789abcdef" {
		t.Fatalf("finding was not reconstructed: %+v", state)
	}

	resolution, err := manager.Write(ctx, run.ID, 1, "remediation", "Resolve review finding rev-0123456789abcdef", "reviewer", map[string]any{
		"findingId": "rev-0123456789abcdef", "status": "fixed", "reason": "credential was removed",
	}, nil, nil, reviewRecord.DataDigest)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendEvidenceJSON(ctx, store, run.ID, resolution); err != nil {
		t.Fatal(err)
	}
	state, err = execution.CollectReviewEvidence(ctx, store, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Unresolved) != 0 || state.Resolved["rev-0123456789abcdef"].Status != "fixed" {
		t.Fatalf("resolution was not reconstructed: %+v", state)
	}
}

func TestCollectReviewEvidenceRejectsStaleReview(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := sqliteadapter.Open(filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run := execution.Run{ID: "run-review-2", CandidateSHA: "offline-current"}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	manager := evidence.New(filepath.Join(root, ".aef", "evidence"), filepath.Join(root, ".aef", "objects"))
	record, err := manager.Write(ctx, run.ID, 0, "review", "Local diff pattern scan", "aef-review", map[string]any{
		"candidateSHA": "offline-stale", "findings": []any{},
	}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := appendEvidenceJSON(ctx, store, run.ID, record); err != nil {
		t.Fatal(err)
	}
	if _, err := execution.CollectReviewEvidence(ctx, store, run); err == nil || !strings.Contains(err.Error(), "stale candidate SHA") {
		t.Fatalf("stale review was accepted: %v", err)
	}
}

func appendEvidenceJSON(ctx context.Context, store *sqliteadapter.Store, runID string, record evidence.Record) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return store.AppendEvidence(ctx, execution.EvidenceRecord{RunID: runID, JSON: string(raw)})
}
