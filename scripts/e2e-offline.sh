#!/usr/bin/env bash
# Local offline end-to-end check. It builds the CLI, copies the repository to a
# temporary directory, and runs a fake-provider workflow with no credentials,
# network calls, branches, commits, or deployments.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/aef-e2e.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT
BIN="$WORK_DIR/aef"
REPO_COPY="$WORK_DIR/repo"

# Keep the end-to-end run independent of the user's shared Go build cache.
# This is important for fresh machines, restricted sandboxes, and CI runners
# where the default cache may be absent or unwritable.
export GOCACHE="$WORK_DIR/go-cache"

(cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/aef)
mkdir -p "$REPO_COPY"
tar --exclude='./.git' --exclude='./.aef' --exclude='./aef.yaml' --exclude='./dist' \
  -cf - -C "$REPO_ROOT" . | tar -xf - -C "$REPO_COPY"

"$BIN" --root "$REPO_COPY" init --profile go >/dev/null

validate_output="$("$BIN" --root "$REPO_COPY" --json validate)"
validate_status="$(printf '%s' "$validate_output" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')"
if [[ "$validate_status" != "valid" ]]; then
  echo "validate did not report valid: $validate_output" >&2
  exit 1
fi

plan_output="$("$BIN" --root "$REPO_COPY" --json plan --issue examples/offline-workflow/issue-PROJ-101.json)"
plan_order="$(printf '%s' "$plan_output" | python3 -c 'import json,sys; print(",".join(json.load(sys.stdin)["taskOrder"]))')"
if [[ "$plan_order" != intake,* ]]; then
  echo "plan did not order tasks from intake: $plan_output" >&2
  exit 1
fi

eval_output="$("$BIN" --root "$REPO_COPY" --json eval --suite all --execute)"
eval_status="$(printf '%s' "$eval_output" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(str(d["executed"]).lower()+"/"+str(d["modelBehaviorExecuted"]).lower()+"/"+d["status"])')"
if [[ "$eval_status" != "true/false/passed" ]]; then
  echo "structural eval check failed: $eval_output" >&2
  exit 1
fi

dry_output="$("$BIN" --root "$REPO_COPY" --json run feature-development \
  --issue examples/offline-workflow/issue-PROJ-101.json --provider fake --dry-run)"
dry_flag="$(printf '%s' "$dry_output" | python3 -c 'import json,sys; print(str(json.load(sys.stdin)["dryRun"]).lower())')"
if [[ "$dry_flag" != "true" ]]; then
  echo "dry run was not marked dryRun=true: $dry_output" >&2
  exit 1
fi

start_output="$("$BIN" --root "$REPO_COPY" --json run feature-development \
  --issue examples/offline-workflow/issue-PROJ-101.json --provider fake --actor demo-user)"
run_id="$(printf '%s' "$start_output" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
start_state="$(printf '%s' "$start_output" | python3 -c 'import json,sys; print(json.load(sys.stdin)["state"])')"
if [[ "$start_state" != "awaiting_approval" ]]; then
  echo "expected architecture approval gate, got $start_state" >&2
  exit 1
fi

"$BIN" --root "$REPO_COPY" approve "$run_id" --gate architecture --approver reviewer-1 >/dev/null
resume_output="$("$BIN" --root "$REPO_COPY" --json resume "$run_id")"
resume_state="$(printf '%s' "$resume_output" | python3 -c 'import json,sys; print(json.load(sys.stdin)["state"])')"
if [[ "$resume_state" != "awaiting_merge" ]]; then
  echo "expected merge-eligible state after approval, got $resume_state" >&2
  exit 1
fi

"$BIN" --root "$REPO_COPY" approve "$run_id" --gate merge --approver reviewer-2 --role release-manager >/dev/null
set +e
blocked_output="$("$BIN" --root "$REPO_COPY" --json resume "$run_id")"
blocked_code=$?
set -e
if [[ "$blocked_code" -ne 4 ]]; then
  echo "expected policy exit code 4 for missing evidence, got $blocked_code" >&2
  exit 1
fi
blocked_error="$(printf '%s' "$blocked_output" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("error",""))')"
if [[ "$blocked_error" != *"missing required evidence"* ]]; then
  echo "missing evidence was not reported: $blocked_output" >&2
  exit 1
fi
status_output="$("$BIN" --root "$REPO_COPY" --json status "$run_id")"
blocked_status="$(printf '%s' "$status_output" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"]["status"])')"
blocked_reason="$(printf '%s' "$status_output" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"].get("terminalReason",""))')"
if [[ "$blocked_status" != "blocked" || "$blocked_reason" != *"missing required evidence"* ]]; then
  echo "missing evidence was not recorded honestly: $status_output" >&2
  exit 1
fi

trace_output="$("$BIN" --root "$REPO_COPY" --json trace "$run_id" --format json)"
stage_count="$(printf '%s' "$trace_output" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["stages"]))')"
feedback_status="$(printf '%s' "$trace_output" | python3 -c 'import json,sys; print([s["status"] for s in json.load(sys.stdin)["stages"] if s["id"]=="feedback"][0])')"
if [[ "$stage_count" -ne 7 || "$feedback_status" != "missing" ]]; then
  echo "traceability report did not report honest stage coverage: $trace_output" >&2
  exit 1
fi
markdown_trace="$("$BIN" --root "$REPO_COPY" --json trace "$run_id" --format markdown --out "$WORK_DIR/trace.md")"
if [[ ! -s "$WORK_DIR/trace.md" ]]; then
  echo "trace markdown export failed: $markdown_trace" >&2
  exit 1
fi

export_output="$("$BIN" --root "$REPO_COPY" --json evidence export "$run_id" --format json --out "$WORK_DIR/export")"
export_count="$(printf '%s' "$export_output" | python3 -c 'import json,sys; print(json.load(sys.stdin)["records"])')"
if [[ "$export_count" -le 0 || ! -s "$WORK_DIR/export/evidence.json" ]]; then
  echo "evidence export failed: $export_output" >&2
  exit 1
fi

verified_start="$("$BIN" --root "$REPO_COPY" --json run feature-development \
  --issue examples/offline-workflow/issue-PROJ-101.json --provider fake --actor demo-user)"
verified_id="$(printf '%s' "$verified_start" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
"$BIN" --root "$REPO_COPY" approve "$verified_id" --gate architecture --approver reviewer-3 >/dev/null
"$BIN" --root "$REPO_COPY" resume "$verified_id" >/dev/null
verify_output="$("$BIN" --root "$REPO_COPY" --json verify "$verified_id" --profile go)"
verify_passed="$(printf '%s' "$verify_output" | python3 -c 'import json,sys; print(str(json.load(sys.stdin)["allPassed"]).lower())')"
if [[ "$verify_passed" != "true" ]]; then
  echo "local verification did not pass: $verify_output" >&2
  exit 1
fi
cat > "$WORK_DIR/candidate.diff" <<'DIFF'
diff --git a/src/app.ts b/src/app.ts
--- a/src/app.ts
+++ b/src/app.ts
@@ -1,1 +1,1 @@
-const safe = 1;
+const html = dangerouslySetInnerHTML;
DIFF
review_output="$("$BIN" --root "$REPO_COPY" --json review --diff "$WORK_DIR/candidate.diff" --run "$verified_id")"
finding_id="$(printf '%s' "$review_output" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["findings"][0]["id"] if d.get("findings") else "")')"
if [[ -z "$finding_id" ]]; then
  echo "review scan did not produce a deterministic finding: $review_output" >&2
  exit 1
fi
resolve_output="$("$BIN" --root "$REPO_COPY" --json review resolve --run "$verified_id" \
  --finding "$finding_id" --status fixed --reason "unsafe rendering removed after local verification" \
  --actor reviewer-4)"
resolve_status="$(printf '%s' "$resolve_output" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')"
if [[ "$resolve_status" != "fixed" ]]; then
  echo "review finding resolution failed: $resolve_output" >&2
  exit 1
fi

"$BIN" --root "$REPO_COPY" approve "$verified_id" --gate merge --approver reviewer-4 --role release-manager >/dev/null
verified_resume="$("$BIN" --root "$REPO_COPY" --json resume "$verified_id")"
verified_state="$(printf '%s' "$verified_resume" | python3 -c 'import json,sys; print(json.load(sys.stdin)["state"])')"
if [[ "$verified_state" != "completed" ]]; then
  echo "verified run did not reach a local terminal state: $verified_resume" >&2
  exit 1
fi

chat_first="$("$BIN" --root "$REPO_COPY" --json chat --provider fake --message "first question" --run "$verified_id" --timeout 10s)"
chat_session="$(printf '%s' "$chat_first" | python3 -c 'import json,sys; print(json.load(sys.stdin)["sessionId"])')"
chat_second="$("$BIN" --root "$REPO_COPY" --json chat --provider fake --session "$chat_session" \
  --message "follow up question" --run "$verified_id" --timeout 10s)"
chat_check="$(printf '%s' "$chat_second" | python3 -c 'import json,sys; d=json.load(sys.stdin); t=d["turns"][0]; print(str(d["sessionId"]==t["sessionId"] and t["simulated"] and bool(t["evidenceRecordId"])).lower())')"
if [[ -z "$chat_session" || "$chat_check" != "true" ]]; then
  echo "fake conversation session or evidence was not preserved: $chat_first $chat_second" >&2
  exit 1
fi

echo "offline E2E passed: validate, plan, structural eval, dry run, approvals, fail-closed merge gate, local verification, review resolution, traceability, evidence export, fake chat continuity"
