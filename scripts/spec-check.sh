#!/usr/bin/env bash
# Verify specs/traceability.yaml against the working tree.
# Warn-only for planned REQs; fails on landed/partial entries with missing artefacts.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
YAML="${ROOT}/specs/traceability.yaml"
CONF="${ROOT}/specs/conformance.md"
REQ_REG="${ROOT}/specs/REQ.md"

fail=0
warn=0

die() { echo "spec-check: error: $*" >&2; fail=1; }
warn_msg() { echo "spec-check: warning: $*" >&2; warn=$((warn + 1)); }

[[ -f "$YAML" ]] || die "missing $YAML"
[[ -f "$CONF" ]] || die "missing $CONF"
[[ -f "$REQ_REG" ]] || die "missing $REQ_REG"

# --- helpers: parse simple YAML blocks (no external deps) -----------------

current_id=""
current_impl=""
in_packages=0
in_probes=0
in_tests=0
in_plans=0

flush_req() {
  [[ -n "$current_id" ]] || return 0
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
      grep -q "#### ${pr} " "$CONF" || die "$current_id: ${pr} not found in conformance.md"
    done
  fi
  current_id=""
  current_impl=""
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
    flush_req
    current_id="${BASH_REMATCH[1]}"
    continue
  fi
  if [[ -n "$current_id" && "$line" =~ implementation:[[:space:]]*(landed|partial|planned|withdrawn) ]]; then
    current_impl="${BASH_REMATCH[1]}"
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

# Registry mentions every yaml id
while IFS= read -r id; do
  [[ -z "$id" ]] && continue
  grep -q "| ${id} |" "$REQ_REG" || warn_msg "${id} in traceability.yaml but not in REQ.md registry"
done < <(grep -E '^[[:space:]]*-[[:space:]]*id:[[:space:]]*REQ-' "$YAML" | sed -E 's/.*id:[[:space:]]*//')

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
