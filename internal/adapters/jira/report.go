package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/util"
)

const (
	maxBugSummaryRunes   = 200
	maxBugDescription    = 32 << 10
	maxJiraReportResp    = 4 << 20
	maxADFParagraphRunes = 4000
)

var (
	projectKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,19}$`)
	issueTypePattern  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _-]{0,49}$`)
)

// BugReport describes one bounded, sanitized Jira bug report. Exactly one of
// IssueKey or ProjectKey must be set: IssueKey adds a comment to an existing
// ticket, while ProjectKey creates a new ticket.
type BugReport struct {
	IssueKey    string
	ProjectKey  string
	IssueType   string
	Summary     string
	Description string
}

// BugReportResult is the provider-neutral result of an explicit Jira write.
// It contains no credential material and no unverified claims.
type BugReportResult struct {
	Mode       string `json:"mode"`
	Operation  string `json:"operation"`
	IssueKey   string `json:"issueKey"`
	ProjectKey string `json:"projectKey,omitempty"`
	IssueType  string `json:"issueType,omitempty"`
	ID         string `json:"id,omitempty"`
	URL        string `json:"url"`
	Note       string `json:"note"`
}

// NormalizeBugReport validates and sanitizes a report without any network or
// environment access. It is safe for dry-run previews.
func NormalizeBugReport(report BugReport) (BugReport, error) {
	summary, err := cleanBugText("title", report.Summary, maxBugSummaryRunes, true)
	if err != nil {
		return BugReport{}, err
	}
	description, err := cleanBugText("description", report.Description, maxBugDescription, false)
	if err != nil {
		return BugReport{}, err
	}
	if (report.IssueKey == "") == (report.ProjectKey == "") {
		return BugReport{}, fmt.Errorf("bug report requires exactly one of issue key or project key")
	}
	report.Summary = summary
	report.Description = description
	if report.IssueKey != "" {
		key := strings.ToUpper(strings.TrimSpace(report.IssueKey))
		if !keyPattern.MatchString(key) {
			return BugReport{}, fmt.Errorf("invalid Jira issue key %q", util.Sanitize(report.IssueKey))
		}
		report.IssueKey = key
		report.ProjectKey = ""
		report.IssueType = ""
		return report, nil
	}
	project := strings.ToUpper(strings.TrimSpace(report.ProjectKey))
	if !projectKeyPattern.MatchString(project) {
		return BugReport{}, fmt.Errorf("invalid Jira project key %q", util.Sanitize(report.ProjectKey))
	}
	issueType := strings.TrimSpace(report.IssueType)
	if issueType == "" {
		issueType = "Bug"
	}
	if !issueTypePattern.MatchString(issueType) {
		return BugReport{}, fmt.Errorf("invalid Jira issue type %q", util.Sanitize(issueType))
	}
	report.ProjectKey = project
	report.IssueType = issueType
	return report, nil
}

// ReportBug writes one bug report through Jira Cloud REST v3. This is a live
// mutation and must only be called after explicit CLI authorization.
func (s *Source) ReportBug(ctx context.Context, report BugReport) (BugReportResult, error) {
	if s == nil || s.BaseURL == "" || !config.IsEnvName(s.TokenEnv) {
		return BugReportResult{}, fmt.Errorf("Jira source is not configured")
	}
	if !strings.HasPrefix(strings.ToLower(s.BaseURL), "https://") {
		return BugReportResult{}, fmt.Errorf("Jira baseURL must use https before sending a token")
	}
	if os.Getenv(s.TokenEnv) == "" {
		return BugReportResult{}, fmt.Errorf("Jira token environment variable %s is not set", s.TokenEnv)
	}
	report, err := NormalizeBugReport(report)
	if err != nil {
		return BugReportResult{}, err
	}
	if report.IssueKey != "" {
		return s.commentBug(ctx, report.IssueKey, report.Summary, report.Description)
	}
	return s.createBug(ctx, report.ProjectKey, report.IssueType, report.Summary, report.Description)
}

func (s *Source) commentBug(ctx context.Context, value, summary, description string) (BugReportResult, error) {
	key := strings.ToUpper(strings.TrimSpace(value))
	if !keyPattern.MatchString(key) {
		return BugReportResult{}, fmt.Errorf("invalid Jira issue key %q", util.Sanitize(value))
	}
	body := map[string]any{"body": adfDocument("AE bug report: "+summary, description)}
	var payload struct {
		ID string `json:"id"`
	}
	if err := s.doJSON(ctx, http.MethodPost, "/rest/api/3/issue/"+url.PathEscape(key)+"/comment", body, &payload, http.StatusCreated); err != nil {
		return BugReportResult{}, err
	}
	if strings.TrimSpace(payload.ID) == "" {
		return BugReportResult{}, fmt.Errorf("Jira comment response did not include an id")
	}
	return BugReportResult{
		Mode: "existing", Operation: "comment", IssueKey: key, ID: util.Sanitize(payload.ID),
		URL:  s.BaseURL + "/browse/" + key,
		Note: "Bug report was added as a comment. Jira content is untrusted data and no AE gate was satisfied.",
	}, nil
}

func (s *Source) createBug(ctx context.Context, value, issueType, summary, description string) (BugReportResult, error) {
	project := strings.ToUpper(strings.TrimSpace(value))
	if !projectKeyPattern.MatchString(project) {
		return BugReportResult{}, fmt.Errorf("invalid Jira project key %q", util.Sanitize(value))
	}
	if strings.TrimSpace(issueType) == "" {
		issueType = "Bug"
	}
	issueType = strings.TrimSpace(issueType)
	if !issueTypePattern.MatchString(issueType) {
		return BugReportResult{}, fmt.Errorf("invalid Jira issue type %q", util.Sanitize(issueType))
	}
	body := map[string]any{"fields": map[string]any{
		"project":     map[string]any{"key": project},
		"issuetype":   map[string]any{"name": issueType},
		"summary":     summary,
		"description": adfDocument(summary, description),
	}}
	var payload struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	if err := s.doJSON(ctx, http.MethodPost, "/rest/api/3/issue", body, &payload, http.StatusCreated); err != nil {
		return BugReportResult{}, err
	}
	key := strings.ToUpper(strings.TrimSpace(payload.Key))
	if !keyPattern.MatchString(key) || !strings.HasPrefix(key, project+"-") {
		return BugReportResult{}, fmt.Errorf("Jira response key %q does not match project %q", util.Sanitize(key), project)
	}
	if strings.TrimSpace(payload.ID) == "" {
		return BugReportResult{}, fmt.Errorf("Jira issue response did not include an id")
	}
	return BugReportResult{
		Mode: "new", Operation: "create", IssueKey: key, ProjectKey: project,
		IssueType: issueType, ID: util.Sanitize(payload.ID),
		URL:  s.BaseURL + "/browse/" + key,
		Note: "New Jira ticket created. Jira content is untrusted data and no AE gate was satisfied.",
	}, nil
}

func cleanBugText(label, value string, max int, runeLimit bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("Jira bug %s is required", label)
	}
	if util.SecretShapeCount(value) > 0 {
		return "", fmt.Errorf("Jira bug %s contains a credential-shaped value; remove it before reporting", label)
	}
	if runeLimit {
		if utf8.RuneCountInString(value) > max {
			return "", fmt.Errorf("Jira bug %s exceeds %d characters", label, max)
		}
	} else if len(value) > max {
		return "", fmt.Errorf("Jira bug %s exceeds %d bytes", label, max)
	}
	return util.Sanitize(value), nil
}

func adfDocument(summary, description string) map[string]any {
	lines := []string{"AE bug report: " + summary}
	lines = append(lines, strings.Split(strings.ReplaceAll(description, "\r\n", "\n"), "\n")...)
	content := make([]any, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(util.Sanitize(raw))
		runes := []rune(line)
		for len(runes) > maxADFParagraphRunes {
			content = append(content, adfParagraph(string(runes[:maxADFParagraphRunes])))
			runes = runes[maxADFParagraphRunes:]
		}
		if len(runes) > 0 {
			content = append(content, adfParagraph(string(runes)))
		}
	}
	if len(content) == 0 {
		content = append(content, adfParagraph("No description provided."))
	}
	return map[string]any{"version": 1, "type": "doc", "content": content}
}

func adfParagraph(text string) map[string]any {
	return map[string]any{
		"type":    "paragraph",
		"content": []any{map[string]any{"type": "text", "text": text}},
	}
}

func (s *Source) doJSON(ctx context.Context, method, path string, body, out any, wantStatus int) error {
	token := os.Getenv(s.TokenEnv)
	if token == "" {
		return fmt.Errorf("Jira token environment variable %s is not set", s.TokenEnv)
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode Jira request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.BaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build Jira request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: s.Timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Jira request failed: %s", util.Sanitize(err.Error()))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxJiraReportResp+1))
	if err != nil {
		return fmt.Errorf("read Jira response: %w", err)
	}
	if len(raw) > maxJiraReportResp {
		return fmt.Errorf("Jira response exceeds %d bytes", maxJiraReportResp)
	}
	if resp.StatusCode != wantStatus {
		return fmt.Errorf("Jira returned status %d: %s", resp.StatusCode, util.Sanitize(string(raw)))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("parse Jira response: %w", err)
		}
	}
	return nil
}
