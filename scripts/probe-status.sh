#!/usr/bin/env bash
# probe-status.sh — list every PROBE in conformance.md with its declared
# status, Modes, Effect, and whether a probe test file exists on disk.
#
# Modes / Effect are the REQ-082 catalog fields (an absent Effect is
# treated as mutating). The test-file column is a filename heuristic
# and is not the runner's per-mode state — `go test ./testkit/probe/`
# is.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONF="${ROOT}/docs/specifications/conformance.md"
PROBES_DIR="${ROOT}/testkit/probes"

printf '%-11s | %-16s | %-28s | %-12s | %s\n' "PROBE" "Status" "Modes" "Effect" "Test file"
printf '%s-+-%s-+-%s-+-%s-+-%s\n' "-----------" "----------------" "----------------------------" "------------" "---------"

awk '
  /^#### PROBE-[0-9]+/ {
    if (id != "") print id "\t" status "\t" modes "\t" effect
    id=$2; status="(no status)"; modes="(none)"; effect="mutating"; next
  }
  id != "" && /^- \*\*Status:\*\*/ {
    s=$0
    sub(/^- \*\*Status:\*\*[[:space:]]*/, "", s)
    sub(/ —.*/, "", s)
    sub(/ - .*/, "", s)
    sub(/\.[[:space:]]*$/, "", s)
    status=s
  }
  id != "" && /^- \*\*Modes:\*\*/ {
    s=$0
    sub(/^- \*\*Modes:\*\*[[:space:]]*/, "", s)
    sub(/ —.*/, "", s)
    sub(/ \(.*/, "", s)
    sub(/;.*$/, "", s)
    sub(/\.[[:space:]]*$/, "", s)
    modes=s
  }
  id != "" && /^- \*\*Effect:\*\*/ {
    s=$0
    sub(/^- \*\*Effect:\*\*[[:space:]]*/, "", s)
    sub(/ —.*/, "", s)
    sub(/ \(.*/, "", s)
    sub(/\.[[:space:]]*$/, "", s)
    effect=s
  }
  END { if (id != "") print id "\t" status "\t" modes "\t" effect }
' "$CONF" | while IFS=$'\t' read -r id status modes effect; do
  num="${id#PROBE-}"
  f="$(ls "${PROBES_DIR}"/*/probe_"${num}"_*.go 2>/dev/null | head -1 || true)"
  if [[ -n "$f" ]]; then file="${f#"${ROOT}/"}"; else file="MISSING"; fi
  printf '%-11s | %-16s | %-28s | %-12s | %s\n' "$id" "$status" "$modes" "$effect" "$file"
done
