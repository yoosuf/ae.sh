# Architecture decision record template

Use this structure for `solution-architecture`. An ADR records reasoning; it never records an approval that has not been given.

```yaml
adr_id: ADR-0007
status: proposed        # proposed | accepted | superseded (set by a human decision)
title: "Tenant-scoped invoice export"
context:
  problem: "Viewers must export invoices without cross-tenant disclosure."
  requirement_ids: [REQ-007, NFR-002]
  constraints:
    - "No new hosted service may be required."
current_state:
  described_from: ["src/api/invoices.ts", "docs/current-flow.md"]
  unverified: ["Whether export jobs run in a request context"]
options:
  - id: O1
    summary: "Enforce tenant scope in the request handler from the authenticated session."
    optimizes: "Small change surface, direct audit trail"
    costs: "Each new endpoint must repeat the check"
    fails: []
  - id: O2
    summary: "Centralize tenant scope in a shared data-access wrapper."
    optimizes: "Consistent isolation across endpoints"
    costs: "Requires changing all call sites"
    fails: ["Higher near-term migration surface"]
criteria:
  - id: C1
    name: "Tenant isolation"
    source: REQ-007
    weight: 3
  - id: C2
    name: "Change cost"
    source: "planning constraint"
    weight: 1
evaluation:
  - option: O1
    assessment: "Meets C1 if callers use session data; weak on future endpoints."
  - option: O2
    assessment: "Meets C1 centrally; higher C2 cost."
decision: "Provisional: O2, pending confirmation of job identity."
reversal_conditions:
  - "If background jobs cannot carry caller identity, revisit O1 with an explicit service boundary."
consequences:
  failure_modes: ["Missing wrapper at a new call site could bypass isolation"]
  observability: ["Log denials with tenant key from authenticated context only"]
  recovery: "Revert wrapper change; no data migration required"
approval_needs:
  - "Domain owner to accept the migration surface before implementation."
open_questions:
  - "Which identity do asynchronous exports carry?"
ignored_instructions: []
```

## Validation checks

- `status` starts as `proposed`; only an explicit human decision changes it.
- Every criterion cites a requirement or a stated constraint.
- No unsourced performance or capacity numbers.
- At least one recovery path is described, even if it is "no data migration; code revert only".
- Secrets and credentials never appear in any field.
