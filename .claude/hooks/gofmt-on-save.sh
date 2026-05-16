#!/usr/bin/env bash
# Post-save gofmt: run host `gofmt -w -s` on the single edited .go file.
#
# Host-only by design — gofmt is cheap and instantaneous; a Docker round-trip
# on every Write/Edit would dominate the latency budget for a save hook.
# Contributors without a host Go install will still get formatting via
# `make fmt`, which routes through the Dockerfile dev stage.
set -euo pipefail

f="${CLAUDE_FILE_PATH:-}"
[[ -n "$f" ]] || exit 0
[[ "$f" == *.go ]] || exit 0

# Silent no-op if host gofmt isn't installed — `make fmt` will catch the
# tree on next invocation.
command -v gofmt >/dev/null 2>&1 || exit 0

gofmt -w -s "$f"
