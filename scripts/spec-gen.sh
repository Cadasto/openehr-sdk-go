#!/usr/bin/env bash
# Regenerate the SDD index blocks from their single sources:
#
#   traceability.yaml tests:    each row's list <- the test files that cite its REQ
#   docs/specifications/REQ.md  registry table  <- docs/specifications/traceability.yaml
#   docs/plans/README.md        plan index      <- the **Status:** / **Covers:** headers
#                                                  of docs/plans/*.md
#
# In REQ.md and plans/README.md only the text between the BEGIN/END GENERATED
# markers is rewritten; the prose around it is hand-written. In the map only
# the rows' tests: lists are rewritten. Never edit a generated part by hand:
# edit the source and run `make spec-gen`.
#
# A test file cites REQ-NNN when the token REQ-NNN appears in it with no
# letter, digit or underscore right before or after it (grep -w), so a hyphen
# or a dot is a boundary and pre-REQ-117 counts. Test files are the *_test.go files git tracks or would track (new
# files count, ignored ones do not), plus every .go file under testkit/probes/,
# where the probe implementations live.
#
# Usage: spec-gen.sh           rewrite the generated parts in place
#        spec-gen.sh --check   exit 1 if any part is stale (spec-check runs this)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
YAML="${ROOT}/docs/specifications/traceability.yaml"
REQ_REG="${ROOT}/docs/specifications/REQ.md"
PLANS_DIR="${ROOT}/docs/plans"
PLANS_README="${PLANS_DIR}/README.md"

check=0
[[ "${1:-}" == "--check" ]] && check=1

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Print "REQ-NNN<TAB>path" for every test file citing REQ-NNN, sorted.
gen_citations() {
  local files
  files="$(git -C "$ROOT" ls-files --cached --others --exclude-standard -- '*_test.go' 'testkit/probes/*.go' 2>/dev/null)" \
    || { echo "spec-gen: cannot list the test files (git ls-files failed in ${ROOT})" >&2; return 1; }
  [[ -n "$files" ]] || return 0
  # A tracked file deleted from the working tree is not a test file any more.
  # grep exits 1 when nothing matches, which xargs reports as 123; any other
  # failure is fatal.
  local rc=0
  ( cd "$ROOT" && printf '%s\n' "$files" | while IFS= read -r f; do
      [[ -f "$f" ]] && printf '%s\0' "$f"
    done | xargs -0 grep -owHE 'REQ-[0-9]+' /dev/null ) > "${TMP}/cites" || rc=$?
  [[ $rc -eq 0 || $rc -eq 123 ]] \
    || { echo "spec-gen: scanning the test files for REQ citations failed (exit ${rc})" >&2; return 1; }
  awk '{ i = length($0); while (substr($0, i, 1) != ":") i--; print substr($0, i + 1) "\t" substr($0, 1, i - 1) }' \
    "${TMP}/cites" | LC_ALL=C sort -u
}

# Rewrite traceability.yaml with each row's tests: list replaced by the files
# that cite the row's REQ (block form, sorted; no key when none cite it).
# A row without a tests: key gets one after its last line.
#   render_tests write   print the rewritten map
#   render_tests diag    print one line per stale row, naming the first difference
render_tests() {
  local mode="$1"
  awk -v mode="$mode" -v cf="${TMP}/citations" '
    BEGIN {
      MARK = "\001tests"
      while ((getline line < cf) > 0) {
        split(line, f, "\t")
        k = ++cnt[f[1]]; cite[f[1], k] = f[2]
      }
    }
    function emit(   k) {
      if (emitted || cnt[id] == 0) return
      emitted = 1
      print "    tests:"
      for (k = 1; k <= cnt[id]; k++) print "      - " cite[id, k]
    }
    function diag(   k, a, b, why, p, inold, innew) {
      for (k = 1; k <= oldn || k <= cnt[id]; k++) {
        a = (k <= oldn) ? old[k] : ""
        b = (k <= cnt[id]) ? cite[id, k] : ""
        if (a == b) continue
        split("", inold); split("", innew)
        for (p = 1; p <= oldn; p++) inold[old[p]] = 1
        for (p = 1; p <= cnt[id]; p++) innew[cite[id, p]] = 1
        if (b != "" && !(b in inold))      why = b " cites " id " but is not listed"
        else if (a != "" && !(a in innew)) why = a " is listed but does not cite " id
        else                               why = "the list is not in sorted order at " a
        printf "spec-gen: traceability.yaml %s tests list is stale: %s; run make spec-gen\n", id, why
        return
      }
      # Same paths in the same order: only the layout can differ.
      if (inline)
        printf "spec-gen: traceability.yaml %s tests list is not in block form; run make spec-gen\n", id
      else if (haskey && cnt[id] == 0)
        printf "spec-gen: traceability.yaml %s has an empty tests: key; run make spec-gen\n", id
    }
    function flush(   i, last) {
      if (id == "") return
      if (mode == "diag") diag()
      else {
        last = 0
        for (i = 1; i <= n; i++) if (buf[i] !~ /^[[:space:]]*$/) last = i
        for (i = 1; i <= n; i++) {
          if (buf[i] == MARK) emit(); else print buf[i]
          if (i == last && !haskey) emit()
        }
      }
      id = ""; n = 0; oldn = 0; haskey = 0; emitted = 0; intests = 0; inline = 0
    }
    { sub(/\r$/, "") }
    /^  - id: REQ-[0-9]+[[:space:]]*$/ { flush(); id = $3; buf[n = 1] = $0; next }
    id == "" { if (mode != "diag") print; next }
    intests && /^      - / { v = $0; sub(/^      - /, "", v); sub(/[[:space:]]+$/, "", v); old[++oldn] = v; next }
    { intests = 0 }
    /^    tests:/ {
      haskey = 1; buf[++n] = MARK
      v = $0; sub(/^    tests:[[:space:]]*/, "", v)
      if (v == "") { intests = 1; next }
      inline = 1
      gsub(/[][[:space:]]/, "", v)
      k = split(v, parts, ","); for (i = 1; i <= k; i++) if (parts[i] != "") old[++oldn] = parts[i]
      next
    }
    { buf[++n] = $0 }
    END { flush() }
  ' "$YAML"
}

stale=0
apply_tests() {
  local out
  gen_citations > "${TMP}/citations" || exit 1
  out="$(render_tests write)" || exit 1
  [[ "$out" == "$(tr -d '\r' < "$YAML")" ]] && return 0
  if [[ $check -eq 1 ]]; then
    render_tests diag >&2
    echo "spec-gen: docs/specifications/traceability.yaml tests lists are stale; run make spec-gen" >&2
    stale=1
  else
    printf '%s\n' "$out" > "$YAML"
  fi
}

# The registry: one row per traceability.yaml entry, in id order.
gen_registry() {
  echo '| ID | Title | Canonical | Stability | Impl. |'
  echo '|---|---|---|---|---|'
  awk '
    function flush() {
      if (id == "") return
      file = canon; sub(/^docs\/specifications\//, "", file)
      text = file; sub(/#.*/, "", text)
      printf "| %s | %s | [%s](%s) | %s | %s |\n", id, title, text, file, stab, impl
      id = ""
    }
    function unquote(s) {
      sub(/^[[:space:]]+/, "", s); sub(/[[:space:]]+$/, "", s)
      if (s ~ /^".*"$/ || s ~ /^\047.*\047$/) s = substr(s, 2, length(s) - 2)
      gsub(/\|/, "\\|", s)
      return s
    }
    { sub(/\r$/, "") }
    /^  - id: REQ-[0-9]+[[:space:]]*$/ { flush(); id = $3; title = canon = stab = impl = ""; next }
    /^[[:space:]]*-[[:space:]]*id:/ {
      printf "spec-gen: traceability.yaml line %d: a row must start with exactly \"  - id: REQ-NNN\"\n", NR > "/dev/stderr"
      bad = 1; exit 1
    }
    id != "" && /^    title:/          { sub(/^    title:/, ""); title = unquote($0); next }
    id != "" && /^    canonical:/      { canon = $2; next }
    id != "" && /^    status:/         { stab = $2; next }
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

apply_tests
apply "$REQ_REG" registry gen_registry
apply "$PLANS_README" plans gen_plans

[[ $stale -eq 0 ]] || exit 1
[[ $check -eq 1 ]] || echo "spec-gen: regenerated the map's tests lists, the REQ.md registry and the plans/README.md index"
