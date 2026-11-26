package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqliteadapter "github.com/yoosuf/ae.sh/internal/adapters/sqlite"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/execution"
	"github.com/yoosuf/ae.sh/internal/util"
)

func createDashboardRun(t *testing.T, root, runID string) {
	t.Helper()
	configPath, err := config.Find(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run := execution.Run{
		ID: runID, WorkflowID: "feature-development", WorkflowVersion: "1.0.0",
		IssueKey: "PROJ-101", IssuePath: "fixture", State: "awaiting_approval",
		Status: "awaiting-approval", Risk: "low", Provider: "fake",
		BaseSHA: "base-sha", CandidateSHA: "candidate-sha", PlanDigest: "sha256:plan",
		CurrentTask: 2, Profiles: []string{"go"}, Actor: "tester",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", Version: 1,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
}

func runDashboardCLI(t *testing.T, root string, args ...string) (int, map[string]any) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true, Args: append([]string{"dashboard"}, args...)}
	code := app.Run(context.Background())
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("parse dashboard output (%d): %v: %s %s", code, err, stdout.String(), stderr.String())
	}
	return code, payload
}

func TestDashboardSnapshotIsRunScopedSanitizedAndSelfContained(t *testing.T) {
	root := initEvidenceTestRepo(t)
	runID := "run-dashboard-1"
	createDashboardRun(t, root, runID)
	credential := "sk-" + strings.Repeat("z", 24)
	appendCredentialEvidence(t, root, runID, credential)
	out := filepath.Join(root, "dashboard-out")
	code, payload := runDashboardCLI(t, root, "snapshot", "--run", runID, "--out", out)
	if code != ExitOK || payload["runID"] != runID || payload["evidence"] != float64(1) {
		t.Fatalf("snapshot failed: %d %+v", code, payload)
	}
	for _, name := range []string{"snapshot.json", "index.html", "app.js", "styles.css"} {
		info, err := os.Stat(filepath.Join(out, name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("missing or empty snapshot file %s: %v", name, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(out, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["schemaVersion"] != "1.0" || snapshot["run"].(map[string]any)["id"] != runID {
		t.Fatalf("unexpected snapshot envelope: %+v", snapshot)
	}
	evidenceRows := snapshot["evidence"].([]any)
	data := evidenceRows[0].(map[string]any)["data"].(map[string]any)
	if data["token"] != "[REDACTED]" {
		t.Fatalf("credential-shaped value was not sanitized: %+v", data)
	}
	review := snapshot["review"].(map[string]any)
	if review["note"] == "" || !strings.Contains(review["note"].(string), "untrusted hypotheses") {
		t.Fatalf("review findings were not labeled as hypotheses: %+v", review)
	}
	html := string(mustReadFile(t, filepath.Join(out, "index.html")))
	if strings.Contains(html, "__AE_SNAPSHOT__") || !strings.Contains(html, "ae-snapshot") {
		t.Fatal("generated viewer did not embed a complete snapshot")
	}
	for _, name := range []string{"snapshot.json", "index.html", "app.js", "styles.css"} {
		if count := util.SecretShapeCount(string(mustReadFile(t, filepath.Join(out, name)))); count > 0 {
			t.Errorf("%s contains a credential shape", name)
		}
	}
}

func TestDashboardSnapshotRequiresKnownRunAndOutputDirectory(t *testing.T) {
	root := initEvidenceTestRepo(t)
	code, payload := runDashboardCLI(t, root, "snapshot", "--run", "missing-run", "--out", filepath.Join(root, "out"))
	if code != ExitValidation || payload["exitCode"] != float64(ExitValidation) {
		t.Fatalf("missing run was not rejected: %d %+v", code, payload)
	}
	createDashboardRun(t, root, "run-dashboard-2")
	code, payload = runDashboardCLI(t, root, "snapshot", "--run", "run-dashboard-2")
	if code != ExitUsage || payload["exitCode"] != float64(ExitUsage) {
		t.Fatalf("missing output directory was not rejected: %d %+v", code, payload)
	}
}

func appendCredentialEvidence(t *testing.T, root, runID, credential string) {
	t.Helper()
	configPath, err := config.Find(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqliteadapter.Open(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := appendRunEvidence(context.Background(), cfg, store, runID, "note", "sanitization", "tester",
		map[string]any{"token": credential}, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
