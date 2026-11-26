package execution

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/yoosuf/ae.sh/internal/evidence"
)

func TestGateEvidenceRequiresIndependentProviderReview(t *testing.T) {
	ctx := context.Background()
	exit := 0
	cases := []struct {
		name string
		kind string
		data map[string]any
		gate string
		want bool
	}{
		{
			name: "local pattern scan cannot satisfy independent review",
			kind: "review",
			data: map[string]any{
				"gate": "independent-review", "mode": "local-pattern-scan",
				"routingProfile": "independent-review", "requestedModel": "fable",
			},
			gate: "independent-review",
		},
		{
			name: "simulated fake review cannot satisfy independent review",
			kind: "review",
			data: map[string]any{
				"gate": "independent-review", "mode": "provider-backed-review",
				"routingProfile": "independent-review", "provider": "fake",
				"requestedModel": "fable", "independentReview": true, "simulated": true,
			},
			gate: "independent-review",
		},
		{
			name: "observed provider review satisfies independent review",
			kind: "review",
			data: map[string]any{
				"gate": "independent-review", "mode": "provider-backed-review",
				"routingProfile": "independent-review", "provider": "claude",
				"requestedModel": "fable", "independentReview": true, "simulated": false,
				"modelIdentityObserved": true,
			},
			gate: "independent-review",
			want: true,
		},
		{
			name: "unobserved model identity cannot satisfy independent review",
			kind: "review",
			data: map[string]any{
				"gate": "independent-review", "mode": "provider-backed-review",
				"routingProfile": "independent-review", "provider": "claude",
				"requestedModel": "fable", "independentReview": true, "simulated": false,
				"modelIdentityObserved": false,
			},
			gate: "independent-review",
		},
		{
			name: "observed test evidence satisfies its own gate",
			kind: "test",
			data: map[string]any{"gate": "unit-tests", "passed": true},
			gate: "unit-tests",
			want: true,
		},
		{
			name: "test evidence cannot satisfy independent review",
			kind: "test",
			data: map[string]any{"gate": "independent-review", "passed": true},
			gate: "independent-review",
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			manager := evidence.New(filepath.Join(root, ".ae", "evidence"), filepath.Join(root, ".ae", "objects"))
			runID := "run-gate"
			record, err := manager.Write(ctx, runID, i, tc.kind, tc.name, "ae-test", tc.data, &exit, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			got := gateEvidenceFromRecords([]EvidenceRecord{{RunID: runID, JSON: string(raw)}})
			if got[tc.gate] != tc.want {
				t.Fatalf("gate %q = %v, want %v; evidence map: %v", tc.gate, got[tc.gate], tc.want, got)
			}
		})
	}
}

func TestEvidenceKindPrefersReviewSuffixOverTest(t *testing.T) {
	cases := map[string]string{
		"test-review":         "review",
		"security-review":     "review",
		"architecture-review": "review",
		"code-review":         "review",
		"unit-testing":        "test",
		"risk-assessment":     "risk",
		"agent-handoff":       "artifact",
		"backend-development": "note",
	}
	for skill, want := range cases {
		if got := evidenceKind(skill); got != want {
			t.Errorf("evidenceKind(%q) = %q, want %q", skill, got, want)
		}
	}
}
