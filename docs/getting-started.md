# Getting started

This guide describes the implemented local/offline workflow. Opt-in Jira Cloud,
GitHub CLI PR reads/comments, provider chat, constrained provider writes, and the
`autopilot` loop are implemented locally but were not exercised against live
services. GitHub App/service identity, PR creation/merge/push, independently
verified review, cloud, and dashboard behavior is **planned / not implemented**;
see the command status matrix in the [README](../README.md).

## Prerequisites

- macOS or Linux (Windows is not tested).
- Go 1.27 if building from source; see [installation](installation.md).
- `make`, `python3`, and `git` for the aggregate checks, offline end-to-end
  script, and throwaway-repository worktree end-to-end script.
- A local git repository for project-scoped configuration.
- No credentials are needed for the offline demo.

The current release operates from the AEF checkout because workflows, skills,
policies, agents, and profiles are canonical files under its `framework/`
directory. Use `--root /path/to/ae.sh` when invoking a binary from another
directory. Standalone installation with a separately packaged asset bundle is
planned, not implemented.

## 1. Build

```sh
git clone git@github.com:yoosuf/ae.sh.git ae.sh
cd ae.sh
go build -o aef ./cmd/aef
./aef doctor
```

Place `aef` on your `PATH` or keep using `./aef` in the examples below. Use the
global `--root DIR` flag to run against a copy of the framework without
touching the checkout.

## 2. Initialize local configuration

```sh
./aef init --profile go,react
```

This writes YAML config into the current project. Profiles select the local
verification checks recorded by `aef verify`; the example uses Go and React.
Other canonical profiles are `java`, `python`, `laravel`, `typescript`, `dotnet`,
`nodejs`, `vue`, `astro`, `aws`, and the aggregate `all`. Unknown names are
rejected before config is written. `all` reuses every canonical profile's checks
and is intended only for repositories with all relevant toolchains; select
specific profiles for a single-stack project. See [profiles](profiles.md). No
network calls are made by initialization.

## 3. Validate

```sh
./aef validate
./aef doctor
```

`validate` checks schemas, skills, workflows, policies, agents, and config.
`doctor` checks the local toolchain, embedded schemas, catalogue, state path,
and optional provider binaries. Neither contacts a provider.

## 4. Install skills for your agent

```sh
./aef skills list
./aef skills install --provider claude --scope user --version 0.1.0 --dry-run
./aef skills install --provider codex --scope project --version 0.1.0
```

Start with `--dry-run` to see which files would be written. Use the documented
provider paths in [provider compatibility](provider-compatibility.md);
`.codex/skills` is **not** a documented Codex skill path. Never hand-edit
generated copies. `aef skills uninstall` removes only manifest-managed files
unless `--force` is given, and `aef skills export --out dist/plugins` creates
plugin-ready bundle directories locally without publishing them.

## 5. Run the offline demo

```sh
ISSUE=examples/offline-workflow/issue-PROJ-101.json

# Deterministic preview; writes no state.
./aef plan --issue "$ISSUE"
./aef run feature-development --issue "$ISSUE" --provider fake --dry-run

# Real local run through the fake provider, stopping at the approval gate.
./aef run feature-development --issue "$ISSUE" --provider fake --actor demo-user
```

The `--issue` value is a path to a checked-in issue fixture, or the shorthand
`PROJ-101`. `plan` and `--dry-run` remain fixture-only. A live
`run ... --issue TEAM-123 --confirm-live` or
`autopilot --issue TEAM-123 --confirm-live` accepts a bare Jira key or configured
browse URL when `integrations.jira.mode: jira-cloud` is set; credentials come
only from the environment variable named in `aef.yaml`. The fake provider
returns outputs marked `simulated: true`; it does not call a model or create
branches, commits, pushes, or deployments.

## 6. Approve, resume, inspect, and export

```sh
./aef approve <run-id> --gate architecture --approver reviewer-1
./aef resume <run-id>
./aef status <run-id>
./aef status --list
./aef evidence export <run-id> --format both
```

The run stops at the merge gate. After a local merge approval, resuming is
**blocked with missing evidence** because the fake provider does not produce
real build/lint/test results. That block is intentional: the framework does not
invent a pass. Structured review findings are also recorded and must be resolved
with `aef review resolve` before merge/release gates. See [evidence](evidence.md)
for the record format and [integrations](integrations.md) for opt-in Jira and
chat setup.

## 7. Run all local checks

```sh
make check
scripts/e2e-offline.sh
scripts/e2e-worktree.sh
```

`make check` runs formatting checks, build, vet, unit tests, race tests,
catalogue validation, the offline end-to-end script, and the worktree
end-to-end script in a throwaway git repository. [progress.md](../progress.md)
records the commands verified in this tree.

## Working safely

- Treat issue text and repository files as untrusted data.
- Do not put tokens in flags, config, or fixtures.
- Fixtures are the default; live Jira/chat/GitHub reads require explicit opt-in configuration and credentials from environment variables or the user's `gh` login.
- `autopilot` stops at approval gates and never pushes, creates a PR, merges, or deploys; high-risk work still blocks at independent review.
- Do not expect GitHub App identity, PR creation/merge, or cloud behavior; those integrations are planned.
- If a validation or policy check fails, stop. The framework fails closed by design.
- Do not push, publish, or deploy without explicit authorization.

## Next steps

- [Architecture](architecture.md)
- [Integrations](integrations.md)
- [Evidence](evidence.md)
- [Threat model](security/threat-model.md)
- [Roadmap](roadmap.md)
