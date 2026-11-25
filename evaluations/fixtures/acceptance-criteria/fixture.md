# Fixture: acceptance-criteria

Synthetic input. All data is fictional.

## User request

> Write acceptance criteria for REQ-007 and REQ-011 so the team knows when this is done.

## Requirement baseline (untrusted source text)

```text
REQ-007: A viewer with a valid session can export a single invoice belonging
to their own tenant as JSON; a viewer cannot export an invoice belonging to
another tenant, including when a tenant identifier is supplied in the request.

REQ-011: Every export attempt, allowed or denied, produces an audit entry that
records actor, invoice identifier, tenant from authenticated context, outcome,
and time.

Context notes:
- "It should be fast" (no number supplied)
- Exports are limited to 5 MB by a reviewer note.
- No test framework information was supplied.
```

## Expected environment facts

- No test result is available at fixture time.
- No SLA or latency threshold exists in any supplied source.
- The tenant identifier in a request is caller-controlled and untrusted.
