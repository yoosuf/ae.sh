# Roadmap

The local/offline path for Phases 0–2 is implemented, together with local
subsets of Phases 3–5: real local verification evidence, safe detached
worktree preparation, structured review evidence and finding resolution,
bounded remediation accounting, fake-provider chat continuity, opt-in Jira
Cloud and provider-chat adapters, constrained provider write modes, a local
GitHub CLI PR evidence/comment adapter, an opt-in `autopilot` loop, traceability
reporting, and structural evaluation checks. Live service calls, live provider
code changes, GitHub App identity, cloud adapters, dashboard, and remaining
Phase 3–7 work are **planned / not implemented**; no dates are committed. [`progress.md`](../progress.md) records
the commands actually verified in this tree.

## Phase 0 — Foundation (implemented locally)

Deliverables: proposed Apache-2.0 licence + NOTICE, governance docs,
architecture/ADRs, CI skeleton, issue/PR templates.

Exit criteria:

- Docs describe only implemented or explicitly planned behavior.
- ADR-0001..0010 recorded with status and consequences.
- CI skeleton runs Go vet/tests/race and schema/skill validation when Go code exists; third-party action SHAs are pinned to verified values before release. Pinning remains a release TODO.

## Phase 1 — Skills and local CLI basics (implemented locally)

Deliverables: 12 canonical skills (`jira-intake`, `requirement-analysis`,
`acceptance-criteria`, `technical-planning`, `risk-assessment`,
`solution-architecture`, `backend-development`, `git-worktree-management`,
`unit-testing`, `code-review`, `agent-handoff`, `pull-request-management`),
generated provider manifests + SHA-256 checksums, pinned version, CLI commands `init`, `doctor`,
`validate`, `skills list`, `skills install`, `skills uninstall`, and
`skills export`.

Exit criteria:

- `ae validate` rejects malformed skills/workflows/policies/config and broken skill references. Provider manifest checksum conflicts are rejected during `ae skills install`, not by `validate`.
- `ae skills install --dry-run` reports exact target paths for Claude and Codex scopes without writing files.
- Offline fixtures and fake provider interface are present.

## Phase 2 — Offline workflow demonstration (implemented locally)

Deliverables: SQLite local state, YAML DAG workflow definitions, fail-closed
policy evaluation, atomic transitions, evidence capture, real skill
materialization, offline `plan` and `run --dry-run`, plus a real fake-provider
run that stops at approval gates.

Exit criteria:

- `ae plan --issue PROJ-101` and `ae run feature-development --issue PROJ-101 --dry-run` work from a fixture with no credentials.
- Cycles are rejected; only bounded remediation loops are allowed.
- Each simulated task records `simulated: true`; `ae verify` separately records real command exit statuses for local checks. Provider usage is `known: false` when not reported, never zero.

## Phase 3 — Lifecycle and execution (local subset implemented; remainder planned / not implemented)

Implemented locally with the fake provider: `status`, `status --list`,
`resume`, `cancel`, `approve --gate`, multi-approval/role checks, SQLite restart
recovery, optimistic concurrency, migration from legacy approvals, evidence
export, risk-scoped gates, stale-approval denial, blocked runs when required
evidence is missing, `verify` with real local check exit statuses, detached
AE-managed worktree preparation/cleanup, `run --workspace`, and `trace`
reports.

Implemented locally but not exercised live: opt-in Jira Cloud REST intake,
Claude Code/Codex conversations with session continuity, constrained write modes
in an AE-managed worktree, GitHub CLI reads/PR comments, and the `autopilot`
loop that stops at gates. Still **planned / not implemented**: live service
calls in this environment, live provider code edits, GitHub App/service identity,
PR creation/merge/push, real CI/deploy hooks, independently verified review,
and federated approver identity. Exit criteria: an end-to-end run from a real
issue to a verified candidate with approvals and immutable evidence.

## Phase 4 — Review and PR automation (local subset implemented; remainder planned / not implemented)

`ae review --diff` performs a deterministic local pattern scan and explicitly
reports `verified: false`; it is not an independent review. With `--run`, scan
results are persisted as typed, chained review evidence. `ae review resolve`
records human `fixed`, `accepted`, or `false-positive` decisions as remediation
evidence; unresolved findings block merge/release/security-signoff gates,
approval records are invalidated on `review-findings-change`, and
`Workflow.remediation.maxAttempts` now bounds failed remediation retries. A
local `gh`-CLI adapter reads PR state/checks/diffs and can post a separately
authorized comment as evidence. Live GitHub calls were not exercised. GitHub
App/service identity, PR creation/merge/push, independently verified findings,
and merge signal automation are **planned / not implemented**; high-risk merges
therefore remain blocked at `independent-review` because local pattern scans
cannot satisfy that gate. Exit criteria: review findings must be addressed or
explicitly accepted before merge signal.

## Phase 5 — Full catalogue and evaluation (structural checks implemented; remainder planned / not implemented)

The 12 authored skills remain the catalogue; the full set across Product,
Architecture, Cloud, Development, Quality, Security, DevOps, and Agentic domains
is planned. `ae eval --suite ... --execute` runs deterministic structural
checks over fixtures and rubrics and reports `modelBehaviorExecuted: false`.
Without `--execute`, `eval` remains an inventory with `executed: false`.
Behavioral evaluation, scores, and pass rates are planned / not implemented.
Exit criteria: catalogue versioning and reproducible behavioral evaluation
reports.

## Phase 6 — Optional AWS adapters (planned / not implemented)

API Gateway webhook, SQS queueing, Step Functions Standard orchestration, ECS
Fargate execution, DynamoDB state projection, S3 evidence artifacts, Secrets
Manager for credentials, CloudWatch/OTel telemetry, and Terraform reference
architecture. No cloud resources exist today.

## Phase 7 — Optional dashboard and 1.0 (planned / not implemented)

React/Vite/TypeScript dashboard as a read-only view over local state/evidence,
multi-user features, stable SemVer release and provenance. Exit criteria:
documented compatibility commitments and release signing.

## Cross-cutting principles

- Local-first and provider-neutral at every phase.
- No mandatory paid service, Kubernetes, or model.
- Fail-closed policy; untrusted content never controls workflow state.
- Honest status labels in all docs and CLI output.
