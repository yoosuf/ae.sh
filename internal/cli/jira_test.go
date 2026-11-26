package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yoosuf/ae.sh/internal/adapters/jira"
	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
)

const jiraTestTokenEnv = "AE_TEST_JIRA_TOKEN"

type jiraTransportFunc func(*http.Request) (*http.Response, error)

func (f jiraTransportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func initJiraRepo(t *testing.T, root string) config.Config {
	t.Helper()
	cfg := initChatRepo(t, root)
	cfg.Integrations.Jira = config.Integration{
		Mode: "jira-cloud", BaseURL: "https://example.atlassian.net", TokenEnv: jiraTestTokenEnv,
	}
	if err := config.Write(filepath.Join(root, "ae.yaml"), cfg, true); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func createJiraRun(t *testing.T, cfg config.Config, runID string) {
	t.Helper()
	ctx := context.Background()
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.CreateRun(ctx, execution.Run{
		ID: runID, WorkflowID: "local-flow", WorkflowVersion: "1.0.0",
		IssueKey: "TEST-1", State: "queued", Status: "running", Risk: "low",
		Provider: "fake", Actor: "tester", CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z", Version: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

func jiraSourceFactory(t *testing.T, status int, response string, capture *jiraRequest) func(config.Integration) (*jira.Source, error) {
	t.Helper()
	return func(cfg config.Integration) (*jira.Source, error) {
		source, err := jira.New(cfg)
		if err != nil {
			return nil, err
		}
		source.Client = &http.Client{Transport: jiraTransportFunc(func(req *http.Request) (*http.Response, error) {
			raw, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if capture != nil {
				*capture = jiraRequest{method: req.Method, path: req.URL.Path, body: raw}
			}
			return &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(response)),
			}, nil
		})}
		return source, nil
	}
}

type jiraRequest struct {
	method string
	path   string
	body   []byte
}

func TestJiraReportBugDryRunDoesNotBuildSource(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	initJiraRepo(t, root)
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"jira", "report-bug", "--title", "Crash on save", "--description", "Steps",
			"--issue", "TEST-1", "--dry-run"},
		JiraSourceFactory: func(config.Integration) (*jira.Source, error) {
			t.Fatal("dry run built a Jira source")
			return nil, nil
		},
	}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("jira report-bug exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	var preview jiraBugPreview
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.DryRun || preview.Mode != "existing" || preview.Operation != "comment" ||
		preview.IssueKey != "TEST-1" || preview.DescriptionBytes != len("Steps") {
		t.Fatalf("unexpected dry-run preview: %+v", preview)
	}
	if !strings.Contains(preview.Note, "no Jira call") {
		t.Errorf("preview note did not disclaim the live call: %q", preview.Note)
	}
}

func TestJiraReportBugDryRunPreviewsNewTicket(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	initJiraRepo(t, root)
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"jira", "report-bug", "--title", "Crash on save", "--description", "Steps",
			"--project", "test", "--dry-run"},
	}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("jira report-bug exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	var preview jiraBugPreview
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Mode != "new" || preview.Operation != "create" ||
		preview.ProjectKey != "TEST" || preview.IssueType != "Bug" {
		t.Fatalf("unexpected new-ticket preview: %+v", preview)
	}
}

func TestJiraReportBugRequiresExplicitMutationConfirmation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	initJiraRepo(t, root)
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"jira", "report-bug", "--title", "Crash", "--description", "Steps", "--issue", "TEST-1"},
		JiraSourceFactory: func(config.Integration) (*jira.Source, error) {
			t.Fatal("unconfirmed command built a Jira source")
			return nil, nil
		},
	}
	if code := app.Run(ctx); code != ExitPolicy {
		t.Fatalf("expected policy denial, got %d: %s %s", code, stdout.String(), stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result["error"].(string), "--confirm-mutate") {
		t.Errorf("unexpected policy error: %v", result["error"])
	}
}

func TestJiraReportBugRequiresJiraCloudMode(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	initChatRepo(t, root)
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"jira", "report-bug", "--title", "Crash", "--description", "Steps",
			"--issue", "TEST-1", "--dry-run"},
	}
	if code := app.Run(ctx); code != ExitPolicy {
		t.Fatalf("expected policy denial for fixture mode, got %d: %s %s", code, stdout.String(), stderr.String())
	}
}

func TestJiraReportBugRejectsCredentialShapedDescription(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	initJiraRepo(t, root)
	credential := "sk-" + strings.Repeat("q", 24)
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"jira", "report-bug", "--title", "Crash",
			"--description", "token " + credential, "--issue", "TEST-1", "--dry-run"},
	}
	if code := app.Run(ctx); code != ExitValidation {
		t.Fatalf("expected validation error, got %d: %s %s", code, stdout.String(), stderr.String())
	}
}

func TestJiraReportBugCommentWritesChainedEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := initJiraRepo(t, root)
	const runID = "run-jira-comment"
	createJiraRun(t, cfg, runID)
	t.Setenv(jiraTestTokenEnv, "FAKE-report-token")
	descriptionPath := filepath.Join(root, "bug.txt")
	if err := os.WriteFile(descriptionPath, []byte("Open editor\nPress save\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var captured jiraRequest
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"jira", "report-bug", "--title", "Crash on save", "--description-file", descriptionPath,
			"--issue", "TEST-1", "--run", runID, "--actor", "tester", "--confirm-mutate"},
		JiraSourceFactory: jiraSourceFactory(t, http.StatusCreated, `{"id":"10001"}`, &captured),
	}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("jira report-bug exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	var output jiraBugOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.Mode != "existing" || output.Operation != "comment" || output.IssueKey != "TEST-1" ||
		output.ID != "10001" || output.EvidenceRecordID == "" {
		t.Fatalf("unexpected result: %+v", output)
	}
	if captured.method != http.MethodPost || captured.path != "/rest/api/3/issue/TEST-1/comment" {
		t.Fatalf("unexpected request %s %s", captured.method, captured.path)
	}
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records, err := store.Evidence(ctx, runID)
	if err != nil || len(records) != 1 {
		t.Fatalf("expected one evidence record, got %d: %v", len(records), err)
	}
	var record evidence.Record
	if err := json.Unmarshal([]byte(records[0].JSON), &record); err != nil {
		t.Fatal(err)
	}
	if record.Kind != "artifact" || record.Actor != "tester" || record.RecordID != output.EvidenceRecordID {
		t.Fatalf("unexpected evidence metadata: %+v", record)
	}
	if record.Data["issueKey"] != "TEST-1" || record.Data["confirmedMutate"] != true {
		t.Fatalf("unexpected evidence data: %+v", record.Data)
	}
	raw, err := json.Marshal(record.Data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Open editor") || strings.Contains(string(raw), "Crash on save") {
		t.Error("evidence stored raw bug content instead of digests")
	}
	if record.Data["titleDigest"] == "" || record.Data["descriptionDigest"] == "" {
		t.Error("evidence is missing content digests")
	}
}

func TestJiraReportBugCreatesNewTicketViaCLI(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := initJiraRepo(t, root)
	const runID = "run-jira-create"
	createJiraRun(t, cfg, runID)
	t.Setenv(jiraTestTokenEnv, "FAKE-report-token")
	var captured jiraRequest
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"jira", "report-bug", "--title", "Crash on save", "--description", "Steps",
			"--project", "test", "--issue-type", "Bug", "--run", runID, "--confirm-mutate"},
		JiraSourceFactory: jiraSourceFactory(t, http.StatusCreated, `{"id":"10002","key":"TEST-42"}`, &captured),
	}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("jira report-bug exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	var output jiraBugOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.Mode != "new" || output.Operation != "create" || output.ProjectKey != "TEST" ||
		output.IssueKey != "TEST-42" || output.EvidenceRecordID == "" {
		t.Fatalf("unexpected result: %+v", output)
	}
	if captured.path != "/rest/api/3/issue" || !strings.Contains(string(captured.body), `"key":"TEST"`) {
		t.Fatalf("unexpected create request: %s %s", captured.path, captured.body)
	}
}

func TestJiraReportBugRejectsSymlinkedDescriptionFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test requires POSIX permissions")
	}
	ctx := context.Background()
	root := t.TempDir()
	initJiraRepo(t, root)
	target := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(target, []byte("secret context"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "bug.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"jira", "report-bug", "--title", "Crash", "--description-file", link,
			"--issue", "TEST-1", "--dry-run"},
	}
	if code := app.Run(ctx); code != ExitValidation {
		t.Fatalf("expected validation error for symlink, got %d: %s %s", code, stdout.String(), stderr.String())
	}
}
