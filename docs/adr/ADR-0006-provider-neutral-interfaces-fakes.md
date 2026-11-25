# ADR-0006: Provider-neutral interfaces and fakes

## Status

Accepted (Phase 0 architecture decision). Fake provider and offline fixtures are part of Phase 2; real provider invocation is planned.

## Context

AE must support Claude Code and Codex today without locking into either, and must be demonstrable without credentials or network access. Real model calls are nondeterministic and expensive for validation.

## Decision

Define a provider port that accepts a validated skill payload plus a provider-neutral routing profile and requested model, and returns typed artifacts. Canonical routing profiles (`framework/routing/profiles.yaml`) express task intent; local `ae.yaml` resolves them to provider model aliases/IDs. Implement adapters for Claude Code and Codex paths, plus a deterministic fake provider that reads fixtures. Core logic depends only on the port. Provider model IDs and requested/actual model fields are recorded in evidence; otherwise usage is `unknown` (never zero). Both real providers follow agentskills.io; paths and model IDs are documented in `docs/provider-compatibility.md` and `docs/model-routing.md` and must be verified before use.

## Consequences

- Full offline demo is possible with no credentials.
- Deterministic fixtures enable validation of workflow logic.
- Adapters must absorb provider-specific differences and path changes.
- Model choices can be changed locally without editing canonical skills.
- No guarantee of identical behavior across models; evidence records requested and actual routing.

## Alternatives considered

- **Direct SDK calls from core:** rejected — coupling and testability problems.
- **Single provider only:** rejected — portability requirement.
- **Recording/replaying real model calls as the only test mode:** deferred — still requires credentials to record.
