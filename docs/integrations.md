# Integrations

All integrations sit behind ports in the optional adapter layer. Core packages never import provider SDKs. The implemented path is local filesystem, config, SQLite state, offline fixtures, fake provider, generated skill copies, local approval/evidence lifecycle commands, opt-in Jira Cloud REST intake, opt-in GitHub CLI PR reads/comments, opt-in provider conversations with constrained write modes, and an opt-in `autopilot` orchestration loop. Live services were not called in this session; only local HTTP/TLS and CLI-stub tests were run.

## Jira Cloud

Configured opt-in adapter (implemented locally, not exercised live):

```yaml
integrations:
  jira:
    mode: jira-cloud
    baseURL: https://your-site.atlassian.net
    tokenEnv: AEF_JIRA_TOKEN
```

- A live `aef run ... --issue TEAM-123 --confirm-live` or `aef autopilot --issue TEAM-123 --confirm-live` accepts a bare issue key or a supported Jira Cloud browse URL when `mode: jira-cloud` is configured. `plan` and `--dry-run` remain fixture-only and never fetch a live issue.
- `baseURL` must be an HTTPS origin with no path, query, fragment, or embedded credentials. A supplied link must use the same host; HTTP links, other hosts, userinfo, and token-bearing query strings are rejected.
- The token is read only from the environment variable named by `tokenEnv`; it is sent as an `Authorization: Bearer` header and never placed in flags, config, prompts, fixtures, URLs, or evidence.
- The adapter requests `summary`, `description`, and `labels` from `/rest/api/3/issue/<key>` and normalizes the response into the provider-neutral `integration.Issue` contract. Response text is sanitized and validated against `framework/schemas/issue.schema.json`.
- AEF metadata is mandatory: `aef-acceptance:`, `aef-path:`, optional `aef-semantics:`, `aef-dependency:`, and `aef-base-sha:` labels, or the equivalent description sections. Missing acceptance/path metadata fails closed; no criteria are invented.
- Fixtures remain the offline default. A Jira link without `mode: jira-cloud`, or a live key without `--confirm-live`, is denied with exit code 4.
- An approved Jira MCP server remains a design target, not an implemented adapter.

## GitHub

Two distinct modes must be kept separate:

1. **Local CLI mode (implemented locally, not exercised live):** uses the user's authenticated `gh` CLI on the developer machine. Identity is the local user.
2. **GitHub App / service identity mode (planned / not implemented):** a GitHub App with least-privilege permissions would provide auditable actor identity for approvals and PR operations. It is required for later unattended/server-side flows and does not exist today.

Config validation accepts `integrations.github.mode: local-cli` (default) or
`disabled`. `baseURL`, when set, must be an HTTPS origin with no path, query,
fragment, or credentials. `tokenEnv` and `webhookSecretEnv` must be environment
variable *names*. Credentials are never placed in config, flags, prompts,
fixtures, URLs, or evidence.

The local CLI adapter uses argument arrays and supports only:

- `gh pr view` for PR metadata, reviews, and comments.
- `gh pr checks` for check conclusions.
- `gh pr diff` for a bounded diff that can be scanned locally.
- `gh pr comment` for a sanitized comment, and **only** with separate
  `--comment --confirm-mutate` authorization.

`aef autopilot --github-pr <ref> --confirm-live` records those reads as chained
`kind: github` evidence and blocks on failing checks. It never creates a branch
or PR, pushes, merges, or deploys. `aef review --pr 123` without `--diff`
remains **planned / not implemented**; use `--github-pr` on `autopilot` for PR
state evidence only.

`aef review --diff <path> [--run id]` is a deterministic local pattern scan that
reports hypotheses with `verified: false`; it is not an independent review. With
`--run`, the result is written as chained `kind: review` evidence. `aef review
resolve --run id --finding rev-... --status fixed|accepted|false-positive --reason
text` records a human decision as chained `kind: remediation` evidence.
Unresolved findings block merge, release, and security-signoff gates; resolution
does not turn a hypothesis into an independently verified finding. Approvals
bound to a review-evidence digest are invalidated when later review or
resolution evidence changes the digest (`review-findings-change`). High-risk
policies still require the `independent-review` gate, which local pattern
scanning cannot satisfy, so high-risk merges remain blocked until an
independently verified review path exists.

## Claude Code

- Skills are Markdown instruction contracts at canonical `framework/skills/<slug>/SKILL.md`.
- Materialized user scope: `~/.claude/skills/<slug>/SKILL.md`.
- Materialized project scope: `.claude/skills/<slug>/SKILL.md`.
- Plugin scope: `<plugin>/skills/<slug>/SKILL.md` with `.claude-plugin/plugin.json`; `aef skills export` generates bundle directories locally.
- Paths above were verified on 2026-11-25. Recheck before use; see [provider compatibility](provider-compatibility.md).
- The Claude Code CLI adapter can run noninteractive prompts from argument arrays. `aef chat --provider claude --confirm-live --workspace <managed-worktree>` opts in to a live conversation; `--resume <session-id>` is used on later turns when the CLI returns `session_id`. `run`, `resume`, and `autopilot` additionally accept `--allow-writes`, which switches tools from `Read` to `Read,Write,Edit` without a shell tool and passes temporary permission settings that block file-tool reads and writes outside the managed worktree. Claude Code v2.1.314 or later is required for those settings; older binaries are rejected. No live model run, resume, or file edit was performed; local stub tests verify both argument shapes.

## Codex

- User scope: `$HOME/.agents/skills/<slug>/SKILL.md`.
- Repo scope: `$CWD/.agents/skills` or `$REPO_ROOT/.agents/skills`.
- Admin scope: `/etc/codex/skills`.
- Optional metadata: `agents/openai.yaml`.
- **`.codex/skills` is not a documented Codex skill path** — do not use or claim it.
- Both Claude Code and Codex follow the agentskills.io open standard.
- The Codex CLI adapter can run noninteractive prompts from argument arrays. Persistent `aef chat --provider codex --confirm-live --workspace <managed-worktree>` uses `codex exec` without `--ephemeral`; later turns use `codex exec resume <thread-id>`. `run`, `resume`, and `autopilot` additionally accept `--allow-writes`, which changes the sandbox from `read-only` to `workspace-write`. No live model run, resume, or file edit was performed; local stub tests verify both argument shapes and thread ID capture.

## Provider conversations

`aef chat` is a bounded, opt-in conversation path over the existing provider
port:

- `--provider` is required. `fake` works offline and simulates responses;
  `claude` and `codex` are live providers.
- Exactly one of `--message` or `--script` is accepted. Scripts are JSON files
  with a `messages` array and at most 20 turns; each turn has a 10s–30m timeout.
- Live providers require `--confirm-live` and `--workspace` pointing to an
  AEF-managed detached worktree. Credentials come from the provider's own
  environment; AEF never accepts them as flags.
- Session IDs are preserved across turns. If a provider advertises session
  metadata but returns no session ID, the command fails closed.
- With `--run`, each turn writes sanitized `kind: provider` evidence with the
  prompt, response, session ID, model, simulation flag, usage, and a chained
  `previousDigest`.
- Provider output remains untrusted text. It is not a check pass, an approval,
  or an independently verified review.
- `chat` is read-only. File edits require `run`, `resume`, or `autopilot` with
  `--allow-writes`, a live provider, `--confirm-live`, and a validated AEF-managed
  worktree. Writes are still untrusted until real local checks and gates pass.

## Autopilot (opt-in orchestration)

`aef autopilot` is an explicit opt-in loop that binds a managed detached
worktree to a run, advances workflow tasks, runs real local verification in that
worktree, scans the workspace diff, and stops at every approval or policy gate:

- `--dry-run` is the safe preview mode for planning; it writes no state and
  makes no service call. A non-dry run requires `--issue` or `--resume`.
- Live Jira, provider, and GitHub **reads** require `--confirm-live`.
- Provider file edits require a live provider plus `--allow-writes` and a managed
  worktree.
- Delegated local approvals require both `--approver <name>` and
  `--confirm-gates`. The approver must differ from the run actor. These approvals
  are recorded as `local-manual` and cannot satisfy critical-change identity,
  distinct-role, or independently verified review requirements.
- GitHub PR comments require `--github-pr`, `--comment`, and `--confirm-mutate`.
  That is the only mutating GitHub action in AEF.
- The command never pushes, creates a PR, merges, deploys, or invents a check or
  review result. High-risk work still blocks at `independent-review`; unresolved
  pattern-scan findings must be resolved with `aef review resolve`.

## Git worktrees

Implemented local isolation model: `aef worktree prepare <run-id>` creates a
detached worktree under `.aef/worktrees/<run-id>` from the recorded `HEAD`, and
`aef worktree clean <run-id>` removes it without `--force`. Paths outside the
managed root, symlink escapes, unsafe run IDs, branches, commits, pushes, and
deployments are rejected. `aef run --workspace <path>` accepts only those
managed worktrees and records the resolved path in run state and evidence. This
is exercised by `scripts/e2e-worktree.sh` in a throwaway git repository. SQLite
`BindWorkspace` attaches a workspace once and refuses to re-point a run already
bound to a different directory. `git-worktree-management` is also one of the 12
Phase 1 instruction contracts. Live provider code changes inside the worktree
are implemented only behind explicit opt-in flags and were **not exercised
live**; live CI/deploy hooks do not exist.

## MCP servers

MCP may be used for tool access (for example an approved Jira MCP server), but:

- MCP is optional; the framework must work without it.
- MCP output is untrusted data, never policy or control input.
- Tool descriptions and responses can contain prompt injection; sanitize and constrain them.
- Only allow explicitly configured servers; no automatic discovery of arbitrary servers.
- Credentials for MCP servers follow the same no-flags/no-prompts rule.

## Cloud (AWS) adapters — planned / not implemented

AWS usage is optional and adapter-only: API Gateway webhook ingress, SQS queueing, Step Functions Standard orchestration, ECS Fargate execution, DynamoDB state projection, S3 evidence artifacts, Secrets Manager for credentials, CloudWatch/OTel telemetry, and Terraform reference architecture. No cloud resources are created by Phase 0–2, and no account or deployment is assumed.

## Dashboard — planned / not implemented

An optional React/Vite/TypeScript dashboard may read local state/evidence in a later phase. It is not required for any workflow and does not exist yet.
