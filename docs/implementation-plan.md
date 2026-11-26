# Remaining work: implementation plan

This document turns the project's `Planned / not implemented` labels into an
ordered, checkable plan. It is a planning contract, not a claim that any item
below is working. Status labels match `README.md` and `progress.md`:

- **Implemented locally** — code and local tests exist; no live service was used.
- **Planned / not implemented** — design only; no code ship is implied.
- **Needs authorization** — live calls, writes, or cloud/push/PR actions that
  require explicit human approval before an agent may perform them.
- **In progress this session** — the scoped items being implemented alongside
  this document; their real status is recorded in `progress.md` after testing.

Nothing here authorizes a push, publish, deploy, PR creation or merge, cloud
resource creation, or a live provider/service call. Those gates remain human.
Credentials stay in environment variables or an external secret store; they are
never placed in flags, config, prompts, fixtures, URLs, or commits.

## 1. Inventory

| Item | Area | Current status | Gate / dependency |
| --- | --- | --- | --- |
| Live Jira/Claude/Codex/GitHub calls | Phase 3 | Implemented locally as opt-in adapters; not exercised live | Needs authorization + credentials |
| Live provider code edits | Phase 3 | Constrained write adapters exist; not exercised live | Needs authorization + managed worktree |
| Real CI/deploy hooks | Phase 3 | Planned / not implemented | Needs authorization; repo rules forbid deploy |
| Federated approver identity | Phase 3, 4 | Planned / not implemented | External identity provider decision |
| GitHub App / service identity | Phase 4 | Planned / not implemented | App registration + credentials |
| PR creation, merge, push | Phase 4 | Planned / not implemented; intentionally absent | Needs explicit authorization; repo rules forbid agent push/merge |
| Merge signal automation | Phase 4 | Planned / not implemented | Depends on GitHub App identity + verified review |
| Externally verified findings | Phase 4 | Planned / not implemented; provider output is untrusted hypotheses | Needs authorization + independent provider/verification design |
| Gemini adapter | Provider compatibility | Planned / not implemented; model IDs are forward-compatibility only | Optional adapter; no core dependency |
| Full skill catalogue | Phase 5 | Planned / not implemented; 15 skills authored | Editorial work + versioning |
| Executed behavioral evaluation | Phase 5 | Structural fixtures implemented; `modelBehaviorExecuted: false` | Local fake-provider execution feasible now |
| Optional AWS adapters + Terraform | Phase 6 | Planned / not implemented | Needs authorization; no cloud resources may be created |
| Read-only dashboard | Phase 7 | Planned / not implemented | Local static snapshot feasible now |
| Multi-user features | Phase 7 | Planned / not implemented | Depends on hosted identity; not mandatory |
| Stable SemVer 1.0 + provenance | Phase 7 | Planned / not implemented; licensing proposed | Release checklist + signing |
| Signed evidence bundles | Cross-cutting | Planned / not implemented | Local signing key handling feasible now |
| Cloud artifact storage / transparency log | Cross-cutting | Planned / not implemented | Needs authorization + external service |
| Security audit | Cross-cutting | Planned / not implemented | Local review/check feasible now |
| SHA-pinned CI actions | Cross-cutting | Planned / not implemented where unverifiable | Authoritative SHAs require authorized lookup |

## 2. Execution order

### 2.1 Now, local-only (in progress this session)

1. **Executed behavioral evaluation.** Keep `ae eval --suite ... --execute`
   structural by default. Add an explicit opt-in executed mode that drives the
   fake provider through authored scenarios and records deterministic
   behavioral outcomes, still reporting `modelBehaviorExecuted: false` because
   no real model ran. Never let fixture text change gates or policy.
2. **Signed local evidence bundles.** Add an opt-in signing step to evidence
   export using a local Ed25519 key from a file path or environment variable
   *name*. Keep signatures in a sidecar manifest, preserve existing JSON/Markdown
   exports, and fail closed on missing keys or tampered records.
3. **Release/provenance checklist and security audit.** Add a documented
   checklist and a local `ae audit` command that checks for credential shapes,
   generated-file drift, broken canonical references, and unpinned CI actions.
   This is a local report, not an external audit or a release approval.
4. **Read-only local dashboard.** Generate a static, self-contained snapshot
   from local state/evidence/trace and render it with an optional Vite + React +
   TypeScript app. No hosted service, no write API, no credentials, and no
   mandatory step in any workflow.

### 2.2 Plan only this session (needs authorization or external identity)

5. **Provider/integration adapters.** Define extension points and config shapes
   for a Gemini adapter, GitHub App identity, PR create/merge/push, federated
   approver identity, and externally verified review. Do not call live services
   or implement mutation paths until separately authorized. See §4.
6. **AWS adapters and Terraform reference architecture.** Keep as optional
   adapters behind ports; no cloud resource is created. See §5.
7. **Multi-user, hosted service, stable 1.0.** Requires identity, release
   signing, and compatibility commitments. See §6.

## 3. Design details for local-only work

### 3.1 Executed behavioral evaluation

- Inputs: authored scenario JSON under `evaluations/` with a fixed skill slug,
  a fixture issue, an expected structural outcome, and a bounded fake-provider
  script.
- Execution: reuse the existing workflow engine with `provider: fake` and a
  throwaway repo; never invoke a live provider or write outside the temp dir.
- Output: per-case `passed`, `reason`, `evidenceRecordIDs`, plus a top-level
  `modelBehaviorExecuted: false` and `provider: fake`. Scores are structural
  pass rates only, not model quality.
- Failure behavior: malformed fixtures, unknown slugs, or unexpected provider
  calls fail closed with a non-zero exit and a JSON error envelope.

### 3.2 Signed evidence bundles

- Key input: an environment variable whose value is a path to a local Ed25519
  private key, or an explicit `--key-file` path. The variable *name* may live in
  config; the key material never does.
- Bundle: existing `JSON`/`Markdown` exports plus a `signature.json` sidecar with
  key ID, algorithm, and a signature over a canonical digest of every exported
  record in sequence order.
- Verification: `ae evidence verify <run-id> --bundle DIR` recomputes the digest
  chain and checks the signature. Missing or mismatched signatures fail closed.
- Scope: local authenticity of an exported bundle only; not a trusted timestamp,
  transparency log, or externally verified attestation.

### 3.3 `ae audit`

- Read-only local checks over the canonical tree: `util.SecretShapeCount` on
  authored files, generated provider copies vs canonical manifests, canonical
  skill references, schema validity, and CI action pin status.
- Output: JSON/Markdown report with `status: passed|failed` and per-check details.
- Non-goals: does not approve a release, satisfy an AE gate, or replace a human
  security review.

### 3.4 Read-only local dashboard

- Snapshot command: `ae dashboard snapshot --run <id> --out DIR` writes
  `index.html`, `app.js`, `styles.css`, and a sanitized `snapshot.json`.
- Data: run metadata, status/history, evidence records, trace stages, and
  review-finding states. All text passes through `util.Sanitize` before export.
- Viewer: optional Vite + React + TypeScript app under `ui/` that reads the
  snapshot statically (no server, no API, no write path).
- Boundaries: never include credentials or raw untrusted provider payloads;
  mark provider findings as untrusted hypotheses; show `missing` trace stages
  honestly.

## 4. Provider/integration adapter plan (not implemented here)

| Capability | Extension point | Prerequisite | Status |
| --- | --- | --- | --- |
| Gemini adapter | `integration.Provider` in `internal/adapters/gemini`; route via local `provider.routes` | Credential env var, capability declaration, local stubs | Planned / not implemented |
| GitHub App identity | `integrations.github.mode: github-app`; separate actor model in approval evidence | App registration, installation token, actor mapping | Needs authorization |
| PR create/merge/push | New adapter methods behind explicit `--confirm-mutate` and policy allowlist | GitHub App identity + human authorization | Needs authorization; intentionally absent |
| Federated approver identity | Policy actor model + approval evidence issuer | External identity provider | Needs authorization/design |
| Externally verified review | Review gate accepts a signed, independently attested result | Attestation format + independent verifier | Needs authorization; provider text alone is insufficient |

Core packages remain provider-neutral; every item above belongs in
`internal/adapters/` plus local config. No model ID enters canonical skills.

## 5. Phase 6: optional AWS adapters

Planned ports only: API Gateway webhook, SQS queueing, Step Functions Standard,
ECS Fargate execution, DynamoDB state projection, S3 evidence artifacts, Secrets
Manager for credentials, CloudWatch/OTel telemetry, and Terraform reference
architecture. Each stays optional and behind an adapter port. No cloud resource
is created, and no deployment or publish action is performed by an agent.

## 6. Phase 7: dashboard, multi-user, stable 1.0

- Dashboard: the local static viewer in §3.4 is the implementable subset.
  Hosted/multi-user remains planned and optional.
- Stable 1.0: published module compatibility, signed/notarized bundles, verified
  action SHAs, and a release-provenance checklist. Provenance artifacts are not
  implemented; the checklist in this plan is documentation only.

## 7. Honesty and verification rules

- Update `README.md`, `CONTRIBUTING.md`, `progress.md`, `CHANGELOG.md`, and this
  document when a planned item becomes implemented.
- Record exact commands run and distinguish local stub tests from live calls.
- Keep status labels accurate: no live provider, Jira, GitHub, cloud, or deploy
  result may be asserted as exercised.
- Do not push, publish, deploy, create cloud resources, open PRs, or merge.
