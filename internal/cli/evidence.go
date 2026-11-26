package cli

import (
	"context"
	"encoding/json"
	"fmt"

	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
	"github.com/yoosuf/ae.sh/internal/integration"
)

// appendRunEvidence writes one sanitized evidence record to a run. Sequence
// allocation and predecessor selection happen inside one SQLite transaction so
// concurrent reviewers cannot fork the audit chain. File persistence happens
// after the row commits and is idempotent for immutable records.
func appendRunEvidence(ctx context.Context, cfg config.Config, store *sqliteadapter.Store, runID, kind, subject, actor string, data map[string]any, exitStatus *int, usage *integration.Usage) (evidence.Record, error) {
	manager := evidence.New(cfg.EvidenceDir(), cfg.ObjectsDir())
	var built evidence.Record
	_, err := store.AppendEvidenceChained(ctx, runID, func(sequence int, previous string) (execution.EvidenceRecord, error) {
		record, err := manager.Build(ctx, runID, sequence, kind, subject, actor, data, exitStatus, usage, previous)
		if err != nil {
			return execution.EvidenceRecord{}, err
		}
		built = record
		encoded, err := json.Marshal(record)
		if err != nil {
			return execution.EvidenceRecord{}, fmt.Errorf("encode evidence record: %w", err)
		}
		return execution.EvidenceRecord{RunID: runID, Sequence: sequence, JSON: string(encoded)}, nil
	})
	if err != nil {
		return evidence.Record{}, err
	}
	if err := manager.Persist(built); err != nil {
		return evidence.Record{}, err
	}
	return built, nil
}
