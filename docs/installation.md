# Installation

AEF is distributed as a single Go CLI (`aef`). There is no daemon, database server, dashboard, or cloud account requirement. The install instructions below are for **macOS and Linux**; **Windows is not tested**.

The current pre-1.0 CLI is checkout-scoped: its canonical `framework/` assets
must be available under the AEF checkout used as `--root`. It does not yet
install a standalone binary plus a globally discoverable asset bundle for
arbitrary project directories.

## Prerequisites

- Go 1.27 (to build from source).
- A POSIX shell and git.
- Optional: Claude Code or Codex installed if you want materialized skills. The offline demo needs neither.

## Install from source (macOS / Linux)

```sh
git clone git@github.com:yoosuf/ae.sh.git ae.sh
cd ae.sh
go build -o aef ./cmd/aef
./aef doctor
```

Run project configuration commands from this checkout, or pass its path
explicitly with `--root /path/to/ae.sh`. `aef init` fails closed when the
canonical assets are not present instead of creating an unusable configuration.

To place it on your `PATH`, use the provided installer (no `sudo` escalation; it asks for confirmation before writing to `/usr/local/bin`):

```sh
scripts/install.sh --prefix "$HOME/.local/bin" --yes   # review target first
```

`scripts/install.sh --help` lists options and writes an install manifest for `scripts/uninstall.sh`. No provider skills are installed by this script.

## Install with `go install`

Once the module path is published (placeholder in the README):

```sh
go install github.com/aef-framework/aef/cmd/aef@latest
```

Verify with `aef doctor`. Confirm the module path from `go.mod` before relying on this command.

## Verify an installation

```sh
aef doctor
aef validate
aef skills list
```

These commands are local-only. They should report the local Go/toolchain state and the canonical catalogue; they should not print credentials or contact a provider. If a check fails, fix the reported problem rather than bypassing it. From a source checkout, `make check` also runs build, vet, tests, race tests, validation, the offline end-to-end script, and the worktree end-to-end script in a throwaway git repository.

## Upgrade

For a source checkout:

```sh
cd agentic-framework
git pull --ff-only
git describe --tags --always        # confirm the version you are building
go build -o aef ./cmd/aef
./aef doctor
```

Review the [CHANGELOG](../CHANGELOG.md) and ADRs for breaking changes first; pre-1.0 releases may change behavior. Reinstall provider skills after upgrading so manifests and checksums match the new catalogue version:

```sh
aef skills install --provider all --scope user --version <v> --dry-run
```

## Uninstall (macOS / Linux)

1. Remove generated provider skill copies you no longer want. Check the exact paths in [provider compatibility](provider-compatibility.md) before deleting anything; only remove directories/files written by AEF.
2. Remove the binary:

```sh
rm /usr/local/bin/aef        # or wherever you installed it
```

3. Remove local AEF configuration/state directories you created under your home or project directory, if you no longer need runs and evidence. Back up evidence first if needed.
4. Remove any `PATH` entries you added.

Uninstall does not delete Jira/GitHub data or cloud resources. No cloud resources are created by the implemented local/offline path.

## Offline use

After installation, `aef plan --issue PROJ-101` and `aef run feature-development --issue PROJ-101 --dry-run` use bundled fixtures and the fake provider. A real local run with `--provider fake` creates SQLite state and stops at approval gates; it still needs no credentials or network access. See [getting started](getting-started.md).

## Known limitations

- No signed binaries or checksum-based release verification yet.
- No Windows support or testing.
- No automatic updater.
- Installation from source is the only supported method in Phase 0–2 until the module path and releases are published.
