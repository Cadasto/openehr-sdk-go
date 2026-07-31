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
extended with coded/ordinal list values and ordinals, `listOpen`, `terminology`,
temporal validation patterns, and numeric validation ranges — against **three**
vendored references since REQ-116 Phase 4: `constrain_test` **104/104** (exact,
node and input), `Corona_Anamnese` **230/230** and `GECCO_Diagnose` **34/34**
(exact structurally; their documented input and `min` deltas are pinned by count
below). The deltas below are the parts of the reference this slice deliberately
does **not** reproduce. Any change that makes a *pinned* field diverge is a test
failure, not a deviation.

## Node-level

- **`termBindings`, `annotations`, `inContext`** — not emitted. The reference tags
  RM-attribute leaves with `inContext: true` and carries per-node term bindings and
  UI annotations; this slice omits them (the leaves themselves, including their
  capitalized `name`, are emitted).
- **inContext coverage** — the fixed RM-attribute leaf table covers the container
  types the fixture exercises (COMPOSITION, EVENT_CONTEXT, the ENTRY types, EVENT
  variants). The reference also synthesizes ACTIVITY `timing` /
  `action_archetype_id` and ACTION `ism_transition` leaves; those are not emitted
  yet. *(Deferred: extend parity with a fixture exercising INSTRUCTION/ACTION.)*
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
  name (Phase 4), so all four layers are closed: parse + expose the OPT node name
  (REQ-100), carry it through the compiled tree (REQ-111), emit name predicates in
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

- **Ordinal fallback for siblings that still collide** — matches the reference; no
  divergence, recorded because it corrects the "there is no suffix rule" reading
  above. When two siblings sanitise to one `id` and neither pins a distinguishing
  name, the second and later claimants take an ordinal: `dv_text`, `dv_text2`.
  Evidence is the vendored upstream FLAT corpus, whose bodies key the two ELEMENTs
  under `conformance-ehrbase.de.v0`'s ACTION as
  `…/conformance_action/dv_text` and `…/conformance_action/dv_text2` — that OPT
  carries the term text "DV_TEXT" ten times and pins no name on either node. The
  ordinal is a *last resort*, never the primary disambiguator: it appears in none of
  the corona / multi_occurrence / AlternativeEvents goldens, because there the pinned
  name already separates every sibling.

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
  widen to cover a real regression.

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
  archetype at-code terms where present, but is **empty for external-terminology codes**
  (e.g. `openehr::433`), and `localizedLabels` / `localizedDescriptions` / per-item
  `termBindings` are not emitted.
- **`terminology`** — emitted for external bindings only (e.g. `openehr`). The
  archetype-internal `local` value is omitted, mirroring the reference (pinned by
  `TestInputParity`).
- **`listOpen`** — emitted (`true`) for an open coded list: an empty constraint list, or
  a DV_CODED_TEXT with a DV_TEXT alternative (the free-text `other` input admits values
  beyond the enumerated codes), mirroring the reference (pinned by `TestInputParity`).
- **Oracle input deltas (pre-REQ-116, now visible)** — extending `TestInputParity` to
  the two oracles in Phase 4 surfaced a gap this builder always had, unrelated to node
  naming: where the reference enumerates a constrained DV_TEXT's allowed values in the
  input `list`, this builder emits the bare `:TEXT` input. **Corona 20 deltas, GECCO 1**
  (`problem_diagnosis…/items[at0002]/value`, golden 2 inputs vs 1 — the plan predicted
  this one); `constrain_test` remains 104/104 exact. Structural parity is exact for all
  three, so only the input signature differs. The counts are pinned per fixture in
  `TestInputParity`, so the gap must shrink deliberately and cannot grow unnoticed.

## Scope

- **Archetype-reuse-under-slot templates** (e.g. `corona_anamnese`) — exportable as of
  REQ-116: `templatecompile` admits shared-path subtrees, the name predicate separates
  the reused siblings' paths, and the pinned name gives each a distinct web `id`. See
  § Sibling `id` disambiguation above for the verified mechanism.
- **Multiple value alternatives** — only the first value alternative is used, except the
  DV_CODED_TEXT + DV_TEXT pair, which is rendered as `code` + `other` inputs.
- **Byte parity** — field ordering and absent optional fields differ from the reference by
  design; only the structural surface above is guaranteed.
