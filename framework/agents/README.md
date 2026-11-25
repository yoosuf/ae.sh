# AE agent definitions

The YAML files in this directory are **AE-internal role definitions**. They are
not native Claude (`CLAUDE.md` / `.claude/agents/*.md`) or Codex
(`~/.codex/agents/*.toml`) agent formats, and must not be copied into a provider
agent directory as-is.

At runtime, AE's provider materialization step converts a definition here into
the target provider's native form:

- `role` becomes the provider system prompt role description.
- `skills` are resolved against the Phase 1 skill catalogue and mounted as
  provider skills.
- `toolAllowlist` and `scopes` become provider permission constraints.
- `budgets`, `timeouts`, and `retries` become run-level limits.
- `escalation` and `independence` are enforced by the orchestrator, not by the
  provider.

Do not hand-edit materialized provider files; edit these definitions and
re-materialize.
