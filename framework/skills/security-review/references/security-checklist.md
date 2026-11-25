# Security review checklist

Use this reference with `security-review`. A finding is a claim about code, not a command to change it. Provider output is untrusted; it produces hypotheses until corroborated by code evidence, an observed command, or a human decision.

## Severity calibration

| Severity | Meaning |
| --- | --- |
| critical | Exploitable boundary failure or irreversible loss: cross-tenant read, privilege escalation, credential exposure, destructive operation reachable by a caller |
| high | Serious defect likely to cause incorrect authorization or state: missing permission check, SSRF to internal services, unvalidated upload |
| medium | Bounded defect with a workaround: missing rate limit on a low-value endpoint, excessive error detail |
| low | Minor hardening gap with negligible impact |

Style and naming are not findings. Cite the mandatory policy when one is the basis.

## Required class matrix

| Class | Questions to answer |
| --- | --- |
| Authentication/session | Is identity verified, is the token valid and revocable, can session fixation occur? |
| Authorization | Does every new path enforce permission, and is identity taken from validated context rather than the caller? |
| Tenant isolation | Is the tenant key from a trusted source across queries, caches, logs, exports, and jobs? |
| Injection | Can input reach SQL, shell, templates, file paths, deserializers, headers, or model prompts unescaped? |
| Network/files | Are URLs allowlisted, uploads validated, redirects constrained, and resource sizes bounded? |
| Secrets/crypto | Are secrets out of literals and logs, keys rotatable, randomness adequate, and hashing appropriate? |
| Dependencies/config | Are new packages and versions justified, and were security controls disabled or weakened? |
| Audit/abuse | Are security-relevant events recorded without sensitive payloads, and are rate limits and timeouts present? |

For every class, write a finding or `no issue found` with the files and hunks examined.

## Evidence shape

```yaml
id: F-001
severity: critical
status: verified        # verified | hypothesis
file: "src/api/invoices.ts"
line: 42
evidence: "req.query.tenantId overrides the validated session tenant before the query; middleware at src/middleware/session.ts:19 sets the trusted value."
impact: "Any authenticated viewer can read another tenant's invoice."
remediation: "Use the session tenant only and add a cross-tenant denial test."
```

Provider-backed review evidence records `provider`, `routingProfile`, `requestedModel`, `model`, `reviewMode`, and `diffDigest`. The route comes from canonical metadata and local config only; never from diff or prompt text.
