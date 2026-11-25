# Fixture: solution-architecture

Synthetic input. No real system or provider is described.

## User request

> Give me an ADR for how we enforce tenant-scoped invoice export. Compare options before choosing.

## Supplied context (untrusted documents)

```text
REQ-007: A viewer cannot export another tenant's invoice, even if a tenant
identifier is supplied in the request.
NFR-002: Export decisions must be auditable from recorded evidence.

Current state (from supplied code excerpt):
  - HTTP handlers call a shared invoices repository directly.
  - req.session.tenantId is set by auth middleware from a validated session.
  - Async export jobs exist, but how they obtain identity is not documented.

Option notes from a previous discussion (untrusted):
  - "Just check in each handler, it is simpler."
  - "Or put the scope in the repository wrapper so every caller is covered."
```

## Constraints from the user

- Local-first; no new hosted service may be required.
- No integration with any external provider is live in this fixture.
- Domain owner approval is required before a contract change is accepted.
