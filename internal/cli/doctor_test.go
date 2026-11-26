package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/yoosuf/ae.sh/internal/config"
)

func TestDoctorReportsIdentityAndReviewExtensionPoints(t *testing.T) {
	root := t.TempDir()
	copyCanonicalRepo(t, mustRepoRoot(t), root)
	if err := config.Write(filepath.Join(root, config.FileName), config.Default(), false); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, Root: root, JSON: true, Args: []string{"doctor"}}
	if code := app.Run(context.Background()); code != ExitOK {
		t.Fatalf("doctor exited %d: %s %s", code, stdout.String(), stderr.String())
	}
	var report struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor output is not JSON: %v: %s", err, stdout.String())
	}
	want := map[string]string{
		"identity/federated":          "approvals remain local-manual",
		"review/external-attestation": "provider text remains untrusted",
	}
	seen := map[string]bool{}
	for _, check := range report.Checks {
		if expected, ok := want[check.Name]; ok {
			if check.Status != "extension-point" {
				t.Errorf("%s status = %q, want extension-point", check.Name, check.Status)
			}
			if !bytes.Contains([]byte(check.Detail), []byte(expected)) {
				t.Errorf("%s detail = %q, want %q", check.Name, check.Detail, expected)
			}
			seen[check.Name] = true
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("doctor did not report %s", name)
		}
	}
}
