// Package evidence creates content-addressed, immutable audit records. Missing
// checks are recorded as missing; they are never marked as passed.
package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yoosuf/ae.sh/internal/integration"
	"github.com/yoosuf/ae.sh/internal/schema"
	"github.com/yoosuf/ae.sh/internal/util"
)

// Record is one evidence entry.
type Record struct {
	SchemaVersion  string             `json:"schemaVersion"`
	RecordID       string             `json:"recordID"`
	RunID          string             `json:"runID"`
	Sequence       int                `json:"sequence"`
	Timestamp      string             `json:"timestamp"`
	Kind           string             `json:"kind"`
	Subject        string             `json:"subject"`
	Data           map[string]any     `json:"data"`
	DataDigest     string             `json:"dataDigest"`
	PreviousDigest string             `json:"previousDigest,omitempty"`
	Actor          string             `json:"actor,omitempty"`
	ExitStatus     *int               `json:"exitStatus"`
	Usage          *integration.Usage `json:"usage,omitempty"`
}

// Manager writes evidence records and their content-addressed objects.
type Manager struct {
	Dir        string
	ObjectsDir string
}

var evidenceNamePattern = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// New creates an evidence manager.
func New(dir, objectsDir string) *Manager {
	return &Manager{Dir: dir, ObjectsDir: objectsDir}
}

// Write builds, validates, stores and exports one evidence record.
func (m *Manager) Write(ctx context.Context, runID string, sequence int, kind, subject, actor string, data map[string]any, exitStatus *int, usage *integration.Usage, previous string) (Record, error) {
	if data == nil {
		data = map[string]any{}
	}
	safe := util.SanitizeValue(data).(map[string]any)
	raw, err := json.Marshal(safe)
	if err != nil {
		return Record{}, err
	}
	record := Record{
		SchemaVersion:  "1.0",
		RecordID:       util.NewID("ev"),
		RunID:          util.Sanitize(runID),
		Sequence:       sequence,
		Timestamp:      util.Now(),
		Kind:           util.Sanitize(kind),
		Subject:        util.Sanitize(subject),
		Data:           safe,
		DataDigest:     util.Digest(raw),
		PreviousDigest: util.Sanitize(previous),
		Actor:          util.Sanitize(actor),
		ExitStatus:     exitStatus,
		Usage:          usage,
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Record{}, err
	}
	generic, err := util.JSONValue(record)
	if err != nil {
		return Record{}, err
	}
	if err := schema.Validate("evidence", generic); err != nil {
		return Record{}, err
	}
	dir := filepath.Join(m.Dir, runID)
	if err := util.EnsureDir(dir); err != nil {
		return Record{}, err
	}
	name := fmt.Sprintf("%04d-%s.json", sequence, evidenceNamePattern.ReplaceAllString(record.Kind, "-"))
	if err := os.WriteFile(filepath.Join(dir, name), append(encoded, '\n'), 0o600); err != nil {
		return Record{}, err
	}
	objectDir := filepath.Join(m.ObjectsDir, strings.TrimPrefix(record.DataDigest, "sha256:")[:2])
	if err := util.EnsureDir(objectDir); err != nil {
		return Record{}, err
	}
	if err := os.WriteFile(filepath.Join(objectDir, strings.TrimPrefix(record.DataDigest, "sha256:")), raw, 0o600); err != nil {
		return Record{}, err
	}
	return record, nil
}

// ExportJSON writes a complete run evidence bundle.
func ExportJSON(records []Record, path string) error {
	raw, err := json.MarshalIndent(map[string]any{
		"schemaVersion": "1.0",
		"recordCount":   len(records),
		"records":       records,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

// ExportMarkdown writes a human-readable evidence bundle.
func ExportMarkdown(records []Record, path string) error {
	var sb strings.Builder
	sb.WriteString("# Evidence export\n\n")
	fmt.Fprintf(&sb, "Records: %d\n\n", len(records))
	for _, record := range records {
		fmt.Fprintf(&sb, "## %04d — %s (%s)\n\n", record.Sequence, record.Subject, record.Kind)
		fmt.Fprintf(&sb, "- Record ID: `%s`\n- Timestamp: %s\n- Actor: %s\n- Data digest: `%s`\n",
			record.RecordID, record.Timestamp, record.Actor, record.DataDigest)
		if record.PreviousDigest != "" {
			fmt.Fprintf(&sb, "- Previous digest: `%s`\n", record.PreviousDigest)
		}
		if record.ExitStatus != nil {
			fmt.Fprintf(&sb, "- Exit status: %d\n", *record.ExitStatus)
		}
		if record.Usage != nil {
			fmt.Fprintf(&sb, "- Usage known: %t\n", record.Usage.Known)
		}
		raw, err := json.MarshalIndent(record.Data, "", "  ")
		if err != nil {
			return err
		}
		sb.WriteString("\n```json\n")
		sb.Write(raw)
		sb.WriteString("\n```\n\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sb.String()), 0o600)
}

// SortRecords orders records by sequence.
func SortRecords(records []Record) {
	sort.Slice(records, func(i, j int) bool { return records[i].Sequence < records[j].Sequence })
}
