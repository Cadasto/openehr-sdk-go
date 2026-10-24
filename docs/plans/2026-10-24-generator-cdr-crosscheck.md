---
kind: plan
---

# Plan: REQ-107 generator, what the CDR cross-check found

**Date:** 2026-10-24
**Status:** Draft, from the cross-check of 2026-10-24 on `main` at `9e29377a` ([evidence](cdr-crosscheck-evidence/README.md)). The maintainer's rulings of the same day are recorded in the phases they decide.
**Covers:** REQ-107 ([§ REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator)); REQ-101 through the skeleton `composition.NewBuilder` seeds from `Large` ([§ REQ-101](../specifications/clinical-modeling.md#req-101--generic-opt-driven-composition-builder))
**Probes:** PROBE-027
**Depends on:** [the validator and parsers plan](2026-10-24-validator-parsers-cdr-crosscheck.md) Phase 1 for the duration, pattern and property-unit rows of Phase 4, and its Phase 2 for the attribute mark Phase 3 reads. Phase 1 here lands before that plan's Phase 4.
**Defers:** a real `complete` level (ITS-REST `detail_level=complete`: a second occurrence of each repeating node, alternatives spread across occurrences, RM-optional attributes such as `provider`, `workflow_id`, `guideline_id`, `end_time`, `health_care_facility`); `Medium`'s one-optional-archetype rule; a C_CODE_REFERENCE placeholder in the terminology its `referenceSetUri` names; the ELEMENT's name as DV_TEXT placeholder text.

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

Make every document `Generate` writes, under each policy and both value fills, one that EHRbase and FerroEHR accept wherever the OPT allows it, and make `Large` cover the template the way both CDRs' examples do. Each row below names its evidence in the [findings](cdr-crosscheck-evidence/README.md) (G1 to G13 and the coverage notes) and the § REQ-107 sentence it amends through `/sdd-specify`. Every phase is full lane.

## Phases

### Phase 1: `Required` is the minimal committable document

Maintainer ruling (2026-10-24): `Required` follows both readings below.

**Tasks:**
- Amend the definition of a *required* attribute in § REQ-107 *Visit rule*. An attribute becomes required when the BMM marks it mandatory or its existence lower bound is 1 or more. Where the OPT declares it multi-valued, it is also required when an allowed child that is not a slot has an occurrences lower bound of 1 or more (G1).
  - Sources: ADL 1.4 `master05-cadl.adoc:284,316`, "mandatory in the GROUP"; AOM 2 VACMCO, "mandatory child object".
  - A cardinality lower bound counts members only once the attribute is visited (G2; AOM 1.4 `master04-constraint_model_package.adoc:33`).
  - A single-valued attribute's child never makes the attribute required: ADL 1.4 `:284` says its existence decides.
  - The COMPOSITION `context` exception stays.
- Change `visitRequired` and `isRequired` (`openehr/instance/generate.go:478`, `:2265`) to match, using `tcimpl.AsksForMember` for the list rule.
- Add reproduction tests that fail first:
  - an optional list holding a 1..1 child, such as `ITEM_TREE.items` over an ELEMENT;
  - an optional `content` with cardinality 1..* and an optional child;
  - an optional `content` with a 1..1 archetype root;
  - `testkit/corpus/templates/family_history.v.1.2.3.opt`, which is vendored.
- Check the knock-on:
  - `Medium` and `Large` documents do not change: a census of the 202 templates found no unmet mandatory child under them.
  - The `Required` documents of 52 templates lose a topped-up `content`.
  - Bring `docs/examples.md` and the PROBE-027 text in line where they show a `Required` document.
- Delete the backlog lead on the `Required` visit of an optional attribute (`generate.go:466`). Its Demonstration at0016 clause is EHRbase's single-attribute reading (findings, *CDR observations*), not this cause.

**Definition of done:** the `Required` documents of the ten G1 templates are accepted by both CDRs on a rerun of the cross-check, and pass both SDK validators. `TestREQ107_CorpusRatchet` and `make ci` are green.

### Phase 2: RM invariants and OPTs that contradict themselves

**Tasks:**
- **HISTORY `Events_valid` (G4).**
  - In `writeRMNeeded` (`generate.go:268`): when a HISTORY has neither `events` nor `summary`, write `events` with a member of its first allowed EVENT child.
  - Add the rule to the "Present" list of § REQ-107 *Precedence* item 2.
- **Interval bounds that cannot be compared (G5).**
  - Give a side the OPT leaves without units the unit of the other side, under both value fills, before ordering (`openehr/instance/interval_order.go`).
  - Amend § REQ-107 *Intervals* so that both bounds are strictly comparable wherever both sides admit such a pair (RM `DV_INTERVAL.Limits_consistent`).
  - This settles the backlog lead on `sharedQuantityUnit` (`interval_order.go:1131`).
- **Unsatisfiable cardinality (G12).**
  - Return `ErrConstraintUnsatisfiable` when the occurrence lower bounds of an attribute's allowed children add up past its bounded cardinality upper bound (ADL 1.4 VCOC, AOM 2 VACMCO).
  - Amend § REQ-107 *Errors*, and remove the case from *Exceptions and known gaps*.

**Definition of done:** 10_alternative_types.en.v1, PROMIS-29 and test_intervals.en.v1 yield documents both CDRs accept under every fill. cardinality_of_section returns the error. `make ci` is green.

### Phase 3: OPT shapes `Generate` fails on

**Tasks:**
- **Attributes the RM class does not have (G10).** Examples: `ISM_TRANSITION.name` and `ELEMENT.null_flavor`.
  - Skip them in `walkSkips` (`generate.go:345`) as the non-storable set is skipped, reading the mark the validator plan's Phase 2 adds at compile.
  - Amend § REQ-107 *Visit rule* and the *Exceptions and known gaps* sentence on such attributes. That sentence now claims the root fails the validator and that no vendored OPT has one; neither holds.
- **An optional attribute the generator has no writer for (G11).** Examples: LOCATABLE `feeder_audit` and `links`.
  - Leave it unvisited unless the BMM or its existence requires it, and fail only then.
  - Amend the writer-gap sentence of *Exceptions and known gaps*.

**Decision to confirm:** an OPT that names no archetype for an ENTRY (section_admin_test) makes `Generate` return `ErrArchetypeIDMissing` under `Medium` and `Large`. One option is to treat that member as undescribed content and leave the optional attribute unvisited, which returns a valid root. Confirm today's error, or take the option.

**Definition of done:** GECCO core, RIPPLE - Conformance Test template and genericFHIRTemplate generate under every policy, and both CDRs accept the documents. `make ci` is green.

### Phase 4: values the CDRs refuse

Maintainer ruling (2026-10-24): property units, temporal patterns and the coded-name code are in scope.

**Tasks:**
- **Durations (G3).**
  - With the range and pattern the validator plan's Phase 1 parses, `CDuration.ExampleValue` and the sampler (`openehr/instance/sample.go:160`) write a value inside the range and in the units the pattern admits.
  - A zero takes the pattern's spelling: `PT0S`, `P0Y` or `P0W`, not `P0D` everywhere.
- **Date and time patterns (G9).** Example and random dates, times and date-times leave out the fields the pattern forbids (`XX`), for example `yyyy-??-XX`.
- **Property units (G8).** A C_DV_QUANTITY constrained by `property` with no unit list gets a unit of that property, from the table the validator plan vendors, under both fills.
- **Term text under the bare code (G6).**
  - `termKey` (`openehr/instance/coded_text.go:107-113`) falls back to the bare code for a code of another terminology when the OPT defines no `<terminology>::<code>` term; a `local` code is read bare already.
  - Amend the term-text paragraph of § REQ-107, after *RM defaults*.
- **A coded name whose text the OPT pins (G7).**
  - Write the listed code whose term text the C_STRING admits, not the first listed code.
  - Amend the same sentence.
- **`ACTIVITY.action_archetype_id` (G13).**
  - Where the OPT gives no pattern, write the RM default `/.*/` (BMM documentation of the attribute).
  - Add it to § REQ-107 *RM defaults*.

**Definition of done:**
- New Template, a1, ehrn_vital_signs.v2, constrain_test, test_all_types.en.v1, Test_all_types_v2, the two property-only quantity templates, Stationärer Versorgungsfall, Patientenaufenthalt, COVID-19 Pneumonia Diagnosis and Treatment, and ePrescription (FHIR) are accepted by both CDRs. The only exceptions are the CDR-specific readings the findings list.
- REQ-103's contract that a bounded constraint validates its own example still holds for every constraint.
- `make ci` is green.

### Phase 5: what `Large` writes

Maintainer ruling (2026-10-24): `Large` writes named clones, writes no empty item structure, and writes no ENTRY uid.

**Tasks:**
- **Named clones.**
  - Under `Large`, write a member of every optional named use of one archetype that needs no undescribed content, not only the first.
  - Amend § REQ-107 *What a visit writes*.
  - `composition.NewBuilder` seeds from `Large`, so the builder's skeleton grows. Check `openehr/composition/named_sibling_test.go` and the REQ-101 *Set* paths that rely on it, and record the change as consumer-visible.
- **No empty item structure.**
  - Leave unvisited an optional attribute whose only allowed child would hold no member, such as an ITEM_TREE under `protocol`, `state` or `other_context`.
  - A structure the RM or the OPT requires, such as `EVENT.data`, is still written.
  - Amend § REQ-107 *Visit rule* and the sentence of *What a visit writes* that writes such a tree with no items.
- **No ENTRY uid.**
  - Write a locatable's `uid` only where the OPT constrains it or the RM requires it (`PARTY` `Uid_mandatory`), as the RM Common IM describes (`master03-archetyped_package.adoc:39-41`).
  - Amend § REQ-107 *Identity*.
  - Check what depends on entry uids: the `UIDsFrom` determinism tests and the builder's uid-source tests.

**Definition of done:**
- `Large` covers at least 95% of the archetype roots the 162 cross-checked templates name, against 64% today.
- No document carries an empty optional item structure or an ENTRY uid.
- Both CDRs accept the documents.
- `make ci` is green.

### Phase 6: verification

**Tasks:**
- **Reproduction OPTs.** Add a vendored OPT for each shape a phase fixes. Take it from a licensed source with its notice, or write a minimal synthetic OPT; the forum templates in the evidence carry no licence.
- **Decision to confirm:** whether to land the cross-check harness.
  - The option: an opt-in Go tool behind a make target. It needs local CDRs and skips without them, and records the numbers each phase's definition of done quotes.
  - The alternative: keep the harness local and record the numbers in each PR body.
- **Rerun and update.** After the last phase, rerun the cross-check and update the evidence README with the new numbers.

**Definition of done:** both CDRs accept every document of the 202 templates under every policy, except the documents the findings attribute to a template defect or to one CDR's own reading.
