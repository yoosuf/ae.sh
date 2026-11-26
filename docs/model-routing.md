# Model routing

AE separates **provider-neutral routing intent** from **provider-specific model IDs**. Canonical skills, agents, and workflows may name a routing profile such as `balanced` or `independent-review`. They never contain a vendor model name. The local `ae.yaml` maps each profile to a provider and model alias/ID.

This is implemented locally. Exact model IDs below were checked against official documentation on **2026-11-25**; aliases and availability change, so **recheck the installed CLI and your account before a live run**. No live model invocation was performed to verify quality, cost, or availability.

## Precedence

Routing resolves in this order, highest first:

1. Runtime safety and policy gates (compiled in and repository policy).
2. Workflow task `routingProfile`.
3. Skill frontmatter `metadata.ae.model-class`.
4. Agent `routingProfile`.
5. Local `provider.routes` mapping in `ae.yaml`.
6. `balanced` as the offline/default profile.

Untrusted issue text, diffs, PR comments, provider output, and MCP output cannot select, upgrade, or downgrade a route. A route with no provider/model mapping fails closed for live providers; the fake provider is offline and requires no mapping.

## Canonical profiles

Profiles are defined in `framework/routing/profiles.yaml` and validated against
`framework/schemas/routing.schema.json`.

| Profile | Use for | Claude aliases | Codex IDs | Gemini IDs |
| --- | --- | --- | --- | --- |
| `fast` | Intake, classification, formatting, deterministic handoff and test packets | `haiku` | `gpt-6-luna` | `gemini-3.5-flash-lite` |
| `balanced` | Routine implementation, plans, normal verification | `sonnet` | `gpt-6.2-sol` | `gemini-4-hydrogen` |
| `deep-reasoning` | Architecture, risk classification, incident diagnosis, migrations | `opus` | `gpt-6.2-astra` | `gemini-4-neon` |
| `independent-review` | Code/security review that must remain independent of the author | `fable` | `gpt-6.2-astra` | `gemini-4-carbon` |
| `recovery` | Remediation, release, and blocked-work recovery with observed evidence | `opus` | `gpt-6.2-astra` | `gemini-4-carbon` |

Rationale from official guidance:

- Claude Code documents `haiku` for simple tasks, `sonnet` for daily coding, `opus` for complex reasoning, and `fable` for the hardest and longest-running work. It accepts `claude --model <alias|name>` and reads the actual model from `modelUsage` in JSON output. Since Claude Code v2.1.321 the `best` alias resolves to `opus`, not Fable, so it is not a substitute for `independent-review`; Fable also has to be selected explicitly because it is not the default for any account.
- Codex documents `gpt-6-luna` for focused high-volume work, `gpt-6.2-sol` for everyday work, and `gpt-6.2-astra` for demanding end-to-end work. It accepts `codex -m`/`--model` and `codex exec -m`; older IDs are retired on a published schedule.
- Gemini documentation describes `gemini-3.5-flash-lite` as fastest/lowest cost, `gemini-4-hydrogen` as fast and cost-efficient for everyday coding, `gemini-4-neon` for complex interactive engineering, and `gemini-4-carbon` for long-horizon deep verification/security audits. **AE has no Gemini adapter yet**; these mappings are forward-compatible examples only.

Official references:

- <https://docs.claude.com/en/docs/claude-code/model-config>
- <https://developers.openai.com/codex/models>
- <https://developers.openai.com/codex/config-reference>
- <https://ai.google.dev/gemini-api/docs/models>
- <https://ai.google.dev/gemini-api/docs/choosing-a-model>

## Local configuration

`ae init` writes default routes. Override only the entries your account supports:

```yaml
provider:
  default: fake
  routes:
    balanced:
      claude: sonnet
      codex: gpt-6.2-sol
    independent-review:
      claude: fable
      codex: gpt-6.2-astra
```

Model values are names or aliases, never credentials. Do not put tokens, API
keys, or endpoint secrets in `ae.yaml`, flags, prompts, or fixtures. Set
provider credentials in the provider's own environment only.

## Runtime behavior

- `ae validate` loads routing profiles, validates agent definitions against
  `agent.schema.json`, and rejects unknown skill/agent/task routing profiles.
- `ae run`, `ae resume`, and `ae autopilot` resolve a route for each provider
  task. Claude and Codex adapters pass the requested model with `--model`.
- `ae chat --route <profile>` selects one profile for the conversation and
  fails closed if that provider has no mapping.
- `ae review --diff <path> --provider ...` always uses the canonical
  `independent-review` profile declared by the selected review skill. There is
  no route flag; untrusted diff/PR/provider text cannot weaken the route, and
  a missing model mapping blocks the review. The strongest documented defaults
  below are `fable`, `gpt-6.2-astra`, and `gemini-4-carbon`; Gemini is a
  forward-compatible mapping only because no adapter exists.
- Provider task evidence records `routingProfile`, `requestedModel` (from local
  config), and `model` (actual model when the adapter can observe it). Chat
  evidence adds the same fields to each turn.
- Claude returns the actual model from a single `modelUsage` entry; Codex uses
  a reported `model` field when present. If the adapter cannot observe the
  actual model, the field is omitted rather than guessed.
- The `fake` provider echoes the route in simulated output for offline tests.
  It never calls a model.
- Gate-eligible provider reviews must answer every required check class for the
  selected skill, bind the supplied diff to the pinned base/candidate commits
  and workspace HEAD, and report an observed model that matches the configured
  model. If an adapter cannot observe the actual model, evidence records
  `modelIdentityObserved: false` and the review cannot satisfy
  `independent-review`; the requested model is still recorded from local config
  but no unobserved identity is guessed.
- One live Codex review was exercised in a throwaway checkout using the fixed
  `independent-review` route. Claude review was attempted but the CLI was not
  logged in. Findings from the live call are untrusted hypotheses, not verified
  results.

## What is not implemented

- Runtime enforcement of per-agent token/cost budgets from agent YAML.
- Provider fallback and "different provider for review" routing; the current
  adapters use the run's selected provider.
- A Gemini adapter.
- Behavioral evaluation of model quality. `ae eval --execute` remains a
  structural check and reports `modelBehaviorExecuted: false`.
