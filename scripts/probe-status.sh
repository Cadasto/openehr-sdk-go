#!/usr/bin/env bash
# probe-status.sh — list every PROBE in conformance.md with its declared status
# and whether a probe test file exists on disk.
#
# For wire/client work the PROBE state is the definition of done: a change isn't
# finished while its probe is still Draft with no test file (unless the plan
# explicitly defers it).
#
# The conformance.md Status column is authoritative. The test-file column is a
# heuristic keyed on the `probe_NNN_*.go` filename, so a probe co-located in
# another probe's file (e.g. PROBE-026 living in probe_025_*.go) reads MISSING.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONF="${ROOT}/docs/specifications/conformance.md"
PROBES_DIR="${ROOT}/testkit/probes"

printf '%-11s | %-24s | %s\n' "PROBE" "Status (conformance.md)" "Test file"
printf '%s-+-%s-+-%s\n' "-----------" "------------------------" "---------"

awk '
  /^#### PROBE-[0-9]+/ {
    if (id != "") print id "\t" status
    id=$2; status="(no status line)"; next
  }
  id != "" && /^- \*\*Status:\*\*/ {
    s=$0
    sub(/^- \*\*Status:\*\*[[:space:]]*/, "", s)   # strip "- **Status:** "
    sub(/ —.*/, "", s)                             # drop em-dash tail ("— see ...")
    sub(/ - .*/, "", s)                            # drop ascii "- ..." tail
    sub(/\.[[:space:]]*$/, "", s)                  # drop trailing period
    status=s
  }
  END { if (id != "") print id "\t" status }
' "$CONF" | while IFS=$'\t' read -r id status; do
  num="${id#PROBE-}"
  f="$(ls "${PROBES_DIR}"/*/probe_"${num}"_*.go 2>/dev/null | head -1 || true)"
  if [[ -n "$f" ]]; then file="${f#"${ROOT}/"}"; else file="MISSING"; fi
  printf '%-11s | %-24s | %s\n' "$id" "$status" "$file"
done
