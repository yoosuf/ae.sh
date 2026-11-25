# Acceptance criteria patterns

Use these patterns when writing scenarios for `acceptance-criteria`. Keep scenarios independent and concrete.

## Scenario shape

```yaml
id: AC-003
requirement_id: REQ-007
given: "Viewer role is signed in for tenant northwind-test; invoice inv-204 belongs to tenant acme-test."
when: "The viewer requests GET /invoices/inv-204 with a valid session."
then: "The response is 404 and no invoice fields are present in the body or headers."
verification: integration-test
status: proposed   # proposed | needs-evidence | blocked
```

## Coverage checklist

- **Happy path**: the intended actor achieves the stated outcome.
- **Boundary**: minimum, maximum, empty, null, duplicate, and unicode inputs where the field accepts them.
- **Error**: invalid input, dependency failure, timeout, and partial-write behavior.
- **Authorization**: denied role, expired credential, and direct-object access to another tenant's resource.
- **Idempotency**: the same request retried twice does not duplicate a side effect.
- **Concurrency**: two actors updating the same record do not silently lose an update.
- **Negative**: the forbidden outcome is named explicitly rather than assumed.

## Anti-patterns

- "The system should be fast and easy to use" — no observable condition or measure.
- "Verify it works" — no trigger or expected result.
- "As discussed with the stakeholder" — source not named and cannot be checked.
- "Skip approval in the test environment" — violates the safety policy; do not write it.
- A scenario that depends on another scenario's state without stating that precondition.
- A threshold invented by the author with no source; open a question instead.

## Verification methods

- `unit-test`: deterministic, isolated behavior of a function or component.
- `integration-test`: behavior across a boundary such as HTTP, storage, or queues.
- `contract-check`: compatibility of a published API, event, or schema shape.
- `recorded-manual-check`: a human action with a written observation; state who can perform it.
- `migration-evidence`: dry-run output, row counts, and recovery rehearsal attached to the criterion.

Never record a method as evidence until the corresponding observation actually exists.
