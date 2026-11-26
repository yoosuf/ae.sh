// Package workflow loads versioned DAG workflow definitions and rejects cycles,
// unknown skills, unsupported providers, and invalid policies.
package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/yoosuf/ae.sh/internal/framework"
	"github.com/yoosuf/ae.sh/internal/routing"
	"github.com/yoosuf/ae.sh/internal/schema"
	"github.com/yoosuf/ae.sh/internal/util"
)

// MaxTaskTimeout bounds a single provider task. Canonical workflows use up to
// 120m for long-running implementation and verification tasks.
const MaxTaskTimeout = 120 * time.Minute

// Task is a node in the workflow DAG.
type Task struct {
	ID                string         `yaml:"id" json:"id"`
	Name              string         `yaml:"name" json:"name"`
	Type              string         `yaml:"type" json:"type"`
	Skill             string         `yaml:"skill" json:"skill,omitempty"`
	Agent             string         `yaml:"agent" json:"agent,omitempty"`
	RoutingProfile    string         `yaml:"routingProfile" json:"routingProfile,omitempty"`
	DependsOn         []string       `yaml:"dependsOn" json:"dependsOn"`
	Timeout           string         `yaml:"timeout" json:"timeout,omitempty"`
	Outputs           []string       `yaml:"outputs" json:"outputs,omitempty"`
	Condition         string         `yaml:"condition" json:"condition,omitempty"`
	Gate              string         `yaml:"gate" json:"gate,omitempty"`
	RequiredApprovers int            `yaml:"requiredApprovers" json:"requiredApprovers,omitempty"`
	BindToArtifact    string         `yaml:"bindToArtifact" json:"bindToArtifact,omitempty"`
	On                map[string]any `yaml:"on" json:"on,omitempty"`
	OnFailure         map[string]any `yaml:"onFailure" json:"onFailure,omitempty"`
	BoundedRetries    map[string]any `yaml:"boundedRetries" json:"boundedRetries,omitempty"`
}

// Workflow is a validated workflow definition.
type Workflow struct {
	SchemaVersion   string         `yaml:"schemaVersion" json:"schemaVersion"`
	ID              string         `yaml:"id" json:"id"`
	Version         string         `yaml:"version" json:"version"`
	Name            string         `yaml:"name" json:"name"`
	Description     string         `yaml:"description" json:"description,omitempty"`
	DeploymentScope string         `yaml:"deploymentScope" json:"deploymentScope,omitempty"`
	States          []string       `yaml:"states" json:"states"`
	Initial         string         `yaml:"initial" json:"initial"`
	Terminal        []string       `yaml:"terminal" json:"terminal"`
	Tasks           []Task         `yaml:"tasks" json:"tasks"`
	Remediation     map[string]any `yaml:"remediation" json:"remediation,omitempty"`
	Policies        []string       `yaml:"policies" json:"policies,omitempty"`
	Artifacts       []Artifact     `yaml:"artifacts" json:"artifacts,omitempty"`
	Path            string         `yaml:"-" json:"-"`
}

// Artifact is a typed workflow output.
type Artifact struct {
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"`
	Producer string `yaml:"producer" json:"producer"`
}

// Load parses and validates a workflow file.
func Load(path string, knownSkills, knownAgents map[string]bool) (Workflow, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Workflow{}, err
	}
	return Parse(path, raw, knownSkills, knownAgents)
}

// Parse validates YAML, JSON Schema, DAG structure and policy references.
func Parse(path string, raw []byte, knownSkills, knownAgents map[string]bool) (Workflow, error) {
	var generic map[string]any
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		return Workflow{}, fmt.Errorf("parse workflow YAML: %w", err)
	}
	doc, err := util.JSONValue(generic)
	if err != nil {
		return Workflow{}, fmt.Errorf("workflow must be JSON-serializable: %w", err)
	}
	if err := schema.Validate("workflow", doc); err != nil {
		return Workflow{}, err
	}
	var wf Workflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		return Workflow{}, err
	}
	wf.Path = filepath.ToSlash(path)
	if err := wf.Validate(knownSkills, knownAgents); err != nil {
		return Workflow{}, err
	}
	return wf, nil
}

// Validate enforces DAG and reference invariants beyond JSON Schema.
func (w Workflow) Validate(knownSkills, knownAgents map[string]bool) error {
	if w.SchemaVersion != "1.0" {
		return fmt.Errorf("unsupported workflow schemaVersion %q", w.SchemaVersion)
	}
	byID := map[string]Task{}
	for _, task := range w.Tasks {
		if _, exists := byID[task.ID]; exists {
			return fmt.Errorf("duplicate task id %q", task.ID)
		}
		byID[task.ID] = task
	}
	states := map[string]bool{}
	for _, s := range w.States {
		states[s] = true
	}
	if !states[w.Initial] {
		return fmt.Errorf("initial state %q is not declared", w.Initial)
	}
	stateNames := map[string]bool{
		"queued": true, "analyzing": true, "planning": true, "awaiting_approval": true,
		"implementing": true, "verifying": true, "reviewing": true, "remediating": true,
		"awaiting_merge": true, "staging": true, "awaiting_release": true,
		"production_verification": true, "completed": true, "blocked": true,
		"failed": true, "cancelled": true,
	}
	for _, s := range w.States {
		if !stateNames[s] {
			return fmt.Errorf("unknown workflow state %q", s)
		}
	}
	for _, t := range w.Terminal {
		if !states[t] {
			return fmt.Errorf("terminal state %q is not declared", t)
		}
	}
	for _, task := range w.Tasks {
		if task.Type != "approval" {
			if knownSkills != nil && !knownSkills[task.Skill] {
				return fmt.Errorf("task %s references unknown skill %q", task.ID, task.Skill)
			}
		}
		if task.Agent != "" && knownAgents != nil && !knownAgents[task.Agent] {
			return fmt.Errorf("task %s references unknown agent %q", task.ID, task.Agent)
		}
		if task.RoutingProfile != "" {
			if _, err := routing.Normalize(task.RoutingProfile); err != nil {
				return fmt.Errorf("task %s routingProfile: %w", task.ID, err)
			}
		}
		for _, dep := range task.DependsOn {
			if _, ok := byID[dep]; !ok {
				return fmt.Errorf("task %s depends on unknown task %q", task.ID, dep)
			}
			if dep == task.ID {
				return fmt.Errorf("task %s depends on itself", task.ID)
			}
		}
		if task.Type == "approval" && task.Gate == "" {
			return fmt.Errorf("approval task %s has no gate", task.ID)
		}
		if len(task.BoundedRetries) > 0 {
			max, ok := task.BoundedRetries["max"].(int)
			if !ok || max < 0 || max > 5 {
				return fmt.Errorf("task %s has invalid boundedRetries.max", task.ID)
			}
		}
		if task.Timeout != "" {
			d, err := time.ParseDuration(task.Timeout)
			if err != nil || d <= 0 || d > MaxTaskTimeout {
				return fmt.Errorf("task %s has invalid timeout %q (want positive duration up to %s)", task.ID, task.Timeout, MaxTaskTimeout)
			}
		}
		if value, ok := task.On["state"]; ok {
			if !stateNames[fmt.Sprint(value)] {
				return fmt.Errorf("task %s maps to unknown state %q", task.ID, value)
			}
		}
	}
	if order, err := w.TopologicalOrder(); err != nil {
		return err
	} else if len(order) != len(w.Tasks) {
		return fmt.Errorf("workflow has unreachable tasks")
	}
	for _, policy := range w.Policies {
		clean := filepath.Clean(policy)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("policy path %q must be repository-relative", policy)
		}
	}
	artifacts := map[string]bool{}
	for _, a := range w.Artifacts {
		if artifacts[a.Name] {
			return fmt.Errorf("duplicate artifact %q", a.Name)
		}
		artifacts[a.Name] = true
		if _, ok := byID[a.Producer]; !ok {
			return fmt.Errorf("artifact %q has unknown producer %q", a.Name, a.Producer)
		}
	}
	return nil
}

// TopologicalOrder returns task IDs in dependency order. Cycles are rejected;
// remediation loops are modeled as bounded retries, not DAG cycles.
func (w Workflow) TopologicalOrder() ([]string, error) {
	indegree := make(map[string]int, len(w.Tasks))
	dependents := make(map[string][]string, len(w.Tasks))
	byID := map[string]Task{}
	for _, t := range w.Tasks {
		byID[t.ID] = t
		indegree[t.ID] = 0
	}
	for _, t := range w.Tasks {
		for _, dep := range t.DependsOn {
			if _, ok := byID[dep]; !ok {
				return nil, fmt.Errorf("task %s depends on unknown task %q", t.ID, dep)
			}
			indegree[t.ID]++
			dependents[dep] = append(dependents[dep], t.ID)
		}
	}
	var ready []string
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	var order []string
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		next := append([]string{}, dependents[id]...)
		sort.Strings(next)
		for _, child := range next {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
			}
		}
		sort.Strings(ready)
	}
	if len(order) != len(w.Tasks) {
		var cyclic []string
		for id, degree := range indegree {
			if degree > 0 {
				cyclic = append(cyclic, id)
			}
		}
		sort.Strings(cyclic)
		return nil, fmt.Errorf("workflow contains a cycle involving: %s", strings.Join(cyclic, ", "))
	}
	return order, nil
}

// LoadFromRepo loads a workflow by id from root/framework/workflows.
func LoadFromRepo(root, id string, knownSkills, knownAgents map[string]bool) (Workflow, error) {
	workflowsDir := framework.Dir(root, framework.Workflows)
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		return Workflow{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !isYAML(entry.Name()) {
			continue
		}
		wf, err := Load(filepath.Join(workflowsDir, entry.Name()), knownSkills, knownAgents)
		if err != nil {
			return Workflow{}, err
		}
		if wf.ID == id {
			return wf, nil
		}
	}
	return Workflow{}, fmt.Errorf("workflow %q not found", id)
}

// LoadDirectory loads every workflow YAML in dir.
func LoadDirectory(dir string, knownSkills, knownAgents map[string]bool) ([]Workflow, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Workflow
	var errs []string
	for _, entry := range entries {
		if entry.IsDir() || !isYAML(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		wf, err := Load(path, knownSkills, knownAgents)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", entry.Name(), err))
			continue
		}
		out = append(out, wf)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("workflow validation failed: %s", strings.Join(errs, "; "))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}
