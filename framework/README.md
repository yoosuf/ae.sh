# Canonical framework assets

This directory groups the checked-in assets that define AE behavior. They are
source files, not generated provider copies.

```text
framework/
├── agents/     provider-neutral agent definitions
├── policies/   risk-scoped policy bundles
├── profiles/   executable local verification profiles
├── routing/    provider-neutral model routing profiles
├── schemas/    JSON Schemas mirrored into internal/schema/assets
├── skills/     canonical SKILL.md instruction contracts
├── templates/  issue, plan, evidence, ADR, and review templates
└── workflows/  YAML DAG workflow definitions
```

All Go code resolves these locations through `internal/framework/paths.go`, so
the catalogue loader, workflow loader, policy loader, config loader, schema
tests, and CLI docs share one path contract. Provider materialization still
writes to the documented Claude and Codex skill directories; those generated
paths are intentionally unchanged.
