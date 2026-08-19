# ADR 0005 — Compiled OPT foundation (rminfo + internal templatecompile)

- **Status:** Accepted, 2026-05-22.
- **Supersedes:** —
- **Superseded by:** —
- **Tracks:** [`docs/plans/archive/2026-05-22-template-req100-followups.md`](../plans/archive/2026-05-22-template-req100-followups.md) Phases 4 + 4-bis. Phases 5 (walker pattern at `internal/templatecompile/walk/`) and 6 (REQ-103 primitive constraints at `openehr/template/constraints/`) build on this foundation without changing C1–C6.

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

**Extended by [REQ-048](../specifications/bmm-conformance.md#req-048--rm-meta-model-introspection-surface)** (2026-08-19), without changing this decision's shape: the same generated, stdlib-only, no-runtime-BMM table also carries the RM **class graph** (abstractness, immediate parents, and the attribute declaration site the fold erases), and its emitted class set widens from the attribute-BEARING classes to every class the RM target types — a descendant expansion rooted at `DATA_VALUE` or `PATHABLE` was unanswerable while those classes were absent. `KnownRMTypes()` therefore reports 139 classes rather than 125. The compiled-in / no-runtime-parse rule is unchanged and is now normative in that requirement rather than only here.

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
- **Negative (REQ-048 extension, 2026-08-19):** the emitted class set widens 125 → 139, so a caller that counted on 125 `KnownRMTypes()` entries breaks; and two questions stay open against the widened surface — [STRAND-12](../specifications/research-strands.md#strand-12--bmm-interface-classes-carry-no-is_abstract-flag) (BMM-interface abstractness) and [STRAND-13](../specifications/research-strands.md#strand-13--properties-inherited-from-a-primitive-mapped-ancestor-are-dropped) (a property inherited from a primitive-mapped ancestor is dropped).
- **Negative:** Two path resolution semantics (wire vs compiled) — document in package godoc and REQ-100 follow-up material.
- **Negative:** Public API promotion requires a follow-up ADR amendment or supersession when `template.Compile` is exported.
