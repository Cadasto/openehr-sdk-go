# Plan — OPT/OET template parser and path utilities

**Date:** 2026-05-21
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-013, REQ-014; proposed **REQ-100** (OPT/OET parse + paths) — normative stub in Phase 0
**Probes:** PROBE-022 (proposed — OPT round-trip / path resolution); ratification deferred (REQ-081)
**Implementation:** planned
**Depends on:** [`2026-05-15-bmm-codegen.md`](2026-05-15-bmm-codegen.md); [`2026-05-21-phase-2-clinical-building-blocks.md`](2026-05-21-phase-2-clinical-building-blocks.md)
**Defers:** ADL2 / AOM 2.4 OPT; OET authoring helpers beyond parse; package-deployment to CDR (use `openehr/client/definition/`); FLAT path keys (REQ-053)

## Goal

Parse **ADL 1.4 Operational Templates (OPT)** and **OET** sources into an in-memory `template.Template` model with stable **openEHR path** utilities (`/content[...]`, archetype node ids, RM attribute segments). Tooling and the composition builder consume this package **without** HTTP.

**Distinct from** `openehr/client/definition/`: the REST client uploads OPT XML to a CDR; this package interprets OPT bytes locally (CI, editors, offline validation).

## Integration with existing stack

| Piece | Location | Role |
|---|---|---|
| AOM 1.4 generated types | `openehr/aom/aom14/` | Archetype constraints embedded in OPT |
| RM types | `openehr/rm/` | RM class names on path segments |
| Definition upload | `openehr/client/definition/` | Optional: fetch OPT from deployment; parse via `template.Parse` |
| BMM loader | `openehr/bmm/` | Not required for v1 parse — OPT is self-contained XML |

## v1 scope (ADL 1.4 only)

- Input: OPT XML (`application/xml` on wire — same as `definition.FormatADL14`).
- Output: `Template` with template id, concept, definition tree (`C_COMPLEX_OBJECT` / `C_ARCHETYPE_ROOT` / slots), terminology bindings where present.
- Path API: parse path strings → `Path` struct; walk definition tree; resolve slot → archetype id; **no** full Archie linker semantics in v1.
- OET: parse enough to recover template id + included archetype refs for CI; full OET→OPT compile is **out of scope** (assume OPT is the deployment artifact).

## Out of scope

- ADL2 operational templates.
- Runtime template registry inside the SDK (CDR owns registry).
- Terminology expansion / external terminology services.
- Archetype slot validation against remote archetype repository (v1 uses OPT-embedded constraints only).

## Phases

### Phase 0 — Normative text, fixtures, package skeleton

**Outcome:** REQ registered; golden OPTs vendored; API sketched in tests.

**Tasks:**

1. **Add canonical spec** — new [`specs/clinical-modeling.md`](../../specs/clinical-modeling.md) section **REQ-100 — OPT/OET parse and paths** (or extend [`specs/wire.md`](../../specs/wire.md) `#optoet-handling` if scope link is preferred). Cover:
   - ADL 1.4 OPT XML as v1 input.
   - `template.Template` identity fields (`TemplateID`, `Concept`, `UID`).
   - Path syntax subset the SDK guarantees (document exclusions: e.g. no predicates beyond `[at0001]` style in v1).
   - Error taxonomy: `ErrInvalidOPT`, `ErrPathSyntax`, `ErrPathNotFound`.
2. **Registry** — row in [`specs/REQ.md`](../../specs/REQ.md) + [`specs/traceability.yaml`](../../specs/traceability.yaml) (`implementation: planned`).
3. **Fixtures** — `openehr/template/testdata/`:
   - At least one small CKM-style OPT (e.g. vitals fragment) + minimal hand-crafted OPT for unit tests.
   - Provenance in `testdata/README.md` (source, license).
4. **`openehr/template/doc.go`** — building-block import path, ADL 1.4 pin, relationship to `client/definition`.
5. **API sketch** (compile-only tests or `*_test.go` with `// REQ-100`):
   ```go
   func ParseOPT(r io.Reader) (*Template, error)
   func ParseOET(r io.Reader) (*OET, error) // metadata + refs only in v1
   type Path struct { /* segments */ }
   func ParsePath(s string) (Path, error)
   func (t *Template) NodeAt(p Path) (Node, error)
   ```

**Definition of done:**

- Spec + REQ-100 row exist; `make spec-check` passes.
- Fixtures committed; `go test ./openehr/template/...` compiles (may skip unimplemented with `t.Skip` only until Phase 1 starts — prefer no skip after Phase 1).

### Phase 1 — OPT XML parse (MVP)

**Outcome:** `ParseOPT` loads real OPTs from `testdata/`; definition tree walk for archetype roots and attributes.

**Tasks:**

1. **XML decoder** — `encoding/xml` with explicit structs for OPT `template` / `definition` / `attributes` / `children` (no generic `map[string]any` at leaves).
2. **`Template` model** — unexported internals OK; exported fields for id, concept, root `C_ARCHETYPE_ROOT`.
3. **`Node` interface** — closed set: `ArchetypeRoot`, `ComplexObject`, `Attribute`, `Slot` (maps to AOM constraint shapes where possible; reuse `aom14` types for constraint payloads when stable).
4. **Tests** — round-trip identity: template id + root archetype id match golden; walk known paths from fixture README.
5. **Example** — `cmd/examples/template-parse/main.go` (stdin or file path → print template id + root path).

**Definition of done:**

- `make test` green for `./openehr/template/...`.
- `traceability.yaml` lists package + tests; REQ-100 `implementation: partial`.
- No imports from `transport/`, `auth/`, `openehr/client/*`.

### Phase 2 — Path utilities and OET stub

**Outcome:** `ParsePath`, `NodeAt`, slot resolution; OET metadata parse.

**Tasks:**

1. **Path parser** — openEHR path grammar subset per REQ-100; reject unsupported constructs with `ErrPathSyntax`.
2. **`NodeAt`** — resolve through `items` / `attributes` / archetype roots; return typed `Node` + RM type hint string for composition builder.
3. **`ParseOET`** — template id, archetype refs, no compilation.
4. **PROBE-022** (draft) — sandbox probe: given fixture OPT + path list, `NodeAt` matches expected node ids (add under `testkit/probes/template/` when stable).

**Definition of done:**

- Composition plan Phase 1 can import `template` and resolve paths on fixture OPT.
- REQ-100 `implementation: landed` when probe + docs agree.

## Public API (target)

```go
// ParseOPT reads one ADL 1.4 operational template (XML).
func ParseOPT(r io.Reader) (*Template, error)

// TemplateID returns the template identifier string.
func (t *Template) TemplateID() string

// ParsePath parses an openEHR path against this template's definition tree.
func (t *Template) ParsePath(path string) (Path, error)

// NodeAt resolves a parsed path to a definition node.
func (t *Template) NodeAt(p Path) (Node, error)
```

Functional options only if needed (e.g. `WithStrictPaths()`); default strict.

## Implementation checklist

| Step | Status |
|---|---|
| REQ-100 in `specs/clinical-modeling.md` + REQ.md + traceability | |
| Fixtures + README | |
| `ParseOPT` + tree walk | |
| Path parse + `NodeAt` | |
| `cmd/examples/template-parse` | |
| `make spec-check` + `make ci` | |

## Mapping to specs

- [`specs/module-layout.md`](../../specs/module-layout.md) — `openehr/template/` row
- [`specs/scope.md`](../../specs/scope.md) — OPT/OET in v1 scope
- [`specs/use-cases.md`](../../specs/use-cases.md) — building-block: template alone
- Proposed: [`specs/clinical-modeling.md`](../../specs/clinical-modeling.md) § REQ-100
