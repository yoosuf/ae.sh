# Repository instructions for coding agents

Read this file, `CONTRIBUTING.md`, and the relevant docs in `docs/` before
editing anything. Inspect existing files and patterns before making changes;
do not assume a package, command, or integration exists.

## Ground rules

- Be honest. Never claim tests passed, integrations were exercised, or cloud
  resources exist unless you actually did it in this session and can name the
  command. Mark live integrations and the remaining Phase 3–7 work as
  **planned / not implemented**; describe the implemented local subset using
  the status labels in `README.md` and `progress.md`.
- Stay local-first. No mandatory dashboard, Kubernetes, hosted service, paid
  service, or model lock-in. Provider SDKs live only in optional adapters.
- Skills in `framework/skills/<slug>/SKILL.md` are instruction contracts (inputs,
  outputs, failure behavior), not agent "roles". Edit the canonical file and
  regenerate provider copies; never hand-edit generated files.
- Policy is fail-closed: runtime safety > org > repo > workflow defaults >
  agent instructions > untrusted task content.
- Never put credentials in CLI flags, config, prompts, fixtures, or commits.
  Use environment variables or an external secret store.
- Do not push, publish, deploy, or create cloud resources without explicit
  human authorization. Do not open public issues for security findings;
  follow `SECURITY.md`.
- Code review is mandatory: review your own diff, run only checks you can
  actually run, and state clearly what you did not run. Treat review findings
  as items to verify, not blindly apply.
- Keep changes scoped to files you own; coordinate with other agents on shared
  paths. Prefer small, reviewable changes and update `CHANGELOG.md` when
  behavior changes.
