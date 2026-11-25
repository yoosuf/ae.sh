package jira

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aef-framework/aef/internal/config"
)

const tokenEnv = "AEF_TEST_JIRA_TOKEN"

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testConfig(baseURL string) config.Integration {
	return config.Integration{Mode: "jira-cloud", BaseURL: baseURL, TokenEnv: tokenEnv}
}

func TestParseIssueAcceptsConfiguredLinkAndBareKey(t *testing.T) {
	cfg := testConfig("https://example.atlassian.net")
	for _, value := range []string{
		"TEST-1",
		"https://example.atlassian.net/browse/TEST-1",
		"https://example.atlassian.net/jira/software/projects/TEST/boards/1?selectedIssue=TEST-1",
	} {
		key, err := ParseIssue(value, cfg)
		if err != nil || key != "TEST-1" {
			t.Errorf("ParseIssue(%q) = %q, %v", value, key, err)
		}
	}
	for _, value := range []string{
		"https://evil.example/browse/TEST-1",
		"http://example.atlassian.net/browse/TEST-1",
		"https://user:pass@example.atlassian.net/browse/TEST-1",
		"https://example.atlassian.net/browse/TEST-1?token=secret",
		"not-a-key",
	} {
		if _, err := ParseIssue(value, cfg); err == nil {
			t.Errorf("ParseIssue accepted unsupported value %q", value)
		}
	}
}

func TestLoadNormalizesJiraIssueAndUsesEnvironmentToken(t *testing.T) {
	t.Setenv(tokenEnv, "FAKE-test-token")
	var gotAuth string
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/rest/api/3/issue/TEST-1" {
			t.Errorf("unexpected Jira path %q", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
			"key":"TEST-1",
			"fields":{
				"summary":"Add Jira link support",
				"description":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Untrusted issue text"}]}]},
				"labels":[
					"aef-acceptance:acceptance one",
					"aef-path:internal/adapters/jira/source.go",
					"aef-semantics:integration",
					"aef-dependency:schema",
					"aef-base-sha:abc1234"
				]
			}
		}`)),
		}, nil
	})}
	cfg := testConfig("https://example.atlassian.net")
	source, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	source.Client = client
	issue, err := source.Load(context.Background(), "TEST-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer FAKE-test-token" {
		t.Fatalf("unexpected authorization header %q", gotAuth)
	}
	if issue.Key != "TEST-1" || issue.Source != "jira-cloud" || issue.Summary != "Add Jira link support" {
		t.Fatalf("unexpected normalized issue: %+v", issue)
	}
	if len(issue.AcceptanceCriteria) != 1 || len(issue.ChangedPaths) != 1 || issue.BaseSHA != "abc1234" {
		t.Fatalf("AEF metadata was not normalized: %+v", issue)
	}
	if len(issue.Links) != 1 || !strings.HasPrefix(issue.Links[0], cfg.BaseURL+"/browse/TEST-1") {
		t.Errorf("Jira link was not recorded: %v", issue.Links)
	}
}

func TestLoadFailsClosedOnMissingMetadata(t *testing.T) {
	t.Setenv(tokenEnv, "FAKE-test-token")
	source, err := New(testConfig("https://example.atlassian.net"))
	if err != nil {
		t.Fatal(err)
	}
	source.Client = &http.Client{Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"key":"TEST-2","fields":{"summary":"Missing metadata","labels":[]}}`)),
		}, nil
	})}
	if _, err := source.Load(context.Background(), "TEST-2"); err == nil || !strings.Contains(err.Error(), "missing AEF metadata") {
		t.Fatalf("missing metadata was accepted: %v", err)
	}
}

func TestNewRejectsUnsafeConfiguration(t *testing.T) {
	for _, cfg := range []config.Integration{
		{Mode: "fixture", BaseURL: "https://example.atlassian.net", TokenEnv: tokenEnv},
		{Mode: "jira-cloud", BaseURL: "http://example.atlassian.net", TokenEnv: tokenEnv},
		{Mode: "jira-cloud", BaseURL: "https://user@example.atlassian.net", TokenEnv: tokenEnv},
		{Mode: "jira-cloud", BaseURL: "https://example.atlassian.net", TokenEnv: "lower-case"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("unsafe Jira config was accepted: %+v", cfg)
		}
	}
}
