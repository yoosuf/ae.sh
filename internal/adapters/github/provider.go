// Package github implements an opt-in GitHub adapter over the local `gh` CLI.
// It uses argument arrays, reads credentials only from the environment named in
// config, and never performs push, PR creation, merge, or deployment actions.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/aef-framework/aef/internal/config"
	"github.com/aef-framework/aef/internal/util"
)

const maxDiffBytes = 2 << 20

var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$`)

// Review is a PR review summary returned by `gh pr view`.
type Review struct {
	Author      ghActor `json:"author"`
	State       string  `json:"state"`
	Body        string  `json:"body"`
	SubmittedAt string  `json:"submittedAt"`
}

// Comment is a PR issue comment returned by `gh pr view`.
type Comment struct {
	Author    ghActor `json:"author"`
	Body      string  `json:"body"`
	CreatedAt string  `json:"createdAt"`
}

type ghActor struct {
	Login string `json:"login"`
}

// PR is the provider-neutral subset of GitHub PR state used for evidence.
type PR struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	State          string    `json:"state"`
	URL            string    `json:"url"`
	HeadSHA        string    `json:"headSHA"`
	BaseRef        string    `json:"baseRef"`
	ReviewDecision string    `json:"reviewDecision,omitempty"`
	Reviews        []Review  `json:"reviews,omitempty"`
	Comments       []Comment `json:"comments,omitempty"`
}

type prJSON struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	State          string    `json:"state"`
	URL            string    `json:"url"`
	HeadSHA        string    `json:"headRefOid"`
	BaseRef        string    `json:"baseRefName"`
	ReviewDecision string    `json:"reviewDecision"`
	Reviews        []Review  `json:"reviews"`
	Comments       []Comment `json:"comments"`
}

// UnmarshalJSON maps gh CLI field names onto the provider-neutral PR shape.
func (p *PR) UnmarshalJSON(raw []byte) error {
	var value prJSON
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*p = PR{
		Number: value.Number, Title: value.Title, State: value.State, URL: value.URL,
		HeadSHA: value.HeadSHA, BaseRef: value.BaseRef, ReviewDecision: value.ReviewDecision,
		Reviews: value.Reviews, Comments: value.Comments,
	}
	return nil
}

// Check is one PR status check.
type Check struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	Conclusion string `json:"conclusion,omitempty"`
}

// Source talks to GitHub through the user's authenticated `gh` CLI. Binary is
// injectable for local stub tests. AllowMutations must be enabled explicitly
// for the single supported write action: posting a PR comment.
type Source struct {
	Binary         string
	Host           string
	TokenEnv       string
	AllowMutations bool
}

// New validates a GitHub local-cli integration without reading credentials.
func New(cfg config.Integration) (*Source, error) {
	if cfg.Mode != "local-cli" {
		return nil, fmt.Errorf("GitHub adapter requires integrations.github.mode: local-cli")
	}
	source := &Source{Binary: "gh", TokenEnv: cfg.TokenEnv}
	if cfg.BaseURL != "" {
		parsed, err := parseHTTPSOrigin(cfg.BaseURL)
		if err != nil {
			return nil, err
		}
		source.Host = parsed
	}
	return source, nil
}

func parseHTTPSOrigin(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("integrations.github.baseURL must be an https origin with no path, query, or credentials")
	}
	return parsed.Host, nil
}

// ViewPR reads PR metadata, reviews, and comments. It never mutates GitHub.
func (s *Source) ViewPR(ctx context.Context, ref string) (PR, error) {
	if err := validateRef(ref); err != nil {
		return PR{}, err
	}
	raw, err := s.run(ctx, []string{"pr", "view", ref, "--json",
		"number,title,state,url,headRefOid,baseRefName,reviewDecision,reviews,comments"})
	if err != nil {
		return PR{}, err
	}
	var pr PR
	if err := json.Unmarshal(raw, &pr); err != nil {
		return PR{}, fmt.Errorf("parse gh pr view output: %w", err)
	}
	pr.Title = util.Sanitize(pr.Title)
	for i := range pr.Reviews {
		pr.Reviews[i].Body = util.Sanitize(pr.Reviews[i].Body)
	}
	for i := range pr.Comments {
		pr.Comments[i].Body = util.Sanitize(pr.Comments[i].Body)
	}
	return pr, nil
}

// Checks reads the current PR check conclusions.
func (s *Source) Checks(ctx context.Context, ref string) ([]Check, error) {
	if err := validateRef(ref); err != nil {
		return nil, err
	}
	raw, err := s.run(ctx, []string{"pr", "checks", ref, "--json", "name,state,conclusion"})
	if err != nil {
		return nil, err
	}
	var checks []Check
	if err := json.Unmarshal(raw, &checks); err != nil {
		return nil, fmt.Errorf("parse gh pr checks output: %w", err)
	}
	for i := range checks {
		checks[i].Name = util.Sanitize(checks[i].Name)
		checks[i].State = util.Sanitize(checks[i].State)
		checks[i].Conclusion = util.Sanitize(checks[i].Conclusion)
	}
	return checks, nil
}

// Diff reads the unified PR diff for local, deterministic review scanning.
func (s *Source) Diff(ctx context.Context, ref string) ([]byte, error) {
	if err := validateRef(ref); err != nil {
		return nil, err
	}
	raw, err := s.run(ctx, []string{"pr", "diff", ref})
	if err != nil {
		return nil, err
	}
	if len(raw) > maxDiffBytes {
		return nil, fmt.Errorf("PR diff exceeds %d bytes", maxDiffBytes)
	}
	return raw, nil
}

// Comment posts a sanitized PR comment. This is the only mutating operation in
// the adapter and requires AllowMutations.
func (s *Source) Comment(ctx context.Context, ref, body string) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	if !s.AllowMutations {
		return fmt.Errorf("GitHub comment requires explicit mutation authorization")
	}
	body = strings.TrimSpace(util.Sanitize(body))
	if body == "" {
		return fmt.Errorf("GitHub comment body is empty")
	}
	if len(body) > 4000 {
		body = body[:4000] + "…"
	}
	_, err := s.run(ctx, []string{"pr", "comment", ref, "--body", body})
	return err
}

func validateRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if !refPattern.MatchString(ref) {
		return fmt.Errorf("invalid GitHub PR reference %q", util.Sanitize(ref))
	}
	return nil
}

func (s *Source) run(ctx context.Context, args []string) ([]byte, error) {
	binary := s.Binary
	if binary == "" {
		binary = "gh"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("GitHub CLI %q is not installed; no GitHub call was made", binary)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = s.env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %s failed: %s: %w", strings.Join(args[:2], " "), util.Sanitize(strings.TrimSpace(stderr.String())), err)
	}
	return stdout.Bytes(), nil
}

func (s *Source) env() []string {
	env := []string{}
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "GH_TOKEN=") || strings.HasPrefix(entry, "GITHUB_TOKEN=") || strings.HasPrefix(entry, "GH_HOST=") {
			continue
		}
		env = append(env, entry)
	}
	if s.TokenEnv != "" {
		if token := os.Getenv(s.TokenEnv); token != "" {
			// The token is passed only to the child process environment; it is
			// never logged, written to evidence, or placed in an argument.
			env = append(env, "GH_TOKEN="+token)
		}
	}
	if s.Host != "" {
		env = append(env, "GH_HOST="+s.Host)
	}
	return env
}
