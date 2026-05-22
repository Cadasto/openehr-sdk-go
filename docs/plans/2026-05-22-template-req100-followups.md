# Plan — REQ-100 template parser follow-ups and clinical-modeling foundation

**Date:** 2026-05-22
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-100 (hardening); PROBE-022 (breadth); foundation for REQ-101 (composition builder), REQ-102 (validation), REQ-103 (primitive constraints), REQ-104 (slot assertions), REQ-105 (terminology bindings)
**Implementation:** planned
**Depends on:** [2026-05-21-template-parser.md](2026-05-21-template-parser.md) (PR #10 landing — done)
**Defers:** AOM 2 / ADL 2; OET parse; remote slot-fill repository; JSON-format simplified template export (separate plan — [2026-05-22-webtemplate-export.md](2026-05-22-webtemplate-export.md))

## Goal

Take the v1 OPT parser delivered by REQ-100 from a wire-decoder to a **composition-modelling foundation** capable of supporting the composition builder (REQ-101), template validation (REQ-102), and structured primitive constraint introspection (REQ-103). Work in small incremental PRs on `main`; preserve building-block independence (REQ-013) throughout.

### Normative reference

The Ocean / openEHR-standard OPT XSD: [`specifications.openehr.org/releases/ITS-XML/Release-2.0.0/components/AM/Release-1.4/Template.xsd`](https://specifications.openehr.org/releases/ITS-XML/Release-2.0.0/components/AM/Release-1.4/Template.xsd) (+ companion `OpenehrProfile.xsd`).

Key findings from the XSD confirmed during planning:

- The `<definition>` element is **always** a `C_ARCHETYPE_ROOT` (extends `C_COMPLEX_OBJECT` with `archetype_id`, `template_id`, `term_definitions`, `term_bindings`).
- `T_COMPLEX_OBJECT` extends `C_COMPLEX_OBJECT` with an optional `default_value` payload.
- Top-level OPT carries `language`, `description`, `revision_history`, `uid`, `template_id`, `concept`, `definition`, `ontology`, `component_ontologies`, `annotations`, `constraints`, `view`. Of these the v1 parser today reads only `language`, `template_id`, `concept`, `uid`, `definition`; the rest are deliberate Phase 2 captures.
- The constraint subtree uses `xsi:type` to discriminate `C_COMPLEX_OBJECT` / `C_ARCHETYPE_ROOT` / `ARCHETYPE_SLOT` / primitive subtypes (`C_DV_QUANTITY`, `C_DV_ORDINAL`, `C_CODE_PHRASE`, `C_STRING`, `C_INTEGER`, `C_REAL`, `C_BOOLEAN`, `C_DATE`, `C_TIME`, `C_DATE_TIME`, `C_DURATION`).

## When to run

| Trigger | Action |
|---|---|
| Now (PR #10 merged) | Work in this branch `feat/template-req100-followups` |
| Before REQ-100 `implementation: landed` | Complete **Phase 1** |
| Before composition builder (REQ-101) Phase 1 | Complete **Phases 4 + 5 + RMInfoLookup** |
| Before validation (REQ-102) Phase 1 | Complete **Phase 6** (REQ-103 primitives) |

## Phase 1 — Tests and traceability honesty

**Outcome:** REQ-100 test surface matches normative claims; traceability `landed` is defensible.

**Tasks:**

1. **`TestNodeAt_PredicateAtCode`** — `path_test.go`: resolve a stable at-code from `vital_signs.opt` via `NodeAt` (not only `ParsePath` accept table).
2. **`TestParseFile_VitalSigns_ContainsSlot`** — find `*template.Slot` in tree; assert `Includes()` non-empty where fixture has `<includes>`.
3. **`TestNodeAt_CannotDescendSlot`** — path into slot child → `errors.Is(err, ErrPathNotFound)`.
4. **`TestParseFile_ClinicalNote_Path`** — at least one deep `/content/...` assertion on `clinical_note.opt`.
5. **`TestParseOPT_UnsupportedAttributeType`** — minimal XML with unknown attribute `xsi:type`; assert `errors.Is(..., ErrUnsupportedNode)`.
6. **PROBE-022** — extend `probes_test.go` assertions: one at-code path; optional second fixture body for `clinical_note.opt`.
7. **`TestParseOPT_InvalidXML_UnwrapsXMLError`** — assert `var se *xml.SyntaxError; errors.As(err, &se)` reaches the inner decoder error through the double-`%w` wrap.
8. **`TestParsePath_RejectsCharAfterCloseBracket`** — `/content[at0001]extra` must fail with `ErrPathSyntax`.
9. **`TestNodeAt_LeafMidPath`** — synthetic OPT with two-level path through a leaf `*ComplexObject` that has no attributes — exercises the "cannot descend" branch in `walkPath`.
10. **`TestParseOPT_AcceptsBOM` cleanup** — drop the dead `os.ReadFile` read, or use it for dual-prove.
11. **`TestPathAssertion_PrecedenceContradiction`** — PROBE-022 `PathAssertion` with both `ExpectNotFound: true` and `WantRMType: "X"`; document/test precedence.
12. **Align status labels** — sync `traceability.yaml`, `roadmap.md`, plan header to `partial` until Phase 1 done; `landed` only when tests + spec edits complete.
13. **`conformance.md`** — add coverage-matrix row: Clinical modeling / PROBE-022 → `testkit/probes/template/`.

**Definition of done:** `make ci` green; REQ-100 `implementation: landed` in `traceability.yaml`.

## Phase 2 — Parser hardening

**Outcome:** Safer defaults for production callers; no breaking change to default parse behaviour.

**Tasks:**

1. **Getter immutability** — `Attributes()`, `Children()`, `Includes()`, `Excludes()` return `slices.Clone` of internal slices (or document copy-on-read in godoc if semver prefers deferral).
2. **`TrimSpace` on `ArchetypeID()`** — parse path in `buildArchetypeRoot` / promotion branch (`parse.go`).
3. **Unknown child `xsi:type` with children — strict mode option.** Choose and document:
   - **A)** recurse via `buildComplexObject` when attributes present (lossy-leaf becomes lossy-subtree silently), or
   - **B)** return `ErrUnsupportedNode` when unknown type has nested XML (loud failure), or
   - **C)** add `ParseOPTStrict(...)` opt-in (default remains forward-compatible leaf).
   Recommendation: **C** — keeps default safe for forward compatibility, adds opt-in strictness for production validators.
4. **Trailing XML** — after `Decode`, reject non-whitespace tokens until EOF (`ErrInvalidOPT`).
5. **BOM handling** — propagate `Peek`/`Discard` errors as `ErrInvalidOPT` wrap.
6. **Parse edge tests** — `ParseOPT(nil)`, non-`<template>` root, `.OPT` extension case-insensitive acceptance.
7. **Defensive xsi:type namespace anchor** — change struct tags on `xmlCObject.Type` and `xmlCAttribute.Type` from `xml:"type,attr"` to `xml:"http://www.w3.org/2001/XMLSchema-instance type,attr"`.
8. **Annotations capture** — OPTs carry `<annotations path="...">` for UI-side hints. Currently discarded; capture as `OperationalTemplate.Annotations() map[Path][]Annotation` for editor tooling consumers. Low risk, additive only.
9. **`integrity_checks` / `revision_history` / `description`** — currently discarded; capture top-level metadata so consumers can audit OPT provenance. Optional metadata methods on `OperationalTemplate`.

**Definition of done:** New tests for each behaviour; CHANGELOG bullet only if public API adds options/types.

## Phase 3 — Path resolution ergonomics

**Outcome:** Composition builder consumers hit fewer footguns on unambiguous path resolution.

**Tasks:**

1. **`ErrAmbiguousPath`** (new sentinel) — when predicate-less segment has `len(children) > 1`, or duplicate predicate match; optional `WithStrictPaths()` on `OperationalTemplate` resolution (default: current first-child rule per REQ-100).
2. **`ValidatePath(p Path) error`** — optional walk that checks segment names exist on tree (today `ParsePath` is syntax-only).
3. **`Multiplicity` validation** — reject `lower > upper` at parse time if both set.
4. **`Attribute` in `Node` interface — category-error fix.** Two cleaner shapes:
   - **(a) ObjectNode/AttributeNode split** — recommended.
   - (b) Move `RMTypeName/NodeID` off the interface onto concrete object types.
   `ObjectNode` supertype enables walker `case ObjectNode:` to collapse `*ComplexObject` + `*ArchetypeRoot` arms.
5. **`Root() Node` union collapse.** Store `*ComplexObject` directly; lift `archetypeID` to `OperationalTemplate.RootArchetypeID() string`. Smaller mental model.
6. **`Cardinality` ergonomics** — add `String() string` and `IsValid() bool`.
7. **`Attribute.children []Node` typing invariant** — document `*ComplexObject | *ArchetypeRoot | *Slot` only (fold into 4 if adopted).

## Phase 4 — Compiled template (the foundation layer)

**Outcome:** A pre-processed, walker-friendly representation that the composition builder and validator can consume without re-traversing raw OPT XML each call.

### Rationale

The raw `OperationalTemplate` produced by `ParseOPT` is faithful to the wire shape, but downstream consumers (composition builder, validator, example generator) repeatedly need information that is implicit in the wire form:

- **Stable AQL paths** for every node — computing them per visit is wasteful and a non-trivial source of subtle bugs (segment quoting, predicate normalisation).
- **Implicit RM attributes** the OPT omits but the RM declares as required. The OPT often says "this composition contains an observation", and downstream code needs to know the composition also requires `category`, `language`, `territory`, `composer`, `context` — none of which the OPT mentions explicitly.
- **Flattened term bindings + term definitions** — the raw OPT scatters these across `<term_definitions>` and `<term_bindings>` ontology blocks. Walkers want a per-node-id lookup keyed by language.
- **Normalised primitive constraints** — wire-side `<c_dv_quantity>` / `<c_code_phrase>` / `<c_string>` XML shapes are awkward to validate against; downstream code wants typed constraint values (REQ-103).
- **Defaults / assumed values** captured as a structured payload rather than raw XML.

This is a well-established two-layer pattern: a faithful wire decoder feeding a compiled, walker-friendly representation. The two should not be conflated — different consumers want different things from each.

### Design

Introduce `template.Compiled` as a new exported type. The raw `OperationalTemplate` and its `Node` taxonomy stay as the wire-side representation; `template.Compile(*OperationalTemplate, CompileOption...) (*Compiled, error)` produces the foundation type.

```go
type Compiled struct {
    // identity
    TemplateID()       string
    Concept()          string
    UID()              string
    Language()         string
    DefaultLanguage()  string
    Languages()        []string

    // tree
    Root()             *CompiledNode

    // O(1) lookup
    NodeAt(p Path)     (*CompiledNode, error)   // memoised; equivalent to walkPath but cached
    AllByRMType(rm string)   []*CompiledNode
    AllByNodeID(at string)   []*CompiledNode

    // ontology / term bindings
    Terms()            ArchetypeOntology         // per-language definitions
    TermBindings()     map[NodeRef][]TermBinding // flattened, keyed by node-id
}

type CompiledNode struct {
    AQLPath()          Path                      // computed once; stable for caching
    RMTypeName()       string
    NodeID()           string                    // at-code if present; archetype id at root
    ArchetypeID()      string                    // non-empty for archetype-root nodes
    Occurrences()      (Multiplicity, bool)
    Existence()        (Multiplicity, bool)
    Cardinality()      Cardinality
    DefaultValue()     []byte                    // raw default_value XML; consumer decides parse
    Attributes()       []*CompiledAttribute      // OPT-declared + implicit-RM-injected
    PrimitiveConstraint() PrimitiveConstraint    // nil for non-leaf nodes; typed leaf otherwise (see REQ-103)
    IsSlot()           bool
    SlotIncludes()     []SlotAssertion           // raw + parsed (when REQ-104 lands)
    Term(code, lang string) *ArchetypeTerm       // per-language term lookup
    Parent()           *CompiledNode              // back-pointer for walker context
}
```

### Tasks

1. **`internal/templatecompile/`** package with the `Compile` function (kept internal until the surface is stable; expose via `template.Compile` once REQ-101/102 confirm the right shape).
2. **AQL path computation** — single recursion over the raw tree assigning each node a `Path` value:
   - Container attributes (multiple): `/content[archetype-id]` or `/content[at-code]` segment.
   - Single attributes: `/data` segment without predicate.
   - Root path is `/` (matches REQ-100).
3. **Implicit RM attribute injection** — see Phase 4-bis (RMInfoLookup) below. The compile step calls `RMInfoLookup.RequiredAttributes(rmType)` to materialise placeholder `CompiledAttribute` entries for required RM fields the OPT omits.
4. **Term-binding flattening** — walk `<term_definitions>` and `<term_bindings>` ontology blocks; emit per-node maps keyed by archetype node id. Per-language definitions stored once on the `Compiled` aggregate, indexed by node id.
5. **`AllByRMType` / `AllByNodeID` indexes** — build during compile; constant-time lookup for validators and example-generators.
6. **`Compile` is pure** — no I/O, no mutation of the input `*OperationalTemplate`. Re-callable.

### Open questions

- Should `Compiled` be the **primary** public API (with `OperationalTemplate` as wire-only), or sit alongside it? Decision deferred to first REQ-101 implementation PR (when call-site preferences emerge).
- The compiled tree is internal infrastructure — distinct from any JSON-format simplified template representation used for UI / form-generation consumption. The latter is a serialisation concern with its own integration plan: [2026-05-22-webtemplate-export.md](2026-05-22-webtemplate-export.md).

## Phase 4-bis — RM info lookup (foundation for Phase 4)

**Outcome:** A small helper that answers "what attributes does RM class X require / allow?" — needed by the compiled-template implicit-attribute injection and by the composition skeleton builder.

### Design

```go
// openehr/rm/rminfo/  (or as a sub-package — TBD; depends on building-block weight)
package rminfo

type Lookup interface {
    // RequiredAttributes returns the names of attributes the RM declares as mandatory
    // on the given RM type (e.g. for COMPOSITION: category, language, territory, composer,
    // context, content).
    RequiredAttributes(rmType string) []string

    // AttributeRMType returns the RM type of an attribute on a parent type.
    // (e.g. AttributeRMType("OBSERVATION", "data") = "HISTORY").
    AttributeRMType(parentRMType, attrName string) (string, bool)

    // IsContainer reports whether the attribute is multi-valued (list / set).
    IsContainer(parentRMType, attrName string) (bool, bool)

    // KnownRMTypes returns all RM class names this lookup recognises (for diagnostics).
    KnownRMTypes() []string
}
```

### Tasks

1. **Codegen path** — extend `internal/bmmgen/` to emit a `Lookup` implementation from the pinned BMM. The BMM already carries this exact information (attribute lists per class with cardinalities). Output: `openehr/rm/rminfo/lookup_gen.go` shipping a `Default Lookup` plus a `New` for testing.
2. **Conformance** — `RMInfoConformance` probe (PROBE-023 candidate) verifying `Default.RequiredAttributes("COMPOSITION")` contains `{category, language, territory, composer, context, content}` against a golden BMM corpus.
3. **Building-block weight** — the codegen output is single-file, dependency-free Go (no reflection, no BMM runtime). Acceptable for `openehr/template/` to import.

## Phase 5 — Walker pattern

**Outcome:** A single visitor abstraction that composition builder (REQ-101), validator (REQ-102), example generator, and serialisation walkers all share.

### Design

```go
// openehr/template/walk/
package walk

// Visitor receives each node twice — pre-order and post-order.
// PreHandle returns SkipSubtree to skip children but continue siblings;
// any other non-nil error aborts the walk.
type Visitor interface {
    PreHandle(ctx *Context) error
    PostHandle(ctx *Context) error
}

// SkipSubtree is the sentinel returned from PreHandle to skip children.
var SkipSubtree = errors.New("walk: skip subtree")

// Context carries the parallel stacks. Generics for the accumulator T
// follow once the first consumer (REQ-101) confirms the shape.
type Context struct {
    Node()     *template.CompiledNode
    Parent()   *template.CompiledNode
    Path()     template.Path
    Depth()    int
    // RM context (composition walks only)
    RMObject() any
    RMPath()   string
}

func Walk(c *template.Compiled, v Visitor) error
func WalkSubtree(c *template.Compiled, start template.Path, v Visitor) error
```

### Tasks

1. **Package `openehr/template/walk/`** — separate from `openehr/template/` to keep the core package import surface lean.
2. **`Walk` + `WalkSubtree`** — depth-first, pre + post hooks, `SkipSubtree` sentinel.
3. **Composition-side variant** — `WalkComposition(c *template.Compiled, comp *rm.Composition, v CompositionVisitor) error` walks both trees in lockstep. Internal `Context` tracks both the OPT node and the RM object at each step. (Required for validator.)
4. **Two reference implementations** — short example visitors landed alongside the walker:
   - `templatedump.Walker` — pretty-prints the compiled tree (replaces ad-hoc loops in `opt-parse` example).
   - `templatedump.PathCollector` — accumulates all AQL paths to a `[]Path`.
5. **Choice handling** — when a `CompiledNode` has multiple-RM-type children at the same path (e.g. `DV_TEXT | DV_CODED_TEXT`), the walker exposes them as a `Choice` group on the `Context`. The visitor decides which branch to descend.

### Open questions

- Generic accumulator (`Context[T]`) — wait for REQ-101 / REQ-102 to confirm whether all consumers can use the same `T` shape.
- Should the walker abort on first error or collect all errors? Validators want collect-all; serialisation wants fail-fast. Provide both as `Walk` / `WalkUntilError`.

## Phase 6 — REQ-103 primitive constraint introspection

**Outcome:** Validators can check primitive constraints (DV_QUANTITY ranges, CODE_PHRASE code lists, C_STRING patterns) without re-parsing OPT XML. This is the **REQ-102 prerequisite**.

### Design

Introduce a new sealed interface `PrimitiveConstraint` exposed on `CompiledNode.PrimitiveConstraint()`. Implementations correspond 1:1 with OPT XSD primitive types:

```go
// openehr/template/constraints/  (new sub-package)

type PrimitiveConstraint interface {
    isPrimitive()
    Validate(value any) []Violation       // nil = pass
}

type DvQuantity struct {
    Units    []QuantityUnit                // (units string, magnitude range, precision range)
    Property string                        // optional terminology binding for the property
    Default  *DvQuantityDefault            // assumed_value when present
}

type CodePhrase struct {
    Terminology string                     // "openehr" | "snomed-ct" | ...
    CodeList    []string                   // empty => external; populated => closed list
    External    bool                       // true when only the terminology is constrained
}

type CDvOrdinal struct {
    Values []OrdinalValue                  // (value int, symbol CodePhrase, terminology)
}

type CString struct {
    Pattern   string                       // regex (POSIX-like)
    List      []string                     // allowed strings (closed list)
    Default   string
}

type CBoolean   struct{ TrueValid, FalseValid bool; Default *bool }
type CInteger  struct{ Range NumericRange; List []int }
type CReal     struct{ Range NumericRange; List []float64 }
type CDate     struct{ Pattern string }    // ISO 8601 partial-date pattern
type CTime     struct{ Pattern string }
type CDateTime struct{ Pattern string }
type CDuration struct{ Pattern string }    // PnYnMnDTnHnMnS pattern

type NumericRange struct {
    Lower, Upper       float64
    LowerInclusive     bool
    UpperInclusive     bool
    LowerUnbounded     bool
    UpperUnbounded     bool
}
```

`Violation` is a typed payload — `Code` (e.g. `out_of_range`, `pattern_mismatch`, `not_in_list`), `Detail` string, optional `Path Path`.

### Tasks

1. **Spec REQ-103** in `docs/specifications/clinical-modeling.md` — new top-level requirement defining the constraint interface, the closed-set of primitive types, the `Validate` contract, and out-of-scope items.
2. **Wire parse extension** — extend `parse.go` to recognise primitive `xsi:type` values; decode XML payload into the wire structs; store on the `CompiledNode` during compile.
3. **`Validate(value any)` per type** — pure functions; no reflection. Pattern type-switches over expected RM Go types (`*rm.DvQuantity`, `*rm.DvCodedText`, etc. when REQ-102 wires it up; for now type-switch over `int64`, `float64`, `string`, `time.Time`).
4. **PROBE-024** (proposed) — sandbox probe: given a fixture OPT + a node path + a sample value, assert `Validate` returns the expected violation set.
5. **CHANGELOG** bullet under `### Added` once landed.

### Out of scope (this REQ)

- ARCHETYPE_SLOT assertion parsing (REQ-104).
- External terminology lookup (REQ-105).
- AOM 2 / ADL 2 `tuple_constraint` — not used by ADL 1.4.

## Phase 7 — REQ-104 slot assertion grammar

**Outcome:** Validators can determine whether a candidate archetype satisfies a slot's `includes` / `excludes` assertions, instead of falling back to RM-type prefix match.

### Background

The OPT XSD exposes slot assertions as XML expression trees:

```xml
<archetype_slot rm_type_name="OBSERVATION" node_id="at0002">
  <includes>
    <expression><value>archetype_id matches {/openEHR-EHR-OBSERVATION\.body_weight\..*/}</value></expression>
  </includes>
</archetype_slot>
```

A pragmatic compromise widely adopted in practice: validation falls back to "the candidate archetype id must start with `openEHR-EHR-<rmType>.`". The full assertion grammar can be wired in later when consumers demand stricter slot-fit checking.

### Tasks (deferred until REQ-101 / REQ-102 surface a concrete use case)

1. **Spec REQ-104** documenting the assertion grammar subset to be supported (initially just `archetype_id matches {regex-list}`).
2. **`SlotAssertion` typed AST** in `openehr/template/constraints/` with `MatchesArchetypeID(string) bool`.
3. **Parse** the expression sub-tree at compile time; cache the compiled regex per slot.
4. **Pragmatic default** — until REQ-104 lands, expose `Slot.RawIncludes() []string` (current behaviour) **and** add `Slot.AllowsRMType(rm string) bool` implementing the RM-type-prefix fallback. Validators use the prefix fallback unless a structured AST is available.

## Phase 8 — REQ-105 terminology bindings

**Outcome:** Consumers can resolve archetype-node-id (`at0001`) to display text in any of the OPT's languages, and follow `term_bindings` to external terminologies (SNOMED, LOINC, ICD-10).

### Tasks (deferred until composition rendering / FHIR-mapping consumer arrives)

1. **Spec REQ-105** documenting the `ArchetypeTerm` / `TermBinding` surface, the per-language map shape, and the fallback rule when the requested language is missing.
2. **Compile-time flattening** — already prescribed in Phase 4.4; this REQ formalises the public accessor (`compiled.Term(nodeID, lang)`, `compiled.TermBindings(nodeID)`).
3. **External terminology lookup** is **out of scope** — REQ-105 only exposes the bindings the OPT carries. A separate REQ in the `auth/` or a new `terminology/` package would handle live SNOMED/LOINC resolution.

## Sequencing summary

```
                   Phase 1 (tests)
                        │
                        ├──> Phase 2 (parser hardening)
                        │
                        └──> Phase 3 (path ergonomics)
                              │
                              └──> Phase 4 (compiled template) ◄──── Phase 4-bis (RM info lookup)
                                    │
                                    ├──> Phase 5 (walker pattern)
                                    │       │
                                    │       └──> REQ-101 composition builder
                                    │
                                    └──> Phase 6 (REQ-103 primitives)
                                          │
                                          └──> REQ-102 validation
                                                │
                                                ├──> Phase 7 (REQ-104 slot assertions, optional)
                                                └──> Phase 8 (REQ-105 terminology bindings, optional)
```

Phases 1-3 are independent — can land in any order, each as its own small PR.
Phase 4 is the **load-bearing foundation**: it depends on Phase 4-bis (RMInfoLookup) and unlocks Phases 5 and 6. REQ-101 needs Phases 4 + 5; REQ-102 needs Phases 4 + 5 + 6.

## Out of scope (this plan)

- **OET parse**, **ADL 2 OPT**, **remote slot-fill repository** — REQ-100 v1 bounds unchanged.
- **Lowering ADL 1.4 into AOM 2** — we keep AOM 1.4 first-class because the OPT XSD emits AOM-1.4-shaped elements (`C_DV_QUANTITY`, `C_DV_ORDINAL`, etc.) and re-synthesising them on serialisation would be wasted work.
- **Importing `openehr/aom/aom14/` into the parser** — the compiled template + REQ-103 primitive constraints supersede the original "reuse aom14 types" idea from the REQ-100 plan. The compile step produces its own typed surface; `aom14` remains a separate consumer-facing API for direct AOM XML decode if needed.
- **JSON-format simplified template export** — a separate concern; see [2026-05-22-webtemplate-export.md](2026-05-22-webtemplate-export.md). The compiled tree here is internal infrastructure consumed by builder + validator; the JSON-format export targets UI-rendering consumers and has its own design constraints.

## Implementation checklist

| Step | Status |
|---|---|
| Phase 1 tests + traceability `landed` | |
| Phase 2 parser hardening | |
| Phase 3 path ergonomics (ErrAmbiguousPath, NodeKind, ObjectNode) | |
| Phase 4-bis RMInfoLookup (codegen + PROBE-023) | |
| Phase 4 Compiled template (internal/templatecompile, AQL paths, implicit attrs, term flattening) | |
| Phase 5 Walker pattern + composition walker | |
| Phase 6 REQ-103 primitive constraints (spec + types + Validate + PROBE-024) | |
| Phase 7 REQ-104 slot assertions (when REQ-101 / REQ-102 surfaces real call sites) | |
| Phase 8 REQ-105 terminology bindings (when renderer / FHIR consumer arrives) | |
| `make ci` green throughout | |

## Mapping to specs

- [`docs/specifications/clinical-modeling.md` § REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) — current
- Pending: REQ-101 (composition builder, [plan](2026-05-21-composition-builder.md)), REQ-102 (validation, [plan](2026-05-21-validation.md)), REQ-103 (primitive constraints), REQ-104 (slot assertions), REQ-105 (terminology bindings)

## References (research baseline, informational)

Plan informed by analysis of two reference implementations and the openEHR specifications. The Go SDK retains its own AOM 1.4 / ADL 1.4 model and Ocean OPT XSD wire shapes; these references are cited for reviewer / future-maintainer convenience.

- **OPT XSD (normative)** — [`specifications.openehr.org/releases/ITS-XML/Release-2.0.0/components/AM/Release-1.4/Template.xsd`](https://specifications.openehr.org/releases/ITS-XML/Release-2.0.0/components/AM/Release-1.4/Template.xsd) + companion `OpenehrProfile.xsd`. Source of truth for top-level wrapper, definition tree, primitive subtypes.
- **openEHR AM specifications** — [`specifications.openehr.org/releases/AM/latest`](https://specifications.openehr.org/releases/AM/latest). Class invariants and AOM 1.4 / ADL 1.4 normative semantics.
- **ehrbase openEHR_SDK** — [`github.com/ehrbase/openEHR_SDK`](https://github.com/ehrbase/openEHR_SDK). Java reference. Sourced design patterns: two-layer model (raw OPT → `WebTemplate` compiled), `Walker<T>` with `Context<T>` of three parallel stacks, RM-type-prefix slot fallback, `WebTemplateSkeletonBuilder` for default-composition build. Deepwiki summary: [`deepwiki.com/ehrbase/openEHR_SDK/3.2-template-structure`](https://deepwiki.com/ehrbase/openEHR_SDK/3.2-template-structure).
- **openEHR/archie** — [`github.com/openEHR/archie`](https://github.com/openEHR/archie). Java reference (AOM 2 internally, with ADL 1.4 ingest via conversion). Sourced design patterns: single `CAttribute` with `multiple bool` + `Cardinality`, three-layer path system (`APathQuery` parser → `AOMPathQuery` walks template → `RMPathQuery` walks RM instance), first-class primitive constraint types in `aom/primitives/`, `RMObjectValidator` composition-vs-template walker.
- **WebTemplate format (deferred plan)** — JSON-format simplified template representation widely used for UI / form-generation consumption. Originating reference: [`github.com/better-care/web-template`](https://github.com/better-care/web-template). Not in scope here; see [2026-05-22-webtemplate-export.md](2026-05-22-webtemplate-export.md).
