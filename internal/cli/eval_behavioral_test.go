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

type evalEnvelope struct {
	Suite                 string           `json:"suite"`
	Executed              bool             `json:"executed"`
	ModelBehaviorExecuted bool             `json:"modelBehaviorExecuted"`
	Mode                  string           `json:"mode"`
	Provider              string           `json:"provider"`
	Status                string           `json:"status"`
	Behavioral            behavioralReport `json:"behavioral"`
	Error                 string           `json:"error"`
}

func runEvalCLI(t *testing.T, root string, args ...string) (int, evalEnvelope) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true, Args: append([]string{"eval"}, args...)}
	code := app.Run(context.Background())
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	var payload evalEnvelope
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("parse eval output (%d): %v: %s %s", code, err, stdout.String(), stderr.String())
	}
	return code, payload
}

func TestBehavioralEvalExecutesDeterministicFakeScenarios(t *testing.T) {
	root := mustRepoRoot(t)
	code, payload := runEvalCLI(t, root, "--suite", "all", "--execute", "--behavioral")
	if code != ExitOK || payload.Status != "passed" || !payload.Executed || payload.ModelBehaviorExecuted {
		t.Fatalf("unexpected behavioral eval result: %d %+v", code, payload)
	}
	if payload.Provider != "fake" || payload.Behavioral.Provider != "fake" {
		t.Fatalf("behavioral eval did not bind the fake provider: %+v", payload)
	}
	if payload.Behavioral.Cases != 15 || payload.Behavioral.Passed != 15 || payload.Behavioral.Failed != 0 {
		t.Fatalf("unexpected behavioral counts: %+v", payload.Behavioral)
	}
	if len(payload.Behavioral.Results) != payload.Behavioral.Cases {
		t.Fatalf("expected one result per case, got %d", len(payload.Behavioral.Results))
	}
	for _, result := range payload.Behavioral.Results {
		if !result.Passed || len(result.EvidenceRecordIDs) != 0 || result.OutputDigest == "" {
			t.Fatalf("unexpected case result: %+v", result)
		}
	}
}

func TestStructuralEvalRemainsUnchangedWithoutBehavioralFlag(t *testing.T) {
	code, payload := runEvalCLI(t, mustRepoRoot(t), "--suite", "all", "--execute")
	if code != ExitOK || payload.Status != "passed" || !payload.Executed || payload.ModelBehaviorExecuted {
		t.Fatalf("unexpected structural eval result: %d %+v", code, payload)
	}
	if payload.Mode != "structural-fixture-checks" || payload.Provider != "" || payload.Behavioral.Cases != 0 {
		t.Fatalf("structural eval changed unexpectedly: %+v", payload)
	}
}

func TestBehavioralEvalFailsClosedOnAuthoredScenarioDrift(t *testing.T) {
	root := t.TempDir()
	copyCanonicalRepo(t, mustRepoRoot(t), root)
	path := filepath.Join(root, "evaluations", "behavioral", "index.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	needle := "expectedFields: [simulated, taskId, skill, routingProfile, requestedModel, result, summary, artifacts]"
	replacement := "expectedFields: [simulated, taskId, skill, routingProfile, requestedModel, result, summary, artifacts, missing-field]"
	if !strings.Contains(string(raw), needle) {
		t.Fatal("behavioral fixture did not contain the expected assertion field list")
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), needle, replacement, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	code, payload := runEvalCLI(t, root, "--suite", "skills", "--behavioral")
	if code != ExitValidation || payload.Error == "" ||
		!strings.Contains(payload.Error, "eval behavioral checks failed: 1 case(s)") {
		t.Fatalf("scenario drift was not reported as one error envelope: %d %+v", code, payload)
	}
	if payload.Behavioral.Cases != 0 || payload.Status != "" {
		t.Fatalf("failed behavioral eval emitted a result payload: %+v", payload)
	}
}
