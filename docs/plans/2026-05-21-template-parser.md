# Plan — ADL 1.4 operational template (OPT) parser and path utilities

**Date:** 2026-05-21
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-013, REQ-014; proposed **REQ-100** (ADL 1.4 OPT parse + paths) — normative stub in Phase 0
**Probes:** PROBE-022 (proposed — OPT path resolution); ratification deferred (REQ-081)
**Implementation:** planned
**Depends on:** [`2026-05-15-bmm-codegen.md`](2026-05-15-bmm-codegen.md); [`2026-05-21-phase-2-clinical-building-blocks.md`](2026-05-21-phase-2-clinical-building-blocks.md)
**Defers:** ADL2 / AOM 2.4 OPT; **OET** (`.oet` authoring templates); package-deployment to CDR (use `openehr/client/definition/`); FLAT path keys (REQ-053)

## Goal

Parse **ADL 1.4 operational templates (OPT)** — XML `OPERATIONAL_TEMPLATE` artifacts, typically filename suffix `.opt` — into an in-memory `template.OperationalTemplate` with stable **openEHR path** utilities (`/content[...]`, archetype node ids, RM attribute segments). Tooling and the composition builder consume this package **without** HTTP.

In openEHR terminology, “template” without qualification often means the authoring **OET**; in this SDK v1 **“template” in package and REST names means operational template (OPT)** unless stated otherwise.

**Distinct from** `openehr/client/definition/`: the REST client uploads OPT XML to a CDR; this package interprets OPT bytes locally (CI, editors, offline validation).

## Integration with existing stack

| Piece | Location | Role |
|---|---|---|
| AOM 1.4 generated types | `openehr/aom/aom14/` | Archetype constraints embedded in OPT |
| RM types | `openehr/rm/` | RM class names on path segments |
| Definition upload | `openehr/client/definition/` | Optional: fetch OPT from deployment; parse via `template.ParseOPT` |
| BMM loader | `openehr/bmm/` | Not required for v1 parse — OPT is self-contained XML |

## v1 scope (ADL 1.4 OPT only)

- **Input:** OPT XML only — root element `OPERATIONAL_TEMPLATE`, wire `application/xml` (same as `definition.FormatADL14`). Callers supply `.opt` bytes or streams; `ParseFile` **MUST** reject non-`.opt` paths in v1.
- **Output:** `OperationalTemplate` with template id, concept, definition tree (`C_COMPLEX_OBJECT` / `C_ARCHETYPE_ROOT` / slots), terminology bindings where present.
- **Path API:** parse path strings → `Path` struct; walk definition tree; resolve slot → archetype id; **no** full Archie linker semantics in v1.

## Out of scope

- **OET** (`.oet`) — authoring/design-time templates; no parse, no OET→OPT compile in v1.
- ADL2 operational templates.
- Runtime template registry inside the SDK (CDR owns registry).
- Terminology expansion / external terminology services.
- Archetype slot validation against remote archetype repository (v1 uses OPT-embedded constraints only).

## Naming conventions (package `openehr/template/`)

| Use | Name |
|---|---|
| Go package import path | `openehr/template` (unchanged — aligns with REST “template” resource) |
| Parsed artifact type | `OperationalTemplate` (not `Template` — avoids OET ambiguity) |
| Parse entrypoints | `ParseOPT`, `ParseFile` (`.opt` only) |
| REQ / spec title | “ADL 1.4 operational template (OPT) parse and paths” |

## Phases

### Phase 0 — Normative text, fixtures, package skeleton

**Outcome:** REQ registered; golden `.opt` files vendored; API sketched in tests.

**Tasks:**

1. **Add canonical spec** — new [`specs/clinical-modeling.md`](../../specs/clinical-modeling.md) section **REQ-100 — ADL 1.4 operational template (OPT) parse and paths**. Cover:
   - v1 input: OPT XML / `.opt` only; OET explicitly excluded.
   - `OperationalTemplate` identity fields (`TemplateID`, `Concept`, `UID`).
   - Path syntax subset the SDK guarantees (document exclusions: e.g. no predicates beyond `[at0001]` style in v1).
   - Error taxonomy: `ErrInvalidOPT`, `ErrNotOPTFile`, `ErrPathSyntax`, `ErrPathNotFound`.
2. **Registry** — row in [`specs/REQ.md`](../../specs/REQ.md) + [`specs/traceability.yaml`](../../specs/traceability.yaml) (`implementation: planned`).
3. **Fixtures** — `openehr/template/testdata/*.opt`:
   - At least one small CKM-style OPT (e.g. vitals fragment) + minimal hand-crafted OPT for unit tests.
   - Provenance in `testdata/README.md` (source, license).
4. **`openehr/template/doc.go`** — OPT-only scope, `OperationalTemplate` naming, relationship to `client/definition`.
5. **API sketch** (compile-only tests or `*_test.go` with `// REQ-100`):
   ```go
   func ParseOPT(r io.Reader) (*OperationalTemplate, error)
   func ParseFile(path string) (*OperationalTemplate, error) // .opt suffix required
   type Path struct { /* segments */ }
   func (t *OperationalTemplate) ParsePath(path string) (Path, error)
   func (t *OperationalTemplate) NodeAt(p Path) (Node, error)
   ```

**Definition of done:**

- Spec + REQ-100 row exist; `make spec-check` passes.
- Fixtures committed; `go test ./openehr/template/...` compiles (may skip unimplemented with `t.Skip` only until Phase 1 starts — prefer no skip after Phase 1).

### Phase 1 — OPT XML parse (MVP)

**Outcome:** `ParseOPT` loads real `.opt` files from `testdata/`; definition tree walk for archetype roots and attributes.

**Tasks:**

1. **XML decoder** — `encoding/xml` with explicit structs for OPT `template` / `definition` / `attributes` / `children` (no generic `map[string]any` at leaves).
2. **`OperationalTemplate` model** — unexported internals OK; exported fields for id, concept, root `C_ARCHETYPE_ROOT`.
3. **`Node` interface** — closed set: `ArchetypeRoot`, `ComplexObject`, `Attribute`, `Slot` (maps to AOM constraint shapes where possible; reuse `aom14` types for constraint payloads when stable).
4. **Tests** — round-trip identity: template id + root archetype id match golden; walk known paths from fixture README.
5. **Example** — `cmd/examples/opt-parse/main.go` (file path → print template id + root path).

**Definition of done:**

- `make test` green for `./openehr/template/...`.
- `traceability.yaml` lists package + tests; REQ-100 `implementation: partial`.
- No imports from `transport/`, `auth/`, `openehr/client/*`.

### Phase 2 — Path utilities

**Outcome:** `ParsePath`, `NodeAt`, slot resolution on fixture OPTs.

**Tasks:**

1. **Path parser** — openEHR path grammar subset per REQ-100; reject unsupported constructs with `ErrPathSyntax`.
2. **`NodeAt`** — resolve through `items` / `attributes` / archetype roots; return typed `Node` + RM type hint string for composition builder.
3. **PROBE-022** (draft) — sandbox probe: given fixture OPT + path list, `NodeAt` matches expected node ids (add under `testkit/probes/template/` when stable).

**Definition of done:**

- Composition plan Phase 1 can import `template` and resolve paths on fixture OPT.
- REQ-100 `implementation: landed` when probe + docs agree.

## Public API (target)

```go
// ParseOPT reads one ADL 1.4 operational template (OPERATIONAL_TEMPLATE XML).
func ParseOPT(r io.Reader) (*OperationalTemplate, error)

// ParseFile reads a .opt file from disk.
func ParseFile(path string) (*OperationalTemplate, error)

// TemplateID returns the operational template identifier string.
func (t *OperationalTemplate) TemplateID() string

// ParsePath parses an openEHR path against this OPT's definition tree.
func (t *OperationalTemplate) ParsePath(path string) (Path, error)

// NodeAt resolves a parsed path to a definition node.
func (t *OperationalTemplate) NodeAt(p Path) (Node, error)
```

Functional options only if needed (e.g. `WithStrictPaths()`); default strict.

## Implementation checklist

| Step | Status |
|---|---|
| REQ-100 in `specs/clinical-modeling.md` + REQ.md + traceability | |
| Fixtures + README (`.opt` only) | |
| `ParseOPT` + tree walk | |
| Path parse + `NodeAt` | |
| `cmd/examples/opt-parse` | |
| `make spec-check` + `make ci` | |

## Mapping to specs

- [`specs/module-layout.md`](../../specs/module-layout.md) — `openehr/template/` row
- [`specs/scope.md`](../../specs/scope.md) — OPT parse in v1 scope
- [`specs/glossary.md`](../../specs/glossary.md) — Operational Template (OPT)
- [`specs/use-cases.md`](../../specs/use-cases.md) — building-block: `openehr/template/` alone
- Proposed: [`specs/clinical-modeling.md`](../../specs/clinical-modeling.md) § REQ-100
