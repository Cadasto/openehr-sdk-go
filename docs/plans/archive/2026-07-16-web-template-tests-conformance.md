# Plan — Upstream FLAT serialisation conformance harness

**Date:** 2026-07-16 (re-scoped 2026-07-29 — corpus pivot + blocker found, see § Re-scope)
**Status:** Landed — all phases done 2026-08-01 (corpus + pin + tooling; the round-trip harness and its measured skip inventory; PROBE-086 wired, traceable and Implemented; roadmap row, cross-links, archive)
**Owner:** SDK maintainers
**Covers:** [REQ-080](../../specifications/conformance.md#conformance-scope) (openEHR wire conformance — the simplified-format slice this plan advances)
**Verifies:** [REQ-053](../../specifications/wire.md#req-053) (FLAT/STRUCTURED), [REQ-106](../../specifications/clinical-modeling.md#req-106--webtemplate-json-export) (Web Template export) — exercised, not advanced
**Partially satisfies:** [REQ-082](../../specifications/conformance.md#req-082--runnability) — PROBE-086 is **in-repo/Sandbox-only** in v1; the Cassette/Live modes REQ-082 mandates are deferred (see Defers), so this is a documented partial, not full runnability.
**Probes:** **PROBE-086** (upstream FLAT serialisation parity) — **Implemented (Sandbox)** 2026-08-01: harness Phase 1, wiring Phase 2
**Implementation:** landed — Phases 0–3 done. REQ-080 itself stays `partial`: this plan delivers one Sandbox slice of wire conformance, not ratification.
**Depends on:** landed FLAT/STRUCTURED codecs (REQ-053), Web Template export (REQ-106), PROBE-075 structural parity. **Blocked by:** ~~the sibling web-`id` derivation gap~~ — **cleared 2026-07-31** by [REQ-116](../../specifications/clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths) ([its plan](2026-07-29-template-node-naming.md), landed). `webtemplate.Build(conformance-ehrbase.de.v0)` now succeeds: its two ACTION ELEMENTs that both sanitise to `dv_text` take the reference's ordinal fallback (`dv_text`, `dv_text2`) — the spelling the upstream FLAT bodies use — guarded by `TestBuild_FlatConformanceOPTUsesOrdinalFallback`. Phase 1 landed 2026-08-01.
**Defers:** Running the upstream Java suite verbatim (the adapter asserts against vendored fixtures instead); Cassette/Live modes for PROBE-086; Better-platform dialect corpora and STRUCTURED goldens (Better `web-template` `compatibility/`), deferred per the fit-gap review's "Better not a target" recommendation and [ADR 0014](../../adr/0014-webtemplate-reference-implementation-lock.md)

## Goal

Catch FLAT/STRUCTURED/WebTemplate drift against an **upstream-authored** conformance corpus in CI, without requiring Java on the default `make test` path. Closes the P2 gap from the peer-SDK ecosystem fit-gap review and fills the upstream-byte-conformance follow-up that [PROBE-076](../../specifications/conformance.md#probe-076--flat--structured-composition-round-trip) names in its own scope limit: PROBE-076's input is the SDK's *own* FLAT output, so it cannot catch a path the SDK never emits, a suffix it names differently, or a leaf it drops symmetrically. PROBE-086 fixes that by feeding in FLAT this SDK did not write.

## Re-scope (2026-07-29)

Two findings changed this plan's shape. Both are recorded here so the decisions are auditable.

**1. Corpus pivot — `better-care/web-template-tests` cannot serve the goal.** The original plan named that repo. Verified upstream: it was last pushed **2021-03-02**, and its `res/` tree holds OPTs plus canonical compositions in the **legacy Better `@class` dialect wrapped in `//` license headers** (JSONC, which `encoding/json` rejects). It contains **no FLAT, no STRUCTURED, and no WebTemplate** — the `.json` beside each `.opt` is a raw canonical composition, not a Web Template. Vendoring it would add nothing beyond the ~28 OPT + canonical pairs already in `testkit/cassettes/`, and PROBE-086 would assert nothing new.

The corpus that *does* supply upstream simplified output is EHRbase `openEHR_SDK` `test-data/…/composition/flat/simSDT/conformance/` — **34 FLAT bodies over one OPT**, actively maintained, and inside the [ADR 0014](../../adr/0014-webtemplate-reference-implementation-lock.md) EHRbase reference lock. That is what Phase 0 vendored. (Better's `web-template` `compatibility/` tree does carry real STRUCTURED goldens and remains the option if a second dialect is ever wanted — see Defers.)

**2. Two blockers surfaced, one fixed.** Neither was visible before the corpus existed:

- **`templatecompile` rejected the corpus OPT** — `duplicate AQL path .../ism_transition/current_state/defining_code`. Cause: the ACTION constrains `ism_transition` with two `ISM_TRANSITION` alternatives, and the existing alternatives carve-out compared `pathAttr[path] == currentAttr`, which holds for the alternatives' root nodes but fails one level down (each alternative owns its own `defining_code` attribute). **Fixed** — shared-path subtrees are now admitted, scoped to the descent so each node's own path keeps the full cross-attribute collision guard. The same fix made `corona_anamnese` compile, correcting the PROBE-075 note that had attributed its failure to archetype reuse under a slot.
- **`webtemplate.Build` still returns `ErrIDCollision`** — and this one blocked Phases 1–3 *(cleared 2026-07-31 by REQ-116; see **Blocked by** in the header)*. Sibling archetype roots that reuse one archetype id all derive the same web `id`, because this builder takes the id from the *archetype's* concept term while the reference takes it from the **template-level node name** (OPT `name` → fixed `C_STRING`). There is **no suffix rule** — checked across the `corona_anamnese`, `multi_occurrence`, and `AlternativeEvents` goldens, no sibling group carries a numeric suffix; `deviations.md`'s earlier guess to that effect was wrong and has been corrected. *(Corrected 2026-07-31: an ordinal fallback does exist as EHRbase's **last resort** where no pinned name separates siblings — which is why none of those goldens shows one. It is exactly what this corpus OPT needed; see REQ-116 Phase 4.)* The reference also name-predicates `aqlPath` (`[…SECTION.adhoc.v1,'Symptome']` — 350 occurrences in the corona golden, 0 in `constrain_test`, which is why PROBE-075 holds 104/104 without it). Closing it spans four layers — parse + expose the OPT node name (REQ-100), carry it through the compiled tree (REQ-111), emit name predicates (consumed by REQ-102 validation and REQ-053 FLAT paths), prefer it for `id` (REQ-106) — so it is a specified feature in its own right, not a fix folded into this plan.

## Definition of Ready

- [x] `Covers:` separates the advanced REQ (080) from the merely-exercised (053/106) and the partial (082).
- [x] Canonical prose exists for each: REQ-080/082 in `conformance.md`, REQ-053 in `wire.md`, REQ-106 in `clinical-modeling.md`.
- [x] Vendored-fixture-vs-JAR scoping recorded (no irreversible fork, so no ADR gate) — continues ADR 0014's vendored-reference pattern.
- [x] **PROBE-086 defined in `conformance.md` (Draft) before any adapter code** — the "Adding probes" rule.
- [x] Pin policy + layout recorded (`MANIFEST.txt`, `THIRD_PARTY_LICENSES.md`, `testkit/cassettes/README.md`).
- [x] **Blocking:** the name-derived web `id` feature is specified and landed (REQ-116, 2026-07-31).

## Definition of Done

- `make flat-conformance-verify` fails on fixture drift. *(landed — the offline gate `make ci` runs; `make flat-conformance-check` adds the network drift report, see Phase 3 item 1)*
- PROBE-086 runs under `make test` / `make ci` with no Docker/Java. *(landed — harness Phase 1, wrapper Phase 2)*
- `traceability.yaml` maps PROBE-086 to REQ-080 and records the REQ-053/106 coverage + REQ-082 Sandbox-partial. *(landed Phase 2)*
- REQ.md **Impl.** for REQ-080 reflects the advance; residual skips documented in `SKIPPED.md`. *(row flipped to `partial` + this plan registered on it, 2026-07-30; `SKIPPED.md` landed Phase 1)*
- `make spec-check` and `make ci` pass. *(green)*

## Implementation checklist

| Step | Status |
|---|---|
| PROBE-086 defined in `conformance.md` (Draft) | done |
| Fixtures vendored + PIN committed + sync/check tooling | done |
| Blocker documented (`deviations.md`, PROBE-075 note corrected) | done |
| Name-derived web `id` feature specified + landed | done — REQ-116 (Phases 0–5, landed 2026-07-31) |
| Adapter + runner code | done — `testkit/conformance/webtemplate/` (`case.go`, `runner.go`) |
| Tests with `// PROBE-086` comments | done — `runner_test.go` (34 sub-tests + `-census`), `SKIPPED.md` |
| `traceability.yaml` + REQ.md row | done — REQ-080 row `partial`; PROBE-086 mapped with harness package + tests (2026-08-01) |
| `make spec-check` / `make ci` | green |

## Phases

### Phase 0 — Inventory, pin & probe definition — **landed**

- [x] Audited the upstream EHRbase `openEHR_SDK` `test-data/` layout and the `better-care` alternatives; chose the EHRbase FLAT conformance corpus (see § Re-scope).
- [x] [`scripts/sync-flat-conformance.sh`](../../../scripts/sync-flat-conformance.sh) — `sync`/`ingest`/`check`, mirroring `sync-its-rest-specs.sh`: resolves the ref to a concrete commit, fetches at that commit, regenerates `MANIFEST.txt` (per-file `sha256`), removes stale fixtures; `check` does offline integrity plus best-effort upstream-staleness reporting.
- [x] `make flat-conformance-sync` / `make flat-conformance-check`.
- [x] Corpus vendored at commit `30ab1c51308f` — 1 OPT + 34 FLAT bodies; provenance + Apache-2.0 attribution in `THIRD_PARTY_LICENSES.md`, layout in `testkit/cassettes/README.md`.
- [x] `fixtures.FlatConformanceRoot/Opt/Flat` + `ListFlatConformance` resolvers.
- [x] **PROBE-086 catalogued in `conformance.md`** (Draft — blocked), with the blocker named.
- [x] `templatecompile` shared-path-subtree fix + regression tests, including an end-to-end compile test over the vendored OPT.

**Verification:** `make ci` (green), `make flat-conformance-check` (Integrity OK, 35 files).

### Phase 1 — Go adapter (core) — **done**

**Prerequisite:** ~~`webtemplate.Build` succeeds for `conformance-ehrbase.de.v0`~~ — **met 2026-07-31** (REQ-116 Phase 4's ordinal fallback; guarded by `TestBuild_FlatConformanceOPTUsesOrdinalFallback`).

**Tasks:**

1. `testkit/conformance/webtemplate/` — `case.go` (one conformance case: OPT path, upstream FLAT path, expected skips), `runner.go` (compile → build → `UnmarshalFlat(WithTemplate)` → `MarshalFlat` → compare path set + leaf values), `runner_test.go` (table-driven over `fixtures.ListFlatConformance`, each case citing `// PROBE-086`).
2. Do **not** re-assert what PROBE-075/076 already cover — no WebTemplate structural compare (PROBE-075 owns it, 104/104) and no self-round-trip idempotence (PROBE-076 owns it). PROBE-086's distinct assertion is parity against *upstream-authored* FLAT.
3. Record every skip in `testkit/conformance/webtemplate/SKIPPED.md` with its reason.

**Definition of done:** `go test ./testkit/conformance/webtemplate/...` green over the vendored corpus, with skips enumerated. — **done 2026-08-01.**

**Outcome — the corpus is far further from this codec than the plan assumed.** Straight decode of the upstream bodies is **0/34**; the modelled subset is **73 of 1824 keys (4.0%)**, with 1331 keys refused and 420 composition-metadata keys held out as a spelling difference. *(Both figures moved later: refusal scope in review follow-up 1, and the metadata hold-out down to 318 in follow-up 3 — `category` was never a spelling difference.)* That is not a harness defect — it is the measurement this probe exists to take, and PROBE-076 reports 24/25 green over the same codec because its input is the SDK's own output. Full inventory in [`SKIPPED.md`](../../../testkit/conformance/webtemplate/SKIPPED.md).

Two design choices follow from it, both landed:

1. **The skip inventory is generated, not hand-kept.** The runner decodes, records whatever key family the codec refuses (REQ-053 fails loudly, never drops silently), removes it, and retries. Closing a gap shrinks the excluded set automatically — no list to maintain in lockstep. Per-fixture `excluded` / `compared` / `knownGaps` counts are pinned in `runner_test.go` as a ratchet. *(`knownGaps` was removed by review follow-up 2 below — the pins are `excluded` / `compared` only.)*
2. **The comparison is exact on what remains.** Inside the modelled subset a missing key, an extra key, or a changed value fails — never a skip.

**First catch: a silent encode-side data loss.** 32 keys across 30 fixtures (`EVENT.time`, `INSTRUCTION.narrative`, `INSTRUCTION.expiry_time`) decode correctly and are then dropped on re-encode, because `rmpath` resolves no *synthesized in-context* RM attribute and `flat_encode` routes that `ErrPathNotFound` into `skipNotFound` alongside genuinely absent optionals. Structurally invisible to PROBE-076. Diagnosed, counted, pinned; the fix is REQ-121 (rmpath coverage) plus a REQ-053 decision on whether encode should distinguish *unknown attribute* from *absent value* — **new work, not in this plan**.

### Phase 2 — PROBE-086 wiring & traceability — **done**

1. ~~`probe_086_upstream_flat_parity.go` wrapper~~ — **done**, beside PROBE-076; thin over the shared runner, invoked by `TestProbe086` (34/34 pass).
2. ~~`traceability.yaml`~~ — **done**: PROBE-086 under REQ-080 with the harness package and both test files; the modelled-subset scope (4.9% after review follow-up 1 below; **10.5%** after follow-up 3) and the REQ-082 Sandbox-partial stated in the entry.
3. ~~Flip `Status:` Draft → Implemented + CHANGELOG~~ — **done**. `spec-check` passing is the proof of consistency: REQ-080 is `partial` and now cites PROBE-086, which the Draft-probe guard rejects unless the status actually flipped.

### Phase 3 — CI gate & documentation — **done**

1. ~~Add the corpus integrity check to `make ci`~~ — **done in Phase 0** (review follow-up): `make flat-conformance-verify` (offline `sha256` only, no network or `curl`/`jq`) runs in `make ci` and as a `ci.yml` step. The network-touching `flat-conformance-check` stays a dev helper, matching the `its-rest-check` convention.
2. ~~Document in [`docs/ci.md`](../../ci.md); add the AGENTS.md tooling-table row when the probe itself lands~~ — **done**, both.
3. ~~`roadmap.md`: REQ-080 `planned → partial`~~ — **done**: the conformance-ratification row now carries the landed Sandbox slice and states what ratification still needs.
4. ~~Cross-link PROBE-086 from the peer-SDK ecosystem notes; archive this plan~~ — **done**.

### Review follow-up (PR #85)

Three findings from review over two rounds, all real, all about the harness understating or excusing what it measures:

1. **Refusal scope.** The first cut dropped a whole leaf family *plus its subtree* for every refusal, whatever the refusal actually said. One unmodelled `|precision` therefore withdrew the `|magnitude` and `|unit` beside it, and one `any_event:0|sample_count` withdrew an entire EVENT subtree — leaving `interval_event` comparing **zero** keys while sitting in the suite looking green. `dropRefused` now reads the scope off the error: one suffix, one leaf, or a subtree. Coverage 73 → **90 keys (4.0% → 4.9%)** with no new divergence, i.e. those keys were round-tripping correctly all along and simply were not being checked. *(Later corrected again to **192 keys (10.5%)** in follow-up 3 below, when the `category` hold-out turned out to be a measurement error — `category` round-trips identically on both sides.)*
2. **A fail-open tolerated-drop bucket.** `knownEncodeGaps` still matched `/time`, `/narrative`, `/expiry_time` after Phase 1's rmpath fix emptied it, and `Report.Clean()` ignored the bucket — so a regression in exactly the area this probe was built to watch would have landed there and *passed*. Both the list and `Report.KnownGaps` are gone: a key that decodes and then does not re-encode is a failure. Verified by regressing `rmpath` on purpose.
3. **A hold-out that was not a respelling, hiding a real drop (2026-08-03).** The metadata allow-list carried `category` as composition metadata. It is nothing of the kind — a template-constrained Web Template leaf on its own FLAT path, spelled identically by upstream and by this codec, whose 102 corpus keys (34 fixtures × 3) were already round-tripping byte-for-byte. Removing it moved coverage **90 → 192 keys (4.9% → 10.5%)**: a measurement correction, not a closed codec gap. `context/setting` stays held out but is now recorded as what it is — a **waiver** of a real encode-side drop (`ctx/setting` emission is deferred and the real-path spelling is not emitted either), not a spelling difference. In the same round `rmpath` gained the remaining `EVENT_CONTEXT` attributes (`other_context`, `health_care_facility`, `location`, `participations`), `ACTIVITY.timing`, and `ACTION.time`, so template-constrained `other_context` content genuinely round-trips via its Web Template paths and a populated `timing` rides `|raw` instead of vanishing; `EVENT_CONTEXT.start_time` / `setting` stay deliberately unresolved (double-spelling against `ctx/time`; zero-value emission for `setting`). The hazard model that had excused them — "resolving an unserialisable leaf would hard-fail encode" — was wrong and is corrected in `SKIPPED.md`: the encoder resolves first and leaf-maps second, and `leafToFlat` skips a non-`DV_` datatype silently while writing an unmapped `DV_*` as `|raw`. Finally, ownership of the ratchet is now explicit — the per-fixture `excluded`/`compared` pins are the harness's own tests, and `Probe086UpstreamFlatParity` contributes a coverage **floor** (`Compared == 0` fails) instead of the ratchet.

Also: the `MUST`s in [PROBE-086](../../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) now state the modelled-subset scoping, the per-refusal drop rule, and the no-tolerated-drop rule, instead of reading as whole-body parity against a deviations list the harness never had; `incontext_rmpath_test.go` is mapped under REQ-121; and the harness's key bookkeeping has unit tests (`helpers_test.go`) alongside the corpus run.

**Declined:** replacing the harness's error-string scraping with a typed key carried on the codec's errors. It would mean a new exported type in the released `openehr/serialize/simplified` surface for a test harness's convenience, and the scraping is already fail-loud by construction — `decodeReducing` checks the scraped key against the body and errors out when it does not match (which is how the PR #84 decode regression surfaced), while a reworded suffix message would collapse the `compared` counts and break the pins.

## Skipped / deferred cases (initial)

Expected entries for `SKIPPED.md`, to be confirmed once the runner executes:

| Upstream construct | Reason |
|---|---|
| Java-specific builder tests | No generated entity layer in Go (not in this corpus, listed for completeness) |
| `ctx/` fields absent from the encoder | Known REQ-053 deviation — `openehr/serialize/simplified/deviations.md` |
| Exotic null-flavour / feeder-audit combinations | Land as the REQ-053 deviations close |

## Mapping to specs

- [conformance.md § REQ-080](../../specifications/conformance.md#conformance-scope) — the requirement this plan advances (registry row: [REQ.md](../../specifications/REQ.md))
- [conformance.md § PROBE-086](../../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) — the probe definition
- [conformance.md § REQ-082](../../specifications/conformance.md#req-082--runnability) — runnability (Sandbox partial)
- [wire.md § REQ-053](../../specifications/wire.md#req-053) — FLAT/STRUCTURED codec (exercised)
- [clinical-modeling.md § REQ-106](../../specifications/clinical-modeling.md#req-106--webtemplate-json-export) — Web Template export (exercised); the blocking gap is in its `deviations.md`
- [ADR 0014](../../adr/0014-webtemplate-reference-implementation-lock.md) — vendored-fixture reference lock

## References

- Corpus: EHRbase `openEHR_SDK` `test-data/src/main/resources/composition/flat/simSDT/conformance/` — pin in [`MANIFEST.txt`](../../../testkit/cassettes/flat-conformance/MANIFEST.txt).
- Reference WebTemplate goldens (14 upstream, incl. `corona_anamnese`, `multi_occurrence`, `AlternativeEvents`) — the oracles for the blocking feature.
- Cadasto: `openehr/serialize/simplified/roundtrip_test.go`, PROBE-075, PROBE-076.
- Motivation: peer-SDK ecosystem fit-gap review, § Wire `web-template-tests`.
