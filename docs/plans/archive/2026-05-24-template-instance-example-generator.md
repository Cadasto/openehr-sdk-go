# Plan — Template-driven RM instance example generator

**Date:** 2026-05-24
**Status:** Landed (PRs #18 + #20)
**Owner:** SDK maintainers
**Covers:** REQ-013, REQ-024, REQ-030–033, REQ-040; **REQ-107** (generic OPT → RM instance synthesis)
**Probes:** **PROBE-027** (Implemented Sandbox — generate → `validation.ValidateComposition` on `vital_signs.opt` + `clinical_note.opt`)
**Implementation:** landed (Phases 0–3: spec + `ExampleValue()` + `rmwrite` + `instance.Generate` + non-composition accessors + PROBE-027; Phase 4 REQ-101 integration covered in [`archive/2026-05-21-composition-builder.md`](2026-05-21-composition-builder.md))
**Depends on:** [`2026-05-22-template-req100-followups.md`](2026-05-22-template-req100-followups.md) Phases 4–6 (compiled template + walker + REQ-103); [`archive/2026-05-24-composition-validation-template-driven.md`](2026-05-24-composition-validation-template-driven.md) (shared template-driven walk + `rmread`/`rmwrite`); [`archive/2026-05-21-composition-builder.md`](2026-05-21-composition-builder.md) (REQ-101 — composition authoring API; consumes this engine). Follow-up: [`2026-05-26-c-primitive-object-wire-parser.md`](2026-05-26-c-primitive-object-wire-parser.md) covers the `C_PRIMITIVE_OBJECT.<item>` parser gap + UID emission needed to widen PROBE-023 to full round-trip.
**Defers:** Per-template generated Go structs; FLAT/STRUCTURED export; OET authoring; runtime federated slot-fill repository; clinically realistic synthetic data (FHIR Synthea-style); multi-language term translation

## Goal

Ship a **template-authoritative instance synthesiser** that walks a compiled OPT and materialises a conformant openEHR RM object graph — not only `COMPOSITION`, but **any root `rm_type_name`** the template declares (`OBSERVATION`, `EVALUATION`, `CLUSTER`, `SECTION`, `ADMIN_ENTRY`, …).

Primary use cases:

| Consumer | Need |
|---|---|
| Tests / probes | Fixture compositions without hand-building RM trees |
| Examples / sandboxes | `cmd/examples/*` that POST realistic sample data |
| Composition builder (REQ-101) | Skeleton + default population before `Set(path, …)` |
| Validation development | Positive instances that should pass REQ-102 |
| Benchmark / CDR seeding (STRAND-01) | Bulk instance generation from OPT catalogue |

The output is **synthetic example data** — structurally and constraint-valid for the OPT, not clinically meaningful. Callers treat it like factory defaults, not patient data.

---

## Problem statement

### What exists today

| Artefact | Scope | Gap |
|---|---|---|
| Hand-built test helpers (`validVitalSignsComposition`, `bloodPressureComposition`) | Per-fixture | Do not scale to every OPT |
| [`2026-05-21-composition-builder.md`](2026-05-21-composition-builder.md) (REQ-101, planned) | `*rm.Composition` + path `Set` API | Not landed; composition-only surface |
| [`openehr/validation/`](../../../openehr/validation) (REQ-102) | Validates existing graphs | Does not create instances |
| `typereg` + BMM codegen | Construct empty RM types by name | No OPT-aware tree assembly |

### What is required

A single engine that answers:

> Given `*templatecompile.Compiled`, produce an RM value whose shape and leaf values satisfy the OPT's structural rules and REQ-103 primitive constraints.

That requires the **same trust model as validation v2** ([`archive/2026-05-24-composition-validation-template-driven.md`](2026-05-24-composition-validation-template-driven.md)): the **OPT walk is authoritative**; the RM graph is assembled attribute-by-attribute from compiled metadata, not by guessing paths from an empty composition.

---

## Relationship to REQ-101 (composition builder)

REQ-101 and REQ-107 are **layered**, not duplicate:

```
┌─────────────────────────────────────────────────────────┐
│  openehr/composition  (REQ-101)                         │
│  NewSkeleton / Builder.Set / Build → *rm.Composition    │
│  Path-first authoring API, composer/territory options     │
└───────────────────────────┬─────────────────────────────┘
                            │ uses
┌───────────────────────────▼─────────────────────────────┐
│  openehr/instance  (REQ-107)                            │
│  Generate(c, opts) → Instance (typed erasure + helpers) │
│  Template-driven synthesis for ANY root RM type          │
└───────────────────────────┬─────────────────────────────┘
                            │ uses
┌───────────────────────────▼─────────────────────────────┐
│  internal/templateinstance (or shared with validation v2) │
│  WalkTemplate + rmwrite + example value factory           │
└─────────────────────────────────────────────────────────┘
```

- **REQ-107** owns the recursive OPT → RM tree algorithm and primitive example values.
- **REQ-101** owns the composition-specific options (`WithComposer`, `WithTerritory`, `SetQuantity`, REST `TemplateID`) and delegates skeleton creation to REQ-107 with `RootPolicy: Minimal` or `Example`.

Implement REQ-107 **before or in parallel with** REQ-101 Phase 1 so the composition builder does not embed a second walker.

---

## Target architecture

### Template-driven synthesis walk

Mirror the validation v2 lockstep pattern in the **opposite direction**:

| Step | Validation v2 | Instance generator |
|---|---|---|
| Driver | Compiled OPT | Compiled OPT |
| RM side | Read attribute (`rmread`) | Write / append attribute (`rmwrite`) |
| Per attribute | Check existence / cardinality | Create container if missing; size slice to lower bound |
| Per child constraint | Match RM child to OPT child | Instantiate OPT child; attach under parent |
| Primitive leaf | `PrimitiveConstraint.Validate(value)` | `PrimitiveConstraint.ExampleValue()` → set on RM |
| Output | `[]Issue` | `any` (root RM value) + optional path log |

```text
Compile(OPT) → Compiled
       │
       ▼
WalkTemplate(root CompiledNode, synthesiser)
       │
       ├─ For each CompiledAttribute on node:
       │     ensure RM field exists (rmwrite)
       │     Single → one child / value
       │     Multiple → [] sized to max(lower, 1) or policy
       │
       ├─ Archetype root → set archetype_node_id, archetype_details
       ├─ Slot → pick first allowed archetype child from OPT subtree (v1)
       └─ Primitive leaf → example value from REQ-103 constraint
       │
       ▼
Root RM value (e.g. *rm.Composition)
```

### Example value factory (REQ-103 extension)

Add an **`ExampleValue()`** (or `SampleValue()`) method on each `PrimitiveConstraint` implementation in `openehr/template/constraints/`, parallel to `Validate`:

| Constraint | Example strategy (v1) |
|---|---|
| `CInteger` / `CReal` | Midpoint of bounded range, or first list entry |
| `DvQuantity` | Mid magnitude in allowed units; first unit in list |
| `CodePhrase` / coded lists | First closed-list code; terminology id from constraint |
| `CString` | First list entry, or literal `"example"` if only pattern |
| `CBoolean` | `true` if allowed, else `false` |
| `CDvOrdinal` | First ordinal in list |
| `CDate` / `CTime` / `CDateTime` / `CDuration` | Fixed documented sentinel (`2020-01-01`, `12:00:00`, …) satisfying pattern when cheap to check |

Values MUST satisfy `Validate(example) == nil` when a constraint is bounded — generator and validator stay aligned.

Optional OPT `<assumed_value>` / `<default_value>` (when compile captures them — see prerequisites) **override** the factory.

### Generation policies

Functional options control how much tree is materialised:

| Policy | Behaviour |
|---|---|
| `Minimal` | Only attributes with existence lower ≥ 1 (and BMM-mandatory implicit attrs). Smallest valid tree. Default for skeleton. |
| `Example` | `Minimal` + populate every primitive leaf under visited nodes with example values. Default for demos/fixtures. |
| `Full` (later) | Fill optional branches up to cardinality upper bound — larger trees for stress tests |

Slot handling (v1): when the OPT pins concrete archetype roots under a slot, synthesise those children; when only `ARCHETYPE_SLOT` assertions exist, use REQ-104 prefix match or first include pattern — same compromise as validation slot-fit.

---

## Package layout

Per [`docs/specifications/module-layout.md`](../../specifications/module-layout.md) and REQ-013:

| Package | Role |
|---|---|
| `openehr/instance/` | Public API — `Generate`, options, `AsComposition`, `AsObservation`, … |
| `internal/templateinstance/` | Walk engine, `rmwrite`, slot expansion (optional split from validation's `rmread` sibling) |
| `openehr/template/constraints/` | Add `ExampleValue()` to each primitive type |
| `internal/templatecompile/walk/` | Extend with template-driven RM walk (shared interface with validation v2) |

**Forbidden imports in `openehr/instance/`:** `transport/`, `auth/`, `openehr/client/*`, `serialize/` (same rule as validation — `canjson` only in tests/examples).

**Module-local v1 note:** Like validation today, `Generate` may take `*templatecompile.Compiled` until ADR 0005 promotes `template.Compile` publicly.

---

## Public API (target)

```go
package instance

// Policy controls how much of the OPT tree is materialised.
type Policy int

const (
    Minimal Policy = iota // required structure only
    Example               // required + example primitive values
)

type Options struct {
    Policy    Policy
    Language  string         // ISO 639-1 for DV_TEXT / names; default from Compiled.Language()
    Territory string         // for COMPOSITION roots
    Composer  rm.PartyProxy  // required when root is COMPOSITION
    Now       time.Time      // clock for EVENT / context times; default time.Now().UTC()
}

// Generate synthesises an RM instance for the compiled template's root type.
// Returns the root as any — use AsComposition, AsObservation, or a type switch.
func Generate(ctx context.Context, c *templatecompile.Compiled, opts Options) (any, error)

// Typed accessors after Generate (fail with ErrTypeMismatch if wrong root).
func AsComposition(v any) (*rm.Composition, error)
func AsObservation(v any) (*rm.Observation, error)
// … closed set matching validation ContentItem + standalone archetype roots
```

Composition-only sugar (optional thin wrapper):

```go
// In openehr/composition — delegates to instance.Generate + AsComposition.
func NewExample(c *templatecompile.Compiled, opts ...Option) (*rm.Composition, error)
```

---

## Prerequisites

| Prerequisite | Owner | Blocks |
|---|---|---|
| Compiled template with existence + implicit attrs | Landed | — |
| REQ-103 primitive constraints | Landed | Example factory |
| `internal/templatecompile/walk` | Landed | OPT DFS |
| Validation v2 `rmread` / lockstep walk design | Plan | Shared attribute naming table |
| Parse `<default_value>` / `<assumed_value>` on constraints | Follow-up plan / Phase 0 | Override example factory |
| Attribute `<cardinality>` interval on `C_MULTIPLE_ATTRIBUTE` | [composition validation plan](2026-05-24-composition-validation-template-driven.md) Phase 0 | Correct child counts |

---

## Phases

### Phase 0 — Spec + constraint example factory

**Outcome:** Normative REQ-107 stub; primitives can emit valid example values.

**Tasks:**

1. **`docs/specifications/clinical-modeling.md` § REQ-107** — contract, policies, root-type closed set, relationship to REQ-101/102.
2. **`ExampleValue()` on `PrimitiveConstraint`** — each implementation in `openehr/template/constraints/`; tests: `Validate(ExampleValue())` empty for bounded constraints.
3. **REQ.md + traceability.yaml** — row for REQ-107 (planned).
4. **PROBE-027 stub** in conformance matrix.

**Definition of done:** `go test ./openehr/template/constraints/...` green; `make spec-check` passes with new REQ row.

---

### Phase 1 — `rmwrite` + RM construction table

**Outcome:** Given parent RM value + `CompiledAttribute`, attach child object(s).

**Tasks:**

1. **`internal/templateinstance/rmwrite/`** (or shared `internal/templatewalk/rm/`) — inverse of validation v2 `rmread`:
   - `EnsureSingle(parent, attrName, child any)`
   - `AppendMultiple(parent, attrName, child any)`
   - `NewRM(rmTypeName string) (any, error)` via `typereg` / BMM registry
2. Set LOCATABLE fields: `archetype_node_id`, `name` (from `CompiledNode.Term`), `uid` where required.
3. Archetype roots: `archetype_details` with `archetype_id`; `template_id` from `Compiled.TemplateID()` when root is top-level template instance.
4. Table-driven tests per `(parentType, attr)` row used by `vital_signs.opt`.

**Definition of done:** Unit tests construct a standalone `OBSERVATION` subtree without hand-written structs.

---

### Phase 2 — Core synthesiser walk

**Outcome:** `instance.Generate` works for `COMPOSITION` and at least one non-composition root (e.g. `OBSERVATION` extracted as template root in test).

**Tasks:**

1. **Template walk** — depth-first over `CompiledNode` + attributes; respect existence lower bound under `Minimal` policy.
2. **Single vs multiple** — one child constraint for Single; for Multiple, create `lower` children (default `1` when existence requires presence).
3. **Alternatives** — when `C_SINGLE_ATTRIBUTE` has N children, pick first child constraint in v1 (document; match validation v2 alternative order).
4. **Primitive leaves** — call `ExampleValue()`; assign to `ELEMENT.value` or inline DV on attribute.
5. **COMPOSITION defaults** — category `433|event|`, language, territory, composer, `context.start_time` from `Options`.
6. **EVENT defaults** — `PointEvent` with `time = opts.Now`.
7. **`Generate` + `AsComposition`** — integration test on `vital_signs.opt`.
8. **`cmd/examples/generate-example/main.go`** — OPT path flag → emit `canjson` to stdout (example imports serialize; library does not).

**Definition of done:** `Generate` on `vital_signs.opt` returns non-nil `Content`; `go test ./openehr/instance/...` green.

---

### Phase 3 — Non-composition roots + probe

**Outcome:** Any template whose root `rm_type_name` is in the closed set can synthesise an instance.

**Tasks:**

1. Document **closed root types** v1: `COMPOSITION`, `OBSERVATION`, `EVALUATION`, `INSTRUCTION`, `ACTION`, `ADMIN_ENTRY`, `CLUSTER`, `SECTION`, `GENERIC_ENTRY`, `ELEMENT` (archetype-only templates).
2. `AsObservation`, `AsEvaluation`, … typed accessors with `ErrTypeMismatch`.
3. **PROBE-027** — `Generate` + `validation.ValidateComposition` (or generic validator when v2 lands) returns `OK` on same OPT for `Minimal` and `Example` policies.
4. **REQ-107** → `partial` in traceability.

**Definition of done:** PROBE-027 pass; at least two fixture OPTs (vital_signs + clinical_note).

---

### Phase 4 — REQ-101 integration + polish

**Outcome:** Composition builder uses shared engine; no duplicate walk logic.

**Tasks:**

1. **`composition.NewSkeleton`** → `instance.Generate(..., Policy: Minimal)` + `AsComposition`.
2. **`composition.NewExample`** (optional) → `Policy: Example`.
3. Slot expansion improvements when REQ-104 lands.
4. **PROBE-023** (composition builder) can reuse instances from Phase 2.

**Definition of done:** REQ-101 Phase 1 checklist references REQ-107; `openehr/composition` contains no independent OPT walker.

---

## Correctness contract

Generated instances MUST satisfy:

| Check | When |
|---|---|
| REQ-102 v1 (current) | Primitive + root checks that apply to present nodes |
| REQ-102 v2 (when landed) | Full structural validation including missing-node detection |
| `canjson.Marshal` round-trip | Tests / examples only |

The generator is **sound** (valid instances), not **complete** (does not generate every valid instance). Different policies may produce different but equally valid trees.

---

## Testing strategy

| Layer | Focus |
|---|---|
| `constraints/*_example_test.go` | Each `ExampleValue` passes its own `Validate` |
| `rmwrite` | Attribute attachment in isolation |
| `instance` integration | `vital_signs.opt`, `clinical_note.opt` — spot paths via `Compiled.NodeAt` |
| PROBE-027 | Cross-package: generate → validate |
| Regression | `Generate` → delete random required node → validator MUST fail (after v2) |

---

## Non-goals

- Clinically realistic distributions (names, plausible vitals).
- FLAT / STRUCTURED example strings (REQ-053).
- Generating **all** valid instances or combinatorial coverage.
- Writing to a CDR (caller's `client/ehr` responsibility).
- Validating during generation (separate `validation` call — but see PROBE-027).

---

## Shared infrastructure with validation v2

Implementers SHOULD treat these as **one internal attribute-access layer** with read and write sides:

| Component | Validation v2 | Instance generator |
|---|---|---|
| Attribute name → RM field | `rmread` | `rmwrite` |
| OPT traversal | `WalkComposition` | `WalkSynthesise` (same stack frames; different visitor) |
| Path strings | `CompiledNode.AQLPath()` | Same |

A single design session for both plans avoids divergent `(RMType, attrName)` tables.

---

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0: REQ-107 spec + `ExampleValue()` on constraints | |
| Phase 1: `rmwrite` + typereg construction | |
| Phase 2: `instance.Generate` (COMPOSITION + OBSERVATION test) | |
| Phase 3: Non-composition roots + PROBE-027 | |
| Phase 4: REQ-101 `NewSkeleton` delegation | |
| `generate-example` cmd example | |
| `TestInstanceNoSerializeImport` | |
| REQ-107 `implementation: landed` | |

---

## Cross-references

- [`2026-05-21-composition-builder.md`](2026-05-21-composition-builder.md) — REQ-101 authoring API
- [`archive/2026-05-24-composition-validation-template-driven.md`](2026-05-24-composition-validation-template-driven.md) — inverse walk; share `rmread`/`rmwrite`
- [`2026-05-22-template-req100-followups.md`](2026-05-22-template-req100-followups.md) — compiled template foundation
- [`docs/adr/0005-compiled-template-foundation.md`](../../adr/0005-compiled-template-foundation.md) — public `Compiled` promotion timing
