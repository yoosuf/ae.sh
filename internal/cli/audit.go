package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yoosuf/ae.sh/internal/catalog"
	"github.com/yoosuf/ae.sh/internal/config"
	"github.com/yoosuf/ae.sh/internal/schema"
	"github.com/yoosuf/ae.sh/internal/util"
)

type auditCheck struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	Detail       string   `json:"detail,omitempty"`
	FilesScanned int      `json:"filesScanned,omitempty"`
	Findings     []string `json:"findings,omitempty"`
}

type auditReport struct {
	SchemaVersion string       `json:"schemaVersion"`
	Status        string       `json:"status"`
	Checks        []auditCheck `json:"checks"`
	Note          string       `json:"note"`
}

var (
	auditUsesPattern = regexp.MustCompile(`(?m)^\s*uses:\s*(\S+)`)
	auditSHAPin      = regexp.MustCompile(`^[^@\s]+@[0-9a-f]{40}$`)
)

func (a *App) cmdAudit(args []string) error {
	fs := newFlagSet("audit")
	if err := fs.Parse(args); err != nil {
		return usage("audit: %v", err)
	}
	if fs.NArg() != 0 {
		return usage("audit takes no positional arguments")
	}
	root := a.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	if configPath, err := config.Find(root); err == nil {
		root = filepath.Dir(configPath)
	}
	checks := []auditCheck{
		auditCredentialShapes(root),
		auditEmbeddedSchemas(root),
		auditCanonicalReferences(root),
		auditGeneratedCopies(root),
		auditActionPinning(root),
	}
	status := "passed"
	for _, check := range checks {
		if check.Status == "failed" {
			status = "failed"
			break
		}
		if check.Status == "needs-authorization" {
			status = "incomplete"
		}
	}
	report := auditReport{
		SchemaVersion: "1.0",
		Status:        status,
		Checks:        checks,
		Note:          "Local read-only audit. No live service, release approval, or AE gate is satisfied. 'needs-authorization' items are reported, not guessed.",
	}
	a.emit(report)
	if status == "failed" {
		return invalid("local audit found %d failing check(s)", failedAuditChecks(checks))
	}
	return nil
}

func failedAuditChecks(checks []auditCheck) int {
	count := 0
	for _, check := range checks {
		if check.Status == "failed" {
			count++
		}
	}
	return count
}

func auditCredentialShapes(root string) auditCheck {
	filesScanned := 0
	findings := []string{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".ae", "dist", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > 1<<20 {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil || bytes.IndexByte(raw, 0) >= 0 {
			return nil
		}
		filesScanned++
		if util.SecretShapeCount(string(raw)) > 0 {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			findings = append(findings, util.Sanitize(filepath.ToSlash(rel)))
		}
		return nil
	})
	status := "passed"
	if len(findings) > 0 {
		status = "failed"
	}
	return auditCheck{
		Name: "credential-shapes", Status: status, FilesScanned: filesScanned,
		Detail: fmt.Sprintf("scanned %d readable text files", filesScanned), Findings: findings,
	}
}

func auditEmbeddedSchemas(root string) auditCheck {
	findings := []string{}
	for _, name := range schema.Names {
		canonical, err := os.ReadFile(filepath.Join(root, "framework", "schemas", name+".schema.json"))
		if err != nil {
			findings = append(findings, name+": canonical schema is missing or unreadable")
			continue
		}
		embedded, err := schema.Asset(name)
		if err != nil {
			findings = append(findings, name+": embedded schema is unreadable")
			continue
		}
		if !bytes.Equal(canonical, embedded) {
			findings = append(findings, name+": embedded copy differs from canonical schema")
		}
	}
	status := "passed"
	if len(findings) > 0 {
		status = "failed"
	}
	return auditCheck{Name: "embedded-schemas", Status: status, Findings: findings}
}

func auditCanonicalReferences(root string) auditCheck {
	cat, err := catalog.Load(root)
	if err != nil {
		return auditCheck{Name: "canonical-references", Status: "failed", Findings: []string{util.Sanitize(err.Error())}}
	}
	broken := catBrokenReferences(cat)
	for i := range broken {
		broken[i] = util.Sanitize(broken[i])
	}
	status := "passed"
	if len(broken) > 0 {
		status = "failed"
	}
	return auditCheck{Name: "canonical-references", Status: status, Findings: broken}
}

type auditManifest struct {
	Provider string `json:"provider"`
	Skills   []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Slug   string `json:"slug"`
	} `json:"skills"`
}

func auditGeneratedCopies(root string) auditCheck {
	paths := []string{
		filepath.Join(root, ".claude", "skills", ".ae-manifest.json"),
		filepath.Join(root, ".agents", "skills", ".ae-manifest.json"),
	}
	manifests := 0
	findings := []string{}
	verified := 0
	for _, manifestPath := range paths {
		raw, err := os.ReadFile(manifestPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			findings = append(findings, util.Sanitize(filepath.ToSlash(manifestPath))+": unreadable manifest")
			continue
		}
		manifests++
		dir := filepath.Dir(manifestPath)
		var manifest auditManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			findings = append(findings, util.Sanitize(filepath.ToSlash(manifestPath))+": invalid manifest JSON")
			continue
		}
		for _, skill := range manifest.Skills {
			target := filepath.Join(dir, filepath.FromSlash(skill.Path))
			if !withinRoot(dir, target) {
				findings = append(findings, manifest.Provider+"/"+skill.Slug+": path escapes the skill directory")
				continue
			}
			digest, err := util.DigestFile(target)
			if err != nil {
				findings = append(findings, manifest.Provider+"/"+skill.Slug+": generated copy is missing")
				continue
			}
			if strings.TrimPrefix(digest, "sha256:") != skill.SHA256 {
				findings = append(findings, manifest.Provider+"/"+skill.Slug+": checksum mismatch")
				continue
			}
			verified++
		}
	}
	if manifests == 0 {
		return auditCheck{Name: "generated-skill-copies", Status: "skipped", Detail: "no installed provider skill copies found"}
	}
	status := "passed"
	if len(findings) > 0 {
		status = "failed"
	}
	return auditCheck{
		Name: "generated-skill-copies", Status: status,
		Detail:   fmt.Sprintf("verified %d generated skill copies across %d manifest(s)", verified, manifests),
		Findings: findings,
	}
}

func auditActionPinning(root string) auditCheck {
	matches, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.y*ml"))
	if err != nil || len(matches) == 0 {
		return auditCheck{Name: "ci-action-pinning", Status: "skipped", Detail: "no CI workflow files found"}
	}
	unpinned := []string{}
	uses := 0
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		for _, found := range auditUsesPattern.FindAllSubmatch(raw, -1) {
			uses++
			value := string(found[1])
			if !auditSHAPin.MatchString(value) {
				unpinned = append(unpinned, util.Sanitize(fmt.Sprintf("%s: %s", filepath.ToSlash(rel), value)))
			}
		}
	}
	if uses == 0 {
		return auditCheck{Name: "ci-action-pinning", Status: "skipped", Detail: "no action references found"}
	}
	if len(unpinned) > 0 {
		return auditCheck{
			Name: "ci-action-pinning", Status: "needs-authorization",
			Detail:   fmt.Sprintf("%d of %d action references are not SHA-pinned; authoritative SHAs require an authorized lookup", len(unpinned), uses),
			Findings: unpinned,
		}
	}
	return auditCheck{Name: "ci-action-pinning", Status: "passed", Detail: fmt.Sprintf("%d action references are SHA-pinned", uses)}
}
