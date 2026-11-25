package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sqliteadapter "github.com/aef-framework/aef/internal/adapters/sqlite"
	"github.com/aef-framework/aef/internal/evidence"
	"github.com/aef-framework/aef/internal/execution"
)

type traceEvidence struct {
	RecordID   string         `json:"recordId"`
	Sequence   int            `json:"sequence"`
	Kind       string         `json:"kind"`
	Subject    string         `json:"subject"`
	Actor      string         `json:"actor,omitempty"`
	DataDigest string         `json:"dataDigest"`
	ExitStatus *int           `json:"exitStatus,omitempty"`
	Summary    map[string]any `json:"summary,omitempty"`
}

type traceStage struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Status      string          `json:"status"`
	Evidence    []traceEvidence `json:"evidence"`
	Transitions []string        `json:"transitions,omitempty"`
}

type traceReport struct {
	RunID          string                     `json:"runId"`
	WorkflowID     string                     `json:"workflowId"`
	State          string                     `json:"state"`
	Status         string                     `json:"status"`
	Risk           string                     `json:"risk"`
	TerminalReason string                     `json:"terminalReason,omitempty"`
	Stages         []traceStage               `json:"stages"`
	Approvals      []executionApprovalSummary `json:"approvals"`
	EvidenceChain  []string                   `json:"evidenceChain"`
	Note           string                     `json:"note"`
}

type executionApprovalSummary struct {
	Gate           string `json:"gate"`
	Approver       string `json:"approver"`
	Role           string `json:"role,omitempty"`
	IdentitySource string `json:"identitySource"`
	ArtifactDigest string `json:"artifactDigest"`
	CandidateSHA   string `json:"candidateSHA"`
}

var traceStageKeywords = []struct {
	id, title string
	keywords  []string
}{
	{"requirements", "Requirements", []string{"normalized plan", "task intake", "task acceptance", "requirement"}},
	{"architecture", "Architecture and risk", []string{"task classify-risk", "task plan", "solution-architecture", "risk assessment"}},
	{"implementation", "Implementation", []string{"task isolate-worktree", "task implement", "worktree prepare", "backend-development"}},
	{"testing", "Verification", []string{"task unit-tests", "task verify-final", "verify format", "verify lint", "verify build", "verify unit-tests", "verify dependency-audit", "verify secret-scan"}},
	{"review", "Independent review and remediation", []string{"task review", "task remediate", "verified: false", "local-pattern-scan"}},
	{"release", "Release", []string{"task pull-request", "task merge-authorization", "task staging-checks", "task release-approval", "task deploy", "task production-checks", "gate merge", "gate release"}},
	{"feedback", "Operational feedback", []string{"task jira-closure", "operational-feedback", "requirements feedback"}},
}

func (a *App) cmdTrace(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage("trace requires a run id")
	}
	fs := newFlagSet("trace")
	format := fs.String("format", "json", "json or markdown")
	out := fs.String("out", "", "write the report to FILE")
	if err := fs.Parse(args[1:]); err != nil {
		return usage("trace: %v", err)
	}
	if fs.NArg() != 0 {
		return usage("trace accepts exactly one run id")
	}
	if *format != "json" && *format != "markdown" {
		return usage("trace --format must be json or markdown")
	}
	runID := args[0]
	repo, err := a.loadRepo()
	if err != nil {
		return err
	}
	store, err := sqliteadapter.Open(repo.Config.StatePath())
	if err != nil {
		return err
	}
	defer store.Close()
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return a.notFound(err)
	}
	report, err := buildTraceReport(ctx, store, run)
	if err != nil {
		return err
	}
	if *out != "" {
		path, err := filepath.Abs(*out)
		if err != nil {
			return err
		}
		content := traceMarkdown(report)
		if *format == "json" {
			raw, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			content = string(raw)
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0o600); err != nil {
			return err
		}
		a.emit(map[string]any{"runID": run.ID, "file": filepath.ToSlash(path), "stages": len(report.Stages)})
		return nil
	}
	if *format == "markdown" {
		a.emit(traceMarkdown(report))
		return nil
	}
	a.emit(report)
	return nil
}

func buildTraceReport(ctx context.Context, store *sqliteadapter.Store, run executionRun) (traceReport, error) {
	report := traceReport{
		RunID: run.ID, WorkflowID: run.WorkflowID, State: run.State, Status: run.Status,
		Risk: run.Risk, TerminalReason: run.TerminalReason,
		Stages: make([]traceStage, 0, len(traceStageKeywords)), Approvals: []executionApprovalSummary{},
		EvidenceChain: []string{},
		Note:          "Traceability is reconstructed only from persisted state and evidence. Missing stages are reported as missing; a local pattern scan is not independent review, and no live provider, Jira, or GitHub result is asserted.",
	}
	stored, err := store.Evidence(ctx, run.ID)
	if err != nil {
		return report, err
	}
	records := make([]evidence.Record, 0, len(stored))
	for _, item := range stored {
		var record evidence.Record
		if err := json.Unmarshal([]byte(item.JSON), &record); err != nil {
			return report, invalid("stored evidence is malformed: %v", err)
		}
		records = append(records, record)
		report.EvidenceChain = append(report.EvidenceChain, record.RecordID)
	}
	evidence.SortRecords(records)
	transitions, err := store.Transitions(ctx, run.ID)
	if err != nil {
		return report, err
	}
	byStage := map[string][]traceEvidence{}
	gateSet := map[string]bool{}
	for _, record := range records {
		item := traceEvidence{
			RecordID: record.RecordID, Sequence: record.Sequence, Kind: record.Kind,
			Subject: record.Subject, Actor: record.Actor, DataDigest: record.DataDigest,
			ExitStatus: record.ExitStatus, Summary: traceSummary(record),
		}
		if gate, ok := record.Data["gate"].(string); ok && gate != "" {
			gateSet[gate] = true
		}
		for _, stage := range traceStageKeywords {
			if matchesTraceStage(record, stage.keywords) {
				byStage[stage.id] = append(byStage[stage.id], item)
			}
		}
	}
	for gate := range gateSet {
		approvals, err := store.Approvals(ctx, run.ID, gate)
		if err != nil {
			return report, err
		}
		for _, approval := range approvals {
			report.Approvals = append(report.Approvals, executionApprovalSummary{
				Gate: approval.Gate, Approver: approval.Approver, Role: approval.Role,
				IdentitySource: approval.IdentitySource, ArtifactDigest: approval.ArtifactDigest,
				CandidateSHA: approval.CandidateSHA,
			})
		}
	}
	sort.Slice(report.Approvals, func(i, j int) bool {
		if report.Approvals[i].Gate != report.Approvals[j].Gate {
			return report.Approvals[i].Gate < report.Approvals[j].Gate
		}
		return report.Approvals[i].Approver < report.Approvals[j].Approver
	})
	for _, stage := range traceStageKeywords {
		result := traceStage{ID: stage.id, Title: stage.title, Status: "missing", Evidence: []traceEvidence{}}
		items := byStage[stage.id]
		if len(items) > 0 {
			result.Status = "observed"
			result.Evidence = items
			for _, item := range items {
				if passed, ok := item.Summary["passed"].(bool); ok && !passed {
					result.Status = "failed"
				}
				if item.ExitStatus != nil && *item.ExitStatus != 0 && result.Status != "failed" {
					result.Status = "failed"
				}
				if passed, ok := item.Summary["approved"].(bool); ok && passed && result.Status == "observed" {
					result.Status = "approved"
				}
			}
		}
		for _, transition := range transitions {
			if strings.Contains(strings.ToLower(transition.TaskID), stage.id) {
				result.Transitions = append(result.Transitions, transition.From+" -> "+transition.To+" ("+transition.TaskID+")")
			}
		}
		report.Stages = append(report.Stages, result)
	}
	return report, nil
}

// executionRun is aliased to keep the trace builder's signature explicit.
type executionRun = execution.Run

func matchesTraceStage(record evidence.Record, keywords []string) bool {
	haystack := strings.ToLower(record.Subject + " " + record.Kind)
	for key, value := range record.Data {
		if key == "output" || key == "issue" {
			continue
		}
		haystack += " " + strings.ToLower(fmt.Sprint(key)) + " " + strings.ToLower(fmt.Sprint(value))
	}
	for _, keyword := range keywords {
		if strings.Contains(haystack, keyword) {
			return true
		}
	}
	return false
}

func traceSummary(record evidence.Record) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"gate", "passed", "simulated", "command", "profile", "provider", "skill", "agent", "workspace", "baseSHA", "candidateSHA"} {
		if value, ok := record.Data[key]; ok {
			out[key] = value
		}
	}
	if record.Kind == "approval" {
		out["approved"] = true
	}
	if findings, ok := record.Data["findings"]; ok {
		out["findings"] = findings
	}
	return out
}

func traceMarkdown(report traceReport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Traceability report: %s\n\n", report.RunID)
	fmt.Fprintf(&sb, "- Workflow: `%s`\n- State: `%s` (%s)\n- Risk: `%s`\n", report.WorkflowID, report.State, report.Status, report.Risk)
	if report.TerminalReason != "" {
		fmt.Fprintf(&sb, "- Terminal reason: %s\n", report.TerminalReason)
	}
	sb.WriteString("\n## Lifecycle coverage\n\n")
	sb.WriteString("| Stage | Status | Evidence records |\n| --- | --- | --- |\n")
	for _, stage := range report.Stages {
		fmt.Fprintf(&sb, "| %s | %s | %d |\n", stage.Title, stage.Status, len(stage.Evidence))
	}
	sb.WriteString("\n## Evidence chain\n\n")
	for _, stage := range report.Stages {
		fmt.Fprintf(&sb, "### %s\n\n", stage.Title)
		if len(stage.Evidence) == 0 {
			sb.WriteString("_No evidence recorded for this stage._\n\n")
			continue
		}
		for _, item := range stage.Evidence {
			fmt.Fprintf(&sb, "- #%d `%s` %s — %s (digest `%s`)\n", item.Sequence, item.Kind, item.Subject, item.RecordID, item.DataDigest)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## Approvals\n\n")
	if len(report.Approvals) == 0 {
		sb.WriteString("_No approvals recorded._\n\n")
	} else {
		for _, approval := range report.Approvals {
			fmt.Fprintf(&sb, "- `%s`: %s (%s, %s) — artifact `%s`, candidate `%s`\n", approval.Gate, approval.Approver, approval.Role, approval.IdentitySource, approval.ArtifactDigest, approval.CandidateSHA)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## Limits\n\n")
	sb.WriteString(report.Note + "\n")
	return sb.String()
}
