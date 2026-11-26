package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
	"github.com/yoosuf/ae.sh/internal/util"
)

const maxDashboardEvidenceDataBytes = 32 << 10

type dashboardEvidence struct {
	RecordID       string         `json:"recordId"`
	RunID          string         `json:"runId"`
	Sequence       int            `json:"sequence"`
	Timestamp      string         `json:"timestamp"`
	Kind           string         `json:"kind"`
	Subject        string         `json:"subject"`
	DataDigest     string         `json:"dataDigest"`
	PreviousDigest string         `json:"previousDigest,omitempty"`
	Actor          string         `json:"actor,omitempty"`
	ExitStatus     *int           `json:"exitStatus,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
	DataOmitted    bool           `json:"dataOmitted,omitempty"`
}

type dashboardFinding struct {
	ID           string `json:"id"`
	RuleID       string `json:"ruleId,omitempty"`
	Severity     string `json:"severity,omitempty"`
	RecordID     string `json:"recordId"`
	CandidateSHA string `json:"candidateSHA,omitempty"`
	Hypothesis   bool   `json:"hypothesis"`
}

type dashboardReview struct {
	Findings   []dashboardFinding           `json:"findings"`
	Resolved   []execution.ReviewResolution `json:"resolved"`
	Unresolved []string                     `json:"unresolved"`
	Digest     string                       `json:"digest"`
	Note       string                       `json:"note"`
}

type dashboardSnapshot struct {
	SchemaVersion string                     `json:"schemaVersion"`
	GeneratedAt   string                     `json:"generatedAt"`
	Run           execution.Run              `json:"run"`
	History       []execution.Transition     `json:"history"`
	Approvals     []executionApprovalSummary `json:"approvals"`
	Evidence      []dashboardEvidence        `json:"evidence"`
	Trace         traceReport                `json:"trace"`
	Review        dashboardReview            `json:"review"`
	Note          string                     `json:"note"`
}

func (a *App) cmdDashboard(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "snapshot" {
		return usage("dashboard requires: snapshot --run <id> --out DIR")
	}
	fs := newFlagSet("dashboard snapshot")
	runID := fs.String("run", "", "run id to snapshot")
	out := fs.String("out", "", "snapshot output directory")
	if err := fs.Parse(args[1:]); err != nil {
		return usage("dashboard snapshot: %v", err)
	}
	if fs.NArg() != 0 {
		return usage("dashboard snapshot takes no positional arguments")
	}
	if *runID == "" || *out == "" {
		return usage("dashboard snapshot requires --run <id> and --out DIR")
	}
	_, store, err := a.openStore()
	if err != nil {
		return err
	}
	defer store.Close()
	snapshot, err := buildDashboardSnapshot(ctx, store, *runID)
	if err != nil {
		return a.notFound(err)
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if util.SecretShapeCount(string(raw)) > 0 {
		return invalid("dashboard snapshot contains a credential shape and was not written")
	}
	dir := *out
	if err := util.EnsureDir(dir); err != nil {
		return err
	}
	files := []string{
		filepath.Join(dir, "snapshot.json"),
		filepath.Join(dir, "index.html"),
		filepath.Join(dir, "app.js"),
		filepath.Join(dir, "styles.css"),
	}
	html := strings.Replace(dashboardIndexHTML, "__AE_SNAPSHOT__", string(raw), 1)
	contents := []string{string(raw), html, dashboardAppJS, dashboardStylesCSS}
	for index, path := range files {
		if err := os.WriteFile(path, []byte(contents[index]+"\n"), 0o600); err != nil {
			return err
		}
	}
	a.emit(map[string]any{
		"runID": runID, "out": filepath.ToSlash(dir), "files": files,
		"stages":   len(snapshot.Trace.Stages),
		"evidence": len(snapshot.Evidence),
		"note":     "Read-only local snapshot. Open index.html directly; no server, API, credentials, or write path is used.",
	})
	return nil
}

func buildDashboardSnapshot(ctx context.Context, store *sqliteadapter.Store, runID string) (dashboardSnapshot, error) {
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return dashboardSnapshot{}, err
	}
	trace, err := buildTraceReport(ctx, store, run)
	if err != nil {
		return dashboardSnapshot{}, err
	}
	reviewState, err := execution.CollectReviewEvidence(ctx, store, run)
	if err != nil {
		return dashboardSnapshot{}, err
	}
	transitions, err := store.Transitions(ctx, runID)
	if err != nil {
		return dashboardSnapshot{}, err
	}
	stored, err := store.Evidence(ctx, runID)
	if err != nil {
		return dashboardSnapshot{}, err
	}
	records := make([]dashboardEvidence, 0, len(stored))
	for _, item := range stored {
		var record evidence.Record
		if err := json.Unmarshal([]byte(item.JSON), &record); err != nil {
			return dashboardSnapshot{}, invalid("stored evidence is malformed: %v", err)
		}
		projected := dashboardEvidence{
			RecordID: record.RecordID, RunID: record.RunID, Sequence: record.Sequence,
			Timestamp: record.Timestamp, Kind: record.Kind, Subject: record.Subject,
			DataDigest: record.DataDigest, PreviousDigest: record.PreviousDigest,
			Actor: record.Actor, ExitStatus: record.ExitStatus,
		}
		data, omitted := dashboardEvidenceData(record)
		projected.Data = data
		projected.DataOmitted = omitted
		records = append(records, projected)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Sequence < records[j].Sequence })
	findings := make([]dashboardFinding, 0, len(reviewState.Findings))
	for _, finding := range reviewState.Findings {
		findings = append(findings, dashboardFinding{
			ID: finding.ID, RuleID: finding.RuleID, Severity: finding.Severity,
			RecordID: finding.RecordID, CandidateSHA: finding.CandidateSHA, Hypothesis: true,
		})
	}
	resolvedIDs := make([]string, 0, len(reviewState.Resolved))
	for id := range reviewState.Resolved {
		resolvedIDs = append(resolvedIDs, id)
	}
	sort.Strings(resolvedIDs)
	resolved := make([]execution.ReviewResolution, 0, len(resolvedIDs))
	for _, id := range resolvedIDs {
		resolved = append(resolved, reviewState.Resolved[id])
	}
	snapshot := dashboardSnapshot{
		SchemaVersion: "1.0",
		GeneratedAt:   util.Now(),
		Run:           run,
		History:       transitions,
		Approvals:     trace.Approvals,
		Evidence:      records,
		Trace:         trace,
		Review: dashboardReview{
			Findings: findings, Resolved: resolved, Unresolved: reviewState.Unresolved,
			Digest: reviewState.Digest,
			Note:   "Review findings are untrusted hypotheses recorded in evidence. They are not verified findings or approval; human resolutions are recorded separately.",
		},
		Note: "Read-only local snapshot derived from persisted state and evidence. Missing trace stages are shown honestly; no live provider, Jira, or GitHub result is asserted.",
	}
	generic, err := util.JSONValue(snapshot)
	if err != nil {
		return dashboardSnapshot{}, err
	}
	encoded, err := json.Marshal(util.SanitizeValue(generic))
	if err != nil {
		return dashboardSnapshot{}, err
	}
	var safe dashboardSnapshot
	if err := json.Unmarshal(encoded, &safe); err != nil {
		return dashboardSnapshot{}, err
	}
	return safe, nil
}

func dashboardEvidenceData(record evidence.Record) (map[string]any, bool) {
	switch record.Kind {
	case "provider", "review", "remediation":
		return nil, true
	}
	generic, err := util.JSONValue(record.Data)
	if err != nil {
		return nil, true
	}
	raw, err := json.Marshal(generic)
	if err != nil || len(raw) > maxDashboardEvidenceDataBytes {
		return nil, true
	}
	safe, ok := util.SanitizeValue(generic).(map[string]any)
	if !ok {
		return nil, true
	}
	return safe, false
}
