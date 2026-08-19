#!/usr/bin/env bash
# Post-save Go formatter: format the single edited .go file with the project
# formatters — gofumpt (+ goimports), matching `make fmt` (golangci-lint fmt).
#
# Host-only by design — gofumpt/goimports are instantaneous; a Docker round-trip
# on every Write/Edit would dominate the latency budget for a save hook.
# Graceful no-op when a formatter isn't on the host: `make fmt`, which routes
# through the pinned golangci-lint image, is the authoritative full-tree pass.
#
# Claude Code delivers the edited path as JSON on stdin (`.tool_input.file_path`).
# It does NOT set CLAUDE_FILE_PATH / CLAUDE_FILE_PATHS — reading those yields an
# empty value and turns this hook into a permanent no-op, which is how it sat
# silently broken. The payload-parse failure below is therefore loud, not quiet:
# a missing formatter is an acceptable degradation, a missing path is a defect.
set -euo pipefail

payload="$(cat)"

if command -v jq >/dev/null 2>&1; then
  f="$(printf '%s' "$payload" | jq -r '.tool_input.file_path // empty' 2>/dev/null)" || {
    echo "goformat-on-save: hook payload is not valid JSON; run 'make fmt'" >&2
    exit 0
  }
elif command -v python3 >/dev/null 2>&1; then
  f="$(printf '%s' "$payload" | python3 -c \
    'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("file_path") or "")' 2>/dev/null)" || {
    echo "goformat-on-save: hook payload is not valid JSON; run 'make fmt'" >&2
    exit 0
  }
else
  echo "goformat-on-save: neither jq nor python3 on PATH; cannot read the hook payload — run 'make fmt'" >&2
  exit 0
fi

[[ -n "$f" ]] || exit 0
[[ "$f" == *.go ]] || exit 0

# A relative file_path resolves against the project root.
[[ "$f" == /* ]] || f="${CLAUDE_PROJECT_DIR:-$PWD}/$f"
[[ -f "$f" ]] || exit 0

# Generated files belong to bmmgen — reformatting here would diverge from
# `make codegen-verify`. golangci-lint excludes them too (Code generated marker).
case "$f" in
  *_gen.go) exit 0 ;;
esac

# A formatter that rejects the file is not a hook failure: Claude Code writes a
# file in stages, so a half-written *.go legitimately does not parse yet. Left to
# `set -e` the non-zero gofumpt propagates out of the hook — and a non-zero
# PostToolUse hook is a blocking error fed back to the model, on an ordinary
# save. `make fmt` remains the authoritative pass; here we say so and step aside.
unparsed() {
  echo "goformat-on-save: ${f##*/} left unformatted (does not parse yet); run 'make fmt' once it compiles" >&2
  exit 0
}

# Prefer gofumpt (project standard); fall back to gofmt so files stay at least
# gofmt-clean until the next `make fmt` upgrades them to gofumpt.
if command -v gofumpt >/dev/null 2>&1; then
  gofumpt -w "$f" || unparsed
elif command -v gofmt >/dev/null 2>&1; then
  gofmt -w -s "$f" || unparsed
fi

# Import grouping/sorting to match the goimports formatter, when available.
if command -v goimports >/dev/null 2>&1; then
  goimports -w "$f" || unparsed
fi
