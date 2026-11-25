package execution

import (
	"testing"

	"github.com/yoosuf/ae.sh/internal/adapters/claude"
	"github.com/yoosuf/ae.sh/internal/adapters/fake"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/routing"
	"github.com/yoosuf/ae.sh/internal/workflow"
)

func TestResolveRouteUsesTrustedPrecedence(t *testing.T) {
	e := &Engine{
		Provider:    fake.New(),
		Routes:      config.DefaultRoutes(),
		SkillRoutes: map[string]string{"unit-testing": routing.DeepReasoning},
		AgentRoutes: map[string]string{"qa-engineer": routing.IndependentReview},
	}
	cases := []struct {
		name string
		task workflow.Task
		want string
	}{
		{
			name: "task override wins",
			task: workflow.Task{Skill: "unit-testing", Agent: "qa-engineer", RoutingProfile: routing.Fast},
			want: routing.Fast,
		},
		{
			name: "skill wins over agent",
			task: workflow.Task{Skill: "unit-testing", Agent: "qa-engineer"},
			want: routing.DeepReasoning,
		},
		{
			name: "agent fallback",
			task: workflow.Task{Skill: "unknown-skill", Agent: "qa-engineer"},
			want: routing.IndependentReview,
		},
		{
			name: "balanced default",
			task: workflow.Task{Skill: "unknown-skill", Agent: "unknown-agent"},
			want: routing.Balanced,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile, model, err := e.resolveRoute(tc.task)
			if err != nil {
				t.Fatal(err)
			}
			if profile != tc.want {
				t.Fatalf("route %q, want %q", profile, tc.want)
			}
			// The fake provider is offline and must not require a model mapping.
			if model != "" {
				t.Fatalf("fake provider received requested model %q", model)
			}
		})
	}
}

func TestResolveRouteLiveProviderResolvesModelFailsClosed(t *testing.T) {
	e := &Engine{
		Provider:    claude.New(),
		Routes:      config.DefaultRoutes(),
		SkillRoutes: map[string]string{"unit-testing": routing.Fast},
	}
	profile, model, err := e.resolveRoute(workflow.Task{Skill: "unit-testing"})
	if err != nil {
		t.Fatal(err)
	}
	if profile != routing.Fast || model != "haiku" {
		t.Fatalf("got route %q model %q, want fast/haiku", profile, model)
	}

	e.Routes = map[string]map[string]string{routing.Fast: {"codex": "gpt-6-luna"}}
	if _, _, err := e.resolveRoute(workflow.Task{Skill: "unit-testing"}); err == nil {
		t.Fatal("missing Claude model mapping did not fail closed")
	}

	if _, _, err := e.resolveRoute(workflow.Task{RoutingProfile: "cheapest"}); err == nil {
		t.Fatal("unknown routing profile did not fail closed")
	}
}
