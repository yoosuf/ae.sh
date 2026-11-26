package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoosuf/ae.sh/internal/config"
)

func githubStub(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	script := filepath.Join(dir, "gh-stub")
	body = strings.ReplaceAll(body, "$AE_TEST_ARGS", argsPath)
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s ' \"$@\" > \"$AE_TEST_ARGS\"\nprintf '%s\\n' \"$AE_TEST_STUB_OUT\"\nif [ -n \"$GH_TOKEN\" ]; then printf 'token-present\\n' >> \"$AE_TEST_ARGS\"; fi\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AE_TEST_ARGS", argsPath)
	t.Setenv("AE_TEST_STUB_OUT", body)
	return script, argsPath
}

func TestNewRejectsGitHubAppExtensionPoint(t *testing.T) {
	_, err := New(config.Integration{
		Mode:                 "github-app",
		BaseURL:              "https://github.example.com",
		AppIDEnv:             "AE_GITHUB_APP_ID",
		InstallationTokenEnv: "AE_GITHUB_INSTALLATION_TOKEN",
		Actor:                "ae-bot",
	})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("GitHub App identity did not fail closed: %v", err)
	}
}

func TestViewPRChecksAndDiffUseReadOnlyArguments(t *testing.T) {
	script, argsPath := githubStub(t, `{"number":12,"title":"Add [REDACTED] feature","state":"OPEN","headRefOid":"abc1234","baseRefName":"main","reviewDecision":"CHANGES_REQUESTED","reviews":[{"author":{"login":"reviewer"},"state":"CHANGES_REQUESTED","body":"please fix"}],"comments":[{"author":{"login":"user"},"body":"note","createdAt":"2026-01-01T00:00:00Z"}]}`)
	source := &Source{Binary: script}
	pr, err := source.ViewPR(context.Background(), "12")
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 12 || pr.HeadSHA != "abc1234" || len(pr.Reviews) != 1 {
		t.Fatalf("PR was not parsed: %+v", pr)
	}
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	if !strings.Contains(args, "pr view 12 --json") {
		t.Fatalf("unexpected gh arguments: %q", args)
	}
	if strings.Contains(args, "merge") || strings.Contains(args, "repo create") {
		t.Fatalf("read operation used mutating arguments: %q", args)
	}
}

func TestChecksAndDiff(t *testing.T) {
	script, argsPath := githubStub(t, `[{"name":"build","state":"COMPLETED","conclusion":"SUCCESS"}]`)
	source := &Source{Binary: script}
	checks, err := source.Checks(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].Conclusion != "SUCCESS" {
		t.Fatalf("checks were not parsed: %+v", checks)
	}
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "pr checks main --json") {
		t.Fatalf("unexpected checks arguments: %q", raw)
	}
	t.Setenv("AE_TEST_STUB_OUT", "diff --git a/x b/x")
	diff, err := source.Diff(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(diff), "diff --git") {
		t.Fatalf("diff was not returned: %q", diff)
	}
}

func TestCommentRequiresMutationAndKeepsTokenOutOfArguments(t *testing.T) {
	script, argsPath := githubStub(t, `{}`)
	source := &Source{Binary: script, TokenEnv: "AE_TEST_GH_TOKEN"}
	t.Setenv("AE_TEST_GH_TOKEN", "FAKE-gh-token")
	if err := source.Comment(context.Background(), "12", "summary"); err == nil {
		t.Fatal("comment was accepted without mutation authorization")
	}
	source.AllowMutations = true
	if err := source.Comment(context.Background(), "12", "run complete"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	if strings.Contains(args, "FAKE-gh-token") {
		t.Fatalf("token leaked into arguments: %q", args)
	}
	if !strings.Contains(args, "token-present") {
		t.Fatalf("environment token was not supplied to the child: %q", args)
	}
	if !strings.Contains(args, "pr comment 12 --body run complete") {
		t.Fatalf("unexpected comment arguments: %q", args)
	}
}

func TestNewRejectsNonLocalCLIMode(t *testing.T) {
	integration := config.Integration{Mode: "app", TokenEnv: "AE_GITHUB_TOKEN"}
	if _, err := New(integration); err == nil {
		t.Fatal("GitHub App mode was accepted by the local CLI adapter")
	}
}
