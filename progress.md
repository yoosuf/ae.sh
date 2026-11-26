# Progress

Last verified: 2026-11-26 (local macOS session, Go 1.27). This file records
what was actually run in this tree. It is not a release report.

## Implemented and verified locally

### Checks

| Command | Result |
| --- | --- |
| `make fmt-check` / `gofmt -l cmd internal` | Passed; no unformatted Go files. |
| `go build ./...` | Passed. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed. Packages with tests: `internal/adapters/attestation`, `internal/adapters/claude`, `internal/adapters/codex`, `internal/adapters/github`, `internal/adapters/identity`, `internal/adapters/jira`, `internal/adapters/sqlite`, `internal/catalog`, `internal/cli`, `internal/config`, `internal/execution`, `internal/framework`, `internal/policy`, `internal/routing`, `internal/schema`, `internal/util`, `internal/workflow`. Other packages have no test files. |
| `go test -race ./...` | Passed with the same package coverage. |
| `gofmt -w internal/execution internal/adapters/sqlite` and targeted `go test ./internal/execution ./internal/adapters/sqlite ./internal/cli` | Passed while adding review-evidence approval invalidation. |
| `scripts/check-schemas.sh` | Passed; `validate` reported `status: valid`, 15 skills, 10 workflows, 2 policies, 15 agents, 12 profiles (`go`, `java`, `python`, `dotnet`, `nodejs`, `typescript`, `react`, `vue`, `astro`, `laravel`, `aws`, `all`), 5 routing profiles, and `evaluations: authored; no behavioral/model evaluation executed`. |
| `go test ./internal/catalog ./internal/cli ./internal/schema` and `go build ./...` | Passed after adding the strengthened review contracts, specialist reviewers, and evaluation fixtures. |
| `go test ./internal/cli ./internal/schema` and `go test ./internal/execution ./internal/workflow ./internal/cli` | Passed after adding provider-backed review parsing/evidence and the specialist-review gate changes. |
| `scripts/e2e-offline.sh` | Passed. |
| `scripts/e2e-worktree.sh` | Passed. |
| `make check` | Passed (build, vet, unit tests, race tests, schema validation, offline E2E, worktree E2E). |
| `gofmt -w` on reorganized Go files, `go build ./...`, targeted package tests, then `make check` | Passed after moving canonical assets under `framework/` and adding `internal/framework/paths.go`. |
| `go build ./...`, `go test ./...`, then `make check` | Passed after renaming the module, CLI, config, state, env, schema, metadata, and label identifiers to `ae`. |
| `gofmt -w` on routing code, targeted `go test` for `internal/config`, `internal/workflow`, `internal/execution`, `internal/cli`, `internal/routing`, and both live adapters, then `make check` | Passed after adding model routing. `make check` covered the offline and worktree E2E scripts as well. |
| `ae eval --suite all --execute --json` in a throwaway checkout | Passed: 9 security cases, 15 skill fixtures, `modelBehaviorExecuted: false`, `status: passed`. Only structural fixture checks ran; no provider was invoked. |
| `ae skills install --provider all --scope project` in a throwaway checkout | Passed: Claude and Codex provider manifests with 15 skills each and zero SHA-256 checksum mismatches. No generated file was hand-edited in the source tree. |
| `make check` after the review upgrades | Passed: build, vet, unit tests, race tests, schema validation, offline E2E, and worktree E2E. |
| Throwaway-checkout `ae review --diff ... --provider fake --skill test-review --candidate-sha ... --base-sha ... --json` | Passed after the fake-adapter fix: one simulated no-op result with zero findings, `verified: false`, and `simulated: true`. |
| `go test ./internal/cli ./internal/execution ./internal/evidence ./internal/adapters/sqlite ./internal/schema` | Passed after adding pinned live-diff binding, required check-class coverage, observed-model enforcement, read-only reviewer scopes, combined specialist findings, and atomic evidence chaining. |
| `make check` after the live-review fixes | Passed: build, vet, unit tests, race tests (including the concurrent evidence-chain test), schema validation, offline E2E, and worktree E2E. |
| Fresh-throwaway `ae eval --suite all --execute --json` and `ae skills install --provider all --scope project` after final hardening | Passed: 9 security cases and 15 skill fixtures with `modelBehaviorExecuted: false`; 15 skills in both Claude and Codex manifests with zero SHA-256 mismatches. |
| Authorized live `ae review --provider codex --skill code-review --confirm-live` in a throwaway managed worktree | Exited 0 with `routingProfile: independent-review`, `requestedModel: gpt-6.2-astra`, `model: null`, `verified: false`, and nine untrusted hypotheses. Claude review was attempted but the CLI returned `Not logged in`; no write, push, PR, merge, deploy, or live service mutation occurred. |
| `go build ./...`, `go test ./internal/adapters/jira/... ./internal/cli/...`, and `git diff --check` after adding Jira bug reporting | Passed: comment/new-ticket paths, ADF payloads, key/project/type validation, credential-shape rejection, HTTPS enforcement, dry-run with no source/token/evidence access, confirmation denial, chained evidence with digests, and symlink rejection. No live Jira call was made. |
| `go run ./cmd/ae --help` | Passed; `autopilot`, `jira report-bug`, and the live/write flags on `run` and `resume` are exposed, along with `chat --route`. |
| `bash -n scripts/uninstall.sh` and a throwaway-manifest safety check | Passed; documented Claude/Codex roots are accepted and `.codex/skills` is rejected. |
| `go test ./internal/cli/... -run 'TestEvidence|TestAudit' -count=1` | Passed after adding local evidence signing/verification and `ae audit`. |
| `go test ./internal/cli/... -run 'TestBehavioral|TestStructuralEval' -count=1` | Passed after adding fake-provider behavioral scenarios; all 15 cases passed and the structural mode stayed unchanged. |
| `go test ./internal/cli/... -run TestDashboard -count=1` | Passed; run-scoped snapshot files were written, credential-shaped evidence was redacted, review findings were labeled hypotheses, and missing run/output-directory inputs failed closed. |
| `cmp -s framework/schemas/evidence.schema.json internal/schema/assets/evidence.schema.json`, `go test ./internal/evidence ./internal/schema -count=1`, and `go test ./internal/cli/... -run TestAutopilot -count=1` | Passed after adding the `github` evidence kind; canonical and embedded schemas match and evidence builds validate. |
| `go run ./cmd/ae audit --json` | Exited 0 with overall `incomplete`: `credential-shapes`, `embedded-schemas`, and `canonical-references` passed; `generated-skill-copies` was `skipped` (no installed copies); `ci-action-pinning` was `needs-authorization`. This is a local report, not release approval. |
| `go run ./cmd/ae eval --suite all --execute --behavioral --json` | Exited 0 with `status: passed`, `mode: structural-fixture-checks+behavioral-fake-provider`, `provider: fake`, `executed: true`, `modelBehaviorExecuted: false`, and 15/15 fake-provider cases passed. No model was invoked. |
| `go run ./cmd/ae doctor --json` after adding the extension points | Exited 0 with `provider/gemini`, `identity/federated`, and `review/external-attestation` reported as `extension-point`; no live verifier or service was called. |
| `go run ./cmd/ae --help` | Exposed `audit`, `jira report-bug`, `evidence verify`, `dashboard snapshot`, and `eval ... [--behavioral]`. |
| `npm test --offline` in `ui/` | Passed: 2 Node tests verified no runtime network/write path and the embedded-snapshot element. |
| `npm install --offline --no-audit --no-fund` in `ui/` | Failed with `ENOTCACHED` for `@types/react` from the configured npm registry. UI `typecheck` and `build` were therefore **not run**; the CLI does not depend on npm. |
| `make check` after all local hardening, schema fix, and docs updates | Passed: format check, build, vet, unit tests, race tests, schema validation, offline E2E, and worktree E2E. |
| `git diff --check` after final edits | Passed; no whitespace errors in tracked changes. |
| Targeted `go test ./internal/config ./internal/adapters/identity ./internal/adapters/attestation ./internal/cli -count=1` after adding extension points | Passed: config validation rejects HTTP/userinfo/query issuers, credential values in env-name fields, unsafe labels, and unknown modes; both adapter `Verify` methods fail closed without reading env values; `ae doctor` reports both extension points. No live verifier or service was called. |
| `make check` after the onboarding/hygiene documentation changes | Passed: format check, build, vet, unit tests, race tests, schema validation, offline E2E, and worktree E2E. |
| `go run ./cmd/ae audit --json` | Exited 0 with overall `incomplete`: `credential-shapes` (281 readable text files), `embedded-schemas`, and `canonical-references` passed; `generated-skill-copies` was `skipped` (no installed copies); `ci-action-pinning` was `needs-authorization` for `actions/checkout@v4` and `actions/setup-go@v5`. No unverified SHAs were invented. |
| `go run ./cmd/ae eval --suite all --execute --behavioral --json` | Exited 0 with `status: passed`, `mode: structural-fixture-checks+behavioral-fake-provider`, `provider: fake`, `executed: true`, `modelBehaviorExecuted: false`, 9 security cases passed, and 15/15 fake-provider cases passed. No model was invoked. |
| `go run ./cmd/ae doctor --json` in the checkout without `ae.yaml` | Exited 0 and reported `repository: fail` with `ae.yaml not found (run \`ae init\` first)`, confirming the docs now require init before doctor. `provider/gemini`, `identity/federated`, and `review/external-attestation` were reported as `extension-point`. No live service was called. |
| `cd ui && npm test --offline` | Passed: 2 Node tests verified no runtime network/write path and the embedded-snapshot element. |
| `bash -n scripts/uninstall.sh` | Passed; updated allowlist syntax is valid. |
| `git diff --check` and a credential-shape grep over docs/scripts/config/source | Passed; no whitespace errors and no real credential values. Only pre-existing synthetic `FAKE`/`EXAMPLE` test fixtures matched. |
| `gofmt -w internal/config internal/cli internal/workflow internal/execution` and `go test ./internal/config ./internal/cli ./internal/workflow ./internal/execution -count=1` | Passed after adding config path containment, fail-closed secret scanning, and task timeout validation. The new tests cover absolute/traversal/symlink config paths, symlink skip and unreadable-root failure in the secret scan, and invalid/valid timeout bounds. |
| `go run ./cmd/ae audit --json` after the new tests | Exited 0 with overall `incomplete` and the same `needs-authorization` CI pinning note; the credential-shape check stayed `passed` because test secrets are built at runtime rather than committed as literals. |

### Local hardening observed

The new local-only commands keep clear trust boundaries:

- `ae evidence export ... --sign` and `ae evidence verify` use a local Ed25519
  key and offline digest/chain checks only. No external timestamp, notarization,
  transparency log, or provider attestation is claimed.
- `ae eval --behavioral` invokes the deterministic fake provider with sanitized
  fixture prompts and asserts routing, skill, output-field, `Simulated`, and
  `Usage.Known` bindings. It does not invoke a model, write evidence, or affect
  gates; `modelBehaviorExecuted` stays `false`.
- `ae audit` is read-only. On this checkout it reported `incomplete`, not
  `passed`, because CI action SHAs are not authoritatively pinned and no provider
  skill copies are installed to verify.
- `ae dashboard snapshot` is run-scoped and self-contained. The generated viewer
  reads an embedded snapshot from disk; no server, API, credentials, or write
  path is used. Review findings are shown as untrusted hypotheses.

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
model name appears in canonical assets. The 15 skills include four
strongest-route review contracts (`code-review`, `security-review`,
`architecture-review`, `test-review`) with matching independent reviewers.

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
  skill class, agent role, then `balanced`), requested/actual model evidence on
  provider task records, `*-review` evidence classification, and gate evidence
  that only accepts observed, non-simulated, model-selected provider reviews.
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
  headers, and fail-closed missing metadata. Bug reporting adds bounded and
  sanitized title/description validation, credential-shape rejection, HTTPS
  enforcement, project/key/issue-type checks, response key/id validation on
  create and comment, and separate ADF payload paths for new and existing
  tickets.
- `internal/cli`: `jira report-bug` dry-run/JSON previews, explicit
  `--confirm-mutate` policy denial, fixture-mode denial, description-file
  handling that rejects symlinks and oversized files, and chained `kind:
  artifact` evidence containing digests and identifiers rather than raw report
  text.
- `internal/adapters/github`: local `gh` stubs verifying PR JSON parsing, read
  argument shapes, check/diff reads, env-only tokens kept out of arguments,
  mutation denial, and mode validation; no live GitHub call was made.
- `internal/adapters/claude` and `internal/adapters/codex`: local executable
  stubs verifying noninteractive argument shape, session resume, constrained
  write-mode flags, Claude outside-worktree permission blocks, rejection of
  Claude versions without those settings, `--model` from the resolved request,
  and requested/actual model capture; no live model was invoked in those stub
  tests. The fake adapter echoes route fields in its simulated output for observability and
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
  local model-route counts. Provider-backed review tests cover canonical skill
  and route resolution, dry-run with no provider call, live confirmation and
  managed-workspace denial, strict JSON normalization, stable hypothesis IDs,
  rejected verification claims, requested/actual model evidence, and chained
  `kind: review` evidence with reconstructable findings, plus an end-to-end
  fake-provider review that returns a simulated no-op. `validate` reports
  15 skills, 15 agents, 10 workflows, and 5 routing profiles.

## Authorized live review observed

With explicit authorization, a throwaway checkout was prepared at the pinned
base `af337e5` and candidate `62c5fa5`. A 2,838-line, 152,685-byte diff was
credential-scanned and passed to the read-only Codex route. The exact review
command used `--provider codex --skill code-review --candidate-sha 62c5fa5...
--base-sha af337e5... --run ... --workspace <managed worktree> --confirm-live
--timeout 15m`. It completed with exit 0, `verified: false`, and nine hypotheses
covering diff/SHA binding, test evidence at the review gate, reviewer write
access, incomplete review coverage, model identity, specialist remediation,
concurrent evidence chains, diff-size enforcement, and the local schema limit.

The hypotheses were checked against code before edits; they were not applied as
provider instructions. Locally confirmed gaps were fixed with tests: live review
now binds the diff to committed, clean workspace state; every review gate must
answer the canonical check classes; test records cannot satisfy
`independent-review`; reviewers are forced read-only and intersect agent write
scopes; specialist findings feed remediation; evidence append is serialized;
diff reads are bounded while reading; the 100-finding schema rule applies only
to provider results; and gate credit requires an observed model matching local
config. The live Codex adapter reported no observed model, so under the final
rules that particular output would not satisfy the gate.

## Implemented but not exercised live

- Claude Code CLI chat (`claude --bare -p ... --output-format json`, with
  `--resume` on later turns).
- Codex CLI chat (`codex exec --json --sandbox read-only`, with
  `codex exec resume <thread-id>` on later turns).
- Jira Cloud REST issue intake and `ae jira report-bug` writes (existing-ticket
  comments and new-ticket creation); an approved Jira MCP adapter remains planned.
- GitHub CLI PR reads/comments; PR creation, merge, push, and GitHub
  App/service identity remain planned.
- Constrained provider write modes (`--allow-writes`) and the live file edits
  they enable; no live model edited a file. Real commits in the user's
  repository, branch pushes, CI/deploy hooks, and cloud deployment also remain
  unexercised.
- Live model selection through `--model` for chat and writes; routing resolves
  to local aliases or IDs and is passed to the Claude/Codex adapters. One
  authorized Codex review used the configured route, but its adapter did not
  report an observed model identity; Claude chat/write paths remain unexercised.
- Opt-in provider-backed `ae review --provider claude|codex` with a canonical
  strongest-route skill. One authorized Codex review returned hypotheses in a
  throwaway checkout; the Claude route was attempted but blocked by CLI
  authentication. No provider result is treated as externally verified.
- Behavioral model evaluation; `eval --execute` runs deterministic structure
  checks only. The added model-route injection cases are authored fixtures,
  not executed model behavior.
- The new `java`, `python`, `laravel`, and `typescript` profiles and the `all`
  aggregate were loaded and validated, but their external toolchains were not
  installed or exercised in this session. Only the `go` profile's checks were
  executed in the offline E2E; missing tools in other profiles remain explicit
  failures under `ae verify`.
- `review --diff` is a local pattern scan with `verified: false`, not an
  independent code review. The provider-backed path returned one live Codex
  hypothesis set, but no result has been independently verified.
- Live federated approver identity; only a validated config shape and a
  fail-closed adapter boundary exist. Local identity and role capture are
  implemented.
- Externally verified review; only a validated config shape and a fail-closed
  verifier boundary exist. No attestation was verified or accepted by a gate.

Except for the authorized Codex review above, the opt-in adapters in this
section were implemented and locally tested with HTTP or executable stubs.
Their live paths require separate explicit authorization and credentials (or
organizational context) and were **not run**. The fake chat path and all local
review-resolution behavior were exercised offline.

## Planned / not implemented

- Remaining Phase 3 work: authorized live issue/provider calls, real code
  changes, live federated approver identity, and real CI/deploy integration.
- Phase 4: independently verified findings, live external review attestation,
  GitHub PR creation/merge/push, and merge signal. One authorized live provider
  review was exercised; the local provider path can record the
  `independent-review` gate only when an observed, non-simulated provider result
  matches the configured model. The external-attestation config boundary does
  not satisfy that gate.
- Phase 5: full skill catalogue and real model behavioral evaluation. Structural
  checks and deterministic fake-provider scenarios are implemented.
- Phase 6: optional AWS adapters and Terraform reference architecture.
- Phase 7: hosted/multi-user dashboard, stable 1.0 release. The local read-only
  dashboard snapshot is implemented.
- Notarized/externally attested bundles, cloud artifact storage, external
  transparency logs, external security audit, release signing, and verified SHA
  pinning of CI actions. Local Ed25519 signatures, offline verification, and a
  read-only `ae audit` report are implemented.

## Repository state

- The directory is a git repository on `main` tracking `origin/main`. The
  earlier framework, rename, and repository-link work is committed in 11
  logical commits; model routing is recorded in four small commits; the review
  upgrades are recorded in `31e48bf` (contracts/reviewers), `07d18d5`
  (provider-backed review), `7113f1d` (lifecycle gates), `68fefe0` (live
  workspace denial test), `2f0ddf2` (fake-provider no-op review), and `62c5fa5`
  (status docs). The live-review fixes are in `6152470` (binding, coverage,
  evidence chain, schema), `9d466ce` (reviewer scopes and combined findings),
  and `5f90c35` (observed-model gate and canonical check slugs), followed by a
  final status commit. Jira bug reporting is in `aeec491`, `73118d8`, and
  `b832251`; the remaining-work plan is in `e9eee7b` and `6f21c1d`. This
  hardening batch added `62c6d38` (`ae audit`), `35802a7` (signed bundles),
  `bf5e62a` (behavioral eval), `6dfbc0c` (dashboard snapshot), `f4e7e68`
  (optional UI source), `302f7b0` (github evidence schema fix), and `2c94b7e`
  (single JSON error envelope on behavioral eval failure), followed by a final
  docs/status commit. No push, publish, deploy, PR, merge, or cloud
  resource creation was executed, requested, or approved by the agent.
  Immediately before the final docs commit, `git rev-parse HEAD origin/main`
  showed local `2c94b7e5c3c938103181a9b9ae5a54d587d92b4e` and remote-tracking
  `bf5e62a3029b8b5dec9af01f8f8960fac6537d7e`; the earlier remote-tracking reflog
  attributes its prior advance to `update by push` outside this session's
  commands. Exact final refs are reported in the session handoff.
- Visual-layer files (`assets/`, `docs/superpowers/`, and image links in several
  docs) were kept out of the routing and review code commits.
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
- Review-upgrade self-review found two gate defects before final verification:
  `test-review` was classified as test evidence because `test` matched before
  the `-review` suffix, and the high-risk `independent-review` gate only
  accepted `kind: test` records so no review evidence could satisfy it. The
  suffix now wins, and gate evidence accepts only an observed, non-simulated,
  model-selected provider review. Provider claims of `verified: true` are
  rejected, findings stay hypotheses with stable IDs, and the fake provider is
  explicitly excluded from gate satisfaction. A CLI smoke test then exposed
  that the generic fake response was not valid review JSON; the fake adapter
  now emits a strict simulated no-op payload for review tasks.
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
- The authorized live review returned nine hypotheses. Each was compared with
  the local code before any edit; generated provider text was not executed as
  instructions. All nine were addressed where locally confirmed, with new tests
  and a final self-review that found two further gaps: two code-review check
  slugs did not match their canonical checklist headings, and a review with no
  observed model identity could still earn gate credit. Both are now
  fail-closed, with canonical reference slugs and explicit tests.
- No live Jira read/write, GitHub, cloud, or dashboard result is asserted. The
  one live provider result is recorded above as untrusted hypotheses, never as
  verified findings. Jira bug reporting was exercised only through local
  HTTP/CLI stubs; no ticket was created and no comment was posted.

## Next steps

1. Obtain explicit human authorization before any push, publish, deployment,
   cloud resource creation, or additional live provider/Jira/GitHub call. Local
   commits are made in small reviewable batches.
2. Add executed behavioral evaluation and independently verified findings; one
   authorized live provider review produced only hypotheses.
3. Exercise live Claude/Jira/GitHub adapters, both Jira bug-report modes, and
   live Codex chat/write paths only after authorization and credential setup,
   recording exact commands and sanitized outcomes.
4. Pin CI actions to verified commit SHAs and produce a signed release before
   calling the project stable.
