# ADR-0001: Classify risk before planning

- Status: proposed
- Date: not recorded until accepted
- Issue: PROJ-101
- Deciders: principal-architect, product-owner

## Context

Plans were being written before changed paths and semantics were classified. That
allowed high-risk authentication and migration changes to flow through the same
approval path as ordinary source edits, and approval evidence did not identify
which policy rule applied.

## Decision

AE runs `risk-assessment` from `principal-architect` after acceptance criteria
are fixed and before `technical-planning`. The resulting `risk-assessment.json`
is an input to the plan and to approval gates. Unknown semantics map to
`unknown` risk and block the run; no approval may be inferred from a missing
artifact.

## Consequences

- Plans bind to a stable risk classification; `plan-digest-change` invalidates approval.
- High-risk paths require the `high-risk` policy and independent review.
- Every run must record observed evidence; missing checks are never marked passed.

## Alternatives considered

1. Classify during review. Rejected: too late to influence the plan and approvals.
2. Let each agent self-classify. Rejected: violates separation of duties and produces inconsistent levels.
3. Skip classification for small changes. Rejected: small diffs can touch authentication paths.

## Approval record

- Architecture gate: pending
- Reviewer: not yet assigned
- Evidence: `evidence.json`, `review-findings.json`
