# Technical plan template

Fill every field. Use `not available` when a value cannot be sourced; do not infer completion from a plan document.

```yaml
plan_id: PLAN-004
status: ready            # provisional | ready | superseded
base_sha: "7c1f4a9e..."  # exact SHA; omit only when status is provisional
scope: "Allow tenant-scoped invoice export for viewers."
non_goals:
  - "No changes to the billing provider or hosted infrastructure."
requirement_coverage:
  - requirement_id: REQ-007
    task_ids: [T-01, T-03]
change_surface:
  paths: ["src/api/invoices.ts", "src/core/export.ts"]
  contracts: ["GET /invoices/:id"]
  migrations: []
  discovery_needed: ["How export jobs obtain caller identity"]
tasks:
  - id: T-01
    owner_role: backend-engineer
    owned_paths: ["src/core/export.ts"]
    depends_on: []
    input_artifacts: [AC-003]
    output_artifacts: ["export service change", "unit tests"]
    verification: "unit tests recorded with command and result"
  - id: T-02
    owner_role: backend-engineer
    owned_paths: ["src/api/invoices.ts"]
    depends_on: [T-01]
    output_artifacts: ["handler change", "integration test"]
    verification: "integration test for cross-tenant denial"
ownership_map:
  "src/core/export.ts": T-01
  "src/api/invoices.ts": T-02
verification_gates:
  - gate: unit-tests
    evidence: "command output attached after execution"
  - gate: review
    tier: medium
    requirement: "designated reviewer plus cross-provider review if available"
assumptions:
  - "Export runs in the caller's request context; confirm before T-01."
decisions_required:
  - question: "Which role may approve a contract change?"
    owner_role: domain-owner
integration_steps:
  - "Apply task commits in ascending task id order from the immutable base SHA."
  - "Stop on any conflict; do not discard changes."
  - "Verify the final integrated SHA and record all component SHAs."
```

## Rules

- A `ready` plan requires a base SHA, non-overlapping ownership for parallel tasks, and a named verification for each task.
- Estimates are relative and include a rationale; they are not schedules.
- Gates can be stricter than the risk tier, never weaker.
- This template is a structure only; it asserts no work as complete.
