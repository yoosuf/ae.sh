#!/usr/bin/env bash
set -euo pipefail

binary_name="ae"
default_prefix="/usr/local/bin"
prefix="$default_prefix"
manifest=""
assume_yes=0

usage() {
  cat <<'EOF'
Usage: scripts/uninstall.sh --manifest FILE [--prefix DIR] [--yes]

Removes only paths explicitly listed in FILE (the manifest written by
scripts/install.sh). Nothing is deleted by guessing paths, and every removal
is confirmed unless --yes is passed.

Manifest format (one entry per line, '#' starts a comment):
  binary=/absolute/path/to/ae
  skill=/absolute/path/to/provider/skills/<slug>

Safety checks:
  - The binary entry must equal <prefix>/ae.
  - A skill entry must be under ~/.claude/skills/, ~/.agents/skills/,
    $PWD/.claude/skills/, or $PWD/.agents/skills/ and its basename must be a
    canonical AE skill slug (optionally prefixed with 'ae-').
  - .codex/skills is not a documented provider path and is rejected.
  - Entries outside these rules are rejected and nothing is removed.

Add a skill entry only if you verified that AE materialization installed it.
This script never deletes source files, the canonical `framework/skills/` directory, or
any path not present in the manifest. To uninstall manually: remove the binary
listed in the manifest and each verified provider-installed skill directory.
EOF
}

fail() {
  printf 'uninstall.sh: %s\n' "$1" >&2
  exit 2
}

confirm() {
  local question="$1"
  local answer=""
  if [[ ! -t 0 ]]; then
    printf 'uninstall.sh: non-interactive shell; pass --yes to confirm: %s\n' "$question" >&2
    return 2
  fi
  printf '%s [y/N] ' "$question"
  read -r answer
  case "$answer" in
    y | Y | yes | YES) return 0 ;;
    *) return 1 ;;
  esac
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --manifest)
      [[ $# -ge 2 ]] || fail "--manifest requires a file"
      manifest="$2"
      shift 2
      ;;
    --manifest=*)
      manifest="${1#*=}"
      shift
      ;;
    --prefix)
      [[ $# -ge 2 ]] || fail "--prefix requires a directory"
      prefix="$2"
      shift 2
      ;;
    --prefix=*)
      prefix="${1#*=}"
      shift
      ;;
    --yes | -y)
      assume_yes=1
      shift
      ;;
    --help | -h)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[[ -n "$manifest" ]] || fail "--manifest is required (see --help)"
[[ -f "$manifest" ]] || fail "manifest not found: $manifest"
[[ -n "$prefix" ]] || fail "--prefix must not be empty"

expected_binary="$prefix/$binary_name"
skills_claude="$HOME/.claude/skills/"
skills_codex="$HOME/.agents/skills/"
skills_claude_project="$PWD/.claude/skills/"
skills_codex_project="$PWD/.agents/skills/"
ae_slug_pattern='^(ae-)?(acceptance-criteria|agent-handoff|architecture-review|backend-development|code-review|git-worktree-management|jira-intake|pull-request-management|requirement-analysis|risk-assessment|security-review|solution-architecture|technical-planning|test-review|unit-testing)$'

kinds=()
targets=()

while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line%%#*}"
  line="${line#"${line%%[![:space:]]*}"}"
  line="${line%"${line##*[![:space:]]}"}"
  [[ -n "$line" ]] || continue

  key="${line%%=*}"
  value="${line#*=}"
  [[ "$key" != "$line" ]] || fail "malformed manifest line: $line"
  [[ "$value" == /* ]] || fail "path is not absolute: $value"

  case "$key" in
    binary)
      [[ "$value" == "$expected_binary" ]] || fail "binary path $value is not $expected_binary"
      kinds+=("binary")
      targets+=("$value")
      ;;
    skill)
      base="${value##*/}"
      parent="$(dirname -- "$value")"
      [[ "$base" =~ $ae_slug_pattern ]] || fail "skill basename is not a canonical AE skill: $value"
      case "$value" in
        "$skills_claude"* | "$skills_codex"* | "$skills_claude_project"* | "$skills_codex_project"*)
          case "$parent/" in
            "$skills_claude" | "$skills_codex" | "$skills_claude_project" | "$skills_codex_project") ;;
            *) fail "skill path is nested below a provider skill root: $value" ;;
          esac
          ;;
        *) fail "skill path is outside provider skill roots: $value" ;;
      esac
      kinds+=("skill directory")
      targets+=("$value")
      ;;
    *) fail "unknown manifest key: $key" ;;
  esac
done < "$manifest"

[[ ${#targets[@]} -gt 0 ]] || fail "manifest lists no removable paths"

printf 'Planned removals from %s:\n' "$manifest"
for i in "${!targets[@]}"; do
  printf '  %s: %s\n' "${kinds[$i]}" "${targets[$i]}"
done

for i in "${!targets[@]}"; do
  path="${targets[$i]}"
  [[ -e "$path" || -L "$path" ]] || {
    printf 'Skipping missing path: %s\n' "$path"
    continue
  }
  if [[ "$assume_yes" -eq 0 ]]; then
    confirm "Remove ${kinds[$i]} $path?" || {
      printf 'Aborted; no further paths were removed.\n' >&2
      exit 1
    }
  fi
  if [[ "${kinds[$i]}" == "binary" ]]; then
    rm -f -- "$path"
  else
    rm -rf -- "$path"
  fi
  printf 'Removed: %s\n' "$path"
done

printf 'Uninstall complete. Remove %s yourself if you no longer need it.\n' "$manifest"
