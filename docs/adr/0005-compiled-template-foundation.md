# ADR 0005 — Compiled OPT foundation (rminfo + internal templatecompile)

- **Status:** Accepted, 2026-05-22.
- **Supersedes:** —
- **Superseded by:** —
- **Tracks:** [`docs/plans/2026-05-22-template-req100-followups.md`](../plans/2026-05-22-template-req100-followups.md) Phases 4 + 4-bis.

## Context

REQ-100 delivered a wire-faithful OPT parser (`openehr/template/`). The composition builder (REQ-101) and template validator (REQ-102) need a walker-friendly view: stable AQL paths, implicit RM-mandatory attributes the OPT omits, scoped terminology, and O(1) path lookup. Two new pieces were added on `main` after PR #10/#11:

1. **`openehr/rm/rminfo/`** — BMM-derived structural metadata (required attributes, attribute RM types, containers).
2. **`internal/templatecompile/`** — `Compile(*template.OperationalTemplate)` producing a `Compiled` tree.

The public export shape (`template.Compile` / `template.Compiled`) is deferred until REQ-101/102 confirm the API.

## Decision

### C1 — `rminfo` is a generated, stdlib-only lookup table

- Emit `lookup_gen.go` from `internal/bmmgen` alongside RM codegen (effective property sets: own + inherited, ancestor-first).
- Expose a small `Lookup` interface + `Default` accessor; no runtime BMM parse in consumers.
- `RequiredAttributes` order follows BMM declaration order for implicit injection.

### C2 — `templatecompile` stays under `internal/` until REQ-101 lands

- Composition and validation import via Go's `internal/` rule (same module).
- Wire parse remains in `openehr/template/`; compile is a pure transform with no I/O.

### C3 — `Compiled.NodeAt` is exact-match on precomputed AQL paths

- Indexes store **fully qualified** paths (e.g. `/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]`).
- This differs from wire `OperationalTemplate.NodeAt`, which walks the tree and applies lenient first-child rules for predicate-less segments (e.g. `/content` on multi-child attributes).
- Callers must use the path strings produced at compile time (via `CompiledNode.AQLPath()`), not assume parity with wire path resolution.

### C4 — Implicit injection is required-only

- Only BMM-mandatory attributes missing from the OPT are injected (`composer`, `language`, `territory` on `COMPOSITION`, etc.).
- `Options.SkipImplicitAttributes` disables injection for round-trip / explicit-only shapes.

### C5 — Per-archetype-root term scope

- `term_definitions` attach to `*ArchetypeRoot` nodes; `CompiledNode.Term(code)` walks parents to the nearest root.
- Term bindings flatten to `Compiled.TermBindings()` (records carry their own terminology + locator).

### C6 — Duplicate AQL paths fail compile

- `byPath` registration rejects colliding paths (silent overwrite would return the wrong node from `NodeAt`).
- Sibling disambiguation: multiple-cardinality children without at-code use slot `Includes` patterns; otherwise a 1-based `[@N]` compile suffix (OPT document order).

## Consequences

- **Positive:** REQ-101/102 can share one compiled tree; rminfo is reusable without importing `templatecompile`.
- **Positive:** Codegen drift check recurses into `openehr/rm/rminfo/` (idempotent emit).
- **Negative:** Two path resolution semantics (wire vs compiled) — document in package godoc and REQ-100 follow-up material.
- **Negative:** Public API promotion requires a follow-up ADR amendment or supersession when `template.Compile` is exported.
