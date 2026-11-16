---
kind: adr
id: ADR-0035
title: "The compile step reads the generated operational template model, which replaces the OPT parser and its tree"
status: accepted
date: 2026-11-03
---

# ADR 0035 — The compile step reads the generated operational template model, which replaces the OPT parser and its tree

- **Status:** Accepted, 2026-11-03 — maintainer decision, taken in the delivery of [REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model) once the corpus showed what the OPT parser leaves out, with the decisions for the switch and the removal that followed.
- **Strand:** resolves [STRAND-17](../specifications/research-strands.md#strand-17--the-opt-parser-and-the-generated-operational-template-model).
- **Amends:** [REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model) (its purpose and *Out of scope*), [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge) (`Compile` takes the model; *Reading the model*, *Path lookup*), [REQ-100](../specifications/retired-requirements.md#req-100--adl-14-operational-template-opt-parse-and-paths) (retired into REQ-111), [REQ-103](../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection) (`C_CODE_REFERENCE`), [REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) and REQ-116 (their readers; REQ-116 is since retired into [REQ-111](../specifications/clinical-modeling.md#node-names-and-name-predicates)), [REQ-108](../specifications/clinical-modeling.md#req-108--untrusted-document-bounds) (the OPT bounds on the model's reader), the reader rows of [wire.md § REQ-169](../specifications/wire.md#req-169--interval-boundary-flag-defaults), [wire.md § REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode) (a repeated `Hash` key), and the `openehr/template` row of [REQ-013](../specifications/module-layout.md#req-013--building-block-independence).
- **Related:** [ADR 0034](0034-sdk-authored-root-schemas.md) (the model and its root schema; it opened STRAND-17), [ADR 0005](0005-compiled-template-foundation.md) (the compiled template), [ADR 0010](0010-public-compiled-template-bridge.md) (its public bridge).

## Context

**Two readers of one document.**
[REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model) gives the SDK generated types that read an ADL 1.4 OPT with no unmatched content.
REQ-100's parser, `template.ParseOPT`, read the same documents into a tree of its own, and that tree was what `templatecompile.Compile` ([REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge)) turned into the compiled template that the composition builder, the validator, the instance generator, FLAT and STRUCTURED, the Web Template builder and the AQL lint take.
The parser read only what it had code for.
A `C_CODE_REFERENCE`, in 10 of the 74 corpus OPTs, reached those consumers with no constraint, and the template-level `<constraints>`, whose default values are RM `DATA_VALUE`s, in 10 OPTs, did not reach them at all.
Each such gap closed in the parser would have been a second implementation of something the model already reads.

**The forces.**

- The compile step adds what no OPT holds: AQL paths, the attributes the RM requires that the OPT leaves out, parent links and indexes. That work stays whatever the compile step reads.
- The parser was a tuned hot path with about twenty consumers. [REQ-108](../specifications/clinical-modeling.md#req-108--untrusted-document-bounds)'s size and depth bounds applied to it alone; canonical-XML decode had no such bound for an untrusted OPT.
- The parser had tolerances its consumers relied on: `null_flavor` read as `null_flavour`, white space around identifiers, `T_ARCHETYPE_ROOT`, and an unknown child type admitted as a leaf.
- [REQ-013](../specifications/module-layout.md#req-013--building-block-independence) keeps the RM and AOM types out of `openehr/template`'s closure, and path lookup (`NodeAt`, `ParsePath`, `ValidatePath`) lived there, over the parser's tree.

**Why this is a fork.**
Which reader feeds the compile step decides the public surface of `openehr/template`, the closure REQ-013 holds it to, where the bounds and the tolerances live, and whether the SDK keeps two models of one document.

## Decision

**The compile step reads the generated operational template model, which replaces `template.ParseOPT`'s tree as its source.**

- `templatecompile.Compile` takes the model's `OperationalTemplate`, read by `aom14.ReadOperationalTemplate`, which applies the REQ-108 bounds; a direct `canxml` call stays unbounded.
- Path lookup runs over the compiled template, not over the model, and `template.ParsePath` stays a function over strings.
- The model's reader and the compile step read every OPT the parser's default mode read, as it read it, with four refusals the reader makes in every mode: an integer past the 32-bit range, a `match_negated` of true, an infinite integer bound that admits no integer, and a `<definition>` the parser read as a slot or a primitive constraint. Under their strict options they refuse what the parser's strict mode refused. [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge) *Reading the model* states each reading as its own.
- `template.ParseOPT` and its tree, `template.NodeAt`, `template.ValidatePath` and `template.WithStrictPaths` were deprecated in 0.45.0 and removed in 0.46.0, and REQ-100 retired into REQ-111. `openehr/template` keeps `constraints`, `webtemplate` and the small shared types (`Path`, `Cardinality`, `Multiplicity`, `ArchetypeTerm`, `TermBinding`, `CodedTermRef` and the path errors).
- While both readers existed, a parity check held the compiled tree built from the model to the one built from the parser over every corpus OPT, apart from what the model adds. The compiled trees it held, recorded before the parser went, replace it ([§ REQ-111 *Recorded compiled trees*](../specifications/clinical-modeling.md#recorded-compiled-trees)).

**Not taken.** STRAND-17's three options.
Two readers: the consumers keep missing what the parser leaves out, and each reader keeps tolerances of its own.
A view over the model, the parser decoding with the model and projecting its tree: the tree, its API and the projection are kept in step for no consumer the compiled template does not already serve.
A converter outside `openehr/template` between the model and the tree: a third representation to keep in step.

## Consequences

- **Easier:** OPTs have one reader; the compiled template carries `C_CODE_REFERENCE` and the `<constraints>` default values with no parser code for them; each tolerance is stated once.
- **Harder:** every compile pays the generated decoders' cost, measured on the compile benchmarks at the switch. The parser's users moved: the examples that read the tree, `cmd/cdrcheck`, `testkit/fixtures` and the probes. Each tolerance is restated on the model without weakening the report of REQ-170.
- **Permanently constrained:** `template.ParseOPT` and its tree left the public API in 0.46.0, a break the pre-1.0 policy allows, announced by the 0.45.0 deprecation. The `openehr/template` row of REQ-013 states what the package now holds.

## Traceability

- Resolves: STRAND-17
- Amends: REQ-176, REQ-111, REQ-100 (retired), REQ-103, REQ-107, REQ-108, REQ-116, REQ-169, REQ-170, REQ-013
