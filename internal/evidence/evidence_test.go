package evidence_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoosuf/ae.sh/internal/evidence"
)

func TestBuildAcceptsDocumentedEvidenceKinds(t *testing.T) {
	manager := evidence.New(t.TempDir(), t.TempDir())
	kinds := []string{
		"input", "plan", "risk", "approval", "provider", "github", "test",
		"review", "remediation", "gate", "artifact", "transition", "merge",
		"deployment", "note",
	}
	for _, kind := range kinds {
		if _, err := manager.Build(context.Background(), "run-evidence-kind", 0, kind,
			"subject", "tester", map[string]any{"value": "ok"}, nil, nil, ""); err != nil {
			t.Errorf("schema rejected evidence kind %q: %v", kind, err)
		}
	}
	if _, err := manager.Build(context.Background(), "run-evidence-kind", 0, "unknown-kind",
		"subject", "tester", map[string]any{"value": "ok"}, nil, nil, ""); err == nil {
		t.Error("schema accepted an unknown evidence kind")
	}
}

func TestWriteSanitizesIdentityFields(t *testing.T) {
	root := t.TempDir()
	recordsDir := filepath.Join(root, "records")
	manager := evidence.New(recordsDir, filepath.Join(root, "objects"))
	secret := "sk-FAKEabcdefghijklmnop"
	record, err := manager.Write(context.Background(), "run-evidence-1", 0, "note",
		"subject "+secret, secret, map[string]any{"value": "ok"}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(record.Actor, secret) || strings.Contains(record.Subject, secret) {
		t.Fatalf("identity fields were not sanitized: %+v", record)
	}
	raw, err := os.ReadFile(filepath.Join(recordsDir, "run-evidence-1", "0000-note.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("persisted evidence contains a credential-shaped value: %s", raw)
	}
}
