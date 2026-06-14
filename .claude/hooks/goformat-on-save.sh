#!/usr/bin/env bash
# Post-save Go formatter: format the single edited .go file with the project
# formatters — gofumpt (+ goimports), matching `make fmt` (golangci-lint fmt).
#
# Host-only by design — gofumpt/goimports are instantaneous; a Docker round-trip
# on every Write/Edit would dominate the latency budget for a save hook.
# Graceful no-op when a formatter isn't on the host: `make fmt`, which routes
# through the pinned golangci-lint image, is the authoritative full-tree pass.
set -euo pipefail

f="${CLAUDE_FILE_PATH:-}"
[[ -n "$f" ]] || exit 0
[[ "$f" == *.go ]] || exit 0

# Generated files belong to bmmgen — reformatting here would diverge from
# `make codegen-verify`. golangci-lint excludes them too (Code generated marker).
case "$f" in
  *_gen.go) exit 0 ;;
esac

# Prefer gofumpt (project standard); fall back to gofmt so files stay at least
# gofmt-clean until the next `make fmt` upgrades them to gofumpt.
if command -v gofumpt >/dev/null 2>&1; then
  gofumpt -w "$f"
elif command -v gofmt >/dev/null 2>&1; then
  gofmt -w -s "$f"
fi

# Import grouping/sorting to match the goimports formatter, when available.
if command -v goimports >/dev/null 2>&1; then
  goimports -w "$f"
fi
