---
kind: analysis
---

# Cross-check of generated and validated examples against EHRbase and FerroEHR

This note records a live cross-check of the SDK's example generator ([§ REQ-107](../../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator)) and its template validator ([§ REQ-102](../../specifications/clinical-modeling.md#req-102--composition-validation)) against two openEHR CDRs, run on 2026-10-24. It is the evidence behind two plans: [the generator plan](../2026-10-24-generator-cdr-crosscheck.md) and [the validator and parsers plan](../2026-10-24-validator-parsers-cdr-crosscheck.md). CDR behaviour is described as observed, with the specification text that decides each case.

## Method

- **SDK:** `main` at `9e29377a`.
- **CDRs:** EHRbase 2.36.0 (openEHR_SDK 2.35.0, Archie 3.13.0) and FerroEHR 5.0.0 (image `sha-947bc4f`), both local, both on ITS-REST 1.1.0.
- **Templates:** 202 operational templates, deduplicated by `template_id` (the first copy kept where two files share an id with different content).
  - The 67 OPTs vendored under `testkit/corpus`.
  - 135 public OPTs from the CaboLabs openEHR-SDK test set, the openEHR conformance-verification templates, openEHR Discourse threads and the freshEHR team's repositories.
  - 183 are rooted at COMPOSITION. The others are PERSON, ROLE, AGENT, GROUP, ORGANISATION, ADDRESS, PARTY_RELATIONSHIP, EHR_STATUS, FOLDER, SECTION, OBSERVATION and EVALUATION roots.
- **Upload:** each OPT to each CDR. Three templates a CDR already held in an older form were uploaded again under a suffixed id, so both CDRs and the SDK read the same content.
- **Generation:** `instance.Generate` under `Required`, `Medium` and `Large`, each with `ExampleFill` and with `RandomFill` on two seeds: 1,818 runs. Each output was checked with `validation.Validate` and `validation.ValidateRM`.
- **Posting:** every COMPOSITION posted to both CDRs, each to a fresh EHR (`POST /ehr/{id}/composition`).
- **CDR examples:** each CDR's own template example fetched (`GET /definition/template/adl1.4/{id}/example`). EHRbase builds one level and takes no `detail_level`. FerroEHR builds `required`, `medium` and `complete`.
  - Each example was validated with the SDK, and posted to both CDRs with its `uid` removed.

## Results

| Measure | EHRbase | FerroEHR |
|---|---|---|
| Templates accepted at upload | 199 / 202 | 180 / 202 |
| SDK documents accepted | 1,475 / 1,596 | 1,410 / 1,509 |
| CDR's own examples fetched | 176 / 199 | 510 / 540 |
| SDK validator: CDR examples valid | 106 / 176 | 459 / 510 |
| CDR accepts its own examples | 162 / 176 | 488 / 510 |
| CDR accepts the other CDR's examples | 479 / 507 (FerroEHR's) | 92 / 165 (EHRbase's) |

Of the SDK documents the SDK itself accepts:
- 55 were refused by both CDRs;
- 39 by EHRbase alone;
- 44 by FerroEHR alone.

Generation failed in 24 of 1,818 runs (5 templates). 21 outputs fail the SDK's own validator (3 templates, each an OPT defect, see *Template defects*).

## Generator findings

Each row was confirmed by a modified document that both CDRs then accepted, unless the row says otherwise.

| # | What happens | Templates | Cause in the SDK | Plan |
|---|---|---|---|---|
| G1 | Under `Required`, a list the OPT leaves optional is left out although its child has occurrences lower 1 or more. Both CDRs refuse: EHRbase "Attribute has 0 occurrences, but must be 1..1", FerroEHR "mandatory node is missing (occurrences 1..)". | 10: Pulse, family_history (two copies), Patientenaufenthalt, Stationärer Versorgungsfall, Documento de cita instruccion, Cytokine release syndrome monitoring, EHRN-ABDM-OPConsultRecord.v2.0, composition_evaluation_test, section_admin_test | § REQ-107 *Visit rule*: an attribute is required by the BMM, its existence or its cardinality, never by a child's occurrences (`visitRequired`, `openehr/instance/generate.go:478`) | generator Phase 1 |
| G2 | Under `Required`, an optional container whose only demand is cardinality lower 1 or more is still created and topped up (COMPOSITION `content` 0..1 with cardinality 1..*). FerroEHR's `required` omits it, and both CDRs accept that. | 52 | the same sentence counts cardinality without existence | generator Phase 1 |
| G3 | Every DV_DURATION is `P0D`, whatever the OPT's `C_DURATION.range` or pattern. Both CDRs refuse it where the range excludes zero (`width` PT24H..PT24H, `P4Y..`). | New Template, a1, ehrn_vital_signs.v2, constrain_test; 12 pattern mismatches (PTHMS, PTM, PW, PY, PYM) that no CDR checks | the parser drops `range` (`openehr/template/parse_primitives.go:262`); `CDuration.ExampleValue` returns `P0D` (`openehr/template/constraints/temporal.go:124`); the sampler has no duration case (`openehr/instance/sample.go:160`) | validator Phase 1, generator Phase 4 |
| G4 | Under `Required`, a HISTORY gets neither `events` nor `summary`, which breaks RM `Events_valid`. Both CDRs refuse. | 10_alternative_types.en.v1, PROMIS-29 (OBSERVATION root, not posted) | *Precedence* item 2's "Present" list and `writeRMNeeded` (`generate.go:268`) leave it out | generator Phase 2 |
| G5 | A DV_INTERVAL<DV_QUANTITY> gets bounds in different units (`kPa` and `1`), which RM `Limits_consistent` forbids (`is_strictly_comparable_to`). FerroEHR refuses every fill; EHRbase refuses only random fills, since it compares magnitudes across units. | test_intervals.en.v1 | an unconstrained side gets the unit `1` (`openehr/template/constraints/quantity.go:45`); the ordering leaves pairs it cannot compare (`openehr/instance/interval_order.go:425`) | generator Phase 2 |
| G6 | A coded text keeps the placeholder text `example` where the OPT defines the term under the bare code (`term_definitions code="planned"`). EHRbase refuses: "DV_CODED_TEXT/value does not match. expected: Planned". | Stationärer Versorgungsfall, Patientenaufenthalt, COVID-19 Pneumonia Diagnosis and Treatment; 17 OPTs define terms this way | `termKey` reads only `<terminology>::<code>` (`openehr/instance/coded_text.go:107`), as *Placeholders* says | generator Phase 4 |
| G7 | A coded `name` whose text a C_STRING pins keeps the pinned text but takes the first listed code, whose term text differs. EHRbase refuses. | ePrescription (FHIR) | the term-text rule never changes the code | generator Phase 4 |
| G8 | A DV_QUANTITY constrained by `property` alone gets the unit `1`, which openEHR's property-unit table does not list for that property (Pressure). FerroEHR refuses; the specification's own conformance test schedule (`CONT-DV_QUANTITY-validate_property`) expects a refusal. | Test restricciones vacias, test empty constraints | `DvQuantity.ExampleValue` ignores `Property` (`quantity.go:45-47`) | validator Phase 1, generator Phase 4 |
| G9 | A DV_DATE with pattern `yyyy-??-XX` (ADL 1.4: month optional, day not allowed) gets `2020-01-01`. FerroEHR refuses. | test_all_types.en.v1, Test_all_types_v2 | patterns are out of scope for REQ-103 | validator Phase 1, generator Phase 4 |
| G10 | `Generate` fails on an OPT that constrains an attribute the RM class does not have: `ISM_TRANSITION.name` (5 OPTs) and `ELEMENT.null_flavor` (2). | GECCO core, RIPPLE - Conformance Test template | only `rminfo.IsNonStorableAttr` is skipped (`walkSkips`, `generate.go:345`) | generator Phase 3 |
| G11 | `Generate` fails under `Medium` and `Large` where the OPT puts an object under `feeder_audit`, an attribute it has no writer for, although the attribute is optional. | genericFHIRTemplate | *Exceptions and known gaps* states this as intended | generator Phase 3 |
| G12 | An OPT whose children's occurrence lower bounds add up past its cardinality upper bound gets a root that fails the SDK's own validator, where an error would say why. | cardinality_of_section (content 1..1, lower bounds summing to 8) | *Errors* covers primitive narrowings only | generator Phase 2 |
| G13 | `ACTIVITY.action_archetype_id` gets `example` where the OPT gives no pattern; the RM default is `/.*/`, which both CDRs write. Where the OPT gives a pattern, the SDK writes a matching id and both CDRs write `/.*/`, which does not match it. | 16 activities under `Large` | *RM defaults* has no entry for it | generator Phase 4 |

Coverage differences, measured over 162 templates that all three generators handle:

| Generator and level | Archetype roots written | ELEMENTs written |
|---|---|---|
| SDK `Required` | 9% | 0% |
| SDK `Medium` | 34% | 42% |
| SDK `Large` | 64% | 73% |
| FerroEHR `medium` (and `complete`) | 98% | 97% |
| EHRbase (single level) | 100% | 100% |

- **Named clones** (one archetype under several template names) account for most of the gap. `Large` writes only the first of them (§ REQ-107 *What a visit writes*). Example: SDK `Large` writes 2 of 9 content members of the Aboriginal and Torres Strait Islander health check.
- **Empty ITEM_TREEs.** The SDK writes them at `protocol`, `state` and `other_context`: 124, 52 and 37 under `Large`. Neither CDR writes any.
- **ENTRY uid.** The SDK writes a `uid` on every ENTRY. Neither CDR does, and the RM's Common IM says a locatable's `uid` "will usually be empty" below the top-level object (`specifications-RM/docs/common/master03-archetyped_package.adoc:39-41`).

## Validator and parser findings

Classified over all 722 issues the SDK reported on CDR examples.

| # | What happens | Count | Cause | Plan |
|---|---|---|---|---|
| V1 | A C_STRING list with `list_open` true is treated as closed. | 190 issues, 14 documents | the flag is not parsed (`parse_primitives.go:249-260`); REQ-103 lists it as out of scope | validator Phase 1 |
| V2 | A value that matches a later alternative of a single-valued attribute is judged against the first alternative of the same RM type. In this corpus: ISM_TRANSITION alternatives of an ACTION. | 37 issues, 18 documents (7 carry nothing else) | § REQ-102 *Structural, alternatives*: "the first child in OPT order wins" within a tier (`rmbind.Bind`, `openehr/internal/rmbind`) | validator Phase 3 |
| V3 | An OPT attribute the RM class does not have is reported as a missing required attribute (`ISM_TRANSITION.name`), which no instance can carry. | 39 issues | `declaredAttr` skips only non-storable attributes (`openehr/validation/engine.go:715-728`) | validator Phase 2 |
| V4 | An OPT whose `xsi` prefix is bound to a misspelt namespace is read without error: every untyped node becomes a complex object or a single attribute, so constraints vanish. The SDK then reports 9 false findings and misses a category outside the OPT's list. | Case (and a second copy) | `xsi:type` is matched by namespace only, and an empty type takes a default even in strict mode (`openehr/template/parse.go:245,321,414,574`) | validator Phase 2 |
| V5 | Two siblings that share an archetype id and a pinned name both bind to the first, so a valid second member is reported over its occurrences. | 1 issue (SSIAD PRIeSM); same shape in 3 more OPTs | binding is stateless per item (`matchChildByPinnedName`, `openehr/validation/walk_composition.go:542-563`) | validator Phase 3 |
| V6 | An absent list whose mandatory child is missing passes, while the same list present and empty fails; the wire cannot tell the two apart (canonical JSON drops an empty list). | the 10 G1 templates | § REQ-102 *An existence lower bound alone asks for no member* and the absent-list sentence before it (`engine.go:953`) | validator Phase 4 |
| V7 | A list the OPT requires (existence 1..1) is accepted absent or empty where an RM invariant says it is non-empty when present (`EVENT_CONTEXT.Participations_validity`). FerroEHR refuses. | Referral | the same *existence lower bound alone* sentence, and the RM floor does not evaluate the invariant (`openehr/validation/invariants.json`) | validator Phase 4 |
| V8 | DV_INTERVAL bounds that are not strictly comparable (different DV_QUANTITY units) are not reported. FerroEHR refuses its own such example. | test_intervals.en.v1 | the floor orders comparable pairs only; `invariants.json` marks `Limits_consistent` evaluated | validator Phase 5 |
| V9 | Known out-of-scope REQ-103 checks show up as missed refusals: `C_DURATION.range` and patterns (G3), date patterns (G9) and `C_DV_QUANTITY.property` (G8). | see G3, G8, G9 | REQ-103's out-of-scope list | validator Phase 1 |
| V10 | HISTORY `Events_valid` is not evaluated (G4). | 2 templates | `invariants.json` marks it "Not yet evaluated" | validator Phase 4 |

**Where the SDK validator is right and both CDRs accept** (the ordinals verified by script):
- **DV_ORDINAL symbols.** EHRbase's examples code them in terminology `external` where the OPT lists `local` codes: 152 issues in 25 documents.
- **DV_PROPORTION type.** EHRbase's examples always write type 3, outside the OPT's list in 90 issues. FerroEHR refuses these.
- **Coded name options.** EHRbase writes one ELEMENT per coded-name option, past the node's occurrences (41 issues). FerroEHR refuses these too.
- **`action_archetype_id`.** Both CDRs write `/.*/` against an OPT pattern.
- **Precision.** Both CDRs leave out a DV_PROPORTION `precision` that the OPT requires.
- **Hardcoded values.** EHRbase writes `video/H261`, `text/html` and `dev/null` against closed lists and patterns.

## Spec sentences found wrong or underspecified

These are the sentences the plans amend through `/sdd-specify`, each against the specification text that decides it.

| Sentence | Finding | Deciding text |
|---|---|---|
| § REQ-107 *Visit rule*, the definition of a required attribute | Counts cardinality without existence (G2), and never counts a list child's occurrences (G1). | ADL 1.4 `master05-cadl.adoc` (specifications-AM): `:275-280` existence and cardinality ("optional, but if present"), `:284` and `:316` occurrences ("mandatory in the GROUP"; a single attribute's child is already decided by its existence). AOM 2 `master04.5-constraint_model-class_definitions.adoc:159` (VACMCO, "mandatory child object"). AOM 1.4 `master04-constraint_model_package.adoc:33`. |
| § REQ-102, the absent-list sentence and *An existence lower bound alone asks for no member* | An absent list escapes its mandatory children's occurrences (V6), and an existence-required list escapes the RM's non-empty invariants (V7). | As above, plus RM `EVENT_CONTEXT.Participations_validity`, `COMPOSITION.Content_valid`, `INSTRUCTION.Activities_valid`, `SECTION.Items_valid`, `HISTORY.Events_valid` (`resources/bmm/openehr_rm_1.2.0.bmm.json`). |
| § REQ-102 *Structural, alternatives*, "Within a tier the first child in OPT order wins" | A value is judged against one alternative only (V2). | ADL 1.4 `master05-cadl.adoc:237` (alternatives, "only one of which needs to be matched by the data"); AOM 1.4 C_SINGLE_ATTRIBUTE `alternatives`. Archie accepts a value as soon as one alternative validates. |
| § REQ-102 *Structural, binding items* | Siblings that share identity and name bind statelessly (V5). | ADL 1.4 occurrences per object node. |
| § REQ-103 out-of-scope list (`list_open`, `C_DURATION.range`, temporal patterns) and the `Property` note | These four checks are left out (V1, V9). The calendar-conversion reason given for durations no longer holds: BASE defines `Iso8601_duration.to_seconds` and the SDK implements it as `DVDuration.Magnitude`. | AOM 1.4 C_STRING `list_open`, C_DURATION `range`; ADL 1.4 `master05-cadl.adoc:852-950` (date, time and duration patterns); openEHR terminology `PropertyUnitData.xml` (specifications-TERM). |
| § REQ-107 *Placeholders* (term text under `<terminology>::<code>` only) | Misses terms defined under the bare code (G6), and lets a pinned name text and its code disagree (G7). | RM DV_TEXT `value`: for a DV_CODED_TEXT, "the rubric of the complete term". |
| § REQ-107 *Precedence* item 2, the closed "Present" list | Omits HISTORY `Events_valid` (G4). | RM HISTORY `Events_valid`. |
| § REQ-107 *Intervals* | Lets bounds that cannot be compared stand (G5). | RM DV_INTERVAL `Limits_consistent` (`is_strictly_comparable_to`). |
| § REQ-107 *Exceptions and known gaps*: attributes the RM does not have, and the writer gap | Says such a root fails the validator and that no vendored OPT constrains one; in fact `Generate` returns an error, and public OPTs do (G10, G11). | AOM 2 VCARM (`master04.5:126`). |
| § REQ-107 *Identity* | Writes a uid on every ENTRY (coverage note). | RM Common IM `master03-archetyped_package.adoc:39-41`. |
| § REQ-107 *What a visit writes*, named siblings under `Large` | Writes only the first named clone (coverage note). | ITS-REST `detail_level` (`resources/its-rest/definition-validation.openapi.yaml:236-238`). |
| § REQ-100 | Says nothing of an `xsi:type` the parser cannot resolve (V4). | ITS-XML AM 1.4 `Archetype.xsd:32,43`: C_ATTRIBUTE and C_OBJECT are abstract. |

## Template defects

These OPTs cannot yield a document every reader accepts. The SDK's handling of each is given beside it.

| Template | Defect | SDK |
|---|---|---|
| cardinality_of_section | content cardinality 1..1 against children whose occurrence lower bounds sum to 8 (ADL 1.4 VCOC, AOM 2 VACMCO) | writes one member; generator Phase 2 makes it an error |
| section_admin_test | an ADMIN_ENTRY defined inline, against ENTRY `Is_archetype_root` | `ErrArchetypeIDMissing` under `Medium` and `Large`, as § REQ-107 states |
| Referral | `PARTICIPATION.function` coded in `local`, against `Function_valid` | writes it and reports it, as *Exceptions* states |
| TestPerson.v2 | `media_type` in legacy openEHR codes 425-429, against `Media_type_valid` | as *Precedence* item 1 states |
| GECCO core, Antenatal - first visit, NCD - first visit, Aboriginal ... (two copies) | `ISM_TRANSITION.name` constrained 1..1 | G10, V3 |
| RIPPLE - Conformance Test template | `null_flavor` spelt as in US English | G10 |
| Case and a second copy | `xmlns:xsi` bound to `http //www.w3.org/2001/XMLSchema-instance` | V4 |

## CDR observations

Neither CDR gives the same verdict as the other on every document, and no single reading of the specifications matches both.

**EHRbase 2.36.0**
- Template handling:
  - It refuses an OPT that starts with a UTF-8 byte-order mark ("Content is not allowed in prolog"); 61 of the 202 do, 8 of them vendored. The SDK's own upload strips the mark, and so did this run.
  - It refuses an OPT with an ITEM_TABLE ("unsupported types: ITEM_TABLE", the vendored clinical_content_validation), one with an empty description, and answers 500 to one more (NDS - SBAR_Care_Home.v0.0).
  - It collapses repeated spaces in a `template_id` (`IDCR -  Adverse Reaction List.v1` is stored and looked up with one space), as the ITS-XML `xs:token` type of OBJECT_ID's value implies. FerroEHR keeps the literal id.
- Its template example:
  - It answers 500 for every template whose root is not a COMPOSITION.
  - It writes DV_ORDINAL symbols and unconstrained coded texts in terminology `external`, a DV_PROPORTION of type 3 whatever the OPT allows, and `EVALUATION.time_asserted`, which RM 1.2.0 does not define.
  - It writes one ELEMENT per option of a coded name, an INTERVAL_EVENT and POINT_EVENT mix for an abstract EVENT, and fixed values (`video/H261`, `text/html`, `dev/null`, `Lorem ipsum`).
  - It refuses 14 of its own examples, among them for an "Observation contains no results" check, which no RM invariant states, for interval bounds outside the OPT's range, for `Is_archetypeRoot` and for the `null_flavour` reading below.
- Its composition validation:
  - Where an ELEMENT's `value` or `null_flavour` has more than one alternative, it enforces the alternatives' occurrences although the attribute's existence is 0..1. So it asks for a `null_flavour` beside a `value` (Demonstration.v1, Review), which ELEMENT `Inv_null_flavour_indicated` forbids, and for a `value` where the SDK writes a `null_flavour` (GECCO_Diagnose).
  - It checks a coded text's `value` against its term text, the bare-code key included.
  - It checks ISM transitions as whole alternatives (current state with careflow step).
  - It does not check an ordinal symbol's terminology, a C_STRING pattern on `action_archetype_id`, temporal patterns or a quantity's property.

**FerroEHR 5.0.0**
- Template handling:
  - It refuses 22 OPTs at upload: existence looser than the RM's (AOM 2 VCAEX, 11), an ordinal symbol with no `value` element (ITS-XML schema, 6), an incomplete `default_value` (2), a CODE_PHRASE with no `terminology_id` (1), an empty `lifecycle_state` (1) and an unsatisfiable cardinality (VACMCO, 1). EHRbase accepts all but two of these, for reasons of its own, and the SDK all of them; the SDK then applies the RM's existence to the data.
  - It reads `null_flavor` as `null_flavour`, and an unresolvable `xsi` prefix by its letters.
  - It answers 400 with a clear message for a template whose root is not a COMPOSITION.
- Its template example:
  - Its levels nest exactly (`required` within `medium` within `complete`).
  - It refuses 22 of its own examples, among them for interval bounds in different units, sibling minimums, `participations`, `Is_archetype_root`, and `items` it requires on an ITEM_LIST or ITEM_TREE that the OPT leaves without a child at existence 1..1 (the RM and AOM 1.4 have no such rule).
- Its composition validation:
  - It refuses a DV_TEXT where the OPT allows DV_TEXT or DV_CODED_TEXT, since its web template folds the pair into one coded node.
  - It accepts an ISM transition that matches no alternative as a whole.
  - It enforces property units, temporal patterns and duration ranges.
  - It allows one create per EHR for a persistent template.

## Backlog leads checked

- **Settled by this evidence** (the plans close them):
  - the `Required` visit of an optional attribute with a mandatory child (`generate.go:466`). Its Demonstration at0016 clause is EHRbase's single-attribute reading, not the same cause.
  - the shared quantity unit for interval bounds (`interval_order.go:1131`, extended by G5);
  - siblings that share an identity binding to the first (`walk_composition.go:515`, with V5).
- **Unchanged:** the top-up past a child's occurrences upper bound; `PERSON.identities`; untyped first children in `NewSkeleton`; slot occurrences; the GTS formalism; DV_PROPORTION `is_integral`. No template in this run reaches them.

## Reproducing

The harness is a local tool, not part of the repository. It is a Go program that calls `instance.Generate`, `validation.Validate` and `canjson`, and a driver that uploads, posts and fetches over ITS-REST with HTTP Basic. The generator plan's Phase 6 asks whether to land it as an opt-in make target. The CDRs ran from the compose files of each product. A rerun needs the same template set and fresh EHRs per document. The generation clock is fixed, so outputs compare across runs.
