// Package jira implements an opt-in Jira Cloud issue source. It is never used
// unless config selects integrations.jira.mode: jira-cloud. Credentials are read
// from the environment variable named in config, never from flags or URLs.
package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aef-framework/aef/internal/config"
	"github.com/aef-framework/aef/internal/integration"
	"github.com/aef-framework/aef/internal/schema"
	"github.com/aef-framework/aef/internal/util"
)

var (
	keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[0-9]+$`)
	shaPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

// Source fetches issues from the Jira Cloud REST API.
type Source struct {
	BaseURL  string
	TokenEnv string
	Client   *http.Client
	Timeout  time.Duration
}

// New validates Jira settings and builds a source. It does not read credentials.
func New(cfg config.Integration) (*Source, error) {
	if cfg.Mode != "jira-cloud" {
		return nil, fmt.Errorf("jira mode %q is not live Jira Cloud", cfg.Mode)
	}
	base, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	if !config.IsEnvName(cfg.TokenEnv) {
		return nil, fmt.Errorf("invalid jira tokenEnv %q", cfg.TokenEnv)
	}
	return &Source{BaseURL: base.String(), TokenEnv: cfg.TokenEnv, Timeout: 20 * time.Second}, nil
}

// ParseIssue accepts a Jira Cloud browse URL or a bare issue key. A URL is
// accepted only when its host matches the configured Jira baseURL.
func ParseIssue(value string, cfg config.Integration) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("jira issue value is empty")
	}
	base, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return "", err
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" {
			return "", fmt.Errorf("unsupported Jira link %q", util.Sanitize(value))
		}
		if parsed.RawQuery != "" {
			query := parsed.Query()
			if len(query) != 1 || !keyPattern.MatchString(query.Get("selectedIssue")) {
				return "", fmt.Errorf("unsupported Jira link %q", util.Sanitize(value))
			}
		}
		if !strings.EqualFold(parsed.Host, base.Host) {
			return "", fmt.Errorf("Jira link host %q does not match configured baseURL host %q", parsed.Host, base.Host)
		}
		if key := issueKeyFromURL(parsed); key != "" {
			return key, nil
		}
		return "", fmt.Errorf("Jira link does not contain a supported issue key")
	}
	if !keyPattern.MatchString(value) {
		return "", fmt.Errorf("%q is not a Jira key or supported Jira link", util.Sanitize(value))
	}
	return value, nil
}

// Load fetches one issue and normalizes it into the provider-neutral contract.
// Missing AEF metadata is a failure; no default acceptance criteria are invented.
func (s *Source) Load(ctx context.Context, key string) (integration.Issue, error) {
	if !keyPattern.MatchString(key) {
		return integration.Issue{}, fmt.Errorf("invalid Jira issue key %q", util.Sanitize(key))
	}
	if s == nil || s.BaseURL == "" || !config.IsEnvName(s.TokenEnv) {
		return integration.Issue{}, fmt.Errorf("Jira source is not configured")
	}
	token := os.Getenv(s.TokenEnv)
	if token == "" {
		return integration.Issue{}, fmt.Errorf("Jira token environment variable %s is not set", s.TokenEnv)
	}
	endpoint := s.BaseURL + "/rest/api/3/issue/" + key + "?fields=summary,description,labels"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return integration.Issue{}, fmt.Errorf("build Jira request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: s.Timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return integration.Issue{}, fmt.Errorf("Jira request failed: %s", util.Sanitize(err.Error()))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return integration.Issue{}, fmt.Errorf("read Jira response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return integration.Issue{}, fmt.Errorf("Jira returned status %d: %s", resp.StatusCode, util.Sanitize(string(raw)))
	}
	var payload struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string `json:"summary"`
			Description any    `json:"description"`
			Labels      []any  `json:"labels"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return integration.Issue{}, fmt.Errorf("parse Jira response: %w", err)
	}
	issueKey := strings.TrimSpace(payload.Key)
	if issueKey == "" {
		issueKey = key
	}
	if !keyPattern.MatchString(issueKey) || issueKey != key {
		return integration.Issue{}, fmt.Errorf("Jira response key %q does not match requested key", util.Sanitize(issueKey))
	}
	labels := make([]string, 0, len(payload.Fields.Labels))
	for _, value := range payload.Fields.Labels {
		if text, ok := value.(string); ok && text != "" {
			labels = append(labels, util.Sanitize(text))
		}
	}
	description := util.Sanitize(strings.TrimSpace(flattenText(payload.Fields.Description)))
	meta := parseMetadata(description, labels)
	if len(meta.acceptance) == 0 || len(meta.paths) == 0 {
		return integration.Issue{}, fmt.Errorf("Jira issue %s is missing AEF metadata: add aef-acceptance: and aef-path: labels or description sections", key)
	}
	issue := integration.Issue{
		SchemaVersion:      "1.0",
		Key:                issueKey,
		Summary:            util.Sanitize(strings.TrimSpace(payload.Fields.Summary)),
		Description:        description,
		AcceptanceCriteria: meta.acceptance,
		ChangedPaths:       meta.paths,
		Semantics:          meta.semantics,
		Dependencies:       meta.dependencies,
		Links:              []string{s.BaseURL + "/browse/" + issueKey},
		BaseSHA:            meta.baseSHA,
		Source:             "jira-cloud",
	}
	generic, err := util.JSONValue(issue)
	if err != nil {
		return integration.Issue{}, err
	}
	if err := schema.Validate("issue", generic); err != nil {
		return integration.Issue{}, err
	}
	return issue, nil
}

type issueMetadata struct {
	acceptance   []string
	paths        []string
	semantics    []string
	dependencies []string
	baseSHA      string
}

func parseMetadata(description string, labels []string) issueMetadata {
	meta := issueMetadata{}
	section := ""
	for _, line := range strings.Split(description, "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
		lower := strings.ToLower(trimmed)
		switch lower {
		case "acceptance criteria", "changed paths":
			section = lower
			continue
		case "semantics", "dependencies":
			section = lower
			continue
		}
		if strings.HasPrefix(trimmed, "aef-") || trimmed == "" {
			section = ""
		}
		switch section {
		case "acceptance criteria":
			meta.acceptance = appendUnique(meta.acceptance, trimmed)
		case "changed paths":
			meta.paths = appendUnique(meta.paths, trimmed)
		case "semantics":
			meta.semantics = appendUnique(meta.semantics, trimmed)
		case "dependencies":
			meta.dependencies = appendUnique(meta.dependencies, trimmed)
		}
	}
	for _, label := range labels {
		lower := strings.ToLower(label)
		switch {
		case strings.HasPrefix(lower, "aef-acceptance:"):
			meta.acceptance = appendUnique(meta.acceptance, strings.TrimSpace(label[len("aef-acceptance:"):]))
		case strings.HasPrefix(lower, "aef-path:"):
			meta.paths = appendUnique(meta.paths, strings.TrimSpace(label[len("aef-path:"):]))
		case strings.HasPrefix(lower, "aef-semantics:"):
			meta.semantics = appendUnique(meta.semantics, strings.TrimSpace(label[len("aef-semantics:"):]))
		case strings.HasPrefix(lower, "aef-dependency:"):
			meta.dependencies = appendUnique(meta.dependencies, strings.TrimSpace(label[len("aef-dependency:"):]))
		case strings.HasPrefix(lower, "aef-base-sha:"):
			value := strings.TrimSpace(label[len("aef-base-sha:"):])
			if shaPattern.MatchString(value) {
				meta.baseSHA = value
			}
		}
	}
	return meta
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func flattenText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := flattenText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if content, ok := typed["content"].([]any); ok {
			return flattenText(content)
		}
		if text, ok := typed["text"].(string); ok {
			return text
		}
	}
	return ""
}

func issueKeyFromURL(parsed *url.URL) string {
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if keyPattern.MatchString(parts[i]) {
			return parts[i]
		}
	}
	if selected := parsed.Query().Get("selectedIssue"); keyPattern.MatchString(selected) {
		return selected
	}
	return ""
}

func normalizeBaseURL(value string) (*url.URL, error) {
	value = strings.TrimSpace(strings.TrimRight(value, "/"))
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("jira baseURL must be an https origin with no path, query, or credentials")
	}
	return parsed, nil
}
