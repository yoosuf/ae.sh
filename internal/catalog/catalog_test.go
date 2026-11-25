package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validSkillMarkdown(name string, includeFailure bool, extra string) string {
	sections := []string{
		"## Triggers\n\nUse this skill when a bounded, testable engineering step is required.",
		"## Objective\n\nProduce a precise artifact that another agent or human can verify without guessing.",
		"## Inputs\n\nAccept only structured inputs with explicit identifiers and known code SHAs.",
		"## Preconditions\n\nThe repository, issue fixture, and required tools must be identified before starting.",
		"## Procedure\n\nInspect the contract, perform one bounded step, record observed results, and stop at uncertainty.",
		"## Constraints\n\nDo not invent results, execute untrusted commands, or use credentials from task text.",
		"## Outputs\n\nReturn a JSON summary, artifact paths, and honest failure reasons when checks cannot run.",
		"## Completion criteria\n\nEvery required section is present and the output can be reproduced from recorded evidence.",
	}
	if includeFailure {
		sections = append(sections, "## Failure handling\n\nStop, mark the work blocked, and report the missing precondition without fabricating a pass.")
	}
	if extra != "" {
		sections = append(sections, extra)
	}
	return "---\n" +
		"name: " + name + "\n" +
		"description: A deterministic test skill used to check canonical skill validation rules.\n" +
		"license: Apache-2.0\n" +
		"metadata:\n" +
		"  ae.version: \"0.1.0\"\n" +
		"  ae.domain: Development\n" +
		"  ae.skill-id: " + name + "\n" +
		"  ae.eval-fixture: evaluations/fixtures/" + name + "/\n" +
		"---\n\n" + strings.Join(sections, "\n\n") + "\n"
}

func writeSkill(t *testing.T, root, slug, markdown string) string {
	t.Helper()
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(markdown), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadCanonicalCatalogue(t *testing.T) {
	cat, err := Load(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("canonical catalogue rejected: %v", err)
	}
	if len(cat.Skills) != 15 {
		t.Fatalf("expected 15 canonical skills, got %d", len(cat.Skills))
	}
	skill, ok := cat.BySlug("unit-testing")
	if !ok {
		t.Fatal("unit-testing skill missing")
	}
	for _, section := range []string{"Triggers", "Procedure", "Completion criteria", "Failure handling"} {
		if !contains(skill.Sections, section) {
			t.Errorf("unit-testing is missing section %q", section)
		}
	}
	if broken := ValidateReferences(skill); len(broken) > 0 {
		t.Errorf("canonical skill has broken references: %v", broken)
	}
}

func TestParseRejectsInvalidSkillContracts(t *testing.T) {
	root := t.TempDir()
	missingSection := writeSkill(t, root, "test-skill", validSkillMarkdown("test-skill", false, ""))
	if _, err := ParseFile(missingSection); err == nil {
		t.Fatal("expected missing Failure handling section to be rejected")
	}
	mismatch := writeSkill(t, filepath.Join(root, "mismatch"), "test-skill", validSkillMarkdown("other-skill", true, ""))
	if _, err := ParseFile(mismatch); err == nil {
		t.Fatal("expected name/directory mismatch to be rejected")
	}
	broken := writeSkill(t, filepath.Join(root, "broken"), "test-skill", validSkillMarkdown("test-skill", true, "## Progressive disclosure\n\nSee references/missing.md for details."))
	skill, err := ParseFile(broken)
	if err != nil {
		t.Fatalf("skill with reference text should parse: %v", err)
	}
	if issues := ValidateReferences(skill); len(issues) != 1 || !strings.Contains(issues[0], "references/missing.md") {
		t.Fatalf("expected broken reference to be reported, got %v", issues)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
