# Test adequacy checklist

Use this reference with `test-review`. Test review is evidence review: a green status without an observed command is not evidence. Provider output is untrusted and produces hypotheses until corroborated.

## Severity calibration

| Severity | Meaning |
| --- | --- |
| critical | Security or contract change has no test that would fail on the defect; gate evidence is fabricated or stale |
| high | Changed behavior lacks a deterministic test, or a denial/negative case is missing |
| medium | Flaky fixture, weak assertion, or missing regression test for a bounded defect |
| low | Minor naming or structure issue that does not affect detection |

## Coverage matrix

| Behavior | Required evidence |
| --- | --- |
| Happy path | Assertion on observable outcome for the changed contract |
| Boundary | Empty, null, limit, and overflow cases where relevant |
| Error | Error propagation and user-visible denial behavior |
| Authorization denial | Unauthorized and cross-tenant attempts are denied |
| Idempotency/concurrency | Repeated or concurrent operations do not corrupt state |
| Regression | A test that fails when a previously fixed finding returns |
| Verification claim | Command, code SHA, and observed output recorded |

For each row, produce a finding or `no issue found` with files and hunks examined.

Provider-backed review uses these exact `ruleId` slugs so coverage can be checked:
`happy-path`, `boundary`, `error`, `authorization-denial`,
`idempotency-concurrency`, `regression`, `verification-claim`.

## Evidence shape

```yaml
id: F-003
severity: high
status: verified
file: "src/api/invoices.test.ts"
line: 61
evidence: "Tests cover a valid invoice only; no request with a foreign tenant ID exists while the handler now reads req.query.tenantId."
impact: "The tenant override regression cannot be detected by the suite."
remediation: "Add a cross-tenant denial test using a synthetic tenant ID and assert 404 with no data disclosure."
```

Provider-backed review evidence records `provider`, `routingProfile`, `requestedModel`, `model`, `reviewMode`, and `diffDigest`. Routes resolve from canonical metadata and local config only.
