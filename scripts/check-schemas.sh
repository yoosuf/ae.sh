#!/usr/bin/env bash
# Validate the canonical catalogue without writing config into the checkout.
# The CLI requires local config, so validation runs against a temporary copy.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/aef-validate.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT
BIN="$WORK_DIR/aef"
REPO_COPY="$WORK_DIR/repo"

(cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/aef)
mkdir -p "$REPO_COPY"
tar --exclude='./.git' --exclude='./.aef' --exclude='./aef.yaml' --exclude='./dist' \
  -cf - -C "$REPO_ROOT" . | tar -xf - -C "$REPO_COPY"

"$BIN" --root "$REPO_COPY" init --profile go >/dev/null
"$BIN" --root "$REPO_COPY" validate
