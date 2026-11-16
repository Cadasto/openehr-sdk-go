---
kind: adr
id: ADR-0005
title: "Compiled OPT foundation (rminfo + internal templatecompile)"
status: accepted
date: 2026-05-22
---

# ADR 0005 — Compiled OPT foundation (rminfo + internal templatecompile)

- **Status:** Accepted, 2026-05-22.
- **Amended by:** [ADR 0010](0010-public-compiled-template-bridge.md) (C2: the public bridge is `openehr/templatecompile`) and [ADR 0035](0035-compile-step-reads-operational-template-model.md) (`Compile` takes the generated `*aom14.OperationalTemplate`, and the OPT parser and its tree are gone).
- **Related:** the OPT walker at `internal/templatecompile/walk/` and the primitive constraints of REQ-103 at `openehr/template/constraints/` build on this foundation without changing C1–C6.

## Context

REQ-100 delivered a wire-faithful OPT parser (`openehr/template/`). The composition builder (REQ-101) and template validator (REQ-102) need a walker-friendly view: stable AQL paths, implicit RM-mandatory attributes the OPT omits, scoped terminology, and O(1) path lookup. Two new pieces were added on `main` after PR #10/#11:

1. **`openehr/rm/rminfo/`** — BMM-derived structural metadata (required attributes, attribute RM types, containers).
2. **`internal/templatecompile/`** — `Compile(*template.OperationalTemplate)` producing a `Compiled` tree. Since [ADR 0035](0035-compile-step-reads-operational-template-model.md) it takes `*aom14.OperationalTemplate`.

The public export shape (`template.Compile` / `template.Compiled`) is deferred until REQ-101/102 confirm the API.

## Decision

### C1 — `rminfo` is a generated, stdlib-only lookup table

- Emit `lookup_gen.go` from `internal/bmmgen` alongside RM codegen (effective property sets: own + inherited, ancestor-first).
- Expose a small `Lookup` interface + `Default` accessor; no runtime BMM parse in consumers.
- `RequiredAttributes` order follows BMM declaration order for implicit injection.

**Extended by [REQ-048](../specifications/reference-model.md#req-048--rm-meta-model-introspection-surface)**, without changing this decision's shape: the same generated, stdlib-only, no-runtime-BMM table also carries the RM **class graph** (abstractness, immediate parents, and the attribute declaration site the fold erases), and its emitted class set widens from the attribute-BEARING classes to every class the RM target types — a descendant expansion rooted at `DATA_VALUE` or `PATHABLE` was unanswerable while those classes were absent. `KnownRMTypes()` therefore reports 139 classes rather than 125. The compiled-in / no-runtime-parse rule is unchanged and is now normative in that requirement rather than only here.

### C2 — `templatecompile` stays under `internal/` until REQ-101 lands

- Composition and validation import via Go's `internal/` rule (same module).
- Compile is a pure transform with no I/O. The document is read by `aom14.ReadOperationalTemplate` since [ADR 0035](0035-compile-step-reads-operational-template-model.md), not by an `openehr/template` parser.

### C3 — `Compiled.NodeAt` is exact-match on precomputed AQL paths

- Indexes store **fully qualified** paths (e.g. `/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]`).
- This differs from wire `OperationalTemplate.NodeAt`, which walks the tree and applies lenient first-child rules for predicate-less segments (e.g. `/content` on multi-child attributes).
- Callers must use the path strings produced at compile time (via `CompiledNode.AQLPath()`), not assume parity with wire path resolution.
- *Correction:* the wire tree and its `NodeAt` went with the parser ([ADR 0035](0035-compile-step-reads-operational-template-model.md)); the tree walk that takes the first child of a segment without a predicate is now `Compiled.Resolve`.

### C4 — Implicit injection is required-only

- Only BMM-mandatory attributes missing from the OPT are injected (`composer`, `language`, `territory` on `COMPOSITION`, etc.).
- `Options.SkipImplicitAttributes` disables injection for round-trip / explicit-only shapes.

### C5 — Per-archetype-root term scope

- `term_definitions` attach to `*ArchetypeRoot` nodes; `CompiledNode.Term(code)` walks parents to the nearest root.
- Term bindings flatten to `Compiled.TermBindings()` (records carry their own terminology + locator).

### C6 — Duplicate AQL paths fail compile

- `byPath` registration rejects colliding paths (silent overwrite would return the wrong node from `NodeAt`).
- Sibling disambiguation: multiple-cardinality children without at-code use slot `Includes` patterns; otherwise a 1-based `[@N]` compile suffix (OPT document order).
- *Correction:* two kinds of shared path compile, alternatives under one single attribute and siblings with the same node id, with their subtrees; `NodeAt` returns the first walked. Any other collision fails.

## Consequences

- **Positive:** REQ-101/102 can share one compiled tree; rminfo is reusable without importing `templatecompile`.
- **Positive:** Codegen drift check recurses into `openehr/rm/rminfo/` (idempotent emit).
- **Negative (REQ-048 extension):** the emitted class set widens 125 → 139, so a caller that counted on 125 `KnownRMTypes()` entries breaks; and two questions stay open against the widened surface — [STRAND-12](../specifications/research-strands.md#strand-12--bmm-interface-classes-carry-no-is_abstract-flag) (BMM-interface abstractness) and [STRAND-13](../specifications/research-strands.md#strand-13--properties-inherited-from-a-primitive-mapped-ancestor-are-dropped) (a property inherited from a primitive-mapped ancestor is dropped).
- **Negative:** Two path resolution semantics, exact lookup (`NodeAt`) and tree walk (`Resolve`), both on the compiled template since [ADR 0035](0035-compile-step-reads-operational-template-model.md); the package doc states the difference.
- **Negative:** Public API promotion requires a follow-up ADR amendment or supersession when `template.Compile` is exported.
