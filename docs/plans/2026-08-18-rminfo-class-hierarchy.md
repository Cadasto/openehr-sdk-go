# Plan — RM class hierarchy and declaration-site attribute lookup (`rminfo`)

**Date:** 2026-08-18
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** **REQ-124** (RM model introspection — hierarchy) — proposed; canonical home decided in Phase 0: `rm-functions.md` keeps the 120–129 band in one file beside REQ-120–123 (its header scopes itself to runtime behaviour, so admitting structural introspection is a deliberate widening to state); `bmm-conformance.md` fits the content but would split the band across files — if that home wins, allocate in the free BMM headroom (048–049) instead of 124, so no band straddles two documents. Final banding is settled in Phase 0 per the [numbering policy](../specifications/REQ.md#numbering-policy); the id here is a proposal, not an allocation.
**Verifies / builds on:** landed `openehr/rm/rminfo` (BMM-derived structural lookup, [ADR 0005](../adr/0005-compiled-template-foundation.md)), [REQ-041](../specifications/bmm-conformance.md#req-041--pinned-bmm-sources) (pinned BMM sources), [REQ-042](../specifications/bmm-conformance.md#req-042--generated-code-drift-detected) (generated code, drift-detected)
**Probes:** **PROBE-094** (proposed; allocated in Phase 0) — generated hierarchy tables vs the pinned BMM
**Implementation:** planned
**Depends on:** `openehr/bmm` loader (REQ-045), the existing `rminfo` generator
**Defers:** runtime BMM loading (stays forbidden — compiled-in tables only); AOM/AM hierarchy; generic-parameter resolution beyond the root-type reduction `rminfo` already applies; multiple-schema selection

## Goal

Extend `openehr/rm/rminfo` so a caller can ask the **hierarchy** questions the
Reference Model raises, not only the flat attribute questions it answers today:

- *Is this class abstract?* (naming it denotes its concrete descendants; no stored
  instance carries it as `_type`)
- *Who are its ancestors, and does class `A` conform to class `B`?* (the question
  behind AQL `CONTAINS`, polymorphic slot fit, and validation walkers)
- *Which concrete classes does naming an abstract class denote?* (abstract-class
  expansion: `ENTRY` → its concrete subtypes, per the pinned BMM)
- *Where is attribute `x` declared?* The generated per-class maps are already
  inheritance-**flattened** — `AttributeRMType("COMPOSITION", "name")` answers
  today because the generator folds `LOCATABLE`'s attributes into every
  descendant — but the fold erases the declaration *site*; a reader
  distinguishing own vs inherited attributes (BMM-faithful re-serialisation,
  schema diffing) cannot recover it

Consumers: any AQL-executing backend (class-expression expansion and containment
conformance), the SDK's own validation walkers, demographic PARTY reasoning, and a
future template-aware AQL lint layer.

## Motivation

Because `rminfo.Lookup` exposes no ancestry, no abstractness, no descendant
expansion, and no attribute declaration-site information (its flattened maps
answer the resolved question but erase where an attribute comes from), any
AQL-executing
module built on this SDK has to assemble its own BMM reducer, code generator,
generated hierarchy/property tables, and a model-equivalence test —
type-switching over `bmm.Class` and `bmm.Property` variants exactly as the
`rminfo` generator itself does. That is the same reduction of the same pinned
BMM maintained twice, and every module that reasons about the RM (rather than
about one template) faces the same fork: re-derive the model, or hard-code class
lists that drift. The SDK already owns the pinned schemas (REQ-041), the loader
(REQ-045), and the "compiled-in, no runtime BMM" discipline — the hierarchy
surface belongs beside them.

## Architecture

```
resources/bmm/ (pinned, REQ-041)
        │  existing rminfo generator, extended
        ▼
openehr/rm/rminfo
  ClassMeta        + Ancestors []string, Abstract bool   (generated)
  Hierarchy        new optional interface (precedent: AttributeLister)
  DeclaredOn       declaration-site lookup beside the flattened maps
```

Sketch (final shape is Phase 0 / review material, not normative here):

```go
// Hierarchy answers class-level model questions. Optional capability
// interface beside Lookup, following the AttributeLister precedent —
// extending the published Lookup interface would break external
// implementers of New(data).
type Hierarchy interface {
    // IsAbstract reports whether the model forbids instantiating rmType.
    // known=false when the model does not define the class at all.
    IsAbstract(rmType string) (abstract, known bool)
    // Ancestors returns every ancestor class name, transitively, sorted
    // (the Goal's question; an immediate-parents variant is a Phase 0
    // decision, not a doc drift). known=false for an undefined class —
    // an empty slice with known=true is a root class, a different answer.
    Ancestors(rmType string) (ancestors []string, known bool)
    // ConformsTo reports whether sub is rmType or descends from it.
    // known=false when EITHER name is undefined, so "no" and "never
    // heard of it" stay distinguishable.
    ConformsTo(sub, rmType string) (conforms, known bool)
    // ConcreteDescendants returns every concrete class rmType denotes:
    // rmType itself when concrete, plus every non-abstract strict
    // descendant, sorted. known=false for an undefined class; an empty
    // slice with known=true is an abstract class nothing concrete
    // extends — the negative-space rule below demands the distinction
    // on every method, not only IsAbstract.
    ConcreteDescendants(rmType string) (descendants []string, known bool)

    // DeclaredOn returns the class that declares attrName as seen from
    // rmType — rmType itself, or the nearest ancestor whose BMM class
    // declares it. The flattened AttributeRMType / IsContainer /
    // RequiredAttributes maps already answer the inheritance-RESOLVED
    // question and are unchanged; this recovers the site the fold
    // erases. A method on the capability interface, NOT a package-level
    // function: package-level would read Default's package state and
    // bypass the New(data) seam that exists precisely so synthetic-RM
    // unit tests (Phase 2's list) can exercise it.
    DeclaredOn(rmType, attrName string) (declaringClass string, ok bool)
}
```

Design constraints carried over from the landed package:

- **No runtime BMM dependency** — the data is generated into `lookup_gen.go`
  (extending `ClassMeta`), drift-detected against the pinned schemas exactly as
  REQ-042 requires for generated code.
- **Deterministic answers** — sorted ancestor/descendant slices; returned slices
  are copies, never the backing arrays of package state.
- **No reflection** (REQ-024); stdlib-only, no new dependencies.
- **The flattened maps stay flattened.** The generator keeps folding ancestor
  attributes into every class's map — `RequiredAttributes` and the REQ-112
  validation walkers depend on the resolved view. Hierarchy data is additive;
  no table shrinks to own-only declarations.
- Unknown class ≠ abstract class ≠ concrete class with no descendants — the three
  answers stay distinguishable, because a caller refusing a query needs to say
  *which* it is.

## Definition of Ready

Implementation (Phase 1+) may start once **Phase 0 has landed the REQ**:

- `Covers:` names the allocated REQ id (proposed REQ-124; Phase 0 settles band and
  home per the numbering policy — the 124–129 headroom is untouched by the
  150–159 transport extension band PR #107 allocated for REQ-150).
- Canonical normative prose exists — a spec section + a `REQ.md` registry row —
  including the exact method set and the unknown/abstract/dead-end distinction.
- PROBE-094 (or the id Phase 0 allocates) is defined in `conformance.md` (Draft):
  the generated hierarchy answers are equivalent to a fresh reduction of the
  pinned BMM (classes, ancestry, abstractness, per-class declarations).
- Negative space: unknown class ≠ abstract class ≠ concrete class with no
  descendants — three distinguishable answers, never conflated; `DeclaredOn` on
  an attribute the class does not carry reports not-found rather than guessing.
- Each phase names its verification command.

## Definition of Done

- `openehr/rm/rminfo` extended with `// REQ-` citations; generator updated;
  `lookup_gen.go` regenerated from the pinned BMM.
- Drift test (REQ-042 discipline) covers the new generated fields.
- `traceability.yaml` + the REQ.md **Impl.** column updated; `roadmap.md` row.
- `make spec-check` and `make ci` green; plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| REQ § + registry row (Phase 0) | |
| PROBE defined in `conformance.md` (Draft) | |
| Generator + `ClassMeta` extension + regenerated tables | |
| `Hierarchy` interface + `DeclaredOn` | |
| Tests with `// REQ-` / `// PROBE-` comments | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 0 — Spec & registry (the specify gate)

1. Allocate the REQ id and band (proposed: REQ-124 in the 124–129 headroom —
   an existing band, so no numbering-policy table change is needed).
2. Author the canonical section: the method set above, the compiled-in/no-runtime-BMM
   rule restated by citation (REQ-041/042), and the three-way
   unknown/abstract/no-concrete-descendant distinction as normative behaviour.
3. Registry row (**Impl.:** `planned`), probe definition, `traceability.yaml` row.

**Definition of done:** `make spec-check` passes with the new rows.

### Phase 1 — Generator and data

1. Extend the `rminfo` generator to emit `Ancestors` (sorted) and `Abstract` into
   `ClassMeta`; regenerate.
2. Extend the drift test to cover the new fields against the pinned BMM.

**Definition of done:** regenerated tables are byte-stable across two runs;
drift test red when a field is hand-edited.

### Phase 2 — Lookup surface

1. Implement `Hierarchy` on the default lookup (BFS up for `ConformsTo`, walk for
   `ConcreteDescendants`; sorted, copied results).
2. Implement `DeclaredOn` (ancestry walk from rmType toward the root; the first
   class whose BMM declaration carries the attribute wins — the declaration
   *site*, not a re-resolution the flattened maps already provide).
3. Table-driven tests: LOCATABLE-inherited attributes, abstract expansion of
   `ENTRY`/`CARE_ENTRY`, unknown-class behaviour, dead-end abstract classes.

**Definition of done:** `go test ./openehr/rm/rminfo/...` green; PROBE implemented.

## Mapping to specs

- Phase 0 authors the canonical section; until then this plan is the only home and
  deliberately contains **no normative prose** beyond proposals.
- [docs/specifications/REQ.md](../specifications/REQ.md) — registry row (Phase 0)
- [docs/specifications/bmm-conformance.md](../specifications/bmm-conformance.md) —
  REQ-041/042 disciplines this plan inherits
