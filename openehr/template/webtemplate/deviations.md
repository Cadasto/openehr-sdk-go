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
temporal validation patterns, and numeric validation ranges — at **104/104**
parity against the vendored `constrain_test` reference. The deltas below are the
parts of the reference this slice deliberately does **not** reproduce. Any change
that makes a *pinned* field diverge is a test failure, not a deviation.

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
- **Sibling `id` disambiguation** — not implemented; `Build` returns `ErrIDCollision`
  rather than emit ambiguous duplicate `id`s.

  **The mechanism, established against the reference (2026-07-29).** An earlier note
  here guessed that "EHRbase appends a disambiguating suffix". That is **wrong** —
  there is no suffix rule. Checked across the upstream `corona_anamnese`,
  `multi_occurrence`, and `AlternativeEvents` WebTemplate goldens, **no** sibling
  group shares an `id` base with a numeric suffix. EHRbase never needs to
  disambiguate because it derives the `id` from a different source than this builder
  does:

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
  As of REQ-116 Phase 3 both path builders emit the predicates: compiled paths and
  WebTemplate `aqlPath` carry `[archetype_id,'Name']` on every node that pins a name.
  GECCO reproduces its golden's 24 predicated paths exactly; what still differs is the
  four spurious `…/name` data leaves (Phase 4 task 2). `id` derivation is unchanged, so
  `Build(Corona_Anamnese)` still returns `ErrIDCollision` until Phase 4.

  Closing this therefore spans four layers — parse + expose the OPT node name
  (REQ-100, **landed**), carry it through the compiled tree (REQ-111, **landed**), emit
  name predicates in AQL paths (**landed**; consumed by REQ-102 validation and REQ-053
  FLAT paths), and switch `id` derivation to prefer it (REQ-106, open) — so it is a
  scoped feature, not a local fix. Because the trigger is the pinned name, the path change reaches **every** vendored
  OPT that pins one — 9 of the 58 vendored OPTs, i.e. 7 beyond the two oracles, led by
  `test_template_rename_node{,_2}` with 8 names each — not only the archetype-reuse
  templates.
  **PROBE-086 is blocked on it**, as is any WebTemplate whose siblings sanitise to one
  `id`. Three upstream templates reach the gap, by different routes:

  - `conformance-ehrbase.de.v0` — sibling ELEMENTs that both sanitise to `dv_text`
    under its ACTION (its nine archetype ids are all distinct, so this is *not* the
    archetype-reuse case). **Vendored and compile-tested** in this repo.
  - `Corona_Anamnese` — reused archetypes at two levels: four `SECTION.adhoc.v1`
    siblings, and eight `OBSERVATION.symptom_sign_screening.v0` siblings inside the
    Symptome section (ten across the template) all deriving
    `screening-fragebogen_zur_symptomen_anzeichen`. **Vendored with its reference
    WebTemplate** (REQ-116 plan Phase 0) and guarded twice: it compiles
    (`internal/templatecompile` oracle test) and `Build` returns `ErrIDCollision`
    (`req116_gap_test.go` pins the blocked state until REQ-116 flips it).
  - `GECCO_Diagnose` — the **silent** route: no sibling id collision, so `Build`
    succeeds today, but its golden carries 30 name-predicate segments over 24
    `aqlPath`s this builder never emits — output that would fail reference parity
    without any error being raised. Vendored with its golden and pinned by
    `req116_gap_test.go`; extending PROBE-075 parity to it (REQ-116 plan Phase 4
    task 4) is what turns the divergence into a failure.

    Measured against its golden on the PROBE-075 surface with predicates normalised
    away — i.e. **the residual once predicates land** — the path set matches exactly
    (`missing=0`), leaving three deltas:

    - **4 spurious `…/name` `DV_TEXT` leaves.** `build.go` walks the constrained
      `name` attribute and exports the pinned name as *data*; the reference carries it
      on the node and has no `…/name` child anywhere. A second manifestation of this
      same gap, invisible to `constrain_test`.
    - **14 `min`/`max` deltas** (`/language`, `/territory`, `/composer`,
      `/context/setting`, `/context/start_time` and the per-EVALUATION `encoding` /
      `language` / `subject` leaves): golden `1/1` vs this builder's `0/1`. **The
      golden is the outlier** — `constrain_test` and `Corona_Anamnese`, vendored at the
      same commit, both report `0/1`, and GECCO's OPT constrains no `existence` on
      those attributes. Plausibly generated by an older upstream. Phase 4 should
      document this as a fixture deviation, not change `min`.
    - **1 input-count delta** on `problem_diagnosis…/items[at0002]/value` (golden 2 vs
      1) — check against § Multiple value alternatives below before treating it as new.

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

## Scope

- **Archetype-reuse-under-slot templates** (e.g. `corona_anamnese`) — still unexportable,
  but **not** because of compilation: `templatecompile` admits shared-path subtrees, so
  these templates compile. `Build` fails with `ErrIDCollision` because each reused sibling
  derives the same web `id`. The fix is the template-level node name + name predicates
  (REQ-116) — see § Sibling `id` disambiguation above for the verified mechanism.
- **Multiple value alternatives** — only the first value alternative is used, except the
  DV_CODED_TEXT + DV_TEXT pair, which is rendered as `code` + `other` inputs.
- **Byte parity** — field ordering and absent optional fields differ from the reference by
  design; only the structural surface above is guaranteed.
