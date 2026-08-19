# Plan — RM class hierarchy and declaration-site attribute lookup (`rminfo`)

**Date:** 2026-08-18
**Status:** In progress
**Owner:** SDK maintainers
**Covers:** [REQ-048](../specifications/bmm-conformance.md#req-048--rm-meta-model-introspection-surface) — RM meta-model introspection surface.
**Probes:** [PROBE-094](../specifications/conformance.md#probe-094--rm-meta-model-introspection-equals-the-pinned-bmm) (Draft) — the generated tables vs an independent reduction of the pinned BMM
**Verifies / builds on:** landed `openehr/rm/rminfo` (BMM-derived structural lookup, [ADR 0005](../adr/0005-compiled-template-foundation.md)), [REQ-041](../specifications/bmm-conformance.md#req-041--pinned-bmm-sources) (pinned BMM sources), [REQ-042](../specifications/bmm-conformance.md#req-042--generated-code-drift-detected) (generated code, drift-detected)
**Implementation:** planned
**Depends on:** `openehr/bmm` loader (REQ-045), the existing `rminfo` generator
**Defers:** runtime BMM loading (stays forbidden — compiled-in tables only); AOM/AM hierarchy; generic-parameter resolution beyond the root-type reduction `rminfo` already applies; multiple-schema selection

> **Execution:** work the phases sequentially. Run each phase's verification command before moving on; a failing phase blocks the next.

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
  ClassMeta        + Abstract bool, Parents []string        (generated)
  AttrMeta         + DeclaredIn string                      (generated)
  Hierarchy        new optional interface (precedent: AttributeLister)
```

Sketch (settled in Phase 0; the exact doc comments are review material):

```go
// Hierarchy answers class-level model questions. Optional capability
// interface beside Lookup, following the AttributeLister precedent —
// extending the published Lookup interface would break external
// implementers of New(data).
type Hierarchy interface {
    // IsAbstract reports whether the model forbids instantiating rmType.
    // known=false when the model does not define the class at all.
    IsAbstract(rmType string) (abstract, known bool)

    // Parents returns rmType's immediate parent classes in BMM
    // declaration order — the faithful edge set a transitive closure
    // erases. known=false for an undefined class; an empty slice with
    // known=true is a root of the RM class universe (its BMM ancestors,
    // if any, lie outside that universe).
    Parents(rmType string) (parents []string, known bool)

    // Ancestors returns every ancestor class name, transitively, sorted.
    // known=false for an undefined class — an empty slice with
    // known=true is a root class, a different answer.
    Ancestors(rmType string) (ancestors []string, known bool)

    // ConformsTo reports whether sub is rmType or descends from it.
    // known=false when EITHER name is undefined, so "no" and "never
    // heard of it" stay distinguishable.
    ConformsTo(sub, rmType string) (conforms, known bool)

    // ConcreteDescendants returns every concrete class rmType denotes:
    // rmType itself when concrete, plus every non-abstract strict
    // descendant, sorted. known=false for an undefined class; an empty
    // slice with known=true is an abstract class nothing concrete
    // extends — the negative-space rule demands the distinction on
    // every method, not only IsAbstract.
    ConcreteDescendants(rmType string) (descendants []string, known bool)

    // DeclaredOn returns the class that declares attrName as seen from
    // rmType — rmType itself, or the ancestor whose BMM class supplied
    // the attribute the flattened tables report. The flattened
    // AttributeRMType / IsContainer / RequiredAttributes maps already
    // answer the inheritance-RESOLVED question and are unchanged; this
    // recovers the site the fold erases. A method on the capability
    // interface, NOT a package-level function: package-level would read
    // Default's package state and bypass the New(data) seam that exists
    // precisely so synthetic-RM unit tests can exercise it.
    DeclaredOn(rmType, attrName string) (declaringClass string, ok bool)
}
```

Design constraints carried over from the landed package:

- **No runtime BMM dependency** — the data is generated into `lookup_gen.go`
  (extending `ClassMeta` and `AttrMeta`), drift-detected against the pinned
  schemas exactly as REQ-042 requires for generated code.
- **Deterministic answers** — BMM declaration order for `Parents`, sorted
  ancestor/descendant slices; returned slices are copies, never the backing
  arrays of package state.
- **No reflection** (REQ-024); stdlib-only, no new dependencies.
- **The flattened maps stay flattened.** The generator keeps folding ancestor
  attributes into every class's map — `RequiredAttributes` and the REQ-112
  validation walkers depend on the resolved view. Hierarchy data is additive;
  no table shrinks to own-only declarations.
- The three-way distinction the surface must keep is canonical in
  [bmm-conformance.md § Three answers, never conflated](../specifications/bmm-conformance.md#three-answers-never-conflated)
  — not restated here, because a plan-side paraphrase is how the two drift.

### How two of the REQ's rules are implemented

1. **`Parents` ships alongside `Ancestors`.** The plan left the immediate-parents
   variant open. It is in: the BMM's own `ancestors` field *is* the immediate
   edge set, and a transitive sorted closure erases it exactly as the attribute
   fold erases the declaration site — the same loss this REQ exists to stop, for
   the same reader (BMM-faithful re-serialisation, schema diffing).
2. **`DeclaredOn` reads a generated field, it does not re-walk.** The generator
   already knows which class supplied each folded attribute, so it emits
   `AttrMeta.DeclaredIn` in the same assignment that sets the attribute's type /
   required / container triple. A runtime ancestry walk would be a *second*
   derivation of the same fact and could disagree with the fold — the
   divergence REQ-048 forbids. It is also O(1) instead of O(depth).

## Definition of Ready

Met by Phase 0 (landed in this branch):

- [x] `Covers:` names the allocated REQ id — **REQ-048**, in `bmm-conformance.md`.
- [x] Canonical normative prose exists — [bmm-conformance.md § REQ-048](../specifications/bmm-conformance.md#req-048--rm-meta-model-introspection-surface)
      plus the [REQ.md](../specifications/REQ.md) registry row — including the
      question set, the class universe, the graph-closure rule, and the
      unknown/abstract/dead-end distinction.
- [x] [PROBE-094](../specifications/conformance.md#probe-094--rm-meta-model-introspection-equals-the-pinned-bmm)
      is defined in `conformance.md` (Draft): the generated answers are
      equivalent to an **independent** reduction of the pinned BMM — through the
      `openehr/bmm` loader, deliberately not through the generator's own walk.
- [x] Negative space is normative in
      [bmm-conformance.md § Three answers, never conflated](../specifications/bmm-conformance.md#three-answers-never-conflated)
      and in the acceptance criteria there — cited, not restated, so the plan
      cannot drift from it.
- [x] Each phase names its verification command.

### The home decision (settled)

`bmm-conformance.md` / **REQ-048**, not the proposed REQ-124. The rationale is
canonical in [REQ.md § Numbering policy](../specifications/REQ.md#numbering-policy)
and is not repeated here.

## Definition of Done

- `openehr/rm/rminfo` extended with `// REQ-` citations; generator updated;
  `lookup_gen.go` regenerated from the pinned BMM.
- Drift test (REQ-042 discipline) covers the new generated fields.
- `traceability.yaml` + the REQ.md **Impl.** column updated; `roadmap.md` row.
- `make spec-check` and `make ci` green; plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| REQ § + registry row (Phase 0) | done |
| PROBE defined in `conformance.md` (Draft) | done |
| Generator + `ClassMeta` / `AttrMeta` extension + regenerated tables | done |
| `Hierarchy` interface + `DeclaredOn` | |
| Tests with `// REQ-` / `// PROBE-` comments | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 0 — Spec & registry (the specify gate) — **done**

1. [x] Allocated **REQ-048** in the BMM headroom (see § The home decision).
2. [x] Authored the canonical section: the question set, the class universe and
   its closure rule, the compiled-in/no-runtime-BMM rule restated by citation
   (REQ-041/042), the flattened-tables-stay-flattened rule, the
   declaration-site/fold agreement rule, and the three-way
   unknown/abstract/no-concrete-descendant distinction as normative behaviour.
3. [x] Registry row (**Impl.:** `planned`), PROBE-094 definition + coverage-matrix
   row, `traceability.yaml` entry, numbering-policy band update.

**Verification:** `make spec-check` — passes with the new rows.

### Phase 1 — Generator and data — **done**

1. [x] Extend the `rminfo` generator to emit into `ClassMeta`: `Abstract bool`
   (the BMM `is_abstract` flag) and `Parents []string` (the BMM `ancestors`
   list, **filtered to the emitted class universe**, in declaration order).
   Emit `AttrMeta.DeclaredIn` from the fold itself.
2. [x] **Widen the emitted class universe**: drop the `len(attrs) == 0` skip, so
   the attribute-less classes join the table. This is required by REQ-048 (a
   descendant expansion of `DATA_VALUE` is unanswerable while `DATA_VALUE` is
   absent) and it is the one behaviour change to a landed surface:
   `KnownRMTypes()` goes from **125 to 139** entries, gaining exactly the
   attribute-less classes the RM target already types —
   `ACCESS_CONTROL_SETTINGS`, `BASIC_DEFINITIONS`, `CODE_SET_ACCESS`,
   `DATA_VALUE`, `EXTERNAL_ENVIRONMENT_ACCESS`, `Iso8601_timezone`,
   `MEASUREMENT_SERVICE`, `OPENEHR_CODE_SET_IDENTIFIERS`,
   `OPENEHR_DEFINITIONS`, `OPENEHR_TERMINOLOGY_GROUP_IDENTIFIERS`, `PATHABLE`,
   `TERMINOLOGY_ACCESS`, `TERMINOLOGY_SERVICE`, `Time_Definitions`. Nothing is
   removed. The delta is pinned by test so a future widening is a deliberate
   edit; the whole suite (including `openehr/template/optvalidate` and the
   REQ-112 floor, the two `KnownRMTypes()` consumers) stays green.
3. [x] Regenerate; extend the drift coverage to the new generated fields.

**What the parent filter actually drops** (measured against the pinned BMM, not
assumed): `Any` — `PATHABLE`'s only ancestor, so `PATHABLE` is the one class the
filter turns into a root — plus `Ordered`, `Interval`, the `Iso8601_*` types and
the `PROPORTION_KIND` enumeration. Every class naming one of those names an RM
ancestor beside it (`DV_ORDERED` is `[DATA_VALUE, Ordered]`, `DV_INTERVAL` is
`[DATA_VALUE, Interval]`, `DV_PROPORTION` is `[PROPORTION_KIND, DV_AMOUNT]`), so
no RM edge is lost. `OPENEHR_DEFINITIONS` is **not** filtered — the RM target
emits it, so `DATA_VALUE`'s parent edge is real. The only genuine multiple
inheritance inside the universe is the two `support` service classes.

**Verification:** `make codegen-verify` (byte-stable across two runs; red when a
generated field is hand-edited) and `go test ./internal/bmmgen/...`.

### Phase 2 — Lookup surface

1. Implement `Hierarchy` on the default lookup: `Parents`/`IsAbstract` straight
   from `ClassMeta`; `Ancestors` as the transitive closure of `Parents`, sorted;
   `ConformsTo` over that closure; `ConcreteDescendants` over a lazily built
   inverse index (`sync.Once`, as `KnownRMTypes` already does), sorted. All
   returns are copies. Cycle-safe: a malformed synthetic model must not hang.
2. Implement `DeclaredOn` from `AttrMeta.DeclaredIn`.
3. Table-driven tests: `LOCATABLE`-inherited attributes, abstract expansion of
   `ENTRY`/`CARE_ENTRY`/`DATA_VALUE`, `PATHABLE` as a root, unknown-class
   behaviour on every method, dead-end abstract classes, and the synthetic-model
   seam (`New(data)`) for the shapes the pinned RM cannot supply.
4. Implement PROBE-094 beside the package: reduce the pinned BMM through
   `openehr/bmm` and compare the universe, abstractness, parents, ancestor
   closure, descendant expansion, and every declaration site.

**Verification:** `go test ./openehr/rm/rminfo/...` green; PROBE-094 promoted from
`Draft` to `Implemented (inline)` in `conformance.md`.

### Phase 3 — Wiring and close-out

1. `traceability.yaml`: `implementation: landed`, tests listed; REQ.md **Impl.**
   column to match. `roadmap.md` row. `doc.go` REQ citations.
2. CHANGELOG entry (one artefact-class bullet).
3. Archive the plan (`sdd-archive`) in the implementing PR.

**Verification:** `make spec-check` and `make ci` green.

## Mapping to specs

- [docs/specifications/bmm-conformance.md § REQ-048](../specifications/bmm-conformance.md#req-048--rm-meta-model-introspection-surface) — **canonical prose**
- [docs/specifications/REQ.md](../specifications/REQ.md) — registry row + numbering-policy note
- [docs/specifications/conformance.md § PROBE-094](../specifications/conformance.md#probe-094--rm-meta-model-introspection-equals-the-pinned-bmm) — probe definition
- REQ-041/042/045 — the disciplines this REQ inherits rather than restates
