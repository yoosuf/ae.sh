# Progress

Last verified: 2026-11-25 (local macOS session, Go 1.27). This file records
what was actually run in this tree. It is not a release report.

## Implemented and verified locally

### Checks

| Command | Result |
| --- | --- |
| `make fmt-check` / `gofmt -l cmd internal` | Passed; no unformatted Go files. |
| `go build ./...` | Passed. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed. Packages with tests: `internal/adapters/claude`, `internal/adapters/codex`, `internal/adapters/github`, `internal/adapters/jira`, `internal/adapters/sqlite`, `internal/catalog`, `internal/cli`, `internal/config`, `internal/execution`, `internal/framework`, `internal/policy`, `internal/routing`, `internal/schema`, `internal/util`, `internal/workflow`. Other packages have no test files. |
| `go test -race ./...` | Passed with the same package coverage. |
| `gofmt -w internal/execution internal/adapters/sqlite` and targeted `go test ./internal/execution ./internal/adapters/sqlite ./internal/cli` | Passed while adding review-evidence approval invalidation. |
| `scripts/check-schemas.sh` | Passed; `validate` reported `status: valid`, 12 skills, 10 workflows, 2 policies, 12 agents, 12 profiles (`go`, `java`, `python`, `dotnet`, `nodejs`, `typescript`, `react`, `vue`, `astro`, `laravel`, `aws`, `all`), and `evaluations: authored; no behavioral/model evaluation executed`. |
| `scripts/e2e-offline.sh` | Passed. |
| `scripts/e2e-worktree.sh` | Passed. |
| `make check` | Passed (build, vet, unit tests, race tests, schema validation, offline E2E, worktree E2E). |
| `gofmt -w` on reorganized Go files, `go build ./...`, targeted package tests, then `make check` | Passed after moving canonical assets under `framework/` and adding `internal/framework/paths.go`. |
| `go build ./...`, `go test ./...`, then `make check` | Passed after renaming the module, CLI, config, state, env, schema, metadata, and label identifiers to `ae`. |
| `gofmt -w` on routing code, targeted `go test` for `internal/config`, `internal/workflow`, `internal/execution`, `internal/cli`, `internal/routing`, and both live adapters, then `make check` | Passed after adding model routing. `make check` covered the offline and worktree E2E scripts as well. |
| `ae eval --suite all --execute --json` in a throwaway checkout | Passed: 9 security cases, 12 skill fixtures, `modelBehaviorExecuted: false`, `status: passed`. Only structural fixture checks ran; no provider was invoked. |
| `ae skills install --provider all --scope project` in a throwaway checkout | Passed: two provider manifests with 12 skills each and zero SHA-256 checksum mismatches. No generated file was hand-edited in the source tree. |
| `go run ./cmd/ae --help` | Passed; `autopilot` and the live/write flags on `run` and `resume` are exposed, along with `chat --route`. |
| `bash -n scripts/uninstall.sh` and a throwaway-manifest safety check | Passed; documented Claude/Codex roots are accepted and `.codex/skills` is rejected. |

### Canonical asset layout

`agents/`, `policies/`, `profiles/`, `routing/`, `schemas/`, `skills/`,
`templates/`, and `workflows/` now live under `framework/`. The catalogue,
workflow, policy, routing, config, schema, and CLI tests resolve them through
`internal/framework/paths.go`; no top-level copies remain. Workflow policy
references and docs were updated to the new canonical paths. Generated Claude
and Codex skill directories were not moved.

Routing intent is provider-neutral: canonical skills carry `ae.model-class`,
agents and workflow tasks carry `routingProfile`, and the catalogue of five
profiles lives in `framework/routing/profiles.yaml`. Only local
`provider.routes` maps a profile to a Claude/Codex model alias or ID; no vendor
model name appears in canonical assets.

### Offline end-to-end behavior observed

`scripts/e2e-offline.sh` builds the CLI, copies the repository to a temporary
directory, and runs without credentials, network access, branches, commits,
pushes, or deployments. It verified:

1. `ae init --profile go` and schema/catalogue validation.
2. `ae plan --issue examples/offline-workflow/issue-PROJ-101.json` orders tasks
   from `intake`.
3. `ae eval --suite all --execute` reports `executed: true`,
   `modelBehaviorExecuted: false`, and `status: passed` for structural fixture
   checks only.
4. `ae run feature-development --issue ... --provider fake --dry-run` reports
   `dryRun: true` and writes no state.
5. A real local fake-provider run stops at `awaiting_approval`.
6. `ae approve <run> --gate architecture --approver reviewer-1` plus
   `ae resume <run>` reaches `awaiting_merge`.
7. `ae approve <run> --gate merge --approver reviewer-2 --role release-manager`
   followed by `resume` exits with policy code `4` and reports
   `missing required evidence`.
8. `ae status <run>` shows `status: blocked` with the missing-evidence reason
   persisted in `terminalReason`.
9. `ae review --diff candidate.diff --run <run>` records one deterministic
   finding, and `ae review resolve --run <run> --finding <id> --status fixed
   --reason ...` records a human remediation decision as chained evidence.
10. `ae trace <run>` reports seven lifecycle stages; `feedback` remains
    `missing` because no operational evidence exists.
11. `ae evidence export <run> --format json --out ...` writes a non-empty
    `evidence.json`.
12. A second run passes `ae verify <run>` with real local command exit statuses,
    then completes after merge approval and resume.
13. `ae chat --provider fake --message ... --run <run>` preserves a session ID,
    and a second turn with `--session <id>` reuses it, remains simulated, and
    writes chained provider evidence.

This confirms the fail-closed boundary: fake-provider output is
`simulated: true` and does not produce real build/lint/test results, so that
run is blocked. `ae verify` separately executes checked-in profile commands,
records sanitized output and real exit status as `test` evidence, and can
satisfy the merge gate without fabricating a pass.

### Autopilot behavior observed locally

`go test ./internal/cli -run TestAutopilot -count=1 -v` exercised the opt-in loop
in a copied repository with a low-risk fixture and a fast local profile:

1. `--dry-run` reported planned steps, created no run ID, and wrote no
   `.ae/state.db`.
2. A live provider without `--confirm-live`, and `--github-pr` without
   `--confirm-live`, were denied with exit code 4.
3. With no explicit approver the run bound a validated detached worktree, ran
   the pre-approval tasks inside it, then stopped at `awaiting_approval`.
4. With `--approver reviewer-1 --confirm-gates`, a fake-provider run reused that
   managed worktree, ran local checks inside it, found no diff findings,
   recorded a second delegated approval, and reached `completed`.
5. No branch, PR, push, merge, deployment, live provider call, live Jira call, or
   live GitHub call was made.

`TestAutopilotRejectsSelfApproval` verified that delegated approval cannot be
supplied by the run actor. `TestRunRejectsUnsafeLiveAndWriteFlags` also verified
that `--allow-writes` is denied for the fake provider, a live provider without
`--confirm-live` is denied, and a confirmed live provider without a managed
`--workspace` is denied. `TestCollectWorkspaceDiffIncludesUntrackedFiles`
verified that untracked text files enter the review diff and untracked symlinks
fail closed. `TestValidateManagedWorktreeRejectsPlainDirectory` verified that a
plain directory under `.ae/worktrees` is not accepted as a managed worktree.

### Worktree end-to-end behavior observed

`scripts/e2e-worktree.sh` creates a throwaway copy of this repository, makes one
synthetic local commit in that copy, and verifies:

1. `ae worktree prepare <run-id>` creates a detached worktree under
   `.ae/worktrees/<run-id>` at the same `HEAD` as the main worktree.
2. No branch is created and the worktree's `HEAD` is not attached to a ref.
3. `ae run ... --workspace <unmanaged-path>` is rejected with one JSON error
   envelope in `--json` mode.
4. `ae run ... --workspace <managed-worktree>` persists the resolved workspace
   path, and that run passes architecture approval, local verification, merge
   approval, and resume to `completed`.
5. `ae worktree clean <run-id>` removes the worktree without `--force` and
   leaves no worktree registration.

The synthetic commit exists only in the deleted temporary copy. No commit was
made in the source repository.

### Unit and integration coverage added

- `internal/framework`: canonical asset path helpers, with direct tests plus
  indirect coverage from catalogue, config, workflow, schema, CLI, and E2E
  checks.
- `internal/schema`: asset availability and normalized document validation.
- `internal/util`: stable digests, credential-shape redaction, recursive
  sanitization, synthetic-marker handling, and rejection of non-JSON values.
- `internal/policy`: highest-risk classification, unknown-semantics denial,
  risk-scoped gates/approval rules, unsafe-action denial, missing-evidence
  denial, stale-approval denial, and self-approval denial.
- `internal/workflow`: valid DAG ordering, cycle rejection, unknown skill,
  unknown state, and unknown routing profile rejection.
- `internal/catalog`: canonical catalogue loading, required skill sections,
  name/directory mismatch, broken reference reporting, installer dry-run,
  manifest checksums, unmanaged-file conflicts, forced writes, refreshes, and
  uninstall safety.
- `internal/adapters/sqlite`: restart persistence, optimistic concurrency,
  approval-row migration, workspace-column migration for legacy runs, and
  workspace binding that refuses to re-point an existing run.
- `internal/execution`: dry-run with no state, approval-to-completion path,
  stale-approval blocking, missing-evidence blocking at the merge gate,
  review-finding reconstruction after restart, stale-review rejection,
  unresolved-finding blocking, approval invalidation on review evidence change,
  bounded remediation attempts, fail-closed route precedence (task override,
  skill class, agent role, then `balanced`), and requested/actual model
  evidence on provider task records.
- `internal/config`: canonical profile loading, requested stack registration,
  `all` aggregation without duplicate checks, include-cycle rejection,
  fail-closed validation of Jira Cloud HTTPS origins, GitHub local-cli modes,
  HTTPS `baseURL`, environment variable names, and unknown modes. Model routes
  are defaulted per profile, merged with local overrides without mutating the
  shared map, and rejected when a profile is unknown, a provider unsupported,
  or a model empty, oversized, or credential-shaped.
- `internal/routing`: canonical profile loading and schema validation, empty
  profile defaulting to `balanced`, rejection of unknown profiles, provider
  allowlisting, credential-shaped model rejection, and fail-closed resolution
  when a profile/provider mapping is missing.
- `internal/adapters/jira`: bare key and configured browse-link parsing, ADF
  text flattening, AE metadata extraction, sanitization, env-only bearer
  headers, and fail-closed missing metadata.
- `internal/adapters/github`: local `gh` stubs verifying PR JSON parsing, read
  argument shapes, check/diff reads, env-only tokens kept out of arguments,
  mutation denial, and mode validation; no live GitHub call was made.
- `internal/adapters/claude` and `internal/adapters/codex`: local executable
  stubs verifying noninteractive argument shape, session resume, constrained
  write-mode flags, Claude outside-worktree permission blocks, rejection of
  Claude versions without those settings, `--model` from the resolved request,
  and requested/actual model capture; no live model was invoked. The fake
  adapter echoes route fields in its simulated output for observability and
  advertises no model-selection capability.
- `internal/evidence`: credential-shaped values in actor and subject fields are
  redacted in the persisted record. `internal/config`: credential literals in
  every environment-name field are rejected regardless of integration mode.
- `internal/cli`: managed worktree path validation, unsafe profile rejection,
  structural fixture checks, trace stage mapping with honest `missing` stages,
  one JSON error envelope for usage errors, init acceptance of the expanded
  profile set, fake chat session/evidence/redaction and live-policy denial,
  review-finding resolution evidence with unknown-finding rejection, autopilot
  dry-run/no-state, live and GitHub confirmation denial, human-gate stop,
  self-approval denial, fake-provider completion with delegated approval,
  unsafe `run` flag combinations, `chat --route` rejection of unknown profiles,
  and per-turn route/model evidence. `validate` reports routing profile and
  local model-route counts.

## Implemented but not exercised live

- Claude Code CLI chat (`claude --bare -p ... --output-format json`, with
  `--resume` on later turns).
- Codex CLI chat (`codex exec --json --sandbox read-only`, with
  `codex exec resume <thread-id>` on later turns).
- Jira Cloud REST issue intake; an approved Jira MCP adapter remains planned.
- GitHub CLI PR reads/comments; PR creation, merge, push, and GitHub
  App/service identity remain planned.
- Constrained provider write modes (`--allow-writes`) and the live file edits
  they enable; no live model edited a file. Real commits in the user's
  repository, branch pushes, CI/deploy hooks, and cloud deployment also remain
  unexercised.
- Live model selection through `--model`; routing resolves to local aliases or
  IDs and is passed to the Claude/Codex adapters, but no live provider call was
  made and no requested model was confirmed against an installed CLI.
- Behavioral model evaluation; `eval --execute` runs deterministic structure
  checks only. The added model-route injection cases are authored fixtures,
  not executed model behavior.
- The new `java`, `python`, `laravel`, and `typescript` profiles and the `all`
  aggregate were loaded and validated, but their external toolchains were not
  installed or exercised in this session. Only the `go` profile's checks were
  executed in the offline E2E; missing tools in other profiles remain explicit
  failures under `ae verify`.
- `review --diff` is a local pattern scan with `verified: false`, not an
  independent code review.
- Federated approver identity; local identity and role capture are implemented.

The opt-in adapters above were implemented and locally tested with HTTP or
executable stubs, but these live paths require explicit authorization and
credentials (or organizational context) and were **not run**. The fake chat
path and all local review-resolution behavior were exercised offline.

## Planned / not implemented

- Remaining Phase 3 work: authorized live issue/provider calls, real code
  changes, federated approver identity, and real CI/deploy integration.
- Phase 4: GitHub PR creation/merge/push, independently verified review
  findings that can satisfy `independent-review`, and merge signal.
- Phase 5: full skill catalogue and executed behavioral evaluation suites.
- Phase 6: optional AWS adapters and Terraform reference architecture.
- Phase 7: optional dashboard, multi-user features, stable 1.0 release.
- Signed/notarized evidence bundles, cloud artifact storage, external
  transparency logs, security audit, release signing, and verified SHA pinning
  of CI actions.

## Repository state

- The directory is a git repository on `main` tracking `origin/main`. The
  earlier framework, rename, and repository-link work is committed in 11
  logical commits; model routing is recorded in four small commits. No push,
  publish, deploy, or cloud resource creation was executed, requested, or
  approved by the agent. A read-only `git ls-remote` after the final commit
  showed GitHub `main` at the local HEAD, and the remote-tracking reflog
  records `update by push` at 2026-11-25 21:21 IST; that push was external to
  the commands run here.
- Visual-layer files (`assets/`, `docs/superpowers/`, and image links in several
  docs) are unrelated to routing and were kept out of routing commits.
- No credentials were used, stored in CLI flags/config/prompts/fixtures, or
  committed. No production data was accessed.
- Module publication via `go install ...@latest` is unavailable until a release
  exists; build from source or a local binary.

## Manual review

- The changed files were read and checked for consistency with CLI behavior,
  status labels, and the no-live-integration boundary. The routing diff was
  reviewed file by file: resolver, schemas, config, engine, CLI, adapters,
  canonical skills/agents/workflows, docs, evaluations, and tests.
- Real issues found and fixed during verification: the CLI emitted run JSON
  before a JSON error envelope on blocked commands; `validate` targets assumed
  an initialized root config; `README` and several docs overstated generated
  manifests, org policy loading, provider/model capture, and evaluation status;
  the uninstall script rejected documented Codex paths; `verify` and
  `eval --execute` emitted a result payload before an error in JSON mode;
  `verify --profile` and `trace --format|--out` were rejected after the run ID;
  worktree cleanup used the wrong git root after path canonicalization;
  credential-shaped fixtures without synthetic markers tripped the built-in
  secret scan during offline E2E; and the review-state helper initially treated
  `util.JSONValue` as single-valued.
- Routing review found that the offline fake provider advertised
  `ModelSelection: true` although it never selects a model; the capability is
  now `false`, while its simulated output still echoes route fields for tests.
- Review of the new gate logic also found that `review-findings-change` was
  declared by high-risk policy but not enforced. Approvals now bind to a
  deterministic review-evidence digest, with SQLite migration support and
  tests for stale approval, unresolved findings, and review evidence changes
  after restart.
- A read-only review of the autonomy path confirmed that pre-approval provider
  tasks could run in the CLI directory before worktree isolation. `autopilot`
  now creates and git-validates the managed worktree before the first provider
  call, scans staged, unstaged, untracked, and PR diffs, requires registered
  git worktrees for workspace flags, confines Claude writes with version-gated
  permission settings, and redacts every persisted evidence string field.
- No live provider, Jira, GitHub, cloud, or dashboard result is asserted.

## Next steps

1. Obtain explicit human authorization before any push, publish, deployment,
   cloud resource creation, or live provider/Jira/GitHub call. Local commits
   are made in small reviewable batches.
2. Add independent review and behavioral evaluation paths with an authorized
   provider.
3. Exercise live Claude/Codex/Jira/GitHub adapters only after authorization and
   credential setup, recording exact commands and sanitized outcomes.
4. Pin CI actions to verified commit SHAs and produce a signed release before
   calling the project stable.
