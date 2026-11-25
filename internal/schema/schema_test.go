package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yoosuf/ae.sh/internal/framework"
)

func TestEmbeddedAssetsMatchCanonical(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, name := range Names {
		canonical, err := os.ReadFile(framework.Dir(root, framework.Schemas, name+".schema.json"))
		if err != nil {
			t.Fatalf("read canonical %s: %v", name, err)
		}
		embedded, err := Asset(name)
		if err != nil {
			t.Fatalf("read embedded %s: %v", name, err)
		}
		if string(canonical) != string(embedded) {
			t.Errorf("%s: embedded schema differs from framework/schemas/%s.schema.json", name, name)
		}
	}
}

func TestValidateRejectsUnknownFieldsAndBadEnums(t *testing.T) {
	if err := Validate("skill", map[string]any{"slug": "x", "unexpected": true}); err == nil {
		t.Fatal("expected skill validation to fail")
	}
	if err := Validate("workflow", map[string]any{
		"schemaVersion": "1.0", "id": "x", "version": "1.0.0", "name": "X",
		"states": []any{"queued"}, "initial": "queued", "terminal": []any{"completed"},
		"tasks": []any{map[string]any{"id": "a", "name": "A", "dependsOn": []any{}, "skill": "x", "type": "unknown"}},
	}); err == nil {
		t.Fatal("expected workflow enum validation to fail")
	}
}

func TestValidateAcceptsIssueFixture(t *testing.T) {
	doc := map[string]any{
		"schemaVersion": "1.0", "key": "TEST-1", "summary": "Local fixture with no credentials",
		"acceptanceCriteria": []any{"criterion one"}, "changedPaths": []any{"main.go"}, "semantics": []any{"business-logic"},
	}
	if err := Validate("issue", doc); err != nil {
		t.Fatalf("valid issue rejected: %v", err)
	}
}
