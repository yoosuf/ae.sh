# Architecture Decision Records (ADRs)

This directory records architecture decisions for the Agentic Engineering. Records are numbered, immutable once accepted, and superseded only by a new ADR that references the old one.

Phase 0 decisions (ADR-0001 through ADR-0010) are recorded as **Accepted** architecture direction for the Phase 0–2 implementation. Acceptance here means "this is the design we are building", not that every item is implemented; see the [command status matrix](../../README.md#command-status-matrix) and [roadmap](../roadmap.md).

| ADR | Title | Status |
| --- | --- | --- |
| [0001](ADR-0001-modular-monolith.md) | Modular monolith with ports and adapters | Accepted (Phase 0) |
| [0002](ADR-0002-canonical-skills-generated-adapters.md) | Canonical skills with generated provider adapters | Accepted (Phase 0) |
| [0003](ADR-0003-sqlite-authoritative-local-state.md) | SQLite as single authoritative local state | Accepted (Phase 0) |
| [0004](ADR-0004-yaml-dag-json-schema.md) | YAML workflow DAGs with JSON Schema validation | Accepted (Phase 0) |
| [0005](ADR-0005-fail-closed-policy-precedence.md) | Fail-closed policy precedence | Accepted (Phase 0) |
| [0006](ADR-0006-provider-neutral-interfaces-fakes.md) | Provider-neutral interfaces and fakes | Accepted (Phase 0) |
| [0007](ADR-0007-content-addressed-evidence.md) | Content-addressed immutable evidence | Accepted (Phase 0) |
| [0008](ADR-0008-no-credentials-in-flags-auth-outside-prompts.md) | No credentials in flags; auth outside prompts | Accepted (Phase 0) |
| [0009](ADR-0009-optional-aws-and-dashboard.md) | Optional AWS adapters and dashboard | Accepted (Phase 0) |
| [0010](ADR-0010-semver-release-provenance.md) | SemVer releases and provenance | Accepted (Phase 0) |

Template: Title, Status, Context, Decision, Consequences, Alternatives considered.
