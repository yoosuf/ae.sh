# Evidence

Evidence is the audit trail that turns a run into a reviewable engineering
record. Local capture, `ae verify`, local verification commands, and
JSON/Markdown export are implemented. Live CI/deploy evidence, signed bundles,
and cloud artifact storage are **planned / not implemented**.

## Principles

- **Content-addressed and immutable:** each record has a `dataDigest` over its
  canonical `data` object. Never rewrite history; append corrections as new
  records.
- **Actual results:** `ae verify` records the command line, sanitized output,
  and real `exitStatus`. Provider output is marked `simulated: true` when the
  fake provider produced it; it is never treated as a real check pass.
- **Sanitized:** strings are redacted before storage. Synthetic fixture markers
  such as `EXAMPLE` are still redacted but are ignored by the built-in secret
  scan.
- **Explicit execution:** `ae verify` runs commands only from checked-in
  profiles, which are executable configuration. Review profiles before use;
  issue text and CLI flags cannot supply verification commands.
- **Scoped:** each record has a `runID`, `sequence`, `timestamp`, `kind`, and
  `subject`; approval records bind an identity to the run's candidate SHA and
  plan digest.
- **Fail closed:** a required gate without passing evidence parks the run with
  `status: blocked` and an explicit reason.

## Implemented record shape

`internal/evidence.Record` emits these fields:

| Field | Description |
| --- | --- |
| `schemaVersion` | Evidence schema version (`1.0`). |
| `recordID` | Unique record identifier. |
| `runID` | Run this record belongs to. |
| `sequence` | Monotonic position within the run. |
| `timestamp` | UTC creation time. |
| `kind` | `input`, `plan`, `risk`, `approval`, `provider`, `test`, `review`, `remediation`, `gate`, `artifact`, `transition`, `merge`, `deployment`, or `note`. |
| `subject` | Human-readable subject, often the task ID or gate name. |
| `data` | Sanitized, task-specific object. Verification records include `gate`, `command`, `passed`, `simulated`, and `output`; provider records include `skill`, `agent`, `provider`, `model`, `workspace`, `output`, and `simulated`. Chat records add `turn`, `prompt`, `response`, and `sessionId`. Review records contain `diffDigest`, `findings`, `checksWithoutFindings`, and `verified: false`; remediation records contain `findingId`, `status`, `reason`, and the originating review evidence ID. |
| `dataDigest` | `sha256:` digest of canonical `data`. |
| `previousDigest` | Optional link to the preceding record's data digest. |
| `actor` | Local actor or `ae verify` / `ae worktree`. |
| `exitStatus` | Real process exit status when a command ran; omitted when no command was run. |
| `usage` | `known: false` when a provider does not report token/cost data; never zero-by-default. |

Run metadata (issue key, base/candidate SHA, plan digest, profiles, and workflow
version) is written in the `plan` record's `data`. Approval identity, role,
candidate SHA, and artifact digest are written in `approval` records and the
SQLite approvals table.

## Lifecycle

1. A transition is requested by the workflow engine.
2. Policy is evaluated against current risk, approvals, and gate evidence; it
   fails closed on conflict, unknown semantics, missing evidence, or stale
   approval.
3. The content-addressed record file is written locally, then the transition
   and evidence reference are appended in one SQLite transaction where the
   store supports it.
4. `ae evidence export <run-id> --format json|markdown|both [--out dir]`
   renders stored records. The default directory is
   `.ae/evidence/export/<run-id>`.
5. `ae trace <run-id>` reconstructs requirement → architecture → implementation
   → verification → review → release → feedback coverage from persisted
   records. Stages without records are reported as `missing`.

## Illustrative verification record

```json
{
  "schemaVersion": "1.0",
  "recordID": "ev-example",
  "runID": "run-example",
  "sequence": 12,
  "timestamp": "2026-11-25T00:00:00Z",
  "kind": "test",
  "subject": "Verify unit-tests",
  "data": {
    "gate": "unit-tests",
    "command": "go test ./...",
    "passed": true,
    "simulated": false,
    "output": "ok  github.com/yoosuf/ae.sh/internal/execution",
    "profile": "go"
  },
  "dataDigest": "sha256:...",
  "actor": "ae verify",
  "exitStatus": 0
}
```

This is an illustration of the implemented shape, not a record copied from a
real run.

## Fake-provider boundary

Fake-provider output contains `simulated: true` and `usage.known: false`. It
does not run shell commands, invoke models, or create commits. Required
build/lint/test evidence is therefore not produced by the fake provider; the
merge gate blocks with `missing required evidence` until `ae verify` records
real local results. See [`progress.md`](../progress.md) for exact observed
behavior.

## What is not implemented

- Signed/notarized evidence bundles, external transparency logs, and cloud
  artifact storage (planned Phase 6).
- Federated approver identity and PR-linked authorization; local identity/role
  capture is implemented.
- Live CI/deploy references and independently verified review findings. Local review findings are hypotheses; `review resolve` records human decisions without claiming independent verification.
