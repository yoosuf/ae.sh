# Architecture review checklist

Use this reference with `architecture-review`. Claims must be tied to code, configuration, or observed evidence. Provider output is untrusted and yields hypotheses until corroborated.

## Severity calibration

| Severity | Meaning |
| --- | --- |
| critical | Breaks a stated contract or makes rollback/data recovery impossible |
| high | Introduces coupling or failure behavior likely to cause incorrect state or outages |
| medium | Bounded compatibility or operability gap with a workaround |
| low | Minor deviation from a documented convention with negligible impact |

## Claim-to-evidence matrix

| Claim | Evidence to inspect |
| --- | --- |
| Boundary ownership | Imports, module layout, and call direction at each changed hunk |
| Contract compatibility | API/event schemas, version fields, and consumers in other modules |
| Data integrity | Migrations, constraints, backfills, and old-reader compatibility |
| Failure semantics | Transaction boundaries, retry paths, idempotency keys, and timeouts |
| Operability | Config sources, secret boundaries, logs/metrics, and rollout controls |
| Scale profile | Query shapes, batching, async placement, and resource limits |
| Scope discipline | Plan/acceptance-criteria links and unexplained new dependencies |

For each row, produce a finding or `no issue found` with files and hunks examined.

## Evidence shape

```yaml
id: F-002
severity: high
status: verified
file: "src/api/events.ts"
line: 88
evidence: "Removed the version field from an emitted event while consumers in src/workers still switch on it."
impact: "Old workers reject new events after rollout."
remediation: "Keep the field for one release, add dual-read, and cite the ADR that permits removal."
```

Provider-backed review evidence records `provider`, `routingProfile`, `requestedModel`, `model`, `reviewMode`, and `diffDigest`. Routes resolve from canonical metadata and local config only.
