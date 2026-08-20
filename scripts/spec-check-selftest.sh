#!/usr/bin/env bash
# Negative fixtures for scripts/spec-check.sh — the guard's own guard.
#
# spec-check.sh parses traceability.yaml with hand-rolled collectors, so a
# deleted or mis-anchored arm fails silently: the gate stays green while a
# whole list goes unchecked (the block-form `packages:` hole was exactly
# that). Each case below plants one fault in a minimal fake ROOT and asserts
# the checker exits 1 naming it — removing a collector arm MUST fail here.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
CHECK="${HERE}/spec-check.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
fail=0

# Minimal tree that passes spec-check: one landed REQ with block-form
# packages / probes / tests / plans, a two-probe catalog (one Draft), and a
# registry row. The checker resolves ROOT from its own location, so copying
# it into <root>/scripts/ points it at the fixture.
build_baseline() {
  local root="$1"
  mkdir -p "$root/scripts" "$root/docs/specifications" "$root/docs/plans" "$root/pkg/good"
  cp "$CHECK" "$root/scripts/spec-check.sh"
  touch "$root/pkg/good/good_test.go" "$root/docs/plans/plan.md"
  cat > "$root/docs/specifications/topic.md" <<'EOF'
# Topic spec

### REQ-001 — Alpha

Normative fixture text.
EOF
  cat > "$root/docs/specifications/conformance.md" <<'EOF'
# Conformance catalog

#### PROBE-001 — Alpha round-trip

**Status:** Implemented (Sandbox)

#### PROBE-002 — Beta placeholder

**Status:** Draft
EOF
  cat > "$root/docs/specifications/REQ.md" <<'EOF'
| Id | Title | Canonical | Impl. |
|---|---|---|---|
| REQ-001 | Alpha | topic.md | landed |
EOF
  cat > "$root/docs/specifications/traceability.yaml" <<'EOF'
requirements:
  - id: REQ-001
    title: Alpha
    canonical: docs/specifications/topic.md#req-001--alpha
    status: stable
    implementation: landed
    packages:
      - pkg/good  # inline comments on items must be stripped
    probes:
      - PROBE-001
    tests:
      - pkg/good/good_test.go
    plans:
      - docs/plans/plan.md
EOF
}

new_case() {
  local root="$TMP/$1"
  build_baseline "$root"
  printf '%s' "$root"
}

# check <name> <root> ok|fail [needle] — run the copied checker, assert the
# exit code, and on expected failure require the diagnostic to name the fault.
check() {
  local name="$1" root="$2" want="$3" needle="${4:-}"
  local out rc=0
  out="$(bash "$root/scripts/spec-check.sh" 2>&1)" || rc=$?
  if [[ "$want" == ok ]]; then
    if [[ $rc -ne 0 ]]; then
      echo "spec-check-selftest: FAIL ${name}: expected OK, exit ${rc}:" >&2
      echo "$out" >&2
      fail=1
    fi
  else
    if [[ $rc -eq 0 ]]; then
      echo "spec-check-selftest: FAIL ${name}: planted fault passed the gate" >&2
      fail=1
    elif [[ -n "$needle" && "$out" != *"$needle"* ]]; then
      echo "spec-check-selftest: FAIL ${name}: failed without naming '${needle}':" >&2
      echo "$out" >&2
      fail=1
    fi
  fi
}

# 1 — baseline is green (also pins yaml_item's comment stripping: the one
#     good package item carries an inline `# why`).
r="$(new_case baseline)"
check baseline "$r" ok

# 2 — nonexistent path in a block-form packages list.
r="$(new_case block-package)"
sed -i 's|^      - pkg/good  .*|&\n      - pkg/absent|' "$r/docs/specifications/traceability.yaml"
check block-package "$r" fail "missing package path pkg/absent"

# 3 — nonexistent path in a block-form plans list.
r="$(new_case block-plan)"
sed -i 's|^      - docs/plans/plan.md$|&\n      - docs/plans/absent.md|' "$r/docs/specifications/traceability.yaml"
check block-plan "$r" fail "missing plan docs/plans/absent.md"

# 4 — block-form probes citing an uncatalogued probe.
r="$(new_case block-probe)"
sed -i 's|^      - PROBE-001$|      - PROBE-999|' "$r/docs/specifications/traceability.yaml"
check block-probe "$r" fail "PROBE-999 not found"

# 5 — a `# comment` on the key line must still arm the collector.
r="$(new_case commented-key)"
sed -i -e 's|^    packages:$|    packages:  # directories only|' \
       -e 's|^      - pkg/good  .*|      - pkg/absent|' "$r/docs/specifications/traceability.yaml"
check commented-key "$r" fail "missing package path pkg/absent"

# 6 — a landed row citing a Status: Draft probe.
r="$(new_case draft-probe)"
sed -i 's|^      - PROBE-001$|      - PROBE-002|' "$r/docs/specifications/traceability.yaml"
check draft-probe "$r" fail "Status: Draft"

# 7 — REQ.md Impl column drifting from the map.
r="$(new_case impl-drift)"
sed -i 's/| landed |/| planned |/' "$r/docs/specifications/REQ.md"
check impl-drift "$r" fail "disagrees"

if [[ $fail -ne 0 ]]; then
  echo "spec-check-selftest: FAILED" >&2
  exit 1
fi
echo "spec-check-selftest: OK (7 cases)"
