package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yoosuf/ae.sh/internal/util"
	"github.com/yoosuf/ae.sh/internal/version"
)

const manifestName = ".ae-manifest.json"

// Manifest records exactly what AE materialized into a provider directory.
type Manifest struct {
	SchemaVersion    string          `json:"schemaVersion" yaml:"schemaVersion"`
	FrameworkVersion string          `json:"frameworkVersion" yaml:"frameworkVersion"`
	Provider         string          `json:"provider" yaml:"provider"`
	Scope            string          `json:"scope" yaml:"scope"`
	TargetDir        string          `json:"targetDir" yaml:"targetDir"`
	GeneratedAt      string          `json:"generatedAt" yaml:"generatedAt"`
	Skills           []ManifestSkill `json:"skills" yaml:"skills"`
	UnmanagedFiles   []string        `json:"unmanagedFiles,omitempty" yaml:"unmanagedFiles,omitempty"`
}

// ManifestSkill binds a slug to the exact bytes written.
type ManifestSkill struct {
	Slug    string `json:"slug" yaml:"slug"`
	Path    string `json:"path" yaml:"path"`
	SHA256  string `json:"sha256" yaml:"sha256"`
	Version string `json:"version" yaml:"version"`
}

// InstallOptions controls provider materialization.
type InstallOptions struct {
	Provider   string // claude, codex, or all
	Scope      string // user or project
	ProjectDir string // repository root for project scope
	HomeDir    string // override home for tests; defaults to $HOME
	Version    string // catalogue version recorded in the manifest
	Force      bool   // explicit authorization to overwrite divergent files
	DryRun     bool
}

// InstallPlan is a preview of a materialization operation.
type InstallPlan struct {
	Provider     string
	Scope        string
	TargetDir    string
	Writes       []string
	Unchanged    []string
	Blocked      []string
	ManifestPath string
	Manifest     Manifest
}

// InstallResult summarizes an applied plan.
type InstallResult struct {
	Plans []InstallPlan
}

// Validate checks install options without touching the filesystem.
func (o InstallOptions) Validate() error {
	switch o.Provider {
	case "claude", "codex", "all":
	default:
		return fmt.Errorf("unsupported provider %q (want claude, codex, or all)", o.Provider)
	}
	switch o.Scope {
	case "user", "project":
	default:
		return fmt.Errorf("unsupported scope %q (want user or project)", o.Scope)
	}
	if o.Scope == "project" && strings.TrimSpace(o.ProjectDir) == "" {
		return fmt.Errorf("project scope requires a project directory")
	}
	return nil
}

// PlanInstall computes deterministic provider targets and detects conflicts
// with user-authored files. It never writes.
func PlanInstall(cat Catalogue, opts InstallOptions) ([]InstallPlan, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	providers := []string{opts.Provider}
	if opts.Provider == "all" {
		providers = []string{"claude", "codex"}
	}
	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory: %w", err)
		}
	}
	catVersion := opts.Version
	if catVersion == "" {
		catVersion = version.SupportedSkillsVersion
	}
	generatedAt := util.Now()
	plans := make([]InstallPlan, 0, len(providers))
	for _, provider := range providers {
		base := providerBase(provider, opts.Scope, home, opts.ProjectDir)
		plan := InstallPlan{
			Provider:     provider,
			Scope:        opts.Scope,
			TargetDir:    base,
			ManifestPath: filepath.Join(base, manifestName),
		}
		plan.Manifest = Manifest{
			SchemaVersion:    version.SchemaVersion,
			FrameworkVersion: version.Version,
			Provider:         provider,
			Scope:            opts.Scope,
			TargetDir:        filepath.ToSlash(base),
			GeneratedAt:      generatedAt,
		}
		previous, _ := LoadManifest(plan.ManifestPath)
		previousBySlug := map[string]string{}
		if previous != nil {
			for _, item := range previous.Skills {
				previousBySlug[item.Slug] = item.SHA256
			}
		}
		for _, skill := range cat.Skills {
			target := filepath.Join(base, skill.Slug)
			skillPath := filepath.Join(target, "SKILL.md")
			digest := strings.TrimPrefix(util.Digest([]byte(renderSkill(skill))), "sha256:")
			plan.Manifest.Skills = append(plan.Manifest.Skills, ManifestSkill{
				Slug:    skill.Slug,
				Path:    filepath.ToSlash(filepath.Join(skill.Slug, "SKILL.md")),
				SHA256:  digest,
				Version: catVersion,
			})
			existing, err := os.ReadFile(skillPath)
			switch {
			case err != nil && !os.IsNotExist(err):
				plan.Blocked = append(plan.Blocked, fmt.Sprintf("%s: %v", skillPath, err))
			case os.IsNotExist(err):
				plan.Writes = append(plan.Writes, skillPath)
			case util.Digest(existing) == "sha256:"+digest:
				plan.Unchanged = append(plan.Unchanged, skillPath)
			case previousBySlug[skill.Slug] == digest || opts.Force:
				plan.Writes = append(plan.Writes, skillPath)
			default:
				plan.Blocked = append(plan.Blocked, fmt.Sprintf("%s: unrecognized local edits; rerun with --force to overwrite", skillPath))
			}
			for _, extra := range unexpectedFiles(target, provider) {
				plan.Blocked = append(plan.Blocked, fmt.Sprintf("%s: unmanaged file would be preserved but blocks install; move it aside or use --force", extra))
			}
		}
		plan.Manifest.UnmanagedFiles = scanUnmanaged(base, cat.Slugs())
		sort.Strings(plan.Writes)
		sort.Strings(plan.Unchanged)
		sort.Strings(plan.Blocked)
		plans = append(plans, plan)
	}
	return plans, nil
}

// Apply writes a previously computed plan. It rechecks conflicts to avoid
// time-of-check/time-of-write surprises.
func Apply(cat Catalogue, plan InstallPlan, opts InstallOptions) error {
	if len(plan.Blocked) > 0 {
		return fmt.Errorf("install blocked by %d conflict(s): %s", len(plan.Blocked), plan.Blocked[0])
	}
	if err := util.EnsureDir(plan.TargetDir); err != nil {
		return err
	}
	for _, slug := range cat.Slugs() {
		skill, _ := cat.BySlug(slug)
		dir := filepath.Join(plan.TargetDir, slug)
		if err := util.EnsureDir(dir); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(renderSkill(skill)), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", slug, err)
		}
		if plan.Provider == "codex" {
			meta := map[string]any{
				"interface": map[string]any{
					"display_name":      skill.Metadata["ae.domain"] + ": " + skill.Slug,
					"short_description": truncate(skill.Description, 120),
				},
				"policy": map[string]any{"allow_implicit_invocation": true},
			}
			raw, err := yaml.Marshal(meta)
			if err != nil {
				return err
			}
			metaDir := filepath.Join(dir, "agents")
			if err := util.EnsureDir(metaDir); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(metaDir, "openai.yaml"), raw, 0o644); err != nil {
				return fmt.Errorf("write codex metadata for %s: %w", slug, err)
			}
		}
	}
	manifestRaw, err := util.JSONValue(plan.Manifest)
	if err != nil {
		return err
	}
	raw, err := marshalIndent(manifestRaw)
	if err != nil {
		return err
	}
	return os.WriteFile(plan.ManifestPath, raw, 0o600)
}

// LoadManifest reads a previously written manifest.
func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Uninstall removes only files listed in a manifest and preserves everything
// else. It is deliberately conservative: unknown files block removal. The
// expected base is supplied by the caller, never trusted from the manifest,
// because recursive deletion must not be controlled by a checked-in file.
func Uninstall(manifestPath, expectedBase string, force bool) ([]string, error) {
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if m.SchemaVersion != version.SchemaVersion {
		return nil, fmt.Errorf("unsupported manifest schemaVersion %q", m.SchemaVersion)
	}
	base := m.TargetDir
	if base == "" {
		return nil, fmt.Errorf("manifest has no targetDir")
	}
	if expectedBase == "" {
		return nil, fmt.Errorf("uninstall requires the expected target directory")
	}
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return nil, fmt.Errorf("manifest targetDir: %w", err)
	}
	resolvedExpected, err := filepath.EvalSymlinks(expectedBase)
	if err != nil {
		return nil, fmt.Errorf("expected targetDir: %w", err)
	}
	if resolvedBase != resolvedExpected {
		return nil, fmt.Errorf("manifest targetDir %q does not match expected %q", base, expectedBase)
	}
	for _, skill := range m.Skills {
		if !validManifestSlug(skill.Slug) {
			return nil, fmt.Errorf("manifest contains invalid skill slug %q", skill.Slug)
		}
	}
	if extras := scanUnmanaged(base, manifestSlugs(m)); len(extras) > 0 && !force {
		return nil, fmt.Errorf("refusing to remove %s: unmanaged files present (%s)", base, strings.Join(extras, ", "))
	}
	var removed []string
	for _, skill := range m.Skills {
		dir := filepath.Join(base, skill.Slug)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return nil, err
		}
		removed = append(removed, dir)
	}
	if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return removed, nil
}

func validManifestSlug(slug string) bool {
	if slug == "" || len(slug) > 64 {
		return false
	}
	for i, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-' && i > 0 && i < len(slug)-1:
		default:
			return false
		}
	}
	return true
}

func manifestSlugs(m *Manifest) []string {
	out := make([]string, 0, len(m.Skills))
	for _, s := range m.Skills {
		out = append(out, s.Slug)
	}
	return out
}

func providerBase(provider, scope, home, projectDir string) string {
	switch provider {
	case "claude":
		if scope == "user" {
			return filepath.Join(home, ".claude", "skills")
		}
		return filepath.Join(projectDir, ".claude", "skills")
	case "codex":
		if scope == "user" {
			return filepath.Join(home, ".agents", "skills")
		}
		return filepath.Join(projectDir, ".agents", "skills")
	default:
		return ""
	}
}

// renderSkill produces provider-neutral SKILL.md bytes from the canonical
// skill. Provider differences are layered through adjacent files, not by
// rewriting instructions.
func renderSkill(s Skill) string {
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

func unexpectedFiles(dir, provider string) []string {
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	allowed := map[string]bool{"SKILL.md": true}
	if provider == "codex" {
		allowed["agents"] = true
	}
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || allowed[e.Name()] {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

func scanUnmanaged(base string, known []string) []string {
	if _, err := os.Stat(base); err != nil {
		return nil
	}
	knownSet := map[string]bool{}
	for _, k := range known {
		knownSet[k] = true
	}
	var out []string
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || knownSet[name] || name == "README.md" {
			continue
		}
		out = append(out, filepath.Join(base, name))
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}

func marshalIndent(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return nil, err
	}
	return append(pretty.Bytes(), '\n'), nil
}
