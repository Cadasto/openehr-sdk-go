# Plan — Generic OPT-driven Composition builder

**Date:** 2026-05-21
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-013, REQ-024, REQ-030–033; proposed **REQ-101** (generic composition builder)
**Probes:** PROBE-023 (proposed — build + canonical JSON round-trip via `canjson`, not serialize import in builder)
**Implementation:** planned
**Depends on:** [`2026-05-21-template-parser.md`](2026-05-21-template-parser.md) Phase 1; [`2026-05-15-canonical-json-serialization.md`](2026-05-15-canonical-json-serialization.md); umbrella [`2026-05-21-phase-2-clinical-building-blocks.md`](2026-05-21-phase-2-clinical-building-blocks.md)
**Defers:** Per-template generated Go structs; FLAT/STRUCTURED ingest (REQ-053); automatic `EVENT` timing population beyond documented defaults

## Goal

Provide **`openehr/composition`**: build a `*rm.Composition` in memory by assigning values at **openEHR paths** against a parsed OPT. The engine is generic — consumers pass `*template.OperationalTemplate` + path + typed values; they do **not** import codegen’d vital-signs structs from this repo.

Persistence remains **`openehr/client/ehr/composition/`** + `canjson` at the application boundary.

## Integration with existing stack

| Piece | Location | Role |
|---|---|---|
| Parsed OPT | `openehr/template/` (`OperationalTemplate`) | Path resolution + RM type hints |
| RM + typereg | `openehr/rm/` | Target `Composition` graph |
| Canonical JSON | `openehr/serialize/canjson/` | **Tests and apps only** — builder does not import `serialize/` |
| EHR client | `openehr/client/ehr/composition/` | `Create` / `Update` with `composition.WithTemplateID` |

## Design principles

1. **Path-first API** — `Set(path, value)` / `SetDVQuantity(path, magnitude, units)` style; generics where the RM target type is known from template metadata.
2. **Lazy graph materialisation** — create `SECTION` / `OBSERVATION` nodes on first `Set` along a path; archetype roots from OPT definition.
3. **Polymorphism** — use `typereg` / concrete RM types per path hint; no `reflect` for clinical assignment (REQ-024).
4. **Template id** — `Builder.TemplateID()` matches OPT; caller passes same id to REST `WithTemplateID`.
5. **UIDs** — generate missing `uid` on nodes where openEHR expects them (document algorithm in REQ-101); do not require caller-supplied UUIDs for every node in v1.

## Out of scope

- Validating against OPT constraints (→ `openehr/validation/`).
- Encoding to JSON/XML inside the package.
- Multi-language composition merges / contributions (Contribution builder is a later extension).

## Phases

### Phase 0 — REQ-101, API contract, fixture composition

**Outcome:** Normative builder rules; failing tests describing desired behaviour.

**Tasks:**

1. **`specs/clinical-modeling.md` § REQ-101** — builder invariants:
   - Requires `*template.OperationalTemplate` at construction.
   - `Build() (*rm.Composition, error)` returns graph or aggregated path errors.
   - Path must exist on template; wrong RM type → typed error.
   - `language` / `territory` / `category` — document required defaults for v1 (e.g. `ISO_639-1::en`, `ISO_3166-1::NL`, `433` event).
2. **REQ.md + traceability** — planned → partial as code lands.
3. **Golden target** — extend template fixture OPT; document expected paths in `openehr/composition/testdata/README.md`.
4. **Test skeleton** — `builder_test.go` with `// REQ-101` cases (skip until Phase 1 if needed).

**Definition of done:** Spec + registry; tests compile; template Phase 1 fixture paths documented.

### Phase 1 — MVP builder (single archetype root)

**Outcome:** Build minimal Composition for one `C_ARCHETYPE_ROOT` (e.g. one `OBSERVATION` cluster).

**Tasks:**

1. **`NewBuilder(t *template.OperationalTemplate) *Builder`**
2. **`Set(path string, v any) error`** — dispatch on template node RM type:
   - `DV_TEXT`, `DV_CODED_TEXT`, `DV_QUANTITY`, `DV_COUNT`, `DV_BOOLEAN`, `DV_DATE_TIME` (string ISO per REQ-046).
   - Nested `CLUSTER` / `ITEM_TREE` via path prefixes.
3. **`Build() (*rm.Composition, error)`** — wire `name`, `archetype_node_id`, `archetype_details`, template id in `Composition` metadata fields per RM.
4. **Unit tests** — assign 2–3 paths → `Build` → assert struct fields (not wire bytes yet).
5. **Example** — `cmd/examples/composition-build/main.go` → stdout canonical JSON via **app** importing `canjson` (example may import both packages; `composition` package must not).

**Definition of done:**

- `go test ./openehr/composition/...` green.
- No `serialize/` import in `openehr/composition/*.go` (enforced by `go test` import guard or `internal/lint` script in CI — optional script in Phase 2).

### Phase 2 — Multi-root, slots, EVENT context

**Outcome:** Templates with multiple archetype roots and `ITEM_TREE` branches used by CDR benchmark fixtures.

**Tasks:**

1. **Multiple `C_ARCHETYPE_ROOT`** — ordered `content` list.
2. **Slot handling** — follow OPT slot `includes` to nested archetype (embedded OPT fragment or reference by id from same file).
3. **`EVENT` context** — default `HISTORY` + `EVENT` shell when path targets data under an observation.
4. **PROBE-023** — build fixture composition → `canjson.Marshal` → unmarshal → key paths stable (sandbox; lives in `testkit/probes/composition/`).
5. **REQ-101** → `landed` in traceability when probe + MVP stable.

**Definition of done:**

- Benchmark seeder (STRAND-01) can adopt builder for at least one template (follow-up PR outside this plan).
- PROBE-023 implemented (Draft → Implemented in conformance table).

## Public API (target)

```go
func New(t *template.OperationalTemplate) *Builder

// Set assigns v at path. v must match the template node's RM type.
func (b *Builder) Set(path string, v any) error

// Build returns the in-memory Composition or a joined validation error.
func (b *Builder) Build() (*rm.Composition, error)

func (b *Builder) TemplateID() string
```

Typed helpers (`SetQuantity`, `SetText`, …) MAY be added when they remove boilerplate without duplicating every DV type.

## Implementation checklist

| Step | Status |
|---|---|
| REQ-101 spec + registry | |
| Phase 1 builder + tests | |
| Phase 2 multi-root + PROBE-023 | |
| Example without serialize import in library | |
| `make ci` | |

## Mapping to specs

- [`specs/module-layout.md`](../../specs/module-layout.md) — composition vs client split
- [`specs/rm-modeling.md`](../../specs/rm-modeling.md) — concrete types, typereg
- Proposed: [`specs/clinical-modeling.md`](../../specs/clinical-modeling.md) § REQ-101
