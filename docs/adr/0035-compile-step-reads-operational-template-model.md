---
kind: adr
id: ADR-0035
title: "The compiled template: compiled from the generated operational template model, published from `openehr/templatecompile` by type alias"
status: accepted
date: 2026-11-03
---

# ADR 0035 — The compiled template: compiled from the generated operational template model, published from `openehr/templatecompile` by type alias

- **Status:** Accepted, 2026-11-03 — maintainer decision.
- **Amended:** 2026-11-25: takes in the compiled template's foundation and its public placement, and states the compile step as it reads the model today.
- **Strand:** resolves [STRAND-17](../specifications/research-strands.md#strand-17--the-opt-parser-and-the-generated-operational-template-model).
- **Amends:** [REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model) (its purpose and *Out of scope*), [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge) (`Compile` takes the model; *Reading the model*, *Path lookup*, *Node names and name predicates*), [REQ-103](../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection) (`C_CODE_REFERENCE`), [REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) (its reader), [REQ-108](../specifications/clinical-modeling.md#req-108--untrusted-document-bounds) (the OPT bounds on the model's reader), the reader rows of [wire.md § REQ-169](../specifications/wire.md#req-169--interval-boundary-flag-defaults), [wire.md § REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode) (a repeated `Hash` key), and the `openehr/template` row of [REQ-013](../specifications/module-layout.md#req-013--building-block-independence).
- **Requirements:** [REQ-048](../specifications/reference-model.md#req-048--rm-meta-model-introspection-surface) (the generated RM lookup the compile injects from); [REQ-101](../specifications/clinical-modeling.md#req-101--generic-opt-driven-composition-builder) and [REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation) (consumers of the compiled form).
- **Related:** [ADR 0034](0034-sdk-authored-root-schemas.md) (the model and its root schema).

## Context

**One document, many consumers.**
The composition builder, the instance generator, the validators, FLAT and STRUCTURED, the Web Template export and the AQL lint all need more than an OPT holds: AQL paths, the attributes the RM requires that the OPT leaves out, parent links, indexes and the scope of each term.
The generated operational template model ([REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model)) reads an OPT with no unmatched content.
A hand-written OPT reader reads only what it has code for: the retired parser handed consumers no constraint for a `C_CODE_REFERENCE`, in 10 of the 74 corpus OPTs, and nothing of the template-level `<constraints>`, in 10 more.

**Where the constructor can live.**
The compile engine imports `openehr/template` for the path and term types it hands out, and it needs `openehr/rm/rminfo` and `openehr/aom/aom14`, which [REQ-013](../specifications/module-layout.md#req-013--building-block-independence) keeps out of `openehr/template`'s closure.
A `Compile` in `openehr/template` would form an import cycle and break that closure.

**Why this is a fork.**
Which reader feeds the compile step decides the public surface of `openehr/template`, where the bounds and the tolerances live, and whether the SDK keeps two models of one document; where the constructor lives fixes the import path of every consumer.

## Decision

**Every template-driven consumer takes one compiled form, `templatecompile.Compiled`, which the compile step builds from the generated operational template model. The engine stays internal, and `openehr/templatecompile` publishes it by type alias.**

1. **One reader.**
   An OPT is read only into the model, by `aom14.ReadOperationalTemplate`, which applies the bounds of [REQ-108](../specifications/clinical-modeling.md#req-108--untrusted-document-bounds); a direct `canxml` decode stays unbounded.
   `templatecompile.Compile(opt *aom14.OperationalTemplate, opts ...Option)` turns the model into the compiled form.
   `openehr/template` holds no OPT model: it keeps `ParsePath`, the path errors and the small types the compiled template hands out, with `constraints` and `webtemplate` beneath it.
   Each tolerance the SDK grants an OPT is stated once, on the reader ([REQ-176 § What the reader tolerates](../specifications/clinical-modeling.md#what-the-reader-tolerates)) or on the compile step ([REQ-111 § Reading the model](../specifications/clinical-modeling.md#reading-the-model)).
2. **What the compile adds.**
   AQL paths, parent links and indexes; the attributes the BMM marks mandatory that the OPT leaves out, injected from the generated RM lookup `openehr/rm/rminfo` unless the caller passes `WithoutImplicitAttributes`; and term definitions and bindings scoped to each archetype root ([REQ-111 § Term definitions and bindings](../specifications/clinical-modeling.md#term-definitions-and-bindings)).
   `rminfo` is generated, imports only the standard library and reads no BMM at run time ([REQ-048](../specifications/reference-model.md#req-048--rm-meta-model-introspection-surface)).
3. **Lookup runs on the compiled template.**
   `Compiled.NodeAt` looks up the exact AQL path the compile wrote, and `Compiled.Resolve` and `Compiled.ValidatePath` walk the tree along a path `template.ParsePath` reads ([REQ-111 § Path lookup](../specifications/clinical-modeling.md#path-lookup)).
   Two nodes under different attributes that share an AQL path fail the compile ([REQ-111 § Reading the model](../specifications/clinical-modeling.md#reading-the-model)).
4. **Placement.**
   The engine lives in `internal/templatecompile`.
   `openehr/templatecompile` re-exports its types by type alias (`Compiled`, `CompiledNode`, `CompiledAttribute`, `ResolveOption`) and its sentinels `ErrInvalidInput` and `ErrPathNotFound` as the same values, beside a `Compile` whose functional options keep the engine's option struct off the public surface.
   The committed surface is [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge) *Surface*: everything reachable as a method on the three aliased types is public, and the engine and its free helpers are not.
   Consuming packages name `*templatecompile.Compiled` in exported signatures and import the engine, as `tcimpl`, in unexported code; the alias makes the two names one type.

**Not taken.**

- Two readers, the model and a parser: consumers keep missing what the parser leaves out, and each reader keeps tolerances of its own.
- A parser that decodes with the model and projects its own tree, or a converter between the two: a second or third representation to keep in step for no consumer the compiled template does not serve.
- Promoting the whole engine package to public: it exposes the engine and its free helpers, which no consumer needs.
- An opaque wrapper over the compiled form: consumers would convert at every call, and downstream code could not hold the node types in its own signatures.

## Consequences

- **Easier:** OPTs have one reader; the compiled template carries `C_CODE_REFERENCE` and the `<constraints>` default values with no parser code; an external module drives read, compile, build and validate through public packages alone (`openehr/templatecompile/external_test.go`, `cmd/examples/compile-build-validate`).
- **Harder:** every compile pays the cost of the generated XML decoders.
  Two path semantics, exact lookup and tree walk, live side by side on the compiled template, and the package doc states the difference.
  Two packages share the name `templatecompile`; type-alias identity means a mix-up cannot produce a type error.
- **Permanently constrained:** `CompiledNode` and `CompiledAttribute`, with every method on them, are public contracts on an engine that is still young.
  Before 1.0 the one area expected to change is multi-language term resolution, the `lang` parameter of `CompiledNode.Term`.

## Traceability

- Resolves: STRAND-17
- Amends: REQ-176, REQ-111, REQ-103, REQ-107, REQ-108, REQ-169, REQ-170, REQ-013
- Applies: REQ-048, REQ-101, REQ-102
