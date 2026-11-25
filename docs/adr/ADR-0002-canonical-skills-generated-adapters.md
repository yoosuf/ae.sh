# ADR-0002: Canonical skills with generated provider adapters

## Status

Accepted (Phase 0 architecture decision). Phase 1 ships 15 canonical skills and manifests; provider materialization is implemented in Phase 2.

## Context

Skills must be portable across Claude Code and Codex while remaining versioned and auditable. Hand-maintaining copies in each provider's directory risks drift and tampering. Both providers follow the agentskills.io standard with `SKILL.md` files, but their discovery paths differ.

## Decision

Keep exactly one canonical source at `framework/skills/<slug>/SKILL.md`. Each release carries a pinned catalogue version, a manifest of included files, and SHA-256 checksums. Provider copies are generated into documented paths with the manifest/checksum recorded. Generated files are never hand-edited; changes flow canonical → generated. Skills are instruction contracts with inputs, outputs, and failure behavior — not agent "roles".

## Consequences

- Drift is detectable via checksums; installations are reproducible.
- `--dry-run` can show exact writes before any filesystem change.
- A build/generation step is required when skills change.
- Provider path changes require updating the generator, not the skills.

## Alternatives considered

- **Hand-maintained copies:** rejected — drift and integrity risk.
- **Runtime download of skills from a remote registry:** rejected — breaks local-first and offline demo.
- **Embedding skills inside the binary only:** rejected — reduces portability and human reviewability.
