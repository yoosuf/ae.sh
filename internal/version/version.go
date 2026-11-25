// Package version holds the framework version and compatibility pins.
package version

// Version is the current AEF framework version. Provider materialization
// embeds this value in manifests and skill metadata so upgrades are auditable.
const Version = "0.1.0"

// SchemaVersion is the version of the canonical JSON Schema contract.
const SchemaVersion = "1.0"

// SupportedSkillsVersion is the pinned canonical skill catalogue revision.
const SupportedSkillsVersion = "0.1.0"

// Pinned toolchain and library versions verified while authoring this release.
// See docs/provider-compatibility.md for sources and re-verification notes.
const (
	GoMin               = "1.27"
	SQLiteDriver        = "modernc.org/sqlite v1.38.2"
	JSONSchemaLibrary   = "github.com/santhosh-tekuri/jsonschema/v6 v6.0.1"
	YAMLLibrary         = "gopkg.in/yaml.v3 v3.0.1"
	AgentSkillsStandard = "agentskills.io specification (retrieved 2026-11-25)"
	ClaudeSkillsDoc     = "https://code.claude.com/docs/en/skills.md"
	CodexSkillsDoc      = "https://learn.chatgpt.com/docs/build-skills.md"
)
