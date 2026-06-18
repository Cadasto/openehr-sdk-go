# Clinical modeling

**Status:** Draft

Normative contract for the SDK's clinical-modeling artefacts: operational templates (OPT), templated composition assembly, validation against templates, and AQL path semantics on templated trees. Covers REQ-100 onwards.

The "clinical modeling" band sits above the openEHR Reference Model and below the REST clients: it consumes RM types from [`openehr/rm/`](../../openehr/rm/) and AOM 1.4 constraint types from [`openehr/aom/aom14/`](../../openehr/aom/aom14/), and produces typed building blocks usable by `openehr/composition/`, `openehr/validation/`, and `openehr/aql/`.

---

## REQ-100 — ADL 1.4 operational template (OPT) parse and paths

The SDK **MUST** ship a parser for ADL 1.4 **operational templates** (OPT) as a building-block package: `openehr/template/`. Authoring-time templates (`.oet`) are out of scope in v1.

### Scope

In openEHR terminology, "template" without qualification often means the authoring template (`.oet`). In this SDK v1, **"template" in package and REST names means operational template (OPT)** unless explicitly stated otherwise.

- **Input format:** ADL 1.4 OPT XML — root element `<template>` in namespace `http://schemas.openehr.org/v1` (the canonical Ocean Template Designer XSD form), wire `application/xml` (same as `definition.FormatADL14` in `openehr/client/definition/`). The parser **MUST** accept `<?xml ?>` declarations, BOM-prefixed UTF-8, and namespaced XSD-typed children (`xsi:type` discrimination on `attributes` and `children`).
- **File extension:** `ParseFile(path string)` **MUST** reject paths that do not end in `.opt` (case-insensitive) with `ErrNotOPTFile` to keep the v1 surface unambiguous. `ParseOPT(io.Reader)` accepts any reader and applies no path check.
- **Output:** `*OperationalTemplate` carrying the parsed wrapper fields (template id, concept, uid, language) plus the definition tree (`Node` interface).

### Identity fields

`*OperationalTemplate` **MUST** expose at least:

- `TemplateID() string` — the value of `<template_id>/<value>` (e.g. `vital_signs`).
- `Concept() string` — the value of `<concept>` (machine-readable concept slug).
- `UID() string` — the value of `<uid>/<value>` when present; empty string otherwise.
- `Language() string` — the value of `<language>/<code_string>` (ISO 639-1) when present; empty string otherwise.
- `Root() Node` — the root definition node. Its `RMTypeName()` is the composition RM class (conventionally `COMPOSITION`). The concrete type is `*ArchetypeRoot` when the OPT `<definition>` carries an explicit archetype id (the typical Ocean Template Designer shape) and `*ComplexObject` otherwise. Callers that descend into attributes MUST handle both via a type-switch (or match on `ObjectNode`, the supertype of `*ComplexObject` + `*ArchetypeRoot`), or via `NodeAt`.

### Provenance metadata (optional)

`*OperationalTemplate` **MAY** additionally expose top-level OPT provenance for auditing and editor tooling:

- `Description() *Description` — parsed `<description>` block; nil when omitted. The returned `*Description` exposes `LifecycleState() string`, `OriginalAuthors() map[string]string`, and `OtherDetails() map[string]string`. The returned maps are defensive copies — mutation by the caller does not affect the underlying template.
- `Annotations() map[string][]Annotation` — parsed `<annotations path="...">` blocks keyed by the path attribute (empty string when no path). Returns nil when the OPT carries no annotations. The returned map is a defensive copy.

### Node taxonomy

The parsed definition tree is a closed taxonomy. `Node` is a sealed interface implemented by:

| Concrete | OPT XML shape | Carries |
|---|---|---|
| `ComplexObject` | `xsi:type="C_COMPLEX_OBJECT"` | `RMTypeName()`, `NodeID()`, child `Attribute` list, optional occurrences |
| `Attribute` | `xsi:type="C_SINGLE_ATTRIBUTE"` or `C_MULTIPLE_ATTRIBUTE"` | `Name()` (RM attribute name), `Cardinality()` (single vs multiple), child `Node` list |
| `ArchetypeRoot` | `xsi:type="C_ARCHETYPE_ROOT"` | `ArchetypeID()` (e.g. `openEHR-EHR-OBSERVATION.blood_pressure.v1`), plus the same surface as `ComplexObject` |
| `Slot` | `xsi:type="ARCHETYPE_SLOT"` | `Includes()` / `Excludes()` archetype-id assertion lists (lists may be empty) |

Concrete primitive constraints (`C_CODE_PHRASE`, `C_PRIMITIVE_OBJECT`, `C_DV_QUANTITY`, etc.) appear as **leaf `ComplexObject`** values (`RMTypeName()` returns the RM class name, no attribute children). The typed primitive-constraint surface lives on `ComplexObject.PrimitiveConstraint()` and is enumerated under REQ-103.

### Path syntax (subset)

The parser **MUST** accept the following openEHR path subset and reject anything else with `ErrPathSyntax`:

- Absolute paths only — leading `/`.
- Segments are RM attribute names: `/content`, `/data/events/data/items`.
- Optional **archetype node predicate** on a segment: `/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]` or `/items[at0001]`.
- Multiple predicates on the same segment are **NOT** supported in v1 (e.g. no `[at0001,name="Systolic"]`).
- Trailing slash is **NOT** permitted.
- AQL projection syntax (`/value/magnitude`, `[name='...']`) is **NOT** part of this REQ — that surface belongs to `openehr/aql/`.

### Resolution semantics (`NodeAt`)

Given a parsed `Path`, `NodeAt`:

- Walks `Root()` → child attributes → child nodes recursively.
- Matches segment names against `Attribute.Name()` (exact, case-sensitive).
- Matches segment predicates against `Node.NodeID()` (at-codes) **or** `ArchetypeRoot.ArchetypeID()`.
- Returns `ErrPathNotFound` if no node matches a segment.
- Returns the first matching node when a segment has multiple candidates without a predicate (deterministic by document order).

`NodeAt` accepts variadic `ResolveOption` values. `WithStrictPaths()` switches to strict resolution: a predicate-less segment that matches an attribute with more than one candidate child returns `ErrAmbiguousPath` instead of silently selecting the first child. The default (no option) preserves the first-match behaviour above. `ValidatePath(p Path, opts ...ResolveOption) error` is a shorthand for `NodeAt` that discards the resolved node — convenience for code-generator preconditions.

### Strict parse mode (optional)

The default `ParseOPT` / `ParseFile` entry points remain forward-compatible: unknown child `xsi:type` values are admitted as leaf `*ComplexObject` nodes. `ParseOPTStrict` / `ParseFileStrict` opt into stricter behaviour — an unknown child `xsi:type` that carries nested `<attributes>` is rejected with `ErrUnsupportedNode` (the only case where lenient mode would silently drop a non-trivial subtree). Use strict mode in validators and code generators that must surface unsupported shapes rather than silently truncate them.

### Error taxonomy

The package **MUST** expose these typed sentinel errors:

| Sentinel | Triggered by |
|---|---|
| `ErrInvalidOPT` | malformed XML, missing required wrapper element (template_id, definition), unsupported root element |
| `ErrNotOPTFile` | `ParseFile` called with non-`.opt` path |
| `ErrPathSyntax` | path string fails the grammar subset above |
| `ErrPathNotFound` | parsed path traverses through an unknown attribute or unmatched predicate |
| `ErrAmbiguousPath` | strict mode (`WithStrictPaths`) — predicate-less segment matches an attribute with multiple candidate children. Never returned by the default first-match behaviour. |
| `ErrUnsupportedNode` | encountered an `<attributes>` element whose `xsi:type` is outside the v1 attribute taxonomy (`C_SINGLE_ATTRIBUTE`, `C_MULTIPLE_ATTRIBUTE`). Unknown **child** `xsi:type` values are not surfaced through this sentinel by default — they are admitted as leaf `*ComplexObject` nodes (forward-compatible escape hatch). In strict mode (`ParseOPTStrict` / `ParseFileStrict`), an unknown child `xsi:type` that carries nested `<attributes>` is rejected via this sentinel. |

All errors wrap context with `fmt.Errorf("...: %w", err)`; callers compare with `errors.Is`.

### Building-block independence (REQ-013)

`openehr/template/` **MUST** be importable without `transport/`, `auth/`, `openehr/client/*`, `openehr/rm/`, or `openehr/aom/aom14/`. In v1 the package depends only on the standard library plus its own sibling sub-package `openehr/template/constraints/` (REQ-103 typed primitive constraints) — RM class names appear only as string values surfaced from OPT XML, not as Go type references.

### Out of scope (v1)

- **OET** (`.oet` authoring/design-time templates) — no parse, no OET→OPT compile.
- **ADL 2 operational templates** — covered by a later REQ when consumer demand surfaces.
- **Full Archie-style linker** — archetype slot resolution against an external archetype repository. v1 reads only the OPT-embedded constraint tree.
- **Terminology expansion** — external terminology calls.
- **Runtime template registry** — the CDR owns the deployment registry; this package interprets bytes.

- **Lives in:** [`openehr/template/`](../../openehr/template/)
- **Probes:** PROBE-022 (path resolution against fixture OPT)

---

## REQ-108 — Untrusted document bounds

Clinical-modeling and codec entry points **MUST** bound how much untrusted input they read and how deeply they recurse, so hostile OPT XML, BMM JSON, uploaded templates, or crafted canonical JSON cannot exhaust memory or CPU before the caller's own policy kicks in. Landed reasoning: archived [security-hardening plan](../plans/archive/2026-06-11-security-hardening-and-simplification.md).

### OPT parse and path walk (`openehr/template/`)

- **`ParseOPT` / `ParseFile`** **MUST** reject inputs larger than **32 MiB** (`maxOPTBytes`). Oversize input **MUST** wrap `ErrInvalidOPT` with an `input exceeds N bytes` message.
- **Tree build and `walkPath`** **MUST** reject nesting deeper than **128 levels** (`maxOPTDepth`). Exceeding the depth **MUST** wrap `ErrInvalidOPT` (parse) or `ErrPathNotFound` (path walk).

### BMM load (`openehr/bmm/`)

- **`bmm.Load`** **MUST** reject inputs larger than **32 MiB** (`maxBMMBytes`) with `bmm.ErrInputTooLarge`. See also REQ-045.

### Definition template upload (`openehr/client/definition/`)

- **`UploadTemplate`** **MUST** apply the same **32 MiB** cap as OPT parse before forwarding bytes to the CDR.

### Polymorphic JSON decode (`openehr/rm/typereg/`)

- **`Registry.Decode`** (the single polymorphic-dispatch chokepoint used by generated `UnmarshalJSON`) **MUST** reject JSON whose nesting depth exceeds **512 levels** before dispatch, returning `typereg.ErrMaxDepthExceeded`. The guard lives in hand-written `registry.go` (not per-type generated decoders) — see [ADR 0002](../adr/0002-bmm-codegen-decisions.md) and REQ-040. `encoding/json`'s own 10 000-level scanner limit remains a backstop; this REQ covers the amplification window below that ceiling.

Constants **MAY** be package-level variables overridable in tests; defaults above are normative for production.

- **Lives in:** [`openehr/template/`](../../openehr/template/), [`openehr/bmm/`](../../openehr/bmm/), [`openehr/client/definition/`](../../openehr/client/definition/), [`openehr/rm/typereg/`](../../openehr/rm/typereg/)
- **Tests:** `openehr/template/parse_cap_test.go`, `openehr/template/parse_depth_test.go`, `openehr/bmm/load_test.go`, `openehr/rm/typereg/registry_test.go`

---

## REQ-103 — Primitive constraint introspection

The SDK **MUST** expose every OPT primitive constraint as a typed value attached to its leaf node, so validators and composition-builder consumers can introspect ranges, allowed lists, patterns, units, and code lists without re-parsing the OPT XML.

### Scope

The closed set of REQ-103 primitive constraints maps **one-to-one** to ADL 1.4 OPT XSD primitive `xsi:type` values:

| OPT `xsi:type` | Go type (`openehr/template/constraints/`) | Surface |
|---|---|---|
| `C_BOOLEAN` | `CBoolean` | `TrueValid`, `FalseValid`, optional `Default` |
| `C_INTEGER` | `CInteger` | `Range`, optional closed `List`, optional `Default` |
| `C_REAL` | `CReal` | `Range`, optional closed `List`, optional `Default` |
| `C_STRING` | `CString` | `Pattern` (regex), optional closed `List`, optional `Default` |
| `C_DATE` | `CDate` | `Pattern` (AOM partial-date pattern, raw) |
| `C_TIME` | `CTime` | `Pattern` (raw) |
| `C_DATE_TIME` | `CDateTime` | `Pattern` (raw) |
| `C_DURATION` | `CDuration` | `Pattern` (raw) |
| `C_CODE_PHRASE` | `CodePhrase` | `Terminology`, optional `CodeList`, `External()` predicate |
| `C_DV_QUANTITY` | `DvQuantity` | enumerated `Units` (each with magnitude / precision `NumericRange`), optional `Property` (CodedTermRef) |
| `C_DV_ORDINAL` | `CDvOrdinal` | `Values` (closed list of `(int, CodedTermRef)` pairs) |

Each type implements the sealed interface `constraints.PrimitiveConstraint`:

```go
type PrimitiveConstraint interface {
    Validate(value any) []Violation
    isPrimitive()              // unexported — closes the interface
}
```

The set is closed by `isPrimitive()`; new primitive shapes appear in the `constraints` package only, behind their own REQ.

### Accessor

- `template.ComplexObject.PrimitiveConstraint() constraints.PrimitiveConstraint` — returns the typed value when the wire `xsi:type` was a primitive; returns nil for non-primitive nodes (composition root, archetype roots, slots, plain complex objects).
- `templatecompile.CompiledNode.PrimitiveConstraint() constraints.PrimitiveConstraint` — same value, threaded through the compile step unchanged.

### Validate contract

`Validate(value any) []Violation` returns nil when the input satisfies every clause of the constraint, or one `Violation` per failing clause (range, list, pattern, …). Validators **MUST** be pure functions — no I/O, no reflection over user types beyond a small fixed coercion table per type. Concretely:

- Integer / real validators accept any Go integer kind (`int`, `int8`..`int64`, `uint`, `uint8`..`uint64`). `uint` and `uint64` values exceeding `math.MaxInt64` return `CodeWrongType` rather than silently wrapping. `CReal.Validate` additionally accepts `float32` / `float64`.
- String, date, time, date-time, duration validators accept Go `string`.
- `CBoolean.Validate` accepts Go `bool`.
- `CodePhrase.Validate` accepts either a bare `string` (treated as the code under the constrained terminology) or a `constraints.CodedTermRef`.
- `DvQuantity.Validate` accepts a `constraints.QuantityValue` `{Magnitude, Units, Precision}` triple.
- `CDvOrdinal.Validate` accepts either an `int` (ordinal value) or a `constraints.OrdinalSymbol` `(value, symbol)` pair.

A value whose Go type is not in the accepted set returns a single `CodeWrongType` violation; this is a contract failure on the caller side, not a constraint failure.

### Violation taxonomy

Every `Violation` carries a typed `ViolationCode`. The closed set is:

| Code | Triggered by |
|---|---|
| `CodeOutOfRange` | numeric value outside a `NumericRange` |
| `CodePatternMismatch` | string fails a regex / pattern |
| `CodeNotInList` | value is not a member of a closed list (strings, codes, ordinals, etc.) |
| `CodeWrongType` | input Go type cannot be coerced to the constraint's expected type |
| `CodeUnitUnknown` | DV_QUANTITY units string is not in the enumerated allowed list |
| `CodeInvalidValue` | constraint or input is malformed (e.g. unparseable regex in the OPT, malformed date string) |

`Violation.Detail` carries a human-readable message; consumers building structured diagnostics SHOULD pattern-match on `Code`.

### Numeric range

`NumericRange` is the inclusive / exclusive interval shape used by `CInteger`, `CReal`, `DvQuantity.Magnitude`, and `DvQuantity.Precision`:

- `Lower` / `Upper` (float64; lossless for INTEGER up to 2^53)
- `LowerInclusive` / `UpperInclusive` (defaults to true when the OPT omits the wire flags — the AOM 1.4 convention; the wire parser sets them, but consumers constructing ranges manually MUST set the flags explicitly — the struct zero value is *exclusive* on both sides)
- `LowerUnbounded` / `UpperUnbounded` (when true, the corresponding bound is ignored)

The zero-value `NumericRange{}` (no fields set) is treated as "any value accepted" by `Contains` and `IsBounded` — a no-op constraint. AOM 1.4 also models `C_DURATION.range` (as `Interval<Iso8601_duration>`) plus eight per-component allowed-flags (`years_allowed`, `months_allowed`, …, `fractional_seconds_allowed`); v1 captures none of them — `CDuration` exposes the raw `Pattern` only. The richer surface is deferred to a follow-up REQ (calendar conversion is out of scope for v1).

### Out of scope (this REQ)

- **AOM partial date / time pattern enforcement** — `CDate`, `CTime`, `CDateTime`, `CDuration` capture the raw `Pattern` string but `Validate` performs only an ISO 8601 sanity check. Strict AOM-pattern enforcement is a follow-up. Validators that need it interpret the stored pattern directly.
- **`C_STRING.list_open`** — AOM 1.4 declares this mandatory flag on `C_STRING` to distinguish open enumerations (the list is *exemplars*, not the closed set) from closed ones. v1 `CString` does not capture it; `Validate` treats every non-empty `List` as closed. Surfacing the flag (and weakening `Validate` to "advisory" when `list_open=true`) is a follow-up REQ.
- **`ARCHETYPE_SLOT` assertion grammar** — landed under REQ-104 (see below).
- **External terminology lookup** — REQ-105 surfaces bindings carried in the OPT; neither REQ-103 nor REQ-105 calls into a remote terminology service during `Validate`.
- **AOM 2 `tuple_constraint`** — not used by ADL 1.4.

### Building-block independence (REQ-013)

`openehr/template/constraints/` is **stdlib-only**. It is importable independently of `openehr/template/` so codegen and downstream validators can use the constraint types without pulling the OPT parser.

### Example value emission (REQ-107 hook)

Every `PrimitiveConstraint` additionally exposes `ExampleValue() any` — a minimal-valid Go example value in the shape `Validate` accepts. For bounded constraints (closed lists, bounded ranges, enumerated units), `Validate(c.ExampleValue())` MUST return an empty `Violation` slice; unbounded primitives return a documented sentinel (e.g. `"example"`, `int64(0)`, `"2020-01-01"`). The factory is the leaf primitive of the REQ-107 template-driven instance generator and stays on the sealed interface so the closed type-switch (REQ-024 — no reflection) remains the only entry point for new primitive shapes. See § REQ-107 for the generator contract and [`docs/plans/2026-05-24-template-instance-example-generator.md`](../plans/archive/2026-05-24-template-instance-example-generator.md) § "Example value factory" for the per-type strategy table.

- **Lives in:** [`openehr/template/constraints/`](../../openehr/template/constraints/)
- **Probes:** PROBE-024 (primitive constraint validation against fixture inputs)

---

## REQ-104 — Slot assertion grammar

The SDK **MUST** parse the ADL 1.4 `ARCHETYPE_SLOT` include / exclude assertion subset sufficient for slot-fit checking in validators and instance synthesis.

### Supported grammar (v1)

v1 supports the `archetype_id matches {regex}` expression form, including:

- Plain text assertions embedded in OPT XML (`archetype_id matches {/openEHR-EHR-OBSERVATION\.body_weight\..*/}`)
- The OPT XML expression tree where operator `2007` (`matches`) binds `archetype_id/value` to a `C_STRING` `<pattern>` (the Ocean Template Designer shape)

Unparseable assertion blobs are retained on [`Slot.Includes`](../../openehr/template/template.go) / [`Slot.Excludes`](../../openehr/template/template.go) and ignored by the structured matcher; when every include blob fails to compile the slot widens to the RM-type-prefix fallback (observable via `SlotRules.IncludesDroppedUnparsed`).

### Contract

- [`constraints.SlotAssertion`](../../openehr/template/constraints/slot.go) carries a compiled Go `regexp` and exposes `MatchesArchetypeID(string) bool`.
- [`constraints.SlotRules`](../../openehr/template/constraints/slot.go) aggregates includes and excludes for one slot. `AllowsArchetypeID` applies excludes first, then requires a match against at least one include when includes are present; when no includes were parsed it **MUST** fall back to the RM-type-prefix rule (`openEHR-EHR-<rmType>.`). A catch-all exclude (`.*`) is **ignored when includes are present** — template editors auto-generate it as the complement of a closed includes list, so applying it literally would reject the slot's own includes.
- Wire-side [`Slot`](../../openehr/template/template.go) exposes `ParsedIncludes`, `ParsedExcludes`, `AllowsRMType` (prefix fallback), `AllowsArchetypeID`, and `SlotRules`.
- [`templatecompile.CompiledNode`](../../internal/templatecompile/node.go) copies parsed rules at compile time and exposes `AllowsArchetypeID` / `ExampleSlotFillArchetypeID` for validators and the instance synthesiser.

### Building-block independence (REQ-013)

`openehr/template/constraints/` remains stdlib-only. Slot assertion types live alongside primitive constraints.

- **Lives in:** [`openehr/template/constraints/slot.go`](../../openehr/template/constraints/slot.go), [`openehr/template/slot_assertion.go`](../../openehr/template/slot_assertion.go), [`internal/templatecompile/`](../../internal/templatecompile/), [`openehr/validation/walk_composition.go`](../../openehr/validation/walk_composition.go)
- **Tests:** [`openehr/template/constraints/slot_test.go`](../../openehr/template/constraints/slot_test.go), [`openehr/template/slot_assertion_test.go`](../../openehr/template/slot_assertion_test.go)

---

## REQ-105 — Terminology bindings

The SDK **MUST** expose structured accessors for archetype term definitions and external terminology bindings carried in an OPT, without performing live terminology resolution.

### Contract

- [`ArchetypeTerm`](../../openehr/template/template.go) and [`TermBinding`](../../openehr/template/template.go) remain the wire-side records parsed from `<term_definitions>` and `<term_bindings>`.
- [`templatecompile.Compiled.TermLang(nodeID, lang)`](../../internal/templatecompile/compiled.go) resolves an at-code's term text scoped to the composition root archetype. [`CompiledNode.Term(code, lang)`](../../internal/templatecompile/node.go) walks parent archetype roots for context-sensitive lookup.
- **Language fallback:** ADL 1.4 OPTs carry a single document language (`Compiled.Language()`). When the requested `lang` is empty or equals the document language, the OPT's `Items` map (`text`, `description`, …) is returned. When `lang` differs and no translation exists in the OPT, the document-language term **MUST** be returned (no error — callers distinguish absence via the `ok` bool only).
- [`Compiled.TermBindingsForNode(nodeID)`](../../internal/templatecompile/compiled.go) filters the compile-time flattened binding list to entries whose `NodeOrPath` equals the at-code or whose AQL-like locator contains `[nodeID]`.
- External SNOMED / LOINC / ICD lookup is **out of scope** — REQ-105 only surfaces bindings the OPT carries.

- **Lives in:** [`openehr/template/`](../../openehr/template/), [`internal/templatecompile/compiled.go`](../../internal/templatecompile/compiled.go)
- **Tests:** [`internal/templatecompile/compile_test.go`](../../internal/templatecompile/compile_test.go)

---

## REQ-102 — Composition validation

The SDK **MUST** ship a `ValidateComposition(comp *rm.Composition, c *templatecompile.Compiled) Result` entry point that walks a parsed RM `Composition` against a compiled OPT and returns a `Result` aggregating every issue found in a single pass.

### Contract

- **Pure function.** No I/O, no goroutines, no reflection. Stateless — concurrent callers share `c` safely.
- **Collect-all, not fail-fast.** Validators emit one `Issue` per failing clause; the walk completes regardless of how many issues fire. UIs and CI consumers need the full list.
- **Result shape:**
  ```go
  type Result struct {
      OK     bool      // true when no Error-severity issue (Warnings alone leave OK true)
      Issues []Issue
  }
  type Issue struct {
      Path     string   // AQL path of the offending node (empty for global issues)
      Code     string   // stable programmatic identifier — see code taxonomy below
      Detail   string   // human-readable message
      Severity Severity // Error for normative violations; Warning for advisories
  }
  type Severity int
  const (
      Error   Severity = iota
      Warning              // advisory; does not flip Result.OK ([ValidateAQL] emits these)
  )
  ```

### Trust model

The validator treats the **compiled OPT as authoritative for structure** and the **composition as the instance under test**. Structural traversal is template-driven: for each compiled OPT node, the walker reads the corresponding RM property by `rm_attribute_name`, enforces existence / cardinality / alternatives, and recurses into matched RM children. Path strings in `Issue.Path` come from the OPT's pre-computed `AQLPath` (`templatecompile.CompiledNode.AQLPath`) — composition-supplied predicates never form lookup keys, so missing nodes are reported instead of silently bypassed.

The lockstep walker lives in `openehr/validation/` (not `internal/templatecompile/walk/`) — see [ADR 0006](../adr/0006-composition-validation-walker-placement.md). `internal/templatecompile/walk/` remains OPT-only traversal for compile-time tooling.

An RM-guided intermediate (v1) landed on a sibling branch as a stepping stone: it descended the composition graph via typed switches, built AQL paths from the composition's at-codes, looked up OPT constraints at those paths, and applied REQ-103 primitive checks at every matched leaf. That intermediate could not flag missing OPT-required nodes (no RM subtree → no path → no lookup); the template-driven walk closes that gap. See the plan at [`docs/plans/archive/2026-05-24-composition-validation-template-driven.md`](../plans/archive/2026-05-24-composition-validation-template-driven.md) for the migration's phase split.

### Validation dimensions

| Dimension | Implementation |
|---|---|
| **Structural — root archetype match** | comp.ArchetypeNodeID matches the OPT root's archetype id |
| **Structural — required attributes (composition root + recursive)** | RM-mandatory attrs at the root (Category, Composer, Language, Territory); template-driven existence checks at every OPT node whose attribute interval lower ≥ 1 |
| **Structural — child cardinality** | for each C_MULTIPLE_ATTRIBUTE, the RM child count is checked against the parsed `CompiledAttribute.ChildMultiplicity()` interval and each child's `Occurrences()` |
| **Structural — alternatives (C_SINGLE_ATTRIBUTE)** | RM value must match one of the attribute's child constraints; first-match wins. Exactly one child → `rm_type_mismatch` on failure; two or more children → `alternative_mismatch` |
| **Structural — RM type match** | the RM instance's concrete type must satisfy the OPT child's `RMTypeName` (with abstract supertype admission per BMM); single-child attributes surface failures as `rm_type_mismatch` at the attribute path |
| **Identity — archetype / node id pinning** | LOCATABLE.archetype_node_id is checked against the matched OPT child's `ArchetypeID()` (for archetype roots) or `NodeID()` (for inner at-codes) |
| **Primitive constraints** | REQ-103 `PrimitiveConstraint.Validate` runs at every primitive leaf the OPT declares; bound to the RM value found by the structural walk |
| **Slot fit — assertion grammar** | REQ-104 `CompiledNode.AllowsArchetypeID` (includes / excludes with RM-type-prefix fallback when no includes parsed) |
| **Extra RM nodes not declared in OPT** | not flagged in v2; optional `warning` policy is a follow-up |
| **Terminology binding value-set** | deferred — live external terminology lookup; REQ-105 surfaces OPT bindings only |

### Issue codes

`Issue.Code` is a stable programmatic identifier; the closed set is:

| Code | Triggered by |
|---|---|
| `required` | a required attribute (OPT-declared or RM-mandatory) is absent / zero-valued |
| `cardinality` | a multi-valued attribute's child count violates the OPT-declared cardinality / occurrences interval |
| `alternative_mismatch` | no child of a C_SINGLE_ATTRIBUTE with **two or more** alternatives matches the RM value |
| `rm_type_mismatch` | the RM instance's concrete type disagrees with the OPT child's declared `RMTypeName`, including single-child C_SINGLE_ATTRIBUTE type constraints |
| `archetype_id_mismatch` | LOCATABLE.archetype_node_id does not equal the OPT-pinned archetype id at the matched node |
| `node_id_mismatch` | LOCATABLE.archetype_node_id does not equal the OPT-pinned at-code at the matched node |
| `primitive_*` | a REQ-103 primitive `Violation.Code` (`out_of_range`, `pattern_mismatch`, `not_in_list`, `wrong_type`, `unit_unknown`, `invalid_value`) at a leaf |
| `slot_fill` | an RM value under a slot-constrained attribute whose `archetype_node_id` satisfies no OPT child or parsed slot assertion; slots fall back to the RM-type-prefix rule only when no include assertions were parsed |
| `nil_composition` / `nil_template` | global guards — caller supplied a nil argument |

Existence and child-count cardinality are **independent constraints**: a multi-valued attribute with `existence.lower ≥ 1` AND `cardinality.lower ≥ 1` whose RM-side slice is empty fires BOTH `required` AND `cardinality` at the same path. Validators MUST emit both codes when both clauses fail; collect-all semantics make this the natural outcome. Consumers de-duplicating for display SHOULD treat the pair as a single user-facing failure at that path.

**Open multi-valued attributes** — when a C_MULTIPLE_ATTRIBUTE declares no `<children>` (the OPT pinned only existence / cardinality, not membership), validators MUST accept any RM item under that attribute without firing `slot_fill`. The constraint surface is the attribute itself; the items inside are unconstrained. This admits the OPT idiom "items here, any shape allowed" — e.g. a SECTION whose /items is open to any archetype-root content.

### Sentinels

The package **MUST** expose typed sentinels callers compare via `errors.Is` for programmatic dispatch. Issues bridge to sentinels via `Issue.Err() error`:

| Sentinel | Triggered by |
|---|---|
| `ErrCardinality` | `cardinality` code |
| `ErrRequired` | `required` code |
| `ErrTypeMismatch` | `rm_type_mismatch`, `alternative_mismatch`, `archetype_id_mismatch`, `node_id_mismatch` |
| `ErrPrimitive` | any `primitive_*` code |
| `ErrSlotFill` | `slot_fill` code |
| `ErrAQLSyntax` | `aql_syntax`, `aql_empty` codes — the AQL lint surface (REQ-109) |

Caller pattern:

```go
for _, i := range r.Issues {
    if errors.Is(i.Err(), validation.ErrRequired) {
        // typed handling for missing required attributes
    }
}
```

Global guard codes (`nil_composition`, `nil_template`) return `nil` from `Issue.Err()` — they represent caller-side argument errors, not validation failures of a structurally-present composition.

### Building-block independence (REQ-013)

`openehr/validation/` **MUST** be importable without `transport/`, `auth/`, `openehr/client/*`, or `openehr/serialize/`. The validator operates on **in-memory RM graphs**, never on wire bytes — callers responsible for decoding feed already-parsed `*rm.Composition` values. The full forbidden-import set is enforced by `TestValidationForbiddenImports`.

The dependency graph: `openehr/validation/` → `openehr/template/`, `openehr/template/constraints/`, `openehr/rm/`, `openehr/rm/rminfo/`, `internal/templatecompile/` (same-module internal access).

### Public surface scope (resolved by REQ-111)

The `c *templatecompile.Compiled` argument is the compiled-template form. It was, through v0.8.0, typed against the SDK's *internal* compiled-template package, so per Go's `internal/` visibility rule **external consumers (modules outside `github.com/cadasto/openehr-sdk-go`) could not construct it and therefore could not call `ValidateComposition` directly**. The validator was callable only from packages within this module.

**REQ-111 closes this.** The public bridge `openehr/templatecompile.Compile` produces the `*templatecompile.Compiled` (a type alias of the internal compiled form) that this validator accepts, so external modules now drive the full pipeline through public packages with no code change to the validator. [ADR 0005](../adr/0005-compiled-template-foundation.md) §C2 originally proposed re-exporting the constructor as `template.Compile` / `template.Compiled` from `openehr/template`; [ADR 0010](../adr/0010-public-compiled-template-bridge.md) revised the placement to the sibling package `openehr/templatecompile` because hosting it in `openehr/template` would create an import cycle and violate REQ-100's stdlib-only contract. See REQ-111.

### Out of scope (this REQ)

- **AQL lint** (`ValidateAQL`) — **landed** as a separate entry point under REQ-109 (see below); it does not change the composition-validation surface. **Demographic validator** (`ValidateDemographic`) remains deferred.
- **Validating wire bytes / canonical JSON** — the validator never imports `serialize/`. Callers decode first, validate second.
- **External terminology lookup** — value-set membership against SNOMED CT / LOINC / external services. REQ-103 closed-code-list checking is the v1 ceiling.
- **Cross-archetype slot-fill resolution** — no federated archetype repository; slot fit is local to parsed REQ-104 assertions, with RM-type-prefix fallback only when no include assertions were parsed.
- **Full ADL2 / AOM 2 validation semantics.**

- **Lives in:** [`openehr/validation/`](../../openehr/validation/)
- **Probes:** PROBE-025 (composition validation against fixture OPT + composition); PROBE-026 (missing required node, cardinality, alternative_mismatch, rm_type_mismatch, and primitive negative cases — see [`testkit/probes/validation/`](../../testkit/probes/validation/))

---

## REQ-107 — Template-driven RM instance example generator

**Status:** Draft (Phases 0–3 landed).

The SDK **MUST** ship a template-authoritative RM instance synthesiser at `openehr/instance/`: given a compiled OPT, produce a conformant RM object graph whose structure and primitive leaves satisfy the same template-driven contract REQ-102 validates against. The generator is the inverse of validation v2 — same compiled-OPT walk, opposite direction (`rmwrite` instead of `rmread`).

### Scope

The generator is the single skeleton-and-populate engine the composition builder (REQ-101), tests, examples, and data seeding all consume. The root may be **any** RM type the OPT's `rm_type_name` declares — `COMPOSITION`, `OBSERVATION`, `EVALUATION`, `INSTRUCTION`, `ACTION`, `ADMIN_ENTRY`, `CLUSTER`, `SECTION`, `GENERIC_ENTRY`, `ELEMENT`. Output is **synthetic example data**: structurally and constraint-valid for the OPT, not clinically meaningful. The closed root set is v1; new root types appear through a follow-up REQ.

### Contract

Public entry point (target shape, lands with Phase 2):

```go
package instance

type Policy int

const (
    Minimal Policy = iota // required structure only
    Example               // required + populate primitive leaves with example values
)

type Options struct {
    Policy    Policy
    Language  string                       // ISO 639-1; defaults from Compiled.Language()
    Territory string                       // for COMPOSITION roots
    Composer  rm.PartyProxy                // required when root is COMPOSITION
    Now       time.Time                    // clock for EVENT / context times
    UIDSource func() *rm.HierObjectID      // optional determinism hook for LOCATABLE.uid (nil = crypto/rand)
}

func Generate(ctx context.Context, c *templatecompile.Compiled, opts Options) (any, error)
func AsComposition(v any) (*rm.Composition, error)
func AsObservation(v any) (*rm.Observation, error)
// … closed set matching validation ContentItem + standalone archetype roots
```

`Generate` **MUST** return a root RM value satisfying the OPT's structural rules and REQ-103 primitive constraints. Specifically, `Minimal` materialises only attributes with existence lower ≥ 1 (plus BMM-mandatory implicit attrs); `Example` additionally populates every primitive leaf via `PrimitiveConstraint.ExampleValue()`. Multi-valued attributes are sized to `max(existence.lower, 1)`; `C_SINGLE_ATTRIBUTE` alternatives resolve first-child-wins (matching validation v2's first-alternative semantics).

Slot handling (v1): pinned archetype-root children under a slot are synthesised; pure `ARCHETYPE_SLOT` assertions resolve via the parsed REQ-104 include grammar when a safe example id can be derived, or via the RM-type-prefix fallback only when no include assertions were parsed — same compromise as validation slot-fit.

### Trust model

The compiled OPT is **authoritative for structure**. The RM graph is assembled attribute-by-attribute from compiled metadata; the generator never guesses paths from an empty composition. Primitive leaves come from `PrimitiveConstraint.ExampleValue()` (REQ-103), which guarantees `Validate(ExampleValue()) == nil` for bounded constraints. Optional OPT `<assumed_value>` / `<default_value>` (when compile captures them — a Phase 0 follow-up) **override** the factory.

The generator is **sound** (every output is valid against the OPT), not **complete** (it does not enumerate every valid instance — different policies may produce different but equally valid trees). Sound × validator-aligned ⇒ PROBE-027 cross-checks the contract.

### Trust model — phasing

Phases 0–3 landed: `ExampleValue()` on every `PrimitiveConstraint`; `internal/templateinstance/rmwrite/` inverse-of-rmread RM construction table; `openehr/instance/` synthesiser with `Generate` / `Policy` / `UIDSource` test-determinism seam / typed accessors for the closed root set; PROBE-027 implemented (Sandbox) covering `vital_signs.opt` + `clinical_note.opt`; `cmd/examples/generate-example/` worked example. The C_PRIMITIVE_OBJECT inner-`<item>` wire-parser fix + canjson-polymorphic `Composition.uid` emission landed via the [wire-parser plan](../plans/archive/2026-05-26-c-primitive-object-wire-parser.md) (archived); PROBE-023 now exercises the full marshal → unmarshal → re-marshal round-trip. Phase 4 (REQ-101 composition-builder integration delegating to `instance.Generate`) tracked in [`docs/plans/archive/2026-05-24-template-instance-example-generator.md`](../plans/archive/2026-05-24-template-instance-example-generator.md) (archived). REQ-104 slot-fill archetype-id stamping is landed for parsed include patterns that can be synthesized safely; when no includes were parsed the synthesiser uses `openEHR-EHR-<RMType>.example.v1` to satisfy the validator's RM-type-prefix heuristic.

### Out of scope

- **Clinically realistic distributions** (plausible names, plausible vitals, FHIR Synthea-style synthetic patient data).
- **FLAT / STRUCTURED example strings** — REQ-053.
- **Authoring-time templates (OET)** — REQ-100 is OPT-only in v1.
- **Generating every valid instance** — combinatorial coverage is out of scope.
- **Writing to a CDR** — caller's `openehr/client/ehr/` responsibility.
- **Validating during generation** — separate `validation.ValidateComposition` call (cross-checked by PROBE-027).
- **Runtime federated slot-fill repository** — same compromise as validation.
- **Multi-language term translation** — caller seeds `Options.Language`.

### Building-block independence (REQ-013)

`openehr/instance/` **MUST** be importable without `transport/`, `auth/`, `openehr/client/*`, or `openehr/serialize/`. The generator operates on **in-memory RM graphs**, never on wire bytes — callers wanting canonical JSON / XML output run `serialize/canjson` or `canxml` themselves (`cmd/examples/` may import the codec; the library does not).

The public signature accepts `*templatecompile.Compiled`. As with `validation.ValidateComposition`, REQ-111 makes that argument externally constructable via `openehr/templatecompile.Compile`, so `instance.Generate` is now callable from outside the module (see [ADR 0010](../adr/0010-public-compiled-template-bridge.md)).

- **Lives in:** [`openehr/instance/`](../../openehr/instance/) (lands in Phase 2); `openehr/template/constraints/.ExampleValue()` (Phase 0 — landed); `internal/templateinstance/` (Phase 1+).
- **Probes:** PROBE-027 — `instance.Generate` + `validation.ValidateComposition` round-trip clean on the same OPT (Phase 3).

---

## REQ-101 — Generic OPT-driven composition builder

**Status:** Draft (Phases 0–2 landed).

The SDK **MUST** ship a composition-specific authoring layer at `openehr/composition/` that produces an in-memory `*rm.Composition` graph driven by a compiled OPT. REQ-101 owns the composition options and path-first authoring API; REQ-107 owns the skeleton-synthesis engine. The composition builder is a thin shim over `openehr/instance` — no second OPT walker lives here.

### Scope

Two entry points:

1. **`NewSkeleton(ctx, c, opts...) (*rm.Composition, error)`** — produces a structurally-conformant default composition with no clinical data. Delegates to `instance.Generate` with `Policy: Minimal` and unwraps the root via `instance.AsComposition`.
2. **`NewBuilder(ctx, c, opts...) (*Builder, error)`** — seeds a `Builder` from `NewSkeleton`, then accepts `Set(path, value)` calls. `Build()` returns the populated graph and aggregates per-path errors.

### Contract

- **Composition-specific options** — `WithLanguage(code)`, `WithTerritory(code)`, `WithComposer(p)`, `WithCategory(c)`, `WithNow(t)`. The first four translate to fields on `instance.Options` and pin `Composition.language` / `.territory` / `.composer` / `.category`. `WithNow` injects the clock used for `EVENT.time` and `EventContext.start_time` defaults so tests stay deterministic.
- **Path-first API** — `Set(path string, v any) error` looks up the compiled node via `Compiled.NodeAt(path)` and routes the assignment through the parent attribute. Typed helpers `SetText`, `SetQuantity`, `SetCodedText` wrap the most common DV shapes. Paths MUST be canonical OPT paths as produced by the compile step — predicate-bracketed segments included where the OPT pins archetype roots or at-codes.
- **Type enforcement** — `Set` checks the supplied Go value against the compiled node's `RMTypeName()`. A mismatch (e.g. a `*rm.DVText` passed at a DV_QUANTITY path) returns `ErrTypeMismatch`. Unknown paths return `ErrUnknownPath`. Both errors wrap context with `fmt.Errorf("...: %w", err)` and are comparable via `errors.Is`.
- **Aggregated errors** — `Set` records errors against the builder but does NOT short-circuit; subsequent assignments still attempt. `Build()` returns the populated `*rm.Composition` plus the aggregated error (joined via `errors.Join`) so callers can recover every faulty path in one round-trip rather than chasing one error at a time.
- **TemplateID propagation** — `Builder.TemplateID()` returns the OPT's `Compiled.TemplateID()`, suitable for the REST `composition.WithTemplateID` option so the CDR validates against the same template.

### Trust model

REQ-101 trusts REQ-107 for the skeleton walk: every implicit RM attribute, every primitive default, every LOCATABLE identity stamp comes from `instance.Generate`. REQ-101 limits its own dispatch to (a) translating options into `instance.Options` and (b) navigating the path → target attribute → call `rmwrite.EnsureSingle` / `AppendMultiple`. Reads during navigation go through `openehr/validation/rmread.ReadSingle` — the same closed type switch the validator uses — so the read / write halves stay symmetric.

### Out of scope

- **Per-template generated Go structs.** v1 stays generic — consumers do not import codegen'd vital-signs structs through this package. OET-driven authoring is a follow-up.
- **FLAT / STRUCTURED ingest.** Caller decodes externally (REQ-053) and feeds the resulting `*rm.Composition` through validation.
- **Slot resolution against a federated archetype repository.** Same compromise as REQ-102 / REQ-107: pinned slot fills come from the OPT.
- **Encoding to wire bytes.** The builder does not import `openehr/serialize/`; callers run `canjson.Marshal` / `canxml.Marshal` themselves.
- **Validating during Build.** A `Build()` result MUST be runnable through `validation.ValidateComposition` separately; the builder is sound-by-construction but not a validator.

### Building-block independence (REQ-013)

`openehr/composition/` **MUST** be importable without `transport/`, `auth/`, `openehr/client/*`, or `openehr/serialize/`. It depends on `openehr/rm`, `openehr/rm/typereg`, `openehr/template`, `openehr/templatecompile` (the public REQ-111 bridge, referenced in the exported `NewBuilder` / `NewSkeleton` signatures), `openehr/template/constraints`, `openehr/instance`, `openehr/validation/rmread`, `internal/templatecompile`, and `internal/templateinstance/rmwrite`. The forbidden-import set is enforced by `TestCompositionForbiddenImports`.

- **Lives in:** [`openehr/composition/`](../../openehr/composition/)
- **Probes:** PROBE-023 — `composition.NewBuilder` + `Set` → `Build` → `canjson.Marshal` → `canjson.Unmarshal` → re-marshal round-trip preserves values at key paths.

---

## REQ-109 — AQL static lint

The SDK **MUST** ship a building-block parse + lint pipeline for hand-written, imported, or `aql.NewQuery(literal)` AQL, so CI validators, MCP tools, and pre-flight checks can catch defects before a query reaches the CDR — without replacing the typed builders (REQ-055) or the CDR as the execute-time semantic authority (PROBE-021).

The lint runs three layers and **MUST** be collect-all (return every issue, not fail-fast), matching REQ-102.

### Syntax floor — the SDK grammar profile

The parse layer **MUST** validate against the **SDK-maintained grammar profile**, not a live pull from specifications.openehr.org: foundation openEHR AQL (QUERY Release-1.1.0) plus the documented `SDK-AQL-NNN` deltas in [`resources/aql/grammar/DIVERGENCES.md`](../../resources/aql/grammar/DIVERGENCES.md) (ADR [0007](../adr/0007-aql-antlr-grammar-profile.md)). Deltas are classed **relaxation** (admits more than official AQL — e.g. `SELECT *`) or **correction** (fixes a foundation weak spot). The generated parser lives in `openehr/aql/parse/gen/` and is regenerated by `make aqlgen` (containerised ANTLR; never on the build/test path).

**Lint-clean is not spec-conformance, and not execute-success.** Because the profile deliberately admits relaxations and the CDR is the path authority, a query the SDK lints clean **MAY** still be rejected on execution; conversely the lint targets only the contract below.

### Layer 1 — Syntax

- Empty / whitespace-only input **MUST** yield code `aql_empty` (before parse).
- Input that does not parse as `selectQuery` per the profile **MUST** yield code `aql_syntax`, carrying the ANTLR line/column in `Detail`.
- `parse.Parse` returns a `*parse.SyntaxError` wrapping the building-block sentinel `aql.ErrSyntax`; the `validation.ValidateAQL` bridge maps `aql_syntax` / `aql_empty` to `validation.ErrAQLSyntax` via `Issue.Err()`.

### Layer 2 — Shape (AST walk, no CDR)

| Check | Code | Severity | Rule |
|---|---|---|---|
| Alias binding | `aql_unknown_alias` | Error | Every identified path's root alias **MUST** bind to a class in FROM / CONTAINS. |
| Identifiable scope | `aql_from_archetype` | Warning | FROM/CONTAINS **SHOULD** name ≥1 archetype HRID, `$param` archetype predicate, `VERSION` operand, or `EHR` root; otherwise the query scans broadly. |
| Bound parameters | `aql_unbound_param` | Error | When linting an `aql.Query`, every `$name` referenced **MUST** have a key in `Query.Parameters`. |
| Unused parameters | `aql_unused_param` | Warning | A `Query.Parameters` key never referenced is advisory. |

SELECT-present-with-≥1-projection and FROM-present are guaranteed by a successful parse (the grammar requires both), so they raise no Layer-2 issue.

### Layer 3 — Path & template (only when a compiled OPT is supplied)

| Check | Code | Severity | Rule |
|---|---|---|---|
| Archetype membership | `aql_archetype_not_in_template` | Error | Each literal archetype HRID in FROM/CONTAINS **MUST** be present in the compiled OPT (`Compiled.AllByArchetypeID`). |
| Path in template | `aql_path_not_in_template` | Warning | Each identified path **SHOULD** resolve under its alias's archetype subtree. |

`aql_path_not_in_template` resolution walks the **archetype-scoped compiled subtree** (predicate-aware first-child descent) and warns **only on high-confidence structural divergence** — a path segment naming an attribute that does not exist on a node that *has* modelled attributes. It stays silent on unmodelled RM-leaf attributes (e.g. `/value/magnitude`) and on descent below the modelled tree, because the CDR — not the OPT index — is the path authority (PROBE-021). **Documented false-positive mode:** a path through a non-mandatory RM attribute the OPT did not constrain may still warn; the check is a Warning precisely for this reason.

### Issue model and entry points

- `openehr/aql/lint` owns its own `lint.Issue` / `lint.Result` / `lint.Severity` and **MUST NOT** import `openehr/validation` — the dependency arrow is `validation → lint`. `lint.Result.OK()` is true when no **Error**-severity issue is present (Warnings do not make a result not-OK).
- `lint.LintString(q, *Options)` is the raw-AQL entry; `lint.Lint(doc, *Options)` lints an already-parsed `*parse.Document`. `Options{Compiled, Query}` is nilable — nil runs Layers 1–2 only.
- `validation.ValidateAQL(q aql.Query, c *templatecompile.Compiled) validation.Result` is the seam: it parses `q.Q`, runs the layers, and maps `lint.Issue` → `validation.Issue` (code and severity carried verbatim) so callers already using `ValidateComposition` get one uniform `Result`.

### Out of scope (v1)

- **Terminology** (`TERMINOLOGY()` / `MATCHES` value-set membership), function signatures, `ORDER BY` type checking, and version predicates beyond parse.
- **CDR-grade path resolution** — full AQL-path-to-canonical-path mapping (node-id-on-structural-attribute vs canonical placement) is PROBE-021 territory; Layer 3 is best-effort, hence `aql_path_not_in_template` is a Warning.
- **Re-emission / pretty-printing** — parse does not round-trip to AQL text in v1.

### Building-block independence (REQ-013)

`openehr/aql/parse/` and `openehr/aql/lint/` **MUST** be importable without `transport/`, `auth/`, `openehr/client/*`, or `openehr/serialize/`, and `lint` additionally **MUST NOT** import `openehr/validation`. Enforced by `TestAQLParseForbiddenImports` and `TestAQLLintForbiddenImports`.

- **Lives in:** [`openehr/aql/parse/`](../../openehr/aql/parse/), [`openehr/aql/lint/`](../../openehr/aql/lint/); bridge in [`openehr/validation/aql.go`](../../openehr/validation/aql.go)
- **Probes:** PROBE-028 — lint fixed query strings against the grammar profile (+ a compiled OPT for Layer 3) and assert a stable issue-code multiset.
- **Plan:** [`docs/plans/archive/2026-06-15-aql-lint.md`](../plans/archive/2026-06-15-aql-lint.md)

## REQ-110 — Template-driven validation beyond COMPOSITION

REQ-102's walker is **value-source-generic**: the compiled OPT drives traversal and the RM root is the value source, read property-by-property through `openehr/validation/rmread`. The SDK **MUST** expose that walker for **any** archetypeable RM root, not only `COMPOSITION` — the demographic **PARTY** hierarchy and the EHR-IM container roots — so a demographic or directory OPT validates through the same machinery as a clinical template.

### Surface

```go
// Generic entry — root is any RM LOCATABLE concrete the walker recognises.
func Validate(root any, c *templatecompile.Compiled) Result

// Typed convenience wrappers (delegate to Validate):
func ValidateComposition(comp *rm.Composition, c *templatecompile.Compiled) Result  // REQ-102
func ValidateDemographic(party rm.Party, c *templatecompile.Compiled) Result        // PERSON/ORGANISATION/GROUP/AGENT/ROLE
func ValidateFolder(folder *rm.Folder, c *templatecompile.Compiled) Result
func ValidateEHRStatus(status *rm.EHRStatus, c *templatecompile.Compiled) Result
```

`ValidateComposition` keeps its `nil_composition` guard for source compatibility, then delegates. A nil/typed-nil root yields `nil_root` (or the wrapper's `nil_party` / `nil_folder` / `nil_ehr_status`); a root whose concrete RM type does not match the OPT root surfaces `rm_type_mismatch` at `/`, never a silent pass.

### Covered roots

- **Demographic PARTY hierarchy:** `PERSON`, `ORGANISATION`, `GROUP`, `AGENT`, `ROLE`, plus the archetypeable sub-components walked in place or as roots — `ADDRESS`, `CONTACT`, `PARTY_IDENTITY`, `PARTY_RELATIONSHIP`, `CAPABILITY`.
- **EHR-IM roots:** `FOLDER` (directory trees, recursing `folders`) and `EHR_STATUS`.

### Implementation

The walker logic is unchanged; generalisation is a lockstep extension of the four closed routing sets — `rmTypeInfo` and `bmmSubtypes` (`openehr/validation/`), and `ReadSingle`/`ReadMultiple` per-type readers + `isTypedNilPointer` (`openehr/validation/rmread/`). The same change adds the primitive-bearing **DataValue leaf** readers (`DV_DATE`/`DV_TIME`/`DV_DATE_TIME`/`DV_DURATION`.`value`, `DV_BOOLEAN.value`, `DV_IDENTIFIER.id`, `DV_MULTIMEDIA` `media_type`/`size`) so a DV value encoded as a `C_COMPLEX_OBJECT` with an explicit `value` `C_PRIMITIVE_OBJECT` child binds and validates (REQ-103) rather than reporting a false `required`.

### Known limitations

- `DV_INTERVAL<T>` over `DV_ORDERED` is not yet type-matched by the walker (a DataValue gap, not demographic-specific; cf. the `Test_dv_interval_*` round-trip exclusions). A `DV_INTERVAL` instance under an interval-typed OPT slot surfaces `rm_type_mismatch`.
- Reference-typed attributes (`PARTY.roles`, `FOLDER.items` → `OBJECT_REF`/`PARTY_REF`) are addressable for existence/cardinality but their targets are not descended.

### Building-block independence (REQ-013)

`openehr/validation/` and `openehr/validation/rmread/` remain importable without `transport/`, `auth/`, `openehr/client/*`, or `openehr/serialize/` — enforced by `TestValidationForbiddenImports`. Decoding an instance for validation (canjson / canxml) is the caller's concern; `Validate` takes an in-memory root.

- **Lives in:** [`openehr/validation/validate.go`](../../openehr/validation/validate.go), [`openehr/validation/rmread/read.go`](../../openehr/validation/rmread/read.go)
- **Probes:** PROBE-074 — template-driven validation of non-COMPOSITION roots; asserts the issue-code multiset per (OPT, root) shape.
- **Plan:** [`docs/plans/archive/2026-06-17-validation-non-composition-roots.md`](../plans/archive/2026-06-17-validation-non-composition-roots.md)

---

## REQ-111 — Public compiled-template bridge

The compiled-template form (`templatecompile.Compiled`) is the argument every template-driven entry point takes: the composition builder (REQ-101 — `NewBuilder` / `NewSkeleton`), the RM instance synthesiser (REQ-107 — `Generate`), the validator (REQ-102 / REQ-110 — `Validate` and its typed wrappers), and the AQL static lint (REQ-109 — `lint.Options.Compiled`). Through v0.8.0 it was only constructable inside this module, so **none of those entry points was callable from an external module**.

The SDK **MUST** ship a public constructor that turns a parsed OPT into that compiled form without exposing any `internal/` package, so external consumers can drive the full parse → compile → build → validate pipeline through public packages alone.

### Surface

```go
// Package github.com/cadasto/openehr-sdk-go/openehr/templatecompile

// Compiled is the public, externally-constructable compiled template.
// It is a type alias of the internal compiled form, so values returned
// by Compile are accepted as-is by composition, instance, validation
// and aql/lint — REQ-111 adds no conversion and changes no behaviour.
type Compiled = <internal compiled form>

func Compile(opt *template.OperationalTemplate, opts ...Option) (*Compiled, error)

type Option func(*config)
func WithRMInfo(l rminfo.Lookup) Option       // custom RM-info source
func WithoutImplicitAttributes() Option        // OPT-declared attributes only

var ErrInvalidInput error  // re-export; errors.Is works across the boundary
var ErrPathNotFound error

// Introspection tree — also public, for form generation, path discovery,
// and custom mapping/validation. Aliases of the engine node types.
type CompiledNode = <internal compiled node>
type CompiledAttribute = <internal compiled attribute>
```

The committed public surface is `Compile`, `Compiled`, the introspection tree (`CompiledNode` / `CompiledAttribute`), the functional `Option`s, and the two sentinel errors — all aliases of the engine types, so a downstream package can navigate the compiled template (`Compiled.Root` / `NodeAt` → `CompiledNode.Attributes` → `CompiledAttribute.Children`) and hold the node types in its own signatures. Pre-1.0 the one area expected to change is multi-language term resolution (`CompiledNode.Term`'s `lang` parameter, REQ-105); the surface is otherwise stable. Engine-only helpers (e.g. `IsAOMPrimitiveShortName`, the raw slot include/exclude strings) stay internal.

The consuming packages reference the public `*templatecompile.Compiled` in their **exported** signatures (so the rendered API docs link the public package); their unexported code that needs the node-level types imports the internal engine directly. Because `Compiled` is a type alias, the two names denote the identical type and no conversion is needed.

### Placement (ADR 0010)

The constructor **MUST NOT** live in `openehr/template` (the natural home next to `ParseFile`), for two reasons:

1. **Import cycle.** The compile engine imports `openehr/template`; a `Compile` inside `openehr/template` would import the engine, forming `template → templatecompile → template`.
2. **REQ-100 stdlib-only contract.** REQ-100 mandates `openehr/template` import nothing from `openehr/rm/…`; compilation needs `openehr/rm/rminfo` for implicit-attribute injection.

It therefore lives in the sibling package `openehr/templatecompile`. This supersedes [ADR 0005](../adr/0005-compiled-template-foundation.md) §C2's `template.Compile` / `template.Compiled` proposal; see [ADR 0010](../adr/0010-public-compiled-template-bridge.md).

### Building-block independence (REQ-013)

`openehr/templatecompile/` **MUST** be importable without `transport/`, `auth/`, `openehr/client/*`, or `openehr/serialize/`. It imports `openehr/template`, `openehr/rm/rminfo`, and the internal compile engine only.

- **Lives in:** [`openehr/templatecompile/`](../../openehr/templatecompile/)
- **Verification:** unit tests in [`openehr/templatecompile/compile_test.go`](../../openehr/templatecompile/compile_test.go); the public-only acceptance proof (external-shape build → canjson round-trip → validate, plus `ValidateEHRStatus` reachability) in [`openehr/templatecompile/external_test.go`](../../openehr/templatecompile/external_test.go); and the runnable [`cmd/examples/compile-build-validate`](../../cmd/examples/compile-build-validate/) whose direct imports are public-only. No new PROBE — this is an API-reachability requirement, not a wire-conformance assertion (the builder round-trip itself is PROBE-023).
- **Plan:** [`docs/plans/2026-06-17-public-compiled-template-bridge.md`](../plans/2026-06-17-public-compiled-template-bridge.md)

