---
kind: plan
---

# Plan — Generated instances pass the RM floor

**Date:** 2026-09-29
**Status:** Done — phases 0 to 4 landed
**Owner:** SDK maintainers
**Covers:** [REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator), [REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor); evidence for [STRAND-14](../specifications/research-strands.md#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants) (REQ-102)
**Probes:** PROBE-027. PR 196's corpus census already runs `ValidateRM`; this plan makes it see the ELEMENT rule (Phases 2 and 3)
**Implementation:** landed for phases 0 to 4
**Depends on:** PR 196, merged: it fixed defects 1 to 4 and 6 in the generator, added the corpus census `TestREQ107_CorpusRatchet`, and bound generated output to the RM floor in REQ-107. PR 188 has landed, and Phase 0's spec edits sit on top of its catalogue.
**Defers:** the STRAND-14 decision (whether `ValidateComposition` runs the floor); evaluating `Language_valid` and `Encoding_valid` in the floor, which needs the ISO 639-1 and IANA character-set registers vendored first. Phase 4 declined two parser changes: strict mode does not grow checks for a missing language (with original_language in its place), a top-level archetype id, or empty description fields, and the lenient parse stays silent when it drops a nested subtree.

## Goal

`instance.Generate`, and `composition.NewSkeleton` built on it, produce compositions that pass template
validation (REQ-102) but break Reference Model rules. The bodies carry:
- the literal `"example"` as a date-time or a duration;
- `"example"` as an entry's language and encoding code;
- an ELEMENT with no name;
- a CLUSTER with no items;
- an ELEMENT with both a value and a null flavour, or with neither;
- a DV_ORDINAL with no symbol.

`ValidateComposition` passes all of them, and the RM floor (`ValidateRM`, REQ-112) catches only some. A CDR
that checks RM invariants on commit refuses every such body.

PR 196 fixes all of them except the ELEMENT one, which it makes more frequent (see Root causes, item 5). What
this plan still owns:
- the generator fix for that ELEMENT rule;
- floor rows for the ELEMENT rule and for temporal `Value_valid`, so the floor reports defects 1 and 5 on any
  body, not only on generated ones (the language and encoding codes stay out of the floor, see Validator gaps);
- the census additions that make PR 196's ratchet see them, and a coverage floor;
- the STRAND-14 evidence, and the Code24 OPT conversion (Phase 4).

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
- The [`roadmap.md`](../roadmap.md) Planned row for this work is removed once it lands.
- The implementing PR flips REQ-107 back to `landed` (`REQ.md`, `traceability.yaml`), removes its Known gap and the two "specified ahead of the code" sentences on the REQ-112 rows, and marks PROBE-027's `ValidateRM` arm Implemented. STRAND-14 already carries the evidence.
- `make spec-check` and `make ci` pass.

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry updated (`traceability.yaml`, REQ.md row) | done (Phase 0) |
| Indexes `spec-check` misses (`roadmap.md` row) | done (the Planned row is removed) |
| Code | done (phases 1 to 3) |
| Tests with `// REQ-` comments | done |
| `make spec-check` | done |
| `make ci` | done |

## The gap, reproduced

**Setup.**
- `main` at `89c34233`. The counts below were re-checked on `main` at `f38f1ed4` (2026-10-01) and did not move. What did move is the floor: PR 188 taught it to read DV_ORDINAL, which exposes defect 6.
- Corpus OPTs `testkit/corpus/templates/{vital_signs,Demonstration.v1,BMI,body_weight}.opt`.
- `composition.NewSkeleton(ctx, compiled, WithComposer(…), WithTerritory("NL"), WithValueFill(instance.RandomFill))`.
- The counts are for the `Minimal` policy. `Example` carries more (`vital_signs`: 8 date-times, 4 nameless ELEMENTs, 3 empty CLUSTERs, 4 ELEMENTs with neither value nor null flavour). `ExampleFill` and `RandomFill` give the same counts.
- The template pass returned OK on all 16 runs (4 templates, 2 policies, 2 fills). `ValidateRM` reports nothing for BMI and body_weight, defects 3 and 4 for vital_signs, and defects 4 and 6 for Demonstration.v1.
- **On `main` after PR 196 (`861e9997`, measured 2026-10-01)** only defect 5 remains, and `ValidateRM` reports nothing on any of the 16 runs. ELEMENTs with neither value nor null flavour, under `Minimal` / `Example`: vital_signs 2 / 7, Demonstration.v1 17 / 17, BMI 0 / 2, body_weight 1 / 1. With both: Demonstration.v1 4 / 4 (`at0016`). The fill makes no difference.

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
| 6 | a DV_ORDINAL with no `symbol` | `DV_ORDINAL.symbol` is mandatory (`DV_CODED_TEXT`, 1..1) | `/content[0]/data/events[0]/data/items[1]/items[11]/value/symbol` (Demonstration.v1) | 0 / 4 / 0 / 0 | passes | reports (`required`) |

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

## Root causes (`openehr/instance/generate.go` on `main`, before PR 196)

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
   PR 196 adds a source of its own: to give an empty CLUSTER, ITEM_LIST, ITEM_TREE or ITEM_SINGLE a member, it
   synthesises an ELEMENT (`archetype_node_id` `at0000`, name `element`) with neither `value` nor `null_flavour`
   (every `applyLocatableIdentity(el, "at0000", "element", …)` call in `generate.go`). That is why the neither
   count went up.
6. **A DV_ORDINAL with no symbol.** The generator fills a DV_ORDINAL's integer `value` and never its `symbol`,
   although `CDvOrdinal.Values` pairs each integer with a coded symbol (`local::at0038` for the first entry of
   Demonstration.v1's first ordinal list). The floor could not see it until PR 188 taught it to read DV_ORDINAL.

## Validator gaps

- **The floor misses three rules, and can evaluate two of them.** `ValidateRM` (REQ-112) reports defects 3, 4 and 6,
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
- With the root renamed, `ParseOPT` accepts it and drops every subtree under a `T_ARCHETYPE_ROOT` without a word
  (next paragraph). `ParseOPTStrict` refuses it, naming the type.
- A strict consumer refuses such a file outright. FerroEHR 4.3.1 refused seven Code24 exports: biografie.v2,
  Crisis Monitor - DagScore.v2, Honos_plus.v1, Laboratorium uitslagen.v1, MiddelenGebruik.v1, Screening.v1
  and SocialeAnamnese.v1.

**The lenient parse drops content silently.** REQ-100 keeps `ParseOPT` forward-compatible: an unknown child
`xsi:type` is admitted as a leaf node, and everything nested under it is discarded with no error and no report.
A `T_ARCHETYPE_ROOT` is such a type, so every entry archetype under one is lost and the template compiles hollow.
Measured on `social.opt` (SocialeAnamnese.v1) with the root renamed: a generated body has 0 ELEMENTs under
`Minimal` and 0 under `Example`, against 2 and 26 once its seven `T_ARCHETYPE_ROOT` become `C_ARCHETYPE_ROOT`.
On the other Code24 exports the same conversion takes the ELEMENT count per generated body from 0 to between 2
and 26 (biografie.v2 0 to 11, Honos_plus.v1 0 to 26); those files are not in the corpus, so only `social.opt`
reproduces here. The existing real-world synthesis test loads `social.opt` through this lenient path and asserts
the content count only, so it does not see the hollow entries. A hollow body has nothing to violate: it passes
`ValidateComposition` and `ValidateRM` vacuously, and a consumer that stores such an OPT through the lenient
parse serves hollow example compositions without knowing.

These eight changes make such a file standard OPT 1.4. With them, all seven upload to FerroEHR 4.3.1, and the SDK
compiles them with the same template ids:

1. **The root element.** `<OPERATIONAL_TEMPLATE … xsi:type="OPERATIONAL_TEMPLATE">` becomes
   `<template … xsi:type="OPERATIONAL_TEMPLATE">`, with the closing tag to match. The type value stays `OPERATIONAL_TEMPLATE`; EHRbase accepts that on a `<template>` root and refuses only the element name `OPERATIONAL_TEMPLATE`.
2. **The template language.** The first `<original_language>` becomes `<language>`
   (`OPERATIONAL_TEMPLATE.language`).
3. **The top-level `archetype_id`.** It is removed; an OPT has none at the top level.
4. **The archetype root type.** `xsi:type="T_ARCHETYPE_ROOT"` becomes `xsi:type="C_ARCHETYPE_ROOT"`.
   openEHR `Template.xsd` (ITS-XML, AM Release-1.4) defines only `C_ARCHETYPE_ROOT`.
5. **The template id.** The `::<uuid>` suffix is dropped from the `template_id` value.
6. **Empty `lifecycle_state`.** `<lifecycle_state/>` gets a value, for example `unmanaged`.
7. **Empty `details`.** `<details/>` gets a details item with a `<language>` (an ISO_639-1 code) and a
   non-empty `<purpose>`.
8. **Empty `original_author`.** `<original_author/>` becomes
   `<original_author id="Original Author">Not Specified</original_author>`, the form the other corpus OPTs use.
   `Resource.xsd` (ITS-XML, AM Release-1.4) declares the element as a `StringDictionaryItem` with a required
   `id` attribute, and it is mandatory (`minOccurs` 1, unbounded). Screening.v1 carries the bare form.

```python
import re, sys, pathlib
src, dst = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
s = src.read_text()
s = re.sub(r'^<OPERATIONAL_TEMPLATE ([^>]*)xsi:type="OPERATIONAL_TEMPLATE">', r'<template \1xsi:type="OPERATIONAL_TEMPLATE">', s, count=1)
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
s = s.replace('<original_author/>', '<original_author id="Original Author">Not Specified</original_author>', 1)
dst.write_text(s)
```

The `nl` language and the `Not specified` purpose fit these Dutch exports. Pick values that fit the file.

## Phases

### Phase 0 — Spec (`sdd-specify`)

**Status:** done on this branch. What it wrote:
- **REQ-107.** PR 196 made passing the RM floor binding on generated output. Phase 0 adds that the rule binds the RM itself, so the generator satisfies the invariants the floor does not check yet (the ELEMENT rule, temporal `Value_valid`), plus the ENTRY `language` and `encoding` defaults and a Known gap for the ELEMENT rule. REQ-107 goes `partial` in `REQ.md` and `traceability.yaml` until the code lands.
- **REQ-112.** Two catalogue rows: `Value_valid` on the four temporal data values, and ELEMENT `Inv_null_flavour_indicated`. Each is marked as specified ahead of the code. `Language_valid` and `Encoding_valid` are not catalogue rows: the trust model excludes external-code validation and the SDK ships neither register, so the trust model section records them as deferred.
- **PROBE-027.** The title now names `ValidateRM` and both `ValueFill` values. Its wire assertion and status are PR 196's, whose census already runs both.
- **STRAND-14.** The evidence above is recorded; the strand stays open.
- **`roadmap.md`.** A Planned row, and the coded-invariants Deferred row now names the two rules.

**Definition of done:** `make spec-check` passes, and each amended section cites the RM rule. Met.

### Phase 1 — Generator fix for the ELEMENT rule

PR 196 did tasks 1 to 4 and 6 of the original list (the `"example"` overwrite, ENTRY language and encoding,
nameless LOCATABLEs, empty CLUSTERs, ordinal symbols). One task is left.

**Tasks:**
5. Fill exactly one of `value` and `null_flavour` on every ELEMENT, never both and never neither:
   - a synthesised member (PR 196's `at0000` ELEMENT) gets a `null_flavour` from the openEHR `null flavours`
     group (`openehr/terminology`), since it has no value constraint to fill;
   - an ELEMENT the OPT constrains with both attributes gets the `value` only;
   - trace the other neither cases first, then fill the `value` where the OPT constrains one and a `null_flavour`
     where it does not.

**Definition of done:** no ELEMENT with both or neither attribute in the 16 runs above, and none in the census.

### Phase 2 — Floor additions

**Tasks:** implement the two Phase 0 catalogue rows in `ValidateRM` (`Value_valid` on the four temporal data
values, and ELEMENT `Inv_null_flavour_indicated` as an XOR), each with a test that fails when its rule is removed.
Both report the catch-all `rm_invariant` code, so the `floorInvariantCodes` pin in
`strand14_nonchaining_test.go` needs no new entry; check that it still passes.

**Definition of done:** the floor reports defects 1 and 5 on a body that carries them. Defect 2 is not
floor-detectable (see Validator gaps).

### Phase 3 — Corpus ratchet

**Tasks:** extend PR 196's `TestREQ107_CorpusRatchet` (`testkit/probes/instance/corpus_ratchet_test.go`), which
already runs `Generate` (both policies, both fills) and `composition.NewBuilder` over every vendored OPT that
compiles, with `ValidateRM` and the template validator, and walks the output for placeholders and example
language codes.
- Once Phase 2 lands, the census sees the ELEMENT rule and temporal `Value_valid` through `ValidateRM`. Give
  `issueReason` a category for them, so a failure is keyed like the existing rows.
- Add a coverage floor: each generated body must contain at least one ELEMENT, because a hollow body (see OPT
  fixes) passes both validators vacuously. PROBE-086's harness applies the same floor to its fixtures.
- `templates/social` is already in the census's compile-failure allowlist (`ParseFile` refuses its
  `<OPERATIONAL_TEMPLATE>` root), so it reaches neither check until Phase 4 replaces it.

The ENTRY `language` and `encoding` assertion is already covered: the census's placeholder walk reports an
example language code.

**Definition of done:** `make ci` passes with the ratchet in place.

### Phase 4 — Code24 OPTs

**Status:** done. `social.opt` was converted in place, so `ParseFile` accepts it, the census drops its compile-failure allowlist row, and a generated body contains ELEMENTs.

**Decisions (declined; the parser is unchanged):**
- Strict mode does not grow checks for a missing language with `original_language` in its place, a top-level archetype id, or empty description fields.
- The lenient parse stays silent when it admits an unknown child type as a leaf and drops the nested subtree.

**Definition of done:** the decisions are recorded. Neither answer adds a refusal, so there is no new refusal test.

## Mapping to specs

- [clinical-modeling.md § REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator): the generator contract.
- [clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor): the RM floor.
- [research-strands.md § STRAND-14](../specifications/research-strands.md#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants): template-driven validation and the floor.
- [conformance.md § PROBE-027](../specifications/conformance.md#probe-027--generated-instance-validates-clean): the generator and validator probe.
- [REQ.md](../specifications/REQ.md): the registry rows.
