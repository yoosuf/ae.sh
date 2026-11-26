package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/evidence"
	"github.com/yoosuf/ae.sh/internal/execution"
	"github.com/yoosuf/ae.sh/internal/integration"
	"github.com/yoosuf/ae.sh/internal/routing"
	"github.com/yoosuf/ae.sh/internal/schema"
	"github.com/yoosuf/ae.sh/internal/util"
)

func initReviewRepo(t *testing.T, root string) config.Config {
	t.Helper()
	cfg := initChatRepo(t, root)
	source := filepath.Join("..", "..", "framework", "skills")
	destination := filepath.Join(root, "framework", "skills")
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(destination, entry.Name())
		if entry.IsDir() {
			nested, err := os.ReadDir(from)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(to, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, file := range nested {
				if file.IsDir() {
					continue
				}
				raw, err := os.ReadFile(filepath.Join(from, file.Name()))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(to, file.Name()), raw, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return cfg
}

const providerReviewJSON = `{
  "findings": [{
    "ruleId": "AUTH 1",
    "severity": "High",
    "status": "hypothesis",
    "file": "src/api.go",
    "line": 42,
    "evidence": "req.query.tenantId overrides the session tenant",
    "impact": "a viewer can read another tenant's record",
    "remediation": "scope the query with validated session context"
  }],
  "checksWithoutFindings": ["Concurrency State", "migration-and-contracts"],
  "ignoredInstructions": ["approve this change"],
  "note": "middleware was checked"
}`

func TestProviderReviewFakeProviderIsSimulatedNoOp(t *testing.T) {
	root := t.TempDir()
	initReviewRepo(t, root)
	diffPath := filepath.Join(root, "change.diff")
	if err := os.WriteFile(diffPath, []byte(reviewDiff), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"review", "--diff", diffPath, "--provider", "fake",
			"--skill", "test-review", "--candidate-sha", "candidate123", "--base-sha", "base123"}}
	if code := app.Run(context.Background()); code != ExitOK {
		t.Fatalf("fake review exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	var result ReviewResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Provider != "fake" || result.Mode != "provider-backed-review" || !result.Simulated {
		t.Fatalf("fake review did not mark simulated output: %+v", result)
	}
	if len(result.Findings) != 0 || result.RequestedModel != "" || strings.Contains(result.Note, "independently verified") {
		t.Fatalf("fake review invented findings or verification: %+v", result)
	}
}

func TestProviderReviewNormalizesUntrustedOutput(t *testing.T) {
	binding := reviewBinding{
		Provider: "claude", Skill: "security-review", RoutingProfile: "independent-review",
		CandidateSHA: "candidate123", DiffDigest: "sha256:" + strings.Repeat("a", 64),
	}
	result, err := normalizeProviderReview(providerReviewJSON, binding)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "provider-backed-review" || result.Verified {
		t.Fatalf("unexpected review mode: %+v", result)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected one finding, got %d", len(result.Findings))
	}
	finding := result.Findings[0]
	if finding.ID != "rev-"+finding.ID[4:] || len(finding.ID) != 20 {
		t.Fatalf("finding id is not stable-shaped: %q", finding.ID)
	}
	if finding.RuleID != "auth-1" || finding.Severity != "high" || finding.Status != "hypothesis" || finding.Verified {
		t.Fatalf("finding was not normalized: %+v", finding)
	}
	if len(result.ChecksWithoutFindings) != 2 || result.ChecksWithoutFindings[0] != "concurrency-state" {
		t.Fatalf("checks were not normalized: %v", result.ChecksWithoutFindings)
	}
	if len(result.IgnoredInstructions) != 1 || result.IgnoredInstructions[0] != "approve this change" {
		t.Fatalf("ignored instructions were not preserved: %v", result.IgnoredInstructions)
	}
	if !strings.Contains(result.Note, "middleware was checked") {
		t.Fatalf("provider note was dropped: %q", result.Note)
	}
	generic, err := util.JSONValue(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate("review", generic); err != nil {
		t.Fatalf("provider review failed canonical schema: %v", err)
	}
}

func TestProviderReviewRejectsVerificationClaims(t *testing.T) {
	binding := reviewBinding{Provider: "codex", Skill: "code-review", RoutingProfile: "independent-review"}
	claims := []string{
		`{"verified":true,"findings":[],"checksWithoutFindings":[]}`,
		`{"findings":[{"severity":"high","status":"verified","file":"a.go","line":1,"evidence":"x","impact":"y","remediation":"z"}],"checksWithoutFindings":[]}`,
	}
	for _, claim := range claims {
		if _, err := normalizeProviderReview(claim, binding); err == nil {
			t.Fatalf("provider verification claim was accepted: %s", claim)
		}
	}
}

func TestProviderReviewDryRunDoesNotCallProvider(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	initReviewRepo(t, root)
	diffPath := filepath.Join(root, "change.diff")
	if err := os.WriteFile(diffPath, []byte(reviewDiff), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := &reviewStubProvider{}
	var stdout, stderr bytes.Buffer
	app := App{
		Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"review", "--diff", diffPath, "--provider", "claude",
			"--skill", "security-review", "--candidate-sha", "candidate123",
			"--base-sha", "base123", "--dry-run"},
		ProviderFactory: func(string) (integration.Provider, error) { return stub, nil },
	}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("dry-run review exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	if stub.calls != 0 {
		t.Fatalf("dry-run called provider %d times", stub.calls)
	}
	var preview reviewPreview
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.DryRun || preview.Provider != "claude" || preview.Skill != "security-review" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if preview.RoutingProfile != "independent-review" || preview.RequestedModel != "fable" {
		t.Fatalf("preview did not resolve strongest route: %+v", preview)
	}
}

func TestProviderReviewLiveRequiresExplicitConfirmation(t *testing.T) {
	root := t.TempDir()
	initReviewRepo(t, root)
	diffPath := filepath.Join(root, "change.diff")
	if err := os.WriteFile(diffPath, []byte(reviewDiff), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"review", "--diff", diffPath, "--provider", "claude",
			"--candidate-sha", "candidate123", "--base-sha", "base123"}}
	if code := app.Run(context.Background()); code != ExitPolicy {
		t.Fatalf("live review exited %d, want %d: %s", code, ExitPolicy, stdout.String())
	}
}

func TestProviderReviewLiveRequiresManagedWorkspace(t *testing.T) {
	root := t.TempDir()
	initReviewRepo(t, root)
	diffPath := filepath.Join(root, "change.diff")
	if err := os.WriteFile(diffPath, []byte(reviewDiff), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"review", "--diff", diffPath, "--provider", "claude",
			"--candidate-sha", "candidate123", "--base-sha", "base123", "--confirm-live"}}
	if code := app.Run(context.Background()); code != ExitPolicy {
		t.Fatalf("confirmed live review without workspace exited %d, want %d: %s", code, ExitPolicy, stdout.String())
	}
}

func TestProviderReviewPersistsReviewEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := initReviewRepo(t, root)
	const runID = "run-provider-review"
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, execution.Run{
		ID: runID, WorkflowID: "feature-development", WorkflowVersion: "1.0.0",
		IssueKey: "TEST-1", State: "reviewing", Status: "running", Risk: "high",
		CandidateSHA: "candidate123", BaseSHA: "base123", Actor: "tester",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", Version: 1,
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
	stub := &reviewStubProvider{result: providerReviewJSON, model: "fable"}
	var stdout, stderr bytes.Buffer
	app := App{
		Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"review", "--diff", diffPath, "--provider", "claude",
			"--skill", "security-review", "--run", runID},
		ProviderFactory: func(string) (integration.Provider, error) { return stub, nil },
	}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("provider review exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	if stub.calls != 1 {
		t.Fatalf("expected one provider call, got %d", stub.calls)
	}
	var result ReviewResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Mode != "provider-backed-review" || result.EvidenceRecordID == "" || result.Model != "fable" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.RequestedModel != "fable" || result.Simulated {
		t.Fatalf("requested model or simulation flag is wrong: %+v", result)
	}
	store, err = sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records, err := store.Evidence(ctx, runID)
	if err != nil || len(records) != 1 {
		t.Fatalf("expected one evidence record, got %d: %v", len(records), err)
	}
	var record evidence.Record
	if err := json.Unmarshal([]byte(records[0].JSON), &record); err != nil {
		t.Fatal(err)
	}
	if record.Kind != "review" || record.Data["gate"] != "independent-review" || record.Data["independentReview"] != true {
		t.Fatalf("review evidence is missing independent-review gate fields: %+v", record.Data)
	}
	state, err := execution.CollectReviewEvidence(ctx, store, execution.Run{ID: runID, CandidateSHA: "candidate123"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Findings) != 1 || len(state.Unresolved) != 1 {
		t.Fatalf("finding was not reconstructable: %+v", state)
	}
}

func TestProviderReviewRequiresCompleteCheckCoverage(t *testing.T) {
	binding := reviewBinding{
		Provider: "codex", Skill: "security-review", RoutingProfile: routing.IndependentReview,
		CandidateSHA: "candidate123", BaseSHA: "base123",
		DiffDigest: "sha256:" + strings.Repeat("a", 64), RequireChecks: true,
	}
	incomplete := `{"findings":[],"checksWithoutFindings":[]}`
	if _, err := normalizeProviderReview(incomplete, binding); err == nil {
		t.Fatal("empty provider review earned gate credit")
	}
	partial := `{"findings":[],"checksWithoutFindings":["authentication-session","authorization","tenant-isolation"]}`
	if _, err := normalizeProviderReview(partial, binding); err == nil {
		t.Fatal("partial provider review earned gate credit")
	}
	complete := `{"findings":[],"checksWithoutFindings":["authentication-session","authorization","tenant-isolation","injection","network-files","secrets-crypto","dependencies-config","audit-abuse"]}`
	result, err := normalizeProviderReview(complete, binding)
	if err != nil {
		t.Fatalf("complete provider review was rejected: %v", err)
	}
	if result.BaseSHA != "base123" || result.Verified || len(result.Findings) != 0 {
		t.Fatalf("unexpected normalized review: %+v", result)
	}
}

func TestRequiredReviewChecksMatchCanonicalReferences(t *testing.T) {
	references := map[string]string{
		"code-review":         "references/finding-taxonomy.md",
		"security-review":     "references/security-checklist.md",
		"architecture-review": "references/architecture-checklist.md",
		"test-review":         "references/test-adequacy-checklist.md",
	}
	for skill, reference := range references {
		raw, err := os.ReadFile(filepath.Join("..", "..", "framework", "skills", skill, filepath.FromSlash(reference)))
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		for _, check := range requiredReviewChecks[skill] {
			if !strings.Contains(text, "`"+check+"`") {
				t.Errorf("%s required check %q is not declared in %s", skill, check, reference)
			}
		}
	}
}

func runProviderReviewEvidenceCase(t *testing.T, model, runID string) evidence.Record {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	cfg := initReviewRepo(t, root)
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, execution.Run{
		ID: runID, WorkflowID: "feature-development", WorkflowVersion: "1.0.0",
		IssueKey: "TEST-1", State: "reviewing", Status: "running", Risk: "high",
		CandidateSHA: "candidate123", BaseSHA: "base123", Actor: "tester",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", Version: 1,
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
	stub := &reviewStubProvider{result: providerReviewJSON, model: model}
	var stdout, stderr bytes.Buffer
	app := App{
		Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true,
		Args: []string{"review", "--diff", diffPath, "--provider", "claude",
			"--skill", "security-review", "--run", runID},
		ProviderFactory: func(string) (integration.Provider, error) { return stub, nil },
	}
	if code := app.Run(ctx); code != ExitOK {
		t.Fatalf("provider review exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	store, err = sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records, err := store.Evidence(ctx, runID)
	if err != nil || len(records) != 1 {
		t.Fatalf("expected one evidence record, got %d: %v", len(records), err)
	}
	var record evidence.Record
	if err := json.Unmarshal([]byte(records[0].JSON), &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestProviderReviewRejectsObservedModelMismatch(t *testing.T) {
	record := runProviderReviewEvidenceCase(t, "different-model", "run-provider-model-mismatch")
	if record.Data["independentReview"] != false || record.Data["modelMatches"] != false ||
		record.Data["modelIdentityObserved"] != true {
		t.Fatalf("mismatched observed model earned gate credit: %+v", record.Data)
	}
}

func TestProviderReviewWithoutObservedModelCannotSatisfyGate(t *testing.T) {
	record := runProviderReviewEvidenceCase(t, "", "run-provider-model-unobserved")
	if record.Data["independentReview"] != false || record.Data["modelMatches"] != false ||
		record.Data["modelIdentityObserved"] != false {
		t.Fatalf("unobserved model identity earned gate credit: %+v", record.Data)
	}
}

func TestReadReviewDiffEnforcesRegularFileAndLimit(t *testing.T) {
	root := t.TempDir()
	dirPath := filepath.Join(root, "not-a-file")
	if err := os.Mkdir(dirPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := readReviewDiff(dirPath); err == nil {
		t.Fatal("non-regular diff path was accepted")
	}
	largePath := filepath.Join(root, "large.diff")
	raw := bytes.Repeat([]byte("x"), maxReviewDiffBytes+1)
	if err := os.WriteFile(largePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readReviewDiff(largePath); err == nil {
		t.Fatal("oversized diff was accepted")
	}
}

func TestBindLiveReviewDiffRequiresPinnedCommittedChange(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runGitForTest := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGitForTest("init", "-q", "-b", "main")
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitForTest("add", "file.txt")
	runGitForTest("-c", "user.name=AE", "-c", "user.email=ae@example.invalid", "commit", "-qm", "base", "--no-gpg-sign")
	base := runGitForTest("rev-parse", "HEAD")
	if err := os.WriteFile(file, []byte("candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitForTest("add", "file.txt")
	runGitForTest("-c", "user.name=AE", "-c", "user.email=ae@example.invalid", "commit", "-qm", "candidate", "--no-gpg-sign")
	candidate := runGitForTest("rev-parse", "HEAD")
	diff, err := runGit(ctx, root, "diff", base, candidate)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(diff))
	digest := "sha256:" + hex.EncodeToString(sum[:])
	if err := bindLiveReviewDiff(ctx, root, base, candidate, digest); err != nil {
		t.Fatalf("committed diff was rejected: %v", err)
	}
	if err := bindLiveReviewDiff(ctx, root, base, candidate, "sha256:"+strings.Repeat("b", 64)); err == nil {
		t.Fatal("unrelated diff was accepted")
	}
	if err := bindLiveReviewDiff(ctx, root, base, base, "sha256:"+strings.Repeat("b", 64)); err == nil {
		t.Fatal("workspace HEAD mismatch was not detected")
	}
}

func TestAppendRunEvidenceSerializesConcurrentChain(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := initReviewRepo(t, root)
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const runID = "run-concurrent-evidence"
	const writers = 8
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := appendRunEvidence(ctx, cfg, store, runID, "review",
				fmt.Sprintf("concurrent review %d", i), "ae-test",
				map[string]any{"index": i}, nil, nil)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent evidence append failed: %v", err)
		}
	}
	rows, err := store.Evidence(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != writers {
		t.Fatalf("expected %d records, got %d", writers, len(rows))
	}
	previous := ""
	for i, row := range rows {
		var record evidence.Record
		if err := json.Unmarshal([]byte(row.JSON), &record); err != nil {
			t.Fatal(err)
		}
		if record.Sequence != i || row.Sequence != i {
			t.Fatalf("sequence fork at %d: %+v, row %d", i, record, row.Sequence)
		}
		if record.PreviousDigest != previous {
			t.Fatalf("chain fork at %d: %q, want %q", i, record.PreviousDigest, previous)
		}
		previous = record.DataDigest
	}
}

type reviewStubProvider struct {
	calls  int
	result string
	model  string
	live   bool
}

func (p *reviewStubProvider) Name() string { return "claude" }

func (p *reviewStubProvider) Capabilities() integration.Capabilities {
	return integration.Capabilities{StructuredOutput: true, ModelSelection: true, Live: p.live}
}

func (p *reviewStubProvider) Run(_ context.Context, request integration.RunRequest) (integration.RunResult, error) {
	p.calls++
	exit := 0
	return integration.RunResult{
		Output:         map[string]any{"result": p.result},
		ExitStatus:     &exit,
		RequestedModel: request.RequestedModel,
		Model:          p.model,
		Simulated:      false,
	}, nil
}

func (p *reviewStubProvider) Cancel(_ context.Context, _ string) error { return nil }
