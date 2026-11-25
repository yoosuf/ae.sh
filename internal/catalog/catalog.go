// Package catalog loads the canonical skill catalogue and validates each skill
// as an executable instruction contract. It has no provider SDK dependencies.
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aef-framework/aef/internal/framework"
	"github.com/aef-framework/aef/internal/schema"
	"github.com/aef-framework/aef/internal/util"
	"github.com/aef-framework/aef/internal/version"
)

// Skill is the normalized canonical representation of a SKILL.md file.
type Skill struct {
	SchemaVersion string            `json:"schemaVersion"`
	Slug          string            `json:"slug"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license"`
	Compatibility string            `json:"compatibility,omitempty"`
	AllowedTools  []string          `json:"allowedTools,omitempty"`
	Metadata      map[string]string `json:"metadata"`
	Body          string            `json:"body"`
	Sections      []string          `json:"sections"`
	References    []string          `json:"references,omitempty"`
	Path          string            `json:"path"`
}

// Catalogue is a validated set of skills keyed by unique flat slug.
type Catalogue struct {
	Skills []Skill
	Root   string
	bySlug map[string]Skill
}

// BySlug returns a skill by slug.
func (c Catalogue) BySlug(slug string) (Skill, bool) {
	s, ok := c.bySlug[slug]
	return s, ok
}

// BySlugSet returns a lookup set of all canonical skill slugs.
func (c Catalogue) BySlugSet() map[string]bool {
	out := make(map[string]bool, len(c.Skills))
	for slug := range c.bySlug {
		out[slug] = true
	}
	return out
}

func (c Catalogue) bySlugSet() map[string]bool { return c.BySlugSet() }

// Slugs returns all slugs in deterministic order.
func (c Catalogue) Slugs() []string {
	out := make([]string, 0, len(c.Skills))
	for _, s := range c.Skills {
		out = append(out, s.Slug)
	}
	sort.Strings(out)
	return out
}

// Load walks root/framework/skills/*/SKILL.md and validates every skill. It
// fails closed on duplicate slugs, invalid front matter, or a missing required
// section.
func Load(root string) (Catalogue, error) {
	skillsDir := framework.Dir(root, framework.Skills)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return Catalogue{}, fmt.Errorf("read skills directory: %w", err)
	}
	result := Catalogue{Root: root, bySlug: make(map[string]Skill)}
	seen := map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return Catalogue{}, fmt.Errorf("skill %s has no SKILL.md", entry.Name())
		}
		skill, err := ParseFile(path)
		if err != nil {
			return Catalogue{}, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		if skill.Slug != entry.Name() {
			return Catalogue{}, fmt.Errorf("%s: frontmatter name %q must match directory slug", entry.Name(), skill.Slug)
		}
		if previous, ok := seen[skill.Slug]; ok {
			return Catalogue{}, fmt.Errorf("duplicate skill slug %q (%s and %s)", skill.Slug, previous, skill.Path)
		}
		seen[skill.Slug] = skill.Path
		result.Skills = append(result.Skills, skill)
		result.bySlug[skill.Slug] = skill
	}
	if len(result.Skills) == 0 {
		return Catalogue{}, fmt.Errorf("no skills found under %s", skillsDir)
	}
	sort.Slice(result.Skills, func(i, j int) bool { return result.Skills[i].Slug < result.Skills[j].Slug })
	return result, nil
}

// ParseFile parses and validates one canonical SKILL.md.
func ParseFile(path string) (Skill, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	return Parse(path, raw)
}

// Parse parses and validates SKILL.md bytes.
func Parse(path string, raw []byte) (Skill, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Skill{}, fmt.Errorf("SKILL.md must start with YAML front matter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return Skill{}, fmt.Errorf("SKILL.md front matter is not closed")
	}
	front := text[4 : 4+end]
	body := text[4+end+5:]

	var fields map[string]any
	if err := yaml.Unmarshal([]byte(front), &fields); err != nil {
		return Skill{}, fmt.Errorf("parse front matter: %w", err)
	}
	metadata := map[string]string{}
	if value, ok := fields["metadata"]; ok {
		switch m := value.(type) {
		case map[string]any:
			for k, v := range m {
				metadata[k] = fmt.Sprint(v)
			}
		case map[any]any:
			for k, v := range m {
				metadata[fmt.Sprint(k)] = fmt.Sprint(v)
			}
		default:
			return Skill{}, fmt.Errorf("metadata must be a mapping")
		}
	}
	name := stringField(fields, "name")
	slug := filepath.Base(filepath.Dir(path))
	if name != slug {
		return Skill{}, fmt.Errorf("frontmatter name %q must match directory slug %q", name, slug)
	}
	skill := Skill{
		SchemaVersion: version.SchemaVersion,
		Slug:          slug,
		Name:          name,
		Description:   stringField(fields, "description"),
		License:       stringField(fields, "license"),
		Compatibility: stringField(fields, "compatibility"),
		AllowedTools:  stringList(fields["allowed-tools"]),
		Metadata:      metadata,
		Body:          strings.TrimSpace(body),
		Sections:      sections(body),
		References:    references(body, filepath.Dir(path)),
		Path:          filepath.ToSlash(path),
	}
	document, err := skill.ToDocument()
	if err != nil {
		return Skill{}, fmt.Errorf("normalize skill for validation: %w", err)
	}
	if err := schema.Validate("skill", document); err != nil {
		return Skill{}, err
	}
	return skill, nil
}

func stringField(fields map[string]any, key string) string {
	if v, ok := fields[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func stringList(v any) []string {
	switch list := v.(type) {
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			out = append(out, fmt.Sprint(item))
		}
		return out
	case []string:
		return list
	case string:
		return strings.Fields(list)
	default:
		return nil
	}
}

func sections(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "## ")))
		}
	}
	return out
}

// references returns relative reference files that actually exist. A broken
// reference is a validation failure so progressive disclosure stays honest.
func references(body, dir string) []string {
	var missing []string
	var found []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "references/") {
			continue
		}
		start := strings.Index(trimmed, "references/")
		rest := trimmed[start:]
		end := strings.IndexAny(rest, " \t)`\"'>")
		ref := rest
		if end > 0 {
			ref = rest[:end]
		}
		ref = strings.TrimRight(ref, ".,;:")
		if ref == "" || strings.Contains(ref, "..") {
			missing = append(missing, ref)
			continue
		}
		full := filepath.Join(dir, filepath.FromSlash(ref))
		if _, err := os.Stat(full); err == nil {
			found = append(found, ref)
		} else {
			missing = append(missing, ref)
		}
	}
	_ = missing // reported by ValidateReferences below
	sort.Strings(found)
	return unique(found)
}

// ValidateReferences reports broken references/ links in a skill body.
func ValidateReferences(skill Skill) []string {
	var broken []string
	for _, line := range strings.Split(skill.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		start := strings.Index(trimmed, "references/")
		if start < 0 {
			continue
		}
		rest := trimmed[start:]
		end := strings.IndexAny(rest, " \t)`\"'>")
		ref := rest
		if end > 0 {
			ref = rest[:end]
		}
		ref = strings.TrimRight(ref, ".,;:")
		if ref == "" || strings.Contains(ref, "..") {
			broken = append(broken, ref)
			continue
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(skill.Path), filepath.FromSlash(ref))); err != nil {
			broken = append(broken, ref)
		}
	}
	return unique(broken)
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range in {
		if item != "" && !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

// ToDocument returns the normalized JSON-compatible document used for schema
// validation and manifest generation.
func (s Skill) ToDocument() (any, error) {
	return util.JSONValue(s)
}
