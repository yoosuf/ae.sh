// Package sqlite implements the execution Store with pure-Go SQLite.
// It is the authoritative local state model for AEF.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/yoosuf/ae.sh/internal/execution"
	"github.com/yoosuf/ae.sh/internal/util"
)

// Store is a SQLite-backed execution store.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  workflow_id TEXT NOT NULL,
  workflow_version TEXT NOT NULL,
  issue_key TEXT NOT NULL,
  issue_path TEXT NOT NULL,
  state TEXT NOT NULL,
  status TEXT NOT NULL,
  risk TEXT NOT NULL,
  provider TEXT NOT NULL,
  base_sha TEXT NOT NULL,
  candidate_sha TEXT NOT NULL,
  plan_digest TEXT NOT NULL,
  workspace TEXT NOT NULL DEFAULT '',
  current_task INTEGER NOT NULL,
  remediation_attempts INTEGER NOT NULL,
  profiles TEXT NOT NULL,
  actor TEXT NOT NULL,
  terminal_reason TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  version INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS transitions (
  run_id TEXT NOT NULL,
  sequence INTEGER NOT NULL,
  from_state TEXT NOT NULL,
  to_state TEXT NOT NULL,
  task_id TEXT NOT NULL,
  timestamp TEXT NOT NULL,
  evidence_digest TEXT NOT NULL,
  PRIMARY KEY (run_id, sequence)
);
CREATE TABLE IF NOT EXISTS approvals (
  run_id TEXT NOT NULL,
  gate TEXT NOT NULL,
  approver TEXT NOT NULL,
  role TEXT NOT NULL,
  identity_source TEXT NOT NULL,
  artifact_digest TEXT NOT NULL,
  candidate_sha TEXT NOT NULL,
  review_digest TEXT NOT NULL DEFAULT '',
  timestamp TEXT NOT NULL,
  PRIMARY KEY (run_id, gate, approver)
);
CREATE TABLE IF NOT EXISTS evidence (
  run_id TEXT NOT NULL,
  sequence INTEGER NOT NULL,
  json TEXT NOT NULL,
  PRIMARY KEY (run_id, sequence)
);
CREATE TABLE IF NOT EXISTS artifacts (
  digest TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  raw BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  payload TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runs_state ON runs(state);
CREATE INDEX IF NOT EXISTS idx_transitions_run ON transitions(run_id, sequence);
CREATE INDEX IF NOT EXISTS idx_evidence_run ON evidence(run_id, sequence);
`

// Open creates or opens the state database and applies migrations.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=FULL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	legacy, err := legacyApprovalsPresent(db)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("inspect approvals schema: %w", err)
	}
	if legacy {
		if _, err := db.Exec(`ALTER TABLE approvals RENAME TO approvals_legacy`); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("migrate approvals: %w", err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	hasWorkspace, err := columnExists(db, "runs", "workspace")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("inspect runs schema: %w", err)
	}
	if !hasWorkspace {
		if _, err := db.Exec(`ALTER TABLE runs ADD COLUMN workspace TEXT NOT NULL DEFAULT ''`); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("migrate runs workspace: %w", err)
		}
	}
	hasReviewDigest, err := columnExists(db, "approvals", "review_digest")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("inspect approvals schema: %w", err)
	}
	if !hasReviewDigest {
		if _, err := db.Exec(`ALTER TABLE approvals ADD COLUMN review_digest TEXT NOT NULL DEFAULT ''`); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("migrate approvals review digest: %w", err)
		}
	}
	if legacy {
		copySQL := `INSERT OR IGNORE INTO approvals
(run_id, gate, approver, role, identity_source, artifact_digest, candidate_sha, review_digest, timestamp)
SELECT run_id, gate, approver, 'local-manual', identity_source, artifact_digest, candidate_sha, '', timestamp
FROM approvals_legacy`
		if _, err := db.Exec(copySQL); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("copy approvals: %w", err)
		}
		if _, err := db.Exec(`DROP TABLE approvals_legacy`); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("drop legacy approvals: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func legacyApprovalsPresent(db *sql.DB) (bool, error) {
	var definition string
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='approvals'`).Scan(&definition)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(definition, "PRIMARY KEY (run_id, gate)"), nil
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) CreateRun(ctx context.Context, run execution.Run) error {
	if run.Version == 0 {
		run.Version = 1
	}
	profiles, err := json.Marshal(run.Profiles)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runs (id, workflow_id, workflow_version, issue_key, issue_path, state, status,
  risk, provider, base_sha, candidate_sha, plan_digest, workspace, current_task, remediation_attempts,
  profiles, actor, terminal_reason, created_at, updated_at, version)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		run.ID, run.WorkflowID, run.WorkflowVersion, run.IssueKey, run.IssuePath, run.State, run.Status,
		run.Risk, run.Provider, run.BaseSHA, run.CandidateSHA, run.PlanDigest, run.Workspace, run.CurrentTask,
		run.RemediationAttempts, string(profiles), run.Actor, run.TerminalReason, run.CreatedAt,
		run.UpdatedAt, run.Version)
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	return nil
}

func (s *Store) GetRun(ctx context.Context, id string) (execution.Run, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, workflow_id, workflow_version, issue_key, issue_path,
  state, status, risk, provider, base_sha, candidate_sha, plan_digest, workspace, current_task,
  remediation_attempts, profiles, actor, terminal_reason, created_at, updated_at, version
  FROM runs WHERE id = ?`, id)
	var run execution.Run
	var profiles string
	err := row.Scan(&run.ID, &run.WorkflowID, &run.WorkflowVersion, &run.IssueKey, &run.IssuePath,
		&run.State, &run.Status, &run.Risk, &run.Provider, &run.BaseSHA, &run.CandidateSHA,
		&run.PlanDigest, &run.Workspace, &run.CurrentTask, &run.RemediationAttempts, &profiles, &run.Actor,
		&run.TerminalReason, &run.CreatedAt, &run.UpdatedAt, &run.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return run, execution.ErrNotFound{Kind: "run", ID: id}
	}
	if err != nil {
		return run, err
	}
	if err := json.Unmarshal([]byte(profiles), &run.Profiles); err != nil {
		return run, err
	}
	return run, nil
}

func (s *Store) UpdateRun(ctx context.Context, run execution.Run, expectedVersion int64) error {
	profiles, err := json.Marshal(run.Profiles)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE runs SET state=?, status=?, risk=?, provider=?, base_sha=?, candidate_sha=?, plan_digest=?,
  workspace=?, current_task=?, remediation_attempts=?, profiles=?, actor=?, terminal_reason=?, updated_at=?,
  version=version+1
WHERE id=? AND version=?`,
		run.State, run.Status, run.Risk, run.Provider, run.BaseSHA, run.CandidateSHA, run.PlanDigest,
		run.Workspace, run.CurrentTask, run.RemediationAttempts, string(profiles), run.Actor, run.TerminalReason,
		run.UpdatedAt, run.ID, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return execution.ErrConflict{ID: run.ID}
	}
	return nil
}

// BindWorkspace attaches a validated AEF-managed worktree to a run. A run that
// already has a workspace is never silently re-pointed at another directory.
func (s *Store) BindWorkspace(ctx context.Context, runID, workspace string) error {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if workspace == "" {
		return fmt.Errorf("workspace cannot be empty")
	}
	if run.Workspace != "" && run.Workspace != workspace {
		return execution.ErrConflict{ID: runID}
	}
	if run.Workspace == workspace {
		return nil
	}
	run.Workspace = workspace
	run.UpdatedAt = util.Now()
	return s.UpdateRun(ctx, run, run.Version)
}

func (s *Store) ListRuns(ctx context.Context, limit int) ([]execution.Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM runs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]execution.Run, 0, len(ids))
	for _, id := range ids {
		run, err := s.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}

func (s *Store) AppendTransition(ctx context.Context, t execution.Transition) error {
	return s.AppendStep(ctx, t, nil)
}

func (s *Store) AppendStep(ctx context.Context, t execution.Transition, record *execution.EvidenceRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), -1) + 1 FROM transitions WHERE run_id=?`, t.RunID).Scan(&next); err != nil {
		return err
	}
	t.Sequence = next
	if _, err := tx.ExecContext(ctx, `INSERT INTO transitions (run_id, sequence, from_state, to_state, task_id, timestamp, evidence_digest)
VALUES (?,?,?,?,?,?,?)`, t.RunID, t.Sequence, t.From, t.To, t.TaskID, t.Timestamp, t.EvidenceDigest); err != nil {
		return err
	}
	if record != nil {
		var evidenceNext int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), -1) + 1 FROM evidence WHERE run_id=?`, t.RunID).Scan(&evidenceNext); err != nil {
			return err
		}
		record.RunID = t.RunID
		record.Sequence = evidenceNext
		if _, err := tx.ExecContext(ctx, `INSERT INTO evidence (run_id, sequence, json) VALUES (?,?,?)`, record.RunID, record.Sequence, record.JSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Transitions(ctx context.Context, runID string) ([]execution.Transition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT run_id, sequence, from_state, to_state, task_id, timestamp, evidence_digest
FROM transitions WHERE run_id=? ORDER BY sequence`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []execution.Transition
	for rows.Next() {
		var t execution.Transition
		if err := rows.Scan(&t.RunID, &t.Sequence, &t.From, &t.To, &t.TaskID, &t.Timestamp, &t.EvidenceDigest); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SaveApproval(ctx context.Context, a execution.ApprovalRecord) error {
	if a.Role == "" {
		a.Role = "local-manual"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO approvals (run_id, gate, approver, role, identity_source, artifact_digest, candidate_sha, review_digest, timestamp)
VALUES (?,?,?,?,?,?,?,?,?)`, a.RunID, a.Gate, a.Approver, a.Role, a.IdentitySource, a.ArtifactDigest, a.CandidateSHA, a.ReviewDigest, a.Timestamp)
	return err
}

func (s *Store) Approval(ctx context.Context, runID, gate string) (execution.ApprovalRecord, error) {
	all, err := s.Approvals(ctx, runID, gate)
	if err != nil {
		return execution.ApprovalRecord{}, err
	}
	if len(all) == 0 {
		return execution.ApprovalRecord{}, execution.ErrNotFound{Kind: "approval", ID: runID + "/" + gate}
	}
	return all[0], nil
}

func (s *Store) Approvals(ctx context.Context, runID, gate string) ([]execution.ApprovalRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT run_id, gate, approver, role, identity_source, artifact_digest, candidate_sha, review_digest, timestamp
FROM approvals WHERE run_id=? AND gate=? ORDER BY timestamp, approver`, runID, gate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []execution.ApprovalRecord
	for rows.Next() {
		var a execution.ApprovalRecord
		if err := rows.Scan(&a.RunID, &a.Gate, &a.Approver, &a.Role, &a.IdentitySource,
			&a.ArtifactDigest, &a.CandidateSHA, &a.ReviewDigest, &a.Timestamp); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AppendEvidence(ctx context.Context, record execution.EvidenceRecord) error {
	var next int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), -1) + 1 FROM evidence WHERE run_id=?`, record.RunID).Scan(&next); err != nil {
		return err
	}
	record.Sequence = next
	_, err := s.db.ExecContext(ctx, `INSERT INTO evidence (run_id, sequence, json) VALUES (?,?,?)`, record.RunID, record.Sequence, record.JSON)
	return err
}

func (s *Store) Evidence(ctx context.Context, runID string) ([]execution.EvidenceRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT run_id, sequence, json FROM evidence WHERE run_id=? ORDER BY sequence`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []execution.EvidenceRecord
	for rows.Next() {
		var record execution.EvidenceRecord
		if err := rows.Scan(&record.RunID, &record.Sequence, &record.JSON); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Store) PutArtifact(ctx context.Context, a execution.Artifact) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO artifacts (digest, kind, raw) VALUES (?,?,?)`, a.Digest, a.Kind, a.Raw)
	return err
}

func (s *Store) GetArtifact(ctx context.Context, digest string) (execution.Artifact, error) {
	var a execution.Artifact
	err := s.db.QueryRowContext(ctx, `SELECT digest, kind, raw FROM artifacts WHERE digest=?`, digest).Scan(&a.Digest, &a.Kind, &a.Raw)
	if errors.Is(err, sql.ErrNoRows) {
		return a, execution.ErrNotFound{Kind: "artifact", ID: digest}
	}
	return a, err
}

func (s *Store) Enqueue(ctx context.Context, kind, payload string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO outbox (run_id, kind, payload, created_at) VALUES (?,?,?,?)`, "", kind, payload, nowUTC())
	return err
}

func (s *Store) Outbox(ctx context.Context, runID string) ([]execution.OutboxEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, run_id, kind, payload, created_at FROM outbox ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []execution.OutboxEntry
	for rows.Next() {
		var e execution.OutboxEntry
		if err := rows.Scan(&e.ID, &e.RunID, &e.Kind, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func nowUTC() string { return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339) }
