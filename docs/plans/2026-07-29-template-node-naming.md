# Plan — Template-level node naming and name-predicated paths

**Date:** 2026-07-29
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** **[REQ-116](../specifications/clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths)** (template-level node naming and name-predicated paths) — canonical prose landed, registry row `proposed`
**Amends:** [REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) (parse surface), [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge) (compiled carry), [REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) (`id` source — prose already amended to defer to REQ-116)
**Must not regress:** [REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation), [REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator), [REQ-053](../specifications/wire.md#req-053) — all consume compiled path shape
**Probes:** [PROBE-075](../specifications/conformance.md#probe-075--webtemplate-structural-parity) (extended to new fixtures), [PROBE-086](../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) (unblocked by this plan)
**Implementation:** planned
**Depends on:** the shared-path-subtree compile fix and the vendored FLAT corpus (PR #79, landed)
**Defers:** Better-platform dialect naming; multi-language name selection beyond the document default language; retro-fitting name predicates onto AQL *builder* output (REQ-055) — this plan changes compiled-template paths only

## Goal

Close the gap that blocks PROBE-086 and any WebTemplate for a template that reuses an archetype among siblings: the SDK reads a node's display name from the *archetype's* concept term, where the reference reads the **template-level node name** pinned in the OPT. Consequences reach four layers — OPT parse, the compiled tree, AQL path predicates, and WebTemplate `id` derivation. Consumers are `webtemplate` (distinct sibling ids), the FLAT/STRUCTURED codecs and validators (addressable sibling paths), and PROBE-086.

**Architecture.** No new packages. The name is parsed in `openehr/template`, carried on `CompiledNode`, consulted by the path builder in `internal/templatecompile`, and preferred by `idOf` in `openehr/template/webtemplate`. Public-API additions only (one accessor per layer); no signature changes.

**Evidence base** (established 2026-07-29, recorded in [`deviations.md`](../../openehr/template/webtemplate/deviations.md) § Sibling `id` disambiguation): there is **no** numeric-suffix rule — verified across the upstream `corona_anamnese`, `multi_occurrence`, and `AlternativeEvents` WebTemplate goldens. The reference disambiguates by name alone and name-predicates `aqlPath` **350×** in the corona golden versus **0×** in `constrain_test`, which is why PROBE-075 holds 104/104 today without any of this.

## Definition of Ready

- [x] Canonical REQ-116 prose exists, with acceptance criteria.
- [x] REQ.md row + `traceability.yaml` entry landed.
- [x] Mechanism verified against upstream goldens (not guessed).
- [x] No irreversible fork: [ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md) already locks the SDK to the EHRbase reference, and this plan *converges* on it — so no new ADR is needed. (If reference behaviour turns out to be unmatchable on some construct, that exception needs an ADR amendment, not a silent deviation.)
- [ ] Reference goldens vendored for the fixtures under test (Phase 0).

## Definition of Done

- REQ-116 acceptance criteria all demonstrated by tests citing `// REQ-116`.
- `corona_anamnese` and `conformance-ehrbase.de.v0` build a WebTemplate matching the vendored goldens on the `id` and `aqlPath` sets, within documented deviations.
- PROBE-075 parity extended to at least one name-predicated fixture and still 104/104 on `constrain_test`.
- **Predicate-free paths byte-identical to pre-change output** — asserted, not assumed.
- REQ.md `Impl.` for REQ-116 → landed; `traceability.yaml` tests/probes filled; `deviations.md` updated (this deviation removed or narrowed).
- `make spec-check` + `make ci` green; PROBE-086 unblocked (its own plan's Phase 1 can start).

## Phases

### Phase 0 — Vendor the oracles

**Tasks:**

1. Vendor `corona_anamnese.opt` + `corona_anamnese.json` (reference WebTemplate) beside the existing PROBE-075 fixture in `testkit/cassettes/webtemplate/`, following the established stem convention (`*.webtemplate.json`); record commit + Apache-2.0 provenance in `THIRD_PARTY_LICENSES.md`. Consider `multi_occurrence` too — small, and a second name-predicated case.
2. Note the size cost in the cassettes README (corona OPT ≈ 1.2 MB, golden ≈ 235 KB).

**DoD:** fixtures resolve via `testkit/fixtures`; `make ci` green (no behaviour change yet).

### Phase 1 — Parse and expose the name (REQ-100)

**Tasks:**

1. In `openehr/template`, parse the `name` attribute's fixed `C_STRING` (`<item xsi:type="C_STRING"><list>…</list></item>`) on definition nodes; store it and expose one accessor (e.g. `NodeName() string`) on the node types that can carry it. Absent ⇒ empty string, never the archetype concept term (REQ-116).
2. Table-driven tests over a small synthetic OPT plus the vendored corona fixture.

**DoD:** accessor returns `Husten` / `Symptome` etc. for the corona nodes; existing `openehr/template` tests unchanged.

### Phase 2 — Carry through the compiled tree (REQ-111)

**Tasks:**

1. Add the field + accessor to `internal/templatecompile.CompiledNode`, populated in `descend`; re-export through the `openehr/templatecompile` bridge per REQ-111's aliasing approach.
2. Assert the name survives compile for the corona fixture.

**DoD:** compiled nodes expose the name; no path change yet, so all existing tests stay green.

### Phase 3 — Name predicates on compiled paths (the risky one)

**Tasks:**

1. In the path builder, emit `[archetype_node_id,'Name']` **only** where siblings share an archetype node id *and* pin distinct names (REQ-116). Sole children and un-named collisions keep today's form.
2. Interaction with the landed shared-path fix: name predicates make the *named* sibling case distinct, so `dupDepth` should stop being reached for it while continuing to cover genuine AOM alternatives and un-named same-`node_id` siblings. Assert both paths explicitly — the fix must not become dead code by accident, nor mask a predicate that failed to apply.
3. **Regression guard first:** before changing emission, snapshot the compiled path set for every vendored template and assert byte-identity afterwards for templates with no repeated sibling archetype ids.
4. Re-run REQ-102 validation, REQ-107 instance, and REQ-053 FLAT/STRUCTURED suites; investigate any diff rather than re-baselining it.

**DoD:** corona's sibling SECTIONs each resolve at a distinct path; predicate-free templates byte-identical; downstream suites green.

### Phase 4 — Name-derived WebTemplate `id` (REQ-106)

**Tasks:**

1. `idOf` prefers the template-level name, falling back to today's term text, then attribute name, then RM type.
2. Diff generated `id` + `aqlPath` sets against the corona golden; fold genuine, justified differences into `deviations.md` and delete the sibling-collision deviation once it no longer applies.
3. Extend PROBE-075 parity to corona (and `multi_occurrence` if vendored); keep `constrain_test` at 104/104.

**DoD:** `webtemplate.Build` succeeds for both blocked templates; parity asserted against goldens.

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
