// Package config loads the repository-local AEF configuration. Configuration
// never contains credentials; environment variable *names* only.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/yoosuf/ae.sh/internal/framework"
	"github.com/yoosuf/ae.sh/internal/util"
	"github.com/yoosuf/ae.sh/internal/version"
)

const (
	FileName        = "aef.yaml"
	DefaultState    = ".aef/state.db"
	DefaultEvidence = ".aef/evidence"
	DefaultObjects  = ".aef/objects"
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

type Provider struct {
	Default string `yaml:"default" json:"default"`
}

type Deployment struct {
	// local stops after merge-eligible PR evidence; optional requires an
	// explicit release authorization; required is reserved for cloud mode.
	Scope string `yaml:"scope" json:"scope"`
}

type Integrations struct {
	Jira   Integration `yaml:"jira" json:"jira"`
	GitHub Integration `yaml:"github" json:"github"`
}

type Integration struct {
	Mode             string `yaml:"mode" json:"mode"`
	BaseURL          string `yaml:"baseURL,omitempty" json:"baseURL,omitempty"`
	TokenEnv         string `yaml:"tokenEnv,omitempty" json:"tokenEnv,omitempty"`
	WebhookSecretEnv string `yaml:"webhookSecretEnv,omitempty" json:"webhookSecretEnv,omitempty"`
}

// Check is a local verification command from a canonical profile. Commands
// come from checked-in profile files, never from issue text or CLI flags.
type Check struct {
	Gate              string `yaml:"gate" json:"gate"`
	Command           string `yaml:"command" json:"command"`
	ExpectEmptyOutput bool   `yaml:"expectEmptyOutput,omitempty" json:"expectEmptyOutput,omitempty"`
}

// Profile is the canonical local profile definition used by `aef verify`.
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

// IsKnownProfile reports whether name can be selected by `aef init` or
// `aef verify`. The file must still load successfully before use.
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
		Provider:      Provider{Default: "fake"},
		Deployment:    Deployment{Scope: "local"},
		Integrations: Integrations{
			Jira:   Integration{Mode: "fixture", TokenEnv: "AEF_JIRA_TOKEN", WebhookSecretEnv: "AEF_JIRA_WEBHOOK_SECRET"},
			GitHub: Integration{Mode: "local-cli", TokenEnv: "AEF_GITHUB_TOKEN", WebhookSecretEnv: "AEF_GITHUB_WEBHOOK_SECRET"},
		},
	}
}

// Find walks upward from dir looking for aef.yaml.
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
			return "", fmt.Errorf("%s not found (run `aef init` first)", FileName)
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
	if cfg.Provider.Default == "" {
		cfg.Provider.Default = "fake"
	}
	switch cfg.Provider.Default {
	case "fake", "claude", "codex":
	default:
		return Config{}, fmt.Errorf("unsupported provider %q", cfg.Provider.Default)
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
	default:
		return Config{}, fmt.Errorf("unsupported integrations.github.mode %q", cfg.Integrations.GitHub.Mode)
	}
	if cfg.Integrations.GitHub.Mode == "local-cli" && cfg.Integrations.GitHub.BaseURL != "" && !httpsOrigin(cfg.Integrations.GitHub.BaseURL) {
		return Config{}, fmt.Errorf("integrations.github.baseURL must be an https origin with no path, query, or credentials")
	}
	return cfg, nil
}

func httpsOrigin(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil &&
		parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

var profileIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var envNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// IsEnvName reports whether name is a safe environment variable name. Config
// stores env names only; secrets must never be written into aef.yaml.
func IsEnvName(name string) bool { return envNamePattern.MatchString(name) }

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
	for _, p := range c.Profiles {
		if !validProfile(p) {
			problems = append(problems, "unknown profile: "+p)
		}
	}
	return problems
}

func validProfile(name string) bool { return IsKnownProfile(name) }
