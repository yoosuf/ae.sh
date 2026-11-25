package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sqliteadapter "github.com/aef-framework/aef/internal/adapters/sqlite"
	"github.com/aef-framework/aef/internal/evidence"
)

func TestReviewResolveWritesHumanDecisionEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := initChatRepo(t, root)
	const runID = "run-review-resolve-1"
	createChatRun(t, cfg, runID)
	diffPath := filepath.Join(root, "candidate.diff")
	diff := "diff --git a/src/app.ts b/src/app.ts\n--- a/src/app.ts\n+++ b/src/app.ts\n@@ -1,1 +1,1 @@\n-const safe = 1;\n+const html = dangerouslySetInnerHTML;\n"
	if err := os.WriteFile(diffPath, []byte(diff), 0o600); err != nil {
		t.Fatal(err)
	}

	var reviewOut bytes.Buffer
	app := App{Stdout: &reviewOut, Stderr: &bytes.Buffer{}, Root: root, JSON: true,
		Args: []string{"review", "--diff", diffPath, "--run", runID}}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("review exited %d: %s", code, reviewOut.String())
	}
	var review ReviewResult
	if err := json.Unmarshal(reviewOut.Bytes(), &review); err != nil {
		t.Fatal(err)
	}
	if len(review.Findings) != 1 {
		t.Fatalf("expected one deterministic finding: %s", reviewOut.String())
	}

	var resolveOut bytes.Buffer
	app = App{Stdout: &resolveOut, Stderr: &bytes.Buffer{}, Root: root, JSON: true,
		Args: []string{"review", "resolve", "--run", runID, "--finding", review.Findings[0].ID,
			"--status", "fixed", "--reason", "removed unsafe rendering after verification", "--actor", "reviewer"}}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("review resolve exited %d: %s", code, resolveOut.String())
	}
	var resolved map[string]any
	if err := json.Unmarshal(resolveOut.Bytes(), &resolved); err != nil {
		t.Fatal(err)
	}
	if resolved["findingId"] != review.Findings[0].ID || resolved["status"] != "fixed" {
		t.Fatalf("unexpected resolution: %v", resolved)
	}

	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records, err := store.Evidence(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected review and remediation evidence, got %d", len(records))
	}
	var remediation evidence.Record
	if err := json.Unmarshal([]byte(records[1].JSON), &remediation); err != nil {
		t.Fatal(err)
	}
	if remediation.Kind != "remediation" || remediation.Actor != "reviewer" || remediation.Data["findingId"] != review.Findings[0].ID {
		t.Fatalf("unexpected remediation record: %+v", remediation)
	}
}

func TestReviewResolveRejectsUnknownFinding(t *testing.T) {
	root := t.TempDir()
	cfg := initChatRepo(t, root)
	createChatRun(t, cfg, "run-review-resolve-2")
	var stdout bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &bytes.Buffer{}, Root: root, JSON: true,
		Args: []string{"review", "resolve", "--run", "run-review-resolve-2", "--finding", "rev-0000000000000000",
			"--status", "accepted", "--reason", "explicit acceptance by owner", "--actor", "reviewer"}}
	if code := app.Run(context.Background()); code != ExitValidation {
		t.Fatalf("unknown finding exited %d, want %d: %s", code, ExitValidation, stdout.String())
	}
}
