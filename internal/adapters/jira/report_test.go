package jira

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type capturedRequest struct {
	method string
	path   string
	auth   string
	ctype  string
	body   []byte
}

func newReportSource(t *testing.T, status int, response string, capture *capturedRequest) *Source {
	t.Helper()
	t.Setenv(tokenEnv, "FAKE-report-token")
	source, err := New(testConfig("https://example.atlassian.net"))
	if err != nil {
		t.Fatal(err)
	}
	source.Client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			raw, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			*capture = capturedRequest{
				method: req.Method,
				path:   req.URL.Path,
				auth:   req.Header.Get("Authorization"),
				ctype:  req.Header.Get("Content-Type"),
				body:   raw,
			}
			return &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(response)),
			}, nil
		}),
	}
	return source
}

func TestReportBugAddsCommentToExistingTicket(t *testing.T) {
	var captured capturedRequest
	source := newReportSource(t, http.StatusCreated, `{"id":"10001"}`, &captured)
	result, err := source.ReportBug(context.Background(), BugReport{
		IssueKey: "TEST-1", Summary: "Crash on save",
		Description: "Open the editor\nPress save",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "existing" || result.Operation != "comment" || result.IssueKey != "TEST-1" || result.ID != "10001" {
		t.Fatalf("unexpected comment result: %+v", result)
	}
	if result.URL != "https://example.atlassian.net/browse/TEST-1" {
		t.Errorf("unexpected browse URL %q", result.URL)
	}
	if captured.method != http.MethodPost || captured.path != "/rest/api/3/issue/TEST-1/comment" {
		t.Fatalf("unexpected request %s %s", captured.method, captured.path)
	}
	if captured.auth != "Bearer FAKE-report-token" || captured.ctype != "application/json" {
		t.Errorf("unexpected headers: auth=%q content-type=%q", captured.auth, captured.ctype)
	}
	var body struct {
		Body struct {
			Version int    `json:"version"`
			Type    string `json:"type"`
			Content []struct {
				Type    string `json:"type"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"content"`
		} `json:"body"`
	}
	if err := json.Unmarshal(captured.body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Body.Version != 1 || body.Body.Type != "doc" || len(body.Body.Content) < 2 {
		t.Fatalf("unexpected ADF body: %+v", body.Body)
	}
	if !strings.Contains(body.Body.Content[0].Content[0].Text, "AE bug report: Crash on save") {
		t.Errorf("ADF title paragraph is missing: %+v", body.Body.Content[0])
	}
	joined := ""
	for _, paragraph := range body.Body.Content {
		for _, node := range paragraph.Content {
			joined += node.Text + "\n"
		}
	}
	if !strings.Contains(joined, "Open the editor") || !strings.Contains(joined, "Press save") {
		t.Errorf("ADF description is missing: %q", joined)
	}
}

func TestReportBugCreatesNewTicket(t *testing.T) {
	var captured capturedRequest
	source := newReportSource(t, http.StatusCreated, `{"id":"10002","key":"TEST-42"}`, &captured)
	result, err := source.ReportBug(context.Background(), BugReport{
		ProjectKey: "TEST", IssueType: "Bug", Summary: "Crash on save",
		Description: "Steps to reproduce",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "new" || result.Operation != "create" || result.IssueKey != "TEST-42" ||
		result.ProjectKey != "TEST" || result.IssueType != "Bug" || result.ID != "10002" {
		t.Fatalf("unexpected create result: %+v", result)
	}
	if captured.method != http.MethodPost || captured.path != "/rest/api/3/issue" {
		t.Fatalf("unexpected request %s %s", captured.method, captured.path)
	}
	var body struct {
		Fields struct {
			Project struct {
				Key string `json:"key"`
			} `json:"project"`
			IssueType struct {
				Name string `json:"name"`
			} `json:"issuetype"`
			Summary     string `json:"summary"`
			Description struct {
				Type string `json:"type"`
			} `json:"description"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(captured.body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Fields.Project.Key != "TEST" || body.Fields.IssueType.Name != "Bug" ||
		body.Fields.Summary != "Crash on save" || body.Fields.Description.Type != "doc" {
		t.Fatalf("unexpected create payload: %+v", body.Fields)
	}
}

func TestNormalizeBugReportIsFailClosed(t *testing.T) {
	// Built at runtime so a literal credential shape never enters the tree.
	credential := "sk-" + strings.Repeat("q", 24)
	tests := []struct {
		name   string
		report BugReport
		want   string
	}{
		{
			name:   "both targets",
			report: BugReport{IssueKey: "TEST-1", ProjectKey: "TEST", Summary: "s", Description: "d"},
			want:   "exactly one of issue key or project key",
		},
		{
			name:   "no target",
			report: BugReport{Summary: "s", Description: "d"},
			want:   "exactly one of issue key or project key",
		},
		{
			name:   "invalid issue key",
			report: BugReport{IssueKey: "BAD KEY", Summary: "s", Description: "d"},
			want:   "invalid Jira issue key",
		},
		{
			name:   "invalid project key",
			report: BugReport{ProjectKey: "bad-project", Summary: "s", Description: "d"},
			want:   "invalid Jira project key",
		},
		{
			name:   "invalid issue type",
			report: BugReport{ProjectKey: "TEST", IssueType: "Bug; drop", Summary: "s", Description: "d"},
			want:   "invalid Jira issue type",
		},
		{
			name:   "credential in title",
			report: BugReport{IssueKey: "TEST-1", Summary: "token " + credential, Description: "d"},
			want:   "credential-shaped value",
		},
		{
			name:   "empty description",
			report: BugReport{IssueKey: "TEST-1", Summary: "s", Description: "  "},
			want:   "description is required",
		},
		{
			name:   "oversized title",
			report: BugReport{IssueKey: "TEST-1", Summary: strings.Repeat("x", maxBugSummaryRunes+1), Description: "d"},
			want:   "exceeds 200 characters",
		},
		{
			name:   "oversized description",
			report: BugReport{IssueKey: "TEST-1", Summary: "s", Description: strings.Repeat("x", maxBugDescription+1)},
			want:   "exceeds 32768 bytes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NormalizeBugReport(tt.report); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NormalizeBugReport error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestNormalizeBugReportDefaultsIssueTypeAndNormalizesKeys(t *testing.T) {
	normalized, err := NormalizeBugReport(BugReport{
		ProjectKey: "test", Summary: " crash ", Description: " details ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ProjectKey != "TEST" || normalized.IssueType != "Bug" ||
		normalized.Summary != "crash" || normalized.Description != "details" {
		t.Fatalf("unexpected normalization: %+v", normalized)
	}
	existing, err := NormalizeBugReport(BugReport{
		IssueKey: "test-1", Summary: "s", Description: "d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if existing.IssueKey != "TEST-1" || existing.ProjectKey != "" || existing.IssueType != "" {
		t.Fatalf("existing ticket target was not normalized: %+v", existing)
	}
}

func TestReportBugRejectsPlaintextBaseURL(t *testing.T) {
	t.Setenv(tokenEnv, "FAKE-report-token")
	source := &Source{BaseURL: "http://example.atlassian.net", TokenEnv: tokenEnv}
	called := false
	source.Client = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, io.ErrUnexpectedEOF
	})}
	_, err := source.ReportBug(context.Background(), BugReport{
		IssueKey: "TEST-1", Summary: "s", Description: "d",
	})
	if err == nil || !strings.Contains(err.Error(), "must use https") {
		t.Fatalf("plaintext Jira URL was accepted: %v", err)
	}
	if called {
		t.Error("Jira request was sent over plaintext")
	}
}

func TestReportBugRequiresEnvironmentToken(t *testing.T) {
	t.Setenv(tokenEnv, "")
	source, err := New(testConfig("https://example.atlassian.net"))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	source.Client = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, io.ErrUnexpectedEOF
	})}
	_, err = source.ReportBug(context.Background(), BugReport{
		IssueKey: "TEST-1", Summary: "s", Description: "d",
	})
	if err == nil || !strings.Contains(err.Error(), "not set") {
		t.Fatalf("missing token was accepted: %v", err)
	}
	if called {
		t.Error("Jira request was sent without a token")
	}
}

func TestReportBugValidatesCreateResponse(t *testing.T) {
	for _, tt := range []struct {
		name     string
		response string
		want     string
	}{
		{"wrong project", `{"id":"1","key":"OTHER-9"}`, "does not match project"},
		{"missing id", `{"key":"TEST-9"}`, "did not include an id"},
		{"malformed key", `{"id":"1","key":"bad"}`, "does not match project"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var captured capturedRequest
			source := newReportSource(t, http.StatusCreated, tt.response, &captured)
			_, err := source.ReportBug(context.Background(), BugReport{
				ProjectKey: "TEST", IssueType: "Bug", Summary: "s", Description: "d",
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ReportBug error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
