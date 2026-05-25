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
- **`ARCHETYPE_SLOT` assertion grammar** — separate REQ-104. The current `Slot.Includes()` / `Slot.Excludes()` raw-string surface remains the only addressable slot constraint.
- **External terminology lookup** — REQ-105 surfaces bindings, but neither it nor REQ-103 calls into a remote terminology service during `Validate`.
- **AOM 2 `tuple_constraint`** — not used by ADL 1.4.

### Building-block independence (REQ-013)

`openehr/template/constraints/` is **stdlib-only**. It is importable independently of `openehr/template/` so codegen and downstream validators can use the constraint types without pulling the OPT parser.

### Example value emission (REQ-107 hook)

Every `PrimitiveConstraint` additionally exposes `ExampleValue() any` — a minimal-valid Go example value in the shape `Validate` accepts. For bounded constraints (closed lists, bounded ranges, enumerated units), `Validate(c.ExampleValue())` MUST return an empty `Violation` slice; unbounded primitives return a documented sentinel (e.g. `"example"`, `int64(0)`, `"2020-01-01"`). The factory is the leaf primitive of the REQ-107 template-driven instance generator and stays on the sealed interface so the closed type-switch (REQ-024 — no reflection) remains the only entry point for new primitive shapes. See § REQ-107 for the generator contract and [`docs/plans/2026-05-24-template-instance-example-generator.md`](../plans/2026-05-24-template-instance-example-generator.md) § "Example value factory" for the per-type strategy table.

- **Lives in:** [`openehr/template/constraints/`](../../openehr/template/constraints/)
- **Probes:** PROBE-024 (primitive constraint validation against fixture inputs)

---

## REQ-102 — Composition validation

The SDK **MUST** ship a `ValidateComposition(comp *rm.Composition, c *templatecompile.Compiled) Result` entry point that walks a parsed RM `Composition` against a compiled OPT and returns a `Result` aggregating every issue found in a single pass.

### Contract

- **Pure function.** No I/O, no goroutines, no reflection. Stateless — concurrent callers share `c` safely.
- **Collect-all, not fail-fast.** Validators emit one `Issue` per failing clause; the walk completes regardless of how many issues fire. UIs and CI consumers need the full list.
- **Result shape:**
  ```go
  type Result struct {
      OK     bool      // true ⇔ len(Issues) == 0
      Issues []Issue
  }
  type Issue struct {
      Path     string   // AQL path of the offending node (empty for global issues)
      Code     string   // stable programmatic identifier — see code taxonomy below
      Detail   string   // human-readable message
      Severity Severity // Error in v1; Warning reserved
  }
  type Severity int
  const (
      Error   Severity = iota
      Warning              // reserved; not emitted in v1
  )
  ```

### Trust model

The validator treats the **compiled OPT as authoritative for structure** and the **composition as the instance under test**. Structural traversal is template-driven: for each compiled OPT node, the walker reads the corresponding RM property by `rm_attribute_name`, enforces existence / cardinality / alternatives, and recurses into matched RM children. Path strings in `Issue.Path` come from the OPT's pre-computed `AQLPath` (`templatecompile.CompiledNode.AQLPath`) — composition-supplied predicates never form lookup keys, so missing nodes are reported instead of silently bypassed.

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
| **Slot fit — RM-type prefix fallback** | each `Content[i].ArchetypeNodeID` must match one of the OPT's archetype-root or slot-include archetype ids (or, when no slot constraint applies, share the slot's RM-type prefix `openEHR-EHR-<rmType>.`) |
| **Slot assertion grammar** | deferred — REQ-104 |
| **Extra RM nodes not declared in OPT** | not flagged in v2; optional `warning` policy is a follow-up |
| **Terminology binding value-set** | deferred — REQ-105 |

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
| `slot_fill` | an RM item under a C_MULTIPLE_ATTRIBUTE whose `archetype_node_id` matches no OPT child (including slot RM-type-prefix fallback in v2) |
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
| `ErrAQLSyntax` | reserved — AQL lint surface not yet implemented |

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

### Public surface scope (v1 — module-local)

The `c *templatecompile.Compiled` argument is typed against the SDK's internal compiled-template package. Per Go's `internal/` visibility rule, **external consumers (modules outside `github.com/cadasto/openehr-sdk-go`) cannot construct the argument and therefore cannot call `ValidateComposition` directly in v1**. The validator is callable from any package within this module — composition builder, codegen, CI tools, MCP servers vendoring the SDK.

This restriction is intentional and matches [ADR 0005](../adr/0005-compiled-template-foundation.md) §C2: `internal/templatecompile/` stays internal until REQ-101 (composition builder) confirms the public shape. The public re-export (`template.Compile` / `template.Compiled`) lands alongside REQ-101 Phase 1, after which the validator's public signature becomes externally callable without code change. Until then, downstream consumers either vendor the SDK as a private dependency or wait for the promotion.

### Out of scope (this REQ)

- **Demographic validator** (`ValidateDemographic`) and **AQL lint** (`ValidateAQL`) — separate entry points in the same package, deferred.
- **Validating wire bytes / canonical JSON** — the validator never imports `serialize/`. Callers decode first, validate second.
- **External terminology lookup** — value-set membership against SNOMED CT / LOINC / external services. REQ-103 closed-code-list checking is the v1 ceiling.
- **Cross-archetype slot-fill resolution** — no federated archetype repository; slot fit uses the RM-type-prefix fallback.
- **Full ADL2 / AOM 2 validation semantics.**

- **Lives in:** [`openehr/validation/`](../../openehr/validation/)
- **Probes:** PROBE-025 (composition validation against fixture OPT + composition); PROBE-026 (missing required node, cardinality, alternative_mismatch, rm_type_mismatch, and primitive negative cases — see [`testkit/probes/validation/`](../../testkit/probes/validation/))

---

## REQ-107 — Template-driven RM instance example generator

**Status:** Draft (Phase 0 landed).

The SDK **MUST** ship a template-authoritative RM instance synthesiser at `openehr/instance/`: given a compiled OPT, produce a conformant RM object graph whose structure and primitive leaves satisfy the same template-driven contract REQ-102 validates against. The generator is the inverse of validation v2 — same compiled-OPT walk, opposite direction (`rmwrite` instead of `rmread`).

### Scope

The generator is the single skeleton-and-populate engine the composition builder (REQ-101), tests, examples, and CDR seeding (STRAND-01) all consume. The root may be **any** RM type the OPT's `rm_type_name` declares — `COMPOSITION`, `OBSERVATION`, `EVALUATION`, `INSTRUCTION`, `ACTION`, `ADMIN_ENTRY`, `CLUSTER`, `SECTION`, `GENERIC_ENTRY`, `ELEMENT`. Output is **synthetic example data**: structurally and constraint-valid for the OPT, not clinically meaningful. The closed root set is v1; new root types appear through a follow-up REQ.

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
    Language  string         // ISO 639-1; defaults from Compiled.Language()
    Territory string         // for COMPOSITION roots
    Composer  rm.PartyProxy  // required when root is COMPOSITION
    Now       time.Time      // clock for EVENT / context times
}

func Generate(ctx context.Context, c *templatecompile.Compiled, opts Options) (any, error)
func AsComposition(v any) (*rm.Composition, error)
func AsObservation(v any) (*rm.Observation, error)
// … closed set matching validation ContentItem + standalone archetype roots
```

`Generate` **MUST** return a root RM value satisfying the OPT's structural rules and REQ-103 primitive constraints. Specifically, `Minimal` materialises only attributes with existence lower ≥ 1 (plus BMM-mandatory implicit attrs); `Example` additionally populates every primitive leaf via `PrimitiveConstraint.ExampleValue()`. Multi-valued attributes are sized to `max(existence.lower, 1)`; `C_SINGLE_ATTRIBUTE` alternatives resolve first-child-wins (matching validation v2's first-alternative semantics).

Slot handling (v1): pinned archetype-root children under a slot are synthesised; pure `ARCHETYPE_SLOT` assertions resolve via REQ-104 prefix match or the first include pattern — same compromise as validation slot-fit.

### Trust model

The compiled OPT is **authoritative for structure**. The RM graph is assembled attribute-by-attribute from compiled metadata; the generator never guesses paths from an empty composition. Primitive leaves come from `PrimitiveConstraint.ExampleValue()` (REQ-103), which guarantees `Validate(ExampleValue()) == nil` for bounded constraints. Optional OPT `<assumed_value>` / `<default_value>` (when compile captures them — a Phase 0 follow-up) **override** the factory.

The generator is **sound** (every output is valid against the OPT), not **complete** (it does not enumerate every valid instance — different policies may produce different but equally valid trees). Sound × validator-aligned ⇒ PROBE-027 cross-checks the contract.

### Trust model — phasing

Phase 0 landed: `ExampleValue()` on every `PrimitiveConstraint`; spec; PROBE-027 stub; this REQ row. Phases 1–4 (rmwrite + RM construction table, core synthesiser walk, non-composition roots + PROBE-027, REQ-101 integration) are out of scope for this phase and tracked in [`docs/plans/2026-05-24-template-instance-example-generator.md`](../plans/2026-05-24-template-instance-example-generator.md).

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

In v1 the public signature accepts `*templatecompile.Compiled` (module-local), same restriction as `validation.ValidateComposition` per [ADR 0005](../adr/0005-compiled-template-foundation.md) §C2 — the public re-export lands with REQ-101 Phase 1.

- **Lives in:** [`openehr/instance/`](../../openehr/instance/) (lands in Phase 2); `openehr/template/constraints/.ExampleValue()` (Phase 0 — landed); `internal/templateinstance/` (Phase 1+).
- **Probes:** PROBE-027 — `instance.Generate` + `validation.ValidateComposition` round-trip clean on the same OPT (Phase 3).

