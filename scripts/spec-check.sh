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
slugify() {
  printf '%s' "$1" | LC_ALL=C tr '[:upper:]' '[:lower:]' \
    | LC_ALL=C sed -E 's/[^a-z0-9 -]+//g' | LC_ALL=C tr ' ' '-'
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
  fi
  current_id=""
  current_impl=""
  current_canonical=""
  current_status=""
  pkg_paths=()
  probe_ids=()
  test_paths=()
  plan_paths=()
  in_packages=0
  in_probes=0
  in_tests=0
  in_plans=0
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
  if [[ -n "$current_id" && "$line" =~ implementation:[[:space:]]*(landed|partial|planned|deprecated) ]]; then
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
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]*probes:[[:space:]]*\[(.*)\][[:space:]]*$ ]]; then
    IFS=',' read -ra _parts <<< "${BASH_REMATCH[1]}"
    for _p in "${_parts[@]}"; do
      _p="${_p// /}"
      [[ -n "$_p" ]] && probe_ids+=("$_p")
    done
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]*tests:[[:space:]]*$ ]]; then
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
    in_tests=0
    continue
  fi
  if [[ $in_tests -eq 1 && "$line" =~ ^[[:space:]]*-[[:space:]]*(.+)[[:space:]]*$ ]]; then
    test_paths+=("${BASH_REMATCH[1]}")
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]*-[[:space:]]*id: ]]; then
    in_tests=0
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
