package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/yoosuf/ae.sh/internal/adapters/jira"
	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/util"
)

const maxBugDescriptionFileBytes = 32 << 10

type jiraBugPreview struct {
	Mode             string `json:"mode"`
	Operation        string `json:"operation"`
	IssueKey         string `json:"issueKey,omitempty"`
	ProjectKey       string `json:"projectKey,omitempty"`
	IssueType        string `json:"issueType,omitempty"`
	Title            string `json:"title"`
	DescriptionBytes int    `json:"descriptionBytes"`
	RunID            string `json:"runId,omitempty"`
	DryRun           bool   `json:"dryRun"`
	Note             string `json:"note"`
}

type jiraBugOutput struct {
	Mode             string `json:"mode"`
	Operation        string `json:"operation"`
	IssueKey         string `json:"issueKey"`
	ProjectKey       string `json:"projectKey,omitempty"`
	IssueType        string `json:"issueType,omitempty"`
	ID               string `json:"id,omitempty"`
	URL              string `json:"url"`
	EvidenceRecordID string `json:"evidenceRecordID,omitempty"`
	Note             string `json:"note"`
}

func (a *App) cmdJira(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage("jira requires a subcommand: report-bug")
	}
	switch args[0] {
	case "report-bug":
		return a.cmdJiraReportBug(ctx, args[1:])
	default:
		return usage("unknown jira subcommand %q", args[0])
	}
}

func (a *App) cmdJiraReportBug(ctx context.Context, args []string) error {
	fs := newFlagSet("jira report-bug")
	title := fs.String("title", "", "bug title or summary")
	description := fs.String("description", "", "bug description")
	descriptionFile := fs.String("description-file", "", "read the bug description from a regular file")
	issue := fs.String("issue", "", "existing Jira key or browse URL; adds a comment")
	project := fs.String("project", "", "project key for a new Jira bug ticket")
	issueType := fs.String("issue-type", "Bug", "issue type for a new ticket")
	runID := fs.String("run", "", "record one evidence record on this run")
	actor := fs.String("actor", "local-user", "evidence actor")
	dryRun := fs.Bool("dry-run", false, "preview the report without writing to Jira")
	confirmMutate := fs.Bool("confirm-mutate", false, "authorize the Jira write")
	if err := fs.Parse(args); err != nil {
		return usage("jira report-bug: %v", err)
	}
	if fs.NArg() != 0 {
		return usage("jira report-bug takes no positional arguments")
	}
	if *title == "" {
		return usage("jira report-bug requires --title")
	}
	if (*description == "") == (*descriptionFile == "") {
		return usage("jira report-bug requires exactly one of --description or --description-file")
	}
	if (*issue == "") == (*project == "") {
		return usage("jira report-bug requires exactly one of --issue or --project")
	}
	if !*dryRun && !*confirmMutate {
		return &PolicyError{Message: "Jira bug reporting requires --confirm-mutate; use --dry-run to preview"}
	}

	body := *description
	if *descriptionFile != "" {
		var err error
		body, err = readBugDescriptionFile(*descriptionFile)
		if err != nil {
			return err
		}
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	if cfg.Integrations.Jira.Mode != "jira-cloud" {
		return &PolicyError{Message: "Jira bug reporting requires integrations.jira.mode: jira-cloud"}
	}
	report := jira.BugReport{
		IssueKey: *issue, ProjectKey: *project, IssueType: *issueType,
		Summary: *title, Description: body,
	}
	if report.IssueKey != "" {
		key, err := jira.ParseIssue(report.IssueKey, cfg.Integrations.Jira)
		if err != nil {
			return invalid("%v", err)
		}
		report.IssueKey = key
	}
	report, err = jira.NormalizeBugReport(report)
	if err != nil {
		return invalid("%v", err)
	}
	mode, operation := "new", "create"
	if report.IssueKey != "" {
		mode, operation = "existing", "comment"
	}
	if *dryRun {
		a.emit(jiraBugPreview{
			Mode: mode, Operation: operation, IssueKey: report.IssueKey,
			ProjectKey: report.ProjectKey, IssueType: report.IssueType,
			Title: report.Summary, DescriptionBytes: len(report.Description),
			RunID: *runID, DryRun: true,
			Note: "Preview only: no Jira call, token read, or evidence write occurred.",
		})
		return nil
	}

	store := (*sqliteadapter.Store)(nil)
	if *runID != "" {
		store, err = sqliteadapter.Open(cfg.StatePath())
		if err != nil {
			return err
		}
		defer store.Close()
		if _, err := store.GetRun(ctx, *runID); err != nil {
			return invalid("jira report run: %v", err)
		}
	}
	source, err := a.jiraSource(cfg.Integrations.Jira)
	if err != nil {
		return invalid("%v", err)
	}
	result, err := source.ReportBug(ctx, report)
	if err != nil {
		return invalid("%v", err)
	}
	output := jiraBugOutput{
		Mode: result.Mode, Operation: result.Operation, IssueKey: result.IssueKey,
		ProjectKey: result.ProjectKey, IssueType: result.IssueType, ID: result.ID,
		URL: result.URL, Note: result.Note,
	}
	if store != nil {
		record, err := appendRunEvidence(ctx, cfg, store, *runID, "artifact",
			"Jira bug report", *actor, map[string]any{
				"mode":              result.Mode,
				"operation":         result.Operation,
				"issueKey":          result.IssueKey,
				"projectKey":        result.ProjectKey,
				"issueType":         result.IssueType,
				"jiraId":            result.ID,
				"url":               result.URL,
				"titleDigest":       util.Digest([]byte(report.Summary)),
				"descriptionDigest": util.Digest([]byte(report.Description)),
				"confirmedMutate":   true,
			}, nil, nil)
		if err != nil {
			return fmt.Errorf("Jira bug reported as %s, but evidence write failed: %w", result.IssueKey, err)
		}
		output.EvidenceRecordID = record.RecordID
	}
	a.emit(output)
	return nil
}

func (a *App) jiraSource(cfg config.Integration) (*jira.Source, error) {
	if a.JiraSourceFactory != nil {
		return a.JiraSourceFactory(cfg)
	}
	return jira.New(cfg)
}

func readBugDescriptionFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", invalid("jira description file: %v", err)
	}
	if !info.Mode().IsRegular() {
		return "", invalid("jira description file must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", invalid("jira description file: %v", err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBugDescriptionFileBytes+1))
	if err != nil {
		return "", invalid("jira description file: %v", err)
	}
	if len(raw) > maxBugDescriptionFileBytes {
		return "", invalid("jira description file exceeds %d bytes", maxBugDescriptionFileBytes)
	}
	return strings.TrimSpace(string(raw)), nil
}
