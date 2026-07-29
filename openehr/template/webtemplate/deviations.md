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

  Worked example — five `openEHR-EHR-SECTION.adhoc.v1` siblings in `corona_anamnese`:
  the reference emits `symptome`, `kontakt`, `risikogebiet`, `allgemeine_angaben`
  (from their names), where this builder would emit one repeated
  `screening-fragebogen_zur_symptomen_anzeichen`.

  **`aqlPath` is affected too, not just `id`.** Where siblings share an archetype id
  the reference adds a **name predicate** — `/content[openEHR-EHR-SECTION.adhoc.v1,'Symptome']`.
  The `corona_anamnese` golden carries **350** such predicates; `constrain_test`
  carries **0**, which is why PROBE-075 holds 104/104 today without implementing any
  of this. `templatecompile` currently emits no name predicates, and
  `openehr/template` does not parse or expose the OPT node name at all.

  Closing this therefore spans four layers — parse + expose the OPT node name
  (REQ-100), carry it through the compiled tree (REQ-111), emit name predicates in
  AQL paths (consumed by REQ-102 validation and REQ-053 FLAT paths), and switch `id`
  derivation to prefer it (REQ-106) — so it is a scoped feature, not a local fix.
  **PROBE-086 is blocked on it**, as is any WebTemplate for a template that reuses an
  archetype among siblings (`corona_anamnese`, `conformance-ehrbase.de.v0`).

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

- **Archetype-reuse-under-slot templates** (e.g. `corona_anamnese`) — unsupported: they
  produce duplicate compiled AQL paths that `templatecompile` rejects. See REQ-106 and
  ADR 0014 (a possible REQ-100/111 compiler follow-up).
- **Multiple value alternatives** — only the first value alternative is used, except the
  DV_CODED_TEXT + DV_TEXT pair, which is rendered as `code` + `other` inputs.
- **Byte parity** — field ordering and absent optional fields differ from the reference by
  design; only the structural surface above is guaranteed.
