package framework

import (
	"path/filepath"
	"testing"
)

func TestRelativeAndDir(t *testing.T) {
	if got, want := Relative(Policies, "default.yaml"), "framework/policies/default.yaml"; got != want {
		t.Fatalf("Relative() = %q, want %q", got, want)
	}
	if got, want := Dir("/repo", Skills, "test-skill", "SKILL.md"), filepath.Join("/repo", "framework", "skills", "test-skill", "SKILL.md"); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
}
