---
kind: adr
id: ADR-0028
title: "Generated attribute access replaces the hand-written RM readers"
status: proposed
date: 2026-10-13
---

# ADR 0028 — Generated attribute access replaces the hand-written RM readers

- **Status:** Proposed, 2026-10-13 — written from the design review that followed [PR 256](https://github.com/Cadasto/openehr-sdk-go/pull/256); accepted when the maintainer confirms, before any code depends on it.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** none — a direct decision, taken from the design principles agreed with the maintainer on 2026-10-13 (any archetypable root, any node; coverage of RM classes from generated code, never from a hand-maintained list).
- **Amends:** [REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation) (*Building-block independence*, the dependency list), [REQ-110](../specifications/clinical-modeling.md#req-110--template-driven-validation-beyond-composition) (*Implementation*, the closed routing sets), [REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) (*Driver* and *Known gap — classes rmread does not model*), [REQ-013](../specifications/module-layout.md#req-013--building-block-independence) (the block list names `openehr/validation/rmread`). **Applies:** [REQ-171](../specifications/clinical-modeling.md#req-171--validation-engine) (*Structural access*), [REQ-024](../specifications/idiom.md#generics-policy-req-024), [REQ-040](../specifications/rm-modeling.md#type-registry-req-040), [REQ-042](../specifications/bmm-conformance.md#req-042--generated-code-drift-detected) (the codegen drift gate).
- **Related:** [ADR 0002](0002-bmm-codegen-decisions.md) D4 and D6 constrain what the generator emits and where; [ADR 0013](0013-generated-locatable-identity-surface.md) is the precedent: a generated identity surface replaced about 200 hand-written switch arms once before; [ADR 0027](0027-template-admission-before-rm-rules.md) is the walk this access is the foundation of.

## Context

Both validation layers read an instance's values through `openehr/validation/rmread`: 1,931 lines of type switches, one arm per class and attribute, written by hand. Its `Handles` pin counts 71 classes; the type registry decodes 110. Every class the readers do not cover is a leaf to both walks: its attributes are neither read for the required-set check nor entered, so an INSTRUCTION_DETAILS with no `activity_id` passes the floor today, and LINK, FEEDER_AUDIT, DV_STATE, DV_PARAGRAPH and the time-specification types are never looked into. The readers also leave out optional attributes of the classes they do model, LOCATABLE's `uid`, `links` and `feeder_audit` and DV_TEXT's `language` among them, and the walk does not go below an attribute it cannot read. [§ REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) records all of this as a known gap, and the gap is structural: each BMM bump grows the switches by hand, and each class added by hand is one more place for the two walks to disagree.

The generator already emits the forward registry (`typereg_gen.go`), per-type attribute metadata (`rminfo`), the identity surface of [ADR 0013](0013-generated-locatable-identity-surface.md) and the canonical JSON and XML codecs. What it does not emit is a way to list a value's attributes with their values. [REQ-024](../specifications/idiom.md#generics-policy-req-024) rules reflection out, so the choice is between completing the hand-written readers to 110 classes and keeping them in step forever, or emitting the access.

The fork is irreversible on two counts. It changes what the generator emits, which [AGENTS.md](../../AGENTS.md) and [ADR 0002](0002-bmm-codegen-decisions.md) reserve for a decision with its rationale in the architecture narrative. And it retires `openehr/validation/rmread`, a public package, which is a break for any importer.

## Decision

The generator emits, for every concrete RM class the registry knows, access that lists the class's attributes: for each one its name, declared RM type, whether it is a container, whether it is present on the value, and the value, by plain field access. The validators read an instance through that access and through nothing else, and `openehr/validation/rmread` is retired. [REQ-171 *Structural access*](../specifications/clinical-modeling.md#req-171--validation-engine) carries the contract; a test pins that the set of classes the access covers equals the set the registry decodes, so a BMM bump that adds a class fails `make codegen-verify` until the generator is run.

The emitted access lives in a module-internal package unless a consumer need for it appears, so it adds no permanent API to the `rm` types; the alternative, methods on each generated type as [ADR 0013](0013-generated-locatable-identity-surface.md) chose for identity, stays open to the implementing change if the internal placement proves awkward for the walker. Where the template reaches, the compiled node may hold what is resolved once per template rather than once per instance: the accessor for each declared attribute and the conformance answer for each alternative. That precomputation is an optimisation the implementing change measures, not part of this decision.

## Consequences

- Both layers reach every registered class, so coverage of the RM comes from generated code and the *Known gap — classes rmread does not model* paragraph of REQ-112 goes, with the gap it describes. Instances that passed because a class was a leaf are reported; the corpus ratchet shows each new finding and each is reviewed.
- `openehr/validation/rmread` is removed, with its `Handles` pin and reader tests; before 1.0 that ships as a minor version, named in the release notes as a break. Its one helper other packages used, the typed-nil predicate, already delegates to `rm.IsTypedNil`.
- About 1,900 hand-maintained lines become generated code that is larger in total and smaller in what a person maintains, the trade [ADR 0013](0013-generated-locatable-identity-surface.md) made for identity.
- The walk visits more nodes per instance than before, so the visit limit of REQ-171 is reached earlier on a pathological value; the benchmark acceptance of the implementing change measures the cost on the largest corpus composition and on a synthetic one an order of magnitude larger.
- The REQ-013 import guard for `openehr/validation` is updated for the new internal dependency, and the block list in module-layout.md stops naming the reader package.

## Alternatives considered

- **Complete the hand-written readers to 110 classes.** Closes today's gap and reopens it at the next BMM bump; more lock-step, the opposite of what ADR 0013 chose.
- **Reflection.** Rejected outright under REQ-024.
- **Read through the generated JSON encoders.** Wrong layer: it serialises to find attributes, allocates per node, and loses the Go value the rules need.
- **Generate Go types per template.** Useful tooling for a fixed application; unsuitable as the SDK's core, since templates are run-time data.

## Traceability

- **Amends:** REQ-102, REQ-110, REQ-112, REQ-013
- **Applies:** REQ-171, REQ-024, REQ-040, REQ-042
