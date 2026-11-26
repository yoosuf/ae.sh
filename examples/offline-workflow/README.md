# Offline workflow demo

Run the full `feature-development` graph against a local issue fixture. No
network, provider credentials, approvals, or deployments are involved.

From the repository root, with `ae` on `PATH`:

```sh
ae init --profile go
ae run feature-development --issue examples/offline-workflow/issue-PROJ-101.json --dry-run
```

The `--issue` flag accepts a **path** to the JSON fixture in this directory or
the shorthand `PROJ-101`; it does not require a live Jira connection. If the CLI
has not been installed yet, use `go run ./cmd/ae` in place of `ae` from the
repository root. See [integration setup](../../docs/integration-setup.md) for
all opt-in integrations.

Expected behavior in dry-run mode:

- The fixture at `examples/offline-workflow/issue-PROJ-101.json` is parsed as data, not instructions.
- The workflow creates a proposed `issue-brief.json`, `risk-assessment.json`, and `plan.json`.
- Checks are reported as `not-run` in `evidence.json`; dry-run never claims executed tests.
- Approval gates are recorded as pending, never auto-approved.
- Because `deploymentScope` is `optional`, no deployment or production evidence is produced.

If deployment is out of scope, the run finishes at the configured PR/merge
terminal state; it does not invent production evidence.
