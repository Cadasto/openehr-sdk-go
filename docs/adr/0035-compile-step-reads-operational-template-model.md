---
kind: adr
id: ADR-0035
title: "The compile step reads the generated operational template model; the OPT parser and its tree are deprecated once it does"
status: accepted
date: 2026-11-03
---

# ADR 0035 — The compile step reads the generated operational template model; the OPT parser and its tree are deprecated once it does

- **Status:** Accepted, 2026-11-03 — maintainer decision, taken in the delivery of [REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model) once the corpus showed what the OPT parser leaves out.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** resolves [STRAND-17](../specifications/research-strands.md#strand-17--the-opt-parser-and-the-generated-operational-template-model).
- **Amends:** now, [REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model) (its purpose and *Out of scope*). When the switch lands: [REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) (the parser and its tree deprecated, path lookup on the compiled template), [REQ-108](../specifications/clinical-modeling.md#req-108--untrusted-document-bounds) (the OPT bounds on the model's reader), [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge) (`Compile` takes the model), [REQ-103](../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection) (`C_CODE_REFERENCE`), [REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) and [REQ-116](../specifications/clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths) (their readers), the [wire.md § REQ-169](../specifications/wire.md#req-169--interval-boundary-flag-defaults) reader rows, and the `openehr/template` row of [REQ-013](../specifications/module-layout.md#req-013--building-block-independence).
- **Related:** [ADR 0034](0034-sdk-authored-root-schemas.md) (the model and its root schema; it opened STRAND-17), [ADR 0005](0005-compiled-template-foundation.md) (the compiled template), [ADR 0010](0010-public-compiled-template-bridge.md) (its public bridge), [REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode) (what a tolerance must not hide).
- **Amended 2026-11-03, maintainer decisions for the switch:** (1) one change lands the bounded reader, the switch, path lookup, the deprecation and the callers' move. (2) The new path keeps only the tolerances the corpus needs: `null_flavor` read as `null_flavour`, white space trimmed around identifiers and term text, the interval flag defaults and a byte-order mark; the compile step refuses a missing `template_id` or `definition` and an inverted interval; the parser's lenient readings stay the deprecated parser's and are not restated for the new path. (3) Path lookup runs over the compiled template, not over the model, and `template.ParsePath` stays a function over strings. (4) `aom14.ReadOperationalTemplate` applies the REQ-108 bounds, the depth one during the decode through a canonical-XML decode option with its own sentinel; a direct `canxml` call stays unbounded. `openehr/template` also keeps `ArchetypeTerm`, `TermBinding` and `CodedTermRef`. The switch also amends [REQ-103](../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection), [REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator), [REQ-116](../specifications/clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths) and the [wire.md § REQ-169](../specifications/wire.md#req-169--interval-boundary-flag-defaults) reader rows.

## Context

**Two readers of one document.** [REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model) gives the SDK generated types that read an ADL 1.4 OPT with no unmatched content. [REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths)'s parser, `template.ParseOPT`, reads the same documents into a tree of its own, and that tree is what `templatecompile.Compile` ([REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge)) turns into the compiled template that the composition builder, the validator, the instance generator, FLAT and STRUCTURED, the Web Template builder and the AQL lint take. The parser reads only what it has code for. A `C_CODE_REFERENCE`, in 10 of the 74 corpus OPTs, reaches those consumers with no constraint, and the template-level `<constraints>`, whose default values are RM `DATA_VALUE`s, in 10 OPTs, does not reach them at all. Each such gap closed in the parser is a second implementation of something the model already reads.

**The forces.**

- The compile step adds what no OPT holds: AQL paths, the attributes the RM requires that the OPT leaves out, parent links and indexes. That work stays whatever the compile step reads.
- The parser is a tuned hot path with about twenty consumers. [REQ-108](../specifications/clinical-modeling.md#req-108--untrusted-document-bounds)'s size and depth bounds apply to it alone; canonical-XML decode has no such bound for an untrusted OPT.
- The parser has tolerances its consumers rely on: `null_flavor` read as `null_flavour`, white space around identifiers, `T_ARCHETYPE_ROOT`, and an unknown child type admitted as a leaf.
- [REQ-013](../specifications/module-layout.md#req-013--building-block-independence) keeps the RM and AOM types out of `openehr/template`'s closure, and path lookup (`NodeAt`, `ParsePath`, `ValidatePath`) lives there, over the parser's tree.

**Why this is a fork.** Which reader feeds the compile step decides the public surface of `openehr/template`, the closure REQ-013 holds it to, where the bounds and the tolerances live, and whether the SDK keeps two models of one document.

## Decision

**The compile step reads the generated operational template model, which replaces `template.ParseOPT`'s tree as its source.** `templatecompile` stays and takes the model's `OperationalTemplate`. In the change that makes the switch, path lookup and its rules for a path without node ids are rewritten over the model; the parser's tolerances are kept, by the model's reader or by the compile step, for those listed above and whatever else a corpus parity check shows the compile step needs; and `template.ParseOPT` and its node tree are deprecated, to be removed in a later release. `openehr/template` stays the home of `constraints`, `webtemplate` and the small shared types (`Cardinality`, `Multiplicity`, `ErrPathNotFound`). The REQ-108 bounds reach the model's reader before the switch, so an untrusted OPT is bounded on the new path as on the old one. The parity check holds the compiled tree built from the model to the one built from the parser over every corpus OPT, apart from what the model adds (`C_CODE_REFERENCE`, the `<constraints>` default values). Until the switch lands, the parser feeds the compile step, as [§ REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model) *Out of scope* says.

**Not taken.** STRAND-17's three options. Two readers: the consumers keep missing what the parser leaves out, and each reader keeps tolerances of its own. A view over the model, the parser decoding with the model and projecting its tree: the tree, its API and the projection are kept in step for no consumer the compiled template does not already serve. A converter outside `openehr/template` between the model and the tree: a third representation to keep in step.

## Consequences

- **Easier:** OPTs have one reader; the compiled template carries `C_CODE_REFERENCE` and the `<constraints>` default values with no parser code for them; each tolerance is stated once.
- **Harder:** every compile pays the generated decoders' cost, so the switch compares the compile benchmarks before and after. The parser's users move: the examples that read the tree, `cmd/cdrcheck`, `testkit/fixtures` and the probes. Each tolerance is restated on the model without weakening the report of REQ-170.
- **Permanently constrained:** `template.ParseOPT` and its tree leave the public API, a break the pre-1.0 policy allows, announced by a deprecation first. The `openehr/template` row of REQ-013 is restated for what the package then holds.

## Traceability

- Resolves: STRAND-17
- Amends: REQ-176; when the switch lands, REQ-100, REQ-108, REQ-111, REQ-013
