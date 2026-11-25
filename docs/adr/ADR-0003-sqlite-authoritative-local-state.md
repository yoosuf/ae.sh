# ADR-0003: SQLite as single authoritative local state

## Status

Accepted (Phase 0 architecture decision). SQLite state storage is part of Phase 2.

## Context

Runs, tasks, artifacts, approvals, and evidence references need durable local storage with transactional guarantees. A CLI cannot rely on a separate database server. Pure-Go builds avoid CGO cross-compilation problems.

## Decision

Use `modernc.org/sqlite v1.38.2` (pure Go) as the single authoritative local state store. State transitions and evidence writes are committed atomically in one transaction. Cloud projections (DynamoDB/S3) are optional downstream copies, never the source of truth. Exports are derived artifacts.

## Consequences

- Atomic transitions are enforceable locally without a network.
- Easy backup by copying a single database file; concurrent CLI processes need locking discipline.
- Schema migrations must be explicit and versioned.
- No multi-user/remote concurrency guarantees; multi-user work is a later phase.

## Alternatives considered

- **JSON files:** rejected — no atomic multi-record transitions or queries.
- **CGO-based SQLite:** rejected — cross-compilation and dependency friction.
- **Embedded key-value store:** rejected — weaker query/model fit.
- **Remote database as source of truth:** rejected — violates local-first.
