package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runAudit(t *testing.T, root string) (int, auditReport) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true, Args: []string{"audit"}}
	code := app.Run(context.Background())
	// In --json mode, a failing command emits the report first and then one
	// error envelope. Decode only the first JSON value.
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	var report auditReport
	if err := decoder.Decode(&report); err != nil {
		t.Fatalf("audit output is not JSON (%d): %s %s", code, stdout.String(), stderr.String())
	}
	return code, report
}

func auditCheckNamed(report auditReport, name string) (auditCheck, bool) {
	for _, check := range report.Checks {
		if check.Name == name {
			return check, true
		}
	}
	return auditCheck{}, false
}

func TestAuditOnCanonicalRepoReportsUnpinnedCI(t *testing.T) {
	root := t.TempDir()
	copyCanonicalRepo(t, mustRepoRoot(t), root)
	code, report := runAudit(t, root)
	if code != ExitOK || report.Status != "incomplete" {
		t.Fatalf("expected incomplete audit with exit 0, got %d / %q: %+v", code, report.Status, report)
	}
	pinning, ok := auditCheckNamed(report, "ci-action-pinning")
	if !ok || pinning.Status != "needs-authorization" || len(pinning.Findings) == 0 {
		t.Fatalf("unpinned CI actions were not reported: %+v", pinning)
	}
	for _, name := range []string{"credential-shapes", "embedded-schemas", "canonical-references"} {
		check, ok := auditCheckNamed(report, name)
		if !ok || check.Status != "passed" {
			t.Errorf("expected %s to pass, got %+v", name, check)
		}
	}
	copies, ok := auditCheckNamed(report, "generated-skill-copies")
	if !ok || copies.Status != "skipped" {
		t.Errorf("expected generated copies to be skipped, got %+v", copies)
	}
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestAuditFailsOnCredentialShape(t *testing.T) {
	root := t.TempDir()
	copyCanonicalRepo(t, mustRepoRoot(t), root)
	credential := "sk-" + strings.Repeat("q", 24)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("token "+credential+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, report := runAudit(t, root)
	if code != ExitValidation || report.Status != "failed" {
		t.Fatalf("expected failed audit, got %d / %q", code, report.Status)
	}
	check, ok := auditCheckNamed(report, "credential-shapes")
	if !ok || check.Status != "failed" || len(check.Findings) != 1 || check.Findings[0] != "notes.txt" {
		t.Fatalf("credential shape was not reported: %+v", check)
	}
}

func TestAuditFailsOnEmbeddedSchemaDrift(t *testing.T) {
	root := t.TempDir()
	copyCanonicalRepo(t, mustRepoRoot(t), root)
	schemaPath := filepath.Join(root, "framework", "schemas", "skill.schema.json")
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(schemaPath, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	code, report := runAudit(t, root)
	if code != ExitValidation || report.Status != "failed" {
		t.Fatalf("expected failed audit, got %d / %q", code, report.Status)
	}
	check, ok := auditCheckNamed(report, "embedded-schemas")
	if !ok || check.Status != "failed" || len(check.Findings) != 1 {
		t.Fatalf("schema drift was not reported: %+v", check)
	}
}

func TestAuditFailsOnGeneratedCopyChecksumMismatch(t *testing.T) {
	root := t.TempDir()
	copyCanonicalRepo(t, mustRepoRoot(t), root)
	skillDir := filepath.Join(root, ".claude", "skills", "example")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"provider":"claude","schemaVersion":"1","scope":"project","skills":[{"slug":"example","path":"example/SKILL.md","sha256":"0000000000000000000000000000000000000000000000000000000000000000","version":"0.1.0"}]}`
	if err := os.WriteFile(filepath.Join(root, ".claude", "skills", ".ae-manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	code, report := runAudit(t, root)
	if code != ExitValidation || report.Status != "failed" {
		t.Fatalf("expected failed audit, got %d / %q", code, report.Status)
	}
	check, ok := auditCheckNamed(report, "generated-skill-copies")
	if !ok || check.Status != "failed" || len(check.Findings) != 1 {
		t.Fatalf("checksum mismatch was not reported: %+v", check)
	}
}

func TestAuditPassesWhenActionsAreSHAPinned(t *testing.T) {
	root := t.TempDir()
	copyCanonicalRepo(t, mustRepoRoot(t), root)
	workflow := filepath.Join(root, ".github", "workflows", "ci.yml")
	raw, err := os.ReadFile(workflow)
	if err != nil {
		t.Fatal(err)
	}
	pinned := strings.ReplaceAll(string(raw), "actions/checkout@v4", "actions/checkout@0123456789abcdef0123456789abcdef01234567")
	pinned = strings.ReplaceAll(pinned, "actions/setup-go@v5", "actions/setup-go@abcdef0123456789abcdef0123456789abcdef01")
	if err := os.WriteFile(workflow, []byte(pinned), 0o600); err != nil {
		t.Fatal(err)
	}
	code, report := runAudit(t, root)
	if code != ExitOK || report.Status != "passed" {
		t.Fatalf("expected passing audit, got %d / %q: %+v", code, report.Status, report)
	}
	check, ok := auditCheckNamed(report, "ci-action-pinning")
	if !ok || check.Status != "passed" || check.Findings != nil {
		t.Fatalf("pinned actions were not accepted: %+v", check)
	}
}
