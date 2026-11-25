package workflow

import (
	"strings"
	"testing"

	"github.com/yoosuf/ae.sh/internal/framework"
)

const workflowHeader = `
schemaVersion: "1.0"
id: test-workflow
version: "1.0.0"
name: Test workflow
states:
  - queued
  - analyzing
  - planning
  - awaiting_approval
  - implementing
  - verifying
  - reviewing
  - remediating
  - awaiting_merge
  - staging
  - awaiting_release
  - production_verification
  - completed
  - blocked
  - failed
  - cancelled
initial: queued
terminal: [completed, blocked]
`

func TestParseAcceptsValidDAG(t *testing.T) {
	raw := workflowHeader + `
tasks:
  - id: plan
    name: Plan
    type: skill
    skill: unit-testing
    dependsOn: []
    on:
      state: planning
  - id: approve-plan
    name: Approve plan
    type: approval
    gate: architecture
    dependsOn: [plan]
    on:
      state: awaiting_approval
`
	wf, err := Parse(framework.Relative(framework.Workflows, "test-workflow.yaml"), []byte(raw), map[string]bool{"unit-testing": true}, nil)
	if err != nil {
		t.Fatalf("valid workflow rejected: %v", err)
	}
	order, err := wf.TopologicalOrder()
	if err != nil {
		t.Fatalf("topological order failed: %v", err)
	}
	if strings.Join(order, ",") != "plan,approve-plan" {
		t.Errorf("unexpected order: %v", order)
	}
}

func TestParseRejectsCycle(t *testing.T) {
	raw := workflowHeader + `
tasks:
  - id: a
    name: A
    skill: unit-testing
    dependsOn: [b]
  - id: b
    name: B
    skill: unit-testing
    dependsOn: [a]
`
	_, err := Parse(framework.Relative(framework.Workflows, "cycle.yaml"), []byte(raw), map[string]bool{"unit-testing": true}, nil)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle rejection, got %v", err)
	}
}

func TestParseRejectsUnknownSkillAndState(t *testing.T) {
	base := `
tasks:
  - id: plan
    name: Plan
    skill: not-a-skill
    dependsOn: []
  - id: implement
    name: Implement
    skill: unit-testing
    dependsOn: [plan]
`
	_, err := Parse(framework.Relative(framework.Workflows, "unknown-skill.yaml"), []byte(workflowHeader+base), map[string]bool{"unit-testing": true}, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown skill") {
		t.Fatalf("expected unknown skill rejection, got %v", err)
	}

	badState := strings.Replace(workflowHeader, "  - remediating\n", "  - time-travel\n", 1)
	validTasks := strings.Replace(base, "not-a-skill", "unit-testing", 1)
	_, err = Parse(framework.Relative(framework.Workflows, "unknown-state.yaml"), []byte(badState+validTasks), map[string]bool{"unit-testing": true}, nil)
	if err == nil {
		t.Fatal("expected unknown state rejection")
	}
}
