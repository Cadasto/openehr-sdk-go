#!/usr/bin/env bash
# Regenerate the SDD index blocks from their single sources:
#
#   docs/specifications/REQ.md  registry table  <- docs/specifications/traceability.yaml
#   docs/plans/README.md        plan index      <- the **Status:** / **Covers:** headers
#                                                  of docs/plans/*.md
#
# Only the text between the BEGIN/END GENERATED markers is rewritten; the
# prose around it is hand-written. Never edit a generated block by hand —
# edit the source and run `make spec-gen`.
#
# Usage: spec-gen.sh           rewrite the blocks in place
#        spec-gen.sh --check   exit 1 if any block is stale (spec-check runs this)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
YAML="${ROOT}/docs/specifications/traceability.yaml"
REQ_REG="${ROOT}/docs/specifications/REQ.md"
PLANS_DIR="${ROOT}/docs/plans"
PLANS_README="${PLANS_DIR}/README.md"

check=0
[[ "${1:-}" == "--check" ]] && check=1

# The registry: one row per traceability.yaml entry, in id order.
gen_registry() {
  echo '| ID | Title | Canonical | Impl. |'
  echo '|---|---|---|---|'
  awk '
    function flush() {
      if (id == "") return
      file = canon; sub(/^docs\/specifications\//, "", file)
      text = file; sub(/#.*/, "", text)
      printf "| %s | %s | [%s](%s) | %s |\n", id, title, text, file, impl
      id = ""
    }
    function unquote(s) {
      sub(/^[[:space:]]+/, "", s); sub(/[[:space:]]+$/, "", s)
      if (s ~ /^".*"$/ || s ~ /^\047.*\047$/) s = substr(s, 2, length(s) - 2)
      gsub(/\|/, "\\|", s)
      return s
    }
    { sub(/\r$/, "") }
    /^  - id: REQ-[0-9]+[[:space:]]*$/ { flush(); id = $3; title = canon = impl = ""; next }
    /^[[:space:]]*-[[:space:]]*id:/ {
      printf "spec-gen: traceability.yaml line %d: a row must start with exactly \"  - id: REQ-NNN\"\n", NR > "/dev/stderr"
      bad = 1; exit 1
    }
    id != "" && /^    title:/          { sub(/^    title:/, ""); title = unquote($0); next }
    id != "" && /^    canonical:/      { canon = $2; next }
    id != "" && /^    implementation:/ { impl = $2; next }
    END { if (!bad) flush(); exit bad }
  ' "$YAML" | sort -t'|' -k2,2
}

# The plan index: every docs/plans/*.md except the README and the template,
# grouped by the first word of its **Status:** line.
gen_plans() {
  local status f s
  for f in "${PLANS_DIR}"/[0-9]*.md; do
    [[ -f "$f" ]] || continue
    s="$(grep -m1 -E '^\*\*Status:\*\*' "$f" | sed -E 's/^\*\*Status:\*\*[[:space:]]*//; s/^\*\*//; s/[^A-Za-z].*$//' || true)"
    case "$s" in
      Active|Draft|Parked|Done) ;;
      *) echo "spec-gen: ${f#"${ROOT}/"}: **Status:** must start with Active, Draft, Parked or Done (found '${s}')" >&2
         return 1 ;;
    esac
  done
  for status in Active Draft Parked Done; do
    local rows=""
    for f in "${PLANS_DIR}"/[0-9]*.md; do
      [[ -f "$f" ]] || continue
      local s title covers name
      s="$(grep -m1 -E '^\*\*Status:\*\*' "$f" | sed -E 's/^\*\*Status:\*\*[[:space:]]*//; s/^\*\*//; s/[^A-Za-z].*$//')"
      [[ "$s" == "$status" ]] || continue
      name="$(basename "$f")"
      title="$(grep -m1 -E '^# ' "$f" | sed -E 's/^# (Plan — )?//; s/\|/\\|/g')"
      covers="$(grep -m1 -E '^\*\*Covers:\*\*' "$f" | grep -oE 'REQ-[0-9]{3}' | awk '!seen[$0]++' | paste -sd, - | sed 's/,/, /g' || true)"
      rows+="| [${name%.md}](${name}) | ${title} | ${covers:-—} |"$'\n'
    done
    [[ -n "$rows" ]] || continue
    printf '\n### %s\n\n| Plan | Title | Covers |\n|---|---|---|\n%s' "$status" "$rows"
  done
}

# Replace the lines between the BEGIN/END markers named <tag> in <file> with
# the output of <generator>; print the result on stdout.
splice() {
  local file="$1" tag="$2" gen="$3" body
  grep -q "<!-- BEGIN GENERATED: ${tag} " "$file" \
    || { echo "spec-gen: ${file#"${ROOT}/"} has no BEGIN GENERATED: ${tag} marker" >&2; return 1; }
  grep -q "<!-- END GENERATED: ${tag} " "$file" \
    || { echo "spec-gen: ${file#"${ROOT}/"} has no END GENERATED: ${tag} marker" >&2; return 1; }
  body="$("$gen")" || return 1
  # The body goes through ENVIRON, not -v, so awk does not eat its backslashes.
  BODY="$body" awk -v tag="$tag" '
    index($0, "<!-- BEGIN GENERATED: " tag " ") == 1 { print; print ENVIRON["BODY"]; skip = 1; next }
    skip && index($0, "<!-- END GENERATED: " tag " ") == 1 { skip = 0 }
    !skip { print }
  ' "$file"
}

stale=0
apply() {
  local file="$1" tag="$2" gen="$3" out
  out="$(splice "$file" "$tag" "$gen")" || exit 1
  out+=$'\n'
  if [[ $check -eq 1 ]]; then
    if [[ "$out" != "$(cat "$file")"$'\n' ]]; then
      echo "spec-gen: ${file#"${ROOT}/"} ${tag} block is stale — run make spec-gen" >&2
      stale=1
    fi
  else
    printf '%s' "$out" > "$file"
  fi
}

apply "$REQ_REG" registry gen_registry
apply "$PLANS_README" plans gen_plans

[[ $stale -eq 0 ]] || exit 1
[[ $check -eq 1 ]] || echo "spec-gen: regenerated REQ.md registry and plans/README.md index"
