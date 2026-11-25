# ADR-0007: Content-addressed immutable evidence

## Status

Accepted (Phase 0 architecture decision). Evidence capture, local verification evidence, and JSON/Markdown export are implemented; signed bundles and cloud storage remain planned.

## Context

Engineering claims ("tests passed", "approved by X", "deployed to Y") must be verifiable after the fact. Mutable logs can be rewritten accidentally or intentionally. Evidence must survive across local runs and later cloud projections.

## Decision

Write evidence as content-addressed, immutable records keyed by a hash of canonical `data` bytes. Each record includes run ID, sequence, timestamp, kind, subject, sanitized `data`, data digest, optional previous digest, actor, real exit status when a command ran, and usage with `known: false` when unavailable. Run/issue/SHA/version context is written in the `plan` record; provider name, routing profile, requested model, actual model when observable, and workspace are written in provider task records; approval identity/role/digests are written in `approval` records; local verification commands and exit statuses are written in `test` records by `ae verify`. Transitions and evidence are written atomically where the store supports it. Exports render JSON and Markdown views; corrections are new appended records. Live CI/deploy refs and signed bundles remain planned.

## Consequences

- Tampering is detectable; records are reproducible from the local store.
- More storage and hashing overhead than plain logs.
- Redaction must happen before hashing; sensitive data must never enter evidence.
- Export tooling needs to resolve content hashes to readable documents.

## Alternatives considered

- **Append-only plaintext log:** rejected — weaker integrity and query support.
- **Database rows only, mutable:** rejected — no tamper evidence.
- **External audit SaaS as source of truth:** rejected — local-first requirement.
