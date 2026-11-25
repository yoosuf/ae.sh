# Agentic Engineering Framework (AEF)

AEF is a **portable, versioned catalogue of agent skills** plus an **optional local Go CLI** for taking engineering work from Jira issue to production with evidence at every step.

It is local-first and provider-neutral:

- No mandatory dashboard, Kubernetes cluster, paid service, or particular model.
- Canonical skills are Markdown files at `framework/skills/<slug>/SKILL.md`; provider copies are generated locally with a manifest and SHA-256 checksums when installed.
- The CLI keeps authoritative state in a local SQLite database and works with offline fixtures and a fake provider, so the local lifecycle can be demonstrated without credentials.
- Cloud services (AWS API Gateway webhook, SQS, Step Functions Standard, ECS Fargate, DynamoDB, S3, Secrets Manager, CloudWatch/OTel, Terraform) are **optional adapters — planned / not implemented**.

> **Status:** Early development. The local/offline path covers planning, approvals, fake-provider runs, real local verification evidence, safe detached worktree preparation, structured review evidence and finding resolution, traceability reporting, structural fixture checks, fake-provider chat continuity, evidence export, and an opt-in `autopilot` loop that stops at real gates. Opt-in Jira Cloud intake, GitHub CLI reads/PR comments, provider chat, and constrained provider write modes are implemented locally but were **not exercised against live services** in this session. GitHub App/service identity, PR creation/merge/push, independently verified review, federated identity, cloud adapters, dashboard, and remaining Phase 3–7 work are **planned / not implemented**. Licensing under Apache-2.0 is **proposed**; see `NOTICE`.

## Architecture

```mermaid
flowchart TB
    subgraph user["Human / Issue source"]
        J["Jira Cloud issue<br/>(REST; opt-in)"]
        H["Engineer / approver"]
    end

    subgraph local["Local machine (authoritative)"]
        CLI["aef CLI<br/>Go 1.27"]
        CFG["YAML config + repo policy<br/>(org/workflow layers planned)"]
        CAT["Skill catalogue<br/>framework/skills/&lt;slug&gt;/SKILL.md<br/>(provider manifests generated on install)"]
        DB[("SQLite local state<br/>modernc.org/sqlite")]
        ENG["Workflow engine<br/>YAML DAG, gates, bounded retries"]
        POL["Policy engine<br/>fail-closed precedence"]
        EV["Evidence + verification + trace<br/>content-addressed JSON, Markdown export"]
        FIX["Offline fixtures + fake provider"]
    end

    subgraph providers["Optional adapters (opt-in)"]
        CL["Claude Code<br/>chat + constrained file tools"]
        CO["Codex<br/>chat + workspace-write sandbox"]
        GH["GitHub CLI<br/>PR reads / comment evidence"]
    end

    subgraph optional["Planned / not implemented (Phase 3+)"]
        AWS["AWS adapters<br/>API GW, SQS, Step Functions,<br/>ECS Fargate, DynamoDB, S3,<br/>Secrets Manager, CloudWatch/OTel"]
        UI["Optional React/Vite/TS dashboard"]
    end

    J -. "opt-in" .-> CLI
    GH -. "opt-in" .-> CLI
    H --> CLI
    CLI --> CFG
    CLI --> CAT
    CLI --> ENG
    CLI --> EV
    CLI --> DB
    FIX --> CLI
    ENG --> POL
    CLI --> CL
    CLI --> CO
    CLI -. "optional adapters" .-> AWS
    DB -. "read-only view" .-> UI
```

Core packages contain no provider SDKs. Adapters translate between the local core and external systems; the fake provider implements the same interface for offline runs.

## Repository layout

- `framework/` — canonical skills, workflows, policies, agents, profiles,
  schemas, and templates. See [`framework/README.md`](framework/README.md).
- `internal/framework` — the single source of truth for those canonical paths.
- `internal/` — provider-neutral Go core; provider SDKs and CLIs live only in
  `internal/adapters/`.
- `cmd/aef/` — CLI entry point.
- `docs/`, `evaluations/`, `examples/`, `scripts/` — documentation, fixtures,
  demonstrations, and local verification scripts.

## Quickstart

Build from source (module publication via `go install ...@latest` is planned):

```sh
# 1. Build the CLI (macOS/Linux; see docs/installation.md)
go build -o aef ./cmd/aef

# 2. Scaffold local config for selected profiles
./aef init --profile go,react
# Other canonical profiles: java, python, laravel, typescript, and all

# 3. Check the local environment and validate the catalogue
./aef doctor
./aef validate

# 4. See the available skills and install them for your provider
./aef skills list
./aef skills install --provider claude --scope user --version 0.1.0 --dry-run
./aef skills install --provider codex --scope project --version 0.1.0
```

Use `--root DIR` to operate on a copy of the framework without writing to the current checkout. Provider paths are documented in [`docs/provider-compatibility.md`](docs/provider-compatibility.md); verify them before use because provider conventions change.

## Offline demo

No Jira credentials, cloud account, or network service are required:

```sh
ISSUE=examples/offline-workflow/issue-PROJ-101.json

# Deterministic preview; writes no state.
./aef plan --issue "$ISSUE"
./aef run feature-development --issue "$ISSUE" --provider fake --dry-run

# Real local run through the fake provider, stopping at the approval gate.
./aef run feature-development --issue "$ISSUE" --provider fake --actor demo-user

# Approve, resume, inspect, and export evidence.
./aef approve <run-id> --gate architecture --approver reviewer-1
./aef resume <run-id>
./aef status <run-id>

# Run real local checks and append their results as evidence.
./aef verify <run-id> --profile go

# Reconstruct requirement -> release traceability from evidence.
./aef trace <run-id> --format markdown --out trace.md

# Verify fixture structure without invoking a model.
./aef eval --suite all --execute

# Scan a diff, resolve the recorded finding, then export evidence.
./aef review --diff candidate.diff --run <run-id>
./aef review resolve --run <run-id> --finding rev-... \
  --status fixed --reason "fixed after local verification" --actor reviewer-1
./aef evidence export <run-id> --format both

# Multi-turn fake-provider conversation with a preserved session.
./aef chat --provider fake --message "first question" --run <run-id>
./aef chat --provider fake --session <session-id> --message "follow up" --run <run-id>

# Preview the orchestration loop; writes no state and calls no service.
./aef autopilot --issue "$ISSUE" --dry-run
```

The shorthand `PROJ-101` resolves to the fixture above. The fake provider returns outputs marked `simulated: true`; it does not run shell commands, call models, or create branches, commits, pushes, or deployments. A run that relies only on fake-provider output is **blocked with missing evidence** at the merge gate. `aef verify <run-id>` executes checked-in profile commands locally, records the command, sanitized output, and real exit status as evidence, and can satisfy those gates. Structured review findings must be explicitly resolved before merge; see [integrations](docs/integrations.md) for opt-in Jira and live chat setup and [evidence](docs/evidence.md) for the record format.

Live Jira calls, GitHub CLI reads/comments, and live Claude/Codex conversations require explicit opt-in configuration/flags and environment credentials. The adapters and local tests exist, but no live service call was made in this session. `autopilot` executes only within recorded policy: it stops at approval gates, runs real local checks, never creates a branch/PR, never pushes/merges/deploys, and high-risk work remains blocked at independent review. GitHub App/service identity, PR creation/merge, cloud deployment, and federated identity are **planned / not implemented**.

## Example Jira workflow (design target)

1. `jira-intake` reads issue `PROJ-101` from a local fixture or, when explicitly configured and confirmed, Jira Cloud REST. An approved Jira MCP adapter remains planned. Issue text is untrusted data.
2. `requirement-analysis` and `acceptance-criteria` produce structured artifacts; `risk-assessment` classifies changed paths and semantics.
3. `technical-planning` and `solution-architecture` create a YAML DAG. Escalations (auth, billing, tenant isolation, IAM, secrets, infrastructure, migrations, data retention, public API) require approval.
4. `awaiting_approval` → `implementing` → `verifying` → `reviewing` → `remediating` (bounded loop) → `awaiting_merge` → `staging` → `awaiting_release` → `production_verification` → `completed`.
5. Every transition appends a transition record and content-addressed evidence; required evidence and approvals are checked before a gate opens.

Workflow states include `queued`, `analyzing`, `planning`, `awaiting_approval`, `implementing`, `verifying`, `reviewing`, `remediating`, `awaiting_merge`, `staging`, `awaiting_release`, `production_verification`, `completed`, `blocked`, `failed`, `cancelled`. Cycles are rejected except bounded remediation retries.

## Opt-in Jira link intake

Fixtures remain the default; no network call happens unless Jira Cloud is
explicitly configured in `aef.yaml`:

```yaml
integrations:
  jira:
    mode: jira-cloud
    baseURL: https://your-site.atlassian.net
    tokenEnv: AEF_JIRA_TOKEN
```

Export the token in the named environment variable before running:

```sh
# `plan` and `--dry-run` use fixtures; live Jira intake requires an explicit live command.
./aef plan --issue examples/offline-workflow/issue-PROJ-101.json
./aef run feature-development --issue TEAM-123 --provider fake --confirm-live
```

Only HTTPS origins that match `baseURL`, bare issue keys, and supported Jira
browse links are accepted. Credentials never appear in flags, config, prompts,
fixtures, or URLs. Jira issues must carry `aef-acceptance:` and `aef-path:`
labels (or the equivalent description sections); missing AEF metadata fails
closed instead of inventing acceptance criteria. See
[integrations](docs/integrations.md) for the full contract.

## Command status matrix

Status labels describe the current tree, not phase targets.

| Command | Status | Notes |
| --- | --- | --- |
| `aef init --profile go,react` | Implemented | Writes `aef.yaml` and creates `.aef` state, evidence, and objects directories. Canonical profiles include `java`, `python`, `laravel`, `typescript`, and the aggregate `all`; unknown names are rejected. See [profiles](docs/profiles.md). |
| `aef doctor` | Implemented | Local toolchain, schema, catalogue, state, and optional provider-binary checks; no cloud probes. |
| `aef validate` | Implemented | Schemas, skills, workflows, policies, agents, and config validation. |
| `aef skills list` | Implemented | Reads the canonical catalogue. |
| `aef skills install --dry-run` | Implemented | Prints planned writes; no filesystem changes. |
| `aef skills install --provider ... --scope ...` | Implemented | Generates provider copies with manifest + SHA-256; refuses unmanaged conflicts unless `--force`. |
| `aef skills uninstall` | Implemented | Removes only manifest-managed files unless `--force`. |
| `aef skills export --out ...` | Implemented | Generates plugin-ready bundle directories locally; not published. |
| `aef plan --issue <fixture\|Jira link\|key>` | Implemented locally | Fixtures only; a configured Jira Cloud link/key requires a live `run` or `autopilot` with `--confirm-live` and uses the opt-in REST adapter, not exercised live here. |
| `aef run <workflow> --issue <fixture> --dry-run` | Implemented, offline only | Deterministic preview; no state writes. |
| `aef run <workflow> --issue <fixture> --provider fake` | Implemented, local only | Real SQLite run with simulated task results; stops at gates and blocks on missing evidence. |
| `aef run ... --provider claude\|codex --confirm-live --workspace DIR [--allow-writes]` | Implemented locally, not exercised live | Live providers require explicit confirmation and a managed worktree; `--allow-writes` uses constrained file tools/sandbox, never shell tools. |
| `aef status <run-id>` / `status --list` | Implemented, local only | Reads persisted run state and transitions. |
| `aef resume <run-id> [--provider ...] [--confirm-live] [--allow-writes]` | Implemented, local only | Advances until a gate, policy denial, completion, or block; live continuation uses the same opt-in checks as `run`. |
| `aef cancel <run-id>` | Implemented, local only | Atomic cancellation with a recorded reason. |
| `aef approve <run-id> --gate ... --approver ... [--role ...]` | Implemented, local only | Records identity, role, candidate SHA, plan digest, and review-evidence digest; checks distinct roles and invalidates on configured review-findings changes. |
| `aef review --diff <path> [--run id]` | Implemented, local only | Deterministic pattern-scan hypotheses; `verified: false`. With `--run`, persists chained review evidence. Live `--pr` GitHub review is not implemented. |
| `aef review resolve --run id --finding rev-... --status fixed\|accepted\|false-positive --reason text` | Implemented, local only | Records a human decision as chained remediation evidence; unresolved findings block merge/release/security-signoff gates. |
| `aef chat --provider fake --message text` | Implemented, local only | Multi-turn fake conversation; preserves session IDs and writes chained evidence with `--run`. |
| `aef chat --provider claude\|codex --confirm-live --workspace DIR ...` | Implemented locally, not exercised live | Explicit opt-in, env-based auth, managed AEF worktree, bounded turns, and sanitized evidence. |
| `aef autopilot --issue ...\|--resume ... [--dry-run] [--confirm-live] [--allow-writes] [--approver name --confirm-gates] [--github-pr ref] [--comment --confirm-mutate]` | Implemented locally, not exercised live | Opt-in orchestration loop: creates/binds a managed worktree, advances provider tasks, runs real local checks, scans the workspace diff, and stops at gates. No branch, PR creation, push, merge, or deployment. |
| `aef verify <run-id> [--profile ...]` | Implemented, local only | Executes checked-in profile commands plus a built-in secret-shape scan; records command, sanitized output, and real exit status as evidence. Missing tools are recorded as failures, not passes. |
| `aef worktree prepare|clean <run-id>` | Implemented, local only | Creates/removes a detached worktree under `.aef/worktrees`; no branch, commit, force-remove, push, or deploy. Requires a git repo with a commit. |
| `aef run ... --workspace <path>` | Implemented, local only | Accepts only AEF-managed worktree directories and records the workspace in run evidence; a bound workspace is never silently re-pointed. |
| `aef trace <run-id> [--format ...] [--out ...]` | Implemented, local only | Reconstructs lifecycle coverage from persisted evidence; missing stages remain `missing`. |
| `aef evidence export <run-id>` | Implemented | JSON/Markdown exports from stored run evidence. |
| `aef eval --suite ...` | Implemented as inventory only | Reports authored fixtures as `executed: false`. |
| `aef eval --suite ... --execute` | Implemented, local only | Runs deterministic structural checks on fixtures; `modelBehaviorExecuted: false` because no model is invoked. |
| Live Jira/Claude/Codex execution | Implemented locally, not exercised live | Opt-in adapters with env credentials and local stub/HTTP tests; no live call or provider login was performed. |
| GitHub CLI adapter (`gh pr view/checks/diff/comment`) | Implemented locally, not exercised live | Reads PR state and records sanitized evidence; PR comments require `--comment --confirm-mutate`. No push, PR creation, merge, or GitHub App identity. |
| GitHub App/service identity, cloud adapters, dashboard | Planned / not implemented | Docs and interfaces exist; no live behavior was exercised. |

## Skills shipped

`jira-intake`, `requirement-analysis`, `acceptance-criteria`, `technical-planning`, `risk-assessment`, `solution-architecture`, `backend-development`, `git-worktree-management`, `unit-testing`, `code-review`, `agent-handoff`, `pull-request-management`.

The full catalogue across Product, Architecture, Cloud, Development, Quality, Security, DevOps, and Agentic domains is **planned**.

## Security model

- **Policy precedence (highest to lowest):** immutable runtime safety > org > repo > workflow defaults > agent instructions > untrusted task content. Fail closed on conflict, missing policy, unknown semantic tags, or validation error.
- **Trust boundaries:** Jira/GitHub/MCP content, repository files, model output, and provider plugins are untrusted. They can never override runtime safety or policy.
- **Credentials:** no tokens in flags, config, prompts, or fixtures; use environment variables or an external secret store. Approvals record local identity/role and digests; federated identity and PR authorization are planned.
- **Supply chain:** canonical skills are source Markdown; generated provider manifests carry SHA-256 checksums. CI actions should be pinned to verified SHAs before release (currently a TODO in `.github/workflows/ci.yml`).
- **Isolation:** `aef worktree prepare` and `aef autopilot` can create a detached, AEF-managed worktree from `HEAD`; arbitrary paths, branches, commits, force-removal, and user-owned work are refused. Workspace flags require a registered git worktree, and `autopilot` binds a run-owned worktree before any provider task, including pre-approval tasks. Live provider writes require `--allow-writes` plus `--confirm-live` and are constrained to Claude file tools with outside-worktree permission blocks (Claude Code v2.1.314+) or Codex `workspace-write`; they remain unexercised against live providers.
- **Executable profiles:** `aef verify` runs shell commands from checked-in `framework/profiles/*/profile.yaml`. Treat profiles as executable configuration and use `verify` only in repositories you trust; commands never come from issue text or CLI flags.
- Details: [`docs/security/threat-model.md`](docs/security/threat-model.md) and [`SECURITY.md`](SECURITY.md).

## Limitations

- Pre-1.0, no stable compatibility guarantees.
- Only the 12 Phase 1 skills exist; broader catalogue is planned.
- The fake provider is the only exercised end-to-end provider path. Its outputs are simulated and do not satisfy build/lint/test evidence gates until `aef verify` records real local results. The Claude and Codex adapters have local stub tests for command construction, session continuity, and constrained write modes, but no live model conversation or live file edit was run.
- `eval --execute` checks fixture structure only; no behavioral model evaluation or pass rate exists.
- The Java, Python, Laravel, TypeScript, and aggregate `all` profiles are checked in and validated, but their external toolchains were not exercised here; see [profiles](docs/profiles.md).
- Jira Cloud REST intake, GitHub CLI read/comment, provider chat, and constrained provider writes are opt-in local implementations, not live-exercised integrations. `autopilot` was exercised only with the fake provider in a throwaway git repository; it stops at real gates and cannot push, create a PR, merge, or deploy. GitHub App/service identity, independently verified review, cloud, and dashboard work is planned / not implemented. Worktree commands were exercised only in a throwaway git repository with a synthetic commit.
- `aef review --diff` is a local pattern scan with `verified: false`; resolution evidence records human decisions but does not make hypotheses independently verified.
- No commits were made in this source repository, and no releases, published module, branches, pushes, deployments, or cloud resources were created for this work.
- Provider paths are verified as of 2026-11-25 but must be rechecked before use.
- No security audit or penetration test has been performed; no known vulnerabilities are published.

## Roadmap

| Phase | Summary | Status |
| --- | --- | --- |
| 0 | Foundation: docs, ADRs, licence, CI skeleton | Implemented locally |
| 1 | 12 canonical skills, manifests, local CLI basics | Implemented locally |
| 2 | Offline plan/run, SQLite state, policy, evidence capture | Implemented locally with fake provider |
| 3 | Local lifecycle, approvals, verification evidence, safe worktree preparation, traceability, evidence export, opt-in Jira/provider chat and autopilot loop | Local subset and opt-in adapters implemented; live calls and live writes not exercised; CI hooks planned |
| 4 | Structured review evidence, finding resolution, bounded remediation, merge gates, GitHub CLI PR evidence/comments | Local subset implemented; live GitHub calls, PR creation/merge, and independent review planned |
| 5 | Full skill catalogue and executed behavioral evaluation suites | Structural fixture checks implemented; behavioral suites planned / not implemented |
| 6 | Optional AWS adapters and Terraform reference architecture | Planned / not implemented |
| 7 | Optional dashboard, multi-user features, stable 1.0 | Planned / not implemented |

See [`docs/roadmap.md`](docs/roadmap.md) for exit criteria and [`progress.md`](progress.md) for verified commands.

## Documentation

- [Architecture](docs/architecture.md)
- [Getting started](docs/getting-started.md)
- [Installation](docs/installation.md)
- [Profiles](docs/profiles.md)
- [Integrations](docs/integrations.md)
- [Provider compatibility](docs/provider-compatibility.md)
- [Evidence](docs/evidence.md)
- [Threat model](docs/security/threat-model.md)
- [Roadmap](docs/roadmap.md)
- [ADRs](docs/adr/README.md)

## Contributing and license

See [`CONTRIBUTING.md`](CONTRIBUTING.md), [`SECURITY.md`](SECURITY.md), and [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md). Apache-2.0 licensing is proposed in [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
