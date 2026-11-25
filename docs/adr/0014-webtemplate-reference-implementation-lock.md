---
kind: adr
id: ADR-0014
title: "The EHRbase `openEHR_SDK` is the reference for the Web Template and the FLAT and STRUCTURED spellings, with two id departures"
status: accepted
date: 2026-07-14
---

# ADR 0014 — The EHRbase `openEHR_SDK` is the reference for the Web Template and the FLAT and STRUCTURED spellings, with two id departures

- **Status:** Accepted, 2026-07-14 — maintainer sign-off on the REQ-106 specification.
- **Amended:** 2026-11-25: the scope covers the FLAT and STRUCTURED spellings the reference settles, and decision 2 takes in the two `id` spellings that depart from the reference's code.
- **Introduces:** [REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) (Web Template JSON export). **Amends:** [REQ-053](../specifications/wire.md#req-053) (path segments). **Applies:** [REQ-013](../specifications/module-layout.md#req-013--building-block-independence) (building-block independence), [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge) (the export's input, and its [node names](../specifications/clinical-modeling.md#node-names-and-name-predicates), which let a reused archetype under one slot export), [REQ-103](../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection) (the source of `inputs`), PROBE-075, PROBE-086, PROBE-105.
- **Related:** [ADR 0015](0015-flat-metadata-spelling.md) (the composition-metadata spellings); the deviation registers [`webtemplate/deviations.md`](../../openehr/template/webtemplate/deviations.md) and [`simplified/deviations.md`](../../openehr/serialize/simplified/deviations.md).

## Context

**The Web Template is a de facto format, not an openEHR-normative artefact.**
No openEHR schema defines it; only the FLAT and STRUCTURED serializations built on it are standardised, by the ITS-REST *Simplified Formats* specification, which names the mechanisms (suffixes, the `_` prefix, `id` generation) without spelling every case.
Two reference implementations exist and diverge: Better's `web-template` (Kotlin), camelCase `id`s, the originating one but frozen; and EHRbase's `openEHR_SDK` (Java), lower-snake `id`s and `version "2.3"`, maintained, with a corpus of matched OPT, Web Template and FLAT test data.

**The `id` is consumer-critical and effectively irreversible.**
A FLAT key is the Web Template `id`s joined, so a form or a mapping bound to the SDK's output depends on the exact sanitisation and sibling rule.
The two references give different `id`s for one node (`blood_pressure`, `bloodPressure`), so one has to be chosen before the algorithm is written, and with no schema to appeal to, "correct" means matching a concrete reference artefact.

**Where the reference's code and its own material disagree.**
For a duplicate sibling, rule 7 of the *Simplified Formats* § Node ID Generation Rules gives "Blood Pressure (duplicate)" as `blood_pressure_1`, while the reference's code, the format's own RM-mapping examples and the upstream FLAT corpus write `blood_pressure2`.
For an interval value alternative, the reference's code writes `interval<dv_date_time>_value`, a key EHRbase does not read itself, while the reference's IPS test Web Template writes `interval_of_date_time_value`.

## Decision

The SDK's Web Template export, and the FLAT and STRUCTURED spellings the *Simplified Formats* specification leaves open, follow **EHRbase `openEHR_SDK`, Web Template `version "2.3"`**, read at the commit [`testkit/corpus/UPSTREAM_PINS.txt`](../../testkit/corpus/UPSTREAM_PINS.txt) pins.

1. **Shape and version.** The export writes the EHRbase 2.3 Web Template JSON shape (camelCase JSON fields; root `templateId`, `version`, `defaultLanguage`, `languages`, `tree`; the node and input field set), with `version` the string `"2.3"`.
2. **`id` generation.** The `id` is a lower-snake sanitisation of the node's default-language display name, its normalisation derived from the reference's fixtures and pinned by table-driven tests, with two departures from the reference's code ([REQ-106 *`id` generation*](../specifications/clinical-modeling.md#id-generation-adr-0014)):
   - a duplicate sibling takes the *Simplified Formats* suffix `<id>_N`, the first claimant keeping the bare `id`;
   - an interval value alternative takes `interval_of_<type>_value`, the IPS test Web Template's spelling.

   Decode also reads a duplicate sibling in the reference's spelling, `<id><N+1>` for `<id>_N`, under the conditions of [wire.md § REQ-053](../specifications/wire.md#req-053); encode writes only `<id>_N`.
3. **Structural parity, not byte parity.** PROBE-075 compares the export with the reference's goldens structurally, the `id` set, `rmType`, `aqlPath`, `min`/`max` and each input's `suffix` and `type`, against a register of documented deviations, the two spellings of decision 2 among them.
   Field order, absent optional fields and the packaging of localized strings are recorded deviations, not failures.
4. **Fixtures.** The reference OPTs and Web Templates are vendored under `testkit/corpus/webtemplate/` from the pinned `openEHR_SDK` commit (Apache-2.0), with attribution in [`testkit/corpus/THIRD_PARTY_LICENSES.md`](../../testkit/corpus/THIRD_PARTY_LICENSES.md).
5. **FLAT and STRUCTURED spellings.** Where the *Simplified Formats* specification names a mechanism but not its spelling, such as a leaf datatype's suffix set, the `_`-attribute suffix vocabulary or the `|value` member of a bare value beside its suffixes, the codecs write the reference's, pinned by the PROBE-086 corpus ([wire.md § REQ-053](../specifications/wire.md#req-053), [§ REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes)).
   Where neither the specification nor the reference spells a case, the codec chooses within the format's conventions, records the choice in its deviation register, and follows the reference if a spelling appears there later.

The Better camelCase variant and output for more than one Web Template version are not produced.

## Consequences

- **Verifiable.** "Correct" is checked against a concrete, maintained artefact, and the vendored goldens give PROBE-075 a real oracle.
- **Inherited quirks.** Every other `id` and spelling mirrors the reference, including choices the SDK would not make in isolation: parity is worth more than local elegance, because interop is the point of the format.
- **The reference does not read the two departing spellings.** EHRbase reads neither `blood_pressure_1` nor `interval_of_date_time_value` at those nodes, so a consumer that sends the SDK's FLAT to it respells them.
- **One shape is read on a guess.** Beside a sibling whose own `id` is `<id>_K` ("Blood Pressure 1"), the reference's numbering and the SDK's part, and decode can read the reference's `blood_pressure2` as that sibling; no vendored template has that shape, and the codec's deviation register records it.
- **Probes compare in one spelling.** PROBE-075 maps the reference goldens' `id`s, and PROBE-086 and PROBE-105 respell the reference's FLAT keys, each pinning what it respelled.
- **Better-flavoured consumers are not served.** A Better variant would be an additive option, not a change to the default.
- **The deviation registers are load-bearing.** Every accepted structural difference from the reference is listed, so an unlisted one is a test failure; a bump of the reference is a reviewed change to the pin and the fixtures.
- **Archetype reuse under a slot.** Reference templates such as `corona_anamnese` reuse one archetype several times under a multi-valued slot, and export because template-level node names ([REQ-111](../specifications/clinical-modeling.md#node-names-and-name-predicates)) give each instance its own path and `id`.

## Alternatives considered

- **Better's `web-template` as the reference.** Rejected: frozen, with no maintained oracle, and its camelCase `id`s are less common in current deployments.
- **Both references, or output for several versions.** Rejected: it doubles the consumer-critical `id` surface and the conformance matrix with no consumer asking.
- **An `id` scheme of the SDK's own.** Rejected: FLAT interoperability needs a real implementation's `id`s; a bespoke scheme is correct against nothing.
- **Byte-exact parity with EHRbase.** Rejected as the conformance bar: field order and optional-field presence are incidental, and pinning them makes the SDK brittle to cosmetic reference changes.
- **The reference's code for duplicate siblings and interval alternatives.** Rejected: a body keyed from the *Simplified Formats* text would not decode, and the interval key is one the reference does not read itself.

## Traceability

- Introduces: REQ-106
- Amends: REQ-053
- Applies: REQ-013, REQ-103, REQ-111
