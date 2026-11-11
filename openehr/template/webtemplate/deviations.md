# WebTemplate export — deviations catalogue (REQ-106, PROBE-075)

**Informative.** The normative conformance contract — which categories of
divergence from the EHRbase `openEHR_SDK` v2.3 reference are permitted — lives in
the canonical spec, [REQ-106 § Conformance and deviations](../../../docs/specifications/clinical-modeling.md#req-106--webtemplate-json-export)
([ADR 0014](../../../docs/adr/0014-webtemplate-reference-implementation-lock.md)).
This file is the per-field elaboration of that contract, kept beside the parity
tests that pin it.

Conformance is **structural, not byte-exact**. PROBE-075 (`TestStructuralParity`
+ `TestInputParity` in this package) pins the load-bearing surface — every node's
`id`, `rmType`, `nodeId`, `aqlPath`, `min`/`max`, and each input's `suffix`/`type`
extended with coded/ordinal list values and ordinals, a careflow step's
`currentStates`, `listOpen`, `terminology`, temporal validation patterns, and
numeric validation ranges — against **three** vendored references since REQ-116
Phase 4: `constrain_test` **104/104** (exact, node and input), `Corona_Anamnese`
**230/230** and `GECCO_Diagnose` **34/34** (exact structurally; their documented
input, `min` and `rmType` deltas are pinned by count below). A fourth, the
reference's `test_all_types` golden, is compared on two subtrees: its ACTION
(`TestREQ106ActionParity`), with each delta named and pinned by category below,
and its EVALUATION's `choice` ELEMENT, whose value admits two types, exactly
(`TestREQ106ChoiceParity`).
The deltas below are the parts of the reference this slice deliberately does
**not** reproduce. Any change that makes a *pinned* field diverge is a test
failure, not a deviation.

## Node-level

- **`termBindings`, `annotations`, `inContext`** — not emitted. The reference tags
  RM-attribute leaves with `inContext: true` and carries per-node term bindings and
  UI annotations; this slice omits them (the leaves themselves, including their
  capitalized `name`, are emitted).
- **inContext coverage** — the fixed RM-attribute leaf table covers COMPOSITION,
  EVENT_CONTEXT, the ENTRY types, the EVENT variants (INTERVAL_EVENT adds
  `math_function` / `width`), INSTRUCTION (`narrative` / `expiry_time`), ACTIVITY
  (`timing` / `action_archetype_id`) and ACTION (`time`) — the INSTRUCTION/ACTIVITY
  sets landed with REQ-116 Phase 4, measured on the corona golden, and ACTION's
  `time` matches the `test_all_types` golden. Every ACTION also gets one in-context
  `ism_transition` node with no `nodeId`, in one of two shapes:
  - **Merged**, where the walk emits archetyped `ism_transition[atNNNN]` nodes, one
    per careflow step: the reference's node, matching the `test_all_types` golden
    (`OPTParser.handleActionIsmTransitions`). Its `min`/`max` are the first step's,
    and it holds `careflow_step` (each step's careflow-step code, with the
    `currentStates` that step admits), `current_state` (every step's states in step
    order, a state two steps admit listed twice, as the reference lists it) and
    `transition` (`|code` / `|value`). Three readings go beyond the golden:
    - a step that constrains no `careflow_step` still names one, its node id, which
      the FLAT codec gives a transition decoded under it, so `careflow_step` lists it
      by its node id, labelled with its name and localized names, where the
      reference fails on it;
    - a step that constrains no `current_state` adds nothing to that list, where the
      reference fails on it;
    - `careflow_step`'s `max` is 1 where the reference copies the step's `min` into
      it, and its `min` is the first step's that constrains one, else 0.
  - **Unconstrained**, where the walk emits none: `current_state` and `transition`,
    each with `|code` / `|value` inputs, and no `careflow_step`, as the reference's
    filter leaves it out; `min` 0, and the name `Ism_transition` in the in-context
    capitalisation. The `minimal_action_2` corpus template has this shape, and so
    does an ACTION whose archetyped transitions constrain no attribute of the
    transition, since those are pruned as empty containers. A step that constrains its
    `transition` alone is kept, so it is merged.

  The reference also writes a `_reason` family under the node, which neither shape
  gives a node of its own: the FLAT codec reads and writes it as the `_reason:N`
  underscore family (REQ-140).
- **Per-careflow-step `ism_transition[atNNNN]` nodes are kept** — beside the merged
  node, the builder still emits each archetyped transition with its own
  `current_state` / `careflow_step` leaves, where the reference's filter drops them
  (9 nodes in `test_all_types`; in the FLAT conformance template the steps are
  `transition` and `transition2`). Kept so that FLAT bodies keyed per step still decode and
  `rmpath` still matches a transition by its careflow step (REQ-121). Pinned by name
  in `TestREQ106ActionParity`.
- **ACTION `instruction_details` not emitted** — the `test_all_types` golden holds
  `instruction_details/activity_id` (STRING) and `…/instruction_id`
  (LOCATABLE_REF) under its ACTION; the builder models neither. Pinned by name in
  `TestREQ106ActionParity` (2 nodes).
- **`localizedName` / localized maps** — emitted for the compiled template's single
  document language only. The compiled bridge resolves every language to the
  document-language term, so no per-language override options are offered — they
  would relabel text without retranslating it; the reference's exact language
  packaging may differ.
- **Sibling `id` disambiguation** — **RESOLVED** by REQ-116 (Phases 3–4). Kept here
  because the mechanism is the reference contract three other entries refer to, and
  because two of its early readings were wrong in ways worth not repeating.

  `id` now derives from the template-level node name, `aqlPath` carries the name
  predicate, and both oracles reach exact structural parity — `Corona_Anamnese`
  230/230 nodes and `GECCO_Diagnose` 34/34, joining `constrain_test`'s 104/104 in
  the PROBE-075 matrix. `Build` no longer returns `ErrIDCollision` for any vendored
  template.

  **The mechanism, established against the reference (2026-07-29).** An earlier note
  here guessed that "EHRbase appends a disambiguating suffix". That was wrong as a
  description of the *primary* rule — checked across the upstream `corona_anamnese`,
  `multi_occurrence`, and `AlternativeEvents` WebTemplate goldens, **no** sibling
  group shares an `id` base with a numeric suffix, because each sibling pins a
  distinct name and never needs one. (An ordinal *does* exist as EHRbase's last
  resort where names run out — see § Ordinal fallback below, found in Phase 4.)
  EHRbase derives the `id` from a different source than this builder used to:

  - **EHRbase** uses the node's **template-level name** — the `name` attribute
    constrained to a fixed `C_STRING` in the OPT (`<item xsi:type="C_STRING">
    <list>Husten</list></item>`). Sibling archetype roots therefore differ naturally.
  - **This builder** uses the *archetype's* concept term, looked up by `NodeID()`
    (`idOf` → `termText`). Every sibling sharing an archetype id gets the **same**
    text, hence the collision.

  Worked example (counts verified against the vendored OPT, 2026-07-30 — an earlier
  note here said "five" sections): **four** `openEHR-EHR-SECTION.adhoc.v1` siblings in
  `Corona_Anamnese` are distinguished only by their pinned names — the reference emits
  `symptome`, `kontakt`, `risikogebiet`, `allgemeine_angaben`, where this builder
  repeats the shared concept term. The collision `Build` actually reports first is one
  level down: **eight** `OBSERVATION.symptom_sign_screening.v0` siblings under the
  Symptome section (ten across the template) all derive
  `screening-fragebogen_zur_symptomen_anzeichen`, where the reference emits
  `husten`, `schnupfen`, `heiserkeit`, … from their pinned names.

  **`aqlPath` is affected too, not just `id`.** The reference adds a **name
  predicate** — `/content[openEHR-EHR-SECTION.adhoc.v1,'Symptome']` — on the segment of
  **every node that pins a name**. The trigger is the pinned name alone, *not* sibling
  collision: an earlier note here said "where siblings share an archetype id", which is
  measurably wrong. `GECCO_Diagnose` predicates all three of its `/content` children
  although their archetype ids are **distinct**, and predicates a sole
  `CLUSTER.anatomical_location.v1` child. Across both oracles the names used in golden
  predicates are exactly the names the OPT pins — GECCO 6 distinct names (pinned on 7
  nodes: 'Unbekannte Diagnose' twice) / 6 used, Corona 24 / 24, no exceptions either way. The
  `corona_anamnese` golden carries **350** predicate segments over 213 paths;
  `constrain_test` carries **0** because it pins **no** name anywhere — that, not an
  absence of collision, is why PROBE-075 holds 104/104 without implementing any of this.
  Both path builders emit the predicates (Phase 3) and `idOf` prefers the pinned
  name (Phase 4), so all four layers are closed: read + expose the OPT node name
  (REQ-116), carry it through the compiled tree (REQ-111), emit name predicates in
  AQL paths (consumed by REQ-102 validation and REQ-053 FLAT paths), and derive `id`
  from it (REQ-106). Because the trigger is the pinned name, the path change reached
  **every** vendored OPT that pins one — 9 of the 58, i.e. 7 beyond the two oracles,
  led by `test_template_rename_node{,_2}` with 8 names each — not only the
  archetype-reuse templates. **PROBE-086 is unblocked.**

  The three upstream templates that reached the gap, and how each resolved:

  - `conformance-ehrbase.de.v0` — sibling ELEMENTs that both sanitise to `dv_text`
    under its ACTION (its nine archetype ids are all distinct, so this was *not* the
    archetype-reuse case, and no pinned name separates them either). Resolved by the
    ordinal fallback below; `Build` now succeeds, which is what PROBE-086 needs.
  - `Corona_Anamnese` — reused archetypes at two levels: four `SECTION.adhoc.v1`
    siblings, and eight `OBSERVATION.symptom_sign_screening.v0` siblings inside the
    Symptome section (ten across the template) all deriving
    `screening-fragebogen_zur_symptomen_anzeichen`. Now `symptome` / `kontakt` /
    `risikogebiet` / `allgemeine_angaben` and `husten` / `schnupfen` / … from their
    pinned names; 230/230 structural parity.
  - `GECCO_Diagnose` — the **silent** route: no sibling id collision, so `Build`
    always succeeded, but its golden carried 30 name-predicate segments over 24
    `aqlPath`s this builder never emitted — output that would fail reference parity
    with no error raised. Now 34/34 structural parity, with the two residuals below.

- **Ordinal fallback for siblings that still collide** — matches the reference on
  the evidenced case; recorded because it corrects the "there is no suffix rule"
  reading above. When two siblings sanitise to one `id` and neither pins a
  distinguishing name, the second and later claimants take the next **free**
  ordinal spelling: `dv_text`, `dv_text2`. Evidence is the vendored upstream FLAT
  corpus, whose bodies key the two ELEMENTs under `conformance-ehrbase.de.v0`'s
  ACTION as `…/conformance_action/dv_text` and `…/conformance_action/dv_text2` —
  that OPT carries the term text "DV_TEXT" ten times and pins no name on either
  node. The ordinal is a *last resort*, never the primary disambiguator: it appears
  in none of the corona / multi_occurrence / AlternativeEvents goldens, because
  there the pinned name already separates every sibling.

  Two edges of the fallback go beyond the vendored evidence and are this SDK's
  conservative reading, pinned by table test rather than by a golden:

  - **Next-free, not per-id counting** — siblings sanitising to `[x, x2, x]` rename
    the third to `x3`, never onto the already-taken `x2`.
  - **Synthesized in-context leaves participate** — a template ELEMENT whose name
    sanitises to an in-context id (e.g. an ELEMENT named "Language" beside the
    synthesized ENTRY `language` leaf) collides in the same sibling list; the later
    claimant (document order — data children first, in-context last) takes the
    ordinal. No vendored golden exercises this, so the reference's spelling for it
    is unverified; before REQ-116 this case failed with `ErrIDCollision`. The
    in-context leaf's `aqlPath` stays the bare RM attribute path either way — in
    the reference, `id` and `aqlPath` are independent derivations, so a deduped
    `id` with an un-suffixed `aqlPath` is the expected shape, and FLAT keys derive
    from the `id` chain alone.

- **ELEMENT value alternatives** — match the reference (`OPTParser.updateChoiceId`,
  `addAnyNode`, `pushProperties`; `Filter.preHandle`, `skip`). By the value
  alternatives an ELEMENT's template constrains:

  | The ELEMENT's value has | The builder emits |
  |---|---|
  | one alternative | a single leaf of its type at `<element>/value`, with the ELEMENT's `id`, `nodeId`, names and occurrences |
  | two alternatives, one DV_CODED_TEXT and one DV_TEXT, in either order | that leaf for the DV_CODED_TEXT, its `code` list open, plus an `other` TEXT input |
  | any other two or more alternatives | the ELEMENT as a node without inputs, with one child per alternative in template order: `id` `<type>_value`, the ELEMENT's `nodeId` and names, the alternative's own occurrences and inputs, `aqlPath` `<element>/value` |
  | none | the ELEMENT as a node with the reference's 17 children, DV_TEXT to DV_DATE in its order: no `nodeId`, `0..1`, the ELEMENT's names, and the inputs of an unconstrained value of the type (none outside the core subset) |

  The children share one `aqlPath`, as in the reference, so they are told apart by
  `id`. Two alternatives of one type take the sibling ordinal (`text_value`,
  `text_value2`). An unfilled slot for an ELEMENT archetype is no ELEMENT: it is
  pruned as any unfilled slot is, as the reference skips it. Pinned by
  `TestREQ106ElementValueAlternatives`, and against the `test_all_types` golden by
  `TestREQ106ChoiceParity`.

  One spelling **diverges** from the reference's code: an interval alternative,
  `DV_INTERVAL<DV_DATE_TIME>`, takes the `id` `interval_of_date_time_value`, the
  reference's own IPS test Web Template spelling, where its code writes
  `interval<dv_date_time>_value`. The reference's FLAT reader does not read that key.

- **Single-occurrence `EVENT` containers are lifted, not emitted** — matches the
  reference. An abstract `EVENT` constrained to at most one occurrence contributes
  no node of its own; its `events[…]` path segment and its in-context `time` leaf
  are retained and its children are lifted to the parent. A repeating `EVENT`, and
  any concrete `POINT_EVENT` / `INTERVAL_EVENT`, is kept as a node even at `max=1`.
  Both halves of the discriminator are needed: corona drops 14 EVENT nodes (all
  `max=1`) and keeps 2 (`max=-1`); `constrain_test` keeps 3 EVENT at `max=-1` **and**
  an INTERVAL_EVENT `a24_hour_average` at `max=1`; and the upstream FLAT corpus keys
  `any_event` (repeating) in 424 places, which this builder still emits.

- **GECCO golden marks in-context RM-attribute leaves `min=1`** — 14 nodes
  (`/language`, `/territory`, `/composer`, `/context/setting`, `/context/start_time`
  and the per-EVALUATION `encoding` / `language` / `subject` leaves): golden `1/1`
  versus this builder's `0/1`. **The golden is the outlier, not the builder** —
  `constrain_test` and `Corona_Anamnese`, vendored at the same commit, both report
  `0/1` and match exactly, and GECCO's OPT constrains no `existence` on those
  attributes. Plausibly generated by an older upstream. Tolerated as a fixture
  deviation with the count pinned at 14 in `TestStructuralParity`, so it cannot
  widen to cover a real regression. The `test_all_types` golden does the same: on
  its ACTION subtree the `encoding`, `language`, `subject` and `time` leaves, pinned
  by name in `TestREQ106ActionParity`.

- **GECCO golden gives a DV_TEXT leaf for a coded-text and free-text pair** — 1 node,
  `name_des_problems_der_diagnose` (`at0002`), whose value admits DV_TEXT then
  DV_CODED_TEXT: golden `rmType` DV_TEXT, where the builder, as the pinned reference
  code, merges the pair into the DV_CODED_TEXT leaf. **The golden is the outlier** —
  the reference's own `OPTParserTest.parseGECCODiagnose` lists that node as differing
  from its golden. Every other field matches; the count is pinned at 1 in
  `TestStructuralParity`.

## Input-level (contents beyond suffix/type)

- **`defaultValue`** — never emitted (the reference carries assumed values on several
  inputs).
- **`validation` ranges** — emitted for DV_COUNT (INTEGER, exclusive bounds
  normalised to inclusive as the reference does), DV_PROPORTION
  numerator/denominator (including the percent-kind–derived `>=100 <=100`
  denominator bound), and single-unit DV_QUANTITY magnitude, only when the
  constraint actually bounds a side (an unconstrained numeric emits no validation);
  **not** emitted for DV_DURATION per-field ranges (including the reference's
  `>=0` defaults), DV_QUANTITY `precision`, per-unit list validation, or
  multi-unit magnitude. Proportion kinds other than percent (e.g. unitary) derive
  no denominator bound — the fixture does not pin them.
- **Coded/ordinal list labels** — `value` and `ordinal` match; `label` is resolved from
  the at-code terms of the value's own archetype, its nearest enclosing archetype root, as
  the reference reads them (node names and descriptions are read the same way, and a local
  code that archetype does not define is left out of a coded list, as the reference leaves it
  out, even where an enclosing archetype defines it), but is **empty for external-terminology codes**
  (e.g. `openehr::433`), and `localizedLabels` / `localizedDescriptions` / per-item
  `termBindings` are not emitted.
- **`terminology`** — emitted for external bindings only (e.g. `openehr`). The
  archetype-internal `local` value is omitted, mirroring the reference (pinned by
  `TestInputParity`), which itself writes it on one input: the merged ACTION
  `careflow_step` (pinned by name in `TestREQ106ActionParity`).
- **Reference-set `terminology`** — for a `C_CODE_REFERENCE`, the code input's
  `terminology` is the one its code phrase names, else the one its `terminology:` URI
  names, read as `constraints.CodeReference.ReferenceSetTerminology` reads it:
  `terminology:SNOMED-CT` gives `SNOMED-CT`, as the reference does. The two part
  where the reference keeps more of the URI: it drops the scheme and keeps the rest
  as written (`OPTParser.removeSchema`), whatever the code phrase names. So
  `terminology:LOINC/2.73` gives the reference `LOINC/2.73` and the SDK `LOINC(2.73)`,
  `terminology:code24?subset=x` gives `code24?subset=x` and `code24`, and a URI of
  another scheme, such as `http://snomed.info/sct`, gives `//snomed.info/sct` and no
  `terminology`. A code phrase that names a terminology keeps it here, where the
  reference writes the URI's. None of the three vendored references holds a
  `C_CODE_REFERENCE`, so `TestInputParity` does not meet this; the cases are pinned by
  `TestCodedTextInput_REQ106_REQ103_ReferenceSetTerminology`.
- **`listOpen`** — emitted (`true`) for an open coded list: an empty constraint list, or
  the DV_CODED_TEXT leaf of a coded-text and free-text pair, in either order (the
  free-text `other` input admits values beyond the enumerated codes), mirroring the
  reference (pinned by `TestInputParity`).
- **Oracle input deltas** — a constrained DV_TEXT's TEXT input carries the C_STRING
  `list` and `listOpen` as the reference does, which closed 19 of Corona's 20 deltas.
  It also carries a C_STRING `pattern` that is not blank as `validation.pattern`, as
  the reference does; no vendored golden holds one.
  **Corona 1 delta, GECCO 1** remain: Corona's `medication_use` node `at0027` (golden
  `code:TEXT` + `value:TEXT`, ours `code:CODED_TEXT` open) and GECCO's
  `problem_diagnosis…/items[at0002]/value`, the merged coded-text leaf above (golden a
  TEXT input, open, and `other:TEXT`, from its DV_TEXT leaf; ours `code:CODED_TEXT`
  open and `other:TEXT`; the reference's code gives `value:TEXT`, `code:TEXT` open and
  `other:TEXT`, the unconstrained coded text of Corona's node); `constrain_test`
  remains 104/104 exact. Only the input signature differs at either node. The counts
  are pinned per fixture in `TestInputParity`, so the gap must shrink deliberately and
  cannot grow unnoticed.

## Scope

- **Archetype-reuse-under-slot templates** (e.g. `corona_anamnese`) — exportable as of
  REQ-116: `templatecompile` admits shared-path subtrees, the name predicate separates
  the reused siblings' paths, and the pinned name gives each a distinct web `id`. See
  § Sibling `id` disambiguation above for the verified mechanism.
- **Multiple value alternatives** — emitted as the reference does (§ ELEMENT value
  alternatives above). The reference's choice between an ELEMENT's value and a
  `null_flavour` the template constrains is not: the builder reads the value
  alternatives alone, so `Demonstration.v1`'s four ELEMENTs that constrain a DV_BOOLEAN
  value and a `null_flavour` still collapse into their DV_BOOLEAN leaf.
- **Byte parity** — field ordering and absent optional fields differ from the reference by
  design; only the structural surface above is guaranteed.
