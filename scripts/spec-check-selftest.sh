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
GEN="${HERE}/spec-gen.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
fail=0

# Minimal tree that passes spec-check: one landed REQ with block-form
# packages / probes / tests / plans, a registry row, a four-probe catalog
# (one Draft) whose census/all-three sentences are true, two runnable
# example programs matching the tally in examples.md, and one Active plan.
# The generated blocks are produced by the real spec-gen.sh. The checker
# resolves ROOT from its own location, so copying it into <root>/scripts/
# points it at the fixture.
build_baseline() {
  local root="$1"
  mkdir -p "$root/scripts" "$root/docs/specifications" "$root/docs/plans" "$root/pkg/good" \
    "$root/cmd/examples/alpha" "$root/cmd/examples/beta" "$root/cmd/examples/scaffold"
  cp "$CHECK" "$root/scripts/spec-check.sh"
  cp "$GEN" "$root/scripts/spec-gen.sh"
  touch "$root/pkg/good/good_test.go"
  cat > "$root/docs/plans/2026-01-01-plan.md" <<'EOF'
# Plan — Alpha

**Status:** Active — under way
**Covers:** REQ-001
EOF
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
  cat > "$root/docs/specifications/REQ.md" <<'EOF'
# Registry

<!-- BEGIN GENERATED: registry (fixture) -->
<!-- END GENERATED: registry -->
EOF
  cat > "$root/docs/plans/README.md" <<'EOF'
# Plans

<!-- BEGIN GENERATED: plans (fixture) -->
<!-- END GENERATED: plans -->
EOF
  cat > "$root/docs/specifications/traceability.yaml" <<'EOF'
requirements:
  - id: REQ-001
    title: Alpha
    canonical: docs/specifications/topic.md#req-001--alpha
    status: stable
    implementation: landed
    packages:
      - pkg/good
    probes:
      - PROBE-001
    tests:
      - pkg/good/good_test.go
    plans:
      - docs/plans/2026-01-01-plan.md
EOF
  bash "$root/scripts/spec-gen.sh" >/dev/null
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

# 1 — baseline is green.
r="$(new_case baseline)"
check baseline "$r" ok

# 2 — nonexistent path in a block-form packages list.
r="$(new_case block-package)"
sed -i 's|^      - pkg/good$|&\n      - pkg/absent|' "$r/docs/specifications/traceability.yaml"
check block-package "$r" fail "missing package path pkg/absent"

# 3 — nonexistent path in a block-form plans list.
r="$(new_case block-plan)"
sed -i 's|^      - docs/plans/2026-01-01-plan.md$|&\n      - docs/plans/absent.md|' "$r/docs/specifications/traceability.yaml"
check block-plan "$r" fail "missing plan docs/plans/absent.md"

# 4 — block-form probes citing an uncatalogued probe.
r="$(new_case block-probe)"
sed -i 's|^      - PROBE-001$|      - PROBE-999|' "$r/docs/specifications/traceability.yaml"
check block-probe "$r" fail "PROBE-999 not found"

# 5 — a comment inside a row (the map is an index, not a notebook).
r="$(new_case row-comment)"
sed -i 's|^      - pkg/good$|      - pkg/good  # why this package|' "$r/docs/specifications/traceability.yaml"
check row-comment "$r" fail "comment in a row"

# 6 — a landed row citing a Status: Draft probe.
r="$(new_case draft-probe)"
sed -i 's|^      - PROBE-001$|      - PROBE-002|' "$r/docs/specifications/traceability.yaml"
check draft-probe "$r" fail "Status: Draft"

# 7 — the map changes and the generated registry is not refreshed.
r="$(new_case registry-stale)"
sed -i 's/^    implementation: landed$/    implementation: partial/' "$r/docs/specifications/traceability.yaml"
check registry-stale "$r" fail "REQ.md registry block is stale"

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

# 13 — a plan's Status line moves and the generated plan index is not refreshed.
r="$(new_case plans-stale)"
sed -i 's/^\*\*Status:\*\* Active/**Status:** Done/' "$r/docs/plans/2026-01-01-plan.md"
check plans-stale "$r" fail "plans/README.md plans block is stale"

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

# 18 — a `notes:` memoir in a row.
r="$(new_case notes-key)"
printf '    notes: landed last week\n' >> "$r/docs/specifications/traceability.yaml"
check notes-key "$r" fail "key 'notes:' is not part of the index schema"

# 19 — a duplicated key (a YAML parser would silently keep only the last list).
r="$(new_case duplicate-key)"
printf '    tests:\n      - pkg/good/good_test.go\n' >> "$r/docs/specifications/traceability.yaml"
check duplicate-key "$r" fail "key 'tests:' appears twice"

# 20 — a plan whose Status word is outside the four would silently leave the index.
r="$(new_case plan-status-unknown)"
sed -i 's/^\*\*Status:\*\* Active/**Status:** Superseded/' "$r/docs/plans/2026-01-01-plan.md"
check plan-status-unknown "$r" fail "must start with Active, Draft, Parked or Done"

# 21 — a missing END marker must be refused, not read as "truncate to EOF".
r="$(new_case end-marker-gone)"
sed -i '/END GENERATED: registry/d' "$r/docs/specifications/REQ.md"
check end-marker-gone "$r" fail "no END GENERATED: registry marker"

# 22 — a hand-written registry row outside the generated block.
r="$(new_case stray-registry-row)"
printf '| REQ-999 | Stray | [topic.md](topic.md) | landed |\n' >> "$r/docs/specifications/REQ.md"
check stray-registry-row "$r" fail "registry row(s) outside the generated block"

# 23 — the same REQ id in two rows.
r="$(new_case duplicate-id)"
printf '\n  - id: REQ-001\n    title: Again\n    canonical: docs/specifications/topic.md#req-001--alpha\n    status: stable\n    implementation: planned\n' \
  >> "$r/docs/specifications/traceability.yaml"
check duplicate-id "$r" fail "REQ-001: appears in more than one row"

# 24 — free text in a title may hold " # " and "|" (escaped in the table);
#      CRLF line endings must not disarm any check.
r="$(new_case title-text-crlf)"
sed -i 's/^    title: Alpha$/    title: "Rate # Limit | Alpha"/' "$r/docs/specifications/traceability.yaml"
bash "$r/scripts/spec-gen.sh" >/dev/null
grep -qF '| REQ-001 | Rate # Limit \| Alpha |' "$r/docs/specifications/REQ.md" \
  || { echo "spec-check-selftest: FAIL title-text-crlf: title not escaped in the registry" >&2; fail=1; }
check title-text-crlf "$r" ok
sed -i 's/$/\r/' "$r/docs/specifications/traceability.yaml"
sed -i 's|^      - pkg/good\r$|      - pkg/good  # why\r|' "$r/docs/specifications/traceability.yaml"
check title-text-crlf "$r" fail "comment in a row"

# 25 — a missing BEGIN marker must be refused; without it the splice is a
#      no-op and --check would report the block as current.
r="$(new_case begin-marker-gone)"
sed -i '/BEGIN GENERATED: registry/d' "$r/docs/specifications/REQ.md"
check begin-marker-gone "$r" fail "no BEGIN GENERATED: registry marker"

# 26 — nonexistent path in a block-form tests list (the row also lists
#      packages, so only the tests collector can catch it).
r="$(new_case block-test)"
sed -i 's|^      - pkg/good/good_test.go$|&\n      - pkg/good/absent_test.go|' "$r/docs/specifications/traceability.yaml"
check block-test "$r" fail "missing test path pkg/good/absent_test.go"

# 27 — without the generator the stale-block check cannot run; that must
#      fail, not skip.
r="$(new_case spec-gen-missing)"
rm "$r/scripts/spec-gen.sh"
check spec-gen-missing "$r" fail "missing scripts/spec-gen.sh"

if [[ $fail -ne 0 ]]; then
  echo "spec-check-selftest: FAILED" >&2
  exit 1
fi
echo "spec-check-selftest: OK (27 cases)"
