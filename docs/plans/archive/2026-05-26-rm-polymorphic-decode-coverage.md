# Plan — RM generated-types: polymorphic decode coverage

**Date:** 2026-05-26
**Status:** **Landed 2026-05-26.** Phases 0–3 shipped on branch `feat/req040-rm-polymorphic-decode-coverage`. Both Issue A (substitutable subtypes in concrete-typed slots via narrow `<Parent>Like` interfaces) and Issue B (generic-over-abstract-bound dispatch) are closed; PROBE-038 implemented (Sandbox) end-to-end.
**Owner:** SDK maintainers
**Covers:** [REQ-040](../../specifications/rm-modeling.md#type-registry-req-040) (type registry), [REQ-046](../../specifications/bmm-conformance.md#primitive-type-mapping) (BMM mapping), [REQ-052](../../specifications/wire.md#req-052) (canonical JSON), [REQ-095](../../specifications/wire.md#req-095) (OpenAPI authoritative source)
**Probes:** **PROBE-038** — Implemented (Sandbox) at [`testkit/probes/serialize/probe_038_canjson_rm_polymorphic_decode.go`](../../../testkit/probes/serialize/probe_038_canjson_rm_polymorphic_decode.go).
**Implementation:** **landed** — 5 narrow Go interfaces (`DVTextLike`, `DVURILike`, `AuditDetailsLike`, `PartyIdentifiedLike`, `ObjectRefLike`) emitted from `plan.ConcreteSubtypes`; canjson + canxml dispatch via typereg / `DecodeAsOrDefault` with missing-`_type` fallback; helper accessors in [`openehr/rm/like_accessors.go`](../../../openehr/rm/like_accessors.go) cover the migration.
**Depends on:** [`internal/bmmgen/render_jsonunmar.go`](../../../internal/bmmgen/render_jsonunmar.go) (generator); [`openehr/rm/typereg/`](../../../openehr/rm/typereg/) (polymorphic dispatch); [BMM corpus](../../../resources/bmm/) (`openehr_rm_1.2.0.bmm.json` parent-child graph)
**Defers:** Lenient / permissive decoding modes (out of scope — fail loudly on unknown discriminators); BMM-model changes (RM model is correct; gap is purely in the Go-side generator's encoding of substitution semantics).

## Goal

Close **SDK-GAP-11**: `canjson.Unmarshal[Composition]` MUST accept any structurally-valid openEHR RM JSON payload the BMM admits — not just payloads whose concrete `_type` discriminator exactly matches the declared Go field type. The gap surfaces when a consumer routes wire bytes through the typed leaf client (`composition.Save` etc.) — the same path SDK-GAP-09 / SDK-GAP-10 already standardised. Closing it makes the typed client the right choice for **every** spec-conformant fixture, not just the subset whose authoring tool emits the most-narrow concrete type.

## Investigation summary

### Issue A — Substitutable subtype in a concrete-typed slot

Reproduced against a real fixture (composition with `LOCATABLE.name: DV_CODED_TEXT` deep under `other_context`):

```
canjson: COMPOSITION: decode /other_context: ... decode /items/0: typereg.Decode "ELEMENT":
  canjson: ELEMENT: decode /_type:
    canjson: expected "DV_TEXT", got "DV_CODED_TEXT":
      typereg: decoded type does not satisfy target
```

The error trace points at the **`name`** field of an `ELEMENT` / `CLUSTER`, not its `value`. Per the BMM ([`openehr_rm_1.2.0.bmm.json`](../../../resources/bmm/)), `LOCATABLE.name: DV_TEXT`. Per the openEHR RM spec, `DV_TEXT` admits Liskov substitution by any subtype; today the only direct subtype is `DV_CODED_TEXT` (concrete).

`ELEMENT.value` itself is already correctly typed as `DataValue` (abstract) and routed through `typereg.DecodeAs[DataValue]` in [`openehr/rm/data_structures_representation_jsonunmar_gen.go:129`](../../../openehr/rm/data_structures_representation_jsonunmar_gen.go#L129) — the SDK-GAP-11 draft's surface read was off; the actual gap is on every `LOCATABLE`-descended class's `name` (and on any other concrete-typed slot that admits substitution per BMM ancestry).

The root cause is the **strict class-equality check** every generated `UnmarshalJSON` emits today (e.g. [`data_types_text_jsonunmar_gen.go:159`](../../../openehr/rm/data_types_text_jsonunmar_gen.go#L159)):

```go
if aux.Class != "" && aux.Class != "DV_TEXT" {
    return &typereg.DecodeError{ /* "expected DV_TEXT, got DV_CODED_TEXT" */ }
}
```

Even if the check were softened to "DV_TEXT or any subtype", decoding `DV_CODED_TEXT` bytes into a `DVText` *struct* still loses `defining_code` — and the cdr-bench round-trip (decode → re-marshal → POST) needs to be lossless. **Data loss isn't acceptable** because `composition.Save` re-marshals the typed value before sending; subtype-only fields dropped on decode would silently disappear from the wire body.

### Issue B — Generic over an abstract type parameter

Reproduced against the third failing fixture (`DV_INTERVAL[DV_QUANTITY]` payload at `ELEMENT.value`):

```
canjson: DV_INTERVAL:
  json: cannot unmarshal object into Go struct field
    DVIntervalJSONUnmarshaller[...rm.DVOrdered].lower of type rm.DVOrdered
```

The generated [`DVIntervalJSONUnmarshaller[T DVOrdered]`](../../../openehr/rm/data_types_quantity_jsonunmar_gen.go) carries `Lower T` and `Upper T`. When `T` is the *abstract bound itself* — `DVInterval[DVOrdered]` — Go's `encoding/json` cannot decode a JSON object into an interface field. The bench fixture uses `DV_INTERVAL<DV_QUANTITY>` (a concrete subtype) but the SDK's `ReferenceRange.range`, `DVQuantified.normal_range`, etc. all instantiate `DVInterval[DVOrdered]` — so the decoder hits the interface bound at runtime regardless of the on-wire concrete type.

The fix: emit `Lower`, `Upper` as `json.RawMessage` and dispatch via `typereg.DecodeAs[T]`. Same template the generator already uses for `Element.Value` (polymorphic `DataValue`); just generalised to type-parameterised polymorphic fields.

## Design — two changes, one regenerate

Both issues collapse to the same generator improvement: **derive polymorphism from BMM ancestry, not just from an explicit "abstract" marker on the field type**. Concretely:

| Field shape in BMM | Today (bmmgen output) | After this plan |
|---|---|---|
| `attr: ConcreteParent` where `ConcreteParent` has registered subtypes (e.g. `LOCATABLE.name: DV_TEXT`) | `Name DVText` (struct, strict class check) | `Name DVTextLike` (narrow interface, typereg dispatch) |
| `attr: AbstractParent` (e.g. `ELEMENT.value: DATA_VALUE`) | `Value DataValue` + typereg dispatch | unchanged — already correct |
| `attr: T` where `T` is a type-parameter bound to an abstract (e.g. `DV_INTERVAL[T: DV_ORDERED].lower: T`) | `Lower T` (Go's json fails on interface) | `Lower json.RawMessage` + `typereg.DecodeAs[T]` |

For Issue A we **lift the field type** to a narrow interface (one per parent class that has subtypes), so that the decode is lossless. The alternative — keeping the field as the struct and accepting subtype-only field loss — fails the lossless-round-trip requirement.

A narrow interface (e.g. `DVTextLike` = `DV_TEXT | DV_CODED_TEXT`) constrains the API better than the broad `DataValue` (which would also admit `DV_QUANTITY` and the rest of the family). Narrow interfaces are generated from the BMM parent-child graph.

## Phases

### Phase 0 — Failing repro + spec reservation

**Outcome:** Three fixture-driven decode tests fail today; PROBE-038 reserved at Status: Draft.

**Tasks:**

1. **Reserve PROBE-038** in [`docs/specifications/conformance.md`](../../specifications/conformance.md) under Canonical JSON / formats: "Polymorphic RM decode coverage — `canjson.Unmarshal[Composition]` decodes every BMM-admissible discriminator across all substitutable slots and parameterised generic types". Status: **Draft**.
2. **Vendor fixtures** under `testkit/cassettes/rm/polymorphic/` (via [`testkit/fixtures`](../../../testkit/fixtures/) `RMJSON`) — one per failure mode:
   - `name_dv_coded_text.json` — minimal Composition with `LOCATABLE.name: DV_CODED_TEXT` at any nested depth.
   - `dv_interval_quantity.json` — minimal Composition with `ELEMENT.value: DV_INTERVAL<DV_QUANTITY>`.
   - `representative_full.json` — a wider real-world Composition exercising both patterns plus DVOrdinal / DVScale ranges (sourced from a CKM-published exemplar).
3. **Fixture test pin** at `openehr/serialize/canjson/polymorphic_decode_test.go` — table-driven against the three fixtures; Issue B case runs after Phase 1; Issue A cases `Skip` until Phase 2 lands.

**Definition of done:** PROBE-038 entry visible in `docs/specifications/conformance.md`; `make spec-check` happy; the test stub compiles and is `Skip`-ped.

### Phase 1 — Issue B: generic abstract decode (smaller, isolating)

**Outcome:** `DVInterval[T DVOrdered]` and any sibling generic with an abstract bound decode the parameterised field via typereg. Fixture `dv_interval_quantity.json` passes; `representative_full.json` passes its DVInterval portion.

**Tasks:**

1. Extend `internal/bmmgen/render_jsonunmar.go::polymorphicProperty` to additionally classify a property as polymorphic when its declared type is a **type parameter constrained by an abstract type**. (The function today only flags an "abstract element name" — verify and widen.)
2. Emit the standard `RawMessage` + `typereg.DecodeAs[T]` pattern in the generated unmarshaller for these properties. Add a generator-level test in `internal/bmmgen/render_jsonunmar_test.go` pinning the new emission shape against a synthetic mini-BMM.
3. `make codegen` — regenerate. Inspect the `DVInterval`, `ReferenceRange` (and any others surfaced) diffs to verify.
4. `make codegen-verify` + `make test` green.

**Definition of done:** `DVInterval[DVOrdered].Lower` decodes a `DV_QUANTITY` payload via typereg dispatch; the Phase 0 fixture test for `dv_interval_quantity.json` is unskipped and passes.

### Phase 2 — Issue A: narrow polymorphic interfaces for concrete-with-subtype slots

**Outcome:** Every BMM property whose declared type has registered subtypes (per `ancestors` graph) is emitted as a narrow Go interface. `LOCATABLE.name: DV_TEXT` becomes `Name DVTextLike` (admitting `*DVText` and `*DVCodedText`). Decode is lossless and re-marshal round-trips.

**Tasks:**

1. **Generator: ancestry traversal.** Add a step in `internal/bmmgen/plan.go` that builds the BMM parent-child graph from `class_definitions[*].ancestors`. For each concrete class with ≥1 registered subtype, emit a narrow interface (`<ParentClass>Like`) declaring a marker method (`is<ParentClass>Like()`), and `func` declarations attaching it to the parent and every transitive subtype. Skip when the parent already has an abstract Go interface emitted (e.g. `DataValue`, `DVOrdered`) — reuse those.
2. **Generator: field emission.** For any property whose declared type is such a parent class, emit the field as the narrow interface type; emit the unmarshaller's struct field as `json.RawMessage` with a `typereg.DecodeAs[<ParentClass>Like]` dispatch line in the `UnmarshalJSON` body. Marshal side stays unchanged — interfaces marshal via the concrete type's `MarshalJSON` already.
3. **Generator unit test** at `internal/bmmgen/render_jsonunmar_test.go` covering the new emission against a synthetic two-class BMM (`Parent` + `Child(ancestors: [Parent])`).
4. **Regenerate** RM + AOM 1.4 (`make codegen`). Diff is expected to be wide: every `Name DVText` becomes `Name DVTextLike`, every `Numerator DVQuantified` (or similar) shifts to the corresponding `*Like` interface, etc. **Land the regen + the generator change in a single commit** so `make codegen-verify` stays green.
5. **API migration audit.** Run `grep -r "\.Name\.Value" --include="*.go"` and similar across the tree. Inside `internal/templatecompile`, `openehr/template`, `openehr/composition`, `openehr/instance`, `openehr/validation`, etc. — update call sites to use type-assertions / helper accessors. Document the new pattern in the package docs of each affected leaf.
6. **Public-API note.** Add a one-line entry to the v0.x → v0.next breaking-change list (CHANGELOG `[Unreleased]`): `Name`, `<other>` fields on `LOCATABLE`-descended classes are now `<...>Like` interfaces.

**Design alternative considered:** subtype-tolerant strict check (accept `DV_TEXT` or any descendant in the string compare, decode into the parent struct only). **Rejected** because it loses subtype-only fields on decode, breaking the lossless round-trip the typed `composition.Save` flow needs.

**Definition of done:** All three Phase 0 fixtures decode cleanly; `make ci` green; CHANGELOG entry written.

### Phase 3 — PROBE-038 implementation + traceability

**Outcome:** Cross-SDK probe pins the substitution semantics; traceability + conformance prose reflect Implemented (Sandbox).

**Tasks:**

1. Implement `testkit/probes/serialize/probe_038_canjson_rm_polymorphic_decode.go` — table-driven: for each of the three Phase-0 fixtures, decode via `canjson.Unmarshal`, re-marshal via `canjson.Marshal`, and assert (a) decode succeeds, (b) the recovered tree preserves every original `_type` discriminator (no silent narrowing on substitutable slots), (c) re-marshalling produces byte-equivalent JSON for the same logical content (canonical JSON wins ties).
2. Conformance entry flips Draft → **Implemented (Sandbox)**; coverage matrix gains PROBE-038.
3. `docs/specifications/traceability.yaml` REQ-052 entry gains PROBE-038 + probe file.
4. `docs/specifications/wire.md` § Canonical JSON gains a "Polymorphic substitution" paragraph documenting the rule: any property's declared type admits Liskov substitution by any concrete subtype in `typereg`; the Go API surfaces this via `<Parent>Like` interfaces. Cross-reference [ADR 0001](../../adr/0001-bmm-version-bump-runbook.md) for what happens on BMM bump (new subtypes auto-extend the interfaces).

**Definition of done:** `make ci` green; PROBE-038 passes against the three fixtures; `make spec-check` happy.

### Phase 4 — Archive plan + close GAP

**Outcome:** Plan moved to `docs/plans/archive/`; CHANGELOG entry; roadmap row updated; consumer's pre-flight workaround can be dropped.

**Tasks:**

1. CHANGELOG `[Unreleased]` gains a one-line bullet covering Phase 1 + 2 + PROBE-038.
2. Plan moved to `docs/plans/archive/2026-05-26-rm-polymorphic-decode-coverage.md` (rebase internal links). Add row to `docs/plans/archive/README.md`; drop from active.
3. `docs/roadmap.md` — if there's a "RM polymorphic decode" row, flip Planned → Landed; otherwise inline note under the existing canonical-JSON row.

## Cross-references

- **SDK-GAP-11** — consumer gap report (private; concept only — naming and source consumer kept anonymous in this plan).
- **SDK-GAP-09** — response-side bare-decode contract (closed in PR #17).
- **SDK-GAP-10** — request-side `contribution.Commit` shape (closed in PR #22).
- [`internal/bmmgen/render_jsonunmar.go`](../../../internal/bmmgen/render_jsonunmar.go) — current generator for JSON unmarshallers; `polymorphicProperty` is the entry point.
- [`openehr/rm/typereg/registry.go`](../../../openehr/rm/typereg/registry.go) — type registry and `DecodeAs[T]` dispatch.
- [openEHR RM Latest — DV_TEXT inheritance](https://specifications.openehr.org/releases/RM/latest/data_types.html#_dv_text_class) and [DV_INTERVAL](https://specifications.openehr.org/releases/RM/latest/data_types.html#_dv_interval_class).

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0 — PROBE-038 reserved + 3 fixture tests | done |
| Phase 1 — generic abstract decode (Issue B) | done |
| Phase 1 — bmmgen `render_jsonunmar_polymorphic_test.go` | done |
| Phase 1 — `make codegen` regen clean | done |
| Phase 2 — ancestry-driven narrow interface emission | |
| Phase 2 — full RM + AOM 1.4 regen + call-site migration | |
| Phase 2 — CHANGELOG breaking-change note | |
| Phase 3 — PROBE-038 implementation | |
| Phase 3 — conformance + traceability + wire.md | |
| Phase 4 — plan archived, roadmap updated | |
| `make ci` green | |
