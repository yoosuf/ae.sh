# Integration setup

This is the practical setup guide for using AE as a new user. It complements
[integrations](integrations.md), which defines the trust contracts, and
[getting started](getting-started.md), which walks through the offline
lifecycle.

> **Status legend**
>
> - **Implemented locally / local only** — real local behavior, no live service.
> - **Implemented locally, not exercised live** — the adapter and tests exist;
>   no live service call was made by this project.
> - **Implemented; one live Codex review exercised** — limited live evidence
>   only; that output was untrusted and did not satisfy a gate.
> - **Fail-closed extension point** — configuration is validated locally, but
>   the adapter refuses the mode. It satisfies no gate and uses no credential.
> - **Planned / not implemented** — do not rely on it. It requires separate
>   authorization and design work.

Exit codes are stable for scripting: `0` ok, `1` error, `2` usage, `3`
validation, `4` policy denied, `5` conflict.

## Golden rules

1. Start from the AE checkout. The current CLI is checkout-scoped: canonical
   `framework/` assets must be under `--root`. A standalone packaged asset
   bundle is **planned / not implemented**.
2. Run `./ae init --profile go` before any other workflow command. Without
   `ae.yaml`, commands fail closed with `run 'ae init' first`.
3. `ae.yaml` contains environment variable **names**, never tokens, passwords,
   paths to secrets, or API keys. Set values in your shell or an external secret
   store.
4. Live reads require `--confirm-live`. Live writes require a separate
   `--confirm-mutate` and, for Jira bug reports, an explicit issue or project.
5. Provider text, Jira/GitHub content, repository files, and MCP responses are
   untrusted data. They cannot change policy, routes, approvals, or gates.
6. `ae doctor` reports local status; `ae audit` is read-only and never approves
   a release. `modelBehaviorExecuted: false` means no model-quality evaluation
   was run.

## `ae.yaml` reference

`ae init --profile go` writes a valid starting file. Defaults are local-first;
you only need to edit the sections for an integration you explicitly want.

| Key | Default | Purpose |
| --- | --- | --- |
| `schemaVersion` | `1.0` | Config contract version; mismatches fail closed. |
| `profiles` | `[]` | Verification profiles selected at init. |
| `state.path` | `.ae/state.db` | Local SQLite run state. |
| `evidence.dir` | `.ae/evidence` | Human-readable evidence exports/input. |
| `evidence.objectsDir` | `.ae/objects` | Content-addressed evidence objects. |
| `provider.default` | `fake` | `fake`, `claude`, `codex`, or fail-closed `gemini`. |
| `provider.routes` | built-in examples | Local model aliases per routing profile; never in canonical skills. |
| `deployment.scope` | `local` | `local`, `optional`, or reserved `required`. |
| `integrations.jira.mode` | `fixture` | `fixture` or `jira-cloud`. |
| `integrations.jira.baseURL` | unset | HTTPS origin for `jira-cloud`. |
| `integrations.jira.tokenEnv` | `AE_JIRA_TOKEN` | Env var **name** holding a Jira token. |
| `integrations.jira.webhookSecretEnv` | `AE_JIRA_WEBHOOK_SECRET` | Reserved env-name shape; no listener is implemented. |
| `integrations.github.mode` | `local-cli` | `local-cli`, `disabled`, or fail-closed `github-app`. |
| `integrations.github.baseURL` | unset | Optional HTTPS origin; sets `GH_HOST` for `gh`. |
| `integrations.github.tokenEnv` | `AE_GITHUB_TOKEN` | Env var **name** passed to `gh` as `GH_TOKEN`. |
| `integrations.github.webhookSecretEnv` | `AE_GITHUB_WEBHOOK_SECRET` | Reserved env-name shape. |
| `integrations.github.appIDEnv` / `installationTokenEnv` / `actor` | unset | GitHub App shape only; adapter fails closed. |
| `integrations.identity.mode` | `local-manual` | `local-manual` or fail-closed `federated`. |
| `integrations.identity.issuer` / `audience` / `tokenEnv` / `actorClaim` | unset | Federated shape only; no verifier. |
| `integrations.review.mode` | `local` | `local` or fail-closed `external-attestation`. |
| `integrations.review.issuer` / `verifierEnv` | unset | External attestation shape only; no verifier. |

All `tokenEnv`, `appIDEnv`, `installationTokenEnv`, `webhookSecretEnv`, and
`verifierEnv` values are validated as environment variable names. A value that
looks like a credential is rejected by `ae validate` and `ae audit`.

## Setup matrix

| Surface | Status | Config keys | Env var name | Main command(s) | Required safety flags |
| --- | --- | --- | --- | --- | --- |
| Offline fake provider | Implemented locally | `provider.default: fake` (default) | none | `ae plan`, `ae run --provider fake` | none |
| Jira Cloud intake | Implemented locally, not exercised live | `integrations.jira.mode: jira-cloud`, `baseURL`, `tokenEnv` | `AE_JIRA_TOKEN` by default | `ae run --issue KEY`, `ae autopilot --issue KEY` | `--confirm-live` |
| Jira bug reporting | Implemented locally, not exercised live | same as above | `AE_JIRA_TOKEN` by default | `ae jira report-bug` | `--confirm-mutate`; `--dry-run` for preview |
| GitHub CLI PR reads | Implemented locally, not exercised live | `integrations.github.mode: local-cli` (default), optional `tokenEnv` | `AE_GITHUB_TOKEN` by default | `ae autopilot --github-pr REF` | `--confirm-live` |
| GitHub PR comment | Implemented locally, not exercised live | same as above | `AE_GITHUB_TOKEN` by default | `ae autopilot --github-pr REF --comment` | `--confirm-live --confirm-mutate` |
| Claude Code / Codex chat, review, writes | Implemented locally, not exercised live | `provider.default`, `provider.routes` | provider CLI auth | `ae chat`, `ae review --provider`, `ae run/resume/autopilot` | `--confirm-live`; `--workspace` for live; `--allow-writes` for edits |
| Gemini provider | Fail-closed extension point | `provider.default: gemini` | none | `ae doctor` | refuses `Run`/`Cancel` |
| GitHub App identity | Fail-closed extension point | `integrations.github.mode: github-app`, `appIDEnv`, `installationTokenEnv`, `actor` | `AE_GITHUB_APP_ID`, `AE_GITHUB_INSTALLATION_TOKEN` by default | `ae doctor` | adapter refuses mode |
| Federated approver identity | Fail-closed extension point | `integrations.identity.mode: federated`, `issuer`, `tokenEnv`, `audience`, `actorClaim` | `AE_IDENTITY_TOKEN` by default | `ae doctor` | adapter refuses mode; approvals stay `local-manual` |
| External review attestation | Fail-closed extension point | `integrations.review.mode: external-attestation`, `issuer`, `verifierEnv` | `AE_REVIEW_VERIFIER` by default | `ae doctor` | adapter refuses mode; no gate credit |
| Evidence signing | Implemented locally | none | user-chosen key env name | `ae evidence export --sign`, `ae evidence verify` | key file or `--key-env NAME`, never a secret value |
| Dashboard snapshot | Implemented locally | none | none | `ae dashboard snapshot` | read-only, static output |
| Behavioral eval | Implemented locally with fake provider | none | none | `ae eval --behavioral` | `modelBehaviorExecuted: false` |
| Autopilot | Implemented locally, not exercised live | none | depends on enabled integrations | `ae autopilot` | `--dry-run` preview; `--confirm-gates` for delegated approvals |
| Managed worktree | Implemented locally | none | none | `ae worktree prepare/clean` | paths outside managed root refused |
| Cloud/AWS adapters, PR create/merge/push, hosted dashboard | Planned / not implemented | none | none | none | requires explicit authorization |

## 1. Offline setup (no credentials)

```sh
git clone git@github.com:yoosuf/ae.sh.git ae.sh
cd ae.sh
go build -o ae ./cmd/ae

./ae init --profile go
./ae validate
./ae doctor

ISSUE=examples/offline-workflow/issue-PROJ-101.json
./ae plan --issue "$ISSUE"
./ae run feature-development --issue "$ISSUE" --provider fake --dry-run
./ae run feature-development --issue "$ISSUE" --provider fake --actor demo-user
```

The shorthand `PROJ-101` also resolves to that fixture. The fake provider is
the only end-to-end provider path exercised by the offline E2E script. It marks
output `simulated: true`; it cannot satisfy build/lint/test or independent
review gates. `--allow-writes` is rejected with the fake provider. Use `ae
verify` to record real local results.

## 2. Skills for Claude Code or Codex

Skills are instruction contracts, not code. Never hand-edit generated copies.

```sh
./ae skills list
./ae skills install --provider claude --scope user --dry-run
./ae skills install --provider codex --scope project
./ae skills install --provider all --scope project --dry-run
```

Canonical source is `framework/skills/<slug>/SKILL.md`. Generated locations:

- Claude Code user scope: `~/.claude/skills/<slug>/SKILL.md`
- Claude Code project scope: `.claude/skills/<slug>/SKILL.md`
- Codex user scope: `~/.agents/skills/<slug>/SKILL.md`
- Codex repo scope: `.agents/skills/<slug>/SKILL.md`

`.codex/skills` is not a documented Codex path and is rejected by the safety
scripts. To remove only manifest-managed copies, use `ae skills uninstall`. To
generate plugin bundles locally, use `ae skills export --out dist/plugins`.

## 3. Jira Cloud intake and bug reporting

**Status:** implemented locally with HTTP stubs; no live Jira call was made.

1. Create a Jira Cloud API token in your own account and store it in your
   secret manager.
2. Export it under the configured environment variable name. Do not put the
   value in `ae.yaml`, a flag, a prompt, a fixture, or an evidence record:

   ```sh
   # Example only: set AE_JIRA_TOKEN from your secret manager before running.
   ./ae doctor
   ```

3. Configure `ae.yaml`:

   ```yaml
   integrations:
     jira:
       mode: jira-cloud
       baseURL: https://your-site.atlassian.net
       tokenEnv: AE_JIRA_TOKEN
       webhookSecretEnv: AE_JIRA_WEBHOOK_SECRET
   ```

   `baseURL` must be an HTTPS origin with no path, query, fragment, or embedded
   credentials. Links must use the same host. The webhook secret field is a
   reserved env-name shape; no webhook listener is implemented.

4. Verify the issue has AE metadata. Required labels are `ae-acceptance:` and
   `ae-path:`; optional labels are `ae-semantics:`, `ae-dependency:`, and
   `ae-base-sha:`. Description sections are accepted as the equivalent. Missing
   metadata fails closed.

5. Start a live intake (after building and initializing):

   ```sh
   ./ae run feature-development --issue TEAM-123 --confirm-live --actor you
   # or
   ./ae autopilot --issue TEAM-123 --confirm-live
   ```

   `plan` and `--dry-run` remain fixture-only and never fetch a live issue.

6. Report a bug, preview first:

   ```sh
   ./ae jira report-bug --project TEAM --title "Crash on save" \
     --description-file bug.md --dry-run
   ```

   Then write only with explicit mutation confirmation:

   ```sh
   ./ae jira report-bug --project TEAM --title "Crash on save" \
     --description-file bug.md --confirm-mutate --run <run-id>
   ```

   Use `--issue TEAM-123` instead of `--project TEAM` to comment on an existing
   ticket. Bug text is bounded, sanitized, credential-shape checked, and sent as
   ADF. A successful Jira response satisfies no AE gate.

## 4. GitHub CLI reads and comments

**Status:** implemented locally with CLI stubs; no live GitHub call was made.

1. Install and authenticate `gh` yourself:

   ```sh
   gh auth status
   ```

2. Keep the default local CLI mode, or set it explicitly:

   ```yaml
   integrations:
     github:
       mode: local-cli
       # tokenEnv: AE_GITHUB_TOKEN
   ```

   When `tokenEnv` is set, AE removes inherited `GH_TOKEN`, `GITHUB_TOKEN`, and
   `GH_HOST` from the child environment, then passes only that value as
   `GH_TOKEN`; it is never written to arguments, evidence, or logs. Use
   `mode: disabled` to refuse GitHub access. `baseURL`, when set, must be an
   HTTPS origin and is passed as `GH_HOST`.

3. Record PR metadata, checks, and a bounded diff as evidence:

   ```sh
   ./ae autopilot --resume <run-id> --github-pr 123 --confirm-live
   ```

4. Add the only mutating GitHub action, a sanitized PR comment:

   ```sh
   ./ae autopilot --resume <run-id> --github-pr 123 --comment \
     --confirm-live --confirm-mutate
   ```

AE never creates a branch or PR, pushes, merges, or deploys. `ae review --pr
123` without `--diff` is **planned / not implemented**. For local diff scanning
use `ae review --diff path/to.diff --run <run-id>`.

## 5. Claude Code and Codex live providers

**Status:** implemented locally with executable stubs; no live model run or
file edit was made by this project.

1. Install the provider CLI and authenticate outside AE. AE does not accept
   provider credentials as flags, config, prompts, or fixtures.
2. Create a run and a managed detached worktree:

   ```sh
   ./ae run feature-development --issue PROJ-101 --provider fake --actor you
   ./ae worktree prepare <run-id>
   ```

3. Check model routes before a live command. Model IDs live only in local
   `provider.routes`; canonical skills never name a model. Recheck IDs against
   the installed provider CLI; see [model routing](model-routing.md).

   ```yaml
   provider:
     default: codex
     routes:
       independent-review:
         codex: <verified-model-alias-or-id>
   ```

4. Read-only chat:

   ```sh
   ./ae chat --provider codex --message "Summarize the plan" \
     --run <run-id> --workspace .ae/worktrees/<run-id> --confirm-live
   ```

   Later turns may reuse `--session <session-id>`. `chat` is read-only.

5. Provider-backed review that can satisfy `independent-review` only when a
   non-simulated result reports an observed model matching the configured
   route:

   ```sh
   ./ae review --diff path/to.diff --provider codex --skill code-review \
     --candidate-sha <sha> --base-sha <sha> --run <run-id> \
     --workspace .ae/worktrees/<run-id> --confirm-live
   ```

   Gate credit requires pinned candidate/base SHAs, a clean managed workspace,
   a matching diff, a non-simulated provider result, and an observed model that
   matches the configured route. Findings remain untrusted hypotheses until
   `ae review resolve` records a human decision. One earlier authorized Codex
   review was untrusted because the adapter could not observe the model.

6. Constrained writes require `run`, `resume`, or `autopilot` plus
   `--allow-writes`, `--confirm-live`, and a managed worktree:

   ```sh
   ./ae run feature-development --issue PROJ-101 --provider codex \
     --workspace .ae/worktrees/<run-id> --confirm-live --allow-writes
   ```

   Claude writes use `Read,Write,Edit` only, with out-of-worktree file access
   blocked (Claude Code v2.1.314+). Codex uses `workspace-write`. Output is
   still untrusted until real local checks and gates pass.

## 6. Fail-closed extension points

These shapes are accepted and validated locally so users can see the boundary.
They are not live implementations and cannot satisfy a gate.

### Gemini

```yaml
provider:
  default: gemini
```

`ae doctor` reports `provider/gemini` as `extension-point`; `Run` and `Cancel`
return `not implemented`. No binary, API, credential, or model mapping is used.

### GitHub App identity

```yaml
integrations:
  github:
    mode: github-app
    baseURL: https://github.example.com
    appIDEnv: AE_GITHUB_APP_ID
    installationTokenEnv: AE_GITHUB_INSTALLATION_TOKEN
    actor: ae-bot
```

The adapter refuses `github-app`. Live GitHub App/service identity, PR
creation, merge, and push are **planned / not implemented**.

### Federated approver identity

```yaml
integrations:
  identity:
    mode: federated
    issuer: https://id.example.com/realms/ae
    tokenEnv: AE_IDENTITY_TOKEN
    audience: ae-cli
    actorClaim: preferred_username
```

The issuer may have a path but no userinfo, query, or fragment. The adapter
refuses `federated`; approvals remain `local-manual` and cannot satisfy
critical-change or organization-identity gates.

### External review attestation

```yaml
integrations:
  review:
    mode: external-attestation
    issuer: https://attest.example.com
    verifierEnv: AE_REVIEW_VERIFIER
```

The adapter refuses `external-attestation`. It never reads the verifier value,
accepts an attestation, or credits `independent-review`.

## 7. Evidence, dashboard, eval, and autopilot

### Local evidence and signing

Keys are PKCS#8 PEM Ed25519. Keep the private key outside the repository with
restrictive permissions:

```sh
mkdir -p "$HOME/.local/share/ae-keys"
openssl genpkey -algorithm ED25519 -out "$HOME/.local/share/ae-keys/ae.key"
chmod 600 "$HOME/.local/share/ae-keys/ae.key"
openssl pkey -in "$HOME/.local/share/ae-keys/ae.key" -pubout \
  -out "$HOME/.local/share/ae-keys/ae.pub"

./ae evidence export <run-id> --format both --out dist/evidence-<run-id>
./ae evidence export <run-id> --format both --out dist/evidence-<run-id> \
  --sign --key-file "$HOME/.local/share/ae-keys/ae.key"
./ae evidence verify <run-id> --bundle dist/evidence-<run-id> \
  --key-file "$HOME/.local/share/ae-keys/ae.pub"
```

`--key-env NAME` accepts the **name** of an environment variable whose value is
a path to the key file; it never accepts key material or a key value. `--sign`
requires JSON evidence, so `--format markdown` is rejected. Signing provides
local authenticity and offline verification only, not external notarization or
a release approval.

### Read-only dashboard

Requires an existing run ID in local state.

```sh
./ae dashboard snapshot --run <run-id> --out dist/dashboard-<run-id>
```

Open `index.html` directly from disk. There is no server, API, credential, or
write path. The optional React source under `ui/` is for contributors; no AE
command depends on npm. For contributor UI checks, `cd ui && npm test --offline`
works without installing dependencies.

### Evaluation

```sh
./ae eval --suite all --execute
./ae eval --suite all --execute --behavioral
```

The default is structural fixture checks. `--behavioral` adds deterministic
fake-provider scenarios and is rejected for `--suite security`. Neither invokes
a model; `modelBehaviorExecuted` stays `false`.

| Command | `mode` | `executed` | Meaning |
| --- | --- | --- | --- |
| `ae eval --suite all` | inventory only | `false` | Lists authored checks without executing them. |
| `ae eval --suite all --execute` | `structural-fixture-checks` | `true` | Executes deterministic fixture checks. |
| `ae eval --suite all --execute --behavioral` | `structural-fixture-checks+behavioral-fake-provider` | `true` | Adds fake-provider plumbing checks; still no model. |

### Autopilot

```sh
./ae autopilot --issue PROJ-101 --dry-run
./ae autopilot --issue PROJ-101 --approver reviewer-1 --confirm-gates
```

`--dry-run` writes no state and calls no service. Delegated local approvals
require both `--approver` and `--confirm-gates`; `--approver-role` records a role
and defaults to `local-delegate`. These approvals are `local-manual` and cannot
satisfy live identity, distinct-role, or independently verified review
requirements. Autopilot binds a run-owned worktree before provider work and
never pushes, creates a PR, merges, or deploys.

Managed worktrees are created with `./ae worktree prepare <run-id>` and removed
with `./ae worktree clean <run-id>`. Only the registered detached path under
`.ae/worktrees/<run-id>` is accepted; symlink escapes, branches, arbitrary
paths, and force-removal are refused. `clean` fails if the worktree has
changes, so review or commit intentionally before cleanup.

## 8. Verify your setup

From the AE checkout:

```sh
make check
cd ui && npm test --offline && cd ..
./ae doctor --json
./ae audit --json
```

`make check` runs format checks, build, vet, unit/race tests, schema
validation, and both offline E2E scripts. `ae audit` is local and read-only;
it reports `needs-authorization` for CI action SHA pinning until an authorized
verified lookup is performed. It does not approve a release.

## Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| `ae.yaml not found (run 'ae init' first)` | No local config. | Run `./ae init --profile go` from the AE checkout, or pass `--root` to a directory that has config. |
| `run init from the AE checkout or pass --root` | Canonical `framework/` assets are not under the current root. | Run from the checkout or use `--root /path/to/ae.sh`. Standalone packaged assets are planned. |
| `canonical framework assets not found` | Checkout is incomplete or `--root` points elsewhere. | Re-clone or pass the correct root; do not hand-create assets. |
| `doctor` shows `repository: fail` before init | `doctor` was run before `ae.yaml` exists. | Run `./ae init --profile go`, then `./ae doctor`. |
| `verify` fails with a missing tool | Selected profile expects a toolchain not installed. | Install the tool or choose only profiles for your stack; see [profiles](profiles.md). |
| `gh ... failed` | `gh` is missing, unauthenticated, or lacks repo access. | Run `gh auth status`; authenticate outside AE. |
| Provider CLI not found | Optional live CLI is not installed. | Install Claude Code or Codex, or stay with `--provider fake`. |
| `not implemented` from an extension point | Fail-closed Gemini/GitHub App/federated identity/attestation mode. | Use the offline path; do not expect gate credit. |
| `npm install --offline` fails with `ENOTCACHED` | UI dependencies are not cached locally. | UI source is optional. `npm test --offline` needs no install; typecheck/build require a normal install when network access is authorized. |

## What is not available yet

- Live Jira calls, live GitHub calls, live provider runs, or live provider file
  edits were not exercised by this project. They require user opt-in and their
  own credentials.
- GitHub App/service identity, PR creation, merge, push, federated approver
  identity, externally verified review, cloud adapters, hosted/multi-user
  dashboard, and release signing/publishing are **planned / not implemented**.
- No push, deploy, PR, merge, cloud resource, or external attestation is
  performed by AE in the implemented local path.
