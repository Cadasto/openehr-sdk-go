# Plan — REQ-100 template parser follow-ups (post-#10)

**Date:** 2026-05-22
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-100 (hardening); PROBE-022 (breadth)
**Implementation:** planned
**Depends on:** [2026-05-21-template-parser.md](2026-05-21-template-parser.md) (PR #10 landing)
**Defers:** REQ-101 composition builder (consumes hardened paths); AOM 2 / OET

## Goal

Close gaps identified in PR #10 review **without** blocking merge of the initial REQ-100 landing. Work in small PRs or commits on `main` after #10 merges; follow this checklist in order.

**Do not duplicate** the fix-now items called out in PR #10 review comments (error `%w` chain, `Root()` spec/godoc, stdlib-only dependency text, fixture README, `opt-parse` example errors).

## When to run

| Trigger | Action |
|---|---|
| After PR #10 merges | Open follow-up branch `feat/template-req100-followups` from `main` |
| Before REQ-100 `implementation: landed` in traceability | Complete **Phase 1** below |
| Before composition builder Phase 1 | Complete **Phase 2** path/slot coverage |

## Phase 1 — Tests and traceability honesty

**Outcome:** REQ-100 test surface matches normative claims; traceability `landed` is defensible.

**Tasks:**

1. **`TestNodeAt_PredicateAtCode`** — `path_test.go`: resolve a stable at-code from `vital_signs.opt` via `NodeAt` (not only `ParsePath` accept table).
2. **`TestParseFile_VitalSigns_ContainsSlot`** — find `*template.Slot` in tree; assert `Includes()` non-empty where fixture has `<includes>`.
3. **`TestNodeAt_CannotDescendSlot`** — path into slot child → `errors.Is(err, ErrPathNotFound)`.
4. **`TestParseFile_ClinicalNote_Path`** — at least one deep `/content/...` assertion on `clinical_note.opt`.
5. **`TestParseOPT_UnsupportedAttributeType`** — minimal XML with unknown attribute `xsi:type`; assert `errors.Is(..., ErrUnsupportedNode)` **after** parent PR fixes `%w` in `parse.go`.
6. **PROBE-022** — extend `probes_test.go` assertions: one at-code path; optional second fixture body for `clinical_note.opt`.
7. **Align status labels** — pick one ladder and sync `traceability.yaml`, `roadmap.md`, and template parser plan header:
   - Recommended: `partial` until Phase 1 done; `landed` only when tests + spec edits complete.
8. **`conformance.md`** — add coverage-matrix row: Clinical modeling / PROBE-022 → `testkit/probes/template/`.

**Definition of done:** `make ci` green; REQ-100 `implementation: landed` in `traceability.yaml` only if Phase 1 complete.

## Phase 2 — API hardening (optional strict modes)

**Outcome:** Safer defaults for production callers; no breaking change to default parse behavior.

**Tasks:**

1. **Getter immutability** — `Attributes()`, `Children()`, `Includes()`, `Excludes()` return `slices.Clone` of internal slices (or document copy-on-read in godoc if semver prefers deferral).
2. **`NodeKind()`** — add `NodeKind` iota + `func (n Node) Kind() NodeKind` on sealed implementations to reduce consumer type switches.
3. **`TrimSpace` on `ArchetypeID()`** — parse path in `buildArchetypeRoot` / promotion branch (`parse.go`).
4. **Unknown child `xsi:type` with children** — choose and document:
   - **A)** recurse via `buildComplexObject` when attributes present, or
   - **B)** return `ErrUnsupportedNode` when unknown type has nested XML, or
   - **C)** add `ParseOPTStrict(...)` option (default remains forward-compatible leaf).
   Update REQ-100 § Node taxonomy accordingly.
5. **Trailing XML** — after `Decode`, reject non-whitespace tokens until EOF (`ErrInvalidOPT`).
6. **BOM handling** — propagate `Peek`/`Discard` errors as `ErrInvalidOPT` wrap.
7. **`ParseFile` I/O** — wrap `os.Open` with context path; preserve `fs.ErrNotExist` via `%w`.
8. **Parse edge tests** — `ParseOPT(nil)`, non-`<template>` root, `.OPT` extension case-insensitive acceptance.
9. **Defensive xsi:type namespace anchor** — change struct tags on `xmlCObject.Type` and `xmlCAttribute.Type` from `xml:"type,attr"` to `xml:"http://www.w3.org/2001/XMLSchema-instance type,attr"`. The current tag works correctly for all valid OPTs (the only `type` attribute on `<attributes>` / `<children>` is `xsi:type`) but anchoring to the XSI namespace removes a speculative future-mismatch risk surfaced by the PR #10 self-review.

**Definition of done:** New tests for each behavior; CHANGELOG bullet only if public API adds options/types.

### Additional test gaps from PR #10 self-review (not yet in Phase 1)

10. **`TestParseOPT_InvalidXML_UnwrapsXMLError`** — assert `var se *xml.SyntaxError; errors.As(err, &se)` reaches the inner decoder error through the double-`%w` wrap. Regression to single-`%w` would silently break callers using line/column diagnostics.
11. **`TestParsePath_RejectsCharAfterCloseBracket`** — `/content[at0001]extra` must fail with `ErrPathSyntax`. The branch at `path.go:107-109` is currently unexercised.
12. **`TestNodeAt_LeafMidPath`** — synthetic OPT with two-level path through a leaf `*ComplexObject` that has no attributes — exercises the "cannot descend" branch in `walkPath` (distinct from the `*Slot` descent case already in Phase 1 task 3).
13. **`TestParseOPT_AcceptsBOM` cleanup** — current test reads `os.ReadFile` and discards the result (`_ = bytes`). Either parse the bytes for dual-prove or drop the read.
14. **`TestPathAssertion_PrecedenceContradiction`** — PROBE-022 `PathAssertion` with both `ExpectNotFound: true` and `WantRMType: "X"` — document/test which wins; today the negative-path short-circuit hides the contradiction.

## Phase 3 — Ergonomics (before REQ-101)

**Outcome:** Composition builder consumers hit fewer footguns.

**Tasks:**

1. **`ErrAmbiguousPath`** (new sentinel) — when predicate-less segment has `len(children) > 1`, or duplicate predicate match; optional `WithStrictPaths()` on `OperationalTemplate` resolution (default: current first-child rule per REQ-100).
2. **`ValidatePath(p Path) error`** — optional walk that checks segment names exist on tree (today `ParsePath` is syntax-only).
3. **`Multiplicity` validation** — reject `lower > upper` at parse time if both set (or document opaque interval until validation REQ). Field encapsulation landed in PR #10 self-review fix.
4. **`Attribute` in `Node` interface — category-error fix.** `Attribute`'s `RMTypeName()` / `NodeID()` are forced to `""` because attributes are not RM-typed and carry no archetype node id. Two cleaner shapes: (a) split `Node` into `ObjectNode` (RM-typed) and `AttributeNode` (named), or (b) keep one `Node` interface but move `RMTypeName/NodeID` off the interface onto concrete object types and have callers type-switch. Either removes the always-empty methods. Today `NodeAt` cannot return an `*Attribute`, so the cost is conceptual + future evolution friction.
5. **`Root() Node` union collapse.** Today `Root()` returns either `*ComplexObject` or `*ArchetypeRoot`, forcing callers to type-switch on two shapes. Consider storing `*ComplexObject` directly and lifting `archetypeID` to an optional `OperationalTemplate.RootArchetypeID() string` accessor. Smaller mental model for callers.
6. **`Cardinality` ergonomics** — add `String() string` and `IsValid() bool` methods. Today `Cardinality(42)` is constructible and the zero value coincides with `Single`; both are correct but diagnostics would benefit from a stringer.
7. **`Attribute.children []Node` typing** — only `*ComplexObject | *ArchetypeRoot | *Slot` can appear there; `*Attribute` cannot. Either document this invariant in the `Children()` godoc, or fold into the `ObjectNode` split above.

## Phase 4 — Recursive traversal + consumer-driven extensions (before REQ-101 / REQ-102)

**Outcome:** Composition builder (REQ-101) and validation (REQ-102) can walk the OPT tree without rewriting visitor boilerplate, and reach the constraint detail their use cases require.

### What works today (no change needed)

Every node in the definition tree is reachable via the existing public API. A hand-rolled depth-first visitor is ~15 lines:

```go
func Walk(n template.Node, visit func(template.Node)) {
    visit(n)
    switch v := n.(type) {
    case *template.ArchetypeRoot:
        for _, a := range v.Attributes() { Walk(a, visit) }
    case *template.ComplexObject:
        for _, a := range v.Attributes() { Walk(a, visit) }
    case *template.Attribute:
        for _, c := range v.Children()   { Walk(c, visit) }
    case *template.Slot:
        // leaf
    }
}
```

So the **shape** of the tree is fully traversable. What's missing is ergonomic helpers and **constraint payload depth** at the leaves.

### 4.1 Walker helpers (low-risk, can land independently)

**Tasks:**

1. **`template.Walk(n Node, visit func(Node) error) error`** — depth-first visitor; returns the first non-nil error from `visit`. Sentinel `template.SkipSubtree` (returned from `visit`) skips children but continues siblings.
2. **`template.WalkPath(opt *OperationalTemplate, visit func(Path, Node) error) error`** — path-aware variant. `Path` accumulates as the walker descends; predicates are added per archetype-root / node-id boundary so consumers can use the path as a stable key.
3. **`ObjectNode` interface** — supertype of `*ComplexObject` and `*ArchetypeRoot` exposing `Attributes() []*Attribute`. Lets visitors collapse the two type-switch arms. Pairs naturally with Phase 3 task 4 (Attribute category-error fix).

**Open questions** (resolve when REQ-101 implementation begins, not before):

- Does `WalkPath` predicate every archetype-root segment (`/content[openEHR-EHR-OBSERVATION...]`) or every node-id (`/content[at0001]`)? Two different stable-key strategies, FLAT-format vs at-code-keyed; consumer call sites will pick.
- Does `visit` receive parent context (e.g. `(Path, Node, parent ObjectNode)`)? Validator likely yes; composition builder maybe not.

### 4.2 Primitive constraint payload — gap blocking REQ-102

Today v1 leaf nodes (`C_PRIMITIVE_OBJECT`, `C_CODE_PHRASE`, `C_DV_QUANTITY`, `C_DV_ORDINAL`) collapse to a bare `*ComplexObject` carrying only the RM type name string. The constraint detail in the OPT XML is **silently dropped**.

This blocks:

- **REQ-102 validation:** "the systolic value MUST be 0..300 mmHg" — the magnitude range and unit list live in `<children xsi:type="C_DV_QUANTITY">` payload which is not currently parsed.
- **REQ-101 composition builder defaults:** "this OPT defaults the language to en" — `default_value` blocks are not parsed.

**Candidate REQs to file when REQ-101 / REQ-102 begin:**

- **REQ-103 (proposed):** Primitive constraint introspection — extend the `Node` taxonomy with typed leaves (`PrimitiveObject` with method `Constraints() PrimitiveConstraint`) carrying union types for `DV_QUANTITY` (units, magnitude range, precision), `CODE_PHRASE` (terminology id, code list), `C_STRING` (pattern, allowed values), etc.
- **REQ-104 (proposed):** Slot assertion grammar — parse `archetype_id matches {...}` assertion expressions in `*Slot.Includes()` / `*Slot.Excludes()` into a typed predicate AST. Today these are raw text.
- **REQ-105 (proposed):** Terminology bindings (`<term_definitions>` and `<term_bindings>` blocks) — parsed metadata that maps at-codes to display text and external terminology codes. Required for clinical-display tooling and AQL-result rendering, but not for parse/path basics.

**Design choice:** Where do REQ-103 leaves live?

- **Option A:** Extend `openehr/template/` with typed primitive leaves. Keeps the parser self-contained; grows the public surface.
- **Option B:** Add a sibling package `openehr/template/constraints/` that re-walks the OPT XML for primitive payload only. Keeps the v1 surface stable; adds a second pass over the bytes.
- **Option C:** Bring back `openehr/aom/aom14/` for primitive constraint types only (selective import). Reuses generated code but reintroduces the dependency this plan deliberately avoided.

Pick when REQ-102 design begins and the validator's actual access pattern is clear.

### 4.3 What REQ-101 / REQ-102 will and will not get from this package

| Need | Current state | Gap closed in |
|---|---|---|
| Walk every node | ✅ via hand-rolled visitor | helper landing in Phase 4.1 |
| RM type at any path | ✅ `node.RMTypeName()` | — |
| at-code at any path | ✅ `node.NodeID()` | — |
| Archetype id at archetype root | ✅ `ar.ArchetypeID()` | — |
| Cardinality (single / multiple) on attributes | ✅ `attr.Cardinality()` | — |
| Existence / occurrences intervals | ✅ `attr.Existence()` / `co.Occurrences()` | — |
| **Primitive constraint payload** (units, ranges, code lists) | ❌ leaf-only | REQ-103 (proposed) |
| **Slot assertion AST** | ⚠️ raw strings via `slot.Includes()` | REQ-104 (proposed) |
| **Terminology bindings** | ❌ not parsed | REQ-105 (proposed) |
| **Default values** | ❌ not parsed | REQ-103 or REQ-101 design |

### 4.4 Sequencing

1. Phase 4.1 walker helpers **MAY** land before REQ-101 begins, or alongside the first REQ-101 implementation PR. Low risk, additive only.
2. REQ-103 / REQ-104 / REQ-105 **SHOULD NOT** be authored speculatively — defer until REQ-101 (composition builder) or REQ-102 (validation) call sites reveal which constraint detail is actually consumed and at what call-frequency.
3. The walker helper signatures **MUST** be informed by real REQ-101 / REQ-102 implementation usage, not guessed in isolation. Land them in the same PR as the first consumer.

## Out of scope (this plan)

- OET parse; ADL 2 OPT; Archie linker; terminology expansion (unchanged REQ-100 v1 bounds).
- Importing `openehr/aom/aom14/` into parser (defer until constraint payloads are needed for REQ-102 validation; see Phase 4.2 Option C).

## Implementation checklist

| Step | Status |
|---|---|
| Phase 1 tests + PROBE-022 breadth | |
| Traceability `landed` + conformance matrix row | |
| Phase 2 immutability / strict parse (if adopted) | |
| Phase 3 ambiguity / ValidatePath (if needed for composition) | |
| Phase 4.1 walker helpers (alongside first REQ-101 consumer PR) | |
| REQ-103 / REQ-104 / REQ-105 spec drafts (only when REQ-101 / REQ-102 surface real call sites) | |
| `make ci` | |

## Mapping to specs

- [docs/specifications/clinical-modeling.md § REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths)
- [docs/specifications/conformance.md § PROBE-022](../specifications/conformance.md)
