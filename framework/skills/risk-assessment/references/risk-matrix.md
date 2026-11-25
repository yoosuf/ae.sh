# Deterministic risk matrix

This matrix is the canonical Phase 1 default. A stricter matching repository policy overrides it upward; it never lowers a tier.

## Path signals (add each match once per changed path)

| Signal ID | Path pattern (case-sensitive glob) | Weight |
| --- | --- | --- |
| P-AUTH | `**/auth/**`, `**/iam/**`, `**/tenant/**` | +3 |
| P-SECRET | `**/*secret*`, `**/*credential*`, `**/.env*`, `**/keystore/**` | +5, critical flag `secrets-material` |
| P-DATA | `migrations/**`, `db/schema/**`, `db/**` | +3 |
| P-INFRA | `infra/**`, `*.tf`, `.github/workflows/**` | +3 |
| P-API | `api/**`, `**/routes/**`, `**/handlers/**` | +2 |
| P-ORDINARY | `**/*.go`, `**/*.ts`, `**/*.tsx`, `**/*.cs`, `**/*.js` | +0 |

A path matching several patterns contributes the **highest** single weight, not the sum. Count each distinct changed path once.

## Semantic signals (add each observed signal once per diff)

| Signal ID | Evidence required in diff | Weight |
| --- | --- | --- |
| S-AUTHZ | New or changed permission check, role claim, or privileged operation | +3 |
| S-TENANT | Tenant key, query scope, cache key, or export scope changes | +3 |
| S-SECRET | Reading, writing, logging, or transporting credentials or tokens | +5, critical flag `secrets-material` |
| S-MIGRATION | Schema change, backfill, or data retention behavior change | +3 |
| S-DESTRUCTIVE | Irreversible data rewrite, truncation, or drop | +3, critical flag `destructive-migration` |
| S-CONCURRENCY | Locking, transaction boundary, async ordering, or retry behavior | +2 |
| S-EXTERNAL | Public API contract, event schema, or third-party integration contract | +2 |
| S-EXPORT | Bulk export or personal-data disclosure path | +2 |
| S-INFRA | Deployment, CI, or infrastructure configuration change | +3 |

## Score bands and overrides

| Score | Tier |
| --- | --- |
| 0-2 | low |
| 3-5 | medium |
| 6-8 | high |
| 9 or more | critical |

Override to `critical` regardless of score when any critical flag is present: `secrets-material`, `destructive-migration`, `authorization-bypass`, `privilege-escalation`, or `self-approval`.

Return `unknown` (not low) when SHAs, diff, or policy-critical semantics cannot be determined.

## Escalation contract

| Tier | Required escalation |
| --- | --- |
| low | Recorded tests plus independent review; reviewer is not the author. |
| medium | Cross-provider review when available, plus a designated reviewer. If cross-provider review is unavailable, record that fact and ask the designated reviewer before proceeding. |
| high | Relevant specialist reviews (security, data, platform) plus domain-owner approval. |
| critical | Separation of duties across author, implementer, and approver; explicit authorized approval; dry-run and recovery evidence before any irreversible action. |

## Worked example (synthetic)

A change modifies `src/auth/session.ts` and `src/api/invoices/export.ts`. Diff shows a tenant scope filter moving from session data to a request query parameter.

- Paths: P-AUTH +3 for `src/auth/session.ts`, P-API +2 for `src/api/invoices/export.ts` = 5.
- Semantics: S-AUTHZ +3 and S-TENANT +3 = 6. No critical flag from this evidence alone.
- Score = 11 → `critical` by band, with `S-TENANT` evidence requiring verification for an `authorization-bypass` flag.
- Escalation: separation of duties, explicit authorized approval, dry-run and recovery evidence before release.

The example is illustrative; always compute from actual changed paths and observed hunks.
