#!/usr/bin/env bash
# Verify docs/specifications/traceability.yaml against the working tree.
#
# Fail (exit 1):
#   - REQ.md registry <-> traceability.yaml membership (both directions)
#   - REQ.md Impl. column agrees with traceability implementation
#   - landed/partial REQs cite existing packages/tests/plans and catalogued probes
#   - landed/partial REQs do not cite a probe with Status: Draft in conformance.md
#   - canonical: anchors resolve to a real heading in the target spec file
#   - status: is a valid spec-stability value (draft|stable|deprecated)
#   - prose counts restating the tree (probe census, all-three-modes tally,
#     runnable examples) agree with the tree; a reworded guarded sentence dies
#
# Warn only (exit 0 unless other errors):
#   - planned REQs with missing artefacts
#   - missing canonical: link in traceability.yaml
#   - yaml REQ ids absent from REQ.md registry
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
YAML="${ROOT}/docs/specifications/traceability.yaml"
CONF="${ROOT}/docs/specifications/conformance.md"
REQ_REG="${ROOT}/docs/specifications/REQ.md"

fail=0
warn=0
declare -A trace_impl       # REQ id -> implementation, captured from traceability.yaml
declare -A trace_canonical  # REQ id -> canonical "path#anchor"
declare -A anchor_set       # "relpath#slug" -> 1 (GitHub heading anchors, built lazily)
declare -A anchors_built    # relpath -> 1 (files whose anchors have been extracted)

die() { echo "spec-check: error: $*" >&2; fail=1; }
warn_msg() { echo "spec-check: warning: $*" >&2; warn=$((warn + 1)); }

[[ -f "$YAML" ]] || die "missing $YAML"
[[ -f "$CONF" ]] || die "missing $CONF"
[[ -f "$REQ_REG" ]] || die "missing $REQ_REG"

# --- helpers --------------------------------------------------------------

# GitHub-style heading slug: lowercase, drop everything but [a-z0-9 space hyphen],
# then spaces -> hyphens (consecutive specials collapse to repeated hyphens, e.g.
# "REQ-055 — Wire boundary" -> "req-055--wire-boundary"). Matches GitHub's anchor rule
# for the ASCII headings used in these specs.
#
# `_` is KEPT: GitHub preserves underscores in anchors, so stripping them here
# would falsely reject a canonical link to a heading like `EVENT_CONTEXT`.
slugify() {
  printf '%s' "$1" | LC_ALL=C tr '[:upper:]' '[:lower:]' \
    | LC_ALL=C sed -E 's/[^a-z0-9 _-]+//g' | LC_ALL=C tr ' ' '-'
}

# Strip trailing whitespace and an inline `# comment` from a block-list item.
# Paths and PROBE ids must not contain `#`; only ` # why` suffixes are stripped.
yaml_item() {
  local v="$1"
  v="$(printf '%s' "$v" | sed -E 's/[[:space:]]+#.*$//')"
  v="${v#"${v%%[![:space:]]*}"}"
  printf '%s' "${v%"${v##*[![:space:]]}"}"
}

reset_collectors() {
  in_packages=0
  in_probes=0
  in_tests=0
  in_plans=0
}

# Lazily extract every ATX heading anchor from a spec file into anchor_set["rel#slug"].
build_file_anchors() {
  local rel="$1" abs="${ROOT}/$1" line text
  [[ -n "${anchors_built[$rel]:-}" ]] && return 0
  anchors_built[$rel]=1
  [[ -f "$abs" ]] || return 0
  while IFS= read -r line; do
    text="$(printf '%s' "$line" | sed -E 's/^#+[[:space:]]+//; s/[[:space:]]+$//')"
    anchor_set["${rel}#$(slugify "$text")"]=1
  done < <(grep -E '^#{1,6}[[:space:]]+' "$abs")
}

# --- parse traceability.yaml (simple YAML blocks, no external deps) --------

current_id=""
current_impl=""
current_canonical=""
current_status=""
in_packages=0
in_probes=0
in_tests=0
in_plans=0

flush_req() {
  [[ -n "$current_id" ]] || return 0
  trace_impl["$current_id"]="$current_impl"
  trace_canonical["$current_id"]="$current_canonical"
  if [[ -n "$current_status" && ! "$current_status" =~ ^(draft|stable|deprecated)$ ]]; then
    die "$current_id: invalid status '$current_status' (expected draft|stable|deprecated — implementation status is a separate field)"
  fi
  # Out-of-vocabulary implementation values must fail loudly: an unmatched
  # value used to leave the row's implementation empty, silently skipping
  # every artefact check below (the REQ-116 'proposed' hole).
  if [[ -n "$current_impl" && ! "$current_impl" =~ ^(landed|partial|planned|deprecated)$ ]]; then
    die "$current_id: invalid implementation '$current_impl' (expected landed|partial|planned|deprecated)"
  fi
  if [[ "$current_impl" == "landed" || "$current_impl" == "partial" ]]; then
    if [[ ${#pkg_paths[@]} -eq 0 && ${#test_paths[@]} -eq 0 ]]; then
      die "$current_id ($current_impl): no packages or tests listed"
    fi
    for p in "${pkg_paths[@]}"; do
      [[ -e "${ROOT}/${p}" ]] || die "$current_id: missing package path ${p}"
    done
    for t in "${test_paths[@]}"; do
      [[ -f "${ROOT}/${t}" ]] || die "$current_id: missing test path ${t}"
    done
    for pl in "${plan_paths[@]}"; do
      [[ -f "${ROOT}/${pl}" ]] || die "$current_id: missing plan ${pl}"
    done
    for pr in "${probe_ids[@]}"; do
      if ! grep -qF "#### ${pr} " "$CONF"; then
        die "$current_id: ${pr} not found in conformance.md"
        continue
      fi
      # A landed/partial REQ must not claim a Draft (unimplemented) probe as coverage.
      if awk -v h="#### ${pr} " 'index($0,h)==1{f=1;next} f&&/^#### /{exit} f' "$CONF" \
           | grep -qiE '\*\*Status:\*\*[[:space:]]*Draft'; then
        die "$current_id: cites ${pr}, which is Status: Draft in conformance.md (not implemented coverage — drop it until its test lands)"
      fi
    done
  elif [[ "$current_impl" == "planned" ]]; then
    # Header contract: planned rows warn (never fail) on missing artefacts.
    for p in "${pkg_paths[@]}"; do
      [[ -e "${ROOT}/${p}" ]] || warn_msg "$current_id (planned): missing package path ${p}"
    done
    for t in "${test_paths[@]}"; do
      [[ -f "${ROOT}/${t}" ]] || warn_msg "$current_id (planned): missing test path ${t}"
    done
    for pl in "${plan_paths[@]}"; do
      [[ -f "${ROOT}/${pl}" ]] || warn_msg "$current_id (planned): missing plan ${pl}"
    done
  fi
  current_id=""
  current_impl=""
  current_canonical=""
  current_status=""
  pkg_paths=()
  probe_ids=()
  test_paths=()
  plan_paths=()
  reset_collectors
}

pkg_paths=()
probe_ids=()
test_paths=()
plan_paths=()

while IFS= read -r line || [[ -n "$line" ]]; do
  if [[ "$line" =~ ^[[:space:]]*-[[:space:]]*id:[[:space:]]*(REQ-[0-9]+) ]]; then
    # Capture before flush_req: its internal `[[ =~ ]]` tests clobber BASH_REMATCH.
    _next_id="${BASH_REMATCH[1]}"
    flush_req
    current_id="$_next_id"
    continue
  fi
  if [[ -n "$current_id" && "$line" =~ ^[[:space:]]*implementation:[[:space:]]*([A-Za-z-]+) ]]; then
    current_impl="${BASH_REMATCH[1]}"
    continue
  fi
  if [[ -n "$current_id" && "$line" =~ ^[[:space:]]*canonical:[[:space:]]*([^[:space:]]+) ]]; then
    current_canonical="${BASH_REMATCH[1]}"
    continue
  fi
  if [[ -n "$current_id" && "$line" =~ ^[[:space:]]*status:[[:space:]]*([A-Za-z]+) ]]; then
    current_status="${BASH_REMATCH[1]}"
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]*packages:[[:space:]]*\[(.*)\][[:space:]]*$ ]]; then
    IFS=',' read -ra _parts <<< "${BASH_REMATCH[1]}"
    for _p in "${_parts[@]}"; do
      _p="${_p// /}"
      _p="${_p//\"/}"
      [[ -n "$_p" ]] && pkg_paths+=("$_p")
    done
    reset_collectors
    continue
  fi
  # Block-form `packages:` — without this arm a multi-line package list was
  # collected by nobody, so its paths were never existence-checked.
  if [[ "$line" =~ ^[[:space:]]*packages:[[:space:]]*(#.*)?$ ]]; then
    in_packages=1
    in_probes=0
    in_tests=0
    in_plans=0
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]*probes:[[:space:]]*\[(.*)\][[:space:]]*$ ]]; then
    IFS=',' read -ra _parts <<< "${BASH_REMATCH[1]}"
    for _p in "${_parts[@]}"; do
      _p="${_p// /}"
      [[ -n "$_p" ]] && probe_ids+=("$_p")
    done
    reset_collectors
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]*probes:[[:space:]]*(#.*)?$ ]]; then
    in_probes=1
    in_packages=0
    in_tests=0
    in_plans=0
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]*tests:[[:space:]]*(#.*)?$ ]]; then
    in_tests=1
    in_packages=0
    in_probes=0
    in_plans=0
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]*plans:[[:space:]]*\[(.*)\][[:space:]]*$ ]]; then
    IFS=',' read -ra _parts <<< "${BASH_REMATCH[1]}"
    for _p in "${_parts[@]}"; do
      _p="${_p// /}"
      [[ -n "$_p" ]] && plan_paths+=("$_p")
    done
    reset_collectors
    continue
  fi
  # Block-form `plans:` — these used to fall through into the still-open
  # `tests:` collector and pass the -f check only because plans are files.
  if [[ "$line" =~ ^[[:space:]]*plans:[[:space:]]*(#.*)?$ ]]; then
    in_plans=1
    in_packages=0
    in_probes=0
    in_tests=0
    continue
  fi
  if [[ $in_packages -eq 1 && "$line" =~ ^[[:space:]]*-[[:space:]]*(.+)$ ]]; then
    _v="$(yaml_item "${BASH_REMATCH[1]}")"
    [[ -n "$_v" ]] && pkg_paths+=("$_v")
    continue
  fi
  if [[ $in_plans -eq 1 && "$line" =~ ^[[:space:]]*-[[:space:]]*(.+)$ ]]; then
    _v="$(yaml_item "${BASH_REMATCH[1]}")"
    [[ -n "$_v" ]] && plan_paths+=("$_v")
    continue
  fi
  if [[ $in_tests -eq 1 && "$line" =~ ^[[:space:]]*-[[:space:]]*(.+)$ ]]; then
    _v="$(yaml_item "${BASH_REMATCH[1]}")"
    [[ -n "$_v" ]] && test_paths+=("$_v")
    continue
  fi
  if [[ $in_probes -eq 1 && "$line" =~ ^[[:space:]]*-[[:space:]]*(PROBE-[0-9]+) ]]; then
    probe_ids+=("${BASH_REMATCH[1]}")
    continue
  fi
  # REQ-row fields we do not parse (adrs, fixtures, notes, title, …) must
  # not leave a block collector armed for the next list.
  if [[ -n "$current_id" && "$line" =~ ^[[:space:]]{4}[a-z_]+: ]]; then
    reset_collectors
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]*-[[:space:]]*id: ]]; then
    reset_collectors
  fi
done < "$YAML"
flush_req

# Canonical anchors resolve to a real heading in the target spec file.
for id in $(printf '%s\n' "${!trace_canonical[@]}" | sort); do
  c="${trace_canonical[$id]}"
  if [[ -z "$c" ]]; then
    warn_msg "${id}: no canonical link in traceability.yaml"
    continue
  fi
  rel="${c%%#*}"
  if [[ ! -f "${ROOT}/${rel}" ]]; then
    die "${id}: canonical file missing: ${rel}"
    continue
  fi
  [[ "$c" == *#* ]] || continue   # whole-file reference (no anchor)
  anchor="${c#*#}"
  build_file_anchors "$rel"
  [[ -n "${anchor_set["${rel}#${anchor}"]:-}" ]] \
    || die "${id}: canonical anchor '#${anchor}' does not resolve to a heading in ${rel}"
done

# Registry mentions every yaml id
while IFS= read -r id; do
  [[ -z "$id" ]] && continue
  grep -qF "| ${id} |" "$REQ_REG" || warn_msg "${id} in traceability.yaml but not in REQ.md registry"
done < <(grep -E '^[[:space:]]*-[[:space:]]*id:[[:space:]]*REQ-' "$YAML" | sed -E 's/.*id:[[:space:]]*//')

# Every REQ.md registry id has a traceability.yaml entry (completeness — no silent gaps)
while IFS= read -r id; do
  [[ -z "$id" ]] && continue
  grep -qE "^[[:space:]]*-[[:space:]]*id:[[:space:]]*${id}([[:space:]]|$)" "$YAML" \
    || die "${id} in REQ.md registry but missing from traceability.yaml"
done < <(grep -E '^\| REQ-[0-9]{3} ' "$REQ_REG" | sed -E 's/^\| (REQ-[0-9]{3}) .*/\1/')

# REQ.md Impl. column must agree with traceability.yaml implementation (no drift)
while read -r id impl; do
  ti="${trace_impl[$id]:-}"
  [[ -z "$ti" ]] && continue   # missing entry already reported by the completeness check
  [[ "$impl" == "$ti" ]] || die "${id}: REQ.md Impl '${impl}' disagrees with traceability implementation '${ti}'"
done < <(awk -F'|' '/^\| REQ-[0-9]{3} /{id=$2; impl=$(NF-1); gsub(/ /,"",id); gsub(/ /,"",impl); print id, impl}' "$REQ_REG")

# --- prose counts that restate the tree ------------------------------------
#
# A sentence that repeats a number the tree can produce rots quietly: the
# probe census below was wrong in two consecutive PRs, and roadmap.md's
# example tally sat a program behind cmd/examples/. Each guard derives the
# number from the tree and compares it with the one sentence stating it.
# A guard whose sentence no longer matches DIES rather than skipping — a
# guard that silently matches nothing is the drift it was added to catch.

# grep -c exits 1 when nothing matches, which `set -e` would take as fatal.
count_matches() { grep -Ec -- "$2" "$1" || true; }

# Put the sole line of <file> matching <ere> in SOLE_LINE; die and return 1
# when the match count is anything but one (missing file included). Never
# call this in a command substitution: `die` must run in the current shell.
SOLE_LINE=""
sole_line() {
  local file="$1" ere="$2" what="$3" rel n
  SOLE_LINE=""
  rel="${file#"${ROOT}/"}"
  if [[ ! -f "$file" ]]; then
    die "missing ${rel}, which carries the ${what} sentence"
    return 1
  fi
  n="$(count_matches "$file" "$ere")"
  if [[ "$n" -ne 1 ]]; then
    die "${rel}: expected exactly one ${what} sentence matching /${ere}/, found ${n} — reword the sentence and this guard's regex in scripts/spec-check.sh in the same change"
    return 1
  fi
  SOLE_LINE="$(grep -E -- "$ere" "$file")"
}

# Compare the numeral a sentence states with the count derived from the tree.
count_must_match() {
  local rel="$1" what="$2" stated="$3" actual="$4" from="$5"
  [[ "$stated" == "$actual" ]] \
    || die "${rel}: ${what} says ${stated}, tree has ${actual} (${from}) — move the sentence in the change that moved the count"
}

CONF_REL="docs/specifications/conformance.md"
EXAMPLES="${ROOT}/docs/examples.md"
ROADMAP="${ROOT}/docs/roadmap.md"

if [[ -f "$CONF" ]]; then
  probe_total="$(count_matches "$CONF" '^#### PROBE-')"
  inrepo_total="$(count_matches "$CONF" '^- \*\*Modes:\*\*.*In-repo')"
  # Canonical all-three spelling only. The `Sandbox (planned); Cassette, Live
  # not yet scoped.` lines deliberately do NOT match: their Sandbox is planned
  # and the other two unscoped, so they declare no mode the sentence counts.
  allthree_total="$(count_matches "$CONF" '^- \*\*Modes:\*\* Sandbox, Cassette, Live\.')"

  # "<in-repo> of the <total> catalog entries are in-repo by construction".
  census_re='[0-9]+ of the [0-9]+ catalog entries are in-repo by construction'
  if sole_line "$CONF" "$census_re" 'probe census'; then
    _frag="$(printf '%s' "$SOLE_LINE" | grep -oE "$census_re")"
    count_must_match "$CONF_REL" 'probe census (in-repo entries)' \
      "$(printf '%s' "$_frag" | sed -E 's/^([0-9]+) of the ([0-9]+) .*/\1/')" \
      "$inrepo_total" "'- **Modes:**' lines declaring In-repo"
    count_must_match "$CONF_REL" 'probe census (catalog total)' \
      "$(printf '%s' "$_frag" | sed -E 's/^([0-9]+) of the ([0-9]+) .*/\2/')" \
      "$probe_total" "'#### PROBE-' headings"
  fi

  # "Today <n> entries declare all three".
  allthree_re='Today [0-9]+ entries declare all three'
  if sole_line "$CONF" "$allthree_re" 'all-three-modes'; then
    count_must_match "$CONF_REL" 'all-three-modes tally' \
      "$(printf '%s' "$SOLE_LINE" | grep -oE "$allthree_re" | sed -E 's/^Today ([0-9]+) .*/\1/')" \
      "$allthree_total" "'- **Modes:** Sandbox, Cassette, Live.' lines"
  fi

  # Every catalog entry states its modes exactly once — both counts above read
  # Modes lines, so a probe carrying none (or two) would skew them unnoticed.
  while read -r _pr _n; do
    [[ -z "$_pr" ]] && continue
    die "${CONF_REL}: ${_pr} has ${_n} '- **Modes:**' lines (expected exactly 1)"
  done < <(awk '
    /^#### PROBE-/ { if (id != "" && n != 1) print id, n; id = $2; n = 0; next }
    /^- \*\*Modes:\*\*/ { n++ }
    END { if (id != "" && n != 1) print id, n }
  ' "$CONF")
fi

# Runnable example programs: a directory under cmd/examples/ is a program only
# when it holds a main.go — doc.go and scaffold-only directories are not.
example_count=0
for _f in "${ROOT}"/cmd/examples/*/main.go; do
  [[ -f "$_f" ]] && example_count=$((example_count + 1))
done

ex_re='The [0-9]+ runnable programs'
if sole_line "$EXAMPLES" "$ex_re" 'runnable-example count'; then
  count_must_match "docs/examples.md" 'runnable-example count' \
    "$(printf '%s' "$SOLE_LINE" | grep -oE "$ex_re" | sed -E 's/^The ([0-9]+) .*/\1/')" \
    "$example_count" 'cmd/examples/*/main.go'
fi

road_re='[0-9]+ runnable programs'
if sole_line "$ROADMAP" "$road_re" 'runnable-example count'; then
  count_must_match "docs/roadmap.md" 'runnable-example count' \
    "$(printf '%s' "$SOLE_LINE" | grep -oE "$road_re" | sed -E 's/^([0-9]+) .*/\1/')" \
    "$example_count" 'cmd/examples/*/main.go'
fi

if [[ $fail -ne 0 ]]; then
  echo "spec-check: FAILED" >&2
  exit 1
fi

if [[ $warn -ne 0 ]]; then
  echo "spec-check: OK with ${warn} warning(s)"
else
  echo "spec-check: OK"
fi
exit 0
