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
# spec-gen lists test files with git; each fixture is its own repository, so
# a GIT_DIR inherited from a hook must not point it at this one.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
CHECK="${HERE}/spec-check.sh"
GEN="${HERE}/spec-gen.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
fail=0

# Minimal tree that passes spec-check: one landed REQ with block-form
# packages / probes / tests, a registry row, a four-probe catalog
# (one Draft) whose census/all-three sentences are true, two runnable
# example programs matching the tally in examples.md.
# The fixture is a git repository holding one test file that cites the REQ.
# The generated parts are produced by the real spec-gen.sh. The checker
# resolves ROOT from its own location, so copying it into <root>/scripts/
# points it at the fixture.
build_baseline() {
  local root="$1"
  mkdir -p "$root/scripts" "$root/docs/specifications" "$root/pkg/good" \
    "$root/cmd/examples/alpha" "$root/cmd/examples/beta" "$root/cmd/examples/scaffold"
  cp "$CHECK" "$root/scripts/spec-check.sh"
  cp "$GEN" "$root/scripts/spec-gen.sh"
  git -C "$root" init -q
  printf 'package good\n\n// REQ-001: fixture citation.\n' > "$root/pkg/good/good_test.go"
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

# 3 - the retired `plans:` and `adrs:` keys are refused: an ADR names its REQs
#     itself and plans are outside the chain, so the map lists neither.
r="$(new_case retired-plans)"
printf '    plans:\n      - docs/plans/2026-01-01-plan.md\n' >> "$r/docs/specifications/traceability.yaml"
check retired-plans "$r" fail "key 'plans:' is not part of the index schema"
r="$(new_case retired-adrs)"
printf '    adrs: [docs/adr/0001-x.md]\n' >> "$r/docs/specifications/traceability.yaml"
check retired-adrs "$r" fail "key 'adrs:' is not part of the index schema"

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

# 28 - a new test cites the REQ and the map is not regenerated: the tests list
#      is stale. Regenerating it makes the gate green again.
r="$(new_case tests-stale)"
printf 'package good\n\n// REQ-001: a second citing test.\n' > "$r/pkg/good/extra_test.go"
check tests-stale "$r" fail "REQ-001 tests list is stale: pkg/good/extra_test.go cites REQ-001 but is not listed"
bash "$r/scripts/spec-gen.sh" >/dev/null
grep -qxF '      - pkg/good/extra_test.go' "$r/docs/specifications/traceability.yaml" \
  || { echo "spec-check-selftest: FAIL tests-stale: spec-gen did not list pkg/good/extra_test.go" >&2; fail=1; }
check tests-regenerated "$r" ok

# 29 - a listed test stops citing the REQ: the list is stale the other way.
r="$(new_case tests-uncited)"
printf 'package good\n' > "$r/pkg/good/good_test.go"
check tests-uncited "$r" fail "pkg/good/good_test.go is listed but does not cite REQ-001"

# 30 - only test files count. A citation in a non-test file, a git-ignored
#      test, or a longer token (REQ-001x, REQ-001_x, XREQ-001) is not picked
#      up; a probe implementation under testkit/probes/ is, and so is a token
#      bounded by a hyphen or a dot (pre-REQ-001.).
r="$(new_case tests-scope)"
mkdir -p "$r/testkit/probes/alpha"
printf 'package good\n\n// REQ-001: not a test file.\n' > "$r/pkg/good/good.go"
printf 'ignored_test.go\n' > "$r/.gitignore"
printf 'package good\n\n// REQ-001: ignored.\n' > "$r/pkg/good/ignored_test.go"
printf 'package good\n\n// REQ-001x, REQ-001_x and XREQ-001 are other tokens.\n' > "$r/pkg/good/boundary_test.go"
printf 'package good\n\n// A pre-REQ-001. mention counts.\n' > "$r/pkg/good/hyphen_test.go"
printf 'package alpha\n\n// REQ-001: a probe implementation.\n' > "$r/testkit/probes/alpha/probe_001.go"
bash "$r/scripts/spec-gen.sh" >/dev/null
_got="$(awk '/^    tests:/{f=1;next} f&&/^      - /{print $2;next} {f=0}' "$r/docs/specifications/traceability.yaml" | paste -sd' ' -)"
[[ "$_got" == "pkg/good/good_test.go pkg/good/hyphen_test.go testkit/probes/alpha/probe_001.go" ]] \
  || { echo "spec-check-selftest: FAIL tests-scope: generated tests list is '${_got}'" >&2; fail=1; }
check tests-scope "$r" ok

# 31 - a landed row with no packages is satisfied by its generated tests
#      list, and fails once no test cites it.
r="$(new_case tests-only-evidence)"
sed -i '/^    packages:$/,/^      - pkg\/good$/d' "$r/docs/specifications/traceability.yaml"
check tests-only-evidence "$r" ok
printf 'package good\n' > "$r/pkg/good/good_test.go"
bash "$r/scripts/spec-gen.sh" >/dev/null
check tests-only-evidence-gone "$r" fail "REQ-001 (landed): no packages or tests listed"

# 32 - the right paths in inline form: the only difference is the layout,
#      and the diagnostic says so.
r="$(new_case tests-inline)"
sed -i '/^    tests:$/,/^      - pkg\/good\/good_test.go$/c\    tests: [pkg/good/good_test.go]' "$r/docs/specifications/traceability.yaml"
check tests-inline "$r" fail "REQ-001 tests list is not in block form"

# 33 - an empty tests: key is stale, and regenerating removes it.
r="$(new_case tests-empty-key)"
sed -i '/^      - pkg\/good\/good_test.go$/d' "$r/docs/specifications/traceability.yaml"
printf 'package good\n' > "$r/pkg/good/good_test.go"
check tests-empty-key "$r" fail "REQ-001 has an empty tests: key"
bash "$r/scripts/spec-gen.sh" >/dev/null
! grep -q '^    tests:' "$r/docs/specifications/traceability.yaml" \
  || { echo "spec-check-selftest: FAIL tests-empty-key: spec-gen kept an empty tests: key" >&2; fail=1; }
check tests-emptied "$r" ok

# 34 - the last citing test stops citing: regenerating removes the whole key.
r="$(new_case tests-list-emptied)"
printf 'package good\n' > "$r/pkg/good/good_test.go"
bash "$r/scripts/spec-gen.sh" >/dev/null
! grep -q '^    tests:' "$r/docs/specifications/traceability.yaml" \
  || { echo "spec-check-selftest: FAIL tests-list-emptied: spec-gen left a tests: key with no citing test" >&2; fail=1; }
check tests-list-emptied "$r" ok

# 35 - a row without a tests: key gains one, after its last line, when a test
#      starts citing its REQ.
r="$(new_case tests-gained)"
printf '\n### REQ-002 Beta\n\nNormative fixture text.\n' >> "$r/docs/specifications/topic.md"
printf '\n  - id: REQ-002\n    title: Beta\n    canonical: docs/specifications/topic.md#req-002-beta\n    status: stable\n    implementation: planned\n' \
  >> "$r/docs/specifications/traceability.yaml"
bash "$r/scripts/spec-gen.sh" >/dev/null
check tests-gained-before "$r" ok
printf 'package good\n\n// REQ-002: a first citing test.\n' > "$r/pkg/good/beta_test.go"
check tests-gained-stale "$r" fail "pkg/good/beta_test.go cites REQ-002 but is not listed"
bash "$r/scripts/spec-gen.sh" >/dev/null
_row="$(awk '/^  - id: REQ-002$/{f=1} f' "$r/docs/specifications/traceability.yaml" | tail -2 | paste -sd'|' -)"
[[ "$_row" == "    tests:|      - pkg/good/beta_test.go" ]] \
  || { echo "spec-check-selftest: FAIL tests-gained: REQ-002 row ends '${_row}'" >&2; fail=1; }
check tests-gained "$r" ok

# 36 - a withdrawn requirement is `retired`, which needs `status: deprecated`;
#      the old word `deprecated` in implementation: is out of vocabulary.
r="$(new_case impl-retired)"
sed -i 's/^    status: stable$/    status: deprecated/; s/^    implementation: landed$/    implementation: retired/' \
  "$r/docs/specifications/traceability.yaml"
bash "$r/scripts/spec-gen.sh" >/dev/null
check impl-retired "$r" ok
r="$(new_case impl-retired-stable)"
sed -i 's/^    implementation: landed$/    implementation: retired/' "$r/docs/specifications/traceability.yaml"
check impl-retired-stable "$r" fail "implementation retired needs status: deprecated"
r="$(new_case impl-deprecated)"
sed -i 's/^    implementation: landed$/    implementation: deprecated/' "$r/docs/specifications/traceability.yaml"
check impl-deprecated "$r" fail "invalid implementation 'deprecated'"

if [[ $fail -ne 0 ]]; then
  echo "spec-check-selftest: FAILED" >&2
  exit 1
fi
echo "spec-check-selftest: OK (37 cases)"
