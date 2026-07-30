# Plan — Template-level node naming and name-predicated paths

**Date:** 2026-07-29
**Status:** Partial — Phases 0–2 landed 2026-07-30 (oracles vendored + pinned; node name parsed, exposed, carried through compile); Phases 3–4 open
**Owner:** SDK maintainers
**Covers:** **[REQ-116](../specifications/clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths)** (template-level node naming and name-predicated paths) — canonical prose landed, registry row `partial`
**Amends:** [REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) (parse surface), [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge) (compiled carry), [REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) (`id` source — prose already amended to defer to REQ-116)
**Must not regress:** [REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation), [REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator), [REQ-053](../specifications/wire.md#req-053) — all consume compiled path shape
**Probes:** [PROBE-075](../specifications/conformance.md#probe-075--webtemplate-structural-parity) (extended to new fixtures), [PROBE-086](../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) (unblocked by this plan)
**Implementation:** partial — Phases 0–2 landed; Phases 3–4 open
**Depends on:** the shared-path-subtree compile fix and the vendored FLAT corpus (PR #79, landed)
**Defers:** Better-platform dialect naming; multi-language name selection beyond the document default language; retro-fitting name predicates onto AQL *builder* output (REQ-055) — this plan changes compiled-template paths only

## Goal

Close the gap that blocks PROBE-086 and any WebTemplate for a template that reuses an archetype among siblings: the SDK reads a node's display name from the *archetype's* concept term, where the reference reads the **template-level node name** pinned in the OPT. Consequences reach four layers — OPT parse, the compiled tree, AQL path predicates, and WebTemplate `id` derivation. Consumers are `webtemplate` (distinct sibling ids), the FLAT/STRUCTURED codecs and validators (addressable sibling paths), and PROBE-086.

**Architecture.** No new packages. The name is parsed in `openehr/template`, carried on `CompiledNode`, consulted by the path builder in `internal/templatecompile`, and preferred by `idOf` in `openehr/template/webtemplate`. Public-API additions only (one accessor per layer); no signature changes.

**Evidence base** (established 2026-07-29, recorded in [`deviations.md`](../../openehr/template/webtemplate/deviations.md) § Sibling `id` disambiguation): there is **no** numeric-suffix rule — verified across the upstream `corona_anamnese`, `multi_occurrence`, and `AlternativeEvents` WebTemplate goldens. The reference disambiguates by name alone and name-predicates `aqlPath` **350×** in the corona golden versus **0×** in `constrain_test`.

**Correction (2026-07-30).** The predicate trigger is the **pinned name alone**, not sibling collision. Measured across the vendored goldens: every name in a golden predicate is a fixed `C_STRING` in its OPT (1:1 for both oracles), `constrain_test` pins none and carries 0 predicates over 104 nodes — *that* is why PROBE-075 holds 104/104 — and `GECCO_Diagnose` predicates all three `/content` children whose archetype ids are **distinct**, plus a sole `CLUSTER` child. A collision-conditioned rule emits too few predicates and would fail GECCO parity. **Blast radius is therefore wider than "the reuse templates":** counted across all 58 vendored OPTs, **9** pin at least one node name — the 2 oracles (`Corona_Anamnese` 24, `GECCO_Diagnose` 6, each exactly matching its golden's distinct predicate-name count) plus **7** others: `test_template_rename_node{,_2}` (8 each), `clinical_content_validation` (4), `IDCR -  Adverse Reaction List.v1` (4), `IDCR - Laboratory Test Report.v0`, `Test_dv_text_list_constraint.v0`, `Test_dv_text_open_constraint.v0` (1 each). Their compiled paths change too — Phase 3's regression guard must be scoped by *pins-a-name*, not by *has-reused-siblings*.

**Measured residual** (built `GECCO_Diagnose` vs its golden on the PROBE-075 surface, predicates normalised away): path set matches **exactly** (`missing=0`), so predicate emission closes the whole path gap. What remains is 4 extra `…/name` `DV_TEXT` leaves (the pinned name exported as *data* by `build.go`), 14 `min`/`max` deltas where the **GECCO golden is the outlier** (`1/1` vs `0/1`; `constrain_test` and Corona both agree with this builder, and GECCO's OPT constrains no `existence`), and 1 input-count delta. The min/max set needs a documented deviation in Phase 4, not a builder change.

## Definition of Ready

- [x] Canonical REQ-116 prose exists, with acceptance criteria.
- [x] REQ.md row + `traceability.yaml` entry landed.
- [x] Mechanism verified against upstream goldens (not guessed).
- [x] No irreversible fork: [ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md) already locks the SDK to the EHRbase reference, and this plan *converges* on it — so no new ADR is needed. (If reference behaviour turns out to be unmatchable on some construct, that exception needs an ADR amendment, not a silent deviation.)
- [x] Reference goldens vendored for the fixtures under test (Phase 0 — done).

## Definition of Done

- REQ-116 acceptance criteria all demonstrated by tests citing `// REQ-116`.
- `Corona_Anamnese` and `GECCO_Diagnose` — the two vendored oracles — build a WebTemplate matching their vendored goldens on the `id` and `aqlPath` sets, within documented deviations. `conformance-ehrbase.de.v0` is vendored **without** a WebTemplate golden (OPT + FLAT bodies only), so for it the bar is `Build` succeeding, until a golden is vendored.
- PROBE-075 parity extended to at least one name-predicated fixture and still 104/104 on `constrain_test`.
- **Predicate-free paths byte-identical to pre-change output** — asserted, not assumed.
- REQ.md `Impl.` for REQ-116 → landed; `traceability.yaml` tests/probes filled; `deviations.md` updated (this deviation removed or narrowed).
- `make spec-check` + `make ci` green; PROBE-086 unblocked (its own plan's Phase 1 can start).

## Phases

### Phase 0 — Vendor the oracles — **done**

**Tasks:**

1. ~~Vendor `corona_anamnese`~~ — **done**: `Corona_Anamnese.opt` + `Corona_Anamnese.webtemplate.json` vendored at the `constrain_test` pin (`22b01e0c`), stems per `template_id` convention; provenance in `THIRD_PARTY_LICENSES.md`. **`multi_occurrence` was dropped**: its golden carries **zero** name-predicated `aqlPath`s (as does `AlternativeEvents`), so the "second name-predicated case" premise was wrong. Substituted **`GECCO_Diagnose`** (30 predicate segments over 24 paths, space-free `template_id`, ~⅕ corona's size) — and it turned out to cover the gap's *other* failure mode: it **builds without error** today (no sibling id collision) while silently diverging from its golden on `aqlPath`, where corona fails loudly with `ErrIDCollision`. Both modes are pinned by tests: compile guards in `internal/templatecompile/webtemplate_oracle_compile_test.go`, build outcomes in `openehr/template/webtemplate/req116_gap_test.go`.
2. ~~Note the size cost in the cassettes README~~ — **done** (corona OPT 1.2 MB + golden 230 KB; GECCO 210 KB + 73 KB), plus `fixtures.WebTemplateOpt` / `WebTemplateReference` resolvers.

**DoD:** met — fixtures resolve via `testkit/fixtures`; `make ci` green (no behaviour change).

### Phase 1 — Parse and expose the name (REQ-100) — **done**

**Tasks:**

1. ~~Parse + expose~~ — **done**: `deriveNodeName` in `openehr/template/parse.go` walks name → value → `C_STRING`; a **single-entry** list is a fixed name, anything looser (multi-entry list, pattern-only, absent attribute) yields `""` — never the concept term. Exposed as `ComplexObject.NodeName()` (ArchetypeRoot inherits by embedding; slots carry no attributes). Both wire variants covered: `C_PRIMITIVE_OBJECT`-wrapped `<item xsi:type="C_STRING">` and direct `C_STRING` children. REQ-100's node-taxonomy table amended.
2. ~~Tests~~ — **done**: `node_name_test.go` — six synthetic table cases plus the corona oracle (four `SECTION.adhoc.v1` names, eight screening-OBSERVATION names incl. `Husten`). While pinning those counts, the docs' "five SECTION siblings" claim was found wrong (it is **four**; the reported collision is the eight OBSERVATIONs under Symptome) — `deviations.md` and the cassettes README corrected.

**DoD:** met — accessor returns `Husten` / `Symptome` etc. for the corona nodes; existing `openehr/template` tests unchanged.

### Phase 2 — Carry through the compiled tree (REQ-111) — **done**

**Tasks:**

1. ~~Field + accessor~~ — **done**: `CompiledNode.nodeName` populated in `descend` on both the `ArchetypeRoot` and plain-`ComplexObject` arms (slots carry no name); exposed as `CompiledNode.NodeName()`. The `openehr/templatecompile` bridge needs no change — its types are aliases, and REQ-111 commits everything reachable as a method, so the accessor is public automatically.
2. ~~Corona assertion~~ — **done**: `compile_nodename_test.go` — `AllByArchetypeID` retains all four `SECTION.adhoc.v1` siblings with their distinct names post-compile (the exact precondition Phase 3's name predicates need, since `byPath` keeps only the first); the shared path resolves to `Symptome`; an unnamed node (`/context`) stays `""`; a synthetic OPT covers the plain-`ComplexObject` carry.

**DoD:** met — compiled nodes expose the name; no path change, all existing tests green.

### Phase 3 — Name predicates on compiled paths (the risky one)

**Tasks:**

1. In the path builder, emit `[archetype_node_id,'Name']` on the segment of **every node that pins a template-level name** (REQ-116) — regardless of whether any sibling shares its archetype node id. Nodes that pin no name keep today's bare form, including un-named collisions.
2. Interaction with the landed shared-path fix: name predicates make the *named* sibling case distinct, so `dupDepth` should stop being reached for it while continuing to cover genuine AOM alternatives and un-named same-`node_id` siblings. Assert both paths explicitly — the fix must not become dead code by accident, nor mask a predicate that failed to apply.
3. **Regression guard first:** before changing emission, snapshot the compiled path set for every vendored template and assert byte-identity afterwards **for templates that pin no name** — that is the correct partition, and it is narrower than "no repeated sibling archetype ids" (9 of the 58 vendored OPTs pin a name). For those 9, record the expected path delta deliberately.
4. **Two path builders, not one.** `webtemplate` does *not* consume
   `CompiledNode.AQLPath()` — it composes its own (`build.go:154`,
   `childPath := parentPath + "/" + attr.Name() + predicate(c)`) via its own
   `predicate()` helper (`build.go:271`, archetype-id-or-at-code only), and it drops
   structural wrappers, so its paths legitimately differ from the compiled ones
   (GECCO: 38 WebTemplate nodes vs 74 compiled). Changing only the
   `internal/templatecompile` builder therefore leaves WebTemplate `aqlPath`
   unpredicated and PROBE-075 still failing. Both need the name.
5. Re-run REQ-102 validation, REQ-107 instance, and REQ-053 FLAT/STRUCTURED suites; investigate any diff rather than re-baselining it.

**DoD:** corona's sibling SECTIONs each resolve at a distinct path; GECCO's 24 predicated golden paths all resolve (`missing=0`); templates pinning no name byte-identical; downstream suites green.

### Phase 4 — Name-derived WebTemplate `id` (REQ-106)

**Tasks:**

1. `idOf` prefers the template-level name, falling back to today's term text, then attribute name, then RM type.
2. Stop exporting the pinned `name` attribute as a data leaf — the reference carries the name on the node, not as a child. Measured on GECCO: 4 spurious `…/name` `DV_TEXT` nodes today (`build.go` walks the constrained `name` attribute).
3. Diff generated `id` + `aqlPath` sets against the corona **and** GECCO goldens; fold genuine, justified differences into `deviations.md` and delete the sibling-collision deviation once it no longer applies. Expect to document the GECCO golden's `min=1` RM-attribute-leaf outlier (14 nodes) as a fixture deviation — this builder agrees with `constrain_test` and Corona, and GECCO's OPT constrains no `existence`.
4. Extend PROBE-075 parity to `Corona_Anamnese` and `GECCO_Diagnose` (the vendored oracles — `multi_occurrence` was dropped in Phase 0); keep `constrain_test` at 104/104.

**DoD:** `webtemplate.Build` succeeds for both blocked templates; parity asserted against both goldens.

### Phase 5 — Close out

1. `traceability.yaml` tests/probes for REQ-116; REQ.md `Impl.` → landed.
2. CHANGELOG entry (public accessors + path-shape change — call out the path change explicitly for consumers).
3. Hand back to [the FLAT conformance plan](2026-07-16-web-template-tests-conformance.md): its Phase 1 is now unblocked.
4. Archive this plan.

## Risks

| Risk | Mitigation |
|---|---|
| Path change silently breaks a consumer (REQ-102/107/053) | Phase 3 snapshots paths for all vendored templates and asserts byte-identity where no predicate is due; downstream suites re-run before merge |
| Reference `id`/path rule diverges on an untested construct | Assert against goldens, not intuition; anything unmatchable becomes a documented deviation with an ADR-0014 note, never a silent difference |
| Name predicate quoting/escaping (names with `'`, commas, unicode) | Derive escaping from the goldens; add explicit cases — corona names carry umlauts already |
| Scope creep into the AQL builder (REQ-055) | Explicitly deferred; this plan touches compiled-template paths only |

## Mapping to specs

- [clinical-modeling.md § REQ-116](../specifications/clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths) — the normative contract
- [clinical-modeling.md § REQ-106 `id` generation](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) — defers the name source to REQ-116
- [conformance.md § PROBE-075](../specifications/conformance.md#probe-075--webtemplate-structural-parity) / [§ PROBE-086](../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity)
- [ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md) — reference lock this plan converges on
- [`deviations.md`](../../openehr/template/webtemplate/deviations.md) § Sibling `id` disambiguation — the evidence and the four layers
