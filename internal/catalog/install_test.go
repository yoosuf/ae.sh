package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoosuf/ae.sh/internal/framework"
	"github.com/yoosuf/ae.sh/internal/util"
	"github.com/yoosuf/ae.sh/internal/version"
)

func TestInstallPlanApplyConflictAndUninstall(t *testing.T) {
	repoRoot := t.TempDir()
	writeSkill(t, framework.Dir(repoRoot, framework.Skills), "test-skill", validSkillMarkdown("test-skill", true, ""))
	cat, err := Load(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	projectDir := t.TempDir()
	opts := InstallOptions{
		Provider:   "claude",
		Scope:      "project",
		ProjectDir: projectDir,
		HomeDir:    t.TempDir(),
		Version:    "0.1.0",
	}
	base := filepath.Join(projectDir, ".claude", "skills")
	target := filepath.Join(base, "test-skill", "SKILL.md")
	manifestPath := filepath.Join(base, manifestName)

	plans, err := PlanInstall(cat, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || len(plans[0].Writes) != 1 || len(plans[0].Blocked) != 0 {
		t.Fatalf("unexpected dry-run plan: %+v", plans)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote %s", target)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("user-authored skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plans, err = PlanInstall(cat, opts)
	if err != nil || len(plans[0].Blocked) == 0 {
		t.Fatalf("unrecognized file should block install: %+v, %v", plans, err)
	}
	if err := Apply(cat, plans[0], opts); err == nil {
		t.Fatal("apply should refuse a blocked plan")
	}
	opts.Force = true
	plans, err = PlanInstall(cat, opts)
	if err != nil || len(plans[0].Writes) != 1 {
		t.Fatalf("force should allow overwrite: %+v, %v", plans, err)
	}
	if err := Apply(cat, plans[0], opts); err != nil {
		t.Fatalf("forced apply failed: %v", err)
	}
	opts.Force = false

	installed, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Skills) != 1 {
		t.Fatalf("unexpected manifest skills: %+v", manifest.Skills)
	}
	if manifest.Skills[0].SHA256 != strings.TrimPrefix(util.Digest(installed), "sha256:") {
		t.Errorf("manifest checksum does not match installed bytes")
	}
	if _, err := os.Stat(filepath.Join(base, "test-skill", "agents", "openai.yaml")); !os.IsNotExist(err) {
		t.Errorf("Claude install unexpectedly wrote Codex metadata")
	}

	plans, err = PlanInstall(cat, opts)
	if err != nil || len(plans[0].Writes) != 0 || len(plans[0].Unchanged) != 1 {
		t.Fatalf("second plan should be unchanged: %+v, %v", plans, err)
	}

	// A manifest-authorised file may be refreshed without --force, but an
	// unrelated file in the managed directory still blocks the operation.
	if err := os.WriteFile(target, []byte("local edit to managed file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plans, err = PlanInstall(cat, opts)
	if err != nil || len(plans[0].Writes) != 1 || len(plans[0].Blocked) != 0 {
		t.Fatalf("managed file should be refreshable: %+v, %v", plans, err)
	}
	if err := Apply(cat, plans[0], opts); err != nil {
		t.Fatal(err)
	}

	extra := filepath.Join(base, "test-skill", "notes.txt")
	if err := os.WriteFile(extra, []byte("user note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plans, err = PlanInstall(cat, opts)
	if err != nil || len(plans[0].Blocked) == 0 {
		t.Fatalf("unmanaged file should block install: %+v, %v", plans, err)
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}

	keep := filepath.Join(base, "keep-me.md")
	if err := os.WriteFile(keep, []byte("do not delete\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(manifestPath, base, false); err == nil {
		t.Fatal("uninstall should refuse unmanaged files without force")
	}
	removed, err := Uninstall(manifestPath, base, true)
	if err != nil || len(removed) != 1 {
		t.Fatalf("forced uninstall failed: %v, %v", removed, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("installed skill was not removed")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("unmanaged file was removed: %v", err)
	}
}

func TestUninstallRejectsManifestControlledTargetAndSlug(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	manifestPath := filepath.Join(base, manifestName)
	writeManifest := func(target, slug string) {
		t.Helper()
		m := Manifest{
			SchemaVersion: version.SchemaVersion,
			TargetDir:     target,
			Skills:        []ManifestSkill{{Slug: slug, Path: slug + "/SKILL.md", SHA256: "sha256:test"}},
		}
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	writeManifest(outside, "test-skill")
	if _, err := Uninstall(manifestPath, base, true); err == nil {
		t.Fatal("manifest redirected recursive deletion outside the expected base")
	}
	writeManifest(base, "../escape")
	if _, err := Uninstall(manifestPath, base, true); err == nil {
		t.Fatal("manifest supplied a non-canonical skill slug")
	}
}

func TestPlanInstallRejectsUnknownProviderAndScope(t *testing.T) {
	cat, err := Load(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PlanInstall(cat, InstallOptions{Provider: "other", Scope: "user"}); err == nil {
		t.Fatal("unknown provider must fail")
	}
	if _, err := PlanInstall(cat, InstallOptions{Provider: "claude", Scope: "other"}); err == nil {
		t.Fatal("unknown scope must fail")
	}
	if _, err := PlanInstall(cat, InstallOptions{Provider: "claude", Scope: "project"}); err == nil {
		t.Fatal("project scope without directory must fail")
	}
}
