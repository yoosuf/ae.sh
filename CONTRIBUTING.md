# Contributing to AE

Thanks for your interest in the Agentic Engineering. This project is
early-stage: the catalogue, CLI, and workflow engine are being built in phases.
Please read this file, `AGENTS.md`, and `docs/roadmap.md` before opening a pull
request.

## Scope and status

The local/offline path for Phases 0–2 is implemented, together with a local
subset of Phases 3–5: `status`, `resume`, `cancel`, `approve`, fail-closed
gates, `verify` with real local check results, detached worktree preparation,
`run --workspace`, `trace`, evidence export, structured review findings and
resolution evidence, fake-provider chat continuity, and structural evaluation
checks. Opt-in Jira Cloud intake and `jira report-bug`, GitHub CLI PR
reads/comments, provider chat, constrained provider write modes, and the
`autopilot` loop are implemented locally but were not exercised against live
services. GitHub App/service identity, PR creation/merge/push, independently
verified review, federated
identity, cloud adapters, the dashboard, behavioral evaluation, and remaining
Phase 3–7 work are **planned / not implemented**. Do not open PRs that claim
live integrations were exercised.

## Getting set up

1. Read `docs/architecture.md` and `docs/provider-compatibility.md`.
2. Install Go 1.27 when working on the CLI (see `docs/installation.md`).
3. Make a branch for your change; keep it focused.
4. For changes to skills, edit the canonical Markdown at
   `framework/skills/<slug>/SKILL.md` and regenerate provider copies with
   `ae skills install` rather than hand-editing generated files.
5. Run `make check` (format, build, vet, unit tests, race tests, validation,
   the offline end-to-end script, and the worktree end-to-end script) before
   proposing a change. If a check cannot run, say so explicitly in the PR.

## Ground rules

- **No fabricated results.** Never report tests as passing, integrations as
  exercised, or resources as deployed unless you actually ran them locally and
  can describe the command and outcome. Include commands you ran in the PR.
- **Local-first.** Do not add a mandatory network service, API key, dashboard,
  or cloud dependency. Provider SDKs belong only in optional adapters.
- **Skills are instruction contracts, not roles.** Keep `SKILL.md` files as
  precise, testable instructions with inputs, outputs, and failure behavior.
- **Fail closed.** Policy and validation errors must stop the workflow, not
  warn-and-continue.
- **No credentials in flags, prompts, fixtures, or commits.** Use environment
  variables or an external secret store; see `docs/security/threat-model.md`.
- **Do not push, publish, or deploy** without explicit maintainer authorization.

## Pull requests

- Link an issue and describe the behavior change and user-visible impact.
- Update docs when behavior changes; update `CHANGELOG.md` under `Unreleased`.
- Add or update tests for code changes when the test suite exists. If you could
  not run tests, say so explicitly.
- Keep generated files (materialized skills, manifests, checksums) consistent
  with their canonical source.
- Note any new dependency and its license. Changes to dependencies should
  reference an ADR when they affect architecture.

## Reporting problems

- Bugs: use the bug report issue template and include exact commands, versions,
  and sanitized output. Never paste real tokens, issue text, or customer data.
- Security: follow `SECURITY.md` instead of opening a public issue.
- Conduct: see `CODE_OF_CONDUCT.md`.

## Reviews

Reviewers look for correctness, scope, local-first behavior, honest status
labeling, and evidence of the checks the author actually ran. Authors are
expected to respond to review findings before merge; automated review findings
are treated as findings to verify, not instructions to blindly apply.

By contributing you agree that your contribution is licensed under the terms in
`LICENSE` (proposed Apache-2.0) and that you have the right to submit it.
