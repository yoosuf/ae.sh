package policy

import (
	"strings"
	"testing"
)

func testBundle() Bundle {
	baseline := Policy{
		ID:            "default",
		Precedence:    []string{"runtime-safety"},
		RiskRules:     []RiskRule{{ID: "ordinary", Risk: RiskLow, Paths: []string{"**/*.go"}, Semantics: []string{"business-logic"}}},
		ApprovalRules: []ApprovalRule{{Gate: "architecture", MinApprovers: 1, RejectSelfApproval: true}},
		Gates:         Gates{Required: []string{"architecture", "merge"}},
	}
	high := Policy{
		ID:         "high-risk",
		Precedence: []string{"runtime-safety"},
		AppliesTo:  []RiskLevel{RiskHigh, RiskCritical},
		RiskRules: []RiskRule{
			{ID: "auth", Risk: RiskHigh, Paths: []string{"**/*.go"}, Semantics: []string{"authentication"}},
			{ID: "secrets", Risk: RiskCritical, Paths: []string{"**/*.go"}, Semantics: []string{"secrets"}},
		},
		Escalations:   []Escalation{{ID: "security-review", Triggers: []string{"secrets"}}},
		Gates:         Gates{Required: []string{"migration-safety"}},
		ApprovalRules: []ApprovalRule{{Gate: "architecture", MinApprovers: 2, RejectSelfApproval: true, RequireDistinctRoles: true}},
	}
	return NewBundle(baseline, high)
}

func TestClassifyUsesHighestMatchingRisk(t *testing.T) {
	b := testBundle()
	got := b.Classify([]string{"internal/app.go"}, []string{"business-logic", "authentication", "secrets"})
	if got.Level != RiskCritical {
		t.Fatalf("expected critical risk, got %s", got.Level)
	}
	if len(got.Escalations) != 1 || got.Escalations[0] != "security-review" {
		t.Errorf("unexpected escalations: %v", got.Escalations)
	}
}

func TestClassifyUnknownSemanticsFailClosed(t *testing.T) {
	b := testBundle()
	got := b.Classify([]string{"main.go"}, []string{"surprise-tag"})
	if got.Level != RiskUnknown {
		t.Fatalf("expected unknown risk, got %s", got.Level)
	}
	if len(got.Unknown) != 1 || got.Unknown[0] != "surprise-tag" {
		t.Fatalf("unknown semantics were not reported: %+v", got)
	}
	decision := b.Check(Action{Name: "task", Risk: RiskLow}, got, map[string]bool{})
	if decision.Allowed {
		t.Fatal("unknown semantics must deny the action")
	}
	if !strings.Contains(strings.Join(decision.Reasons, "; "), "surprise-tag") {
		t.Errorf("reason did not identify unknown semantics: %v", decision.Reasons)
	}
}

func TestRequiredGatesAndApprovalRulesAreRiskScoped(t *testing.T) {
	b := testBundle()
	low := b.RequiredGatesFor(RiskLow)
	if strings.Contains(strings.Join(low, ","), "migration-safety") {
		t.Errorf("high-risk gate leaked into low-risk run: %v", low)
	}
	high := b.RequiredGatesFor(RiskHigh)
	if !contains(high, "migration-safety") {
		t.Errorf("high-risk gate missing at high risk: %v", high)
	}
	rule, err := b.ApprovalRuleFor("architecture", RiskLow)
	if err != nil || rule.MinApprovers != 1 {
		t.Fatalf("low-risk approval rule: %+v, %v", rule, err)
	}
	rule, err = b.ApprovalRuleFor("architecture", RiskCritical)
	if err != nil || rule.MinApprovers != 2 || !rule.RequireDistinctRoles {
		t.Fatalf("critical approval rule: %+v, %v", rule, err)
	}
	if _, err := b.ApprovalRuleFor("missing-gate", RiskLow); err == nil {
		t.Fatal("unknown gate must fail closed")
	}
}

func TestCheckDeniesUnsafeActionsAndMissingEvidence(t *testing.T) {
	b := testBundle()
	known := Classification{Level: RiskLow}
	cases := []struct {
		name   string
		action Action
	}{
		{"force push", Action{Name: "force-push", Risk: RiskLow}},
		{"deploy without approval", Action{Name: "deploy", Risk: RiskLow}},
		{"missing evidence", Action{Name: "merge", Risk: RiskLow, GatesRequired: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if d := b.Check(tc.action, known, map[string]bool{}); d.Allowed {
				t.Fatalf("expected denial for %s", tc.name)
			}
		})
	}
	d := b.Check(Action{Name: "merge", Risk: RiskLow, GatesRequired: true}, known, map[string]bool{"architecture": true, "merge": true})
	if !d.Allowed {
		t.Fatalf("expected allow with complete evidence, got %v", d.Reasons)
	}
}

func TestCheckRejectsStaleApprovalAndSelfApproval(t *testing.T) {
	b := testBundle()
	classification := Classification{Level: RiskLow}
	action := Action{
		Name: "approve/architecture", Risk: RiskLow,
		CandidateSHA: "new-sha", ArtifactDigest: "sha256:new-plan",
		Approval: &Approval{
			Gate: "architecture", Approver: "reviewer", IdentitySource: "local-manual",
			CandidateSHA: "old-sha", ArtifactDigest: "sha256:old-plan",
		},
	}
	d := b.Check(action, classification, map[string]bool{})
	if d.Allowed {
		t.Fatal("stale approval must be denied")
	}
	reasons := strings.Join(d.Reasons, "; ")
	if !strings.Contains(reasons, "stale approval") {
		t.Errorf("stale approval reason missing: %v", d.Reasons)
	}
	action.Approval.CandidateSHA = "new-sha"
	action.Approval.ArtifactDigest = "sha256:new-plan"
	action.SelfApproval = true
	if d := b.Check(action, classification, map[string]bool{}); d.Allowed {
		t.Fatal("self-approval must be denied")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
