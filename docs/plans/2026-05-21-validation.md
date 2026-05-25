# Plan — Data instance validation (Composition vs OPT, demographic, AQL)

**Date:** 2026-05-21 (research-updated 2026-05-22)
**Status:** Draft (umbrella — composition scope superseded)
**Owner:** SDK maintainers
**Covers:** REQ-013, REQ-014; **REQ-102** composition validation → see [`2026-05-24-validation-v2-template-driven.md`](2026-05-24-validation-v2-template-driven.md) (**landed**)
**Probes:** PROBE-024 (landed under template — REQ-103); PROBE-025/026 (landed — v2 plan); PROBE-021 partially (AQL path errors at execute time — query plan owns syntax)
**Implementation:** partial — REQ-102 composition validator **landed** ([v2 plan](2026-05-24-validation-v2-template-driven.md)); demographic + AQL lint **planned**
**Depends on:** [`2026-05-21-template-parser.md`](2026-05-21-template-parser.md) (REQ-100, landed); [`2026-05-22-template-req100-followups.md`](2026-05-22-template-req100-followups.md) Phases 4 + 4-bis + 5 + 6 (compiled template + RMInfoLookup + walker + REQ-103 primitives); [`2026-05-21-composition-builder.md`](2026-05-21-composition-builder.md) Phase 1 (fixture compositions); [`2026-05-21-aql-builders.md`](2026-05-21-aql-builders.md) optional for AQL lint subset
**Defers:** Full ADL2 / AOM 2 validation surface; terminology server checks (external code-list verification); validate wire bytes / canonical JSON (no `serialize/` import); cross-archetype reference integrity (slot-fill linker)

## Goal

**`openehr/validation`** exposes structural validators that CI pipelines, write webhooks, and pre-flight SDK calls can use to check in-memory RM artefacts **before** they cross a wire boundary:

1. **Composition vs OPT** — required attributes present, cardinality respected, primitive constraints satisfied.
2. **Demographic** — structural RM checks on `PARTY` hierarchies (no OPT involved).
3. **AQL** — static syntax + path-residency lint, **not** full execution. Server-side path resolution remains the CDR's job (PROBE-021).

Clean separation: validators walk **`openehr/rm`** values and **`*template.Compiled`** trees — never decode JSON/XML. The validator is the first non-trivial consumer of the compiled template Phase 4 layer in the [REQ-100 follow-up plan](2026-05-22-template-req100-followups.md).

## Integration with existing stack

| Piece | Location | Role |
|---|---|---|
| RM graph | `openehr/rm/` | Validation input — the data being checked |
| Compiled OPT | `openehr/template/` (`Compiled`) | Constraint source — required attrs, cardinality, primitive constraints |
| Primitive constraints | `openehr/template/constraints/` (REQ-103) | Typed leaf constraint payloads (`DvQuantity`, `CodePhrase`, ...) with `Validate(value any)` |
| RMInfoLookup | `openehr/rm/rminfo/` | Container-vs-single discrimination during walks |
| Walker | `openehr/template/walk/` | `WalkComposition` lockstep traversal of OPT + RM trees |
| AQL models | `openehr/aql/` | Path extraction + lint targets |
| Query client | `openehr/client/query/` | Maps backend AQL errors (`ErrPathResolution` already in `aql/errors.go`) |
| Serialize | `openehr/serialize/` | **Must not import** — validators operate on in-memory graphs only |

## Design rationale (research baseline)

Reference Java implementations (notably [ehrbase openEHR_SDK](https://github.com/ehrbase/openEHR_SDK)'s `ValidationWalker` extending `FromCompositionWalker<List<ConstraintViolation>>`, and [openEHR/archie](https://github.com/openEHR/archie)'s `RMObjectValidator`) converge on these design principles:

1. **Single tree walk, template-driven.** The validator walks the **composition** with the **template** as the guide — not two separate trees zipped after the fact, not a path-by-path lookup loop. At each composition node, the walker peeks at the matching template node and dispatches to per-RM-type validators.
2. **Three orthogonal validator dimensions:**
   - **Structural** (cardinality, required attributes present) — driven by `CompiledNode.Existence()` / `Occurrences()` and RMInfoLookup.
   - **Primitive constraint** (DV_QUANTITY in range, CODE_PHRASE in code list, C_STRING matches pattern) — delegated to `CompiledNode.PrimitiveConstraint().Validate(value)` from REQ-103.
   - **Terminology / external** (a code is actually defined in SNOMED CT) — orthogonal pass; pluggable so callers can plug in a terminology server. Out of v1 scope.
3. **Collect-all, not fail-fast.** Validators emit a `[]ValidationIssue` from one walk. UIs and CIs need the complete list, not the first failure.
4. **`ArchetypeSlot` validation by RM-type prefix fallback.** The OPT carries `<includes>` / `<excludes>` archetype-id assertions, but parsing the assertion grammar is a known-deferred item (REQ-104). v1 validator accepts any candidate whose archetype id starts with `openEHR-EHR-<rmType>.`. This pragmatic shortcut is consistent with reference implementations.
5. **Validators are stateless.** A `Validator` value can be reused concurrently — the walk creates its own per-call accumulator.

## v1 validation depth (honest bounds)

Document in REQ-102 so consumers know what v1 does **not** guarantee:

| Check | v1 | Later |
|---|---|---|
| Required attributes present (via RMInfoLookup + OPT existence) | yes | |
| Cardinality (min/max occurrences) | yes | |
| Primitive value ranges (DV_QUANTITY magnitude, C_REAL range) | yes (via REQ-103) | |
| Primitive code list membership (CODE_PHRASE closed lists) | yes (via REQ-103) | |
| Pattern constraints (C_STRING regex, C_DATE pattern) | yes (via REQ-103) | |
| Terminology binding value-set (external — SNOMED, LOINC) | no | needs terminology client (separate REQ) |
| Cross-archetype reference integrity (slot-fill linker) | no | needs archetype repository |
| ARCHETYPE_SLOT assertion grammar | RM-type-prefix fallback | full grammar (REQ-104) |
| AQL full grammar | no | lint identifiers + FROM clause shape |
| Demographic invariants | structural only | policy rules (party-role consistency, identifier patterns) |
| Multi-language composition consistency (terms match declared language) | no | once REQ-105 lands |

## Out of scope

- Importing `openehr/serialize/` (hard rule — add `go vet` / custom test `TestValidationNoSerializeImport`).
- Replacing backend validation on write (SDK pre-flight only — the CDR is still authoritative).
- OPT diff / merge tools (separate REQ).
- Live terminology lookup (SNOMED CT / LOINC code resolution against external services).
- Federated archetype repository slot-fill resolution.

## Phases

### Phase 0 — REQ-102, error model, package layout

**Outcome:** Spec + sentinel errors + interface stubs.

**Tasks:**

1. **`docs/specifications/clinical-modeling.md` § REQ-102** — three validator entry points, error aggregation (`Result` with `Issues []Issue`), severity (error vs warning in v1 = error only), the "collect-all not fail-fast" contract.
2. **Sentinels** in `openehr/validation/errors.go`:
   ```go
   var (
       ErrCardinality  = errors.New("validation: cardinality")
       ErrRequired     = errors.New("validation: required")
       ErrTypeMismatch = errors.New("validation: type mismatch")
       ErrPrimitive    = errors.New("validation: primitive constraint")
       ErrSlotFill     = errors.New("validation: slot fill")
       ErrAQLSyntax    = errors.New("validation: aql syntax")
   )
   ```
3. **`Issue` payload** — `Path Path`, `Code string` (e.g. `cardinality`, `out_of_range`, `code_not_in_list`), `Detail string`, `Severity` (only `Error` in v1), optional `Constraint` reference (for diagnostics).
4. **Interfaces:**
   ```go
   type CompositionValidator interface {
       Validate(comp *rm.Composition, c *template.Compiled) Result
   }
   type DemographicValidator interface {
       Validate(party rm.PartyProxy) Result
   }
   type AQLValidator interface {
       ValidateQuery(q aql.Query, c *template.Compiled) Result
   }
   ```
5. **Registry** — REQ-102 planned in traceability.

**Definition of done:** `make spec-check`; package compiles; `TestValidationNoSerializeImport` forbids `serialize/` import.

### Phase 1 — Composition validator (structural + primitive)

**Outcome:** Catch missing required attributes, cardinality violations, and primitive constraint violations on fixture compositions.

**Tasks:**

1. **`Validate(comp, c)`** — implements `CompositionValidator` using `template/walk/WalkComposition`:
   - **Structural visitor** — at each composition node, look up the matching `CompiledNode`:
     - Verify RM type matches (composition node's actual RM type ⊆ template's declared `RMTypeName()`); else `ErrTypeMismatch`.
     - Count children per attribute; check against `attr.Cardinality()` and `attr.Existence()`; else `ErrCardinality` or `ErrRequired`.
     - For required attributes the OPT injected via RMInfoLookup that have no corresponding child in the composition: `ErrRequired`.
   - **Primitive visitor** — at each composition leaf with a `PrimitiveConstraint`:
     - Call `cn.PrimitiveConstraint().Validate(rmValue)`; append any violations to the result, mapped to `Issue{Code: violation.Code, Path: cn.AQLPath()}`.
2. **Per-RM-type primitive dispatchers** — small wrapper functions converting RM Go values to the primitives the constraint expects:
   - `*rm.DvQuantity` → `(magnitude, unit)` pair fed to `DvQuantity.Validate`.
   - `*rm.DvCodedText` → `defining_code` fed to `CodePhrase.Validate`.
   - `*rm.DvOrdinal` → ordinal value fed to `CDvOrdinal.Validate`.
   - `*rm.DvText` / `*rm.DvCount` / `*rm.DvBoolean` / `*rm.DvDateTime` → corresponding primitives.
3. **`ArchetypeSlot` handling** — when the walker encounters a slot node whose composition child is an `OBSERVATION` / `SECTION` / etc., apply the RM-type-prefix fallback: `slot.AllowsRMType(child.ArchetypeNodeID())`. Future REQ-104 swaps in the parsed assertion AST.
4. **Tests** — valid built composition (from composition-builder Phase 1 skeleton) passes; remove required node → `ErrRequired` with stable path; set out-of-range quantity → `ErrPrimitive` with `out_of_range` code; set unknown code on a closed CODE_PHRASE → `ErrPrimitive` with `code_not_in_list`.
5. **`cmd/examples/validate-composition/main.go`** — load fixture OPT + composition → run validator → print issues.

**Definition of done:**

- `go test ./openehr/validation/...` green.
- REQ-102 `implementation: partial` in traceability.

### Phase 2 — Demographic validator + AQL lint subset

**Outcome:** Second and third validators usable in CI.

**Tasks:**

1. **`ValidateDemographic(party)`** — structural RM checks on `PARTY`, `PERSON`, `ORGANISATION`, `ROLE`:
   - Required fields per RM (names non-empty, identities list non-empty where BMM marks required).
   - `PartyRelated.relationship` carries a `DvCodedText` with a valid relationship code (terminology binding check deferred to external terminology REQ).
   - Driven by RMInfoLookup — same dependency as Composition validator.
2. **`ValidateAQL(q, c)`** — `AQLValidator` implementation:
   - Non-empty query; brace balance; basic `SELECT ... FROM ... [WHERE]` shape.
   - `FROM` clause contains at least one archetype-id; when `c != nil`, every archetype-id in `FROM` MUST be present in `c.AllByArchetypeID()`.
   - Path lint: `SELECT` projection paths and `WHERE` predicate paths are syntactically valid `Path` strings (delegated to `template.ParsePath`).
   - **Does NOT** perform full grammar resolution against the OPT — that's the CDR's job at execute time (PROBE-021 covers the wire mapping).
3. **PROBE-024** — composition missing required element → stable issue code in result (sandbox; lives in `testkit/probes/validation/`).
4. **REQ-102** → `landed` when Composition validator + demographic + AQL lint + probe stable.

**Definition of done:** All three validators documented in package doc; no `serialize/` import; PROBE-024 implemented.

## Public API (target)

```go
package validation

// Validate the composition against the OPT. Walks both trees in lockstep,
// collects all issues, returns Result.OK == false when len(Issues) > 0.
func ValidateComposition(comp *rm.Composition, c *template.Compiled) Result

// Validate a demographic party (structural RM checks only in v1).
func ValidateDemographic(party rm.PartyProxy) Result

// Lint an AQL query against the OPT (syntactic + path-residency only;
// the CDR retains authoritative path resolution at execute time).
func ValidateAQL(q aql.Query, c *template.Compiled) Result

type Result struct {
    OK     bool
    Issues []Issue
}

type Issue struct {
    Path     template.Path // empty for global issues
    Code     string        // stable, programmatic identifier
    Detail   string        // human-readable
    Severity Severity      // Error in v1; Warning reserved for later
}

type Severity int
const (
    Error   Severity = iota
    Warning              // reserved; not emitted in v1
)
```

## Implementation checklist

| Step | Status |
|---|---|
| Depends-on: REQ-100 follow-up plan Phases 4 + 4-bis + 5 + 6 | |
| Phase 0 REQ-102 spec + sentinels + interfaces + import-guard test | landed |
| Phase 1 Composition validator (structural + primitive dispatchers) | landed (RM-guided intermediate; superseded by v2 — see [2026-05-24-validation-v2-template-driven.md](2026-05-24-validation-v2-template-driven.md)) |
| Phase 2 Demographic validator | |
| Phase 2 AQL lint subset | |
| PROBE-024 sandbox probe | |
| `validate-composition` example | |
| `TestValidationNoSerializeImport` import-guard test | |
| `make ci` green | |

### Phase 1 superseded by template-driven v2

The Phase 1 deliverable above landed on `feat/req102-validation` as an **RM-guided** validator: descend the composition graph via typed switches, build AQL paths from the composition's at-codes, look up OPT constraints at those paths, and apply REQ-103 primitive checks at every matched leaf. That intermediate cannot flag composition-side **missing** required nodes (no RM subtree → no path → no constraint lookup).

The v2 plan ([`2026-05-24-validation-v2-template-driven.md`](2026-05-24-validation-v2-template-driven.md)) inverts the walk: the compiled OPT is the driver, the composition is the value source. v2 closes the structural completion gap (existence, cardinality, alternatives, RM type match) using the same public `ValidateComposition` signature.

## Mapping to specs

- [`docs/specifications/module-layout.md`](../../docs/specifications/module-layout.md) — validation must not import serialize
- [`docs/specifications/rm-modeling.md`](../../docs/specifications/rm-modeling.md) — concrete types, typereg
- [`openehr/validation/doc.go`](../../openehr/validation/doc.go) — package intent
- [`docs/specifications/clinical-modeling.md`](../../docs/specifications/clinical-modeling.md) § REQ-100 — OPT parse + paths (landed)
- Proposed: `docs/specifications/clinical-modeling.md` § REQ-102 — validation surfaces
- [`docs/specifications/conformance.md`](../../docs/specifications/conformance.md) — PROBE-021 (execute), PROBE-024 (proposed)

## References (research baseline, informational)

The validator design draws lessons from reference Java implementations. The SDK retains its own AOM 1.4 / ADL 1.4 typed surface; these references inform the per-RM dispatch table and the slot-fallback compromise.

- **ehrbase openEHR_SDK** — [`github.com/ehrbase/openEHR_SDK`](https://github.com/ehrbase/openEHR_SDK). `validation/.../ValidationWalker.java` is the closest analogue to Phase 1 (single tree walk with per-RM `ConstraintValidator` lookup). `PrimitiveConstraintMapper.java` shows how to synthesise a transient `CPrimitiveObject` from a simplified template input — a useful pattern when the validator needs to delegate to a primitive validator. `TerminologyValidationVisitor.java` shows the orthogonal terminology-check pass we defer to a later REQ.
- **openEHR/archie** — [`github.com/openEHR/archie`](https://github.com/openEHR/archie). `tools/.../rmobjectvalidator/RMObjectValidator.java` is the AOM-2-flavoured equivalent, with `RmPrimitiveObjectValidator`, `RmOccurrenceValidator`, `RmTupleValidator`, `RmMultiplicityValidator` providing a per-dimension breakdown that mirrors our `ErrCardinality` / `ErrRequired` / `ErrPrimitive` sentinel split. Archie's `APathQueryCache` is the pattern for memoising path lookups per OPT.
- **openEHR Reference Model** — [`specifications.openehr.org/releases/RM/latest`](https://specifications.openehr.org/releases/RM/latest). Required-attribute table per RM class; `DvQuantity` magnitude/unit semantics; `DvInterval` bound flags.
