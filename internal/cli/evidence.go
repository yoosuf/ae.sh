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

// appendRunEvidence writes one sanitized evidence record to a run. Missing
// prior records simply start the chain; malformed prior records fail closed.
func appendRunEvidence(ctx context.Context, cfg config.Config, store *sqliteadapter.Store, runID, kind, subject, actor string, data map[string]any, exitStatus *int, usage *integration.Usage) (evidence.Record, error) {
	records, err := store.Evidence(ctx, runID)
	if err != nil {
		return evidence.Record{}, err
	}
	previous := ""
	if len(records) > 0 {
		var last evidence.Record
		if err := json.Unmarshal([]byte(records[len(records)-1].JSON), &last); err != nil {
			return evidence.Record{}, fmt.Errorf("parse previous evidence record: %w", err)
		}
		previous = last.DataDigest
	}
	manager := evidence.New(cfg.EvidenceDir(), cfg.ObjectsDir())
	record, err := manager.Write(ctx, runID, len(records), kind, subject, actor, data, exitStatus, usage, previous)
	if err != nil {
		return evidence.Record{}, err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return evidence.Record{}, err
	}
	if err := store.AppendEvidence(ctx, execution.EvidenceRecord{RunID: runID, JSON: string(encoded)}); err != nil {
		return evidence.Record{}, err
	}
	return record, nil
}
