---
kind: plan
---

# Plan: template validator, RM floor and OPT parser, what the CDR cross-check found

**Date:** 2026-10-24
**Status:** Draft, from the cross-check of 2026-10-24 on `main` at `9e29377a` ([evidence](cdr-crosscheck-evidence/README.md)). The maintainer's rulings of the same day are recorded in the phases they decide.
**Covers:** REQ-100 ([§ REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths)), REQ-103 ([§ REQ-103](../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection)), REQ-102 ([§ REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation)), REQ-112 ([§ REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor)), REQ-171 ([§ REQ-171](../specifications/clinical-modeling.md#req-171--validation-engine))
**Probes:** PROBE-077, PROBE-027
**Depends on:** [the generator plan](2026-10-24-generator-cdr-crosscheck.md) Phase 1 lands before Phase 4 here, so the corpus ratchet stays green when the validator starts counting a list's mandatory children.
**Defers:** template lint for authors (VCAEX, VACMCO, an attribute the RM class lacks, a byte-order mark, an unresolvable `xsi` prefix), which belongs to [the OPT author validator plan](2026-07-16-opt-author-validator.md); a rule tying a coded text's `value` to its term text (EHRbase checks it; no RM invariant states it).

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

Make the SDK validator reach the verdict the openEHR specifications give on the documents both CDRs produce:
- **No false findings** on valid documents: 276 issues today (V1 to V5).
- **No silent pass** where the RM or the OPT refuses a document (V6 to V10).

Each row names its evidence in the [findings](cdr-crosscheck-evidence/README.md) and the sentence it amends through `/sdd-specify`. Every phase is full lane.

## Phases

### Phase 1: REQ-103 constraints the parser drops

Maintainer ruling (2026-10-24): property units and temporal patterns are in scope.

**Tasks:**
- **C_STRING `list_open` (V1, 190 false findings).**
  - Parse the flag (`openehr/template/parse_primitives.go:249-260`) and carry it on `CString`, through a field or a new constructor, so the exported `NewCString` signature stays.
  - `Validate` then skips list membership on an open list and still checks a pattern.
  - Give the Web Template's TEXT input the C_STRING list and a `listOpen` read from the flag (`openehr/template/webtemplate/inputs.go:18-19` emits TEXT with no list today). The coded-text `listOpen` at `inputs.go:60` comes from an empty code list and is another flag.
  - Turn the REQ-103 out-of-scope bullet into a row.
- **C_DURATION range (G3, V9).**
  - Parse `range` (`parse_primitives.go:262`).
  - Compare by `Iso8601_duration.to_seconds`, which the SDK already implements as `DVDuration.Magnitude` (`openehr/rm/temporal_funcs.go:369`).
  - Give `ExampleValue` a value inside the range and the pattern.
  - Amend the REQ-103 rows that list `CDuration` as pattern only and defer the range for calendar conversion.
  - The C_DATE, C_TIME and C_DATE_TIME ranges, dropped at parse today, take the same treatment. This settles that row of [the generator gaps plan](2026-10-06-generator-gaps-after-pr-235.md).
- **Temporal patterns (G9, V9).**
  - Enforce the ADL 1.4 patterns of C_DATE, C_TIME, C_DATE_TIME and C_DURATION (`specifications-AM/docs/ADL1.4/master05-cadl.adoc:852-950`): `XX` forbids a field and `??` makes it optional.
  - `ExampleValue` respects the pattern.
  - Turn the REQ-103 out-of-scope bullet on partial date and time patterns into rows.
- **C_DV_QUANTITY `property` (G8, V9).**
  - Vendor the openEHR property-unit table (`PropertyUnitData.xml`, specifications-TERM) beside `resources/terminology`, after checking its licence.
  - Where the OPT gives a property and no unit list, `Validate` checks that the unit belongs to the property, and `ExampleValue` picks one of its units.
  - A unit list the OPT gives keeps precedence, including units outside the property.
  - Add a REQ-103 row for it; today only the `DvQuantity` doc comment says the property is exposed but not enforced (`openehr/template/constraints/quantity.go:30-32`). The conformance test schedule case `CONT-DV_QUANTITY-validate_property` is the acceptance.

**Definition of done:**
- The 190 `list_open` findings are gone.
- FerroEHR's refusals in the evidence for durations, date patterns and property units are matched by the SDK.
- Every constraint still validates its own example.
- `make ci` is green.

### Phase 2: OPT parse fidelity (REQ-100)

**Tasks:**
- **A node with no type in the XSI namespace (V4).**
  - In strict mode, refuse an `<attributes>` or `<children>` inside `<definition>` that carries no type in the XSI namespace (`openehr/template/parse.go:245,321,414,574`). ITS-XML makes C_ATTRIBUTE and C_OBJECT abstract (`Archetype.xsd:32,43`).
  - Add the REQ-100 sentence.
- **Decision to confirm:** what lenient mode does with an unresolvable type.
  - Keep today's default.
  - Or read a `type` attribute whose prefix is `xsi` by its letters, as FerroEHR does. This recovers the constraints of the two Case OPTs.
- **An attribute the RM class does not have (V3, G10).**
  - Compile marks an attribute that names neither a property nor a function of its class in the pinned BMM. The functions `is_integral` and `offset` stay legal.
  - The validator skips it as it skips a non-storable attribute (`declaredAttr`, `openehr/validation/engine.go:715-728`) and lists it in `Coverage`. The generator plan's Phase 3 reads the same mark.
  - Amend REQ-100, REQ-102 and the REQ-171 sentence on attributes no serialization carries.
- **Decision to confirm:** whether strict mode refuses such an attribute, and whether the parser reads `null_flavor` as `null_flavour`, FerroEHR's reading, or skips it like any attribute the class lacks. Both CDRs accept these OPTs. Recommended: skip; AOM 2 VCARM makes the OPT invalid, and a table of misspellings has no end.

**Definition of done:**
- The 39 `ISM_TRANSITION.name` findings and the 9 Case findings are gone.
- The Case category outside the OPT's list is reported, in lenient mode, if the decision recovers it.
- `make ci` is green.

### Phase 3: binding (REQ-102)

**Tasks:**
- **Alternatives of one RM type (V2, 37 false findings).**
  - Amend § REQ-102 *Structural, alternatives* and the sentence on items that are not a LOCATABLE. The three type tiers stay. Within a tier, the validator binds a value to the first child under whose constraints it validates; when no child validates it, to the first child of the tier, whose findings are reported.
  - Sources: ADL 1.4 `master05-cadl.adoc:237`, "only one of which needs to be matched by the data"; AOM 1.4 C_SINGLE_ATTRIBUTE `alternatives`. Archie (`RMObjectValidator`) and EHRbase (`IsmTransitionValidator`) both check whole alternatives.
  - The trial is the validator's alone. The generator builds under the first child of the tier and `composition.Builder.Set` accepts by type, so both keep the tier rule through the shared binder `rmbind.Bind` (`openehr/internal/rmbind/bind.go:82`, called at `openehr/composition/typecheck.go:58` and `openehr/validation/walk_composition.go:102`); the validator adds its trial on top of that choice. A value the generator builds under the first child validates under it, so § REQ-107 *What a visit writes* keeps its claim. Amend that sentence, which restates the tier rule ("the first child, unless a later one names that type exactly"), and § REQ-101 *Type enforcement*, which binds `Set` to the alternatives rule, so that both name the tier rule and not the trial.
- Implement the trial walk with the engine's issue slice and `withdrawInside` (`engine.go:427`):
  - walk the value under each candidate into a buffered sink, in OPT order, and stop at the first that reports nothing;
  - count every trial visit against `Limits.Nodes`, so a value under k same-tier alternatives costs at most k walks of its subtree, and a run that exhausts the limit still ends with `max_nodes`;
  - restore what a failed trial changed.
  - Add a sentence to § REQ-171 *One walk* that a trial under an alternative is part of the walk and counts toward its limits; the walk order and the union rule do not change. Benchmark `ValidateComposition` on Demonstration.v1 before and after, within the budget of STRAND-04.
- Tests: an ISM_TRANSITION that matches a later alternative (the minimal_action shape), two DV_QUANTITY alternatives, and a value that validates under none and is reported under the first.
- **Siblings that share an identity (V5).**
  - Among equally ranked children, bind an item to the first whose occurrences upper bound is not reached yet and under whose constraints it validates; failing that, to the first with room; failing that, to the first, counting per child, in place of the stateless choice in `matchChildByPinnedName` (`openehr/validation/walk_composition.go:542-563`). This is the alternatives rule with occurrences added, so one method decides both questions.
  - Amend § REQ-102 *Structural, binding items*.
  - This also settles the backlog lead on siblings that share an identity and pin no name (`walk_composition.go:515`), which fails a later required sibling, and removes the reason for the REQ-107 skip of an optional child whose identity a sibling carries (§ REQ-107 *What a visit writes*, "so the validator's binding of members to children stays unambiguous") and for the *An earlier sibling that carries a later one's identity* bullet of its *Exceptions and known gaps*. [The generator plan](2026-10-24-generator-cdr-crosscheck.md) Phase 5 decides what `Large` then writes.

**Definition of done:**
- The 37 alternative findings and the SSIAD PRIeSM finding are gone.
- A transition that matches no alternative as a whole is still refused. EHRbase refuses it; FerroEHR accepts it.
- `make ci` is green.

### Phase 4: existence and non-empty lists (REQ-102, REQ-112)

Maintainer ruling (2026-10-24): the validator follows the generator's `Required` reading. A list's mandatory child counts once its owner exists; a single attribute's child does not.

**Tasks:**
- **A mandatory child under an optional list (V6).**
  - Scope the REQ-102 absent-list sentence to a list the template requires by its existence: such a list, absent, keeps drawing `required` alone when its cardinality or a child's occurrences asks for a member (*An existence lower bound alone asks for no member*).
  - Add a paragraph: on an object the instance carries, each allowed child of a multi-valued attribute that is not a slot is counted against its occurrences, whether the list is present, empty or absent. On a list the template leaves optional by its existence, the list's own cardinality is not counted when the list is absent (ADL 1.4 `:275-280`, "optional, but if present"), and, since canonical JSON carries an empty optional list as an absent one (REQ-102, *An existence lower bound alone asks for no member*), not when it is present and empty either. Decision to confirm: this reads the two alike, as REQ-102 already does for existence.
  - Bring the *child cardinality* row in line.
  - Code: on an absent list at `engine.go:953`, run `appendOccurrenceIssues` with an empty binding. With the cardinality rule above, an absent and an empty optional list draw the same findings.
- **The RM's non-empty lists (V7, V10).**
  - Evaluate in the floor:
    - `EVENT_CONTEXT.Participations_validity`;
    - `COMPOSITION.Content_valid`;
    - `INSTRUCTION.Activities_valid`;
    - `SECTION.Items_valid`;
    - `HISTORY.Events_valid`, with its `summary` alternative;
    - `ATTESTATION`'s items rule.
  - Flip their `openehr/validation/invariants.json` entries, and add each to the REQ-112 catalogue and a PROBE-077 row.
  - Carve these attributes out of *An existence lower bound alone asks for no member*.

**Definition of done:**
- A document shaped as the G1 cases, an absent optional list whose child has occurrences lower 1 or more, is refused at that child in reproduction tests that build the document by hand, since the generator writes none after its Phase 1; the ten G1 templates' `Required` documents, regenerated at `9e29377a` by the harness, get the verdicts both CDRs gave.
- Referral without participations is refused.
- `TestREQ107_CorpusRatchet` and `make ci` are green.

### Phase 5: RM floor, interval comparability (REQ-112)

**Tasks:**
- **Bounds that are not strictly comparable (V8).**
  - Report DV_INTERVAL bounds that are not strictly comparable: different classes, DV_QUANTITY units, DV_PROPORTION types. RM `DV_INTERVAL.Limits_consistent` includes `is_strictly_comparable_to`.
  - Correct the `invariants.json` classification, which marks the invariant evaluated while it skips that part, and the package documentation's "numeric bounds in order".
  - Amend the REQ-112 sentence that orders comparable pairs only.

**Definition of done:** test_intervals.en.v1's mixed-unit pair is reported, as FerroEHR reports it. `make ci` is green.

### Phase 6: verification

**Tasks:**
- Rerun the cross-check after each phase, with the harness decision the generator plan's Phase 6 records.
- After the last phase, update the evidence README.

**Definition of done:** on the CDRs' own examples, every finding the SDK reports is one the specifications support, and every refusal by a CDR that the specifications support is matched by an SDK finding. The findings name the CDR-specific readings that are excluded.
