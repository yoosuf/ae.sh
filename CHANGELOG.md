# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
No stable version has been released; all versions are pre-1.0 and may change
without compatibility guarantees.

## [Unreleased]

### Added

- A fail-closed Gemini provider extension point. `provider.default: gemini` is
  accepted and reported by `ae doctor`, but every invocation returns
  `not implemented`; no binary, API call, or model mapping is exercised.
- A GitHub App identity config extension point. `integrations.github.mode:
  github-app` validates an HTTPS origin, environment variable *names* for the
  app ID and installation token, and a service actor locally; the adapter
  refuses the mode until a live implementation is authorized.
- Fail-closed federated identity and external review attestation extension
  points. `integrations.identity.mode: federated` and
  `integrations.review.mode: external-attestation` validate HTTPS issuers,
  environment variable *names*, and short labels locally; `ae doctor` and config
  `SelfCheck` flag them, and both adapter boundaries return `not implemented`.
  No token, verifier, service call, approval claim, or gate satisfaction is
  added.
- `ae audit`, a read-only local report covering credential shapes, embedded vs
  canonical schemas, canonical references, generated skill copies, and CI action
  pin status. Unverified action SHAs are reported as `needs-authorization`; the
  report does not approve a release or satisfy an AE gate.
- Optional local evidence signing: `ae evidence export ... --sign` accepts
  `--key-file path` or `--key-env NAME` and writes an Ed25519 `signature.json`
  sidecar. `ae evidence verify <run-id> --bundle DIR [--key-file path]` checks export
  digests, chain links, and the signature offline. Key material stays in a local
  file; the environment option supplies only a variable *name* pointing to it.
  This is local authenticity, not external attestation.
- `ae eval --suite skills|all --execute --behavioral`, which runs 15 deterministic
  fake-provider scenarios over authored fixtures and asserts sanitized prompts,
  skill/route binding, output fields, and `Simulated`/`Usage` boundaries.
  `modelBehaviorExecuted` remains `false`; no model-quality pass rate is claimed.
- `ae dashboard snapshot --run <id> --out DIR`, which writes a sanitized,
  self-contained static viewer from local run, history, approval, evidence,
  trace, and review state. An optional React/Vite/TypeScript source viewer lives
  under `ui/`; no server, API, credentials, or workflow dependency is introduced.
- Strongest-route review contracts: expanded `code-review` plus `security-review`,
  `architecture-review`, and `test-review` skills, matching independent
  reviewers, evaluation fixtures, and canonical checklists. All declare
  `ae.model-class: independent-review` and never contain provider model IDs.
- Opt-in `ae review --provider ...`: runs a canonical review skill through a
  Claude/Codex adapter (offline fake provider included), resolves the fixed
  `independent-review` route from local config, strictly parses findings as
  hypotheses, records requested/actual model evidence, and fails closed on
  stale SHAs, missing mappings, malformed output, or unconfirmed live calls.
  The offline fake provider returns a simulated no-op review. A live Codex
  review was later invoked once in a throwaway checkout and returned nine
  hypotheses; Claude review was attempted but the CLI was not logged in. Those
  findings are not externally verified results.
- Specialist review tasks in the full `feature-development` lifecycle and
  risk-specific security, migration, and release workflows. Only an observed,
  non-simulated, model-selected provider review can satisfy the high-risk
  `independent-review` gate; local scans and fake output cannot.
- A curated `assets/docs/` visual layer for installation, getting started,
  architecture, integrations, evidence, security, roadmap, and profiles
  documentation, with descriptive alt text and local-only references.
- Provider-neutral model routing: canonical profiles in
  `framework/routing/profiles.yaml`, a `routing` JSON Schema, local
  `provider.routes` mappings, `ae.model-class` skill metadata, agent
  `routingProfile`, workflow task `routingProfile`, `ae chat --route`, and
  requested/actual model fields in provider evidence.
- Documentation for the task/risk/cost model matrix in
  `docs/model-routing.md`, with official-doc references and explicit
  "verify before pinning" caveats. No live model invocation was performed.
- Phase 0 documentation and decision records: architecture, getting started,
  installation, integrations, evidence, provider compatibility, threat model,
  and roadmap, plus `docs/implementation-plan.md` for the remaining
  `Planned / not implemented` backlog.
- Governance files: proposed Apache-2.0 license, NOTICE, contributing guide,
  security policy, code of conduct, and repository agent instructions.
- GitHub issue/PR templates, CODEOWNERS placeholder, and CI workflow skeleton.
- 15 canonical skills with evaluation fixtures, 10 workflow DAGs, two
  risk-scoped policies, profiles, agents, and templates.
- Local Go CLI (`cmd/ae`) with `init`, `doctor`, `validate`, `skills
  list/install/uninstall/export`, `plan`, `run`, `status`, `resume`, `cancel`,
  `approve`, `review --diff`, `verify`, `worktree`, `trace`, `evidence export`,
  `eval`, `version`, `chat`, and `review resolve`.
- Opt-in Jira Cloud issue source that accepts configured HTTPS issue links or
  bare keys, reads tokens only from the environment, sanitizes issue text, and
  requires explicit AE metadata in labels or the description.
- Opt-in `ae jira report-bug`: report a bounded, sanitized bug as a comment on
  an existing Jira ticket (`--issue`) or as a new ticket (`--project`, optional
  `--issue-type`). Live writes require `mode: jira-cloud` and `--confirm-mutate`;
  `--dry-run` previews without network, token, or evidence access. Optional
  `--run` evidence stores digests and identifiers, not raw bug text. Local
  HTTP/CLI-stub tests only; no live Jira write was performed.
- Opt-in provider chat with session continuity for fake, Claude Code, and Codex
  adapters, bounded turns/timeouts, managed-worktree requirements for live
  providers, and chained `kind: provider` evidence per turn.
- Opt-in constrained write modes for `run`, `resume`, and `autopilot`: Claude
  uses `Read,Write,Edit` without shell tools plus CLI settings that block file
  edits outside the managed worktree (requiring Claude Code v2.1.314 or later),
  and Codex uses `workspace-write`; both require `--confirm-live`, a managed
  worktree, and `--allow-writes`.
- Opt-in GitHub CLI adapter for PR metadata, checks, bounded diffs, and a
  separately authorized PR comment. Credentials remain environment-only and no
  push, PR creation, merge, or deploy action is implemented.
- `ae autopilot`, an opt-in orchestration loop that binds a validated managed
  worktree before any provider task (including pre-approval tasks), advances
  workflow tasks, runs real local checks in that workspace, scans workspace and
  PR diffs, records GitHub PR evidence, and stops at fail-closed approval/policy
  gates without pushing, creating PRs, merging, or deploying.
- SQLite `BindWorkspace` binds a validated worktree to a run once and refuses to
  re-point an already-bound run.
- Structured local review evidence: deterministic findings, `kind: review`
  persistence with `--run`, and `review resolve` recording human decisions as
  chained `kind: remediation` evidence. Unresolved findings block
  merge/release/security-signoff gates, and facts are reconstructed on resume.
- Bounded remediation attempts are now enforced from
  `Workflow.remediation.maxAttempts` and persisted when a failed attempt occurs.
- `ae verify` executes checked-in profile checks and a built-in secret-shape
  scan locally, then records commands, sanitized output, and real exit statuses
  as evidence.
- Canonical `java`, `python`, `laravel`, and `typescript` profiles with local
  format, lint, build, test, and dependency checks.
- Aggregate `all` profile that reuses every canonical profile's checks with
  de-duplication by gate and command, plus `docs/profiles.md`.
- `ae worktree prepare|clean` creates and removes detached worktrees only under
  `.ae/worktrees/<run-id>`; `ae run --workspace` accepts only those managed
  worktrees.
- `ae trace` reconstructs requirement → architecture → implementation →
  verification → review → release → feedback coverage from persisted evidence
  and marks uncovered stages `missing`.
- `ae eval --execute` runs deterministic structural checks over skill and
  security fixtures without invoking a model.
- `scripts/e2e-worktree.sh` verifies detached worktree preparation, managed
  workspace runs, local verification, and clean removal in a throwaway git
  repository.
- SQLite-backed run state with optimistic concurrency, migration from legacy
  approval rows, transitions, evidence references, approvals, and an outbox.
- Fail-closed policy checks for unknown semantics, missing evidence, stale
  approvals, self-approval, and risk-scoped gates/approval rules.
- Unit and integration tests for schema, util sanitization, policy, workflow
  DAGs, catalogue parsing, installer conflicts, SQLite restart/concurrency,
  engine lifecycle behavior, CLI worktree/profile/JSON-error safety, trace stage
  mapping, canonical profile loading, `all` aggregation, include-cycle rejection,
  and init profile validation.
- Unit tests for GitHub argument shapes, env-only tokens, read-only defaults,
  mutation denial, PR JSON parsing, Claude/Codex write-mode flags, GitHub config
  validation, workspace binding conflict handling, autopilot dry-run/no-state,
  live-confirmation denial, human-gate stop, self-approval denial, fake-provider
  completion with delegated approval, and unsafe `run` flag combinations.
- `scripts/e2e-offline.sh` for a credential-free run through validation, plan,
  structural evaluation, dry run, approvals, resume, blocked merge gate,
  local verification, traceability, and evidence export.
- `Makefile` with `check`, `e2e`, `e2e-worktree`, and individual verification
  targets, and `progress.md` recording locally verified commands.

### Changed

- Renamed the project short name and every user-facing identifier to `ae`: the
  CLI binary and entry point, config file, local state/evidence directories,
  environment-variable prefix, schema IDs, skill metadata keys, Jira labels,
  provider manifests, and Go module path. Legacy config/state directories are
  not migrated automatically; initialize a fresh project with `ae init`.
- Repository remote and source-clone documentation now use
  `git@github.com:yoosuf/ae.sh.git`; the GitHub repository is `yoosuf/ae.sh`.
- Canonical framework assets moved from top-level directories into `framework/`
  (`agents/`, `policies/`, `profiles/`, `schemas/`, `skills/`, `templates/`, and
  `workflows/`). Go loaders, tests, scripts, and docs now resolve those paths
  through `internal/framework/paths.go`; generated provider skill paths are
  unchanged.
- Approval gates accept multiple approvals with optional distinct roles;
  `approve` supports `--role`.
- Missing required gate evidence parks the run at a meaningful state with
  `StatusBlocked` instead of inventing a pass.
- CLI errors are emitted before run JSON so a failed command produces one JSON
  object in `--json` mode.
- Documentation and command status labels now distinguish the implemented
  local/offline subset from planned live integrations, including the boundary
  that `eval --execute` is structural, `--behavioral` uses only the fake provider,
  evidence signatures are local authenticity, `audit` is not release approval, the
  dashboard is a local read-only snapshot, and `review --diff` is not independent
  review.
- Verification commands execute only from checked-in profiles; missing tools
  are recorded as failures, never as skipped passes.
- `ae init` validates profile names before writing config, reports the full
  canonical profile list in help, and de-duplicates repeated selections.
- `ae validate` loads every canonical profile, including `all`, and reports the
  resolved profile list. Composite profiles support recursive `includes` with
  cycle rejection.
- Provider prompts require stable finding IDs so review evidence can be
  reconstructed and resolved after a restart. Fixtures remain the offline
  default; live Jira/provider calls require explicit opt-in configuration and
  flags.
- Approval records bind to a deterministic review-evidence digest. A later
  review finding or resolution invalidates approvals whose policy declares
  `review-findings-change`; local pattern scans and simulated provider output
  still cannot satisfy the high-risk `independent-review` gate. An observed
  provider review records the gate explicitly.
- Live Jira issue intake now requires `--confirm-live`; `plan` and `--dry-run`
  remain fixture-only and never fetch from a configured Jira Cloud site.
- Live provider runs require both `--confirm-live` and a managed AE worktree.
  `ae verify` can execute profile checks inside that worktree and records the
  workspace path in evidence.
- GitHub integration config is validated: `local-cli` or `disabled` modes only,
  HTTPS origin `baseURL`, and environment variable *names* for token/webhook
  secret settings.

### Deprecated

- Nothing yet.

### Removed

- Nothing yet.

### Fixed

- The canonical and embedded evidence schemas now accept `kind: github`, so
  `autopilot --github-pr` evidence records validate instead of being rejected at
  build time.
- Failed `ae eval --behavioral --json` runs emit one error envelope instead of a
  result object followed by a second JSON error.
- `ae init` now fails closed when the selected root does not contain the
  canonical `framework/` assets, rather than writing a configuration that
  cannot be validated later.
- Offline and worktree end-to-end checks now use throwaway Go build caches, so
  they do not depend on a writable shared host cache in CI or restricted
  sandboxes.
- Jira adapter tests use an in-process HTTP transport instead of binding a
  loopback TLS listener, keeping the test suite runnable where local listeners
  are prohibited.
- Catalogue parsing validates the normalized JSON document before accepting a
  skill.
- `legacy-refactoring` declares `design` as the producer of `plan.json`.
- Schema validation test compiles and runs.
- Installer tests now cover unmanaged-file conflicts, forced writes, and
  manifest-managed refreshes.
- JSON usage, verification, and evaluation failures emit one error envelope
  instead of a result payload followed by an error.
- `verify --profile` and `trace --format|--out` are accepted after the run ID,
  matching the documented CLI syntax.
- Validation output says no behavioral/model evaluation was executed, rather
  than implying no structural evaluation checks ran.
- CI validation uses `scripts/check-schemas.sh`, which initializes a temporary
  copy because `validate` requires local config.
- Status documentation distinguishes implemented detached worktree orchestration
  from planned live provider code changes and CI/deploy hooks.
- Worktree creation and cleanup use the verified git worktree root after path
  canonicalization, and redirect attempts are removed before returning.
- The built-in secret scan ignores explicitly synthetic `EXAMPLE`/`DUMMY`/
  `FAKE`/`SYNTHETIC` fixture markers while still counting real credential
  shapes.
- `scripts/uninstall.sh` accepts documented Claude and Codex skill roots and
  rejects `.codex/skills` and non-canonical skill names.
- `autopilot` no longer runs pre-approval provider tasks in the CLI directory:
  the managed worktree is bound and git-validated before the first provider
  call, including when a run stops at a human gate.
- Autopilot diff review now includes staged, unstaged, and scannable untracked
  text files; binary, oversized, symlinked, or escaping untracked paths fail
  closed. PR diffs are pattern-scanned before a run can complete.
- Workspace flags on `run`, `resume`, `chat`, and `autopilot` now require a
  registered git worktree whose real path matches the managed path, not merely
  a directory under `.ae/worktrees`; `autopilot` additionally requires the
  worktree dedicated to its own run.
- Claude write mode passes temporary permission settings that block file-tool
  access outside the working directory and requires a CLI version that supports
  those settings; older versions are rejected instead of silently unconfined.
- All evidence string fields, including actor, subject, and previous digest,
  are passed through credential-shape redaction before persistence.
- Config validation rejects credential literals in every environment-name
  field regardless of integration mode.
- Live provider reviews now resolve both pinned commits, require the base to be
  an ancestor of the candidate, require workspace HEAD to equal the candidate,
  reject uncommitted workspace changes, and verify the supplied diff digest
  against the committed change before any provider call. Run-supplied SHAs that
  conflict with persisted pins fail closed.
- Gate-eligible reviews must answer every required check class for the selected
  skill; empty, partial, or unknown-class coverage no longer earns credit.
  A provider that reports a different observed model, or cannot observe model
  identity at all, cannot satisfy `independent-review`; requested and observed
  identities are recorded explicitly and no unobserved identity is guessed.
- A passing test record labelled `independent-review` can no longer satisfy the
  review gate; only qualifying review evidence can.
- Review tasks inherit run-level write authorization no longer: independent
  reviewers are forced read-only, and every task is intersected with its
  canonical agent write scope.
- Specialist review findings now feed the shared `review.findings` fact in the
  same run, so a clean general review with one specialist finding still runs
  bounded remediation.
- Concurrent provider review appends reserve sequence and predecessor inside
  one immediate SQLite transaction; files are persisted only after the row
  commits, and immutable records can be rewritten safely if needed.
- Diff reading now rejects non-regular paths and enforces the 1 MiB limit while
  reading instead of after loading the whole file.
- The 100-finding limit applies only to provider-backed results; local pattern
  scans keep their previous unbounded behavior and remain schema-valid.

### Security

- Nothing yet. See `SECURITY.md` for private reporting; no vulnerabilities are
  currently known or published.
