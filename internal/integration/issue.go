package integration

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/aef-framework/aef/internal/schema"
	"github.com/aef-framework/aef/internal/util"
)

// Issue is the provider-neutral issue contract. Jira and fixture adapters both
// produce this shape. All text fields are untrusted.
type Issue struct {
	SchemaVersion      string   `json:"schemaVersion" yaml:"schemaVersion"`
	Key                string   `json:"key" yaml:"key"`
	Summary            string   `json:"summary" yaml:"summary"`
	Description        string   `json:"description,omitempty" yaml:"description,omitempty"`
	AcceptanceCriteria []string `json:"acceptanceCriteria" yaml:"acceptanceCriteria"`
	ChangedPaths       []string `json:"changedPaths" yaml:"changedPaths"`
	Semantics          []string `json:"semantics,omitempty" yaml:"semantics,omitempty"`
	Dependencies       []string `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Links              []string `json:"links,omitempty" yaml:"links,omitempty"`
	BaseSHA            string   `json:"baseSHA,omitempty" yaml:"baseSHA,omitempty"`
	Source             string   `json:"source,omitempty" yaml:"source,omitempty"`
}

// IssueSource loads an issue from a fixture, Jira Cloud, or another explicitly
// configured adapter. Source output is untrusted data, never control input.
type IssueSource interface {
	Load(ctx context.Context, key string) (Issue, error)
}

// LoadIssue reads JSON or YAML issue fixture and validates it.
func LoadIssue(path string) (Issue, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Issue{}, err
	}
	var generic map[string]any
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		return Issue{}, fmt.Errorf("parse issue fixture: %w", err)
	}
	doc, err := util.JSONValue(generic)
	if err != nil {
		return Issue{}, err
	}
	if err := schema.Validate("issue", doc); err != nil {
		return Issue{}, err
	}
	var issue Issue
	if err := yaml.Unmarshal(raw, &issue); err != nil {
		return Issue{}, err
	}
	if issue.Source == "" {
		issue.Source = "fixture"
	}
	issue.Summary = util.Sanitize(issue.Summary)
	issue.Description = util.Sanitize(issue.Description)
	for i := range issue.AcceptanceCriteria {
		issue.AcceptanceCriteria[i] = util.Sanitize(issue.AcceptanceCriteria[i])
	}
	return issue, nil
}
