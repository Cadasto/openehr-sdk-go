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
| `ComplexObject` | `xsi:type="C_COMPLEX_OBJECT"` | `RMTypeName()`, `NodeID()`, `NodeName()` (template-level node name — the fixed `C_STRING` pinned on the `name` attribute, `""` when absent; [REQ-116](#req-116--template-level-node-naming-and-name-predicated-paths)), child `Attribute` list, optional occurrences |
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
    Policy      Policy
    Language    string                  // ISO 639-1; defaults from Compiled.Language()
    Territory   string                  // for COMPOSITION roots
    Composer    rm.PartyProxy           // required when root is COMPOSITION
    Now         time.Time               // clock for EVENT / context times
    UIDSource   func() *rm.HierObjectID // optional determinism hook for LOCATABLE.uid (nil = crypto/rand)
    ValueFill   ValueFill               // ExampleFill (default) or RandomFill
    ValueSource mrand.Source            // seeds RandomFill; nil = auto-seeded global
}

func Generate(ctx context.Context, c *templatecompile.Compiled, opts Options) (any, error)
func AsComposition(v any) (*rm.Composition, error)
func AsObservation(v any) (*rm.Observation, error)
// … closed set matching validation ContentItem + standalone archetype roots
```

`Generate` **MUST** return a root RM value satisfying the OPT's structural rules and REQ-103 primitive constraints. Specifically, `Minimal` materialises only attributes with existence lower ≥ 1 (plus BMM-mandatory implicit attrs); `Example` additionally populates every primitive leaf via `PrimitiveConstraint.ExampleValue()`. Multi-valued attributes are sized to `max(existence.lower, 1)` subject to AOM `cardinality.upper` when bounded; under `Minimal`, when optional archetype-root siblings share a `node_id`, the synthesiser emits only the first colliding sibling so validator node-id binding stays unambiguous (REQ-107). OPT-declared BMM generic RM types (e.g. `DV_INTERVAL<DV_QUANTITY>`) MUST resolve to the concrete Go typereg constructor before `rmwrite` attachment. `C_SINGLE_ATTRIBUTE` alternatives resolve first-child-wins (matching validation v2's first-alternative semantics).

Slot handling (v1): pinned archetype-root children under a slot are synthesised; pure `ARCHETYPE_SLOT` assertions resolve via the parsed REQ-104 include grammar when a safe example id can be derived, or via the RM-type-prefix fallback only when no include assertions were parsed — same compromise as validation slot-fit.

### Primitive-leaf value fill

`Policy` selects *which* nodes are materialised; an orthogonal **`ValueFill`** selects *how* primitive leaves are valued. The SDK **MUST** offer two fills: `ExampleFill` (default) populates each leaf with its REQ-103 `PrimitiveConstraint.ExampleValue` — a single representative value, byte-identical across calls for one OPT; `RandomFill` draws each leaf from within its constraint (in-range magnitudes, value-set-member codes, enumeration entries), valid by construction and varying between calls. A `ValueFill` other than `RandomFill` **MUST** degrade to `ExampleFill` rather than error.

`RandomFill` reproducibility is caller-controlled via **`Options.ValueSource`** (a `math/rand/v2.Source`): a fixed source makes leaf values byte-reproducible; `nil` draws from the auto-seeded package global so successive calls differ — mirroring the `UIDSource` determinism seam. A `Source` is not safe for concurrent use: each concurrent `Generate` **MUST** own its source (or leave it `nil` for the concurrency-safe global). The composition builder surfaces the seam as `composition.WithValueFill` / `composition.WithValueSource`.

**Deferred.** A third `medium` / `detail_level` structural level — a representative optional-subset fill between `Minimal` and full population — is planned but not delivered; it is **not** part of the v1 `ValueFill` contract. Tracked in the roadmap.

### Trust model

The compiled OPT is **authoritative for structure**. The RM graph is assembled attribute-by-attribute from compiled metadata; the generator never guesses paths from an empty composition. Primitive leaves come from `PrimitiveConstraint.ExampleValue()` (REQ-103), which guarantees `Validate(ExampleValue()) == nil` for bounded constraints. Optional OPT `<assumed_value>` / `<default_value>` (when compile captures them — a Phase 0 follow-up) **override** the factory.

The generator is **sound** (every output is valid against the OPT), not **complete** (it does not enumerate every valid instance — different policies may produce different but equally valid trees). Sound × validator-aligned ⇒ PROBE-027 cross-checks the contract.

### Trust model — phasing

Phases 0–3 landed: `ExampleValue()` on every `PrimitiveConstraint`; `internal/templateinstance/rmwrite/` inverse-of-rmread RM construction table; `openehr/instance/` synthesiser with `Generate` / `Policy` / `UIDSource` test-determinism seam / typed accessors for the closed root set; PROBE-027 implemented (Sandbox) covering `vital_signs.opt` + `clinical_note.opt` + the REQ-107 real-world corpus (`Referral Request.v1`, `Demonstration.v1`, `social`); `cmd/examples/generate-example/` worked example. The C_PRIMITIVE_OBJECT inner-`<item>` wire-parser fix + canjson-polymorphic `Composition.uid` emission landed via the [wire-parser plan](../plans/archive/2026-05-26-c-primitive-object-wire-parser.md) (archived); PROBE-023 now exercises the full marshal → unmarshal → re-marshal round-trip. Phase 4 (REQ-101 composition-builder integration delegating to `instance.Generate`) tracked in [`docs/plans/archive/2026-05-24-template-instance-example-generator.md`](../plans/archive/2026-05-24-template-instance-example-generator.md) (archived). REQ-104 slot-fill archetype-id stamping is landed for parsed include patterns that can be synthesized safely; when no includes were parsed the synthesiser uses `openEHR-EHR-<RMType>.example.v1` to satisfy the validator's RM-type-prefix heuristic.

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
| Deprecated `TOP` | `aql_deprecated_top` | Warning | A `SELECT TOP` clause **SHOULD** be replaced by `LIMIT` with `ORDER BY` — the modifier is deprecated from QUERY Release-1.1.0 and slated for removal ([§ REQ-118](#req-118--deprecated-select-top-clause-and-literal-source-text)). Fires once per query carrying a `TOP`. |
| `TOP` with `LIMIT` | `aql_top_with_limit` | Error | A query **MUST NOT** use `TOP` and the `LIMIT` clause together (QUERY Release-1.1.0 § 4.4.3). Fires in addition to `aql_deprecated_top`. |

SELECT-present-with-≥1-projection and FROM-present are guaranteed by a successful parse (the grammar requires both), so they raise no Layer-2 issue.

Two grammar-admitted, server-executable shapes are explicit **acceptances** of the alias-binding check; the resolution rules are specified normatively by [§ REQ-117 — Lint acceptance](#req-117--aql-expression-catalogue-completion). Codes, severities, and the collect-all contract are unchanged.

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
- **Re-emission / pretty-printing** — not part of this REQ; canonical re-emission from the structured AST is provided by [§ REQ-113](#req-113--execution-oriented-parsed-aql-ast) via [`(*Query).Emit`](../../openehr/aql/parse/query.go).

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

The committed public surface is `Compile`, `Compiled`, the introspection tree (`CompiledNode` / `CompiledAttribute`), the functional `Option`s, and the two sentinel errors — all aliases of the engine types, so a downstream package can navigate the compiled template (`Compiled.Root` / `NodeAt` → `CompiledNode.Attributes` → `CompiledAttribute.Children`) and hold the node types in its own signatures. Pre-1.0 the one area expected to change is multi-language term resolution (`CompiledNode.Term`'s `lang` parameter, REQ-105); the surface is otherwise stable. Everything reachable as a method on `Compiled` / `CompiledNode` / `CompiledAttribute` is committed (including the slot accessors `SlotIncludes` / `SlotExcludes` / `SlotRules` / `AllowsArchetypeID` / `ExampleSlotFillArchetypeID`); the compile engine and free helpers that are not methods on the exported types (e.g. `IsAOMPrimitiveShortName`) stay internal.

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
- **Plan:** [`docs/plans/archive/2026-06-17-public-compiled-template-bridge.md`](../plans/archive/2026-06-17-public-compiled-template-bridge.md)

## REQ-106 — WebTemplate JSON export

**WebTemplate** is a consumer-facing, UI-oriented projection of an operational template: a lossy, flattened JSON view listing each node's stable `id`, cardinality, AQL path, and the leaf **inputs** a form must render. It is a **vendor de-facto** serialisation (Better → EHRbase), **not** a normative openEHR artefact — only the downstream FLAT/STRUCTURED *serialization* it enables is standardised (ITS-REST Simplified Formats). The SDK treats it as a public contract it MUST keep stable for consumers, mirrored from a single locked reference implementation ([ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md)).

The SDK **MUST** provide a WebTemplate JSON export that projects a compiled operational template — the public `*templatecompile.Compiled` (REQ-111) — into the **EHRbase `openEHR_SDK` v2.3** WebTemplate shape. The export **MUST** consume only the compiled form (never `.oet` / `.opt` / `.t.json` authoring artefacts) and **MUST NOT** mutate it.

### Surface

```go
// Package github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate

// Build projects a compiled OPT into the typed WebTemplate tree.
func Build(c *templatecompile.Compiled, opts ...Option) (*WebTemplate, error)

// Marshal is Build followed by deterministic JSON encoding.
func Marshal(c *templatecompile.Compiled, opts ...Option) ([]byte, error)

type Option func(*config) // reserved — no options in the current slice (single-language compiled input; fixed schema version)
```

`Build` returns the typed tree for callers that post-process before encoding; `Marshal` is the common path. A nil or empty compiled input, or an unresolvable default language, **MUST** return an error (never panic).

### Output shape

The exported root **MUST** carry `templateId`, `version` (the string `"2.3"`), `defaultLanguage`, `languages`, and a `tree` of nodes. Each node **MUST** carry `id`, `rmType`, `min`, `max` (with `-1` denoting unbounded), and `aqlPath`; and where the compiled template supplies them, `name` / localized names, `nodeId`, `inputs`, and `children`. JSON field names are camelCase. Encoding **MUST** be deterministic: the same compiled template **MUST** produce byte-identical output across runs and SDK patch releases (fixed field order; map keys sorted).

The exported tree is **not** a 1:1 mirror of the compiled OPT; it follows the reference implementation's node model. The transform **MUST**: keep `COMPOSITION`, the ENTRY types (`OBSERVATION` / `EVALUATION` / `INSTRUCTION` / `ACTION` / `ADMIN_ENTRY`), `EVENT_CONTEXT`, and `CLUSTER` as nodes; keep the event types (`EVENT` / `POINT_EVENT` / `INTERVAL_EVENT`) as nodes **except** an abstract `EVENT` constrained to at most one occurrence, which is **lifted** — the reference emits no node for such a degenerate wrapper, while retaining its `events[…]` path segment on descendants and its synthesized `time` leaf (a repeating `EVENT`, and any concrete `POINT_EVENT` / `INTERVAL_EVENT` even at `max=1`, stays a node — both halves of the discriminator are measured across the vendored goldens); **collapse** each `ELEMENT` into a single leaf carrying the constrained value type as `rmType`, the ELEMENT's node id as `nodeId`, and the ELEMENT path extended by `/value` as `aqlPath`; **drop** the pure structural wrappers (`HISTORY` and the `ITEM_STRUCTURE` family — `ITEM_TREE` / `ITEM_LIST` / `ITEM_SINGLE` / `ITEM_TABLE`) as nodes while **folding their node predicates into the `aqlPath`** of the descendants they enclose; and emit RM-attribute values that carry data directly (e.g. `EVENT_CONTEXT.start_time` / `setting`, `EVENT.time`, the ENTRY `language` / `encoding` / `subject`, `INTERVAL_EVENT.math_function` / `width`) as leaves whose `id` is the attribute name. The `aqlPath` therefore carries every archetype-id and at-code node predicate along the retained path.

### `id` generation (ADR 0014)

The node `id` is the FLAT-path segment consumers bind to, so its stability and cross-implementation fidelity are the export's load-bearing property. The `id` **MUST** mirror the locked EHRbase reference: a lower-snake sanitisation of the node's default-language display name. The exact normalisation is **derived from the vendored reference fixture** and pinned by tests — the SDK **MUST NOT** invent an id scheme, because a bespoke scheme would break FLAT-path interoperability with existing tooling.

For a node that pins a **template-level name**, that name is the display name the `id` derives from — not the archetype's concept term, which is shared by every occurrence of a reused archetype and so cannot distinguish siblings; see [REQ-116](#req-116--template-level-node-naming-and-name-predicated-paths), which owns the naming rule.

A pinned name is the disambiguator wherever one exists, so no suffixing is needed for it — and none appears in any vendored reference golden. Where siblings would **still** collide — no pinned name, and one shared display name, as two ELEMENTs both sanitising to `dv_text` do in the upstream FLAT-conformance template — the export **MUST** apply the reference's ordinal fallback: the first claimant keeps the bare `id` and the second and later claimants take the next **free** ordinal spelling (`dv_text`, `dv_text2`, `dv_text3`) — "free" because a sibling may legitimately sanitise to `dv_text2` on its own, and renaming onto it would recreate the collision. The two-claimant spelling is fixed by the reference, not chosen: the upstream FLAT bodies for that template key those two nodes `…/conformance_action/dv_text` and `…/conformance_action/dv_text2`; the next-free extension beyond it is this SDK's conservative reading, pinned by table test. Emitting duplicate sibling `id`s is **never** permitted; an export that still cannot make siblings unique **MUST** fail with a typed error rather than emit an ambiguous tree.

### `inputs` (core clinical subset)

Each `ELEMENT` value constraint (via REQ-103 primitive constraints) **MUST** map to `inputs[]` for the core clinical datatypes: `DV_TEXT` (one `TEXT` input), `DV_CODED_TEXT` (a `code` CODED_TEXT input with `list` / `listOpen` / `terminology`), `DV_QUANTITY` (`magnitude` DECIMAL + `unit` CODED_TEXT with validation), `DV_COUNT` (INTEGER/COUNT), `DV_ORDINAL` (an ordinal `list`), `DV_DATE_TIME` / `DV_DATE` / `DV_TIME` (temporal with pattern validation), `DV_BOOLEAN` (BOOLEAN), `DV_DURATION` (pattern-ordered INTEGER component fields), and `DV_PROPORTION` (`numerator` + `denominator`, with the percent-kind–derived denominator bound). Datatypes outside this subset (e.g. `DV_MULTIMEDIA`, `DV_PARSABLE`, `DV_IDENTIFIER`, `DV_INTERVAL`) **MUST** emit the node **without** `inputs` and **MUST NOT** error — the omission is a documented gap, recorded so consumers can distinguish "no inputs" from "unsupported".

### Conformance and deviations

Conformance against the reference is **structural, not byte-exact**: PROBE-075 compares the SDK output to the vendored EHRbase fixture on the `id` set, `rmType`, `aqlPath`, `min`/`max`, and per-node input `suffix`/`type` extended with coded/ordinal list values, `listOpen`, `terminology`, temporal validation patterns, and numeric validation ranges. Any structural difference on that pinned surface is a failure; the following categories of divergence are **permitted deviations** and **MUST NOT** be treated as conformance failures:

1. **Field-level presentation** — JSON field ordering, absent optional fields, and localized-string packaging (`localizedName` / localized maps emitted for the compiled template's single document language only).
2. **Reference-only node metadata** — `termBindings`, `annotations`, and the `inContext` flag on synthesized RM-attribute leaves.
3. **Input-content omissions** — `defaultValue`; DV_DURATION per-field ranges; DV_QUANTITY `precision`; per-unit and multi-unit magnitude validation; list `label` for external-terminology codes; `localizedLabels` and per-item `termBindings`.
4. **Local terminology** — `terminology` is emitted only for external bindings (e.g. `openehr`); the archetype-internal `local` value is omitted, mirroring the reference.
5. **Fixture-outlier `min` on in-context leaves** — where a vendored golden marks synthesized RM-attribute leaves `min=1` and the OPT constrains no `existence`, the export **MAY** emit `min=0`: the `GECCO_Diagnose` golden is the sole such outlier across the three vendored references (14 nodes), and the other two agree with `min=0`. Tolerated by pinned count, not waived (see the `id` generation section for sibling disambiguation, which is no longer deferred).

The exact per-field enumeration behind these categories — the informative catalogue pinned by the parity tests — is maintained in [`openehr/template/webtemplate/deviations.md`](../../openehr/template/webtemplate/deviations.md) beside those tests. This section is the normative contract; that file elaborates it.

The media type for the format is `application/openehr.wt+json` (documented for consumers). Emitting the export over a REST endpoint / content negotiation is **out of scope** for this REQ — the package produces the bytes only. Also out of scope: the WebTemplate → OPT round-trip (the format is lossy by design); the Better camelCase `id` variant; multi-version output; and the shared simplified-template model abstraction (extracted with REQ-053 when a second consumer exists — [simplified-formats umbrella](../plans/2026-06-23-simplified-formats.md)).

Templates that **reuse one archetype under a multi-valued slot** (name-distinguished instances) **are exported**, closing what was this REQ's last deferral. Two things had to land, both specified by [REQ-116](#req-116--template-level-node-naming-and-name-predicated-paths) ([ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md)): `openehr/templatecompile` admits shared-path subtrees, and node **identity** now comes from the template-level node name — with `aqlPath` carrying the matching name predicate — so each reused sibling is distinct by construction instead of colliding on the shared concept term.

### Building-block independence (REQ-013)

`openehr/template/webtemplate/` **MUST** be importable without `transport/`, `auth/`, `openehr/client/*`, or `openehr/serialize/`. It imports `openehr/templatecompile` (the compiled input, REQ-111), `openehr/template/constraints` (primitive constraints, REQ-103), `internal/templatecompile` (the shared REQ-116 name-predicate quoting — one rule for both path builders), and the standard library only.

- **Lives in:** [`openehr/template/webtemplate/`](../../openehr/template/webtemplate/).
- **Verification (on delivery):** unit tests for id-generation, per-datatype `inputs` mapping, and tree shape; round-trip goldens per fixture OPT (determinism); and PROBE-075 structural parity against three vendored EHRbase references — `constrain_test` plus the two REQ-116 archetype-reuse oracles. Catalogued in [`conformance.md`](conformance.md).
- **Plan:** [`docs/plans/2026-05-22-webtemplate-export.md`](../plans/archive/2026-05-22-webtemplate-export.md).

## REQ-116 — Template-level node naming and name-predicated paths

An OPT node may pin its runtime `LOCATABLE.name` by constraining the `name` attribute to a fixed value — in ADL 1.4 XML, a `C_STRING` whose raw wire `list` holds **exactly one non-blank entry** (`<item xsi:type="C_STRING"><list>Husten</list></item>`), taken trimmed. Fixedness is a property of what the OPT declared, so it is judged on the wire list *before* any blank-entry normalisation: a multi-entry list is a choice even when all but one entry is blank, and a lone blank entry pins nothing. This **template-level node name** is distinct from the *archetype's* concept term: a template that fills a slot with the same archetype more than once gives each occurrence its own name, while the archetype concept is identical across all of them. It is the only thing that distinguishes those siblings, and both the reference WebTemplate `id` (REQ-106) and the AQL paths addressing them depend on it.

The SDK **MUST** parse the template-level node name and expose it on the parsed OPT node ([REQ-100](#req-100--adl-14-operational-template-opt-parse-and-paths)), **MUST** carry it through the compiled tree so consumers of the public bridge can read it ([REQ-111](#req-111--public-compiled-template-bridge)), and **MUST** use it as the disambiguator described below. Where a node constrains no fixed name, the SDK **MUST** treat the name as absent rather than substituting the archetype concept term.

### Name predicates in AQL paths

An AQL node predicate **MAY** carry a name alongside the archetype node id — `[openEHR-EHR-SECTION.adhoc.v1,'Symptome']` — which is how openEHR addresses one of several siblings that share an archetype node id.

Where a node pins a template-level name, the compiled AQL path **MUST** carry the name predicate on that node's segment. The trigger is the **pinned name alone**, not sibling collision: the reference emits the predicate on every named node — including a sole child, and siblings whose archetype node ids already differ — so a rule conditioned on siblings sharing a node id emits too few. Where a node pins **no** name, the SDK **MUST NOT** invent a predicate; this includes siblings that share an archetype node id and pin no distinct names, which legitimately share a path and are distinguished positionally, and whose compiled form **MUST** retain each of them rather than discarding the collision.

> **Established against the reference (2026-07-30), superseding an earlier collision-conditioned reading.** Verified across the three vendored goldens: the set of names a golden uses in predicates is exactly the set of fixed `C_STRING` names its OPT pins — `GECCO_Diagnose` 6 pinned and 6 distinct predicate names, `Corona_Anamnese` 24 and 24, with no predicate name absent from the pinned set — while `constrain_test` pins **no** name and carries **zero** predicates across its 104 nodes, which is why [PROBE-075](conformance.md#probe-075--webtemplate-structural-parity) holds 104/104 without any of this. `GECCO_Diagnose` predicates all three of its `/content` children although their archetype node ids are **distinct**, and predicates a sole `CLUSTER.anatomical_location.v1` child.

> **Downstream consumers.** Path shape is consumed by composition validation ([REQ-102](#req-102--composition-validation)), the instance generator ([REQ-107](#req-107--template-driven-rm-instance-example-generator)), and FLAT/STRUCTURED path resolution ([REQ-053](wire.md#req-053)). A change here **MUST** keep those conformant: paths whose segments pin **no** template-level name **MUST** be byte-identical to those emitted before. Paths through a named node **do** change shape — that is the consumer-visible break this requirement introduces, and it reaches every vendored OPT that pins a name, not only the archetype-reuse ones.

### Acceptance

- An OPT whose sibling archetype roots reuse one archetype id compiles, and each sibling is retrievable at a distinct name-predicated path.
- A WebTemplate built from such a template emits one distinct `id` per sibling, derived from the template-level name, with no collision error (REQ-106).
- A template that pins **no** template-level name produces paths byte-identical to the pre-change output, and the existing WebTemplate parity fixture (`constrain_test`, 0 predicates) holds at 104/104.
- Every node that pins a name is retrievable at its predicated path, whether or not it shares an archetype node id with a sibling.
- Siblings that pin **no** name and share a display name still export: the second and later claimants take the reference's ordinal (`dv_text`, `dv_text2`), so no template fails on identity. The upstream FLAT-conformance template exercises this, and its upstream FLAT bodies fix the expected spelling.
- Both vendored REQ-116 oracles (`Corona_Anamnese`, `GECCO_Diagnose`) build a WebTemplate whose node `id` and `aqlPath` sets match their vendored reference goldens, within the documented deviations. (`conformance-ehrbase.de.v0`, which [PROBE-086](conformance.md#probe-086--upstream-flat-serialisation-parity) blocks on, is vendored without a WebTemplate golden, so it is a build-succeeds check only until one is vendored.)

## REQ-112 — Template-less Reference Model validation floor

The validators introduced by REQ-102 and generalised by REQ-110 are **template-driven** — every entry point accepts a `*templatecompile.Compiled` as the authoritative driver. A consumer persisting RM roots that bind to no operational template (FOLDER, EHR_STATUS, EHR_ACCESS, and untemplated demographic PARTY on a write path) has no SDK call to assert RM conformance. The strongest substitute today is a strict `canjson` typed decode, which proves JSON↔type correctness but **not** RM invariants — mandatory-attribute omissions decode cleanly, as do `DV_INTERVAL` lower>upper, empty `CODE_PHRASE.code_string`, and `DV_QUANTITY.precision<0`.

The SDK **MUST** expose a **template-less RM validation floor** beneath the OPT-driven path — an entry point that walks any RM root with the BMM as its sole driver and reports `Issue`s for (a) RM-mandatory attribute breaches and (b) per-RM-type invariants on the leaves it touches. The OPT-driven path stays the authoritative template-conformance layer; this floor is what runs when no template applies (template validity implies RM validity, so the two compose).

### Surface

```go
// Generic entry — any RM concrete in the v2 closed set.
func ValidateRM(root any) Result

// Typed convenience wrappers (delegate to ValidateRM):
func ValidateRMFolder(folder *rm.Folder) Result
func ValidateRMEHRStatus(status *rm.EHRStatus) Result
func ValidateRMEHRAccess(access *rm.EHRAccess) Result
func ValidateRMDemographic(party rm.Party) Result   // PERSON / ORGANISATION / GROUP / AGENT / ROLE

// Presence-aware EHR_STATUS entry (REQ-112) — decides value-typed
// mandatory presence from JSON keys; see § Value-typed mandatory presence.
func ValidateRMEHRStatusBytes(data []byte) Result
```

The typed wrappers carry the same nil-guard contract REQ-110 introduced: `nil_folder` / `nil_ehr_status` / `nil_ehr_access` / `nil_party` distinguish wrapper-side guards from the generic `nil_root` that `ValidateRM(nil)` emits. A Go value outside the v2 closed RM set surfaces `rm_type_unknown` at `/`; the floor cannot descend further but does not panic.

### Driver

The floor walker is a **second driver** alongside the template-driven walker — separate type, shared closed-RM-set helpers (`rmTypeInfo` / `describeRMType` / `rmread.ReadSingle` / `rmread.ReadMultiple`). It consumes [`openehr/rm/rminfo`](../../openehr/rm/rminfo) for the structural knowledge:

- `RequiredAttributes(rmType)` drives the per-node required-set check. A single-valued attribute absent (or typed-nil) emits `required`; a multi-valued attribute absent emits `required`, present-but-empty emits `cardinality`.
- `AttributeNames(rmType)` enumerates the descend candidates (the per-type attribute list extends `Lookup` with this method — additive on the existing surface).
- `AttributeRMType` / `IsContainer` carry the declared shape for each attribute. The walker recurses into every present attribute, using the value's *runtime* RM type (from `rmTypeInfo`) when known so the invariant evaluators dispatch correctly across Liskov substitution (`DV_TEXT` carrying `DV_CODED_TEXT`, `PARTY_IDENTIFIED` carrying `PARTY_RELATED`, etc.).

### Per-RM-type invariant catalogue (v1 first cycle)

The catalogue is intentionally small; the value is in the structural required-set walk above, plus a focused set of leaf invariants that the canjson lenient decode accepts but the RM forbids:

- **CODE_PHRASE** — `code_string` non-empty.
- **DV_QUANTITY** — `precision`, when set, non-negative.
- **DV_INTERVAL** — `lower` ≤ `upper` when both bounds are numerically comparable (DV_QUANTITY / DV_COUNT) and neither side is unbounded. Other DVOrdered bound types (DV_DATE, DV_TIME, …) carry richer comparison semantics that integrate with the REQ-123 temporal helpers in a follow-up cycle.
- **OBJECT_REF / PARTY_REF / ACCESS_GROUP_REF / LOCATABLE_REF** — `type` and `namespace` non-empty. The `id` floor is covered by the required-set walk (the field is RM-mandatory).

**Known gap — `EVENT_CONTEXT.setting` (`Setting_valid`).** The RM constrains `setting.defining_code` to a member of the
openEHR terminology's `setting` group (`Terminology(Terminology_id_openehr).has_code_for_group_id(Group_id_setting, …)`).
The floor does **not** evaluate it, because doing so needs the openEHR terminology group tables, which the floor
deliberately does not carry (see *Trust model* below — terminology binding is out of scope). The consequence is real and
was observed: an RM-invalid, archetype-locally-coded setting passed the floor clean and was caught only downstream, by
the REQ-053 FLAT encoder refusing to represent it. Closing this needs the terminology tables as an input to the floor
and is deferred to a follow-up cycle; until then `Setting_valid` is enforced only at the wire boundary
([wire.md § REQ-053](wire.md#req-053)), and REQ-107's generator pins an `openehr`-coded default so the SDK cannot
originate the invalid shape.

Catalogue additions follow [ADR 0001](../adr/0001-bmm-version-bump-runbook.md) — adding a new BMM concrete that needs a leaf invariant requires editing the closed switch in `rmfloor_adapters.go` and adding the evaluator. Each invariant emits `Issue.Code = "rm_invariant"`; consumers dispatch on the code as with the rest of the issue taxonomy.

### Trust model

The floor is the structural RM-only layer. It does **not** evaluate:

- archetype-level or template-level constraints (that is REQ-102 / REQ-110);
- terminology binding or external-code validation;
- semantic validity beyond the BMM and the explicit invariant catalogue.

These exclusions are by design: a CDR may layer template-driven validation on top of the floor, or run the floor alone for resources where no template applies.

### Value-typed mandatory presence

The required-set walk reads presence from the decoded Go value, which cannot detect an omitted **value-typed** mandatory attribute — a Go zero value is indistinguishable from an absent one. `EHR_STATUS.subject` is the case: typed `rm.PartySelf`, a value struct whose only field (`external_ref`) is optional, so an omitted subject and a valid bare `{"_type":"PARTY_SELF"}` decode to the identical zero value. Interface- / pointer- / slice-typed mandatories (e.g. `name`, typed `rm.DVTextLike`) are unaffected — nil is a reliable absence signal — as are the value-level invariants, which inspect fields rather than presence.

The floor closes this by deciding presence from the **source JSON key set** rather than the Go zero value:

```go
func ValidateRMEHRStatusBytes(data []byte) Result
```

`ValidateRMEHRStatusBytes` decodes the EHR_STATUS, runs the value-based `ValidateRMEHRStatus` floor, and additionally emits `required` at `/subject` when the top-level `subject` key is absent from `data` — or present but JSON `null`, which does not satisfy the mandatory attribute and decodes to the same zero `PartySelf`. A supplied subject — even the bare form — yields no spurious `required`; a non-object or undecodable input surfaces a single `invalid_shape` at `/`. The value-based `ValidateRMEHRStatus(*rm.EHRStatus)` is retained; its docstring documents the value-typed-subject blind spot and points to the `…Bytes` entry. Per REQ-013 the decode uses the standard library, not `openehr/serialize/canjson` — the RM types carry their own `UnmarshalJSON`.

### Building-block independence (REQ-013)

`openehr/validation/` continues to import only `openehr/rm`, `openehr/rm/rminfo`, `openehr/template`, `openehr/template/constraints`, `openehr/templatecompile`, `openehr/validation/rmread`, and the internal compile-engine — REQ-112's additions are local to the package. The forbidden-import set is unchanged and is enforced by `TestValidationForbiddenImports`.

- **Lives in:** [`openehr/validation/rmfloor.go`](../../openehr/validation/rmfloor.go) + [`openehr/validation/rmfloor_adapters.go`](../../openehr/validation/rmfloor_adapters.go) + [`openehr/validation/rmfloor_bytes.go`](../../openehr/validation/rmfloor_bytes.go) (the presence-aware EHR_STATUS entry); the closed-RM-set helpers (`rmTypeInfo` / `describeRMType`) and the rmread layer are shared with REQ-102 / REQ-110.
- **Verification:** unit pins in [`openehr/validation/rmfloor_test.go`](../../openehr/validation/rmfloor_test.go) — required-set absences (FOLDER.name missing), the four named per-type invariants, the unbounded-skip negative, and the nil-guard contract on every typed wrapper. The unit-test cassette matrix is the first-cycle verification; a dedicated PROBE-077 against vendored cassettes is deferred to a follow-up cycle. Value-typed mandatory presence (EHR_STATUS.subject) is pinned by **PROBE-081** in [`openehr/validation/rmfloor_bytes_test.go`](../../openehr/validation/rmfloor_bytes_test.go).
- **Plan:** [`docs/plans/archive/2026-06-29-rm-floor-validation.md`](../plans/archive/2026-06-29-rm-floor-validation.md) — REQ-112 (archived after PR #57).

---

## REQ-113 — Execution-oriented parsed AQL AST

REQ-109's [`openehr/aql/parse`](../../openehr/aql/parse) returns a lint-shaped [`Document`](../../openehr/aql/parse/parse.go) that flattens the FROM/CONTAINS tree to a class set and erases the WHERE expression structure: the lint contract reasons over the *set* of bound classes and the *set* of paths, not their containment shape or the operator tree. An execution consumer — a server lowering AQL to SQL, a planner picking up CONTAINS nesting, a query rewriter — needs to read the *structure*. The construction-side [`aql.Builder`](../../openehr/aql/builder.go) (REQ-055) is the write-side mirror of that need; until REQ-113 the read side had no symmetric surface.

The SDK **MUST** expose a **stable, generated-type-free, readable** structured AQL AST: a `string → structured query` entry point whose result a consumer can traverse without importing `openehr/aql/parse/gen` or any `internal/` package. The AST **MUST** preserve containment nesting, the WHERE operator/value tree, SELECT function/aggregate wrappers and aliases, ORDER BY direction, and LIMIT / OFFSET values. The WHERE and Value vocabularies are SHARED with the construction side — `aql.Comparison` / `aql.Junction` / `aql.NotExpr` / `aql.ExistsExpr` / `aql.MatchesExpr` / `aql.LikeExpr` / `aql.ParamValue` / `aql.StringValue` / `aql.IntValue` / `aql.RealValue` / `aql.BoolValue` are populated by both Builder and Parse: one model, two directions.

### Surface

```go
// Package github.com/cadasto/openehr-sdk-go/openehr/aql/parse

// Tier 2 — the target read AST.
func ParseQuery(q string) (*Query, error)              // (*Query, aql.ErrIncompleteAST) on catalogue gap
func (d *Document) Query() *Query                       // best-effort partial AST
func (d *Document) QueryErr() error                     // aql.ErrIncompleteAST diagnostic, nil otherwise

type Query struct {
    Select  SelectClause
    From    FromClause
    Where   aql.WhereExpr  // nil when no WHERE clause
    OrderBy []OrderTerm
    Limit   LimitExpr      // nil when no LIMIT
    Offset  LimitExpr      // nil when no OFFSET
}
func (q *Query) Emit() (string, error)                  // refuses (returns aql.ErrIncompleteAST) on an extractor-incomplete AST

// LIMIT / OFFSET — sealed union of literal and parameter forms.
type LimitExpr  interface { /* sealed */ }
type IntLimit   struct { N int }                        // `LIMIT 50`
type ParamLimit struct { Name string }                  // `LIMIT $rows`

// SELECT
type SelectClause struct {
    Distinct bool
    Star     bool
    Items    []SelectItem
}
type SelectItem  struct { Expr SelectExpr; Alias string }
type SelectExpr  interface{ isSelectExpr() }
type PathExpr    struct { IdentifiedPath }
type FunctionCall struct { Name string; Args []SelectExpr; Distinct, Star bool }

// FROM / CONTAINS — nested containment tree
type FromClause   struct { Root ClassExpr; Contains *Containment }
type Containment  struct { Class ClassExpr; Children []Containment; ChildJoin ContainsJoin; Negated bool }
type ContainsJoin int  // ContainsAnd / ContainsOr

// ORDER BY
type OrderTerm struct { Path IdentifiedPath; Dir OrderDir }
type OrderDir  int  // OrderAsc / OrderDesc
```

The shared `aql.Value` vocabulary additionally exposes [`aql.NullValue`](../../openehr/aql/value.go) (typed NULL sentinel) so the unquoted `NULL` keyword round-trips without colliding with a `StringValue{"NULL"}`. [`aql.ErrIncompleteAST`](../../openehr/aql/errors.go) is the sentinel surfaced by `ParseQuery` / `Document.QueryErr` and is also returned by `(*Query).Emit` when the AST came from an incomplete extraction.

Tier 1 — the cheap interim — exposes the validated ANTLR tree via [`(*Document).Tree`](../../openehr/aql/parse/parse.go) (return type `gen.ISelectQueryContext`, explicitly unstable). It removes the re-parse cost for consumers already recursing the generated parser but does not solve the generated-coupling concern; Tier 2 is the stable read AST.

The WhereExpr vocabulary on the construction side gains [`aql.NotExpr`](../../openehr/aql/where.go) / [`aql.ExistsExpr`](../../openehr/aql/where.go) / [`aql.MatchesExpr`](../../openehr/aql/where.go) / [`aql.LikeExpr`](../../openehr/aql/where.go) (and their `Not` / `Exists` / `Matches` / `Like` constructors) so the parser populates the same shapes the Builder constructs. [`aql.FormatWhere`](../../openehr/aql/where.go) is the public read-side mirror of the internal `.expr()` emitter — used by `(*Query).Emit()` to render the structured AST back to canonical AQL.

### Structured path access

Two path-bearing sub-structures are exposed as parsed structure, not only raw text, so an execution consumer reads them without re-tokenizing AQL grammar the SDK already parsed once:

- **`ClassExpr.PredicateComparison`** — a class *standing* predicate (e.g. `EHR e[ehr_id/value = $x]`) is exposed as an optional `*aql.Comparison` (`{path, operator, value}`, reusing the shared vocabulary) when it is a simple comparison; it is nil for an archetype-HRID predicate (on `ClassExpr.Archetype`), a version predicate, or a non-scalar / complex standing predicate — so a comparison is distinguishable from a non-comparison. The comparison's `ParsedPath` carries the relative left-hand path's structured `Segments` with an **empty `Alias`** (a class-predicate path binds no FROM alias); the verbatim `ClassExpr.Predicate` text is retained.
- **`aql.Comparison.ParsedPath`** — a comparison carries the structured path (`*aql.IdentifiedPath`: alias + segments) alongside the raw `Path` string, populated by the parser. A WHERE comparison sets the alias root; a class standing predicate sets `Alias == ""` (relative path). It is nil on the write side and for a path shape the parser does not structure. `ParsedPath.Raw` equals `Path`.

To carry the structured path on `aql.Comparison` without a package cycle (`aql` cannot import `openehr/aql/parse`), the shared path vocabulary — [`aql.IdentifiedPath`](../../openehr/aql/path.go) and `aql.PathSegment` — lives in `openehr/aql`. `parse.IdentifiedPath` embeds `aql.IdentifiedPath` and adds the parse-only `Clause` / source `Position` (promoted fields keep existing access unchanged); `parse.PathSegment` re-exports `aql.PathSegment`. Emission uses `Comparison.Path` and the verbatim class `Predicate`, so the round-trip property below is unaffected by the structured fields.

### Round-trip property

For any AQL query the parser accepts and the v1 emitter catalogue supports:

```
Emit(ParseQuery(Emit(ParseQuery(x)))) == Emit(ParseQuery(x))
```

The first emit normalises whitespace, keyword casing, optional defaults (ASC), and clause ordering against the canonical write form; the second parse-emit MUST be a fixed point. The buildable-grammar equivalent of [PROBE-020](conformance.md#probe-020--aql-builder-string-stability).

### Trust model

The structured AST is **syntax-faithful for the catalogue**: across the buildable grammar plus the parser-only shapes (`Not` / `Exists` / `Like` / `Matches`) it carries the source path text verbatim (`IdentifiedPath.Raw`); function names are normalised to upper case (`count` → `COUNT`) so emission produces canonical AQL regardless of source casing. It does **not** evaluate:

- archetype / template constraints (that is REQ-102 / REQ-110);
- terminology binding;
- semantic validity beyond the SDK grammar profile (the server remains the execute-time authority, [PROBE-021](conformance.md#probe-021--aql-parse-error-mapping)).

**Catalogue gaps** (shapes the grammar accepts but the structured extractor does not model) surface as [`aql.ErrIncompleteAST`](../../openehr/aql/errors.go) from [`parse.ParseQuery`](../../openehr/aql/parse/parse.go) / [`Document.QueryErr`](../../openehr/aql/parse/parse.go), and a partial AST refuses to render through [`(*Query).Emit`](../../openehr/aql/parse/query.go) (same error) so the loss is never silently emitted as canonical text.

The v1 gap list is closed by [§ REQ-117](#req-117--aql-expression-catalogue-completion), which also specifies the residual gaps and the defensive-branch rule normatively, as narrowed by [§ REQ-118](#req-118--deprecated-select-top-clause-and-literal-source-text) (the `top` clause is in-catalogue; one residual remains) — see those sections rather than restating them here. The buildable grammar (everything `aql.Builder` constructs) is in-catalogue by construction.

### Building-block independence (REQ-013)

`openehr/aql/parse/` MUST stay importable without `transport/`, `auth/`, `openehr/client/*`, or `openehr/serialize/` — unchanged from REQ-109. The forbidden-import set is enforced by `TestAQLParseForbiddenImports`. `Query.Emit` reaches `openehr/aql` (the shared vocabulary) which is itself a building block.

- **Lives in:** [`openehr/aql/parse/parse.go`](../../openehr/aql/parse/parse.go) (entry), [`openehr/aql/parse/query.go`](../../openehr/aql/parse/query.go) (AST + emitter), [`openehr/aql/parse/extract_query.go`](../../openehr/aql/parse/extract_query.go) (translator from the validated tree). Construction vocabulary in [`openehr/aql/where.go`](../../openehr/aql/where.go) and [`openehr/aql/value.go`](../../openehr/aql/value.go).
- **Verification:** structural pins in [`openehr/aql/parse/query_test.go`](../../openehr/aql/parse/query_test.go) (extraction shape across SELECT / FROM / CONTAINS / WHERE / ORDER BY / LIMIT, including COUNT(*), COUNT(DISTINCT), NOT CONTAINS, BoolValue, NullValue, ParamLimit, standing predicate, ParamArchetype, VERSION predicate) and the round-trip property in [`openehr/aql/parse/roundtrip_test.go`](../../openehr/aql/parse/roundtrip_test.go) (87 idempotence cases + 43 canonical-input preservation cases across the catalogue, plus the residual-gap suite asserting ParseQuery and Emit both surface `aql.ErrIncompleteAST` for the residual gap (an unrepresentable numeric literal, including an out-of-range `TOP` count) — the corpus grew under [§ REQ-117](#req-117--aql-expression-catalogue-completion), which pins the closed shapes as PROBE-087, and again under [§ REQ-118](#req-118--deprecated-select-top-clause-and-literal-source-text), which moved the `top` clause into the catalogue). Vocabulary introspection in [`openehr/aql/introspect_test.go`](../../openehr/aql/introspect_test.go). Structured standing-predicate + WHERE-path access (REQ-113) is pinned by **PROBE-082** in [`openehr/aql/parse/structured_test.go`](../../openehr/aql/parse/structured_test.go). The runnable [`cmd/examples/aql-parse-structured`](../../cmd/examples/aql-parse-structured/) demonstrates a consumer walk over the structured AST without any `parse/gen` or `internal/` imports.
- **Plan:** [`docs/plans/archive/2026-06-29-aql-execution-ast.md`](../plans/archive/2026-06-29-aql-execution-ast.md) — REQ-113 (archived after PR #58).


## REQ-117 — AQL expression-catalogue completion

Consumers building AQL execution engines and conformance tooling on the structured AST ([§ REQ-113](#req-113--execution-oriented-parsed-aql-ast)) need the catalogue to cover the **whole SDK grammar profile**, not a subset: every v1 catalogue gap forces a consumer to refuse the statement wholesale (`aql.ErrIncompleteAST` is fail-closed by design), so each gap is a query shape no downstream engine can accept even when its own execution layer could. The same consumers author benchmark and conformance corpora through the builder, which could not express containment shapes the grammar (and the parse side) already admit.

### Structured-AST catalogue (extends REQ-113)

[`parse.ParseQuery`](../../openehr/aql/parse/parse.go) MUST model — without `aql.ErrIncompleteAST` — every shape below (all already admitted by the SDK grammar profile; no grammar change is licensed by this REQ):

1. **Primitive literal in SELECT** (`SELECT 1, e/ehr_id/value FROM …`) — a projection item whose expression is a typed literal.
2. **Mixed `SELECT *, col`** — star and column projections in one SELECT list, order-preserving.
3. **Function-call WHERE LHS** (`WHERE LENGTH(o/name/value) > 5`) — a function call as the left operand of a comparison.
4. **`MATCHES` with `TERMINOLOGY(...)` or `{URI}` operand** — the operand modelled structurally (function name + three string arguments, or the URI), not as raw text.
5. **Path-vs-path comparison** (`WHERE a/x = b/y`) — an identified path as the right operand of a comparison.
6. **Top-level boolean junction at the FROM root** (`FROM COMPOSITION c1 OR COMPOSITION c2`, incl. `AND` and grouping).
7. **Parameter, primitive, or nested-function argument inside a function call** (`SELECT CONCAT('a', $p, LENGTH(x/y)) …`), in SELECT and in WHERE.
8. **AND/OR junctions over any in-catalogue operand** — a junction is in-catalogue exactly when all its operands are.

**One** `ErrIncompleteAST` condition remains, and it is the complete residual list (narrowed from two by [§ REQ-118](#req-118--deprecated-select-top-clause-and-literal-source-text) — see the amendment note below):

1. **A numeric literal the value vocabulary cannot represent** — a `LIMIT`/`OFFSET` value beyond Go `int`, a `SELECT TOP` count beyond `int`, an INTEGER beyond `int64` (`aql.IntValue` carries an `int64`), or a REAL beyond `float64`, in any value position (a SELECT literal, a comparison operand, a `MATCHES` member). Such a literal MUST be refused loudly, never degraded, because degrading it would be silent precision loss.

> **Amendment history (informative, not normative).** As first published, this REQ listed a second residual: a `SELECT TOP n` clause, which the grammar profile admitted but the structured AST could not carry, and which the extractor therefore refused. [§ REQ-118](#req-118--deprecated-select-top-clause-and-literal-source-text) closed that condition by adding the carrier — the clause is now in-catalogue, carried with its direction — and folded an out-of-range `TOP` count into residual 1 above. **No requirement to refuse a `top` clause is in force**; the reason the old one existed (a dropped `TOP` turns a bounded query into an unbounded one) is now met by carrying the bound instead of refusing it.
>
> [§ REQ-119](#req-119--re-parseable-canonical-aql-emission) then narrowed residual 1 itself — see § Canonical value spellings there for the rule; this note only records that the narrowing happened.

Every other extractor branch MUST remain **defensive** — unreachable against the current profile, and recording a gap rather than returning a zero value if a widened grammar ever reaches it.

`(*Query).Emit` MUST round-trip every newly modelled shape under the existing fixed-point property (`Emit(ParseQuery(Emit(ParseQuery(x)))) == Emit(ParseQuery(x))`), and MUST refuse (same error) any AST a future gap still cannot render — the no-silent-loss rule is unchanged.

New vocabulary MUST live in [`openehr/aql`](../../openehr/aql/) and be introspectable in both directions (the REQ-113 pattern: one model, read and write). Extensions to the sealed `SelectExpr` / `WhereExpr` / `Value` sets are **additive**; a consumer type-switching over them MUST treat an unrecognised case as out-of-catalogue rather than panic, and each interface's godoc MUST say so.

### Lint acceptance (extends REQ-109)

The static lint gate MUST NOT reject these grammar-admitted, server-executable shapes:

- **`ORDER BY` referencing a SELECT alias** (`SELECT x/y AS score … ORDER BY score`): an ORDER BY identifier that names no FROM alias MUST be resolved against the SELECT `AS` aliases before `aql_unknown_alias` is raised. An identifier matching neither MUST still raise `aql_unknown_alias`. A SELECT `AS` alias resolves an `ORDER BY` key only: it MUST NOT bind a class, so an identifier carrying a path tail (`score/magnitude`) MUST still raise `aql_unknown_alias`, and a SELECT alias MUST NOT resolve a path root in `WHERE`.
- **Boolean literal operands** — a bare `true` / `false` keyword in a value position, whether a comparison operand (`WHERE s/is_queryable = true`), a SELECT projection item (`SELECT true`), or a function-call argument (`SELECT CONCAT(true, 'x')`), is a literal and MUST NOT be treated as a path root; the structured AST MUST model it as a literal in every one of those positions, so a consumer reads one construct the same way in each clause. A keyword carrying a path predicate or a path tail (`true/nested`) MUST stay a path and keep its alias check, and so MUST an `ORDER BY` key (the grammar's `orderByExpr` admits no literal).

Existing REQ-109 codes, their meanings, and the collect-all/deterministic-order contract are unchanged; lint-clean remains neither spec-conformance nor execute-success (the CDR stays the execute-time authority, [PROBE-021](conformance.md#probe-021--aql-parse-error-mapping)).

### Builder containment algebra and in-text paging (extends REQ-055)

The write side ([`openehr/aql`](../../openehr/aql/) builder) MUST be able to express the following three forms the parse side already models — the enumerated list is the whole normative scope here, not read-write parity across the catalogue:

- **Negated containment** — `CONTAINS … NOT CONTAINS …` per the grammar's `classExprOperand (NOT? CONTAINS containsExpr)?`.
- **Sibling containment junctions** — `AND` / `OR` over containment operands, with parentheses emitted exactly when nesting departs from the default precedence (`NOT` binds tightest, then `AND`, then `OR` — mirroring the parse profile).
- **In-text `LIMIT n [OFFSET m]`** as an explicit opt-in, so a bound survives stored-query registration; the existing envelope paging (`Query.Fetch`/`Query.Offset`) stays the default and the two channels MUST NOT be silently combined — requesting both is a build-time error.

A containment junction MUST be the **final** child of the chain it sits in: the grammar takes a parenthesised group as a whole `containsExpr` alternative (`classExprOperand (NOT? CONTAINS containsExpr)? | '(' containsExpr ')'`), so no `CONTAINS` keyword may follow one, and deeper nesting is written inside the junction's operands instead. A builder tree that places a junction in a non-final chain position — through `Containment.Contains` / `Containment.NotContains`, or across repeated `Builder.Contains` / `Builder.NotContains` calls, which emit as one chain — MUST fail at `Build()` time with an error wrapping `aql.ErrInvalidQuery`; emission MUST NOT be re-shaped to make such a placement expressible, because distributing the chain tail under the junction's operands changes what the query asks.

> **Amendment (informative, not normative).** As first published, this rule and the class-completeness check beside it bound `Build()` only. [§ REQ-119](#req-119--re-parseable-canonical-aql-emission) extends both to `(*parse.Query).Emit`, so the read and write sides refuse the same trees *for these two rules* — the parity `aql.Containment` already claimed. It is not total: `Build()` additionally requires an alias, which the grammar does not.

A builder entry point for a **containment junction at the FROM root** (`FROM COMPOSITION c1 OR COMPOSITION c2`, which the read side models via `parse.FromClause.Junction`) is **deferred** until a consumer needs write-side root junctions: the builder keeps a single root class, so `Builder.From` / `Builder.FromEHR` are unchanged and a builder-emitted FROM root is never parenthesised.

All additions are **additive to the canonical write form** ([wire.md § REQ-055](wire.md#req-055--wire-boundary)): a builder program that uses none of the new API MUST produce byte-identical output to today (semver-minor). The canonical form for the new constructs MUST be: a single space around the `AND` / `OR` / `NOT CONTAINS` keywords; parentheses only where required by precedence; `LIMIT` / `OFFSET` emitted after ORDER BY in clause order.

### Acceptance

- **[PROBE-087](conformance.md#probe-087--aql-structured-ast-catalogue-completeness)** — every shape in the catalogue list parses → models → emits round-trip, pinned per shape; the former gap corpus asserts `ErrIncompleteAST` is gone; the residual guard — the unrepresentable numeric literal — still fires (`TestParseQuerySurfacesIncompleteAST`).
- **[PROBE-088](conformance.md#probe-088--aql-builder-containment-and-paging-stability)** — canonical-string stability goldens for the new builder constructs (the PROBE-020 property extended).
- Building-block independence (REQ-013) unchanged and still enforced by the forbidden-import tests.
- **Plan:** [`docs/plans/archive/2026-08-04-aql-expressivity-completion.md`](../plans/archive/2026-08-04-aql-expressivity-completion.md) — REQ-117 (archived after Phase 4).


## REQ-118 — Deprecated `SELECT TOP` clause and literal source text

Two carriers the structured AST lacked after [§ REQ-117](#req-117--aql-expression-catalogue-completion), both observable by a consumer that reads or re-emits third-party AQL rather than only building its own:

1. **The `SELECT TOP` clause** was the last shape the extractor dropped. A consumer therefore reported a *bounded* query as an extraction defect naming the toolchain, not as the deprecated row-limit spelling the client actually sent — and could not tell a dropped `TOP` from any other dropped construct without keying on prose.
2. **A projected literal's source text.** `parse.LiteralExpr` carried the typed value but not the text it came from, and the openEHR result schema names a projected column with no `AS` alias by its expression text; a path has `IdentifiedPath.Raw` to fall back on and a literal had nothing. Re-rendering the typed value gives the *canonical* form, not the source form — `1.50` → `1.5`, `001` → `1`, `"x"` → `'x'` — so a column name derived that way differs from the query the client sent.

The normative statements below are what this REQ requires of the SDK; both carriers are in force.

### Ground truth

openEHR **QUERY Release-1.1.0 § 4.4.3** governs the clause and is quoted here because every rule below follows from it:

- Syntax: `SELECT TOP integer [FORWARD|BACKWARD]` — "It starts with keyword `TOP`, followed by an integer number and/or the direction (i.e. `BACKWARD`, `FORWARD`)".
- Status: "*Deprecated*: Starting with Release 1.1.0, the use of `TOP` modifier is deprecated in favour of the `LIMIT` clause combined with `ORDER BY`", and "The `TOP` will be removed in a future major release of AQL specification."
- Prohibition: "It is not allowed to use `TOP` while also using `LIMIT` clause in the same query."

**Deprecated upstream is not out of scope here.** A deprecated-but-legal construct is exactly what an SDK must still read: the SDK does not author the queries it is handed, and until the removal release a client, a stored query, or a conformance corpus may legitimately carry `TOP`. So the SDK **MUST** be able to parse it, model it, emit it, build it, and diagnose it — and **MUST NOT** encourage it.

### The `TOP` carrier (read side)

[`parse.ParseQuery`](../../openehr/aql/parse/parse.go) **MUST** model a `top` clause without `aql.ErrIncompleteAST`:

- The `SELECT` clause **MUST** carry the clause as an optional value, so "no `TOP`" stays distinguishable from `TOP 0` — a dropped `TOP` turns a bounded query into an unbounded one, and an absent one that reads as `0` inverts it.
- The carrier **MUST** model the row count **and** the direction. Dropping `BACKWARD` would silently select the opposite end of the result set, which is the no-silent-loss rule's worst case in the same way a dropped bound is. The direction vocabulary **MUST** have an unspecified zero value so a bare `TOP n` round-trips without acquiring a direction the source did not write.
- The carrier **MUST NOT** reuse the `LIMIT` value vocabulary (`parse.LimitExpr` / `IntLimit` / `ParamLimit`): the profile's `top : TOP INTEGER direction=(FORWARD|BACKWARD)?` admits no `PARAMETER`, and `LimitExpr` carries no direction. Modelling `TOP $n` would create an AST shape whose emission the parser rejects, and the [`SDK-AQL-003`](../../resources/aql/grammar/DIVERGENCES.md) parameter relaxation **MUST NOT** be extended to `top` — the SDK does not widen a construct the spec is removing. No grammar change is licensed by this REQ.
- A `TOP` integer the carrier cannot represent **MUST** be refused loudly under the residual unrepresentable-numeric rule below, never truncated or degraded.
- `TOP` and the `LIMIT` / `OFFSET` clause **MUST** each report what the source wrote, independently. The parser **MUST NOT** normalise one into the other, and **MUST NOT** choose between them when a query carries both: that query is spec-invalid (§ 4.4.3), and reporting it as written is what lets the diagnosis name it.

Emission through [`(*Query).Emit`](../../openehr/aql/parse/query.go) **MUST** round-trip the clause under the existing fixed-point property. The canonical form **MUST** be `SELECT [DISTINCT ]TOP <n>[ FORWARD| BACKWARD] <items>` — keywords upper-cased regardless of source casing, single spaces, direction omitted when the source omitted it.

### Literal source text (read side)

- `parse.LiteralExpr` **MUST** carry the literal's **source text as written** alongside its typed `aql.Value`, for a literal in a `SELECT` projection and for a literal in a function-call argument.
- The field is **read-side fidelity only**: emission **MUST** keep rendering the typed value in canonical form, not the source text, because the canonical write form is normative ([wire.md § REQ-055](wire.md#req-055--wire-boundary)) and a directly-constructed AST has no source text. It **MUST** therefore be empty on a `LiteralExpr` the caller built rather than parsed, and a consumer **MUST NOT** treat it as a required field.
- A consumer that needs a deterministic rendering of any `aql.Value` it holds — parsed or constructed — uses the already-public `aql.FormatValue`; no second escaping implementation is warranted, and none **MUST** be added.

### Prohibited `TOP` + `LIMIT` combination

The four layers treat the spec-invalid combination differently, and deliberately:

- **Parse MUST accept and model both.** Nothing is dropped, so no `ErrIncompleteAST`; the parser reports what the source wrote.
- **Emit MUST render both faithfully.** The profile accepts the text, so refusing here would break the round-trip property for a query the parser admits, and the emitter is not the diagnosis layer.
- **Lint owns the diagnosis** — codes `aql_deprecated_top` (Warning, whenever `TOP` is present) and `aql_top_with_limit` (Error). Both are Layer-2 checks whose canonical catalogue home is [§ REQ-109 — Layer 2](#layer-2--shape-ast-walk-no-cdr); this section adds no second copy of them. Both MUST key on the clause's **presence in the source**, not on a successfully decoded bound: an out-of-range count is refused by the extractor and leaves no bound to read, and a query pairing that count with a `LIMIT` is precisely one that must not lint clean.
- **The builder MUST refuse to construct it.** `TOP` counts as an in-text row bound, so setting it together with the in-text `LIMIT`/`OFFSET` channel or with the request-envelope channel **MUST** fail at `Build()` with an error wrapping `aql.ErrInvalidQuery` — the same channel-exclusivity rule REQ-117 set for envelope-versus-in-text paging, and here additionally required by § 4.4.3.

### The `TOP` carrier (write side)

[`aql.Builder`](../../openehr/aql/builder.go) **MUST** be able to construct `SELECT TOP n [FORWARD|BACKWARD]`, in both the plain and directed forms, so a consumer can author the deprecated shape for a corpus, a migration, or a round-trip fixture without hand-assembling AQL text.

- The row count **MUST** be a non-negative integer; a negative count **MUST** fail at `Build()` with an error wrapping `aql.ErrInvalidQuery` (the profile's `top` production admits no sign, so a negative count could only emit text the parser rejects).
- The API's documentation **MUST** state the upstream deprecation and name the in-text `LIMIT` channel as the replacement. The vocabulary **MUST** live in [`openehr/aql`](../../openehr/aql/) and be shared by both sides (the REQ-113 "one model, read and write" rule), not duplicated per package.
- The addition is **additive to the canonical write form**: a builder program that does not use it **MUST** produce byte-identical output to today (semver-minor).

### Amends REQ-117 — the residual list

[§ REQ-117](#req-117--aql-expression-catalogue-completion) enumerated two remaining `ErrIncompleteAST` conditions. With this REQ landed the complete residual list is **one** condition: a numeric literal the value vocabulary cannot represent. A `TOP` clause is in-catalogue; a `TOP` integer outside the carrier's range folds into that one residual, as any other unrepresentable numeric literal does. Every other extractor branch **MUST** remain defensive, unchanged from REQ-117.

### Out of scope

- **Deciding what `TOP` means alongside `LIMIT`.** The spec forbids the combination, so the SDK reports and diagnoses it; execution semantics for a CDR that tolerates it are the consumer's.
- **`TOP n PERCENT` / `WITH TIES`.** Not in the openEHR grammar profile — an SQL-ism, not an AQL shape.
- **Source text for arbitrary `SELECT` expressions.** A path already carries `IdentifiedPath.Raw` and a function call renders canonically; widening a verbatim-span carrier to every expression is a separate ask with its own trust-model consequences.
- **Source text on WHERE-side values.** A comparison operand is not a result column, so it has no result-schema naming rule to satisfy.
- **A `TOP` entry point in the verb-function style.** `TOP` never starts a query; the method on the builder is the whole surface.

### Acceptance

- **[PROBE-087](conformance.md#probe-087--aql-structured-ast-catalogue-completeness)** (extended) — the `TOP` shapes (bare, `FORWARD`, `BACKWARD`, with `DISTINCT`, with `*`, and alongside `LIMIT`/`OFFSET`) parse without `aql.ErrIncompleteAST`, model the count and direction, and round-trip under the PROBE-080 fixed-point property; literal source text is pinned per literal kind, including the cases where it diverges from the canonical rendering; the unrepresentable-numeric guard still fires, now including an out-of-range `TOP`.
- **[PROBE-088](conformance.md#probe-088--aql-builder-containment-and-paging-stability)** (extended) — canonical-string goldens for the builder's plain and directed `TOP`, plus the refusal matrix (negative count; `TOP` with in-text `LIMIT`; `TOP` with envelope paging).
- **[PROBE-028](conformance.md#probe-028--aql-lint-stability)** unchanged: the new codes fire only on a query carrying a `TOP`, and no lint cassette carries one. This is *pinned*, not assumed — the probe asserts exact issue-code multisets per cassette (`valid.aql` → none), so a code that fired spuriously fails `TestProbe028AQLLint`.
- Building-block independence (REQ-013) unchanged and still enforced by the forbidden-import tests.
- **Lives in:** [`openehr/aql/top.go`](../../openehr/aql/top.go) (shared vocabulary), [`openehr/aql/builder.go`](../../openehr/aql/builder.go) (write side), [`openehr/aql/parse/query.go`](../../openehr/aql/parse/query.go) (AST + emitter), [`openehr/aql/parse/extract_query.go`](../../openehr/aql/parse/extract_query.go) (extraction), [`openehr/aql/lint/lint.go`](../../openehr/aql/lint/lint.go) (diagnosis).
- **Plan:** [`docs/plans/archive/2026-08-05-aql-top-carrier-literal-source-text.md`](../plans/archive/2026-08-05-aql-top-carrier-literal-source-text.md) — REQ-118 (archived after Phase 5).

---

## REQ-119 — Re-parseable canonical AQL emission

The structural guarantee that the typed builders cannot emit syntactically invalid AQL is pinned by [PROBE-021](conformance.md#probe-021--aql-parse-error-mapping) (the write-side complement of [§ REQ-055](wire.md#req-055--wire-boundary)'s canonical form), and [§ REQ-117](#req-117--aql-expression-catalogue-completion) requires `(*Query).Emit` to be a fixed point of the parse/emit round trip. Both are stated over the *shapes* the catalogue models, and neither reaches the **value positions inside** those shapes. This REQ binds those positions, so that a consumer gets one guarantee — *what this SDK writes, it can read back* — rather than one that holds per clause and lapses per literal.

Two failure modes are distinguished throughout, because they justify different rules. Emitting text the parser **rejects** is loud: the caller gets an error the moment they re-read their own query, and the requirement is simply that it not happen. Emitting text the parser **accepts as something else** is silent, invisible to every round-trip, golden, and parser check downstream, and is the only thing that justifies refusing an operand that would otherwise be valid AQL. The normative statements below are what this REQ requires of the SDK; the defects that motivated each are in the traceability entry and the implementing PR, not here.

### The emission closure property

Everything this SDK emits, it MUST be able to read back as what it meant:

- **Round-trip closure.** For every value a **validating** write path emits — [`aql.Builder.Build`](../../openehr/aql/builder.go), [`aql.FormatWhere`](../../openehr/aql/where.go), [`(*parse.Query).Emit`](../../openehr/aql/parse/query.go) — `ParseQuery` of the emitted text MUST succeed and MUST recover a value equal to it under `aql.EqualValues`. A validating write path MUST NOT emit a value it cannot read back, and MUST apply the value-level guards below in **every** value position it renders, including the SELECT list — not the `WHERE` clause alone.
- **One unvalidated formatter, named as such.** [`aql.FormatValue`](../../openehr/aql/where.go) returns no error and so cannot refuse; it is the deliberate escape hatch for a value the caller has already checked. It is therefore NOT bound by the closure clause, its godoc MUST say so, and the SDK MUST expose an exported validity check ([`aql.ValidateValue`](../../openehr/aql/value.go)) so a caller — and any write path outside `openehr/aql` — can hold the same line before calling it. Being unvalidated does not license PANICKING: a value with no wire form at all MUST render as the empty string, because a value shape's pointer twin is reachable through the public API (the zero `aql.MatchesExpr.Terminology` is a nil `*aql.FuncCall`). The check side of the recipe MUST agree: `aql.ValidateValue` MUST refuse a value with no wire form — an untyped nil and a typed-nil pointer alike — because no value POSITION has a legal empty spelling, so a validator that passes one green-lights embedding `""` into hand-assembled AQL.
- **No silent substitution.** Where a *caller-supplied* operand is emitted VERBATIM because its grammar token is unquoted, the write side MUST refuse any operand text that would change the query's structure rather than emit it. This is the strong form of the rule: refusal is required even though the resulting text may itself be valid AQL, precisely because valid-but-different text is invisible to every round-trip, golden, and parser check downstream. It binds the `MATCHES {uri}` operand and the single-token identifier positions below, and not the PATH positions REQ-055 rule 3 emits verbatim by contract, where caller data MUST instead flow through `aql.Param` ([§ REQ-055](wire.md#req-055--wire-boundary), the injection guard).
- **Refuse, never re-spell.** A value the grammar cannot carry MUST fail with an error wrapping `aql.ErrInvalidQuery`, not be silently normalised into a spelling that parses. Re-spelling would change what the query asks.
- **Every shape, including its pointer twin — at every dispatch site.** The SDK's sealed interfaces are each satisfied by both the value and the pointer form of every shape, because their methods have value receivers — `aql.Value`, `aql.WhereExpr`, `parse.SelectExpr` and `parse.LimitExpr` all behave this way, and `aql.MatchesExpr.Terminology` is itself a `*aql.FuncCall`, so a pointer is reachable without a caller writing `&`. The invariant is ONE sentence: **a shape decision on a sealed vocabulary — in a validator, an emitter, or the diagnostic that names a refused operand — runs only on the result of that vocabulary's normaliser, which refuses a nil instead of panicking.** The SDK MUST export the normaliser for each vocabulary ([`aql.DerefValue`](../../openehr/aql/value.go), [`aql.DerefWhere`](../../openehr/aql/where.go), [`parse.DerefSelectExpr`](../../openehr/aql/parse/deref.go), [`parse.DerefLimitExpr`](../../openehr/aql/parse/deref.go)), so a consumer type-switching over an additive sealed set binds the same rule to both carriers rather than to whichever one its `switch` happens to list. The invariant is held STRUCTURALLY, not by enumerating known sites: a source-level tripwire fails any new type assertion or switch on these vocabularies whose enclosing function never normalises, and a position×kind×carrier sweep asserts that both carriers of every value kind get the same answer in every value position (§ Acceptance).
  - **Absence is positional.** A predicate that denotes nothing — an untyped nil or a nil pointer — is *no clause* at the top of a WHERE, which is permitted, but MUST be refused as an operand of a `NOT` or a term of a junction, where an arm that quietly vanished changes the result set. The position, not the route, decides: a top-level absence combined with `FromEHR`'s implicit filter MUST still be no clause rather than a refused junction term.
  - **One WHERE emission path.** The three validating write paths MUST behave alike on the same predicate, and for WHERE they MUST do so by construction, not by convention: `aql.Builder.Build` and `(*parse.Query).Emit` both render through `aql.FormatWhere`'s single validate-then-emit sequence. Two hand-kept copies of that sequence is how the same input got different answers by route.

- **A narrower grammar position needs its own guard.** Where a rule narrows a position below the general `terminal` value set, the write side MUST check the narrowing rather than delegate to the shared value validator, whose breadth would otherwise become the operand's. Four positions are in force: `likeOperand : STRING | PARAMETER`; `valueListItem : primitive | PARAMETER | terminologyFunction` for a braced `MATCHES` member; the bare `matchesOperand : terminologyFunction`, which admits no other call however wide the carrying field's type is; and `limitValue : INTEGER | PARAMETER`. The narrowing MUST be read off what the position can LEX, not off the production's name list: `valueListItem`'s `primitive` names the `BOOLEAN` token, but `BOOLEAN` is declared after `IDENTIFIER` and so never lexes — a comparison position survives that because `terminal` admits `identifiedPath` and the keyword is mapped back, while the braced list has no path alternative, so a boolean member MUST be refused even though the production appears to admit it (`MATCHES {true}` is a syntax error to this SDK's own parser).

- **One question, one answer.** Where two functions decide the same question about a node — "is this a class or a junction?" — they MUST agree. Splitting the decision let a `VERSION` class expression be blessed as complete by one and reclassified as a boolean grouping by the other, dropping it from the emitted chain silently. A junction operator MUST likewise be checked against its vocabulary exactly as a comparison operator is, and a junction MUST carry at least one term: the constructors collapse the empty case, and a rule enforced only by a constructor does not bind the struct literal beside it.

### Canonical value spellings

These value-level rules extend the canonical write form, whose canonical home is [wire.md § REQ-055](wire.md#req-055--wire-boundary). Each is derived from the vendored grammar profile (`resources/aql/grammar/active/`, ADR [0007](../adr/0007-aql-antlr-grammar-profile.md)), which is the authority ([§ REQ-109](#req-109--aql-static-lint)'s syntax floor).

Two of them **change** text this SDK previously emitted, so § REQ-055's rule — that a change to the canonical form is semver-major — is engaged and answered here: a literal carrying `'` or `\` and a whole-valued real (`2` → `2.0`) both emit differently from v0.19.0. The two prior spellings failed differently, and the classification argument is honest about which: the string spelling was **not re-parseable at all** (`'O''Brien'` lexes as two tokens), so no conformant program could depend on it; the whole-valued real **re-parsed cleanly as a different value** — `2` came back an `aql.IntValue` — so a program depending on it was depending on a silent type change that already violated the round-trip identity this REQ establishes. On that basis the change is classified **semver-minor**. It nonetheless rewrites any golden that pins such a literal, which is why it is called out in the CHANGELOG rather than left to a consumer's diff.

- **String literals** MUST be escaped with the grammar's `ESCAPE_SEQ` (a backslash before the escaped character). The SQL-style doubled-quote convention MUST NOT be emitted: the lexer's `STRING` token admits only `ESCAPE_SEQ`, `UTF8CHAR`, `OCTAL_ESC`, or a character that is neither a backslash nor the delimiter, so a doubled quote ENDS the token and reopens another one. Decoding MUST be the exact inverse, MUST resolve all three escape shapes, and MUST handle BOTH delimiter forms — the `STRING`, `DATE`, `TIME` and `DATETIME` tokens all admit `"…"` alongside `'…'`, and a delimiter that survives into the decoded value turns a comparison into one that matches nothing, silently. Additionally:
  - The C0 controls that have an `ESCAPE_SEQ` spelling (`\a \b \f \n \r \t \v`) MUST be emitted escaped, so canonical AQL stays single-line and printable. Every other character the token admits raw — including `"`, `?` and `*` — MUST ride through unchanged.
  - A byte that begins no valid UTF-8 sequence MUST be emitted as `OCTAL_ESC` (`\NNN`). A Go string is a byte string, so one lifted from a non-UTF-8 source can carry such a byte; the `STRING` token accepts it raw, but the lexer decodes its input to runes, so a raw byte returns as U+FFFD and closure fails on the SECOND pass. Octal is the grammar's only spelling for an arbitrary byte.
  - Because `UTF8CHAR` is exactly four hex digits, a character outside the BMP has no spelling but a UTF-16 **surrogate pair**. Decoding MUST combine an adjacent high/low pair into the character it denotes; a lone half denotes no character and MUST decode to U+FFFD.
- **Real literals** MUST emit a fractional part, so the text re-lexes as `REAL : DIGIT* '.' DIGIT+` and never as `INTEGER : DIGIT+`. A real with no AQL spelling (an infinity or NaN) MUST be refused. Scientific notation (`SCI_REAL` / `SCI_INTEGER`) MUST be readable and is re-emitted in decimal form — a canonicalisation, not a value change.
- **Integer literals** are bounded by `aql.IntValue`'s `int64`, whose range is asymmetric. The negative bound is exactly representable and MUST be accepted; the residual refusal of [§ REQ-117](#req-117--aql-expression-catalogue-completion) applies only to a literal genuinely outside the type. `numericPrimitive` is recursive, so a repeated unary minus MUST be read at its parity rather than reported as an out-of-range literal.
- **A value-position function name** MUST lex as the grammar's `IDENTIFIER` or one of its `*_FUNCTION_ID` tokens. A name whose spelling matches a keyword token declared BEFORE `IDENTIFIER` in `AqlLexer.g4` MUST be refused — notably the aggregates (`COUNT`, `MIN`, `MAX`, `SUM`, `AVG`), which are real AQL functions but belong to `aggregateFunctionCall`, admitted in `SELECT` alone.
  - Shadowing turns on **declaration order**, not on keyword-ness, so the rule is stated over that order and not over a list of words. A keyword token declared AFTER `IDENTIFIER` does not shadow it: `true` / `false` (`BOOLEAN`) MUST NOT be refused, because the grammar admits them as a function name and refusing them makes `Emit` reject an AST `ParseQuery` itself produced.
  - A **projected** function name in `SELECT` MUST be held to the same rule minus the aggregates, since `aggregateFunctionCall` is reachable from `columnExpr` and from no value position. Admitting the NAME admits the rule that carries it, so the projected call's SHAPE MUST be held to that rule too: `COUNT` takes `DISTINCT? path` or a bare `*` — never a star beside arguments or `DISTINCT`, which emission MUST refuse rather than resolve by dropping the loser (`COUNT(*)` where the AST said `COUNT(c/x)` counts rows instead of values, silently) — `MIN`/`MAX`/`SUM`/`AVG` take exactly one identified path, and no other projected call carries `DISTINCT` or a star at all.
- **`TERMINOLOGY`** has its own grammar rule with a fixed arity and argument type; a call MUST carry exactly three string-literal arguments, and MUST NOT carry `DISTINCT` or a star argument. This binds **both** carriers: the read and write sides model a value-position call and a projected one as different types (`aql.FuncCall` and `parse.FunctionCall`), so checking one leaves the other emitting text the parser rejects.
- **A parameter placeholder** MUST spell the grammar's `PARAMETER : '$' IDENTIFIER_CHAR` (a leading letter, then letters, digits and underscores) in every position that carries one — a value position, and the in-text `LIMIT` / `OFFSET` operand. This position is [§ REQ-055](wire.md#req-055--wire-boundary) rule 4's designated channel for caller data, so leaving it unchecked was the one hole where the injection guard's own recommendation could emit a structure-changing query: an unconstrained name renders verbatim after the `$`, and `$p AND c/secret = 1` re-parses as two predicates where the caller wrote one. A parameter is additionally NOT a projection: `columnExpr` has no `PARAMETER` alternative while a function argument (`terminal`) has, so a bare parameter as a SELECT item MUST be refused positionally and stays legal inside a projected call's arguments. `aql.PathValue` is by contrast **exempt** from the closure clause above: it is one of the identifier positions REQ-055 rule 3 emits verbatim by contract, developer-authored rather than caller-supplied, and the whole point of the parameter channel is that caller data does not go there.
- **A `MATCHES {uri}` operand** MUST be refused unless it lexes as the grammar's `URI` token. The check MUST follow that token's own decomposition (`URI : URI_SCHEME ':' URI_HIER_PART ( '?' URI_QUERY )? ( '#' URI_FRAGMENT )?`) and not a flat union of the delimiter sets, which is broader than any single position admits: `%` leads `URI_PCT_ENCODED` and requires two hex digits, `[` / `]` occur only as an `URI_IP_LITERAL` host, and `#` separates the fragment once and appears nowhere inside it. Spellability is necessary but not sufficient — because the operand is lexed alone between the braces, an operand a token declared BEFORE `URI` would claim MUST also be refused; `matchesOperand` admits `URI` only. `TERM_CODE` (`<term>::<code>`) is the case in force.

### Single-token identifier positions

Three positions carry exactly one `IDENTIFIER` token — a SELECT `AS` alias (`selectExpr : columnExpr (AS aliasName=IDENTIFIER)?`), and a class's alias and RM type (`classExprOperand : IDENTIFIER variable=IDENTIFIER? …`) — and a fourth carries exactly one `ARCHETYPE_HRID` or `PARAMETER` (`archetypePredicate`). All four are spliced VERBATIM into the emitted text, so a caller string carrying a delimiter re-parses as a different query with no error: `Alias: "x, c/y"` emits `SELECT c/x AS x, c/y FROM …`, which is two projections.

Because each admits ONE token, the exact predicate — *this string lexes as that token* — is writable, which is what separates them from the path positions in § Out of scope. Every write path that renders one MUST apply it, and MUST apply it at ALL of them: a rule bound to some of a position class and not the rest is the defect shape this REQ has repeatedly paid for. `aql.Builder` and `(*parse.Query).Emit` MUST refuse the same strings, so Build/Emit parity holds from the day the guard lands rather than being reconciled later. Parity is over the **normalised** string, not the literal one: `aql.Builder` trims leading and trailing whitespace on intake — pre-existing behaviour whose own reason is this REQ, since `From("   ", "c")` once emitted `FROM     c`, which re-parses with the alias as the RM type — and MUST then apply the identical predicate to the result. That trim widens the builder's accept set by exactly one harmless equivalence and cannot manufacture a splice, since it only removes surrounding whitespace and the guard runs on what is left. The claim MUST be tested in that form, because tested as literal-string equality it is simply false.

The rule is stated over what the LEXER does, not over an alphabet, because an alphabet check passes three things it should not:

- **A reserved word.** Shadowing turns on declaration order exactly as it does for a function name, so the refused set is every keyword declared before `IDENTIFIER` — and it is WIDER here than for a function name: `functionCall` lists the `*_FUNCTION_ID` tokens as alternatives, so `LENGTH` may name a function, while an identifier position admits `IDENTIFIER` alone and MUST refuse it.
- **`true` / `false`.** `BOOLEAN` is declared AFTER `IDENTIFIER`, so these lex as identifiers and MUST NOT be refused — the same counter-example that governs function names, and refusing them would reject an alias `ParseQuery` itself produces.
- **A node code.** `at0001` and `id123` lex as `AT_CODE` / `ID_CODE` and MUST be refused. Their `at` / `id` prefixes are LOWERCASE LITERALS rather than the case-insensitive letter fragments the keywords use, so `AT0001` is a perfectly good alias — a distinction no keyword list expresses.

`VERSION` is not an identifier but a separate grammar ALTERNATIVE (`classExprOperand` has `VERSION variable=IDENTIFIER?` beside the `IDENTIFIER` form), carried by `parse.ClassExpr.Version`. The word MUST therefore be refused in an RM-type position when that flag is absent: emitting it produces text that re-parses with the flag SET, an AST the caller did not write. The flag being the sole carrier is what makes the refusal safe — the extractor always sets both together.

Because `openehr/aql` may not import the generated lexer (§ REQ-013 — the dependency runs the other way), these rules are hand-derived and MUST be held honest mechanically rather than by review: the guard's accept set MUST be shown equal to "the parser reads this string back unchanged", over every token name in the vendored lexer plus the spellings carried only in fragments.

### Emit-side structural parity

`(*parse.Query).Emit` MUST apply the containment structural refusals [§ REQ-117](#req-117--aql-expression-catalogue-completion) requires of `Build()`, each with an error wrapping `aql.ErrInvalidQuery`:

- a containment junction may only END a containment chain;
- a class node MUST carry an RM type (or be a `VERSION` class expression), and MUST NOT carry a child-join of its own; and
- a class expression MUST NOT carry both an archetype and a standing predicate: they are the two mutually exclusive spellings of the ONE `[...]` position, and emitting whichever a switch reaches first silently drops the other — a row filter — so the query returns more rows than the AST asked for. This is the same dual-operand rule that binds `aql.Comparison` (`Path` vs `Left`) and `aql.MatchesExpr` (its three operand forms); `parse.ClassExpr` is the third type with two spellings of one position.

These are the rules for a field the emitter reads. A field it does NOT read is the same defect in its **drop** direction, and is the more dangerous half, because a guard that validates a field the emitter then discards reports the malformed value and loses the well-formed one — the guard's own success is what hides the loss. Emit MUST therefore also refuse:

- a `VERSION` class expression carrying an RM type other than `VERSION` itself, or any archetype predicate. `classExprOperand`'s VERSION alternative is `VERSION variable=IDENTIFIER? ('[' versionPredicate ']')?` — a separate alternative rather than a class named "VERSION" — so it has no RM-type slot and no archetype slot, and `{Version: true, Archetype: "…encounter.v1"}` emitted `FROM VERSION v` with no error, a row filter gone. The spelling `RMType: "VERSION"` MUST stay accepted beside the flag: the extractor always sets the two together, so it is the flag's own carrier and says exactly what the emitted text says.
- a class expression that flags a `$param` archetype predicate while carrying no parameter. The flag declares the bracket to BE `archetypePredicate`'s `PARAMETER` alternative, so with nothing to render, no bracket is emitted at all and the predicate the AST announced disappears.
- **any** non-zero `FROM` root beside a root junction, not merely a root that carries a class. Beside a junction the root is never passed to the class emitter, so a lone alias, archetype or standing predicate there is dropped rather than refused — including the very splice text the identifier guards exist to catch, which reaches the wire as an absent term. Checking the whole value is safe precisely because the extractor leaves the root WHOLLY zero in that case; a check keyed on "does it carry a class" is not wrong so much as narrow, and narrowness is what this REQ keeps paying for.

`ClassExpr.HasPredicate` is deliberately NOT constrained here. It is a derived, informational flag that no emitter reads, so no value of it can add or remove text: unlike the fields above, it cannot participate in a drop. Stated so its absence reads as a decision rather than an oversight.

Completeness MUST be checked before placement. The read side infers a node's KIND from an absent RM type, so an incomplete class node is indistinguishable from a junction: checked in the other order it is reported as a misplaced junction the caller never wrote.

The read and write sides therefore refuse the same trees **for the two rules above**, which is what [`aql.Containment`](../../openehr/aql/containment.go) claimed. The parity is deliberately not total: `aql.Builder` also insists on an alias, and `classExprOperand` takes the alias as optional, so requiring one in `Emit` would refuse valid AQL `ParseQuery` had just accepted. That asymmetry is a write-side ergonomic choice, not a grammar rule. The extractor cannot itself build a violating tree (it only ever mirrors AQL that already parsed), so this binds the direct-construction path `parse.Query` blesses: a consumer that assembles or rewrites an AST by hand.

### Value comparability

The shared value vocabulary is **not** `==`-comparable and MUST NOT be used as a map key: `aql.PathValue` and `aql.FuncCall` carry slices, so `==` on either — or on any `WhereExpr` holding one — panics at run time. The SDK MUST expose an equality over `aql.Value` that is panic-free across every value and pointer shape of the catalogue, including a typed-nil pointer, and its godoc MUST state the restriction.

This is a documentation-and-surface requirement, not a behaviour change: the types became uncomparable in v0.18.0 when REQ-117 added the slice fields, and unlike the `aql.Containment` break released beside it the compiler cannot see this one.

### Out of scope

- **Validating URI structure beyond what the token can lex.** The guard follows the `URI` token's decomposition and refuses what no position admits; it is not an RFC-3986 parser. `URI_REG_NAME`, `URI_PORT` and `URI_PATH_*` are character classes in this grammar, so a structurally odd but spellable URI (an empty host, a dotted quad out of range) reaches the wire, where the backend stays the authority ([PROBE-021](conformance.md#probe-021--aql-parse-error-mapping)).
- **Equality over `WhereExpr` / `SelectExpr`.** Only the `Value` vocabulary gets a comparison here. Predicate-tree equality needs a normalisation policy first (is `a AND b` equal to `b AND a`?); that question is deferred until a consumer needs it, not answered by omission.
- **Guarding the identifier positions against splice text.** A path is emitted verbatim by REQ-055 rule 3's contract, so `aql.Exists("c/x AND c/y = 1")` emits text that re-parses as a junction rather than one EXISTS — the same substitution shape the parameter guard closes, reachable through any path-taking constructor (`Eq("c/x = 1 OR c/y", …)` splices the same way). This is out of scope BY the rule-3 exemption, not by oversight: paths are developer-authored identifiers, never the channel for caller data (that channel is `aql.Param`, and it IS guarded), and a syntactic path guard cannot be written without refusing legal name predicates, whose bracketed text may carry `AND`, `OR`, quotes and spaces. Recorded here so the decision is explicit rather than implicit.
- **Guarding a class's standing PREDICATE** (`ClassExpr.Predicate`, the bracket text that is neither an archetype id nor a `$param`). It is the same substitution shape as the identifier positions — `Predicate: "a/b='c'] CONTAINS OBSERVATION o[d/e='f'"` emits and re-parses as an extra containment term — but unlike them it is not ONE token: `pathPredicate` admits `standardPredicate | archetypePredicate | nodePredicate`, and a node predicate lawfully carries `AND`, `OR`, quotes and spaces. So the guard is a sub-grammar validator, not a token check, and a conservative approximation would refuse predicates `ParseQuery` accepts — the tightening failure this REQ otherwise guards against. Deferred to [issue #99](https://github.com/Cadasto/openehr-sdk-go/issues/99) rather than approximated.
- **Widening the grammar profile.** No rule here licenses a grammar change; every refusal is derived FROM the vendored profile.
- **Escaping the control characters that have no `ESCAPE_SEQ` spelling.** The `STRING` token admits them raw and they round-trip, so emitting them as `\uXXXX` would be cosmetic. Contrast the two cases the § above DOES require escaping: an invalid UTF-8 byte and a surrogate pair break the round trip, so they carry correctness content and are not cosmetic.

### Amends REQ-117 — the residual list and the Build()-only refusals

[§ REQ-117](#req-117--aql-expression-catalogue-completion) is narrowed and widened by this REQ, in two places:

- Its residual 1 refuses "an INTEGER beyond `int64`". `math.MinInt64` is exactly representable, so the refusal now applies only to a literal genuinely outside the type. The residual list is otherwise unchanged: **one** condition, a numeric literal the value vocabulary cannot represent.
- Its containment structural refusals were required of `Build()` alone. They are now required of `(*parse.Query).Emit` too (§ Emit-side structural parity above), so the read and write sides refuse the same trees for those two rules. `Build()`'s additional alias requirement is unaffected and stays write-side only.

No other REQ-117 statement is affected, and nothing here licenses a new `ErrIncompleteAST` condition.

### Acceptance

- **[PROBE-090](conformance.md#probe-090--aql-emission-round-trip-closure)** — every value kind survives emit → parse → equal value; the write-side guards are confronted with the grammar rather than with a hand-written expectation.
- The grammar-derived rules are held honest MECHANICALLY, not by a list a maintainer must remember to update: `TestReservedFuncNamesTrackTheGrammar` reads every token name out of `AqlLexer.g4` — plus the keyword spellings that live only in lexer *fragments* — and asserts the validator and the parser agree on each; `TestMatchesURIGuardTracksTheGrammar` asserts round-trip IDENTITY over a corpus that includes the positionally-unspellable operands a flat alphabet admitted; `TestIdentifierGuardTracksTheGrammar` reads the same token names and holds all three identifier positions to one accept set, failing if they ever disagree with each other; and `TestArchetypeIDGuardAgreesOverGeneratedHRIDs` sweeps a GENERATED cross product of the four `ARCHETYPE_HRID` fragments — legal and illegal spellings of each, including the percent-encoded namespace label no hand corpus reached — beside the named traps in `TestArchetypeIDGuardTracksTheGrammar`. A hand-written corpus is the "list a maintainer must remember to update" this clause exists to forbid, so where the position has no token NAMES to walk, the corpus MUST be generated rather than enumerated.
- The pointer-twin invariant is held STRUCTURALLY, by two guards designed as a pair: `TestSealedVocabularyDispatchSitesNormalise` sweeps the SOURCE of both packages and fails any type assertion or switch on a sealed vocabulary whose enclosing function never normalises — its shape list is derived from the marker-method receivers, so a shape added later is covered the day it lands — and `TestEveryValueKindInEveryPositionRoundTripsOrRefuses` sweeps every value kind × value position × carrier at RUN time: whatever a position accepts must re-parse to an equal value, and both carriers must get the same answer. The first catches a site the second's positions miss; the second catches a per-operand asymmetry the first's function granularity cannot see. The pair has one named residual: a function that normalises TWO vocabularies keeps the tripwire quiet when it drops either call, and the sweep's carriers are Value-level — so the nested SELECT-argument positions are held by a third guard, `TestEmitSelectBindsThePointerTwin`'s argument-position rows, whose byte-identity fails if `DerefSelectExpr` is removed from either argument validator. The normalisers themselves are plain exhaustive type switches (the idiom spec bans reflection), held closed by a case-coverage sweep (`TestDerefSwitchesCoverEveryShape`) that derives each vocabulary's shape set from the marker-method receivers and fails when a deref switch is missing a shape's case in either carrier form.
- Every guard MUST be mutation-detectable: removing it MUST fail a named test, not merely narrow coverage. String-literal closure is additionally fuzzed (`FuzzStringLiteralRoundTrip`), seeded with the byte classes that were wrong at least once.
- Building-block independence (REQ-013) unchanged: `openehr/aql` still does not import the generated lexer, which is why the grammar confrontations live in `openehr/aql/parse`.
- **Lives in:** [`openehr/aql/value.go`](../../openehr/aql/value.go) (literal spellings, function names, `ValidateValue`, `EqualValues`, `DerefValue`), [`openehr/aql/identifier.go`](../../openehr/aql/identifier.go) (`ValidateIdentifier`, `ValidateArchetypeID`), [`openehr/aql/containment.go`](../../openehr/aql/containment.go) (the containment intake of both), [`openehr/aql/where.go`](../../openehr/aql/where.go) (`FormatValue`, `FormatWhere`, `DerefWhere`, URI operand guard), [`openehr/aql/builder.go`](../../openehr/aql/builder.go) (`Build` rendering WHERE through `FormatWhere`), [`openehr/aql/parse/deref.go`](../../openehr/aql/parse/deref.go) (`DerefSelectExpr`, `DerefLimitExpr`), [`openehr/aql/parse/extract_query.go`](../../openehr/aql/parse/extract_query.go) (unescaping, both delimiters, signed and scientific numeric extraction), [`openehr/aql/parse/query.go`](../../openehr/aql/parse/query.go) (SELECT-position and shape validation, Emit-side structural parity).
