---
kind: plan
---

# Plan — Simplified Formats: issues 420 to 428 and the FLAT backlog

**Date:** 2026-11-10
**Status:** Wave 1, PR A in review
**Covers:** [REQ-053](../specifications/wire.md#req-053), [REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes), [REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode), [REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export)
**Probes:** PROBE-075, PROBE-086, PROBE-105
**Defers:** the 15 `ctx/` fields the format sketches for EVENT_CONTEXT optionals; the STRUCTURED `"|value"` spelling; Web Template inputs for DV_URI, DV_IDENTIFIER, DV_MULTIMEDIA, DV_PARSABLE and DV_INTERVAL; the compiled path index (backlog, `internal/templatecompile/compile.go:670`)

## Goal

A client that writes FLAT or STRUCTURED from the ITS-REST 1.1.0 *Simplified Formats* text, or from the reference implementation's Web Template, gets its body decoded and validated, and a body this SDK encodes is one the reference can read. The work closes issues [420](https://github.com/Cadasto/openehr-sdk-go/issues/420) to [428](https://github.com/Cadasto/openehr-sdk-go/issues/428) and the five "FLAT and the Web Template" lines of [docs/backlog.md](../backlog.md).

## Evidence

Each issue was reproduced on `main` at `10e6a2f8` and checked three ways: against the EHRbase `openEHR_SDK` at the commit `testkit/corpus/UPSTREAM_PINS.txt` pins ([ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md)), against the ITS-REST Release-1.1.0 *Simplified Formats* sources, and by a census of the openEHR CNF FLAT bodies (15) and the Veredictum 0.4.1 simplified-format cases (112). The census is a scratch tool, re-run after each pull request; neither corpus is vendored.

## Decisions

| | Decision | Consequence |
|---|---|---|
| D1 | An ACTION's Web Template carries both shapes | The reference's in-context `time` and one merged `ism_transition` are added; the per-careflow-step `ism_transition[atNNNN]` nodes stay |
| D2 | Duplicate sibling ids follow the format's own table: "Blood Pressure" is `blood_pressure`, its duplicate `blood_pressure_1` | A divergence from the reference's `blood_pressure2`, recorded against ADR 0014 |
| D3 | A default composition time is an option, `WithDefaultTime(t time.Time)`; the zero time leaves it off | A body without `ctx/time` still decodes as today unless the caller asks |
| D4 | The census stays a scratch tool | No CC BY-SA content enters the tree |
| D5 | A reference set named by a `terminology:` URI names its terminology: `terminology:LOINC/2.73` is `LOINC(2.73)` | Decode fills a missing `\|terminology` from it; the openEHR specifications leave this open |

## Wave 1 — decode changes; callers change nothing

A, B and C touch the same decoder and the same REQ-053 text, so they run one after another.

### PR A — read what the format lets a client send

**Tasks:**

1. Input shorthands ([422](https://github.com/Cadasto/openehr-sdk-go/issues/422)): a bare DV_IDENTIFIER value is its `|id`; `|value` is the bare value of a DV_TEXT (a reference range's `meaning` included) and of a DV_PARSABLE; a bare `ctx/setting` is resolved in the openEHR `setting` group by code, then by rubric. Two spellings of one field that disagree stay an error. The `ctx/setting` error message is completed. Encode is unchanged.
2. Template completion of coded values under `WithTemplate` ([423](https://github.com/Cadasto/openehr-sdk-go/issues/423), widened): a DV_CODED_TEXT `|code` with no `|terminology` takes the terminology the template determines (`local` for a code in a local list, the constrained terminology where the template fixes one), and with no `|value` the code's term text; a DV_ORDINAL `|code` alone takes `|value` and `|ordinal` from the template's C_DV_ORDINAL. Where the template determines nothing, a missing `|terminology` is refused as a missing required suffix rather than decoded to an empty terminology. A decode without the template is unchanged.
3. INSTRUCTION `_wf_definition` ([426](https://github.com/Cadasto/openehr-sdk-go/issues/426)) is carried as a DV_PARSABLE in both directions, encoded as `|value` + `|formalism` as the PROBE-086 corpus writes it; and an empty DV_DATE_TIME writes no key (backlog, `datatypes.go:571`).
4. D5: `constraints.CodeReference` reads the terminology its `terminology:` URI names, and decode uses it where the code phrase names none; an empty duration or URI at a Web Template leaf writes no key either.

**Definition of done:** the issue reproductions decode and validate; PROBE-086 compares the two `_wf_definition` keys; `make ci` green.

### PR B — the format's defaults under `WithTemplate`

**Tasks:** the composition `category` from the template, so a persistent composition gets no EVENT_CONTEXT; then [420](https://github.com/Cadasto/openehr-sdk-go/issues/420) `WithDefaultTime`; then `HISTORY.origin` in the reference's order (an explicit `history_origin`, else the earliest event time, else the context start time); the in-context ACTIVITY `action_archetype_id` defaulted from the template's pattern.

### PR C — attributes the Web Template does not model, and how refusals are classed

**Tasks:** [424](https://github.com/Cadasto/openehr-sdk-go/issues/424) `|sample_count`, an abstract EVENT holding an INTERVAL_EVENT (backlog, `build.go:300`) and `history_origin`, all in the codec, since the reference gives none of them a Web Template node; then [428](https://github.com/Cadasto/openehr-sdk-go/issues/428), a sentinel for a key the format defines and this SDK does not model, which still matches `ErrUnknownPath`; a malformed key (`a//b|`) refused rather than ignored.

## Wave 2 — the Web Template moves toward the reference

Consumers see these changes. D and E amend the same REQ-106 sentence, so E stacks on D; F is independent.

- **PR D** — ACTION ([421](https://github.com/Cadasto/openehr-sdk-go/issues/421), backlog `build.go:299`): add the in-context `time` and one merged `ism_transition` (D1); encode writes the reference's spelling; the per-step keys still decode. Vendor the reference's `test_all_types.json` Web Template as a PROBE-075 fixture.
- **PR E** — an ELEMENT with several value types ([425](https://github.com/Cadasto/openehr-sdk-go/issues/425), backlog `build.go:354`): one `<type>_value` child per type; an unconstrained ELEMENT gets the reference's children instead of being dropped; the coded-text and free-text pair is recognised in either order; `choice|magnitude` still decodes.
- **PR F** — duplicate sibling ids ([427](https://github.com/Cadasto/openehr-sdk-go/issues/427), D2): `dedupeSiblingIDs` writes `_1`, `_2`, … and handles a collision with an ordinary id that already ends that way; PROBE-075 maps the reference's ids through the recorded deviation.

## Upstream

- openEHR ITS-REST *Simplified Formats*: make the RM-mapping examples follow the node-id table (`dv_text_1`, not `dv_text2`) and state rule 7's separator and first number; give the multi-type ELEMENT rule; `/_expiry_time` against `expiry_time`; the ENTRY `/territory` row.
- openEHR CNF: four test bodies break their template (an interval whose lower bound exceeds its upper, a duration outside its pattern, a partial date sent as an object, an ordinal outside the list).
