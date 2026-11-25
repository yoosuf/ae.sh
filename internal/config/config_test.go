package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoosuf/ae.sh/internal/framework"
	"github.com/yoosuf/ae.sh/internal/routing"
	"github.com/yoosuf/ae.sh/internal/version"
)

func TestKnownProfilesIncludeRequestedStacks(t *testing.T) {
	for _, name := range []string{"java", "python", "laravel", "typescript", "all"} {
		if !IsKnownProfile(name) {
			t.Errorf("profile %q is not known", name)
		}
	}
	if IsKnownProfile("not-a-canonical-profile") {
		t.Error("unknown profile was accepted")
	}
}

func TestCanonicalProfilesLoadWithExecutableChecks(t *testing.T) {
	root := repoRoot(t)
	for _, name := range KnownProfiles() {
		profile, err := LoadProfile(root, name)
		if err != nil {
			t.Errorf("load profile %s: %v", name, err)
			continue
		}
		if profile.ID != name {
			t.Errorf("profile id %q does not match %q", profile.ID, name)
		}
		if len(profile.Checks) == 0 {
			t.Errorf("profile %s has no checks", name)
		}
		for _, check := range profile.Checks {
			if check.Gate == "" || check.Command == "" {
				t.Errorf("profile %s has an incomplete check: %+v", name, check)
			}
		}
	}
}

func TestAllProfileAggregatesCanonicalChecksWithoutDuplicates(t *testing.T) {
	root := repoRoot(t)
	all, err := LoadProfile(root, "all")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[checkKey]bool{}
	for _, check := range all.Checks {
		key := checkKey{gate: check.Gate, command: check.Command, expectEmptyOutput: check.ExpectEmptyOutput}
		if seen[key] {
			t.Fatalf("all profile duplicates check %q for gate %q", check.Command, check.Gate)
		}
		seen[key] = true
	}
	for _, name := range KnownProfiles() {
		if name == "all" {
			continue
		}
		profile, err := LoadProfile(root, name)
		if err != nil {
			t.Fatal(err)
		}
		for _, check := range profile.Checks {
			key := checkKey{gate: check.Gate, command: check.Command, expectEmptyOutput: check.ExpectEmptyOutput}
			if !seen[key] {
				t.Errorf("all profile is missing %s check %q", name, check.Command)
			}
		}
	}
}

func TestLoadProfileRejectsIncludeCycle(t *testing.T) {
	root := t.TempDir()
	writeTestProfile(t, root, "a", "includes:\n  - b\n")
	writeTestProfile(t, root, "b", "includes:\n  - a\n")
	if _, err := LoadProfile(root, "a"); err == nil {
		t.Fatal("profile include cycle was accepted")
	}
}

func TestDefaultRoutesCoverCanonicalProfiles(t *testing.T) {
	routes := DefaultRoutes()
	if len(routes) != len(routing.Profiles) {
		t.Fatalf("got %d routed profiles, want %d", len(routes), len(routing.Profiles))
	}
	for _, profile := range routing.Profiles {
		for _, provider := range []string{"claude", "codex", "gemini"} {
			if _, err := routing.ModelFor(routes, provider, profile); err != nil {
				t.Errorf("default route %s/%s: %v", profile, provider, err)
			}
		}
	}
}

func TestLoadRejectsInvalidModelRoutes(t *testing.T) {
	cases := []struct {
		name   string
		routes string
	}{
		{name: "unknown-profile", routes: "    cheapest:\n      claude: haiku\n"},
		{name: "credential-shaped-model", routes: "    fast:\n      claude: haiku?token=secret\n"},
		{name: "unsupported-provider", routes: "    fast:\n      gemini-cli: gemini-4-neon\n"},
		{name: "empty-model", routes: "    fast:\n      claude: \"\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			content := fmt.Sprintf("schemaVersion: %q\nprovider:\n  default: fake\n  routes:\n%s", version.SchemaVersion, tc.routes)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("invalid model route was accepted")
			}
		})
	}
}

func TestLoadRejectsUnsafeJiraCloudConfig(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		token   string
	}{
		{"http", "http://example.atlassian.net", "AE_JIRA_TOKEN"},
		{"credentials-in-url", "https://user@example.atlassian.net", "AE_JIRA_TOKEN"},
		{"path-in-url", "https://example.atlassian.net/jira", "AE_JIRA_TOKEN"},
		{"lowercase-env-name", "https://example.atlassian.net", "token"},
		{"missing-env-name", "https://example.atlassian.net", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			content := fmt.Sprintf(`schemaVersion: %q
integrations:
  jira:
    mode: jira-cloud
    baseURL: %q
    tokenEnv: %q
`, version.SchemaVersion, tc.baseURL, tc.token)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("unsafe Jira config was accepted")
			}
		})
	}
}

func TestLoadAcceptsHttpsJiraCloudOrigin(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	content := fmt.Sprintf(`schemaVersion: %q
integrations:
  jira:
    mode: jira-cloud
    baseURL: "https://example.atlassian.net"
    tokenEnv: AE_JIRA_TOKEN
`, version.SchemaVersion)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Integrations.Jira.Mode != "jira-cloud" || cfg.Integrations.Jira.TokenEnv != "AE_JIRA_TOKEN" {
		t.Fatalf("Jira config was not preserved: %+v", cfg.Integrations.Jira)
	}
}

func TestLoadValidatesGitHubLocalCLIMode(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		baseURL string
		token   string
		wantErr bool
	}{
		{name: "default", mode: "local-cli", token: "AE_GITHUB_TOKEN"},
		{name: "disabled", mode: "disabled"},
		{name: "http-base-url", mode: "local-cli", baseURL: "http://github.example.com", token: "AE_GITHUB_TOKEN", wantErr: true},
		{name: "token-value", mode: "local-cli", token: "FAKE-github-token", wantErr: true},
		{name: "unknown-mode", mode: "app", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			content := fmt.Sprintf(`schemaVersion: %q
integrations:
  github:
    mode: %q
    baseURL: %q
    tokenEnv: %q
`, version.SchemaVersion, tc.mode, tc.baseURL, tc.token)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if tc.wantErr {
				if err == nil {
					t.Fatal("invalid GitHub config was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Integrations.GitHub.Mode != tc.mode {
				t.Fatalf("GitHub mode was not preserved: %q", cfg.Integrations.GitHub.Mode)
			}
		})
	}
}

func TestLoadRejectsCredentialValuesInEnvNameFields(t *testing.T) {
	cases := []struct {
		name         string
		integrations string
	}{
		{name: "jira-fixture-webhook", integrations: "  jira:\n    mode: fixture\n    webhookSecretEnv: FAKE-webhook-secret\n"},
		{name: "jira-cloud-webhook", integrations: "  jira:\n    mode: jira-cloud\n    baseURL: https://example.atlassian.net\n    tokenEnv: AE_JIRA_TOKEN\n    webhookSecretEnv: FAKE-webhook-secret\n"},
		{name: "github-disabled-token", integrations: "  github:\n    mode: disabled\n    tokenEnv: FAKE-github-token\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			content := fmt.Sprintf("schemaVersion: %q\nintegrations:\n%s", version.SchemaVersion, tc.integrations)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("credential value in an environment-name field was accepted")
			}
		})
	}
}

func writeTestProfile(t *testing.T, root, id, extra string) {
	t.Helper()
	dir := framework.Dir(root, framework.Profiles, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "schemaVersion: \"" + version.SchemaVersion + "\"\nid: " + id + "\nversion: \"1.0.0\"\n" + extra + "checks: []\nevidenceCommands: []\n"
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
