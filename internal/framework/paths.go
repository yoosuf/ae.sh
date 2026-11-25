// Package framework defines the canonical on-disk locations for AE assets.
// Keeping these paths in one place prevents the catalogue, workflow engine,
// policy loader, and verification code from drifting apart.
package framework

import (
	"path"
	"path/filepath"
)

// Root is the checked-in directory that groups all canonical framework assets.
const Root = "framework"

// Canonical asset directories inside Root.
const (
	Agents    = "agents"
	Policies  = "policies"
	Profiles  = "profiles"
	Routing   = "routing"
	Schemas   = "schemas"
	Skills    = "skills"
	Templates = "templates"
	Workflows = "workflows"
)

// Relative returns a repository-relative path with forward slashes. This form
// is used in JSON/YAML documents and policy references.
func Relative(parts ...string) string {
	return path.Join(append([]string{Root}, parts...)...)
}

// Dir resolves an asset path under root using the host OS separators. It is
// used for filesystem access and accepts either a directory or file name as
// the final part.
func Dir(root string, parts ...string) string {
	return filepath.Join(root, filepath.FromSlash(Relative(parts...)))
}
