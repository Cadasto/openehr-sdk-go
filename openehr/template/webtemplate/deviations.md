# WebTemplate export — documented deviations (REQ-106, PROBE-075)

Conformance against the EHRbase `openEHR_SDK` v2.3 reference is **structural, not
byte-exact** ([ADR 0014](../../../docs/adr/0014-webtemplate-reference-implementation-lock.md)).
PROBE-075 (`TestStructuralParity` + `TestInputParity` in this package) pins the
load-bearing surface — every node's `id`, `rmType`, `nodeId`, `aqlPath`,
`min`/`max`, and each input's `suffix`/`type` — at **104/104** parity against the
vendored `constrain_test` reference. The deltas below are the parts of the
reference this slice deliberately does **not** reproduce. Any change that makes
a *structural* field diverge is a test failure, not a deviation.

## Node-level

- **`termBindings`, `annotations`, `inContext`** — not emitted. The reference tags
  RM-attribute leaves with `inContext: true` and carries per-node term bindings and
  UI annotations; this slice omits them.
- **`localizedName` / localized maps** — emitted from the archetype terms, but only
  for languages the OPT actually defines; the reference's exact language packaging may
  differ.
- **Sibling `id` disambiguation** — not implemented. When two sibling nodes sanitise
  to the same `id`, EHRbase appends a disambiguating suffix; `constrain_test` contains
  no such collision, so the exact rule is unverified. Templates with colliding sibling
  names will currently emit duplicate `id`s. *(Deferred: derive the suffix rule from a
  fixture that exercises it.)*

## Input-level (contents beyond suffix/type)

- **`defaultValue`** — never emitted (the reference carries assumed values on several
  inputs).
- **`validation` ranges** — emitted for DV_COUNT (INTEGER) and DV_PROPORTION
  (numerator/denominator) and single-unit DV_QUANTITY magnitude; **not** emitted for
  DV_DURATION per-field ranges, DV_QUANTITY `precision`, per-unit list validation, or
  multi-unit magnitude.
- **Date/time `validation.pattern`** — not emitted (reference carries e.g.
  `yyyy-mm-ddTHH:MM:SS`).
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
