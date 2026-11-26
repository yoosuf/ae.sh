package execution_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoosuf/ae.sh/internal/adapters/fake"
	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
	"github.com/yoosuf/ae.sh/internal/framework"
	"github.com/yoosuf/ae.sh/internal/integration"
	"github.com/yoosuf/ae.sh/internal/policy"
	"github.com/yoosuf/ae.sh/internal/workflow"
)

const stateDeclarations = `
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

func workflowYAML(tasks string) string {
	return `schemaVersion: "1.0"
id: local-flow
version: "1.0.0"
name: Local test flow
deploymentScope: optional
` + stateDeclarations + `tasks:
` + tasks
}

const baseTasks = `  - id: intake
    name: Intake
    skill: unit-testing
    dependsOn: []
    on:
      state: analyzing
  - id: plan
    name: Plan
    skill: unit-testing
    dependsOn: [intake]
    on:
      state: planning
  - id: approve-plan
    name: Approve plan
    type: approval
    gate: architecture
    dependsOn: [plan]
    on:
      state: awaiting_approval
  - id: implement
    name: Implement
    skill: unit-testing
    dependsOn: [approve-plan]
    on:
      state: implementing
  - id: pull-request
    name: Pull request
    skill: unit-testing
    dependsOn: [implement]
    on:
      state: awaiting_merge
`

func testPolicies() policy.Bundle {
	return policy.NewBundle(policy.Policy{
		ID: "default", Precedence: []string{"runtime-safety"},
		RiskRules: []policy.RiskRule{{ID: "ordinary", Risk: policy.RiskLow, Paths: []string{"**/*.go"}, Semantics: []string{"business-logic"}}},
		Gates:     policy.Gates{Required: []string{"format"}, NeverMarkMissingAsPassed: true},
		ApprovalRules: []policy.ApprovalRule{
			{Gate: "architecture", MinApprovers: 1, RejectSelfApproval: true},
			{Gate: "merge", MinApprovers: 1, RejectSelfApproval: true},
		},
	})
}

func newEngine(t *testing.T, root, tasks string) *execution.Engine {
	t.Helper()
	if err := os.MkdirAll(framework.Dir(root, framework.Workflows), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".ae"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := framework.Dir(root, framework.Workflows, "local-flow.yaml")
	if err := os.WriteFile(path, []byte(workflowYAML(tasks)), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := sqliteadapter.Open(filepath.Join(root, ".ae", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &execution.Engine{
		Store:    store,
		Policies: testPolicies(),
		Provider: fake.New(),
		Evidence: evidence.New(filepath.Join(root, ".ae", "evidence"), filepath.Join(root, ".ae", "objects")),
		Root:     root,
	}
}

func mustWorkflow(t *testing.T, root string) workflow.Workflow {
	t.Helper()
	wf, err := workflow.LoadFromRepo(root, "local-flow", map[string]bool{
		"unit-testing": true, "code-review": true, "security-review": true,
		"architecture-review": true, "test-review": true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return wf
}

func testIssue() integration.Issue {
	return integration.Issue{
		SchemaVersion: "1.0", Key: "TEST-1", Summary: "Local deterministic test",
		AcceptanceCriteria: []string{"Create a reviewable candidate"},
		ChangedPaths:       []string{"internal/example.go"}, Semantics: []string{"business-logic"},
		BaseSHA: "base-sha", Source: "fixture",
	}
}

func TestDryRunCreatesNoState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	engine := newEngine(t, root, baseTasks)
	run, preview, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ID != "" || !preview.DryRun {
		t.Fatalf("dry run returned run state: %+v, %+v", run, preview)
	}
	runs, err := engine.Store.ListRuns(ctx, 10)
	if err != nil || len(runs) != 0 {
		t.Fatalf("dry run persisted state: %+v, %v", runs, err)
	}
}

func TestProviderEvidenceRecordsRoutingFields(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tasks := `  - id: intake
    name: Intake
    skill: unit-testing
    agent: qa-engineer
    routingProfile: deep-reasoning
    dependsOn: []
    on:
      state: analyzing
  - id: plan
    name: Plan
    skill: unit-testing
    dependsOn: [intake]
    on:
      state: planning
`
	engine := newEngine(t, root, tasks)
	engine.SkillRoutes = map[string]string{"unit-testing": "fast"}
	engine.AgentRoutes = map[string]string{"qa-engineer": "independent-review"}
	run, _, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := engine.Store.Evidence(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var provider evidence.Record
	found := false
	for _, stored := range records {
		var record evidence.Record
		if err := json.Unmarshal([]byte(stored.JSON), &record); err != nil {
			t.Fatal(err)
		}
		if record.Subject == "Task intake" {
			provider, found = record, true
			break
		}
	}
	if !found {
		t.Fatal("no task evidence record was written for intake")
	}
	if provider.Data["routingProfile"] != "deep-reasoning" {
		t.Fatalf("routing profile was not recorded: %+v", provider.Data)
	}
	if provider.Data["requestedModel"] != "" || provider.Data["model"] != "" {
		t.Fatalf("fake provider claimed a model: %+v", provider.Data)
	}
	output, ok := provider.Data["output"].(map[string]any)
	if !ok || output["routingProfile"] != "deep-reasoning" {
		t.Fatalf("provider output lost the routing profile: %+v", provider.Data["output"])
	}
}

func TestDeferredStartDoesNotInvokeProvider(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	engine := newEngine(t, root, baseTasks)
	run, _, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester",
		Workspace: "managed", DeferExecution: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == "" || run.State != string(execution.Queued) || run.Status != execution.StatusRunning {
		t.Fatalf("deferred start advanced the run: %+v", run)
	}
	records, err := engine.Store.Evidence(ctx, run.ID)
	if err != nil || len(records) != 1 {
		t.Fatalf("deferred start should persist only plan evidence: %d records, %v", len(records), err)
	}
	continued, err := engine.Continue(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if continued.State != string(execution.AwaitingApproval) {
		t.Fatalf("continue did not reach the approval gate: %+v", continued)
	}
}

func TestApprovalAndCompletion(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	engine := newEngine(t, root, baseTasks)
	run, _, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.State != string(execution.AwaitingApproval) || run.Status != execution.StatusAwaitingApproval {
		t.Fatalf("expected approval gate, got %+v", run)
	}
	if err := engine.Approve(ctx, run.ID, "architecture", "reviewer-1", "reviewer"); err != nil {
		t.Fatal(err)
	}
	run, err = engine.Continue(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != string(execution.Completed) || run.Status != execution.StatusCompleted {
		t.Fatalf("expected completion, got %+v", run)
	}
}

func TestStaleApprovalBlocks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	engine := newEngine(t, root, baseTasks)
	run, _, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = engine.Store.SaveApproval(ctx, execution.ApprovalRecord{
		RunID: run.ID, Gate: "architecture", Approver: "reviewer-1", Role: "reviewer",
		IdentitySource: "local-manual", ArtifactDigest: "sha256:old-plan", CandidateSHA: "old-sha",
		Timestamp: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Continue(ctx, run.ID)
	if err == nil || !strings.Contains(err.Error(), "stale approval") {
		t.Fatalf("expected stale approval block, got %v", err)
	}
	stored, err := engine.Store.GetRun(ctx, run.ID)
	if err != nil || stored.State != string(execution.Blocked) {
		t.Fatalf("run should be blocked after stale approval: %+v, %v", stored, err)
	}
}

func TestReviewFindingsChangeInvalidatesApproval(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tasks := baseTasks + `  - id: merge-approval
    name: Merge approval
    type: approval
    gate: merge
    dependsOn: [pull-request]
    on:
      state: awaiting_merge
`
	engine := newEngine(t, root, tasks)
	engine.Policies = policy.NewBundle(policy.Policy{
		ID: "review-invalidation", Precedence: []string{"runtime-safety"},
		RiskRules: []policy.RiskRule{{ID: "ordinary", Risk: policy.RiskLow, Paths: []string{"**/*.go"}, Semantics: []string{"business-logic"}}},
		Gates:     policy.Gates{Required: []string{"format"}, NeverMarkMissingAsPassed: true},
		ApprovalRules: []policy.ApprovalRule{
			{Gate: "architecture", MinApprovers: 1, RejectSelfApproval: true},
			{Gate: "merge", MinApprovers: 1, RejectSelfApproval: true, InvalidateOn: []string{"review-findings-change"}},
		},
	})
	run, _, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Approve(ctx, run.ID, "architecture", "reviewer-1", "reviewer"); err != nil {
		t.Fatal(err)
	}
	run, err = engine.Continue(ctx, run.ID)
	if err != nil || run.State != string(execution.AwaitingMerge) {
		t.Fatalf("expected merge gate: %+v, %v", run, err)
	}
	if err := engine.Approve(ctx, run.ID, "merge", "reviewer-2", "release-manager"); err != nil {
		t.Fatal(err)
	}
	record, err := engine.Evidence.Write(ctx, run.ID, 0, "review", "Local diff pattern scan", "ae-review", map[string]any{
		"verified": false, "findings": []any{},
	}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.AppendEvidence(ctx, execution.EvidenceRecord{RunID: run.ID, JSON: string(raw)}); err != nil {
		t.Fatal(err)
	}
	_, err = engine.Continue(ctx, run.ID)
	if err == nil || !strings.Contains(err.Error(), "stale approval: review findings changed") {
		t.Fatalf("expected review-change invalidation, got %v", err)
	}
	blocked, err := engine.Store.GetRun(ctx, run.ID)
	if err != nil || blocked.Status != execution.StatusBlocked {
		t.Fatalf("run should be blocked after review findings change: %+v, %v", blocked, err)
	}
}

func TestUnresolvedReviewFindingBlocksMerge(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tasks := baseTasks + `  - id: merge-approval
    name: Merge approval
    type: approval
    gate: merge
    dependsOn: [pull-request]
    on:
      state: awaiting_merge
`
	engine := newEngine(t, root, tasks)
	run, _, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Approve(ctx, run.ID, "architecture", "reviewer-1", "reviewer"); err != nil {
		t.Fatal(err)
	}
	run, err = engine.Continue(ctx, run.ID)
	if err != nil || run.State != string(execution.AwaitingMerge) {
		t.Fatalf("expected merge gate: %+v, %v", run, err)
	}
	record, err := engine.Evidence.Write(ctx, run.ID, 0, "review", "Local diff pattern scan", "ae-review", map[string]any{
		"verified": false,
		"findings": []any{map[string]any{"id": "rev-0123456789abcdef", "ruleId": "credential-literal", "severity": "high"}},
	}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.AppendEvidence(ctx, execution.EvidenceRecord{RunID: run.ID, JSON: string(raw)}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Approve(ctx, run.ID, "merge", "reviewer-2", "release-manager"); err != nil {
		t.Fatal(err)
	}
	_, err = engine.Continue(ctx, run.ID)
	if err == nil || !strings.Contains(err.Error(), "unresolved review findings: rev-0123456789abcdef") {
		t.Fatalf("expected unresolved finding block, got %v", err)
	}
	blocked, err := engine.Store.GetRun(ctx, run.ID)
	if err != nil || blocked.Status != execution.StatusBlocked {
		t.Fatalf("run should be blocked by unresolved finding: %+v, %v", blocked, err)
	}
}

func TestMergeGateDoesNotInventMissingEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tasks := baseTasks + `  - id: merge-approval
    name: Merge approval
    type: approval
    gate: merge
    dependsOn: [pull-request]
    on:
      state: awaiting_merge
`
	engine := newEngine(t, root, tasks)
	run, _, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Approve(ctx, run.ID, "architecture", "reviewer-1", "reviewer"); err != nil {
		t.Fatal(err)
	}
	run, err = engine.Continue(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != string(execution.AwaitingMerge) {
		t.Fatalf("expected merge gate, got %+v", run)
	}
	if err := engine.Approve(ctx, run.ID, "merge", "reviewer-2", "release-manager"); err != nil {
		t.Fatal(err)
	}
	_, err = engine.Continue(ctx, run.ID)
	if err == nil || !strings.Contains(err.Error(), "missing required evidence: format") {
		t.Fatalf("expected missing-evidence block, got %v", err)
	}
	blocked, err := engine.Store.GetRun(ctx, run.ID)
	if err != nil || blocked.Status != execution.StatusBlocked || !strings.Contains(blocked.TerminalReason, "missing required evidence: format") {
		t.Fatalf("blocked run did not record evidence gap: %+v, %v", blocked, err)
	}
}

type recordingProvider struct {
	requests        []integration.RunRequest
	findingsBySkill map[string][]any
}

func (p *recordingProvider) Name() string { return "fake" }

func (p *recordingProvider) Capabilities() integration.Capabilities {
	return integration.Capabilities{StructuredOutput: true, Cancellation: true}
}

func (p *recordingProvider) Run(_ context.Context, request integration.RunRequest) (integration.RunResult, error) {
	p.requests = append(p.requests, request)
	findings := p.findingsBySkill[request.Skill]
	if findings == nil {
		findings = []any{}
	}
	exit := 0
	return integration.RunResult{
		Output: map[string]any{
			"summary": "ok", "findings": findings, "artifacts": []any{},
		},
		ExitStatus: &exit,
	}, nil
}

func (p *recordingProvider) Cancel(context.Context, string) error { return nil }

func TestReviewTasksNeverReceiveWriteAccess(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tasks := `  - id: implement
    name: Implement
    skill: unit-testing
    agent: backend-engineer
    dependsOn: []
    on:
      state: implementing
  - id: review
    name: Independent review
    skill: unit-testing
    agent: code-reviewer
    routingProfile: independent-review
    dependsOn: [implement]
    on:
      state: reviewing
`
	engine := newEngine(t, root, tasks)
	engine.AllowWrites = true
	engine.AgentWriteAllowed = map[string]bool{"backend-engineer": true, "code-reviewer": false}
	provider := &recordingProvider{}
	engine.Provider = provider
	run, _, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Continue(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected two provider calls, got %d", len(provider.requests))
	}
	if !provider.requests[0].Write {
		t.Fatalf("implementation task lost authorized write access: %+v", provider.requests[0])
	}
	if provider.requests[1].Write {
		t.Fatalf("independent review inherited write access: %+v", provider.requests[1])
	}
}

func TestSpecialistFindingTriggersRemediation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tasks := `  - id: review
    name: General review
    skill: code-review
    dependsOn: []
    on:
      state: reviewing
  - id: security-review
    name: Security review
    skill: security-review
    dependsOn: [review]
    on:
      state: reviewing
  - id: remediate
    name: Bounded remediation
    skill: unit-testing
    dependsOn: [review, security-review]
    condition: "review.findings > 0"
    on:
      state: remediating
`
	engine := newEngine(t, root, tasks)
	provider := &recordingProvider{findingsBySkill: map[string][]any{
		"security-review": []any{map[string]any{
			"id": "rev-0123456789abcdef", "ruleId": "tenant-isolation", "severity": "high",
		}},
	}}
	engine.Provider = provider
	run, _, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Continue(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	remediationRan := false
	for _, request := range provider.requests {
		if request.TaskID == "remediate" {
			remediationRan = true
		}
	}
	if !remediationRan {
		t.Fatalf("specialist finding did not trigger remediation: %+v", provider.requests)
	}
}

type failingProvider struct{ calls int }

func (p *failingProvider) Name() string { return "fake" }

func (p *failingProvider) Capabilities() integration.Capabilities {
	return integration.Capabilities{StructuredOutput: true, Cancellation: true}
}

func (p *failingProvider) Run(context.Context, integration.RunRequest) (integration.RunResult, error) {
	p.calls++
	return integration.RunResult{}, fmt.Errorf("remediation task failed")
}

func (p *failingProvider) Cancel(context.Context, string) error { return nil }

func TestRemediationBudgetStopsFailedRetries(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tasks := `  - id: remediate
    name: Remediate
    skill: unit-testing
    dependsOn: []
    on:
      state: verifying
    boundedRetries:
      max: 3
  - id: verify
    name: Verify
    skill: unit-testing
    dependsOn: [remediate]
    on:
      state: verifying
`
	if err := os.MkdirAll(framework.Dir(root, framework.Workflows), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `schemaVersion: "1.0"
id: local-flow
version: "1.0.0"
name: Local test flow
deploymentScope: local
` + stateDeclarations + `remediation:
  maxAttempts: 1
tasks:
` + tasks
	if err := os.WriteFile(framework.Dir(root, framework.Workflows, "local-flow.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := sqliteadapter.Open(filepath.Join(root, ".ae", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	failing := &failingProvider{}
	engine := &execution.Engine{
		Store:    store,
		Policies: testPolicies(),
		Provider: failing,
		Evidence: evidence.New(filepath.Join(root, ".ae", "evidence"), filepath.Join(root, ".ae", "objects")),
		Root:     root,
	}
	run, _, err := engine.Start(ctx, execution.StartOptions{
		Workflow: mustWorkflow(t, root), Issue: testIssue(), Provider: "fake", Actor: "tester",
	})
	if err == nil || !strings.Contains(err.Error(), "remediation attempt budget exhausted") {
		t.Fatalf("expected remediation budget block, got %v", err)
	}
	if failing.calls != 1 {
		t.Errorf("expected one provider call, got %d", failing.calls)
	}
	if run.RemediationAttempts != 1 || run.Status != execution.StatusBlocked {
		t.Errorf("remediation accounting was not persisted: %+v", run)
	}
	stored, err := store.GetRun(ctx, run.ID)
	if err != nil || stored.RemediationAttempts != 1 {
		t.Errorf("stored remediation attempts: %+v, %v", stored, err)
	}
}
