// Package config loads the repository-local AE configuration. Configuration
// never contains credentials; environment variable *names* only.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yoosuf/ae.sh/internal/framework"
	"github.com/yoosuf/ae.sh/internal/routing"
	"github.com/yoosuf/ae.sh/internal/util"
	"github.com/yoosuf/ae.sh/internal/version"
)

const (
	FileName        = "ae.yaml"
	DefaultState    = ".ae/state.db"
	DefaultEvidence = ".ae/evidence"
	DefaultObjects  = ".ae/objects"
)

// Config is the on-disk configuration.
type Config struct {
	SchemaVersion string       `yaml:"schemaVersion" json:"schemaVersion"`
	Profiles      []string     `yaml:"profiles" json:"profiles"`
	State         State        `yaml:"state" json:"state"`
	Evidence      Evidence     `yaml:"evidence" json:"evidence"`
	Provider      Provider     `yaml:"provider" json:"provider"`
	Deployment    Deployment   `yaml:"deployment" json:"deployment"`
	Integrations  Integrations `yaml:"integrations" json:"integrations"`
	Root          string       `yaml:"-" json:"-"`
}

type State struct {
	Path string `yaml:"path" json:"path"`
}

type Evidence struct {
	Dir        string `yaml:"dir" json:"dir"`
	ObjectsDir string `yaml:"objectsDir" json:"objectsDir"`
}

// Provider selects the default live provider and maps canonical routing
// profiles to provider model IDs. Model IDs are local configuration, never
// canonical skill content and never credentials.
type Provider struct {
	Default string                       `yaml:"default" json:"default"`
	Routes  map[string]map[string]string `yaml:"routes,omitempty" json:"routes,omitempty"`
}

// DefaultRoutes are verified recommendations as of 2026-11-25. Aliases are
// preferred where a provider supports them; the exact IDs must be rechecked
// against the installed CLI before a live run. See docs/model-routing.md.
func DefaultRoutes() map[string]map[string]string {
	return map[string]map[string]string{
		routing.Fast: {
			"claude": "haiku",
			"codex":  "gpt-6-luna",
			"gemini": "gemini-3.5-flash-lite",
		},
		routing.Balanced: {
			"claude": "sonnet",
			"codex":  "gpt-6.2-sol",
			"gemini": "gemini-4-hydrogen",
		},
		routing.DeepReasoning: {
			"claude": "opus",
			"codex":  "gpt-6.2-astra",
			"gemini": "gemini-4-neon",
		},
		routing.IndependentReview: {
			"claude": "fable",
			"codex":  "gpt-6.2-astra",
			"gemini": "gemini-4-carbon",
		},
		routing.Recovery: {
			"claude": "opus",
			"codex":  "gpt-6.2-astra",
			"gemini": "gemini-4-carbon",
		},
	}
}

type Deployment struct {
	// local stops after merge-eligible PR evidence; optional requires an
	// explicit release authorization; required is reserved for cloud mode.
	Scope string `yaml:"scope" json:"scope"`
}

type Integrations struct {
	Jira     Integration         `yaml:"jira" json:"jira"`
	GitHub   Integration         `yaml:"github" json:"github"`
	Identity IdentityIntegration `yaml:"identity,omitempty" json:"identity,omitempty"`
	Review   ReviewIntegration   `yaml:"review,omitempty" json:"review,omitempty"`
}

// IdentityIntegration reserves the federated approver identity boundary. The
// federated mode is validated but no verifier exists; local approvals remain
// local-manual and cannot satisfy organization identity checks.
type IdentityIntegration struct {
	Mode       string `yaml:"mode" json:"mode"`
	Issuer     string `yaml:"issuer,omitempty" json:"issuer,omitempty"`
	Audience   string `yaml:"audience,omitempty" json:"audience,omitempty"`
	TokenEnv   string `yaml:"tokenEnv,omitempty" json:"tokenEnv,omitempty"`
	ActorClaim string `yaml:"actorClaim,omitempty" json:"actorClaim,omitempty"`
}

// ReviewIntegration reserves the externally verified review boundary. The
// external-attestation mode is validated but no independent verifier exists;
// provider text alone never becomes verified review evidence.
type ReviewIntegration struct {
	Mode        string `yaml:"mode" json:"mode"`
	Issuer      string `yaml:"issuer,omitempty" json:"issuer,omitempty"`
	VerifierEnv string `yaml:"verifierEnv,omitempty" json:"verifierEnv,omitempty"`
}

type Integration struct {
	Mode             string `yaml:"mode" json:"mode"`
	BaseURL          string `yaml:"baseURL,omitempty" json:"baseURL,omitempty"`
	TokenEnv         string `yaml:"tokenEnv,omitempty" json:"tokenEnv,omitempty"`
	WebhookSecretEnv string `yaml:"webhookSecretEnv,omitempty" json:"webhookSecretEnv,omitempty"`
	// GitHub App/service identity extension point. These fields are validated
	// locally but no adapter uses them yet; github-app mode fails closed.
	AppIDEnv             string `yaml:"appIDEnv,omitempty" json:"appIDEnv,omitempty"`
	InstallationTokenEnv string `yaml:"installationTokenEnv,omitempty" json:"installationTokenEnv,omitempty"`
	Actor                string `yaml:"actor,omitempty" json:"actor,omitempty"`
}

// Check is a local verification command from a canonical profile. Commands
// come from checked-in profile files, never from issue text or CLI flags.
type Check struct {
	Gate              string `yaml:"gate" json:"gate"`
	Command           string `yaml:"command" json:"command"`
	ExpectEmptyOutput bool   `yaml:"expectEmptyOutput,omitempty" json:"expectEmptyOutput,omitempty"`
}

// Profile is the canonical local profile definition used by `ae verify`.
// Includes allow composite profiles such as `all` to reuse canonical checks
// without duplicating executable commands in multiple files.
type Profile struct {
	SchemaVersion    string   `yaml:"schemaVersion" json:"schemaVersion"`
	ID               string   `yaml:"id" json:"id"`
	Version          string   `yaml:"version" json:"version"`
	Includes         []string `yaml:"includes,omitempty" json:"includes,omitempty"`
	Checks           []Check  `yaml:"checks" json:"checks"`
	EvidenceCommands []string `yaml:"evidenceCommands" json:"evidenceCommands"`
}

// KnownProfiles returns the canonical profile names in documented order.
func KnownProfiles() []string {
	return append([]string(nil), profileNames...)
}

// IsKnownProfile reports whether name can be selected by `ae init` or
// `ae verify`. The file must still load successfully before use.
func IsKnownProfile(name string) bool {
	for _, known := range profileNames {
		if name == known {
			return true
		}
	}
	return false
}

var profileNames = []string{
	"go", "java", "python", "dotnet", "nodejs", "typescript",
	"react", "vue", "astro", "laravel", "aws", "all",
}

// Default returns a new local-first configuration.
func Default() Config {
	return Config{
		SchemaVersion: version.SchemaVersion,
		Profiles:      []string{},
		State:         State{Path: DefaultState},
		Evidence:      Evidence{Dir: DefaultEvidence, ObjectsDir: DefaultObjects},
		Provider:      Provider{Default: "fake", Routes: DefaultRoutes()},
		Deployment:    Deployment{Scope: "local"},
		Integrations: Integrations{
			Jira:     Integration{Mode: "fixture", TokenEnv: "AE_JIRA_TOKEN", WebhookSecretEnv: "AE_JIRA_WEBHOOK_SECRET"},
			GitHub:   Integration{Mode: "local-cli", TokenEnv: "AE_GITHUB_TOKEN", WebhookSecretEnv: "AE_GITHUB_WEBHOOK_SECRET"},
			Identity: IdentityIntegration{Mode: "local-manual"},
			Review:   ReviewIntegration{Mode: "local"},
		},
	}
}

// Find walks upward from dir looking for ae.yaml.
func Find(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(abs, FileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("%s not found (run `ae init` first)", FileName)
		}
		abs = parent
	}
}

// Load reads and validates the configuration file at path.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.Root = filepath.Dir(path)
	if cfg.SchemaVersion != version.SchemaVersion {
		return Config{}, fmt.Errorf("unsupported config schemaVersion %q (want %q)", cfg.SchemaVersion, version.SchemaVersion)
	}
	if cfg.State.Path == "" {
		cfg.State.Path = DefaultState
	}
	if cfg.Evidence.Dir == "" {
		cfg.Evidence.Dir = DefaultEvidence
	}
	if cfg.Evidence.ObjectsDir == "" {
		cfg.Evidence.ObjectsDir = DefaultObjects
	}
	for _, entry := range []struct{ field, value string }{
		{"state.path", cfg.State.Path},
		{"evidence.dir", cfg.Evidence.Dir},
		{"evidence.objectsDir", cfg.Evidence.ObjectsDir},
	} {
		if err := validateContainedPath(entry.field, entry.value, cfg.Root); err != nil {
			return Config{}, err
		}
	}
	if cfg.Provider.Default == "" {
		cfg.Provider.Default = "fake"
	}
	switch cfg.Provider.Default {
	case "fake", "claude", "codex", "gemini":
	default:
		return Config{}, fmt.Errorf("unsupported provider %q", cfg.Provider.Default)
	}
	cfg.Provider.Routes = mergeRoutes(DefaultRoutes(), cfg.Provider.Routes)
	if err := routing.ValidateRoutes(cfg.Provider.Routes); err != nil {
		return Config{}, fmt.Errorf("provider.routes: %w", err)
	}
	switch cfg.Deployment.Scope {
	case "", "local":
		cfg.Deployment.Scope = "local"
	case "optional", "required":
	default:
		return Config{}, fmt.Errorf("unsupported deployment.scope %q", cfg.Deployment.Scope)
	}
	// Environment values must always be names, never credentials, even when
	// an integration mode is disabled or fixture-only.
	envFields := []struct{ field, value string }{
		{"integrations.jira.tokenEnv", cfg.Integrations.Jira.TokenEnv},
		{"integrations.jira.webhookSecretEnv", cfg.Integrations.Jira.WebhookSecretEnv},
		{"integrations.github.tokenEnv", cfg.Integrations.GitHub.TokenEnv},
		{"integrations.github.webhookSecretEnv", cfg.Integrations.GitHub.WebhookSecretEnv},
		{"integrations.github.appIDEnv", cfg.Integrations.GitHub.AppIDEnv},
		{"integrations.github.installationTokenEnv", cfg.Integrations.GitHub.InstallationTokenEnv},
		{"integrations.identity.tokenEnv", cfg.Integrations.Identity.TokenEnv},
		{"integrations.review.verifierEnv", cfg.Integrations.Review.VerifierEnv},
	}
	for _, entry := range envFields {
		if entry.value != "" && !IsEnvName(entry.value) {
			return Config{}, fmt.Errorf("%s must be an environment variable name", entry.field)
		}
	}
	switch cfg.Integrations.Jira.Mode {
	case "", "fixture":
		cfg.Integrations.Jira.Mode = "fixture"
	case "jira-cloud":
		parsed, err := url.Parse(cfg.Integrations.Jira.BaseURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return Config{}, fmt.Errorf("integrations.jira.baseURL must be an https origin with no path, query, or credentials")
		}
		if !IsEnvName(cfg.Integrations.Jira.TokenEnv) {
			return Config{}, fmt.Errorf("integrations.jira.tokenEnv must be an environment variable name")
		}
	default:
		return Config{}, fmt.Errorf("unsupported integrations.jira.mode %q", cfg.Integrations.Jira.Mode)
	}
	switch cfg.Integrations.GitHub.Mode {
	case "", "local-cli":
		cfg.Integrations.GitHub.Mode = "local-cli"
	case "disabled":
	case "github-app":
		if !httpsOrigin(cfg.Integrations.GitHub.BaseURL) {
			return Config{}, fmt.Errorf("integrations.github.baseURL must be an https origin with no path, query, or credentials")
		}
		if !IsEnvName(cfg.Integrations.GitHub.AppIDEnv) {
			return Config{}, fmt.Errorf("integrations.github.appIDEnv must be an environment variable name")
		}
		if !IsEnvName(cfg.Integrations.GitHub.InstallationTokenEnv) {
			return Config{}, fmt.Errorf("integrations.github.installationTokenEnv must be an environment variable name")
		}
		if !validGitHubAppActor(cfg.Integrations.GitHub.Actor) {
			return Config{}, fmt.Errorf("integrations.github.actor must be a short service identity")
		}
	default:
		return Config{}, fmt.Errorf("unsupported integrations.github.mode %q", cfg.Integrations.GitHub.Mode)
	}
	if cfg.Integrations.GitHub.Mode == "local-cli" && cfg.Integrations.GitHub.BaseURL != "" && !httpsOrigin(cfg.Integrations.GitHub.BaseURL) {
		return Config{}, fmt.Errorf("integrations.github.baseURL must be an https origin with no path, query, or credentials")
	}
	switch cfg.Integrations.Identity.Mode {
	case "", "local-manual":
		cfg.Integrations.Identity.Mode = "local-manual"
	case "federated":
		if !httpsIssuer(cfg.Integrations.Identity.Issuer) {
			return Config{}, fmt.Errorf("integrations.identity.issuer must be an https URL with no credentials, query, or fragment")
		}
		if !IsEnvName(cfg.Integrations.Identity.TokenEnv) {
			return Config{}, fmt.Errorf("integrations.identity.tokenEnv must be an environment variable name")
		}
		if !validExtensionLabel(cfg.Integrations.Identity.Audience) {
			return Config{}, fmt.Errorf("integrations.identity.audience must be a short label")
		}
		if !validExtensionLabel(cfg.Integrations.Identity.ActorClaim) {
			return Config{}, fmt.Errorf("integrations.identity.actorClaim must be a short claim name")
		}
	default:
		return Config{}, fmt.Errorf("unsupported integrations.identity.mode %q", cfg.Integrations.Identity.Mode)
	}
	switch cfg.Integrations.Review.Mode {
	case "", "local":
		cfg.Integrations.Review.Mode = "local"
	case "external-attestation":
		if !httpsIssuer(cfg.Integrations.Review.Issuer) {
			return Config{}, fmt.Errorf("integrations.review.issuer must be an https URL with no credentials, query, or fragment")
		}
		if !IsEnvName(cfg.Integrations.Review.VerifierEnv) {
			return Config{}, fmt.Errorf("integrations.review.verifierEnv must be an environment variable name")
		}
	default:
		return Config{}, fmt.Errorf("unsupported integrations.review.mode %q", cfg.Integrations.Review.Mode)
	}
	extensionFields := []struct{ field, value string }{
		{"integrations.identity.issuer", cfg.Integrations.Identity.Issuer},
		{"integrations.identity.audience", cfg.Integrations.Identity.Audience},
		{"integrations.identity.actorClaim", cfg.Integrations.Identity.ActorClaim},
		{"integrations.review.issuer", cfg.Integrations.Review.Issuer},
	}
	for _, entry := range extensionFields {
		if util.SecretShapeCount(entry.value) > 0 {
			return Config{}, fmt.Errorf("%s contains a credential-shaped value", entry.field)
		}
	}
	return cfg, nil
}

func httpsOrigin(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil &&
		parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

// httpsIssuer allows an HTTPS path (for example an OIDC realm) but never
// userinfo, query strings, or fragments that could carry credentials.
func httpsIssuer(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil &&
		parsed.RawQuery == "" && parsed.Fragment == ""
}

var profileIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var envNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var githubAppActorPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,99}$`)
var extensionLabelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,99}$`)

// IsEnvName reports whether name is a safe environment variable name. Config
// stores env names only; secrets must never be written into ae.yaml.
func IsEnvName(name string) bool { return envNamePattern.MatchString(name) }

func validGitHubAppActor(actor string) bool { return githubAppActorPattern.MatchString(actor) }

func validExtensionLabel(value string) bool { return extensionLabelPattern.MatchString(value) }

// LoadProfile reads a checked-in profile definition. Verification commands
// are accepted only from this canonical source, never from untrusted input.
func LoadProfile(root, name string) (Profile, error) {
	return loadProfile(root, name, map[string]bool{})
}

func loadProfile(root, name string, loading map[string]bool) (Profile, error) {
	if !profileIDPattern.MatchString(name) {
		return Profile{}, fmt.Errorf("invalid profile name %q", name)
	}
	if loading[name] {
		return Profile{}, fmt.Errorf("profile include cycle at %q", name)
	}
	loading[name] = true
	defer delete(loading, name)

	path := framework.Dir(root, framework.Profiles, name, "profile.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read profile %s: %w", name, err)
	}
	var profile Profile
	if err := yaml.Unmarshal(raw, &profile); err != nil {
		return Profile{}, fmt.Errorf("parse profile %s: %w", name, err)
	}
	if profile.SchemaVersion != version.SchemaVersion {
		return Profile{}, fmt.Errorf("unsupported profile schemaVersion %q (want %q)", profile.SchemaVersion, version.SchemaVersion)
	}
	if profile.ID != name {
		return Profile{}, fmt.Errorf("profile id %q does not match %q", profile.ID, name)
	}
	for _, include := range profile.Includes {
		if !profileIDPattern.MatchString(include) {
			return Profile{}, fmt.Errorf("profile %s has invalid include %q", name, include)
		}
	}
	for _, check := range profile.Checks {
		if !profileIDPattern.MatchString(check.Gate) {
			return Profile{}, fmt.Errorf("profile %s has invalid gate %q", name, check.Gate)
		}
		if check.Command == "" {
			return Profile{}, fmt.Errorf("profile %s has an empty verification command", name)
		}
	}

	merged := Profile{
		SchemaVersion:    profile.SchemaVersion,
		ID:               profile.ID,
		Version:          profile.Version,
		Checks:           appendUniqueChecks(nil, profile.Checks),
		EvidenceCommands: appendUniqueStrings(nil, profile.EvidenceCommands...),
	}
	for _, include := range profile.Includes {
		included, err := loadProfile(root, include, loading)
		if err != nil {
			return Profile{}, fmt.Errorf("profile %s includes %s: %w", name, include, err)
		}
		merged.Checks = appendUniqueChecks(merged.Checks, included.Checks)
		merged.EvidenceCommands = appendUniqueStrings(merged.EvidenceCommands, included.EvidenceCommands...)
	}
	return merged, nil
}

type checkKey struct {
	gate              string
	command           string
	expectEmptyOutput bool
}

func appendUniqueChecks(dst, src []Check) []Check {
	seen := make(map[checkKey]bool, len(dst)+len(src))
	for _, check := range dst {
		seen[checkKey{gate: check.Gate, command: check.Command, expectEmptyOutput: check.ExpectEmptyOutput}] = true
	}
	for _, check := range src {
		key := checkKey{gate: check.Gate, command: check.Command, expectEmptyOutput: check.ExpectEmptyOutput}
		if seen[key] {
			continue
		}
		seen[key] = true
		dst = append(dst, check)
	}
	return dst
}

func appendUniqueStrings(dst []string, src ...string) []string {
	seen := make(map[string]bool, len(dst)+len(src))
	for _, value := range dst {
		seen[value] = true
	}
	for _, value := range src {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		dst = append(dst, value)
	}
	return dst
}

// mergeRoutes keeps verified defaults and applies local overrides without
// mutating the shared default map. Unknown keys survive so validation can
// reject them instead of silently dropping user configuration.
func mergeRoutes(defaults, overrides map[string]map[string]string) map[string]map[string]string {
	merged := make(map[string]map[string]string, len(defaults)+len(overrides))
	for profile, byProvider := range defaults {
		merged[profile] = make(map[string]string, len(byProvider))
		for provider, model := range byProvider {
			merged[profile][provider] = model
		}
	}
	for profile, byProvider := range overrides {
		if merged[profile] == nil {
			merged[profile] = make(map[string]string, len(byProvider))
		}
		for provider, model := range byProvider {
			merged[profile][provider] = model
		}
	}
	return merged
}

// Write serializes cfg to path. It refuses to overwrite an existing file
// unless overwrite is true, preserving user-authored instructions.
func Write(path string, cfg Config, overwrite bool) error {
	if _, err := os.Stat(path); err == nil && !overwrite {
		return fmt.Errorf("refusing to overwrite existing %s (use --force explicitly)", path)
	}
	cfg.SchemaVersion = version.SchemaVersion
	cfg.Root = ""
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// validateContainedPath rejects absolute paths, ".." traversal, and symlink
// escapes. A checked-in ae.yaml is untrusted input: without this check it
// could redirect state or evidence writes outside the project.
func validateContainedPath(field, value, root string) error {
	if value == "" {
		return nil
	}
	if filepath.IsAbs(value) {
		return fmt.Errorf("%s must be relative to the config root", field)
	}
	clean := filepath.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s must stay inside the config root", field)
	}

	target := filepath.Join(root, clean)
	ancestor := target
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
			resolvedRoot, err := filepath.EvalSymlinks(root)
			if err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
			rel, err := filepath.Rel(resolvedRoot, resolved)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("%s must stay inside the config root", field)
			}
			return nil
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return nil
		}
		ancestor = parent
	}
}

// Abs resolves a configured path relative to the config root.
func (c Config) Abs(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(c.Root, path)
}

// StatePath returns the absolute SQLite state path.
func (c Config) StatePath() string { return c.Abs(c.State.Path) }

// EvidenceDir returns the absolute evidence directory.
func (c Config) EvidenceDir() string { return c.Abs(c.Evidence.Dir) }

// ObjectsDir returns the absolute content-addressed object directory.
func (c Config) ObjectsDir() string { return c.Abs(c.Evidence.ObjectsDir) }

// SelfCheck reports obvious configuration mistakes without contacting services.
func (c Config) SelfCheck() []string {
	var problems []string
	if err := util.EnsureDir(filepath.Dir(c.StatePath())); err != nil {
		problems = append(problems, fmt.Sprintf("state directory: %v", err))
	}
	if c.Provider.Default == "gemini" {
		problems = append(problems, "provider/gemini: local extension point only; live invocation is not implemented")
	}
	if c.Integrations.GitHub.Mode == "github-app" {
		problems = append(problems, "integrations.github: GitHub App identity is configured but not implemented")
	}
	if c.Integrations.Identity.Mode == "federated" {
		problems = append(problems, "integrations.identity: federated approver identity is configured but not implemented; approvals remain local-manual")
	}
	if c.Integrations.Review.Mode == "external-attestation" {
		problems = append(problems, "integrations.review: external review attestation is configured but no independent verifier is implemented")
	}
	for _, p := range c.Profiles {
		if !validProfile(p) {
			problems = append(problems, "unknown profile: "+p)
		}
	}
	return problems
}

func validProfile(name string) bool { return IsKnownProfile(name) }
