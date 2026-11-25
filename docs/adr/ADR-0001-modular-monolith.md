# ADR-0001: Modular monolith with ports and adapters

## Status

Accepted (Phase 0 architecture decision). Implementation is partial: core packages and CLI boundaries are in scope for Phase 0–2.

## Context

AE must run on a developer machine without a cluster, daemon, or hosted control plane. It must be extensible to Jira, GitHub, multiple agent providers, and later AWS services without coupling core logic to their SDKs. The team is small and the workflow semantics are still changing.

## Decision

Build a single Go binary as a modular monolith. Core packages (configuration, policy, catalogue, workflow engine, state, evidence) are pure Go and free of provider SDKs. External systems are reached through narrow ports implemented by adapters. The CLI is a thin presentation layer. A modular monolith is acceptable because deployment is local and scaling requirements are per-run, not per-service.

## Consequences

- One build, one binary, simple offline development and debugging.
- Provider changes are isolated behind adapters and can be faked.
- Discipline is required: imports from core to adapter packages must be prevented by review/CI checks.
- Future extraction of a service is possible but not assumed.

## Alternatives considered

- **Microservices:** rejected — operational burden, contradicts local-first goal.
- **Plugin-only architecture with dynamic loading:** deferred — complexity before interfaces stabilize.
- **Script collection:** rejected — weak validation, state, and evidence guarantees.
