# Plan — Upstream FLAT serialisation conformance harness

**Date:** 2026-07-16 (re-scoped 2026-07-29 — corpus pivot + blocker found, see § Re-scope)
**Status:** Partial — Phase 0 landed (corpus, pin, tooling, PROBE-086 catalogued Draft); Phases 1–3 blocked
**Owner:** SDK maintainers
**Covers:** [REQ-080](../specifications/conformance.md#conformance-scope) (openEHR wire conformance — the simplified-format slice this plan advances)
**Verifies:** [REQ-053](../specifications/wire.md#req-053) (FLAT/STRUCTURED), [REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) (Web Template export) — exercised, not advanced
**Partially satisfies:** [REQ-082](../specifications/conformance.md#req-082--runnability) — PROBE-086 is **in-repo/Sandbox-only** in v1; the Cassette/Live modes REQ-082 mandates are deferred (see Defers), so this is a documented partial, not full runnability.
**Probes:** **PROBE-086** (upstream FLAT serialisation parity) — catalogued Draft/blocked
**Implementation:** partial — Phase 0 landed; Phase 1 blocked on the name-derived web `id` gap
**Depends on:** landed FLAT/STRUCTURED codecs (REQ-053), Web Template export (REQ-106), PROBE-075 structural parity. **Blocked by:** the sibling web-`id` derivation gap in [`deviations.md`](../../openehr/template/webtemplate/deviations.md) § Sibling `id` disambiguation — to be specified and landed separately (REQ-100/111/106).
**Defers:** Running the upstream Java suite verbatim (the adapter asserts against vendored fixtures instead); Cassette/Live modes for PROBE-086; Better-platform dialect corpora and STRUCTURED goldens (Better `web-template` `compatibility/`), deferred per the fit-gap review's "Better not a target" recommendation and [ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md)

## Goal

Catch FLAT/STRUCTURED/WebTemplate drift against an **upstream-authored** conformance corpus in CI, without requiring Java on the default `make test` path. Closes the P2 gap from the peer-SDK ecosystem fit-gap review and fills the upstream-byte-conformance follow-up that [PROBE-076](../specifications/conformance.md#probe-076--flat--structured-composition-round-trip) names in its own scope limit: PROBE-076's input is the SDK's *own* FLAT output, so it cannot catch a path the SDK never emits, a suffix it names differently, or a leaf it drops symmetrically. PROBE-086 fixes that by feeding in FLAT this SDK did not write.

## Re-scope (2026-07-29)

Two findings changed this plan's shape. Both are recorded here so the decisions are auditable.

**1. Corpus pivot — `better-care/web-template-tests` cannot serve the goal.** The original plan named that repo. Verified upstream: it was last pushed **2021-03-02**, and its `res/` tree holds OPTs plus canonical compositions in the **legacy Better `@class` dialect wrapped in `//` license headers** (JSONC, which `encoding/json` rejects). It contains **no FLAT, no STRUCTURED, and no WebTemplate** — the `.json` beside each `.opt` is a raw canonical composition, not a Web Template. Vendoring it would add nothing beyond the ~28 OPT + canonical pairs already in `testkit/cassettes/`, and PROBE-086 would assert nothing new.

The corpus that *does* supply upstream simplified output is EHRbase `openEHR_SDK` `test-data/…/composition/flat/simSDT/conformance/` — **34 FLAT bodies over one OPT**, actively maintained, and inside the [ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md) EHRbase reference lock. That is what Phase 0 vendored. (Better's `web-template` `compatibility/` tree does carry real STRUCTURED goldens and remains the option if a second dialect is ever wanted — see Defers.)

**2. Two blockers surfaced, one fixed.** Neither was visible before the corpus existed:

- **`templatecompile` rejected the corpus OPT** — `duplicate AQL path .../ism_transition/current_state/defining_code`. Cause: the ACTION constrains `ism_transition` with two `ISM_TRANSITION` alternatives, and the existing alternatives carve-out compared `pathAttr[path] == currentAttr`, which holds for the alternatives' root nodes but fails one level down (each alternative owns its own `defining_code` attribute). **Fixed** — shared-path subtrees are now admitted, scoped to the descent so each node's own path keeps the full cross-attribute collision guard. The same fix made `corona_anamnese` compile, correcting the PROBE-075 note that had attributed its failure to archetype reuse under a slot.
- **`webtemplate.Build` still returns `ErrIDCollision`** — and this one blocks Phases 1–3. Sibling archetype roots that reuse one archetype id all derive the same web `id`, because this builder takes the id from the *archetype's* concept term while the reference takes it from the **template-level node name** (OPT `name` → fixed `C_STRING`). There is **no suffix rule** — checked across the `corona_anamnese`, `multi_occurrence`, and `AlternativeEvents` goldens, no sibling group carries a numeric suffix; `deviations.md`'s earlier guess to that effect was wrong and has been corrected. The reference also name-predicates `aqlPath` (`[…SECTION.adhoc.v1,'Symptome']` — 350 occurrences in the corona golden, 0 in `constrain_test`, which is why PROBE-075 holds 104/104 without it). Closing it spans four layers — parse + expose the OPT node name (REQ-100), carry it through the compiled tree (REQ-111), emit name predicates (consumed by REQ-102 validation and REQ-053 FLAT paths), prefer it for `id` (REQ-106) — so it is a specified feature in its own right, not a fix folded into this plan.

## Definition of Ready

- [x] `Covers:` separates the advanced REQ (080) from the merely-exercised (053/106) and the partial (082).
- [x] Canonical prose exists for each: REQ-080/082 in `conformance.md`, REQ-053 in `wire.md`, REQ-106 in `clinical-modeling.md`.
- [x] Vendored-fixture-vs-JAR scoping recorded (no irreversible fork, so no ADR gate) — continues ADR 0014's vendored-reference pattern.
- [x] **PROBE-086 defined in `conformance.md` (Draft) before any adapter code** — the "Adding probes" rule.
- [x] Pin policy + layout recorded (`MANIFEST.txt`, `THIRD_PARTY_LICENSES.md`, `testkit/cassettes/README.md`).
- [ ] **Blocking:** the name-derived web `id` feature is specified and landed.

## Definition of Done

- `make flat-conformance-check` fails on fixture drift. *(landed)*
- PROBE-086 runs under `make test` / `make ci` with no Docker/Java. *(blocked)*
- `traceability.yaml` maps PROBE-086 to REQ-080 and records the REQ-053/106 coverage + REQ-082 Sandbox-partial. *(pending Phase 2)*
- REQ.md **Impl.** for REQ-080 reflects the advance; residual skips documented in `SKIPPED.md`. *(row flipped to `partial` + this plan registered on it, 2026-07-30; residual-skips doc pending Phase 3)*
- `make spec-check` and `make ci` pass. *(green)*

## Implementation checklist

| Step | Status |
|---|---|
| PROBE-086 defined in `conformance.md` (Draft) | done |
| Fixtures vendored + PIN committed + sync/check tooling | done |
| Blocker documented (`deviations.md`, PROBE-075 note corrected) | done |
| Name-derived web `id` feature specified + landed | **blocked — separate REQ** |
| Adapter + runner code | not started |
| Tests with `// PROBE-086` comments | not started |
| `traceability.yaml` + REQ.md row | partial — REQ-080 row `partial`, corpus + plan registered (2026-07-30); PROBE-086 mapping pending Phase 2 |
| `make spec-check` / `make ci` | green |

## Phases

### Phase 0 — Inventory, pin & probe definition — **landed**

- [x] Audited the upstream EHRbase `openEHR_SDK` `test-data/` layout and the `better-care` alternatives; chose the EHRbase FLAT conformance corpus (see § Re-scope).
- [x] [`scripts/sync-flat-conformance.sh`](../../scripts/sync-flat-conformance.sh) — `sync`/`ingest`/`check`, mirroring `sync-its-rest-specs.sh`: resolves the ref to a concrete commit, fetches at that commit, regenerates `MANIFEST.txt` (per-file `sha256`), removes stale fixtures; `check` does offline integrity plus best-effort upstream-staleness reporting.
- [x] `make flat-conformance-sync` / `make flat-conformance-check`.
- [x] Corpus vendored at commit `30ab1c51308f` — 1 OPT + 34 FLAT bodies; provenance + Apache-2.0 attribution in `THIRD_PARTY_LICENSES.md`, layout in `testkit/cassettes/README.md`.
- [x] `fixtures.FlatConformanceRoot/Opt/Flat` + `ListFlatConformance` resolvers.
- [x] **PROBE-086 catalogued in `conformance.md`** (Draft — blocked), with the blocker named.
- [x] `templatecompile` shared-path-subtree fix + regression tests, including an end-to-end compile test over the vendored OPT.

**Verification:** `make ci` (green), `make flat-conformance-check` (Integrity OK, 35 files).

### Phase 1 — Go adapter (core) — blocked

**Prerequisite:** `webtemplate.Build` succeeds for `conformance-ehrbase.de.v0`.

**Tasks:**

1. `testkit/conformance/webtemplate/` — `case.go` (one conformance case: OPT path, upstream FLAT path, expected skips), `runner.go` (compile → build → `UnmarshalFlat(WithTemplate)` → `MarshalFlat` → compare path set + leaf values), `runner_test.go` (table-driven over `fixtures.ListFlatConformance`, each case citing `// PROBE-086`).
2. Do **not** re-assert what PROBE-075/076 already cover — no WebTemplate structural compare (PROBE-075 owns it, 104/104) and no self-round-trip idempotence (PROBE-076 owns it). PROBE-086's distinct assertion is parity against *upstream-authored* FLAT.
3. Record every skip in `testkit/conformance/webtemplate/SKIPPED.md` with its reason.

**Definition of done:** `go test ./testkit/conformance/webtemplate/...` green over the vendored corpus, with skips enumerated.

### Phase 2 — PROBE-086 wiring & traceability — blocked

1. `testkit/probes/serialize/probe_086_upstream_flat_parity.go` — thin wrapper over the shared runner (explicit probe file for traceability), beside PROBE-076.
2. `traceability.yaml`: add PROBE-086 under REQ-080; note the REQ-053/106 coverage and the REQ-082 Sandbox-partial.
3. Flip PROBE-086 `Status:` Draft → Implemented in `conformance.md`; CHANGELOG entry for the transition.

### Phase 3 — CI gate & documentation — blocked

1. ~~Add the corpus integrity check to `make ci`~~ — **done in Phase 0** (review follow-up): `make flat-conformance-verify` (offline `sha256` only, no network or `curl`/`jq`) runs in `make ci` and as a `ci.yml` step. The network-touching `flat-conformance-check` stays a dev helper, matching the `its-rest-check` convention.
2. Document in [`docs/ci.md`](../ci.md) — **done**; add the AGENTS.md tooling-table row when the probe itself lands.
3. `roadmap.md`: REQ-080 `planned → partial` (Sandbox slice landed).
4. Cross-link PROBE-086 from the peer-SDK ecosystem notes; archive this plan.

## Skipped / deferred cases (initial)

Expected entries for `SKIPPED.md`, to be confirmed once the runner executes:

| Upstream construct | Reason |
|---|---|
| Java-specific builder tests | No generated entity layer in Go (not in this corpus, listed for completeness) |
| `ctx/` fields absent from the encoder | Known REQ-053 deviation — `openehr/serialize/simplified/deviations.md` |
| Exotic null-flavour / feeder-audit combinations | Land as the REQ-053 deviations close |

## Mapping to specs

- [conformance.md § REQ-080](../specifications/conformance.md#conformance-scope) — the requirement this plan advances (registry row: [REQ.md](../specifications/REQ.md))
- [conformance.md § PROBE-086](../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) — the probe definition
- [conformance.md § REQ-082](../specifications/conformance.md#req-082--runnability) — runnability (Sandbox partial)
- [wire.md § REQ-053](../specifications/wire.md#req-053) — FLAT/STRUCTURED codec (exercised)
- [clinical-modeling.md § REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) — Web Template export (exercised); the blocking gap is in its `deviations.md`
- [ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md) — vendored-fixture reference lock

## References

- Corpus: EHRbase `openEHR_SDK` `test-data/src/main/resources/composition/flat/simSDT/conformance/` — pin in [`MANIFEST.txt`](../../testkit/cassettes/flat-conformance/MANIFEST.txt).
- Reference WebTemplate goldens (14 upstream, incl. `corona_anamnese`, `multi_occurrence`, `AlternativeEvents`) — the oracles for the blocking feature.
- Cadasto: `openehr/serialize/simplified/roundtrip_test.go`, PROBE-075, PROBE-076.
- Motivation: peer-SDK ecosystem fit-gap review, § Wire `web-template-tests`.
