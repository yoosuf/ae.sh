# ADR-0005: Fail-closed policy precedence

## Status

Accepted (Phase 0 architecture decision). Policy evaluation and local approval identity capture are implemented; a federated identity config boundary is validated locally but fails closed, and a live verifier remains planned.

## Context

AE processes untrusted issue text, repository content, model output, and MCP responses. Agent instructions in those inputs must not be able to weaken safety controls, bypass approvals, or redirect deployments. Policy sources can conflict (org vs repo vs workflow vs agent).

## Decision

Enforce a fixed precedence: immutable runtime safety > org policy > repo policy > workflow defaults > agent instructions > untrusted task content. Any conflict, unknown field, missing required policy, or validation error fails closed — the transition is rejected and evidence records the reason. Risk is derived from changed paths and semantics. Escalation categories requiring approval: auth, billing, tenant isolation, IAM, secrets, infrastructure, migrations, data retention, public API.

## Consequences

- Untrusted content cannot alter control flow or policy.
- Operators get predictable behavior under ambiguity: stop, not guess.
- Policy authoring needs care; overly strict rules block runs (visible in evidence).
- Runtime safety rules require code changes, not config, to alter.

## Alternatives considered

- **Last-writer-wins / merge semantics:** rejected — untrusted input could win.
- **Warn-and-continue:** rejected — unsafe defaults under ambiguity.
- **Policy evaluated only at start:** rejected — context changes during a run.
