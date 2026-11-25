# Backend change contract checklist

Use this checklist before declaring a backend change complete. It describes checks, not results.

## Compatibility

- List every published surface touched: HTTP routes, event payloads, CLI flags, persisted records, and shared library functions.
- For each surface, state whether the change is additive, tolerant, or breaking.
- Breaking changes require an approved migration plan and a versioning or rollout strategy; otherwise keep a compatibility shim.
- Serialization changes must account for old records still in storage and for readers that have not been upgraded.

## Identity and tenancy

- Tenant scope is derived from validated session or token claims, not from query strings, path parameters, or request bodies supplied by the caller.
- Direct-object access re-checks entitlement on every request, including exports, background jobs, and cached reads.
- Cache keys, log fields, and job payloads include tenant identity only when the source is authenticated context.
- Denials return a consistent response that does not reveal whether the object exists in another tenant.

## State and retries

- Identify which operations are side effects: writes, sends, billing events, and external calls.
- State the idempotency key or natural idempotency argument for retried operations.
- Describe the transaction boundary and what happens on partial failure.
- Note concurrent updates to the same record and the chosen conflict behavior (optimistic version, lock, or last-writer with justification).

## Data migration

- Provide a dry-run procedure and observed evidence before applying irreversible changes.
- State backward compatibility for old and new schema during rollout.
- Record row counts or invariants checked before and after, using synthetic or authorized data only.
- Document recovery: what can be restored, by whom, and with which verified artifact.

## Observability and privacy

- New failure modes get a named signal that does not include credentials, full payloads, or personal data.
- Error paths do not expose stack traces or internal hostnames to callers.
- Test fixtures are synthetic; no production export is pasted into a test or report.

Record each check as satisfied with evidence, not satisfied, or not applicable with a reason. Never infer a pass from the absence of an error.
