# Plan — C_PRIMITIVE_OBJECT wire-parser + REQ-107 UID emission

**Date:** 2026-05-26
**Status:** Draft — generator-side surface fixes landed (PR #20); Phases 1 (wire-parser `C_PRIMITIVE_OBJECT.<item>` extraction) + 2 (REQ-107 UID emission) remain open
**Owner:** SDK maintainers
**Covers:** [REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) (wire parser), [REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) (instance synthesiser UID emission), [REQ-101](../specifications/clinical-modeling.md#req-101) (PROBE-023 widening)
**Probes:** PROBE-023 (widening to full unmarshal round-trip — open); PROBE-027 (extension to `clinical_note.opt` — **landed in PR #20**)
**Implementation:** **partial** — PR #20 (merged) landed the generator-side `materialiseSingle` AOM-short-name fix + EVENT_CONTEXT rmread gap + PROBE-027 on `clinical_note.opt`. Wire-parser inner-`<item>` extraction and REQ-107 UID emission **not landed** yet (keep PROBE-023 at "marshal-fragment parity (v1)" until those close).
**Depends on:** REQ-100 follow-ups Phases 1–6 (landed); REQ-107 Phases 0–3 (PR #18 merged, plan archived); REQ-101 Phases 0–2 (PR #19 merged, plan archived); PR #20 follow-ups (merged)
**Defers:** REQ-104 slot-assertion grammar (separate plan); broader AOM 1.4 primitive coverage beyond the closed REQ-103 set

## Goal

Close two paired gaps surfaced by the PR #18 + PR #19 reviews that block broader OPT coverage and full round-trip parity:

1. **Wire parser** — `openehr/template/parse.go` currently treats `<children xsi:type="C_PRIMITIVE_OBJECT">` as the leaf shape, dropping the inner `<item xsi:type="C_*">` constraint element. The compiled tree's `CompiledNode.PrimitiveConstraint()` returns nil even though the OPT carried a real `C_DURATION` / `C_DATE` / etc. — the validator and the REQ-107 synthesizer both lose the constraint.
2. **REQ-107 UID emission** — `openehr/instance.newHierObjectID()` returns a `rm.HierObjectID` **value** (not pointer); `canjson` marshals `OBJECT_ID` polymorphism via a pointer-receiver `MarshalJSON`. The emitted JSON omits the `_type` discriminator on `Composition.uid` and subsequent decoders reject the bytes.

Both bugs are pre-existing (PR #18 surface fix landed `concreteFor` + `bmmSubtypes` + rmwrite writers for the DV temporal types, which made the **mapping** correct but exposed the **wire-parser** gap when the inner constraint is reachable; the UID issue surfaced separately when PR #19's PROBE-023 was narrowed to marshal-fragment parity because full unmarshal failed).

## Out of scope

- **Adding new RM types to typereg or rminfo** — the issue is not which RM types exist; it's that `C_PRIMITIVE_OBJECT.<item>` is dropped at parse time.
- **REQ-104 slot-assertion grammar** — the REQ-107 slot-fill heuristic (`openEHR-EHR-<rmType>.example.v1` stamping) is a separate follow-up.
- **Changing the `PrimitiveConstraint` interface surface** — the closed REQ-103 set already covers `C_DURATION` etc.
- **Validator-side `bmmSubtypes` changes** — already landed in PR #18 for the AOM 1.4 primitive short-names.

## Problem in detail

### `C_PRIMITIVE_OBJECT` wrapping (REQ-100 wire-parser scope)

ADL 1.4 OPT XML uses two shapes for primitive constraints:

```xml
<!-- Shape A: direct primitive on <children> -->
<children xsi:type="C_DURATION">
  <range>...</range>
  ...
</children>

<!-- Shape B: wrapped under C_PRIMITIVE_OBJECT with inner <item> -->
<children xsi:type="C_PRIMITIVE_OBJECT">
  <rm_type_name>DURATION</rm_type_name>
  <occurrences>...</occurrences>
  <node_id/>
  <item xsi:type="C_DURATION">
    <range>...</range>
    ...
  </item>
</children>
```

`clinical_note.opt` uses Shape B; `vital_signs.opt` uses Shape A (which is why the current parser passes PROBE-027 on vital_signs but fails on clinical_note).

The current parser (`openehr/template/parse.go::buildComplexObject`) reaches the wrapper, finds `xsi:type="C_PRIMITIVE_OBJECT"` (not in the `buildPrimitive` switch), falls through to the default branch, calls `buildPrimitive(o, strict)` which returns `(nil, nil)`, and emits a `*ComplexObject` with `primitive: nil`. The inner `<item xsi:type="C_DURATION">` is silently dropped.

Downstream impact:
- **Validator (REQ-102)**: `walkNode` never enters the primitive-leaf branch for this node; cardinality is enforced but the C_DURATION range / pattern is invisible.
- **Synthesizer (REQ-107)**: `walkNode` recurses into the `value` attribute of the synthesized `*rm.DVDuration`, calls `makeChild` for the inner attribute's type (which is `STRING` — DV_Duration's primitive), and fails to attach (the new fix in PR #18's `writeDVTemporalValueSingle` succeeds, but the resulting tree never carries the constraint that the OPT pinned).

### REQ-107 UID emission (`openehr/instance/generate.go`)

```go
func newHierObjectID() rm.HierObjectID {  // returns VALUE
    ...
    return rm.HierObjectID{Value: uuid}
}
```

```go
// applyLocatableIdentity for *rm.Composition:
if v.UID == nil {
    id := newHierObjectID()
    v.UID = id  // assigns value to interface (UID is rm.ObjectID interface)
}
```

`rm.Composition.UID` is typed as `rm.ObjectID` (interface). Assigning a value (not pointer) makes the interface hold `rm.HierObjectID` (concrete value). `canjson` looks up polymorphic dispatch via `typereg.Default.Lookup` keyed on the Go type — it finds the `*HierObjectID` constructor but the marshaller uses a pointer-receiver `MarshalJSON` that's not callable on the value form. The result is canonical JSON with `Composition.uid` missing the `"_type":"HIER_OBJECT_ID"` discriminator that the decoder requires for the polymorphic field.

**Symptom**: PROBE-023's full unmarshal round-trip fails with a polymorphic-type-missing error on `uid`.

## Phases

### Phase 0 — Repro fixtures + failing tests

**Outcome:** Failing tests that pin both bugs against named fixtures; CI gate flips green once Phase 1 + 2 land.

**Tasks:**

1. **Wire-parser failing test** at `openehr/template/parse_primitives_test.go`:
   - Construct an inline OPT fragment with a `<children xsi:type="C_PRIMITIVE_OBJECT">` wrapping a `<item xsi:type="C_DURATION">` with a bounded range.
   - Parse, resolve the leaf `*ComplexObject`, assert `PrimitiveConstraint()` returns `constraints.CDuration{Range: ...}` (not nil).
   - Currently FAILS — pin as the regression gate.
2. **UID emission failing test** at `openehr/instance/instance_test.go`:
   - `Generate(...)` → assert returned `*rm.Composition.UID` is a **pointer** type (`*rm.HierObjectID`) so the canjson polymorphic dispatch fires.
   - Marshal via `canjson.Marshal` → assert the bytes contain `"_type":"HIER_OBJECT_ID"`.
   - Currently FAILS — pin as the regression gate.

**Definition of done:** Two new test cases exist and fail with clear diagnostics; nothing else changes.

### Phase 1 — Wire parser: extract `C_PRIMITIVE_OBJECT.<item>`

**Outcome:** `openehr/template/parse.go` recognises the wrapper shape and threads the inner primitive constraint through to the compiled `*ComplexObject`.

**Tasks:**

1. **`xmlCObject` shape extension** — add an `Item *xmlCObject` field bound to the `<item>` child element. Document that it is only populated when `Type == "C_PRIMITIVE_OBJECT"`.
2. **`buildPrimitive` dispatch** — add a `case "C_PRIMITIVE_OBJECT":` branch that delegates to `buildPrimitive(o.Item, strict)` when `o.Item != nil`. Return nil when missing (lenient mode) or `ErrInvalidOPT` when strict (the wrapper without an `<item>` is malformed).
3. **`buildComplexObject` flow** — confirm that when the wrapper carries an inner C_DURATION, the resulting `*ComplexObject` has `rm_type_name = "DURATION"` (from the wrapper) AND `primitive: constraints.CDuration{...}` (from the inner item). The walker then routes to the primitive-leaf branch.
4. **Tests** — the Phase 0 failing test goes green; add positive coverage for the other AOM 1.4 primitive wrappers that real OPTs use (`C_DATE`, `C_DATE_TIME`, `C_TIME`, `C_BOOLEAN` under C_PRIMITIVE_OBJECT). Match the pattern in `parse_primitives_test.go`.
5. **Validator round-trip** — extend PROBE-027 to run on `clinical_note.opt` (move from "vital_signs.opt only" to a fixture table). **Landed in PR #20** — `TestProbe027ClinicalNotePasses` passes via the generator-side `materialiseSingle` AOM-short-name fix + the EVENT_CONTEXT rmread gap fix. The default sentinel ("P0D" / etc.) satisfies the validator because the OPT-pinned constraint is dropped at parse time and the validator falls back to "is the attribute present at all". Once the wire-parser fix lands, the **constraint** flows through too and PROBE-027 stops depending on the sentinel fallback.

**Definition of done:**

- `go test ./openehr/template/...` green including the wire-parser pin.
- `go test ./testkit/probes/instance/...` green with PROBE-027 running on both vital_signs and clinical_note fixtures.
- `go run ./cmd/examples/generate-example/ --opt openehr/template/testdata/clinical_note.opt --policy=minimal --territory NL --composer-name "Test"` exits 0 with well-formed JSON.
- REQ-107's "Trust model — phasing" note in `clinical-modeling.md` updated: "PROBE-027 covers vital_signs.opt + clinical_note.opt; broader OPT coverage tracked under the SDK-internal compatibility matrix."

### Phase 2 — REQ-107 UID emission: return `*rm.HierObjectID`

**Outcome:** `openehr/instance.newHierObjectID()` returns a pointer; canjson's polymorphic marshalling produces a fully-discriminated `Composition.uid` byte stream; PROBE-023 widens to full unmarshal round-trip.

**Tasks:**

1. **Pointer return type** — change `newHierObjectID() rm.HierObjectID` → `newHierObjectID() *rm.HierObjectID`. Audit all call sites (`applyLocatableIdentity` per LOCATABLE that owns `uid: rm.ObjectID`); they currently assign value to interface — change to pointer assignment.
2. **`UIDSource` injection seam** — pre-existing TODO in REQ-107 plan. Add `Options.UIDSource func() *rm.HierObjectID` (nil → use crypto/rand). Test determinism follows naturally: tests can inject a counting generator. Document in `clinical-modeling.md` REQ-107 § Options.
3. **PROBE-023 widening** — extend `testkit/probes/composition/probe_023_builder_round_trip.go` to do the full marshal → unmarshal → path-equality assertion the spec calls for. Drop the byte-fragment-only assertion (it stays in the in-memory `TestBuilder_SetQuantity_systolic` for cheap regression coverage).
4. **Spec re-widening** — restore PROBE-023's normative wording in `conformance.md` to "marshal → unmarshal → values preserved at paths" and remove the "marshal-fragment parity (v1)" hedge.

**Definition of done:**

- `go test ./openehr/instance/...` green with the UID-shape pin.
- `go test ./testkit/probes/composition/...` green with the widened PROBE-023 doing real round-trips.
- `make ci` clean; `make spec-check` happy with the restored normative wording.
- `clinical-modeling.md` PROBE-023 row no longer carries the "v1 marshal-fragment parity" hedge.

## Cross-references

- [PR #18 review (REQ-107) — landed `aeeca12`](https://github.com/Cadasto/openehr-sdk-go/pull/18) — surface fix for `concreteFor` + `bmmSubtypes` + rmwrite writers; deferred Important 2 (PROBE-027 extension) cites this plan.
- [PR #19 review (REQ-101) — landed `789c6e8`](https://github.com/Cadasto/openehr-sdk-go/pull/19) — Important 2 (PROBE-023 spec ↔ implementation alignment) narrowed normative wording to "marshal-fragment parity (v1)" pending this plan.
- [`2026-05-24-template-instance-example-generator.md`](2026-05-24-template-instance-example-generator.md) §"Correctness contract" — the sound-not-complete property is preserved by this plan; no API surface changes.
- [`2026-05-21-composition-builder.md`](2026-05-21-composition-builder.md) §"v1 scope" — `Builder.Set` on multi-attribute container paths remains a separate v1 limitation tracked there.

## Not landed (search keywords)

When grepping for "what's left after PR #18 + #19":

- `concreteFor` — landed
- `bmmSubtypes` (AOM 1.4 short names) — landed
- `writeDVTemporalValueSingle` / `writeDVBooleanSingle` — landed
- `C_PRIMITIVE_OBJECT` wire-parser — **not landed** (this plan)
- `xmlCObject.Item` field — **not landed** (this plan)
- `newHierObjectID` returns pointer — **not landed** (this plan)
- `Options.UIDSource` test-determinism hook — **not landed** (this plan)
- PROBE-027 on `clinical_note.opt` — **not landed** (waits on this plan)
- PROBE-023 full unmarshal round-trip — **not landed** (waits on this plan)

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0: failing tests pin both bugs | |
| Phase 1.1: `xmlCObject.Item` field | |
| Phase 1.2: `buildPrimitive` C_PRIMITIVE_OBJECT branch | |
| Phase 1.3: positive coverage tests (C_DATE / C_BOOLEAN / etc. wrappers) | |
| Phase 1.4: PROBE-027 extension to clinical_note.opt | |
| Phase 2.1: `newHierObjectID` returns `*rm.HierObjectID` | |
| Phase 2.2: `Options.UIDSource` seam | |
| Phase 2.3: PROBE-023 full unmarshal round-trip | |
| Phase 2.4: `conformance.md` PROBE-023 wording restored | |
| `make ci` green | |
