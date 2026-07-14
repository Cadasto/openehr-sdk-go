# WebTemplate export — documented deviations (REQ-106, PROBE-075)

Conformance against the EHRbase `openEHR_SDK` v2.3 reference is **structural, not
byte-exact** ([ADR 0014](../../../docs/adr/0014-webtemplate-reference-implementation-lock.md)).
PROBE-075 (`TestStructuralParity` + `TestInputParity` in this package) pins the
load-bearing surface — every node's `id`, `rmType`, `nodeId`, `aqlPath`,
`min`/`max`, and each input's `suffix`/`type` extended with coded/ordinal list
values and ordinals, temporal validation patterns, and numeric validation
ranges — at **104/104** parity against the vendored `constrain_test` reference.
The deltas below are the parts of the reference this slice deliberately does
**not** reproduce. Any change that makes a *structural* field diverge is a test
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
- **Sibling `id` disambiguation** — not implemented. When two sibling nodes sanitise
  to the same `id`, EHRbase appends a disambiguating suffix; `constrain_test` contains
  no such collision, so the exact rule is unverified. Rather than emit ambiguous
  duplicate `id`s, `Build` returns `ErrIDCollision` for such templates. *(Deferred:
  derive the suffix rule from a fixture that exercises it.)*

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

## Scope

- **Archetype-reuse-under-slot templates** (e.g. `corona_anamnese`) — unsupported: they
  produce duplicate compiled AQL paths that `templatecompile` rejects. See REQ-106 and
  ADR 0014 (a possible REQ-100/111 compiler follow-up).
- **Multiple value alternatives** — only the first value alternative is used, except the
  DV_CODED_TEXT + DV_TEXT pair, which is rendered as `code` + `other` inputs.
- **Byte parity** — field ordering and absent optional fields differ from the reference by
  design; only the structural surface above is guaranteed.
