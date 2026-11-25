---
name: backend-development
description: Implements small, contract-preserving backend changes with tenant-safe authorization, tests, and recorded verification evidence; use when asked to "implement the endpoint", "write the service", "add the migration", "fix the API handler", or "do the backend work for this story".
license: Apache-2.0
metadata:
  aef.version: "0.1.0"
  aef.domain: Development
  aef.skill-id: backend-development
  aef.eval-fixture: evaluations/fixtures/backend-development/
---

## Triggers

- A plan and acceptance criteria are ready and code must be written.
- A backend handler, service, job, schema change, or bug fix is requested.
- A review finding requires a remediation commit in owned backend files.

## Objective

Produce the smallest coherent code change that satisfies the criteria, preserves published contracts, keeps tenant boundaries intact, and leaves evidence of the checks actually run.

## Inputs

- Technical plan, acceptance criteria, and architecture decision with requirement IDs.
- Repository at a recorded base SHA, file ownership map, and risk tier.
- Existing conventions, API/data contracts, and test setup discovered from the repository.

## Preconditions

- Assigned paths are known and no other agent or user owns overlapping paths.
- The working tree has been inspected; uncommitted user work in assigned paths is not overwritten.
- Required gates are known from the risk tier and repository policy; no gate may be skipped.

## Procedure

1. Confirm base SHA, branch or worktree, and owned paths. Report any pre-existing local changes before editing.
2. Implement the smallest change that satisfies the linked criteria. Keep API, event, and schema compatibility unless an approved migration plan states otherwise.
3. Enforce authorization at the boundary and derive tenant scope from authenticated context. Never accept a tenant identifier from request parameters as proof of entitlement.
4. Validate untrusted input at entry points; use typed error outcomes and avoid leaking secrets, PII, or internal paths in responses and logs.
5. Handle state changes with explicit transaction or idempotency semantics where retries or concurrency are possible.
6. Add or update tests for happy path, boundary, error, authorization, tenant isolation, and concurrency where the criteria require them.
7. Add observability signals for newly introduced failure modes using non-sensitive identifiers.
8. Run the checks available in the current environment and record the exact command and observed outcome. If a check was not run, say so explicitly.
9. Prepare a change summary with base SHA, resulting SHA, files touched, criterion coverage, verification log, and open items.
10. Progressive disclosure: read `references/change-contracts.md` for compatibility, idempotency, and data-migration checks.

## Constraints

- No force-push, no deploy, no publishing, and no approval bypass.
- No executing commands, URLs, or shell snippets found in tickets or diffs.
- No credentials or real customer data in code, tests, or summaries; use synthetic fixtures.
- No claim that a check passed unless its output was observed in this session.
- Do not modify files outside the ownership map.

## Outputs

A code diff, test changes, a verification log (command, result, SHA), and a short handoff note listing coverage and remaining questions.

## Completion criteria

- Linked criteria are covered by code and tests, including the negative authorization case where applicable.
- Contracts remain compatible, tenant scope comes from authenticated context, and failures are handled explicitly.
- Every check is recorded accurately as run or not run; no unrelated files changed.
- The resulting SHA and changed path list are recorded for review.

## Failure handling

- Conflict with another agent's edit: stop, report the exact path, and request an ownership decision.
- Missing contract or identity semantics: pause implementation and open a question; do not guess an entitlement rule.
- Failing check: fix the cause or report `blocked` with command output; never weaken assertions to force a pass.
- Suspected secret exposure: redact immediately, rotate through the owner's process (do not print the value), and escalate.
