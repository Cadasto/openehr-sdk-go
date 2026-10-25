---
kind: analysis
---

# Cross-check of generated and validated examples against EHRbase and FerroEHR

This note records two live cross-checks of the SDK's example generator ([§ REQ-107](../../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator)) and its template validator ([§ REQ-102](../../specifications/clinical-modeling.md#req-102--composition-validation)) against two openEHR CDRs. Run 1, on 2026-10-24, found the defects listed below. Run 2, on 2026-10-25, measured the SDK once they were fixed. Each finding names the specification section that now states the rule. CDR behaviour is described as observed, with the specification text that decides each case. Run 2 used the repository's cross-check tool, [`cmd/cdrcheck`](../../specifications/conformance.md#the-cdr-cross-check-tool) (see *Reproducing*).

## Method

- **SDK:** run 1 on `main` at `9e29377a`; run 2 at `c238324f`, with every fix below.
- **CDRs:** EHRbase 2.36.0 (openEHR_SDK 2.35.0, Archie 3.13.0) and FerroEHR 5.0.0 (image `sha-947bc4f`), both local, both on ITS-REST 1.1.0, in both runs.
- **Templates:** 202 operational templates in run 1, deduplicated by `template_id` (the first copy kept where two files share an id with different content).
  - The 67 templates then vendored under `testkit/corpus`.
  - 135 public OPTs from the CaboLabs openEHR-SDK test set, the openEHR conformance-verification templates, openEHR Discourse threads and the freshEHR team's repositories.
  - 183 are rooted at COMPOSITION. The others are PERSON, ROLE, AGENT, GROUP, ORGANISATION, ADDRESS, PARTY_RELATIONSHIP, EHR_STATUS, FOLDER, SECTION, OBSERVATION and EVALUATION roots.
  - Run 2 adds five synthetic OPTs, written for the shapes the findings fixed and vendored with them ([`testkit/corpus/README.md`](../../../testkit/corpus/README.md)): `sdk_crosscheck_values.v1`, `sdk_crosscheck_structure.v1`, `sdk_crosscheck_unwritten.v1`, `sdk_crosscheck_siblings.v1` and `sdk_crosscheck_padded.v1`. That makes 207 templates, 188 of them rooted at COMPOSITION. `testkit/corpus` now holds 72 templates in 74 `.opt` files: one file repeats a template id with other content, and one carries no template id.
- **Upload:** each OPT to each CDR. Three templates a CDR already held in an older form were uploaded again under the id plus `.xc1`, so both CDRs and the SDK read the same content.
- **Generation:** `instance.Generate` under `Required`, `Medium` and `Large`, each with `ExampleFill` and with `RandomFill` on two seeds, on a fixed clock: 1,818 runs in run 1, 1,863 in run 2. Each output was checked with `validation.Validate` and `validation.ValidateRM`. Run 2 parses every OPT in strict mode.
- **Posting:** every COMPOSITION posted to both CDRs, each to an EHR of its own (`POST /ehr/{id}/composition`).
- **CDR examples:** each CDR's own template example fetched (`GET /definition/template/adl1.4/{id}/example`). EHRbase builds one level and takes no `detail_level`. FerroEHR builds `required`, `medium` and `complete`.
  - Each example was validated with the SDK, and posted to both CDRs with its `uid` removed.

## Results

| Measure | EHRbase, run 1 | EHRbase, run 2 | FerroEHR, run 1 | FerroEHR, run 2 |
|---|---|---|---|---|
| Templates accepted at upload | 199 / 202 | 204 / 207 | 180 / 202 | 184 / 207 |
| SDK documents accepted | 1,475 / 1,596 | 1,578 / 1,629 | 1,410 / 1,509 | 1,503 / 1,539 |
| CDR's own examples fetched | 176 / 199 | 179 / 204 | 510 / 540 | 522 / 552 |
| SDK validator: CDR examples valid | 106 / 176 | 104 / 179 | 459 / 510 | 465 / 522 |
| CDR accepts its own examples | 162 / 176 | 162 / 179 | 488 / 510 | 495 / 522 |
| CDR accepts the other CDR's examples | 479 / 507 (FerroEHR's) | 483 / 519 (FerroEHR's) | 92 / 165 (EHRbase's) | 92 / 168 (EHRbase's) |

| Measure | Run 1 | Run 2 |
|---|---|---|
| SDK documents the SDK accepts that both CDRs refused | 55 | 6 |
| ... that EHRbase alone refused, FerroEHR accepting them | 39 | 30 |
| ... that FerroEHR alone refused, EHRbase accepting them | 44 | 30 |
| Generation runs that failed | 24 of 1,818, in 5 templates | 36 of 1,863, in 4 templates |
| Outputs the SDK's own validators refuse | 21, in 3 templates | 12, in 2 templates |
| SDK validator issues on the CDRs' examples | 722 | 583 |

**Run 1.** EHRbase's other 27 refusals were 9 documents of templates FerroEHR refused at upload, and the 18 COMPOSITIONs the SDK itself rejected (Referral and cardinality_of_section). Generation failed in Case, GECCO core, RIPPLE - Conformance Test template, genericFHIRTemplate and section_admin_test. The four other OPTs that constrain `ISM_TRANSITION.name` never built the ACTION that holds it. The 21 self-invalid outputs came from 3 templates, each an OPT defect (see *Template defects*).

**Run 2.**
- EHRbase also refused 6 documents of the two templates FerroEHR refuses at upload (Life style, SSIAD PRIeSM), and the 9 Referral documents the SDK's own validators refuse.
- Generation failed under every policy and fill in 4 templates (see *Template defects*):
  - Case and duration constraints, which strict parsing refuses;
  - cardinality_of_section, for which `Generate` returns `ErrConstraintUnsatisfiable` (G12);
  - section_admin_test, for which `Generate` returns `ErrArchetypeIDMissing`.
- GECCO core, RIPPLE - Conformance Test template and genericFHIRTemplate now generate under every policy.
- The 12 self-invalid outputs come from Referral (9) and TestPerson.v2 (3), each an OPT defect.
- The SDK validator's issues on the CDRs' examples fell from 722 to 583. The false findings went (V1 to V5). The checks added since report true findings, among them 41 temporal-pattern and time-zone findings on EHRbase's examples. The count of valid EHRbase examples moved from 106 to 104.
- **The five synthetic OPTs at upload.** EHRbase holds all five, each with the same signature as the file. FerroEHR holds four and refuses `sdk_crosscheck_padded.v1` (see *CDR observations*).

## What each CDR still refuses in run 2

Every SDK document a CDR refuses in run 2 falls in one of these classes. Each class is a template defect, a template whose sibling uses cannot be told apart, one CDR's own reading, or, in one case, the SDK's own reading of a template id.

| Class | Documents refused | Templates | Reading |
|---|---|---|---|
| A composition names a `template_id` with two spaces in a row | EHRbase 9 | IDCR -  Adverse Reaction List.v1 | the SDK's own, which FerroEHR shares: both keep the id as written, while ITS-XML types OBJECT_ID's `value` as a `xs:token` (`NonEmptyToken`, `specifications-ITS-XML/components/RM/Release-1.0.2/BaseTypes.xsd:70-71,92`), whose value has the spaces collapsed, as EHRbase stores it ([§ REQ-100](../../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths) trims only leading and trailing white space; see *CDR observations*) |
| EHRbase asks for a `null_flavour` beside a `value` | EHRbase 12, under `Medium` and `Large` | Demonstration.v1, Review | EHRbase's own; ELEMENT `Inv_null_flavour_indicated` forbids both |
| EHRbase asks for the misspelt `null_flavor` beside a `value` | EHRbase 6, under `Large` | RIPPLE - Conformance Test template, sdk_crosscheck_unwritten.v1 | template defect (`null_flavor`), read as the row above; the SDK reads the attribute as `null_flavour` ([§ REQ-100](../../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths)) |
| A coded name pins a text that no listed code's term carries | EHRbase 3, under `Large` | ePrescription (FHIR) | template defect (see *Template defects*) |
| Sibling uses of one archetype or node id share one path | EHRbase 12, FerroEHR 3 | Life style, SSIAD PRIeSM, sdk_crosscheck_siblings.v1 | the OPT leaves the uses indistinguishable, and each CDR binds them its own way (below) |
| FerroEHR asks for a DV_CODED_TEXT where the OPT allows DV_TEXT or DV_CODED_TEXT | FerroEHR 27, under `Medium` and `Large` | GECCO_Diagnose, Molecular Pathology Report, Prescripcion de Medicamentos, ePrescription (FHIR), Aboriginal and Torres Strait Islander health check - MASTER | FerroEHR's own: its web template folds the pair into one coded node |
| FerroEHR asks for `items` on an item structure the OPT requires but gives only slots or no child | FerroEHR 6, under `Medium` and `Large` | vital_signs | FerroEHR's own; the RM and AOM 1.4 have no such rule |
| `PARTICIPATION.function` coded in `local` | EHRbase 9 | Referral | template defect; the SDK's validators refuse these documents too |

**Sibling uses that share a path.** Under `Large` the SDK writes a member for every use of an archetype or node id ([§ REQ-107](../../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) *What a visit writes*). Where two sibling uses carry one archetype id or node id and the same pinned name, or none, a document cannot say which use a member belongs to.
- Life style has seven uses of one ADMIN_ENTRY archetype under `content` that pin no name, each holding an ELEMENT with a name of its own.
- SSIAD PRIeSM has two uses of one OBSERVATION archetype that pin the same name.
- sdk_crosscheck_siblings.v1 has two unnamed uses of one OBSERVATION archetype, and two ELEMENTs of one node id.

ADL 1.4 asks for node identifiers that keep the paths of sibling nodes distinct (`specifications-AM/docs/ADL1.4/master05-cadl.adoc:358,372`; AOM 1.4 `master04-constraint_model_package.adoc:41`). EHRbase binds every member to the first use. It reports the other uses' ELEMENT names as not in the template (Life style), and counts both members against the first use's occurrences (SSIAD PRIeSM). It answers 500 to every `Medium` and `Large` document of sdk_crosscheck_siblings.v1, though a `Medium` document holds one member of each, and to both CDRs' examples of it. FerroEHR counts every member against each use (sdk_crosscheck_siblings.v1, `Large`); it refuses the other two templates at upload. The SDK's validator binds each member to the first use with room under whose constraints it validates ([§ REQ-102](../../specifications/clinical-modeling.md#req-102--composition-validation) *Structural, binding items*).

## Generator findings

In run 1, each row was confirmed by a modified document that both CDRs then accepted, unless the row says otherwise. The cause names the code at `9e29377a`.

| # | What happened in run 1 | Templates | Cause at `9e29377a` | Settled by | Run 2 |
|---|---|---|---|---|---|
| G1 | Under `Required`, a list the OPT leaves optional is left out although its child has occurrences lower 1 or more. Both CDRs refuse: EHRbase "Attribute has 0 occurrences, but must be 1..1", FerroEHR "mandatory node is missing (occurrences 1..)". | 10: Pulse, family_history (two copies), Patientenaufenthalt, Stationärer Versorgungsfall, Documento de cita instruccion, Cytokine release syndrome monitoring, EHRN-ABDM-OPConsultRecord.v2.0, composition_evaluation_test, section_admin_test | an attribute was required by the BMM, its existence or its cardinality, never by a child's occurrences (`visitRequired`, `openehr/instance/generate.go:478`) | § REQ-107 *Visit rule*: a multi-valued attribute is required when an allowed child that is not a slot has occurrences lower 1 or more | Settled. Both CDRs accept the `Required` documents of the nine templates that generate. section_admin_test is a template defect. |
| G2 | Under `Required`, an optional container whose only demand is cardinality lower 1 or more is still created and topped up (COMPOSITION `content` 0..1 with cardinality 1..*). FerroEHR's `required` omits it, and both CDRs accept that. | 52 | the same sentence counted cardinality without existence | § REQ-107 *Visit rule*: a cardinality lower bound counts members only once the attribute is visited | Settled. `Required` documents name 2% of the archetype roots, against 9%. |
| G3 | Every DV_DURATION is `P0D`, whatever the OPT's `C_DURATION.range` or pattern. Both CDRs refuse it where the range excludes zero (`width` PT24H..PT24H, `P4Y..`). | New Template, a1, ehrn_vital_signs.v2, constrain_test; 12 pattern mismatches (PTHMS, PTM, PW, PY, PYM) that neither CDR refused (FerroEHR enforces date and time patterns, G9, not duration patterns) | the parser dropped `range` (`openehr/template/parse_primitives.go:262`); `CDuration.ExampleValue` returned `P0D` (`openehr/template/constraints/temporal.go:124`); the sampler had no duration case (`openehr/instance/sample.go:160`) | [§ REQ-103](../../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection) *Validate contract*: `CDuration` carries its range and pattern | Settled. Both CDRs accept New Template and a1. EHRbase accepts ehrn_vital_signs.v2 and constrain_test, which FerroEHR refuses at upload. |
| G4 | Under `Required`, a HISTORY gets neither `events` nor `summary`, which breaks RM `Events_valid`. Both CDRs refuse. | 10_alternative_types.en.v1, PROMIS-29 (OBSERVATION root, not posted) | *Precedence* item 2's "Present" list and `writeRMNeeded` (`generate.go:268`) left it out | § REQ-107 [*Precedence*](../../specifications/clinical-modeling.md#precedence) item 2 | Settled. Both CDRs accept 10_alternative_types.en.v1 under every fill; PROMIS-29 passes both SDK validators. |
| G5 | A DV_INTERVAL<DV_QUANTITY> gets bounds in different units (`kPa` and `1`), which RM `Limits_consistent` forbids (`is_strictly_comparable_to`). FerroEHR refuses every fill; EHRbase refuses only random fills, since it compares magnitudes across units. | test_intervals.en.v1 | an unconstrained side got the unit `1` (`openehr/template/constraints/quantity.go:45`); the ordering left pairs it could not compare (`openehr/instance/interval_order.go:425`) | § REQ-107 *Intervals* | Settled. Both CDRs accept every fill. |
| G6 | A coded text keeps the placeholder text `example` where the OPT defines the term under the bare code (`term_definitions code="planned"`). EHRbase refuses: "DV_CODED_TEXT/value does not match. expected: Planned". | Stationärer Versorgungsfall, Patientenaufenthalt, COVID-19 Pneumonia Diagnosis and Treatment; 17 OPTs define terms this way | `termKey` read a code of another terminology only under `<terminology>::<code>` (`openehr/instance/coded_text.go:107-113`) | § REQ-107, the term-text paragraph after *RM defaults* | Settled. Both CDRs accept the three templates. |
| G7 | A coded `name` whose text a C_STRING pins keeps the pinned text but takes the first listed code, whose term text differs. EHRbase refuses. | ePrescription (FHIR) | the term-text rule never changed the code | the same paragraph: the first listed code whose term the C_STRING admits | Settled for the use run 1 found. `Large` now writes a second use whose pinned text no listed code carries, a template defect. |
| G8 | A DV_QUANTITY constrained by `property` alone gets the unit `1`, which openEHR's property-unit table does not list for that property (Pressure). FerroEHR refuses; the specification's own conformance test schedule (`CONT-DV_QUANTITY-validate_property`) expects a refusal. | Test restricciones vacias, test empty constraints | `DvQuantity.ExampleValue` ignored `Property` (`quantity.go:45-47`) | § REQ-103 *Validate contract*: `DvQuantity` and the property-unit table | Settled. Both CDRs accept both templates. |
| G9 | A DV_DATE with pattern `yyyy-??-XX` (ADL 1.4: month optional, day not allowed) gets `2020-01-01`. FerroEHR refuses. | test_all_types.en.v1, Test_all_types_v2 | patterns were out of scope for REQ-103 | § REQ-103 *Validate contract*: temporal patterns | Settled. Both CDRs accept both templates. |
| G10 | `Generate` fails on an OPT that constrains an attribute the RM class does not have: `ISM_TRANSITION.name` (5 OPTs) and `ELEMENT.null_flavor` (2). | GECCO core, RIPPLE - Conformance Test template | only `rminfo.IsNonStorableAttr` was skipped (`walkSkips`, `generate.go:345`) | § REQ-107 *Visit rule*: an attribute the compile marks as one the RM class lacks is not visited; § REQ-100 reads `null_flavor` as `null_flavour` | Settled. Both generate under every policy. EHRbase accepts GECCO core, which FerroEHR refuses at upload. EHRbase refuses RIPPLE's `Large` documents for its `null_flavor` reading. |
| G11 | `Generate` fails under `Medium` and `Large` where the OPT puts an object under `feeder_audit`, an attribute it has no writer for, although the attribute is optional. | genericFHIRTemplate | the known gap on attributes the generator cannot write recorded the error; the defect was failing where the attribute is optional | § REQ-107 [*Exceptions and known gaps*](../../specifications/clinical-modeling.md#exceptions-and-known-gaps), *Known gap: attributes the generator cannot write* | Settled. It generates under every policy, and both CDRs accept it. |
| G12 | An OPT whose children's occurrence lower bounds add up past its cardinality upper bound gets a root that fails the SDK's own validator, where an error would say why. | cardinality_of_section (content 1..1, lower bounds summing to 8) | *Errors* named primitive narrowings, slots and archetype ids, no cardinality | § REQ-107 [*Errors*](../../specifications/clinical-modeling.md#errors) | Settled. `Generate` returns `ErrConstraintUnsatisfiable`. |
| G13 | `ACTIVITY.action_archetype_id` gets `example` where the OPT gives no pattern; the RM default is `/.*/`, which both CDRs write. Where the OPT gives a pattern, the SDK writes a matching id and both CDRs write `/.*/`, which does not match it. | 16 activities under `Large` | *RM defaults* had no entry for it | § REQ-107 *RM defaults* | Settled. `Large` writes `/.*/` on 105 of 138 activities, and a matching id where the OPT gives a pattern or a list. |

### Coverage

Run 1 measured 162 templates that all three generators handle:

| Generator and level | Archetype roots written | ELEMENTs written |
|---|---|---|
| SDK `Required` | 9% | 0% |
| SDK `Medium` | 34% | 42% |
| SDK `Large` | 64% | 73% |
| FerroEHR `medium` (and `complete`) | 98% | 97% |
| EHRbase (single level) | 100% | 100% |

- **Named clones** (one archetype under several template names) accounted for most of the gap. `Large` wrote only the first of them. Example: SDK `Large` wrote 2 of 9 content members of the Aboriginal and Torres Strait Islander health check.
- **Empty ITEM_TREEs.** The SDK wrote them at `protocol`, `state` and `other_context`: 124, 52 and 37 under `Large`. Neither CDR writes any.
- **ENTRY uid.** The SDK wrote a `uid` on every ENTRY. Neither CDR does, and the RM's Common IM says a locatable's `uid` "will usually be empty" below the top-level object (`specifications-RM/docs/common/master03-archetyped_package.adoc:39-41`).

Run 2 measured 165 COMPOSITION templates that the SDK generates from and both CDRs give an example for. A use of an archetype in the OPT counts as written when the document holds a root of that archetype under the use's pinned name, each root in the document counted once. An ELEMENT counts when the document holds its node id under its archetype. The two runs' ELEMENT figures are measured differently and do not compare.

| Generator and level | Archetype roots written | ELEMENTs written |
|---|---|---|
| SDK `Required` | 2% | 0% |
| SDK `Medium` | 29% | 40% |
| SDK `Large` | 99.9% (791 of 792) | 94% |
| FerroEHR `medium` | 97% | 93% |
| FerroEHR `complete` | 97% | 93% |
| EHRbase (single level) | 100% | 96% |

- **Named clones.** Settled by § REQ-107 *What a visit writes*: `Large` writes a member of every use of an archetype. The one root it leaves out is a SECTION whose required `items` can hold only an entry the OPT names no archetype for (sdk_crosscheck_structure.v1), which stays unvisited as undescribed content.
- **Empty item structures.** Settled by § REQ-107 *Visit rule* and *What a visit writes*. No document holds an empty item structure under an optional attribute. The SDK still writes one where the OPT requires the structure and gives its `items` only slots or no child (vital_signs, c521b47e-7ec3-4bc7-b58c-8a869912d720), or where the RM requires it (`EVENT.data`, `PARTY_IDENTITY.details`).
- **ENTRY uid.** Settled by § REQ-107 *Identity*. No document carries a `uid` on a COMPOSITION, an ENTRY, a PARTY or a PARTY_RELATIONSHIP, at the root or below it.

## Validator and parser findings

Run 1 classified all 722 issues the SDK reported on the CDRs' examples.

| # | What happened in run 1 | Count | Cause at `9e29377a` | Settled by | Run 2 |
|---|---|---|---|---|---|
| V1 | A C_STRING list with `list_open` true is treated as closed. | 190 issues, 14 documents | the flag was not parsed (`parse_primitives.go:249-260`); REQ-103 listed it as out of scope | § REQ-103 *Validate contract*: `CString` and its open list | Settled. No such finding remains. |
| V2 | A value that matches a later alternative of a single-valued attribute is judged against the first alternative of the same RM type. In this corpus: ISM_TRANSITION alternatives of an ACTION. | 37 issues, 18 documents (7 carry nothing else) | § REQ-102 *Structural, alternatives* said "the first child in OPT order wins" within a tier (`rmbind.Bind`, `openehr/internal/rmbind`) | § REQ-102 *Structural, alternatives*: within a tier, the first child under whose constraints the value validates | Settled. No such finding remains. |
| V3 | An OPT attribute the RM class does not have is reported as a missing required attribute (`ISM_TRANSITION.name`), which no instance can carry. | 39 issues | `declaredAttr` skipped only non-storable attributes (`openehr/validation/engine.go:715-728`) | § REQ-100: the compile marks such an attribute; [§ REQ-171](../../specifications/clinical-modeling.md#req-171--validation-engine): the walk skips it | Settled. No such finding remains. |
| V4 | An OPT whose `xsi` prefix is bound to a misspelt namespace is read without error: every untyped node becomes a complex object or a single attribute, so constraints vanish. The SDK then reports 9 false findings and misses a category outside the OPT's list. | Case (and a second copy) | `xsi:type` was matched by namespace only, and an empty type took a default even in strict mode (`openehr/template/parse.go:245,321,414,574`) | § REQ-100 *Strict parse mode*: strict mode refuses a node with no type in the XSI namespace; lenient mode keeps its default | Settled. Strict mode refuses Case, so run 2 neither generates from it nor validates its examples. Lenient mode reads it with the default types, as before; the defect is the OPT's own. |
| V5 | Two siblings that share an archetype id and a pinned name both bind to the first, so a valid second member is reported over its occurrences. | 1 issue (SSIAD PRIeSM); same shape in 3 more OPTs | binding was stateless per item (`matchChildByPinnedName`, `openehr/validation/walk_composition.go:542-563`) | § REQ-102 *Structural, binding items*: the first sibling with room under whose constraints the item validates | Settled in the SDK. EHRbase still binds to the first (see *What each CDR still refuses in run 2*). |
| V6 | An absent list whose mandatory child is missing passes, while the same list present and empty fails; the wire cannot tell the two apart (canonical JSON drops an empty list). | the 10 G1 templates | § REQ-102 *An existence lower bound alone asks for no member* and the absent-list sentence before it (`engine.go:953`) | § REQ-102 *A mandatory child under an optional list* | Settled. |
| V7 | A list the OPT requires (existence 1..1) is accepted absent or empty where an RM invariant says it is non-empty when present (`EVENT_CONTEXT.Participations_validity`). FerroEHR refuses. | Referral | the same *existence lower bound alone* sentence, and the RM floor did not evaluate the invariant (`openehr/validation/invariants.json`) | [§ REQ-112](../../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) *Per-RM-type invariant catalogue*, *Non-empty lists* | Settled. The examples without participations of Referral and sdk_crosscheck_structure.v1 are reported, as FerroEHR refuses them. |
| V8 | DV_INTERVAL bounds that are not strictly comparable (different DV_QUANTITY units) are not reported. FerroEHR refuses its own such example. | test_intervals.en.v1 | the floor ordered comparable pairs only, a gap REQ-112 then recorded | § REQ-112, the DV_INTERVAL row: bounds that are not strictly comparable break `Limits_consistent` | Settled. The mixed-unit bounds of test_intervals.en.v1 and sdk_crosscheck_values.v1 are reported, as FerroEHR reports them. |
| V9 | Known out-of-scope REQ-103 checks show up as missed refusals: `C_DURATION.range` and patterns (G3), date patterns (G9) and `C_DV_QUANTITY.property` (G8). | see G3, G8, G9 | REQ-103's out-of-scope list; for `property`, REQ-103 was silent | § REQ-103 *Validate contract* | Settled. Every FerroEHR refusal for duration or date patterns and property units is matched by an SDK finding. |
| V10 | HISTORY `Events_valid` is not evaluated (G4). | 2 templates | `invariants.json` marked it "Not yet evaluated" | § REQ-112 *Per-RM-type invariant catalogue* | Settled. |

**Where the SDK validator is right and both CDRs accept** (run 1, the ordinals verified by script; run 2 finds the same kinds, and the same counts for the first two):
- **DV_ORDINAL symbols.** EHRbase's examples code them in terminology `external` where the OPT lists `local` codes: 152 issues in 25 documents.
- **DV_PROPORTION type.** EHRbase's examples always write type 3, outside the OPT's list in 90 issues. FerroEHR refuses these.
- **Coded name options.** EHRbase writes one ELEMENT per coded-name option, past the node's occurrences (41 issues). FerroEHR refuses these too.
- **`action_archetype_id`.** Both CDRs write `/.*/` against an OPT pattern.
- **Precision.** Both CDRs leave out a DV_PROPORTION `precision` that the OPT requires.
- **Hardcoded values.** EHRbase writes `video/H261`, `text/html` and `dev/null` against closed lists and patterns.

**The validator in run 2.** Every finding on the CDRs' examples traces to an OPT constraint or an RM rule. Two kinds are new:
- **Property units.** Under a property with no unit list, EHRbase writes `mm` for Pressure and for Concentration; FerroEHR refuses it for Pressure. FerroEHR writes `g/100ml` and `N/m²` and accepts them. The SDK reads the UCUM column of the property-unit table (`g/(100.ml)`, `N/m2`), since RM `DV_QUANTITY.units` is a UCUM string (§ REQ-103 *Validate contract*).
- **Temporal patterns.** EHRbase's examples carry durations, dates and times outside the OPT's pattern, and times without a zone the OPT makes mandatory (41 issues in 18 examples). The SDK reports every such value FerroEHR refuses.

Every CDR refusal of an example that the SDK does not share is one CDR's own reading, of a sound template or of a defective one, listed under *CDR observations*. Two templates' examples were not validated, since strict parsing refuses the templates (Case, duration constraints).

## Spec sentences the findings amended

Each sentence below was amended after run 1. The last column gives the text that decided it.

| Sentence | What run 1 found, and what the sentence says now | Deciding text |
|---|---|---|
| § REQ-107 *Visit rule*, the definition of a required attribute | It counted cardinality without existence (G2), and never counted a list child's occurrences (G1). It now counts a list's mandatory child that is not a slot, and cardinality once the attribute is visited. | ADL 1.4 `master05-cadl.adoc` (specifications-AM): `:275-280` existence and cardinality ("optional, but if present"), `:284` and `:316` occurrences ("mandatory in the GROUP"; a single attribute's child is already decided by its existence). AOM 2 `master04.5-constraint_model-class_definitions.adoc:159` (VACMCO, "mandatory child object"). AOM 1.4 `master04-constraint_model_package.adoc:33`. |
| § REQ-102, the absent-list sentence and *An existence lower bound alone asks for no member* | An absent list escaped its mandatory children's occurrences (V6), and an existence-required list escaped the RM's non-empty invariants (V7). § REQ-102 *A mandatory child under an optional list* now counts them, and the floor evaluates the invariants. | As above, plus RM `EVENT_CONTEXT.Participations_validity`, `COMPOSITION.Content_valid`, `INSTRUCTION.Activities_valid`, `SECTION.Items_valid`, `HISTORY.Events_valid` (`resources/bmm/openehr_rm_1.2.0.bmm.json`). |
| § REQ-102 *Structural, alternatives*, "Within a tier the first child in OPT order wins" | A value was judged against one alternative only (V2). Within a tier, the first child under whose constraints the value validates now wins. | ADL 1.4 `master05-cadl.adoc:237` (alternatives, "only one of which needs to be matched by the data"); AOM 1.4 C_SINGLE_ATTRIBUTE `alternatives`. Archie accepts a value as soon as one alternative validates. |
| § REQ-102 *Structural, binding items* | Siblings that share identity and name bound statelessly (V5). An item now binds to the first sibling with room under whose constraints it validates. | ADL 1.4 occurrences per object node. |
| § REQ-103 out-of-scope list (`list_open`, temporal patterns), its *Numeric range* paragraph and `C_DURATION` row (the `range` deferral), and its silence on `C_DV_QUANTITY.property` | These four checks were left out (V1, V9). The calendar-conversion reason given for durations no longer held: BASE defines `Iso8601_duration.to_seconds` and the SDK implements it as `DVDuration.Magnitude`. All four are now rows of *Validate contract*. | AOM 1.4 C_STRING `list_open`, C_DURATION `range`; ADL 1.4 `master05-cadl.adoc:852-950` (date, time and duration patterns); openEHR terminology `PropertyUnitData.xml` (specifications-TERM). |
| § REQ-107, the term-text paragraph after *RM defaults* (a code of another terminology read only under `<terminology>::<code>`) | It missed a term of another terminology defined under its bare code (G6), and let a pinned name text and its code disagree (G7). It now reads the bare code as a fallback, and takes the first listed code whose term the pinned text admits. | RM DV_TEXT `value`: for a DV_CODED_TEXT, "the rubric of the complete term". |
| § REQ-107 *Precedence* item 2, the closed "Present" list | It omitted HISTORY `Events_valid` (G4), which the list now holds. | RM HISTORY `Events_valid`. |
| § REQ-107 *Intervals* | It let bounds that cannot be compared stand (G5). Both bounds are now strictly comparable wherever both sides admit such a pair. | RM DV_INTERVAL `Limits_consistent` (`is_strictly_comparable_to`). |
| § REQ-107 *Exceptions and known gaps*: attributes the RM does not have, and the writer gap | It said a root whose OPT constrains an attribute the RM lacks fails the validator, and that no vendored OPT constrains one; in fact `Generate` returned an error, and public OPTs do (G10). The known gap on attributes the generator cannot write recorded the error for an object under `feeder_audit`, but not that an optional one should be left unvisited (G11). § REQ-107 *Visit rule* now skips an attribute the RM lacks, and the known gap leaves an optional attribute it has no writer for unvisited. | AOM 2 VCARM (`master04.5:126`). |
| § REQ-107 *Identity* | It wrote a uid on every ENTRY (coverage). It now writes none on an ENTRY, a PARTY or a PARTY_RELATIONSHIP unless the OPT requires one. | RM Common IM `master03-archetyped_package.adoc:39-41`. |
| § REQ-107 *What a visit writes*, named siblings under `Large` | It wrote only the first named clone (coverage). `Large` now writes a member of every use of an archetype. | ITS-REST `detail_level` (`resources/its-rest/definition-validation.openapi.yaml:236-238`). |
| § REQ-100 | It covered an unknown `xsi:type` value (the default parse and strict mode), but not a node that carries no type in the XSI namespace at all, which took a default type silently (V4). Strict mode now refuses such a node. | ITS-XML AM 1.4 `Archetype.xsd:32,43`: C_ATTRIBUTE and C_OBJECT are abstract. |
| § REQ-107 *Errors* | It named no cardinality case (G12). It now returns `ErrConstraintUnsatisfiable` for one. | ADL 1.4 VCOC, AOM 2 VACMCO. |
| § REQ-107 *RM defaults* | It had no `ACTIVITY.action_archetype_id` entry (G13). It now writes `/.*/`. | RM `ACTIVITY.action_archetype_id`, "Defaults to `/.*/`". |
| § REQ-107 *Visit rule* and *What a visit writes*, the empty item structure | It wrote an ITEM_TREE with no items under `protocol`, `state` and `other_context` (coverage). An optional attribute whose only allowed child would hold no member now stays unvisited. | Neither CDR writes one, and the RM requires none. |
| § REQ-171, the attributes the walk visits | It visited an OPT attribute the RM class lacks and reported it missing (V3). The walk now skips an attribute the compile marks. | AOM 2 VCARM. |
| § REQ-112 *What the floor does not evaluate*, and its DV_INTERVAL row | It left the non-empty list invariants and `Events_valid` unevaluated (V7, V10), and ordered comparable pairs only (V8). The floor now evaluates them, and reports bounds that are not strictly comparable. | RM `COMPOSITION.Content_valid`, `EVENT_CONTEXT.Participations_validity`, `INSTRUCTION.Activities_valid`, `SECTION.Items_valid`, `HISTORY.Events_valid`, `DV_INTERVAL.Limits_consistent`. |

## Template defects

These OPTs cannot yield a document every reader accepts. The SDK's handling of each is given beside it.

| Template | Defect | SDK |
|---|---|---|
| cardinality_of_section | content cardinality 1..1 against children whose occurrence lower bounds sum to 8 (ADL 1.4 VCOC, AOM 2 VACMCO) | `Generate` returns `ErrConstraintUnsatisfiable` (§ REQ-107 *Errors*); FerroEHR refuses the template at upload |
| section_admin_test | the template requires an ADMIN_ENTRY defined inline that it names no archetype for, against ENTRY `Is_archetype_root` | `Generate` returns `ErrArchetypeIDMissing` under every policy, the SDK's reading of the required entry under its list rule (§ REQ-107 *Visit rule* and *Errors*); both CDRs refuse their own examples of it for `Is_archetype_root` |
| Referral | `PARTICIPATION.function` coded in `local`, against `Function_valid` | writes it and reports it, as § REQ-107 *Exceptions and known gaps* states; EHRbase refuses the documents, FerroEHR accepts them |
| TestPerson.v2 | `media_type` in legacy openEHR codes 425-429, against `Media_type_valid` | as § REQ-107 *Precedence* item 1 states |
| GECCO core, Antenatal - first visit, NCD - first visit, Aboriginal ... (two copies) | `ISM_TRANSITION.name` constrained 1..1 | the attribute is neither visited nor reported (G10, V3) |
| RIPPLE - Conformance Test template | `null_flavor` spelt as in US English | read as `null_flavour` (§ REQ-100); EHRbase asks for it beside a `value` |
| sdk_crosscheck_unwritten.v1 | the RIPPLE and ISM_TRANSITION shapes above, on purpose | as above |
| Case and a second copy | `xmlns:xsi` bound to `http //www.w3.org/2001/XMLSchema-instance` | strict mode refuses it; lenient mode reads it with the defaults (V4) |
| duration constraints | a C_DURATION range whose bounds are both `P`, which is no ISO 8601 duration | strict mode refuses it; lenient mode reads both sides as unbounded (§ REQ-103 *Validate contract*). Both CDRs accept the template. |
| ePrescription (FHIR) | the second use of ELEMENT at0002 in CLUSTER.medication_course_summary.v0 pins the name text `Date written` with the codes at0003 and at0004, whose terms are "Date ordered/recommended" and "Date first prescription issued" | keeps the code and the text the walk wrote, as the term-text paragraph of § REQ-107 states where the pinned text admits no listed code's term; EHRbase refuses `Large`'s documents |

## CDR observations

Neither CDR gives the same verdict as the other on every document, and no single reading of the specifications matches both. Observed in run 1 unless the item says run 2.

**EHRbase 2.36.0**
- Template handling:
  - It refuses an OPT that starts with a UTF-8 byte-order mark ("Content is not allowed in prolog"); 61 of the 202 do, 8 of them vendored. The cross-check tool strips the mark before upload. The SDK's `definition.UploadTemplate` sends the body unchanged.
  - It refuses an OPT with an ITEM_TABLE ("unsupported types: ITEM_TABLE", the vendored clinical_content_validation), one with an empty description, and answers 500 to one more (NDS - SBAR_Care_Home.v0.0).
  - It collapses repeated spaces in a `template_id` (`IDCR -  Adverse Reaction List.v1` is stored and looked up with one space), as the ITS-XML `xs:token` type of OBJECT_ID's value implies. FerroEHR keeps the literal id. A composition that names the literal id is refused, in both runs.
- Its template example:
  - It answers 500 for every template whose root is not a COMPOSITION.
  - It writes DV_ORDINAL symbols and unconstrained coded texts in terminology `external`, a DV_PROPORTION of type 3 whatever the OPT allows, and `EVALUATION.time_asserted`, which RM 1.2.0 does not define.
  - It writes one ELEMENT per option of a coded name, an INTERVAL_EVENT and POINT_EVENT mix for an abstract EVENT, and fixed values (`video/H261`, `text/html`, `dev/null`, `Lorem ipsum`).
  - Run 2: under a property with no unit list, it writes `mm`, for Pressure and for Concentration alike.
  - It refuses 14 of its own examples (17 in run 2), among them for an "Observation contains no results" check, which no RM invariant states, for interval bounds outside the OPT's range, for `Is_archetypeRoot` and for the `null_flavour` reading below.
- Its composition validation:
  - Where an ELEMENT's `value` or `null_flavour` has more than one alternative, it enforces the alternatives' occurrences although the attribute's existence is 0..1. So it asks for a `null_flavour` beside a `value` (Demonstration.v1, Review), which ELEMENT `Inv_null_flavour_indicated` forbids, and for a `value` where the SDK writes a `null_flavour` (GECCO_Diagnose). Run 2: it reads the misspelt `null_flavor` the same way (RIPPLE - Conformance Test template, sdk_crosscheck_unwritten.v1).
  - It checks a coded text's `value` against its term text, the bare-code key included.
  - It checks ISM transitions as whole alternatives (current state with careflow step).
  - It does not check an ordinal symbol's terminology, a C_STRING pattern on `action_archetype_id`, temporal patterns or a quantity's property.
  - Run 2: it binds every member of sibling uses that share a path to the first use, and answers 500 to every `Medium` and `Large` document of sdk_crosscheck_siblings.v1 (see *What each CDR still refuses in run 2*).

**FerroEHR 5.0.0**
- Template handling:
  - It refuses 22 OPTs at upload: existence looser than the RM's (AOM 2 VCAEX, 11), an ordinal symbol with no `value` element (ITS-XML schema, 6), an incomplete `default_value` (2), a CODE_PHRASE with no `terminology_id` (1), an empty `lifecycle_state` (1) and an unsatisfiable cardinality (VACMCO, 1). EHRbase accepts all but two of these, for reasons of its own, and the SDK all of them; the SDK then applies the RM's existence to the data.
  - Run 2: it refuses sdk_crosscheck_padded.v1 (VCARM). It reads an `rm_attribute_name` broken onto a second line with the line break and indentation as part of the name, as the ITS-XML schema's `xs:string` type keeps them. EHRbase accepts the template, and § REQ-100 reads such an identifier without the surrounding white space.
  - It reads `null_flavor` as `null_flavour`, and an unresolvable `xsi` prefix by its letters.
  - It answers 400 with a clear message for a template whose root is not a COMPOSITION.
- Its template example:
  - Its levels nest exactly (`required` within `medium` within `complete`).
  - Run 2: under a property with no unit list, it writes the property-unit table's display text (`g/100ml`, `N/m²`), where the UCUM column gives `g/(100.ml)` and `N/m2`, and accepts it.
  - It refuses 22 of its own examples (27 in run 2), among them for interval bounds in different units, sibling minimums, `participations`, `Is_archetype_root`, and `items` it requires on an ITEM_LIST or ITEM_TREE that the OPT leaves without a child at existence 1..1 (the RM and AOM 1.4 have no such rule).
- Its composition validation:
  - It refuses a DV_TEXT where the OPT allows DV_TEXT or DV_CODED_TEXT, since its web template folds the pair into one coded node.
  - It accepts an ISM transition that matches no alternative as a whole.
  - It enforces property units, date and time patterns and duration ranges. In run 1 it accepted `P0D` under duration patterns that do not name days (G3); in run 2 it refuses `PT42H` under the pattern `PTM` (sample template).
  - It allows one create per EHR for a persistent template.
  - It refuses a document that carries a member the RM class does not define, such as EHRbase's `EVALUATION.time_asserted`. The SDK's decoder skips such a member, and run 2 lists it as unmatched (166 members); § REQ-102 does not validate wire bytes (*Out of scope*).
  - Run 2: it counts every member of sibling uses of one identity against each use (sdk_crosscheck_siblings.v1).

## Backlog leads checked

- **Settled since run 1:**
  - the `Required` visit of an optional attribute with a mandatory child (G1). Its Demonstration at0016 clause is EHRbase's single-attribute reading, not the same cause;
  - a shared quantity unit for interval bounds (G5);
  - siblings that share an identity binding to the first (V5).
- **Unchanged by this work:** the top-up past a child's occurrences; `PERSON.identities`; untyped first children in `NewSkeleton`; slot occurrences; the GTS formalism; DV_PROPORTION `is_integral`. No template in run 1 reaches them, except that Demonstration.v1 pins `is_integral`, which no document in either run broke. In run 2, no output fails the SDK's own validators for the first two.

## Reproducing

The cross-check is an opt-in Go tool in `cmd/cdrcheck` that `make cdr-crosscheck` runs ([§ REQ-082 *The CDR cross-check tool*](../../specifications/conformance.md#the-cdr-cross-check-tool)); it records and asserts nothing, so it is not a probe. It is not part of `make ci`.
- It reads the template directory from `OPENEHR_XC_TEMPLATES`, and each CDR's REST base from `OPENEHR_LIVE_EHRBASE` or `OPENEHR_LIVE_FERROEHR`, with its Basic credential in the matching `_BASIC` variable. It writes to the CDRs only when `OPENEHR_LIVE_ALLOW_MUTATING` is set. With no template directory, it skips.
- It uploads each template under its own id, as [§ REQ-082 *The CDR cross-check tool*](../../specifications/conformance.md#the-cdr-cross-check-tool) says. It creates every EHR under an id that begins with its per-run identifier, and it deletes nothing.
- It writes to a directory outside the repository (`OPENEHR_XC_OUT` or `-out`): one JSON Lines file per stage, the documents, the CDRs' examples and `summary.md`. The generation clock is fixed, so outputs compare across runs.
- The template set and the CDRs' compose files stay outside the repository. Run 2 used the 202 templates of run 1 and the five vendored synthetic OPTs.
