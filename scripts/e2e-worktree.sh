#!/usr/bin/env bash
# Local worktree end-to-end check. It builds the CLI, creates a throwaway git
# repository with one synthetic commit, and verifies detached worktree
# preparation, managed-workspace runs, local verification, and clean removal.
# No credentials, network calls, branches, pushes, or deployments are used.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/ae-worktree-e2e.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT
BIN="$WORK_DIR/ae"
REPO_COPY="$WORK_DIR/repo"

# Keep verification in the throwaway repository independent of the host's
# shared Go build cache, which may be absent or unwritable in CI/sandboxes.
export GOCACHE="$WORK_DIR/go-cache"

(cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/ae)
mkdir -p "$REPO_COPY"
tar --exclude='./.git' --exclude='./.ae' --exclude='./ae.yaml' --exclude='./dist' \
  -cf - -C "$REPO_ROOT" . | tar -xf - -C "$REPO_COPY"

# This commit is only in the throwaway copy. It exists so git has a HEAD from
# which to create a detached worktree; the source tree is never committed.
git -C "$REPO_COPY" init -q -b main
git -C "$REPO_COPY" add -A
git -C "$REPO_COPY" -c user.name=AE -c user.email=ae@example.invalid \
  commit -qm 'synthetic fixture commit' --no-gpg-sign

"$BIN" --root "$REPO_COPY" init --profile go >/dev/null

start_output="$("$BIN" --root "$REPO_COPY" --json run feature-development \
  --issue examples/offline-workflow/issue-PROJ-101.json --provider fake --actor worktree-tester)"
run_id="$(printf '%s' "$start_output" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
"$BIN" --root "$REPO_COPY" --json worktree prepare "$run_id" >/dev/null

worktree="$REPO_COPY/.ae/worktrees/$run_id"
worktree_real="$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$worktree")"
head_main="$(git -C "$REPO_COPY" rev-parse HEAD)"
head_worktree="$(git -C "$worktree" rev-parse HEAD)"
if [[ "$head_main" != "$head_worktree" ]]; then
  echo "worktree HEAD $head_worktree does not match $head_main" >&2
  exit 1
fi
if git -C "$worktree" symbolic-ref -q HEAD >/dev/null 2>&1; then
  echo 'worktree was attached to a branch' >&2
  exit 1
fi
branches="$(git -C "$REPO_COPY" branch --format='%(refname:short)')"
if [[ "$branches" != "main" ]]; then
  echo "unexpected branch was created: $branches" >&2
  exit 1
fi

mkdir -p "$REPO_COPY/user-owned"
set +e
rejected_output="$("$BIN" --root "$REPO_COPY" --json run feature-development \
  --issue examples/offline-workflow/issue-PROJ-101.json --provider fake \
  --workspace "$REPO_COPY/user-owned" 2>&1)"
rejected_code=$?
set -e
if [[ "$rejected_code" -eq 0 ]]; then
  echo "unmanaged workspace was accepted: $rejected_output" >&2
  exit 1
fi
printf '%s' "$rejected_output" | python3 -c 'import json,sys; assert "error" in json.load(sys.stdin)'

workspace_run="$("$BIN" --root "$REPO_COPY" --json run feature-development \
  --issue examples/offline-workflow/issue-PROJ-101.json --provider fake \
  --actor worktree-tester --workspace "$worktree")"
workspace_run_id="$(printf '%s' "$workspace_run" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
saved_workspace="$(printf '%s' "$workspace_run" | python3 -c 'import json,sys; print(json.load(sys.stdin)["workspace"])')"
if [[ "$saved_workspace" != "$worktree_real" ]]; then
  echo "workspace was not persisted: $saved_workspace (want $worktree_real)" >&2
  exit 1
fi

"$BIN" --root "$REPO_COPY" approve "$workspace_run_id" --gate architecture --approver reviewer-a >/dev/null
"$BIN" --root "$REPO_COPY" resume "$workspace_run_id" >/dev/null
"$BIN" --root "$REPO_COPY" verify "$workspace_run_id" >/dev/null
"$BIN" --root "$REPO_COPY" approve "$workspace_run_id" --gate merge \
  --approver reviewer-b --role release-manager >/dev/null
final_output="$("$BIN" --root "$REPO_COPY" --json resume "$workspace_run_id")"
final_state="$(printf '%s' "$final_output" | python3 -c 'import json,sys; print(json.load(sys.stdin)["state"])')"
if [[ "$final_state" != "completed" ]]; then
  echo "workspace run did not reach completed: $final_output" >&2
  exit 1
fi

"$BIN" --root "$REPO_COPY" --json worktree clean "$run_id" >/dev/null
if [[ -e "$worktree" ]]; then
  echo "worktree was not removed: $worktree" >&2
  exit 1
fi
git -C "$REPO_COPY" worktree prune
if git -C "$REPO_COPY" worktree list --porcelain | grep -F "$worktree_real" >/dev/null; then
  echo "worktree registration remains: $worktree_real" >&2
  exit 1
fi

echo "worktree E2E passed: detached prepare, managed workspace run, local verification, clean removal"
