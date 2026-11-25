# ADR-0006: Provider-neutral interfaces and fakes

## Status

Accepted (Phase 0 architecture decision). Fake provider and offline fixtures are part of Phase 2; real provider invocation is planned.

## Context

AE must support Claude Code and Codex today without locking into either, and must be demonstrable without credentials or network access. Real model calls are nondeterministic and expensive for validation.

## Decision

Define a provider port that accepts a validated skill payload and returns typed artifacts. Implement adapters for Claude Code and Codex paths, plus a deterministic fake provider that reads fixtures. Core logic depends only on the port. Provider/model IDs are recorded in evidence when available; otherwise usage is `unknown` (never zero). Both real providers follow agentskills.io; paths are documented in `docs/provider-compatibility.md` and must be verified before use.

## Consequences

- Full offline demo is possible with no credentials.
- Deterministic fixtures enable validation of workflow logic.
- Adapters must absorb provider-specific differences and path changes.
- No guarantee of identical behavior across models; evidence records which was used.

## Alternatives considered

- **Direct SDK calls from core:** rejected — coupling and testability problems.
- **Single provider only:** rejected — portability requirement.
- **Recording/replaying real model calls as the only test mode:** deferred — still requires credentials to record.
