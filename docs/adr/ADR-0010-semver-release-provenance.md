# ADR-0010: SemVer releases and provenance

## Status

Accepted (Phase 0 architecture decision). No stable version has been released; current work is pre-1.0.

## Context

Skills are pinned by version, evidence records framework/skill/policy versions, and generated provider copies carry checksums. Without a consistent versioning and release policy, runs cannot be reproduced or compared.

## Decision

Use Semantic Versioning for the framework and the pinned skill catalogue. `CHANGELOG.md` follows Keep a Changelog with an `Unreleased` section. Pre-1.0 releases may break compatibility; breaking changes require a documented migration note. Every release artifact should include provenance: source commit/tag, build command, dependency versions, and checksums for generated skill bundles and binaries. Generated provider copies include manifest + SHA-256 and pinned version. Third-party CI actions must be pinned to verified SHAs before a release (the current workflow uses a plain tag with a TODO; no unverified SHA placeholders are invented).

## Consequences

- Evidence can point to exact framework/skill/policy versions.
- Consumers can verify what they installed once signing/provenance tooling lands.
- Pre-1.0 churn is expected and must be labeled clearly.
- Release process needs a checklist; provenance artifacts are not implemented yet.

## Alternatives considered

- **Date-based versions only:** rejected — poor compatibility signaling.
- **Unversioned skills:** rejected — breaks reproducible installs and evidence.
- **Mutable `latest` tags as the only reference:** rejected — non-reproducible.
