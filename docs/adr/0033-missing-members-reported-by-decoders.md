---
kind: adr
id: ADR-0033
title: "A missing mandatory member is reported by the decoder and read by the validators, not recorded on the value"
status: accepted
date: 2026-10-26
---

# ADR 0033 — A missing mandatory member is reported by the decoder and read by the validators, not recorded on the value

- **Status:** Accepted, 2026-10-26 — maintainer decision on issue [#319](https://github.com/Cadasto/openehr-sdk-go/issues/319), delivered as one rule for every RM class and both canonical formats.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** resolves the presence question of [STRAND-15](../specifications/research-strands.md#strand-15--decode-time-presence-and-unmatched-member-capture-on-the-generated-types); its capture question, whether unmatched members are kept on the value, stays open.
- **Amends:** [REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) (*Value-typed mandatory presence*, now the rule for every level and both formats), [REQ-171](../specifications/clinical-modeling.md#req-171--validation-engine) (*Surface*, *One finding per defect*, *Structural access* and *Out of scope*), [ADR 0002](0002-bmm-codegen-decisions.md) (D9: the generated canonical-XML decoders hand their runtime a description of the class on entry). **Introduces:** [REQ-175](../specifications/wire.md#req-175--missing-members-on-decode).
- **Related:** [REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode) (the dual rule, for content the decoder does not read), [REQ-052](../specifications/wire.md#req-052) (how an absent mandatory member is written back), [REQ-169](../specifications/wire.md#req-169--interval-boundary-flag-defaults) (the flags that take a value when absent), [REQ-168](../specifications/clinical-modeling.md#req-168--value-free-validation-diagnostics).

## Context

**What the value cannot say.** A canonical-JSON or canonical-XML document that leaves out a mandatory member decodes without error, and the member takes its Go type's zero value. Where the SDK holds the member by pointer, interface or slice, nil records the absence and the RM floor reports it. Where it holds the member by value, nothing does: a number, a boolean or a value struct such as `PARTY_SELF` or `ARCHETYPE_ID` reads the same whether the document carried it or not. The pinned RM makes 284 attributes mandatory outside the interval flags; 13 of them are numbers or booleans (`DV_QUANTITY.magnitude`, `DV_COUNT.magnitude`, `DV_BOOLEAN.value`, `DV_PROPORTION.numerator`, `denominator` and `type`, `DV_ORDINAL.value`, `DV_SCALE.value`, `DV_MULTIMEDIA.size`, `DV_STATE.is_terminal`, `ATTESTATION.is_pending`, `EHR_STATUS.is_queryable` and `is_modifiable`), and several more are value structs. Issue #319 measured the effect at v0.41.0: a `DV_QUANTITY` without `magnitude` validates clean and re-encodes as `"magnitude":0`, while the same document without `units` draws `required`. `ValidateRMEHRStatusBytes` closed the gap for one root and three keys by reading the source document; nothing covered the rest of the tree, and STRAND-15 asked where the record of presence should live.

**The forces.**

- `encoding/json/v2` offers no hook that says which fields of a struct a decode set. Recording presence on the generated types would need either a token-level decoder per class, which STRAND-15 reserves for an analysis note that sizes it, or a second read of each object's member names inside the decode, on the default path, for every caller.
- A record on the value is one more field on every generated class. The RM types are compared with `==`, built by hand, by the composition builder and by the instance generator, and read by the generated attribute access; each would have to know the field, and a value built in Go would carry an empty record that reads either as "all present" or as "all absent" depending on the record's direction.
- Canonical JSON already reads the bytes a second time for REQ-170's unmatched-member options, against the RM's attribute lists, and knows each object's class as it walks. Canonical XML already streams through the generated decoders and a runtime those decoders call on entry and exit of every element.
- Canonical JSON and canonical XML carry one instance (PROBE-105), so a consumer that receives either expects one answer, and the validators should take one list whichever format the document came in.
- The validators walk the decoded Go value, with the template's admission before the RM rules at every node (ADR 0027), and write one path per node with archetype-id predicates. A finding merged in after the walk, by string, would have to reproduce that path rule.

**Why this is a fork.** Where the record lives decides the shape of every API that carries it: a decoder option and a validator option that travel beside the value, or a field the value carries into every package that holds RM types. The generated canonical-XML decoders' entry contract with their runtime is a generator decision every generated file repeats.

## Decision

**The decoders report a missing member to the caller, beside the value, and the validators read that report; nothing is recorded on the value.** [§ REQ-175](../specifications/wire.md#req-175--missing-members-on-decode) states the rule for both canonical formats: what is missing, that it is reported on request and never refused, and that both codecs identify it by the member's JSON Pointer in the canonical-JSON form of the instance, so the two formats give one answer. [§ REQ-112 *Value-typed mandatory presence*](../specifications/clinical-modeling.md#value-typed-mandatory-presence) states how the validators consume it: every value-based entry, `ValidateRM`, `Validate` and `Validator.Validate`, takes the report, two entries decode and validate in one call, and a reported member is read as absent at the node its pointer names, inside the one walk, so the existing path rule and the one-finding-per-defect rule apply unchanged.

Canonical JSON reads the report from the same pass that serves REQ-170. Canonical XML reads it as it streams: the generator emits, for every class with a canonical-XML decoder, a static description of the class, its XML attributes, its mandatory members and its multi-valued members, which the decoder hands the runtime on entry, so the runtime needs no RM table and no program that uses the RM types links one.

## Consequences

- **Easier:** any RM class, at any depth, in either canonical format, reports the members its document left out, and the validators report each as `required` at its own path. `ValidateRMEHRStatusBytes` becomes a deprecated alias of the generic entry.
- **Harder:** canonical JSON's report costs a second read of the bytes, as REQ-170's options already do, and the RM attribute tables stay linked into every program that imports `canjson`. The generated canonical-XML decoders each carry one more declaration. The validators gain a per-call option, so a caller that decodes itself passes the report on.
- **Permanently constrained:** a value built in Go carries no report, so a by-value mandatory member left unset there reads as present, as today; only a decode can say what a document said. The pointer form is the one identification in both codecs, so a canonical-XML consumer reads a JSON Pointer for a missing member where REQ-170 gives it an element path for unmatched content.
- **Left open:** whether unmatched members are kept on the value for a lossless round trip (STRAND-15, capture). A later decision to record presence on the value would not undo this one: the options would then read that record instead of the bytes.

## Traceability

- Resolves: STRAND-15 (the presence question)
- Amends: REQ-112, REQ-171, ADR-0002
- Introduces: REQ-175
