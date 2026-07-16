# Plan — OPT author validator + CLI

**Date:** 2026-07-16
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** **REQ-114** (OPT author validator) — proposed; canonical home to be authored at [clinical-modeling.md § REQ-114](../specifications/clinical-modeling.md#req-114--opt-author-validator) in Phase 0. Numbered in the clinical-modeling headroom (110–119), next to the REQ-109/110 tooling.
**Builds on:** landed [REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) (OPT parse), [REQ-104](../specifications/clinical-modeling.md#req-104--slot-assertion-grammar) (slot assertions), [REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) (Web Template export), [REQ-108](../specifications/clinical-modeling.md#req-108--untrusted-document-bounds) (document bounds); complementary to [REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation) (composition validation)
**Probes:** **PROBE-085** (OPT author validator corpus)
**Implementation:** planned
**Depends on:** landed `openehr/template/`, `openehr/templatecompile/`, `openehr/template/webtemplate/`
**Defers:** editor/language-server integration; ADL 2 / OET; full archetype validator (CKM server); automatic OPT auto-fix

## Goal

Validate **authored** OPT 1.4 files before CDR upload — complementary to `ParseOPT` / `ParseOPTStrict` (structural parse) and composition validation (REQ-102). Surfaces **actionable issue codes** for template authors and CI, including FLAT-path-impact analysis via Web Template export. It follows the lint/issue model already landed for AQL (REQ-109); motivation is the OPT-validator gap identified in the peer-SDK ecosystem fit-gap review.

## Architecture

```
.opt file (untrusted bytes)
        │
        ▼
openehr/template/validate/     ← new subpackage (stdlib + existing template deps only)
        │
        ├─ wellformedness  (XML parse, ADL 1.4 namespace, template_id present)
        ├─ semantic        (term definitions, terminology bindings, RM types known to rminfo)
        ├─ structural      (draft lifecycle, slot includes/excludes parseable, cardinalities)
        └─ flat_impact     (Compile → webtemplate.Build → path collision / rename warnings)
        │
        ▼
validate.Result { Issues[] }   ← mirror the lint/issue pattern from REQ-109 (AQL static lint)
        │
        ▼
cmd/examples/validate-opt/     ← CLI (-format json, -strict)
```

- **Reuse** `template.ParseFile` / `ParseOPTStrict` — the validator wraps parse and adds author rules, does not fork the parser.
- **Issue model:** `Code`, `Severity` (error|warning|info), `Category`, optional `Path`, `Suggestion` — the shape REQ-109 established.
- **Bounds:** REQ-108 document size / node count limits enforced before deep walks.

## Definition of Ready

Implementation (Phase 1+) may start once **Phase 0 has landed REQ-114**:

- `Covers:` names the REQ this plan implements (REQ-114) and the landed REQs it builds on (REQ-100/104/106/108) + the complementary REQ-102.
- Canonical normative prose for REQ-114 exists — a `clinical-modeling.md § REQ-114` section + a `REQ.md` registry row — authored via `sdd-specify` (Phase 0). Until then this DoR item is **pending**, not satisfied.
- The issue-category taxonomy and the stable `opt.` code catalogue are defined **once**, in REQ-114 (not duplicated in this plan).
- PROBE-085 fixture list chosen from `testkit/cassettes/templates/`.
- Each phase names its verification command.

## Definition of Done

- `openehr/template/validate/` + CLI example landed with `// REQ-114` citations.
- PROBE-085 green.
- `traceability.yaml` + the REQ.md **Impl.** column (REQ-114 `planned → landed`), `roadmap.md`, `docs/examples.md` updated.
- `make spec-check` + `make ci` green; plan archived (or **Status:** complete).

## Implementation checklist

| Step | Status |
|---|---|
| REQ-114 § + registry row (`clinical-modeling.md`, `REQ.md`) | |
| PROBE-085 defined in `conformance.md` (Draft) | |
| Validator package + CLI code | |
| Tests with `// REQ-114` / `// PROBE-085` comments | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 0 — Spec & issue catalogue (the specify gate)

Author the canonical contract first, so Phases 1–3 cite an existing REQ. The taxonomy and code list below are a **draft seed to author into REQ-114** — the canonical home is `clinical-modeling.md`, not this plan.

**Tasks:**

1. Author **REQ-114** in `docs/specifications/clinical-modeling.md` (via `sdd-specify`), including the issue-category taxonomy:

   | Category | Severity | Examples |
   |---|---|---|
   | wellformedness | error | invalid XML, wrong namespace, missing template_id |
   | semantic | error | unknown RM type, missing term for at-code |
   | structural | warning | draft lifecycle_state, empty slot includes |
   | flat_impact | info/warning | Web Template path collision, digit-prefixed id sanitisation |

   and the **stable `opt.` code catalogue** (durable identifiers — canonical in the spec):
   - `opt.xml_malformed`, `opt.missing_template_id`, `opt.unknown_rm_type`
   - `opt.missing_term_definition`, `opt.unparsed_slot_assertion`
   - `opt.draft_lifecycle`, `opt.flat_path_collision`
   - the CLI exit-code / `-strict` contract (error → exit 1; warning-in-strict → exit 2) is normative and belongs here, not only in Phase 2.
2. Add the `REQ.md` registry row (**Impl.:** `planned`; spec section **Status:** `Draft`).
3. Define PROBE-085 in `conformance.md` (status Draft) + `traceability.yaml` row.

**Definition of done:** `make spec-check` passes with the new rows.

### Phase 1 — Validator library

**Tasks:**

1. Create `openehr/template/validate/`:
   - `issue.go`, `result.go` — the `Code`/`Severity`/`Category` model defined by REQ-114.
   - `validate.go` — `ValidateFile(path string, opts ...Option) (Result, error)`
   - `validate_string.go` — `ValidateString(adl string, opts ...)`
   - `wellformed.go` — XML + template identity
   - `semantic.go` — walk `template.OperationalTemplate` + `rminfo.Default.KnownRMTypes()`
   - `structural.go` — lifecycle, slot-assertion reuse from the REQ-104 parser
   - `flat_impact.go` — `templatecompile.Compile` + `webtemplate.Build`, detect duplicate flat ids
2. Options: `WithStrict(bool)` (warnings → errors), `WithMaxBytes(int)` (REQ-108).
3. Tests `validate_test.go`:
   - `vital_signs.opt` → valid (0 errors).
   - Mutated copies in testdata: missing template_id, bad XML, unknown type string.
   - Golden issue codes for each negative case.

**Files:**

- Create: `openehr/template/validate/*`
- Testdata: `openehr/template/validate/testdata/*.opt` (minimal fragments)

**Definition of done:** `go test ./openehr/template/validate/...` green; no import of `transport/` or `client/`.

### Phase 2 — CLI

**Tasks:**

1. `cmd/examples/validate-opt/main.go`:
   - Args: one or more `.opt` paths.
   - Flags: `-format text|json`, `-strict`, `-show-flat-paths`.
   - Exit codes per the REQ-114 contract (error → 1; warning-in-strict → 2).
2. Document in `docs/examples.md` (CI JSON example for GitHub Actions).

**Definition of done:** `go run ./cmd/examples/validate-opt …` runs on the cassette templates; `make ci` green.

### Phase 3 — PROBE-085 & integration

**Tasks:**

1. `testkit/probes/template/probe_085_opt_author_validator.go`:
   - Run the validator on the full cassette template set; expect 0 errors on known-good OPTs.
   - One deliberately broken fixture must emit `opt.missing_template_id`.
2. Optional (deferred — not v1 unless trivial): hook `template.ParseFileStrict` to call the validator under an `OPT_VALIDATE=1` env toggle. **If adopted, that toggle is normative behaviour and must be specified in REQ-114**, not left as a plan-only task.
3. Update `traceability.yaml`, flip the REQ.md **Impl.** column for REQ-114 to `landed`, archive plan.

**Definition of done:** PROBE-085 in `make test`; REQ-114 **Impl.** = `landed`; `make spec-check` green.

## Mapping to specs

- [clinical-modeling.md § REQ-114](../specifications/clinical-modeling.md#req-114--opt-author-validator) — the requirement this plan implements (registry row: [REQ.md](../specifications/REQ.md))
- [clinical-modeling.md § REQ-109](../specifications/clinical-modeling.md#req-109--aql-static-lint) — the lint/issue-model precedent this mirrors
- [clinical-modeling.md § REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) — parse foundation
- [clinical-modeling.md § REQ-104](../specifications/clinical-modeling.md#req-104--slot-assertion-grammar) — slot-assertion reuse
- [clinical-modeling.md § REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) — flat-impact analysis source
- [clinical-modeling.md § REQ-108](../specifications/clinical-modeling.md#req-108--untrusted-document-bounds) — size limits
- [clinical-modeling.md § REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation) — complementary (data-side) validation

## References

- A peer Python openEHR SDK's OPT validator + issue-code catalogue — the pattern this adapts; see the peer-SDK ecosystem fit-gap review.
- Cadasto: `openehr/template/`, `cmd/examples/opt-parse/`, REQ-109 AQL lint (issue model).
