# ADR-0008: No credentials in flags; auth outside prompts

## Status

Accepted (Phase 0 architecture decision). Applies to all phases; secret-store integrations are planned.

## Context

CLI flags, shell history, config files, prompts, and fixtures are commonly logged or committed. Passing API tokens in any of them risks disclosure and contaminates evidence. Approval identity must be attributable to a human or service, not to text in a prompt.

## Decision

Never accept credentials as CLI flags or store them in YAML config, prompts, or checked-in fixtures. Read credentials from environment variables or an external secret store (AWS Secrets Manager in the optional adapter). Authentication for Jira/GitHub happens in adapters; local authorization/approval identity is recorded by the workflow engine with `identitySource: local-manual`, a role, candidate SHA, and artifact digest. Federated identity from an authenticated organizational context remains planned (Phase 4+). Issue text can never assert an identity or grant approval.

## Consequences

- Users must configure secrets outside AEF; slightly more setup.
- Shell history and evidence stay clean.
- Local approvals are attributable but not federated; critical changes still require an organizational approver outside the local CLI.
- CI must inject secrets through the runner's secret mechanism.

## Alternatives considered

- **Token flags for convenience:** rejected — leaks into history/process lists/evidence.
- **Tokens in repo config:** rejected — commit risk and tenant crossover.
- **Deriving identity from issue comments:** rejected — spoofable.
