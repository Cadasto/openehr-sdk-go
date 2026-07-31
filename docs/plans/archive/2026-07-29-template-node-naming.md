# Plan — Template-level node naming and name-predicated paths

**Date:** 2026-07-29
**Status:** Done (2026-07-31) — all five phases executed; REQ-116 `landed`. Archived via `sdd-archive` in the implementing PR (#84).
**Owner:** SDK maintainers
**Covers:** **[REQ-116](../../specifications/clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths)** (template-level node naming and name-predicated paths) — canonical prose landed, registry row `landed`
**Amends:** [REQ-100](../../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) (parse surface), [REQ-111](../../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge) (compiled carry), [REQ-106](../../specifications/clinical-modeling.md#req-106--webtemplate-json-export) (`id` source — prose already amended to defer to REQ-116)
**Must not regress:** [REQ-102](../../specifications/clinical-modeling.md#req-102--composition-validation), [REQ-107](../../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator), [REQ-053](../../specifications/wire.md#req-053) — all consume compiled path shape
**Probes:** [PROBE-075](../../specifications/conformance.md#probe-075--webtemplate-structural-parity) (extended to new fixtures), [PROBE-086](../../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) (unblocked by this plan)
**Implementation:** landed
**Depends on:** the shared-path-subtree compile fix and the vendored FLAT corpus (PR #79, landed)
**Defers:** Better-platform dialect naming; multi-language name selection beyond the document default language; retro-fitting name predicates onto AQL *builder* output (REQ-055) — this plan changes compiled-template paths only

## Goal

Close the gap that blocks PROBE-086 and any WebTemplate for a template that reuses an archetype among siblings: the SDK reads a node's display name from the *archetype's* concept term, where the reference reads the **template-level node name** pinned in the OPT. Consequences reach four layers — OPT parse, the compiled tree, AQL path predicates, and WebTemplate `id` derivation. Consumers are `webtemplate` (distinct sibling ids), the FLAT/STRUCTURED codecs and validators (addressable sibling paths), and PROBE-086.

**Architecture.** No new packages. The name is parsed in `openehr/template`, carried on `CompiledNode`, consulted by the path builder in `internal/templatecompile`, and preferred by `idOf` in `openehr/template/webtemplate`. Public-API additions only (one accessor per layer); no signature changes.

**Evidence base** (established 2026-07-29, recorded in [`deviations.md`](../../../openehr/template/webtemplate/deviations.md) § Sibling `id` disambiguation): there is **no** numeric-suffix rule — verified across the upstream `corona_anamnese`, `multi_occurrence`, and `AlternativeEvents` WebTemplate goldens. The reference disambiguates by name alone and name-predicates `aqlPath` **350×** in the corona golden versus **0×** in `constrain_test`.

**Correction (Phase 4).** A numeric suffix *does* exist — as EHRbase's **last resort**, not its disambiguator. The claim above holds for every name-disambiguated golden, which is why no suffix appears there: each sibling pins a distinct name, so the fallback is never reached. Where no name separates siblings it is: the vendored upstream FLAT corpus keys `conformance-ehrbase.de.v0`'s two ACTION ELEMENTs — both sanitising to `dv_text`, neither pinning a name — as `…/conformance_action/dv_text` and `…/conformance_action/dv_text2`. Phase 4 implements the ordinal for exactly that case, which is what lets the FLAT-conformance OPT build.

**Correction (2026-07-30).** The predicate trigger is the **pinned name alone**, not sibling collision. Measured across the vendored goldens: every name in a golden predicate is a fixed `C_STRING` in its OPT (1:1 for both oracles), `constrain_test` pins none and carries 0 predicates over 104 nodes — *that* is why PROBE-075 holds 104/104 — and `GECCO_Diagnose` predicates all three `/content` children whose archetype ids are **distinct**, plus a sole `CLUSTER` child. A collision-conditioned rule emits too few predicates and would fail GECCO parity. **Blast radius is therefore wider than "the reuse templates":** counted across all 58 vendored OPTs, **9** pin at least one node name — the 2 oracles (`Corona_Anamnese` 24, `GECCO_Diagnose` 6, each exactly matching its golden's distinct predicate-name count) plus **7** others: `test_template_rename_node{,_2}` (8 each), `clinical_content_validation` (4), `IDCR -  Adverse Reaction List.v1` (4), `IDCR - Laboratory Test Report.v0`, `Test_dv_text_list_constraint.v0`, `Test_dv_text_open_constraint.v0` (1 each). Their compiled paths change too — Phase 3's regression guard must be scoped by *pins-a-name*, not by *has-reused-siblings*.

**Measured residual** (2026-07-29 forecast, all resolved or documented by Phase 4): the GECCO path set already matched exactly (`missing=0`) once predicates were normalised away, so predicate emission closed the whole path gap. The 4 extra `…/name` `DV_TEXT` leaves are gone (Phase 4 task 2). The 14 `min`/`max` deltas — where the **GECCO golden is the outlier** (`1/1` vs `0/1`; `constrain_test` and Corona both agree with this builder, and GECCO's OPT constrains no `existence`) — are a documented fixture deviation with the count pinned, as forecast. The 1 input-count delta is real and now pinned alongside Corona's 20, as a pre-REQ-116 DV_TEXT value-list gap.

## Definition of Ready

- [x] Canonical REQ-116 prose exists, with acceptance criteria.
- [x] REQ.md row + `traceability.yaml` entry landed.
- [x] Mechanism verified against upstream goldens (not guessed).
- [x] No irreversible fork: [ADR 0014](../../adr/0014-webtemplate-reference-implementation-lock.md) already locks the SDK to the EHRbase reference, and this plan *converges* on it — so no new ADR is needed. (If reference behaviour turns out to be unmatchable on some construct, that exception needs an ADR amendment, not a silent deviation.)
- [x] Reference goldens vendored for the fixtures under test (Phase 0 — done).

## Definition of Done

- [x] REQ-116 acceptance criteria all demonstrated by tests citing `// REQ-116` — all five, across `compile_nodename_test.go`, `webtemplate_oracle_compile_test.go`, `pathsnapshot_test.go`, `req116_gap_test.go`, `build_test.go`.
- [x] `Corona_Anamnese` and `GECCO_Diagnose` build WebTemplates matching their goldens on `id` and `aqlPath` — **exactly** (230/230, 34/34), residuals documented and count-pinned. `conformance-ehrbase.de.v0` builds (ordinal fallback), guarded by `TestBuild_FlatConformanceOPTUsesOrdinalFallback`.
- [x] PROBE-075 parity extended to **both** oracles, node facts and inputs; `constrain_test` still 104/104 exact.
- [x] **Predicate-free paths byte-identical to pre-change output** — asserted by `pathsnapshot_test.go`; `no-name.txt` came through the emission change unchanged.
- [x] REQ.md `Impl.` → `landed`; `traceability.yaml` tests/packages filled; `deviations.md` sibling-`id` entry marked RESOLVED and three new findings recorded.
- [x] `make spec-check` + `make ci` green; PROBE-086 unblocked — [its plan](../2026-07-16-web-template-tests-conformance.md) Phase 1 can start.

## Phases

### Phase 0 — Vendor the oracles — **done**

**Tasks:**

1. ~~Vendor `corona_anamnese`~~ — **done**: `Corona_Anamnese.opt` + `Corona_Anamnese.webtemplate.json` vendored at the `constrain_test` pin (`22b01e0c`), stems per `template_id` convention; provenance in `THIRD_PARTY_LICENSES.md`. **`multi_occurrence` was dropped**: its golden carries **zero** name-predicated `aqlPath`s (as does `AlternativeEvents`), so the "second name-predicated case" premise was wrong. Substituted **`GECCO_Diagnose`** (30 predicate segments over 24 paths, space-free `template_id`, ~⅕ corona's size) — and it turned out to cover the gap's *other* failure mode: it **builds without error** today (no sibling id collision) while silently diverging from its golden on `aqlPath`, where corona fails loudly with `ErrIDCollision`. Both modes are pinned by tests: compile guards in `internal/templatecompile/webtemplate_oracle_compile_test.go`, build outcomes in `openehr/template/webtemplate/req116_gap_test.go`.
2. ~~Note the size cost in the cassettes README~~ — **done** (corona OPT 1.2 MB + golden 230 KB; GECCO 210 KB + 73 KB), plus `fixtures.WebTemplateOpt` / `WebTemplateReference` resolvers.

**DoD:** met — fixtures resolve via `testkit/fixtures`; `make ci` green (no behaviour change).

### Phase 1 — Parse and expose the name (REQ-100) — **done**

**Tasks:**

1. ~~Parse + expose~~ — **done**: `deriveNodeName` in `openehr/template/parse.go` walks name → value → `C_STRING`; **exactly one non-blank raw `<list>` entry** is a fixed name (judged on the wire list before blank-entry filtering, taken trimmed — refined by the PR #81 review fix); anything looser (multi-entry or blank-entry list, pattern-only, absent attribute) yields `""` — never the concept term. Exposed as `ComplexObject.NodeName()` (ArchetypeRoot inherits by embedding; slots carry no attributes). Both wire variants covered: `C_PRIMITIVE_OBJECT`-wrapped `<item xsi:type="C_STRING">` and direct `C_STRING` children. REQ-100's node-taxonomy table amended.
2. ~~Tests~~ — **done**: `node_name_test.go` — eleven synthetic table cases (grown from six by the review follow-ups: blank-entry lists, alternative ordering) plus an ArchetypeRoot concept-term case and the corona oracle (four `SECTION.adhoc.v1` names, eight screening-OBSERVATION names incl. `Husten`). While pinning those counts, the docs' "five SECTION siblings" claim was found wrong (it is **four**; the reported collision is the eight OBSERVATIONs under Symptome) — `deviations.md` and the cassettes README corrected.

**DoD:** met — accessor returns `Husten` / `Symptome` etc. for the corona nodes; existing `openehr/template` tests unchanged.

### Phase 2 — Carry through the compiled tree (REQ-111) — **done**

**Tasks:**

1. ~~Field + accessor~~ — **done**: `CompiledNode.nodeName` populated in `descend` on both the `ArchetypeRoot` and plain-`ComplexObject` arms (slots carry no name); exposed as `CompiledNode.NodeName()`. The `openehr/templatecompile` bridge needs no change — its types are aliases, and REQ-111 commits everything reachable as a method, so the accessor is public automatically.
2. ~~Corona assertion~~ — **done**: `compile_nodename_test.go` — `AllByArchetypeID` retains all four `SECTION.adhoc.v1` siblings with their distinct names post-compile (the exact precondition Phase 3's name predicates need, since `byPath` keeps only the first); the shared path resolves to `Symptome`; an unnamed node (`/context`) stays `""`; a synthetic OPT covers the plain-`ComplexObject` carry.

**DoD:** met — compiled nodes expose the name; no path change, all existing tests green.

### Phase 3 — Name predicates on compiled paths (the risky one) — **done**

**Tasks:**

1. ~~Emit on every named node~~ — **done**: `namePredicated`/`NamePredicate` in `internal/templatecompile`, applied in `pathSegment`. The rule is measured, not assumed: across both goldens the name is always *appended to an existing id* (corona 341 archetype-id + 9 at-code segments, GECCO 27 + 3) and never stands alone, so a name never *creates* a bracket. All 57 named nodes in the corpus sit under multiple-valued attributes and already carry an id predicate; the one named root (`IDCR - Laboratory Test Report.v0`) correctly gains nothing — no sibling to disambiguate, no bracket to extend. Commas inside the quoted name are literal (corona has one); the `\'` escape for an embedded quote is the conventional AQL reading, flagged in-code as not golden-verified.
2. ~~dupDepth interaction~~ — **done**: named siblings now separate, and the shared-path route is still live for genuine AOM alternatives — 9 templates keep shared paths (a DV_TEXT and a DV_CODED_TEXT alternative both landing on `…/value`); corona went from whole shared subtrees to exactly one. Both directions pinned: `TestCompile_CoronaSiblingsResolveDistinctly` and `TestCompile_AlternativesStillShareAPath`, plus the oracle test asserting the bare spellings are **gone** so a predicate that failed to apply cannot pass silently.
3. ~~Regression guard first~~ — **done**, landed before emission: `pathsnapshot_test.go` + `testdata/pathsnapshot/{no-name,pins-name}.txt`, partitioned structurally by `NodeName()` (not a hand-kept list) and carrying the corpus census. `no-name.txt` came through the change byte-identical; stripping every `,'…'` from the regenerated `pins-name.txt` reproduces the pre-change file exactly, proving predicate insertion is the only delta — no node added, dropped or moved. Guard verified non-vacuous.
4. ~~Both builders~~ — **done**: `webtemplate`'s own `predicate()` now applies the same rule, sharing `impl.NamePredicate` so the two cannot drift. GECCO reproduces all 24 golden predicated paths (`missing=0`); its 4 surplus predicated paths are the spurious `…/name` leaves inheriting their parent's predicate — Phase 4 task 2.
5. ~~Downstream suites~~ — **done**, and they found two silent REQ-053 breakages, both fixed rather than re-baselined: FLAT **encode** fed the predicated path to `rmpath`, which honours `node,'name'` and so began filtering instances by *runtime* name, dropping values (PROBE-076 caught it); FLAT **decode**'s `parseAQL` would have written `archetype_node_id` as `at0001,'Name'` and split segments on a `/` inside a pinned name. Both now strip the predicate at the resolution boundary (`bareAQLPath`). A third, quieter one: `buildNameIndex` keyed on the compiled path, unreachable from the decoder's bare lookup key — no FLAT fixture covers a name-pinning template, so nothing failed; `flat_nameindex_test.go` closes that hole.

**DoD:** met — corona's four sibling SECTIONs each resolve at a distinct path; GECCO's 24 predicated golden paths all resolve (`missing=0`); templates pinning no name byte-identical; full suite and `make ci` green.

### Phase 4 — Name-derived WebTemplate `id` (REQ-106) — **done**

**Tasks:**

1. ~~`idOf` prefers the pinned name~~ — **done**: name → concept term → attribute name → RM type. This alone cleared `ErrIDCollision` on `Corona_Anamnese`.
2. ~~Stop exporting the pinned name as a data leaf~~ — **done**: `emitAll` skips the LOCATABLE `name` attribute; removed 4 surplus nodes on GECCO and 21 on Corona, and neither golden has a `…/name` node anywhere.
3. ~~Diff both goldens; fold justified differences into `deviations.md`~~ — **done**. The sibling-collision deviation is marked RESOLVED and the Scope entry rewritten. The diff turned up three things beyond the predicted ones, all measured and all now documented:
   - **Single-occurrence `EVENT` containers are lifted, not emitted.** 14 corona extras were EVENT nodes the reference does not carry. Both halves of the discriminator are needed: corona drops 14 (all `max=1`) and keeps 2 (`max=-1`); `constrain_test` keeps 3 EVENT at `max=-1` **and** an INTERVAL_EVENT at `max=1`; the upstream FLAT corpus keys repeating `any_event` in 424 places, still emitted.
   - **Three in-context RM-attribute sets were missing** — INSTRUCTION (`narrative`, `expiry_time`), ACTIVITY (`timing`, `action_archetype_id`), INTERVAL_EVENT (`math_function`, `width`) — the last 6 corona `missing`. Shapes copied from the golden, including the reference's lower-case `expiry_time` name.
   - **An ordinal fallback exists after all** (`dv_text`, `dv_text2`), correcting this plan's "no numeric-suffix rule" evidence note: the suffix is EHRbase's *last resort* where no pinned name separates siblings, which is why it appears in none of the name-disambiguated goldens. Evidence is the upstream FLAT corpus keying `conformance-ehrbase.de.v0`'s two ACTION ELEMENTs as `…/dv_text` and `…/dv_text2`. Implementing it is what makes that template build — the DoD bar for the fixture vendored without a WebTemplate golden. Safe by construction: only templates that previously failed to build can gain a suffix.
   The predicted GECCO `min=1` outlier (14 nodes) is documented as a fixture deviation with the count pinned, not a builder change.
4. ~~Extend PROBE-075 parity to both oracles~~ — **done**: `TestStructuralParity` and `TestInputParity` are table-driven over `constrain_test`, `Corona_Anamnese`, `GECCO_Diagnose`. Extending *input* parity surfaced a pre-REQ-116 gap unrelated to naming — the reference enumerates a constrained DV_TEXT's allowed values where this builder emits the bare input (Corona 20, GECCO 1, the one this plan predicted). Pinned per fixture by count so it must shrink deliberately.

**DoD:** met — `Build` succeeds for `Corona_Anamnese`, `GECCO_Diagnose` **and** `conformance-ehrbase.de.v0`; structural parity is exact on all three vendored goldens (104/104, 230/230, 34/34) with every residual documented and count-pinned.

### Phase 5 — Close out — **done**

1. ~~Traceability + registry~~ — **done**: REQ-116 `implementation: landed` with all nine tests and five packages registered (incl. `openehr/serialize/simplified`, now REQ-116 surface); REQ.md `Impl.` → `landed`. The row's "there is NO numeric-suffix rule" note corrected in place. PROBE-086 stays out of `probes:` — this REQ unblocks it, it does not cover it, and the gate forbids citing a Draft probe as coverage.
2. ~~CHANGELOG~~ — **done**: one `[Unreleased]` bullet calling the path-shape change out explicitly, since it is the consumer-visible break.
3. ~~Hand back~~ — **done**: [the FLAT conformance plan](../2026-07-16-web-template-tests-conformance.md) has its blocker marked cleared, its Definition-of-Ready gate ticked, and Phases 1–3 flipped from blocked to ready. PROBE-086's catalogue status moved from "Draft — blocked" to "Draft — unblocked, adapter not yet written".
4. ~~Archive~~ — **done**: `git mv` into `archive/`, plans index updated, in this PR.

## Risks

| Risk | Mitigation |
|---|---|
| Path change silently breaks a consumer (REQ-102/107/053) | Phase 3 snapshots paths for all vendored templates and asserts byte-identity where no predicate is due; downstream suites re-run before merge |
| Reference `id`/path rule diverges on an untested construct | Assert against goldens, not intuition; anything unmatchable becomes a documented deviation with an ADR-0014 note, never a silent difference |
| Name predicate quoting/escaping (names with `'`, commas, unicode) | Derive escaping from the goldens; add explicit cases — corona names carry umlauts already |
| Scope creep into the AQL builder (REQ-055) | Explicitly deferred; this plan touches compiled-template paths only |

## Mapping to specs

- [clinical-modeling.md § REQ-116](../../specifications/clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths) — the normative contract
- [clinical-modeling.md § REQ-106 `id` generation](../../specifications/clinical-modeling.md#req-106--webtemplate-json-export) — defers the name source to REQ-116
- [conformance.md § PROBE-075](../../specifications/conformance.md#probe-075--webtemplate-structural-parity) / [§ PROBE-086](../../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity)
- [ADR 0014](../../adr/0014-webtemplate-reference-implementation-lock.md) — reference lock this plan converges on
- [`deviations.md`](../../../openehr/template/webtemplate/deviations.md) § Sibling `id` disambiguation — the evidence and the four layers
