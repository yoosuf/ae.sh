package util

import (
	"math"
	"strings"
	"testing"
)

func TestDigestIsStableAndVersioned(t *testing.T) {
	first := Digest([]byte("ae"))
	second := Digest([]byte("ae"))
	if first != second {
		t.Fatalf("digest is not stable: %s != %s", first, second)
	}
	if !strings.HasPrefix(first, "sha256:") {
		t.Fatalf("digest is not sha256-prefixed: %s", first)
	}
	if first == Digest([]byte("ae!")) {
		t.Fatal("different input produced the same digest")
	}
}

func TestSanitizeRedactsCredentialShapes(t *testing.T) {
	// Synthetic shapes only; these are not credentials and grant no access.
	input := strings.Join([]string{
		"openai key sk-EXAMPLEEXAMPLEEXAMPLE",
		"aws key AKIAIOSFODNN7EXAMPLE",
		"github token ghp_EXAMPLEEXAMPLEEXAMPLEEXAMPLE",
		"header Bearer EXAMPLEEXAMPLEEXAMPLE",
		"slack xoxb-EXAMPLEEXAMPLE",
	}, "; ")
	safe := Sanitize(input)
	for _, secret := range []string{"sk-EXAMPLEEXAMPLEEXAMPLE", "AKIAIOSFODNN7EXAMPLE", "ghp_EXAMPLEEXAMPLEEXAMPLEEXAMPLE", "Bearer EXAMPLEEXAMPLEEXAMPLE", "xoxb-EXAMPLEEXAMPLE"} {
		if strings.Contains(safe, secret) {
			t.Errorf("secret %q was not redacted: %s", secret, safe)
		}
	}
	if strings.Count(safe, "[REDACTED]") != 5 {
		t.Errorf("expected five redactions, got %d: %s", strings.Count(safe, "[REDACTED]"), safe)
	}
}

func TestSecretShapeCountIgnoresSyntheticExamples(t *testing.T) {
	// Synthetic shapes only; no real secret is present.
	synthetic := "sk-EXAMPLEEXAMPLEEXAMPLE task-01-invoice-export ghp_EXAMPLEEXAMPLEEXAMPLEEXAMPLE"
	if got := SecretShapeCount(synthetic); got != 0 {
		t.Fatalf("synthetic examples were counted as findings: %d", got)
	}
	// Assembled at runtime so the source fixture stays free of unmarked shapes.
	unmarked := "sk-" + "ABCDEFGHIJKLMNOP"
	if got := SecretShapeCount(unmarked); got != 1 {
		t.Fatalf("credential-shaped string was not counted: %d", got)
	}
}

func TestSanitizeValueRecurses(t *testing.T) {
	value := SanitizeValue(map[string]any{
		"nested": []any{"token sk-EXAMPLEEXAMPLEEXAMPLE", 42},
	})
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("unexpected type %T", value)
	}
	list, ok := object["nested"].([]any)
	if !ok {
		t.Fatalf("unexpected nested type %T", object["nested"])
	}
	if strings.Contains(list[0].(string), "sk-EXAMPLEEXAMPLEEXAMPLE") {
		t.Errorf("nested secret was not redacted: %v", list[0])
	}
	if list[1] != 42 {
		t.Errorf("non-string value changed: %v", list[1])
	}
}

func TestJSONValueRejectsNonJSONValues(t *testing.T) {
	if _, err := JSONValue(math.NaN()); err == nil {
		t.Fatal("expected NaN to be rejected")
	}
}
