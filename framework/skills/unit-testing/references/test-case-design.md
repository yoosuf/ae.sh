# Unit test case design

Use this matrix to design cases for `unit-testing`. It lists intentions, not outcomes.

## Case matrix

| Intention | Typical input | Assertion focus |
| --- | --- | --- |
| Happy path | Valid, representative request | Expected value and side effect |
| Lower/upper boundary | Minimum and maximum accepted values | Accepted at boundary, rejected beyond it |
| Empty/null | Empty collection, missing optional field | Defined behavior, no crash, no leaking of internals |
| Invalid input | Wrong type, malformed format | Typed error, no partial mutation |
| Authorization denial | Insufficient role or expired credential | Denial response, no privileged side effect |
| Tenant isolation | Object belonging to another tenant | Denial indistinguishable from not-found where required |
| Idempotency | Same request retried | Side effect occurs once, same logical result |
| Concurrency | Two overlapping updates | No lost update under the declared strategy |

Only write cases relevant to the behavior and risk tier. Record skipped intentions with a reason.

## Fixture rules

- Use clearly fictional identifiers and domains such as `tenant-test-a` and `example.invalid`.
- Freeze time and randomness at the test boundary; never sleep to wait for deterministic state.
- Avoid real network calls; use the repository's existing test doubles if present.
- Never commit a captured token, cookie, export, or customer record, even in a comment.

## Assertion quality

- Assert what the caller can observe: return value, status, persisted state, or emitted event.
- One logical outcome per test; multiple related assertions are acceptable only when they verify the same outcome.
- Prefer a precise expected value over `not null` or `truthy` checks.
- A regression test for a bug names the bug scenario, not the internal function that failed.

## Reporting results

Record the exact command and tool output. Example record shape:

```yaml
verification:
  command: "<repository test command>"
  code_sha: "91ac3f0d..."
  result: "observed outcome, including counts if the tool printed them"
  coverage: "tool-reported string or not available"
  not_run: ["integration suite - environment unavailable"]
```

Never convert a written test into a claimed pass. The log describes what was observed, nothing more.
