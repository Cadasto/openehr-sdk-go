#!/usr/bin/env bash
# Negative fixtures for scripts/spec-check.sh — the guard's own guard.
#
# spec-check.sh parses traceability.yaml with hand-rolled collectors, so a
# deleted or mis-anchored arm fails silently: the gate stays green while a
# whole list goes unchecked (the block-form `packages:` hole was exactly
# that). Its prose-count guards have the same shape: a sentence reworded out
# from under a regex would leave the count unchecked. Each case below plants
# one fault in a minimal fake ROOT and asserts the checker exits 1 naming it
# — removing a collector arm or a count guard MUST fail here.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
CHECK="${HERE}/spec-check.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
fail=0

# Minimal tree that passes spec-check: one landed REQ with block-form
# packages / probes / tests / plans, a registry row, a four-probe catalog
# (one Draft) whose census/all-three sentences are true, and two runnable
# example programs matching the tallies in examples.md and roadmap.md. The
# checker resolves ROOT from its own location, so copying it into
# <root>/scripts/ points it at the fixture.
build_baseline() {
  local root="$1"
  mkdir -p "$root/scripts" "$root/docs/specifications" "$root/docs/plans" "$root/pkg/good" \
    "$root/cmd/examples/alpha" "$root/cmd/examples/beta" "$root/cmd/examples/scaffold"
  cp "$CHECK" "$root/scripts/spec-check.sh"
  touch "$root/pkg/good/good_test.go" "$root/docs/plans/plan.md"
  # Two programs; `scaffold/` holds no main.go, so it is not one.
  touch "$root/cmd/examples/alpha/main.go" "$root/cmd/examples/beta/main.go" \
    "$root/cmd/examples/scaffold/README.md"
  cat > "$root/docs/specifications/topic.md" <<'EOF'
# Topic spec

### REQ-001 — Alpha

Normative fixture text.
EOF
  cat > "$root/docs/specifications/conformance.md" <<'EOF'
# Conformance catalog

Not every probe is backend-facing — 1 of the 4 catalog entries are in-repo by construction.
Today 2 entries declare all three; the rest are open gaps.

#### PROBE-001 — Alpha round-trip

**Status:** Implemented (Sandbox)
- **Modes:** Sandbox, Cassette, Live.

#### PROBE-002 — Beta placeholder

**Status:** Draft
- **Modes:** Sandbox.

#### PROBE-003 — Gamma property

**Status:** Implemented (Sandbox)
- **Modes:** In-repo (unit-level property; no backend).

#### PROBE-004 — Delta round-trip

**Status:** Implemented (Sandbox)
- **Modes:** Sandbox, Cassette, Live.
EOF
  cat > "$root/docs/examples.md" <<'EOF'
# Examples

The 2 runnable programs under `cmd/examples/` demonstrate each SDK surface.
EOF
  cat > "$root/docs/roadmap.md" <<'EOF'
# Roadmap

| Worked examples | **Landed** | `cmd/examples/` — 2 runnable programs, catalogued in examples.md |
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

# 8 — probe census: in-repo numeral one ahead of the In-repo Modes lines.
r="$(new_case census-inrepo)"
sed -i 's/— 1 of the 4 catalog/— 2 of the 4 catalog/' "$r/docs/specifications/conformance.md"
check census-inrepo "$r" fail "probe census (in-repo entries) says 2, tree has 1"

# 9 — probe census: catalog total drifting from the PROBE headings.
r="$(new_case census-total)"
sed -i 's/of the 4 catalog/of the 5 catalog/' "$r/docs/specifications/conformance.md"
check census-total "$r" fail "probe census (catalog total) says 5, tree has 4"

# 10 — all-three-modes tally ahead of the canonical Modes lines.
r="$(new_case allthree-tally)"
sed -i 's/Today 2 entries/Today 3 entries/' "$r/docs/specifications/conformance.md"
check allthree-tally "$r" fail "all-three-modes tally says 3, tree has 2"

# 11 — a `Sandbox (planned); Cassette, Live not yet scoped.` entry declares no
#      mode: it must lift the catalog total and leave the all-three tally alone.
r="$(new_case planned-modes)"
cat >> "$r/docs/specifications/conformance.md" <<'EOF'

#### PROBE-005 — Epsilon placeholder

**Status:** Draft
- **Modes:** Sandbox (planned); Cassette, Live not yet scoped.
EOF
sed -i 's/of the 4 catalog/of the 5 catalog/' "$r/docs/specifications/conformance.md"
check planned-modes "$r" ok

# 12 — docs/examples.md tally ahead of cmd/examples/.
r="$(new_case examples-count)"
sed -i 's/^The 2 runnable/The 3 runnable/' "$r/docs/examples.md"
check examples-count "$r" fail "docs/examples.md: runnable-example count says 3, tree has 2"

# 13 — docs/roadmap.md tally behind cmd/examples/.
r="$(new_case roadmap-count)"
sed -i 's/2 runnable programs/1 runnable programs/' "$r/docs/roadmap.md"
check roadmap-count "$r" fail "docs/roadmap.md: runnable-example count says 1, tree has 2"

# 14 — a program lands and neither doc moves (the drift that actually shipped).
r="$(new_case examples-tree-grew)"
mkdir -p "$r/cmd/examples/gamma"
touch "$r/cmd/examples/gamma/main.go"
check examples-tree-grew "$r" fail "runnable-example count says 2, tree has 3"

# 15 — the census sentence reworded out from under its regex: a guard that
#      matches nothing must die, not skip.
r="$(new_case census-reworded)"
sed -i 's/catalog entries are in-repo by construction/catalog entries need no backend/' \
  "$r/docs/specifications/conformance.md"
check census-reworded "$r" fail "expected exactly one probe census sentence"

# 16 — the examples tally sentence deleted outright.
r="$(new_case examples-sentence-gone)"
sed -i '/runnable programs/d' "$r/docs/examples.md"
check examples-sentence-gone "$r" fail "runnable-example count sentence"

# 17 — a catalog entry left without a Modes line (both tallies read those).
r="$(new_case probe-without-modes)"
sed -i '/^- \*\*Modes:\*\* In-repo/d' "$r/docs/specifications/conformance.md"
check probe-without-modes "$r" fail "PROBE-003 has 0"

if [[ $fail -ne 0 ]]; then
  echo "spec-check-selftest: FAILED" >&2
  exit 1
fi
echo "spec-check-selftest: OK (17 cases)"
