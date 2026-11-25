package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqliteadapter "github.com/aef-framework/aef/internal/adapters/sqlite"
	"github.com/aef-framework/aef/internal/config"
	"github.com/aef-framework/aef/internal/evidence"
	"github.com/aef-framework/aef/internal/execution"
	"github.com/aef-framework/aef/internal/schema"
	"github.com/aef-framework/aef/internal/util"
)

const reviewDiff = `diff --git a/src/api.go b/src/api.go
--- a/src/api.go
+++ b/src/api.go
@@ -10,3 +10,6 @@ func handler() {
  // unchanged
+	query := "SELECT * FROM users WHERE id = " + req.Param("id")
+	token = "sk-FAKEabcdefghijklmnop"
+	client := exec.Command("sh", "-c", "echo "+userInput)
`

func TestScanDiffCreatesValidStructuredFindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "change.diff")
	if err := os.WriteFile(path, []byte(reviewDiff), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := ScanDiff(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ScanDiff(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.DiffDigest != second.DiffDigest {
		t.Fatal("diff digest is not deterministic")
	}
	if len(first.Findings) != 3 {
		t.Fatalf("expected 3 findings, got %d: %+v", len(first.Findings), first.Findings)
	}
	for _, finding := range first.Findings {
		if !strings.HasPrefix(finding.ID, "rev-") || len(finding.ID) != 20 {
			t.Errorf("finding id is not stable-shaped: %q", finding.ID)
		}
		if finding.Status != "hypothesis" || finding.Verified {
			t.Errorf("finding was marked verified: %+v", finding)
		}
		if finding.Line < 1 {
			t.Errorf("finding line is not set: %+v", finding)
		}
	}
	if first.Findings[0].RuleID != "sql-injection" || first.Findings[0].Severity != "critical" {
		t.Errorf("unexpected first finding: %+v", first.Findings[0])
	}
	if strings.Contains(first.Findings[1].Evidence, "sk-FAKEabcdefghijklmnop") {
		t.Error("credential-like literal was not redacted")
	}
	if len(first.ChecksWithoutFindings) != 3 {
		t.Errorf("unexpected checks without findings: %v", first.ChecksWithoutFindings)
	}
	generic, err := util.JSONValue(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate("review", generic); err != nil {
		t.Fatalf("review result failed canonical schema: %v", err)
	}
	if !reflectFindingsEqual(first.Findings, second.Findings) {
		t.Fatal("finding IDs are not stable across scans")
	}
}

func TestReviewEvidencePersistence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	createMinimalFrameworkRoot(t, root)
	var initOut, initErr bytes.Buffer
	initApp := App{Stdout: &initOut, Stderr: &initErr, Root: root, JSON: true,
		Args: []string{"init", "--profile", "go"}}
	if code := initApp.Run(ctx); code != ExitOK {
		t.Fatalf("init exited %d: %s %s", code, initOut.String(), initErr.String())
	}
	cfg, err := config.Load(filepath.Join(root, "aef.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	const runID = "run-review-1"
	if err := store.CreateRun(ctx, execution.Run{
		ID: runID, WorkflowID: "local-flow", WorkflowVersion: "1.0.0",
		IssueKey: "TEST-1", State: "queued", Status: "running", Risk: "low",
		Provider: "fake", CandidateSHA: "candidate123", BaseSHA: "base123",
		Actor: "tester", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", Version: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	diffPath := filepath.Join(root, "change.diff")
	if err := os.WriteFile(diffPath, []byte(reviewDiff), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"review", "--diff", diffPath, "--run", runID}}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("review exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	var result ReviewResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode review result: %v", err)
	}
	if result.EvidenceRecordID == "" {
		t.Fatal("review result has no evidence record id")
	}
	if result.CandidateSHA != "candidate123" || result.BaseSHA != "base123" {
		t.Errorf("run SHAs were not propagated: %+v", result)
	}

	store, err = sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records, err := store.Evidence(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one review evidence record, got %d", len(records))
	}
	var record evidence.Record
	if err := json.Unmarshal([]byte(records[0].JSON), &record); err != nil {
		t.Fatal(err)
	}
	if record.Kind != "review" || record.RecordID != result.EvidenceRecordID {
		t.Fatalf("unexpected evidence record: %+v", record)
	}
	findings, ok := record.Data["findings"].([]any)
	if !ok || len(findings) != 3 {
		t.Errorf("evidence findings were not persisted: %+v", record.Data)
	}
	if _, exists := record.Data["evidenceRecordID"]; exists {
		t.Error("evidence data contains a self-referential record id")
	}
	if _, err := os.Stat(filepath.Join(cfg.EvidenceDir(), runID, "0000-review.json")); err != nil {
		t.Errorf("review evidence file was not written: %v", err)
	}
}

func reflectFindingsEqual(a, b []ReviewFinding) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
