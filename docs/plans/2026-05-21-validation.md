# Plan — Validation (Composition vs OPT, demographic, AQL)

**Date:** 2026-05-21
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-013, REQ-014; proposed **REQ-102** (validation surfaces)
**Probes:** PROBE-024 (proposed — OPT constraint violation mapping); PROBE-021 partially (AQL path errors at execute time — builder plan owns syntax)
**Implementation:** planned
**Depends on:** [`2026-05-21-template-parser.md`](2026-05-21-template-parser.md) Phase 1; [`2026-05-21-composition-builder.md`](2026-05-21-composition-builder.md) Phase 1 (fixture compositions); [`2026-05-21-aql-builders.md`](2026-05-21-aql-builders.md) optional for AQL lint subset
**Defers:** Full Archie validation; terminology server checks; validate wire bytes / canonical JSON (no `serialize/` import)

## Goal

**`openehr/validation`** exposes validators CI and webhooks can call on in-memory artifacts:

1. **Composition vs OPT** — required nodes, cardinality, primitive ranges (v1 subset).
2. **Demographic** — structural RM checks on `PARTY` hierarchies (no OPT).
3. **AQL** — static checks without execution where possible; execution-time errors stay in `openehr/client/query/`.

Clean separation: validators walk **`openehr/rm`** values and **`openehr/template`** trees — never decode JSON/XML.

## Integration with existing stack

| Piece | Location | Role |
|---|---|---|
| RM graph | `openehr/rm/` | Validation input |
| OPT | `openehr/template/` | Constraint source |
| AQL models | `openehr/aql/` | Optional path extraction for lint |
| Query client | `openehr/client/query/` | Maps backend AQL errors (`ErrPathResolution` already in `aql/errors.go`) |
| Serialize | `openehr/serialize/` | **Must not import** |

## v1 validation depth (honest bounds)

Document in REQ-102 so consumers know what v1 does **not** guarantee:

| Check | v1 | Later |
|---|---|---|
| Required attributes present | yes | |
| Cardinality (min/max occurrences) | yes | |
| Primitive value ranges (e.g. magnitude) | partial | full ADL rules |
| Terminology binding value-set | no | needs TERM client |
| Cross-archetype reference integrity | no | needs linker |
| AQL full grammar | no | lint identifiers + FROM clause shape |
| Demographic invariants | structural only | policy rules |

## Out of scope

- Importing `openehr/serialize/` (hard rule — add `go vet` / custom test `TestValidationNoSerializeImport`).
- Replacing backend validation on write (SDK pre-flight only).
- OPT diff / merge tools.

## Phases

### Phase 0 — REQ-102, error model, package layout

**Outcome:** Spec + sentinel errors + interface stubs.

**Tasks:**

1. **`specs/clinical-modeling.md` § REQ-102** — three validator entry points, error aggregation (`ValidationResult` with `Issues []Issue`), severity (error vs warning in v1 = error only).
2. **Sentinels** in `openehr/validation/errors.go`:
   ```go
   var (
       ErrCardinality = errors.New("validation: cardinality")
       ErrRequired    = errors.New("validation: required")
       ErrTypeMismatch = errors.New("validation: type mismatch")
       ErrAQLSyntax   = errors.New("validation: aql syntax")
   )
   ```
3. **Interfaces:**
   ```go
   type CompositionValidator interface {
       Validate(comp *rm.Composition, tpl *template.Template) ValidationResult
   }
   type DemographicValidator interface {
       Validate(party rm.PartyProxy) ValidationResult
   }
   type AQLValidator interface {
       ValidateQuery(q aql.Query, tpl *template.Template) ValidationResult
   }
   ```
4. **Registry** — REQ-102 planned in traceability.

**Definition of done:** `make spec-check`; package compiles; import guard test forbids `serialize`.

### Phase 1 — Composition vs OPT (MVP)

**Outcome:** Catch missing required paths and obvious cardinality violations on fixture OPT.

**Tasks:**

1. **`ValidateComposition(comp, tpl)`** — walk template definition; for each required child, locate node in composition via path mapping (reuse template path walk + composition navigation helper in `internal/validationpath` or unexported funcs in package).
2. **Cardinality** — count occurrences at template path vs `existence` / `occurrences` in OPT (ADL 1.4 attributes).
3. **Type mismatch** — `DV_QUANTITY` where template expects `DV_TEXT`, etc.
4. **Tests** — valid built composition passes; remove required node → `ErrRequired`.
5. **Example** — `cmd/examples/validate-composition/main.go`.

**Definition of done:** `go test ./openehr/validation/...`; REQ-102 partial.

### Phase 2 — Demographic + AQL lint subset

**Outcome:** Second and third validators usable in CI.

**Tasks:**

1. **`ValidateDemographic`** — `PARTY`, `PERSON`, `ORGANISATION` required fields per RM (names, identities list non-empty where BMM marks required).
2. **`ValidateAQL`** — non-empty `q`; brace balance; `FROM` contains archetype id present in template when `tpl != nil`; does **not** replace server-side path resolution (PROBE-021 execute path).
3. **PROBE-024** — composition missing required element → stable issue code in result (sandbox).
4. **REQ-102** → landed when Composition validator + tests + probe stable.

**Definition of done:** All three validators documented in package doc; no `serialize/` import.

## Public API (target)

```go
func ValidateComposition(comp *rm.Composition, tpl *template.Template) ValidationResult
func ValidateDemographic(party rm.PartyProxy) ValidationResult
func ValidateAQL(q aql.Query, tpl *template.Template) ValidationResult

type ValidationResult struct {
    OK     bool
    Issues []Issue // Path, Code, Message
}
```

## Implementation checklist

| Step | Status |
|---|---|
| REQ-102 + import guard test | |
| Composition validator MVP | |
| Demographic + AQL lint | |
| PROBE-024 | |
| `make ci` | |

## Mapping to specs

- [`specs/module-layout.md`](../../specs/module-layout.md) — validation must not import serialize
- [`openehr/validation/doc.go`](../../openehr/validation/doc.go) — package intent
- Proposed: [`specs/clinical-modeling.md`](../../specs/clinical-modeling.md) § REQ-102
- [`specs/conformance.md`](../../specs/conformance.md) — PROBE-021 (execute), PROBE-024 (proposed)
