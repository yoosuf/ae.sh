# Profiles

Profiles are checked-in, executable configuration under
`framework/profiles/<id>/profile.yaml`. `ae verify` runs their `checks` locally and
records the command, sanitized output, and real exit status as evidence. Missing
tools, missing wrappers, or failed commands are failures, never skipped passes.
See [evidence](evidence.md) for the record format.

## Available profiles

| Profile | Intended use | Primary local checks |
| --- | --- | --- |
| `go` | Go modules | `gofmt`, `go vet`, `go build`, `go test`, `go mod verify` |
| `java` | Java with a committed Maven or Gradle wrapper | Spotless, Maven/Gradle verification, package/assemble, tests, dependency tree |
| `python` | Python 3 projects | Black, Ruff, `python3 -m build`, pytest, `pip check` |
| `dotnet` | .NET SDK projects | `dotnet format`, build, test, deprecated package list |
| `nodejs` | JavaScript/Node.js projects | format, lint, build, test, `npm ls --all` |
| `typescript` | Strict TypeScript projects | format, lint, `typecheck && build`, test, `npm audit` |
| `react` | React applications | format, lint, build, component tests |
| `vue` | Vue 3 applications | format, lint, build, component tests |
| `astro` | Astro sites | format, lint, build, tests |
| `laravel` | PHP/Laravel applications | Pint, `composer validate`, asset build, `php artisan test`, `composer audit` |
| `aws` | Terraform infrastructure | `terraform fmt`, `validate`, `plan`, `test`, provider list |
| `all` | Composite of every canonical profile above | Union of all checks, de-duplicated by gate and command |

Select one or more profiles when initializing a project:

```sh
ae init --profile java
ae init --profile python
ae init --profile laravel
ae init --profile typescript
ae init --profile go,react
```

`ae init` rejects unknown profile names before writing `ae.yaml`. `ae doctor`
also reports unknown profiles in an existing config. `ae validate` loads every
canonical profile, including `all`, and fails on malformed definitions, missing
includes, or include cycles.

## Composite `all` profile

`all` is an aggregate profile, not auto-detection. It reuses every canonical
profile's checks and records the resulting de-duplicated list. Use it only in
repositories that actually contain all of those toolchains, or in CI images
where every command is available. In a single-stack repository, select that
stack explicitly: irrelevant checks would fail closed because their tools,
wrappers, or scripts are absent.

Profiles may declare `includes`. Includes are loaded recursively, cycles are
rejected, and duplicate checks with the same gate and command are collapsed.
Only checked-in profile files can supply verification commands; issue text,
model output, and CLI flags cannot.

## Toolchain expectations

- **Java:** a committed executable `./mvnw` or `./gradlew`. Spotless must
  provide the configured format check. If both wrappers are absent, verification
  exits `127`.
- **Python:** `python3` with `black`, `ruff`, `build`, and `pytest` modules
  available. `pip check` validates installed dependency consistency.
- **Laravel:** Composer dependencies installed so `vendor/bin/pint` exists, plus
  `npm` scripts for frontend assets. API-only Laravel projects should use a
  dedicated profile instead of pretending a frontend build exists.
- **TypeScript:** npm scripts named `format:check`, `lint`, `typecheck`,
  `build`, and `test`; `strict` should be enabled in `tsconfig.json`.
- **`all`:** all of the above plus .NET, Node.js, React, Vue, Astro, AWS, and Go
  toolchains.
