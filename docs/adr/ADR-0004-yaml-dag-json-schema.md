# ADR-0004: YAML workflow DAGs with JSON Schema validation

## Status

Accepted (Phase 0 architecture decision). Workflow definitions and validation are part of Phase 2.

## Context

Workflows must be reviewable by humans, machine-checkable before execution, and expressive enough for typed artifacts, dependencies, conditions, gates, and bounded remediation. Encoding them only in Go code would make review and customization difficult.

## Decision

Define workflows as YAML DAGs of typed tasks with explicit dependencies and conditions. Validate them against JSON Schema using `santhosh-tekuri/jsonschema v6` before a run starts. Reject cycles; permit only explicitly bounded remediation/retry loops. Application configuration also uses YAML (`gopkg.in/yaml.v3 v3.0.1`). Named workflows cover feature, bug, security, infrastructure, database, PR review, release, incident, dependency, and refactoring scenarios.

## Consequences

- Workflows are diffable and reviewable in pull requests.
- Invalid graphs fail before any side effects.
- Schema evolution needs versioning and migration rules.
- Expressiveness is intentionally limited; arbitrary code in workflows is not allowed.

## Alternatives considered

- **Hard-coded Go workflow definitions:** rejected — hard to review/customize.
- **Turing-complete DSL:** rejected — cycles and unbounded loops undermine safety.
- **JSON-only definitions:** rejected — less readable for humans.
