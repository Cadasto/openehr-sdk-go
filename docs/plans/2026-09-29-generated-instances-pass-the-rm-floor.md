# Plan — Generated instances pass the RM floor

**Date:** 2026-09-29
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator), [REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor); evidence for [STRAND-14](../specifications/research-strands.md#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants) (REQ-102)
**Probes:** PROBE-027, extended to also assert `ValidateRM` (spec in Phase 0, probe in Phase 3)
**Implementation:** planned
**Depends on:** nothing functional. PR 188 (REQ-112 archetype roots, open at the time of writing) edits the same floor files, so whichever lands second rebases. Phase 0's spec edits were checked to merge cleanly with it
**Defers:** the STRAND-14 decision (whether `ValidateComposition` runs the floor); strict OPT 1.4 schema checks beyond `T_ARCHETYPE_ROOT` (Phase 4 records them, and they are optional); evaluating `Language_valid` and `Encoding_valid` in the floor, which needs the ISO 639-1 and IANA character-set registers vendored first

## Goal

`instance.Generate`, and `composition.NewSkeleton` built on it, produce compositions that pass template
validation (REQ-102) but break Reference Model rules. The bodies carry:
- the literal `"example"` as a date-time or a duration;
- `"example"` as an entry's language and encoding code;
- an ELEMENT with no name;
- a CLUSTER with no items;
- an ELEMENT with both a value and a null flavour, or with neither.

`ValidateComposition` passes all of them, and the RM floor (`ValidateRM`, REQ-112) catches only some. A CDR
that checks RM invariants on commit refuses every such body. After this plan, every generated instance
passes both `ValidateComposition` and `ValidateRM`, for both policies and both value fills, and the floor
reports every one of these defects it can evaluate. The language and encoding codes stay out of the floor (see
Validator gaps); the generator is what keeps them valid.

Consumers: anyone who seeds data with the generator, or serves an example-composition endpoint from it.

## Definition of Ready

Implementation may start when:

- **`**Covers:**`** lists every REQ-NNN (and STRAND-NN / ADR if applicable) this plan implements.
- Canonical normative prose exists for each covered REQ (topic spec section + registry row in [REQ.md](../specifications/REQ.md)); Phase 0 wrote the amendments.
- Any irreversible fork has an **Accepted** [ADR](../adr/). None was needed: Phase 0 leaves STRAND-14 open.
- Phases list concrete tasks and name the verification command (`make ci`, `make spec-check`).

## Definition of Done

The plan is complete when:

- Code and tests land with `// REQ-` citations.
- [`traceability.yaml`](../specifications/traceability.yaml) and the REQ.md **Impl.** column reflect the implementation.
- A [`roadmap.md`](../roadmap.md) row records what landed.
- The implementing PR flips REQ-107 back to `landed` (`REQ.md`, `traceability.yaml`), removes its Known gap and the two "specified ahead of the code" sentences on the REQ-112 rows, and marks PROBE-027's `ValidateRM` arm Implemented. STRAND-14 already carries the evidence.
- `make spec-check` and `make ci` pass.

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry updated (`traceability.yaml`, REQ.md row) | done (Phase 0) |
| Indexes `spec-check` misses (`roadmap.md` row) | done (Phase 0) |
| Code | |
| Tests with `// REQ-` comments | |
| `make spec-check` | |
| `make ci` | |

## The gap, reproduced

**Setup.**
- `main` at `89c34233`. The counts below were re-checked on `main` at `d1e099f8` (2026-09-30) and did not move.
- Corpus OPTs `testkit/corpus/templates/{vital_signs,Demonstration.v1,BMI,body_weight}.opt`.
- `composition.NewSkeleton(ctx, compiled, WithComposer(…), WithTerritory("NL"), WithValueFill(instance.RandomFill))`.
- The counts are for the `Minimal` policy. `Example` carries more (`vital_signs`: 8 date-times, 4 nameless ELEMENTs, 3 empty CLUSTERs, 4 ELEMENTs with neither value nor null flavour). `ExampleFill` and `RandomFill` give the same counts.
- The template pass returned OK on all 16 runs (4 templates, 2 policies, 2 fills). `ValidateRM` reported nothing for BMI and body_weight, and only defects 3 and 4 for vital_signs and Demonstration.v1.

```go
opt, _ := template.ParseFile("testkit/corpus/templates/vital_signs.opt")
c, _ := templatecompile.Compile(opt)
name := "Dr Bench"
comp, _ := composition.NewSkeleton(ctx, c,
	composition.WithComposer(&rm.PartyIdentified{Name: &name}),
	composition.WithTerritory("NL"),
	composition.WithValueFill(instance.RandomFill))
tv := validation.ValidateComposition(comp, c) // OK, 0 issues
rf := validation.ValidateRM(comp)             // 3 issues (CLUSTER items, ELEMENT name, archetype_node_id)
body, _ := canjson.Marshal(comp)              // "example" as DV_DATE_TIME.value and as ENTRY language/encoding code
```

| # | Defect in the generated body | RM rule (RM BMM) | Example path | Count: vital_signs / Demonstration.v1 / BMI / body_weight | `ValidateComposition` | `ValidateRM` |
|---|---|---|---|---|---|---|
| 1 | `DV_DATE_TIME.value` is `"example"`, and so is `DV_DURATION.value` | `DV_DATE_TIME` `Value_valid: valid_iso8601_date_time (value)`; `DV_DURATION` `Value_valid: valid_iso8601_duration (value)` | `/content[0]/data/events[0]/time` | 2 / 18 / 4 / 4, plus 5 `DV_DURATION` in Demonstration.v1 | passes | passes |
| 2 | `ENTRY.language` and `ENTRY.encoding` are `CODE_PHRASE` `local::example` | `ENTRY` `Language_valid`, `Encoding_valid` (`code_set (…).has_code (…)`) | `/content[0]/language`, `/content[0]/encoding` | 2 per ENTRY | passes | passes |
| 3 | an ELEMENT with no `name` and `archetype_node_id` `""` | `LOCATABLE.name` mandatory; `Archetype_node_id_valid: not archetype_node_id.is_empty` | `/content[0]/data/events[0]/state/items[0]` (vital_signs) | 1 / 0 / 0 / 0 | passes | reports (`required`) |
| 4 | a CLUSTER with `items` null | `CLUSTER.items` mandatory, cardinality `1..*` | `/content[0]/protocol/items[0]` (vital_signs); `/content[0]/data/events[n]/data/items[2]/items[0]`, the `openEHR-EHR-CLUSTER.anatomical_location.v1` slot (Demonstration.v1) | 1 / 4 / 0 / 0 | passes | reports (`cardinality`) |
| 5 | an ELEMENT with both `value` and `null_flavour`, or with neither | `ELEMENT` `Inv_null_flavour_indicated: is_null() xor null_flavour = Void`, an XOR | `/content[0]/data/events[0]/data/items[1]/items[12]` (Demonstration.v1, `at0016`, both) | both 0 / 4 / 0 / 0; neither 1 / 13 / 0 / 0 | passes | passes |

Also seen, not judged: DV_CODED_TEXT values in Demonstration.v1 whose `defining_code` is `local::example`.
Whether the OPT constrains those codes was not checked.

**Independent check.** FerroEHR 4.3.1 runs RM invariants on commit. It refused the v0.28.0 bodies of all six
templates tried (BMI, body_weight, clinical_notes.v0, Referral Request.v1, vital_signs, Demonstration.v1).
The ones that reached validation were refused with `422`, the other two with `400`. Its messages, one per
defect class:

```text
/context/start_time: Invariant Value_valid failed on type DV_DATE_TIME
/content[0]/language: code 'example' is not a valid language (ISO 639-1) (openEHR terminology)
/content[0]/encoding: code 'example' is not a valid character set (IANA) (openEHR terminology)
invalid canonical JSON body: missing field `name` (at $.content[0].data.events[0].state.items[0])       -> 400
invalid canonical JSON body: missing field `items` (at $.content[0].data.events[0].data.items[2].items[0]) -> 400
```

An implementation is not a specification; the rules in the table are the RM's own.

## Root causes (`openehr/instance/generate.go` on `main`)

1. **The `"example"` overwrite.** `populateBMMRequiredAttrs` first calls `populatePrimitiveDefault` on a
   new DV value, which sets a sound sentinel: `Now` for `DV_DATE_TIME`, `at0000`/`local` for `CODE_PHRASE`.
   It then recurses into that same value and writes the literal `"example"` into every BMM-required
   `String` attribute. That overwrites `DV_DATE_TIME.value` and `CODE_PHRASE.code_string`.
   `materialiseImplicitSingle` does the same for `String` attributes.
2. **Wrong codes for ENTRY language and encoding.** The generic `CODE_PHRASE` default (`at0000` / `local`)
   is used for `ENTRY.language` and `ENTRY.encoding`, which need an ISO 639-1 code and an IANA character
   set. The composition's language (`Options.Language`) and `UTF-8` (`IANA_character-sets`) are the values
   to use.
3. **Nameless LOCATABLEs.** `populateBMMRequiredAttrs` skips `name` and `archetype_node_id`. A LOCATABLE
   it materialises, such as an ELEMENT inside a BMM-required container, therefore gets neither.
4. **Empty CLUSTERs.** A CLUSTER created where the OPT gives no child constraint for `items` gets no items.
   This was seen for slot fillers and a protocol CLUSTER.
5. **Value and null flavour together, or neither.** When the OPT constrains both `value` and `null_flavour` (0..1)
   on an ELEMENT, as Demonstration.v1 does at `at0016`, the Example policy fills both. On other ELEMENTs (1 in
   vital_signs and 13 in Demonstration.v1 under `Minimal`) it fills neither. The RM wants exactly one. Why the
   neither cases arise is not traced yet; Phase 1 starts by tracing it.

## Validator gaps

- **The floor misses three rules, and can evaluate two of them.** `ValidateRM` (REQ-112) reports defects 3 and 4,
  but not 1, 2 or 5:
  - `Value_valid` on `DV_DATE_TIME`, `DV_DATE`, `DV_TIME` and `DV_DURATION` (defect 1). The RM defines the rule
    for all four, and Demonstration.v1 carries `"example"` in five `DV_DURATION` values;
  - `Inv_null_flavour_indicated` on `ELEMENT` (defect 5), an XOR: both, and neither, are violations;
  - `Language_valid` and `Encoding_valid` on `ENTRY` (defect 2). The floor cannot evaluate these: they need the
    ISO 639-1 and IANA character-set registers, `openehr/terminology` ships neither (its three code sets are
    compression algorithms, integrity-check algorithms and normal statuses), and the floor's trust model
    excludes external-code validation. They join the deferred coded invariants (`Setting_valid` and its
    siblings). The generator satisfies them by construction, and Phase 3 asserts that directly.
- **The template pass runs no floor at all.** `ValidateComposition` (REQ-102) runs none of these checks,
  which is STRAND-14. Its "Evidence needed" asks how often a template-valid, RM-invalid composition reaches
  a consumer. Here the SDK's own generator produces one on the default path, for every template tried.

## OPT fixes needed to reproduce with Code24 exports

`testkit/corpus/templates/social.opt` (SocialeAnamnese.v1) is a Code24 export, and it is not schema-valid
OPT 1.4:
- `template.ParseFile` refuses it: `expected element type <template> but have <OPERATIONAL_TEMPLATE>`.
- With the root renamed, `ParseOPT` accepts it. `ParseOPTStrict` then refuses only `T_ARCHETYPE_ROOT`.
- A strict consumer refuses such a file outright. FerroEHR 4.3.1 refused seven Code24 exports: biografie.v2,
  Crisis Monitor - DagScore.v2, Honos_plus.v1, Laboratorium uitslagen.v1, MiddelenGebruik.v1, Screening.v1
  and SocialeAnamnese.v1.

These changes make such a file standard OPT 1.4. With them, all seven upload to FerroEHR 4.3.1, and the SDK
compiles them with the same template ids:

1. **The root element.** `<OPERATIONAL_TEMPLATE … xsi:type="OPERATIONAL_TEMPLATE">` becomes
   `<template … xsi:type="OPT">`, with the closing tag to match.
2. **The template language.** The first `<original_language>` becomes `<language>`
   (`OPERATIONAL_TEMPLATE.language`).
3. **The top-level `archetype_id`.** It is removed; an OPT has none at the top level.
4. **The archetype root type.** `xsi:type="T_ARCHETYPE_ROOT"` becomes `xsi:type="C_ARCHETYPE_ROOT"`.
   openEHR `Template.xsd` (ITS-XML, AM Release-1.4) defines only `C_ARCHETYPE_ROOT`.
5. **The template id.** The `::<uuid>` suffix is dropped from the `template_id` value.
6. **Empty `lifecycle_state`.** `<lifecycle_state/>` gets a value, for example `unmanaged`.
7. **Empty `details`.** `<details/>` gets a details item with a `<language>` (an ISO_639-1 code) and a
   non-empty `<purpose>`.

```python
import re, sys, pathlib
src, dst = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
s = src.read_text()
s = re.sub(r'^<OPERATIONAL_TEMPLATE ([^>]*)xsi:type="OPERATIONAL_TEMPLATE">', r'<template \1xsi:type="OPT">', s, count=1)
s = re.sub(r'</OPERATIONAL_TEMPLATE>\s*$', '</template>\n', s)
s = s.replace('<original_language>', '<language>', 1).replace('</original_language>', '</language>', 1)
s = re.sub(r'\n  <archetype_id>\s*<value>[^<]*</value>\s*</archetype_id>', '', s, count=1)
s = s.replace('xsi:type="T_ARCHETYPE_ROOT"', 'xsi:type="C_ARCHETYPE_ROOT"')
s = re.sub(r'(<template_id>\s*<value>)([^<:]+)::[0-9a-f-]+(</value>)', r'\1\2\3', s, count=1)
s = s.replace('<lifecycle_state/>', '<lifecycle_state>unmanaged</lifecycle_state>', 1)
s = s.replace('<details/>', '<details>\n      <language>\n        <terminology_id>\n'
              '          <value>ISO_639-1</value>\n        </terminology_id>\n'
              '        <code_string>nl</code_string>\n      </language>\n'
              '      <purpose>Not specified</purpose>\n    </details>')
dst.write_text(s)
```

The `nl` language and the `Not specified` purpose fit these Dutch exports. Pick values that fit the file.

## Phases

### Phase 0 — Spec (`sdd-specify`)

**Status:** done on this branch. What it wrote:
- **REQ-107.** Floor-clean output for both `Policy` values and both `ValueFill` values, the ENTRY `language` and `encoding` defaults, and a Known gap. REQ-107 goes `partial` in `REQ.md` and `traceability.yaml` until the code lands.
- **REQ-112.** Two catalogue rows: `Value_valid` on the four temporal data values, and ELEMENT `Inv_null_flavour_indicated`. Each is marked as specified ahead of the code. `Language_valid` and `Encoding_valid` are not catalogue rows: the trust model excludes external-code validation and the SDK ships neither register, so the trust model section records them as deferred.
- **PROBE-027.** The spec now says it also asserts `ValidateRM`; the probe itself still asserts `ValidateComposition` only, until Phase 3.
- **STRAND-14.** The evidence above is recorded; the strand stays open.
- **`roadmap.md`.** A Planned row, and the coded-invariants Deferred row now names the two rules.

**Definition of done:** `make spec-check` passes, and each amended section cites the RM rule. Met.

### Phase 1 — Generator fixes

**Tasks:**
1. Stop the `"example"` overwrite: don't recurse into a value whose primitive default is already set, or
   skip `String` attributes that default already filled.
2. Fill `ENTRY.language` from the composition language and `ENTRY.encoding` with `UTF-8`.
3. Give every materialised LOCATABLE a `name` and a non-empty `archetype_node_id`.
4. Give a CLUSTER at least one item, or leave out an optional CLUSTER the OPT does not fill.
5. Fill exactly one of `value` and `null_flavour` on every ELEMENT, never both and never neither.

**Definition of done:** the table's five defects are gone for the four templates.

### Phase 2 — Floor additions

**Tasks:** implement the two Phase 0 catalogue rows in `ValidateRM` (`Value_valid` on the four temporal data
values, and ELEMENT `Inv_null_flavour_indicated` as an XOR), each with a test that fails when its rule is removed.
Both report the catch-all `rm_invariant` code, so the `floorInvariantCodes` pin in
`strand14_nonchaining_test.go` needs no new entry; check that it still passes.

**Definition of done:** the floor reports defects 1 and 5 on a body that carries them. Defect 2 is not
floor-detectable (see Validator gaps).

### Phase 3 — Corpus ratchet

**Tasks:** a test over every COMPOSITION OPT in `testkit/corpus/templates` and `testkit/corpus/webtemplate`,
for both `Policy` values and both `ValueFill` values. Each generated body must pass `ValidateComposition`
and `ValidateRM`, and its ENTRY `language` and `encoding` must equal `Options.Language` and `UTF-8`, asserted
directly because the floor cannot. Extend PROBE-027 (`testkit/probes/instance/probe_027_generated_validates.go`)
to also call `ValidateRM` and to run both `ValueFill` values. Today the ratchet would fail on all four templates
above, once Phase 2 detects defects 1 and 5.

**Definition of done:** `make ci` passes with the ratchet in place.

### Phase 4 — Code24 OPTs (optional)

**Tasks:**
- Add a converted, standard copy of `social.opt` to the corpus, so the ratchet covers it through `ParseFile`.
- Decide whether `ParseOPTStrict` should also refuse three deviations it accepts today:
  - a missing `OPERATIONAL_TEMPLATE.language`, with `original_language` in its place;
  - a top-level `archetype_id`;
  - empty mandatory description fields.

**Definition of done:** the decision is recorded; if it is yes, each refusal has a test.

## Mapping to specs

- [clinical-modeling.md § REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator): the generator contract.
- [clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor): the RM floor.
- [research-strands.md § STRAND-14](../specifications/research-strands.md#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants): template-driven validation and the floor.
- [conformance.md § PROBE-027](../specifications/conformance.md#probe-027--generated-instance-validates-clean): the generator and validator probe.
- [REQ.md](../specifications/REQ.md): the registry rows.
