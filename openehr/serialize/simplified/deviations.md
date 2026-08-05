# Simplified Formats — documented deviations & deferrals (REQ-053)

Parity with the openEHR *Simplified Formats* (ITS-REST, STABLE) is **structural, not
byte-exact**. This file records where `openehr/serialize/simplified` intentionally
deviates from, or has not yet implemented, part of the spec. Each entry says what the
current behaviour is and where the full behaviour lands.

Status legend: **Deviation** = deliberate, permanent-ish choice; **Deferred** = not yet
implemented — residual scope tracked by the
[simplified-formats umbrella plan](../../../docs/plans/2026-06-23-simplified-formats.md)
(the [Phase 3 plan](../../../docs/plans/archive/2026-07-14-flat-structured-codecs.md) that
built this package is done and archived).

## Strict, fail-loud posture

The codec never succeeds while silently losing or altering data (REQ-053 is
semantics-preserving). Concretely:

- **Encode** — a **clinical** datatype (`DV_*`) is emitted as its FLAT suffix form only
  when that form fully captures the value; a **decorated** value (carrying `normal_range`,
  `other_reference_ranges`, `mappings`, a non-`openehr` `normal_status`, … — anything outside
  the datatype's captured keys) and any datatype outside the core set are embedded as a
  lossless `|raw` canonical
  fragment rather than partially/silently dropped. A `DV_*` leaf the Web Template gives no
  input descriptors for (e.g. `DV_URI`, `DV_MULTIMEDIA`, `DV_PARSABLE`) is still emitted (as
  bare/suffixed or `|raw`), not skipped — including the in-context `ACTIVITY.timing`
  (`DV_PARSABLE`), which rides `|raw` when populated now that `rmpath` resolves it (REQ-121). A non-`DV_` leaf (party / context / other RM
  attribute) is a documented skip. A container node that resolves to a
  non-`Locatable` RM object (e.g. `EVENT_CONTEXT`) is recursed via the enclosing Locatable
  ancestor, not dropped. A typed-nil RM pointer is treated as an absent leaf (skipped).
  A **composer** the `ctx/` short forms cannot carry — `PARTY_RELATED`, or a
  `PARTY_IDENTIFIED` without a `name` — is `ErrUnsupportedDatatype`, not an omission
  (omitting it would let a `WithTemplate` decode default the composer to `PARTY_SELF`,
  a silent type substitution).
- **Decode** — a key that does not resolve to a Web Template node returns
  [`ErrUnknownPath`](simplified.go); an unmapped datatype, a suffix outside the datatype's
  allowlist (e.g. a `\|unitt` typo), a misused `\|raw`/`\|other`, or a `\|other` on a closed
  value-set return `ErrUnsupportedDatatype`; a missing **required** suffix is an error, not
  a coerced zero value; trailing JSON after the object and an out-of-bound/over-budget
  `:index` are rejected.

Consequence: a payload that uses a not-yet-supported feature (below) is **rejected**,
not partially/silently accepted.

## Deferred features

| Feature | Current behaviour | Lands in |
|---|---|---|
| `ctx/` context — **core supported**: `ctx/language`, `ctx/territory` (both mandatory on decode → `ErrMissingContext`), `ctx/composer_name` / `ctx/composer_self`, `ctx/time` (context `start_time`), `ctx/setting\|code` + `ctx/setting\|value` (context `setting`; the `openehr` terminology is implied, not written). | Emitted on encode; rebuilt on decode. The **all-zero** setting writes nothing (non-pointer field — "unset" and "zero" coincide, the CODE_PHRASE precedent); a **populated** setting the pair cannot carry — a non-`openehr` (or empty) terminology, extras beyond code+value (`mappings`, `formatting`, a `preferred_term`, …), or a value without a code — is `ErrUnsupportedDatatype` naming `ctx/setting`, not an omission (omitting it would let a `WithTemplate` decode substitute the `238 other care` default silently, the composer PARTY_RELATED stance). On decode one half of the pair without the other is refused naming the missing key. | landed (Task 6; `setting` 2026-08-05, REQ-053 amended) |
| Composition metadata in the reference's **real-path** spelling (`<root>/language\|code`, `<root>/territory\|code`, `<root>/composer\|name`, `<root>/composer_self`, `<root>/context/start_time`) | **Accepted on decode**, normalised onto the `ctx/` short forms ([ADR 0015](../../../docs/adr/0015-flat-metadata-spelling.md)); **never emitted** — `ctx/` is the only output spelling. A `\|terminology` witness is checked against the terminology the `ctx/` form implies and then discarded; a mismatch, or two spellings of one field that disagree, is an error rather than a precedence rule — including a composite (object/array) value, which no `ctx/` field can hold and which is refused before it can be compared. `composer_self: true` beside a composer **name** (either spelling) is refused on the same grounds: they are mutually exclusive representations of one RM attribute, and the `PARTY_SELF` branch would silently drop the name (`composer_self: false` beside a name is fine). Respellings only: `context/setting\|code` / `\|value` **are** aliases since 2026-08-05 (the amended REQ-053 closed the `ctx/setting` emission gap ADR 0015 had left open), with `context/setting\|terminology` as an `openehr` witness; the composer's `external_ref` remains the one non-alias (see § `ctx/` context). | landed (ADR 0015; `setting` alias 2026-08-05) |
| `ctx/` context — **rest deferred**: the `ctx/` short forms for `work_flow_id` and the composer `external_ref` (`composer_id` / `id_namespace` / `id_scheme`). The EVENT_CONTEXT optionals are **not** in this row — see the next one. | Not emitted on encode (those source values are dropped); any such `ctx/*` key is rejected on decode (`ErrUnknownPath`). All are optional, so nothing is silently substituted on decode. Note the fields that do NOT belong here: `category` is a template-constrained Web Template leaf and round-trips via its own path; a composer **name** round-trips via `ctx/composer_name` (only the external ref is lost); `setting` left this row on 2026-08-05 (§ core supported above); template-constrained `other_context` content rides its Web Template paths, and does so end-to-end since `rmpath` resolves `EVENT_CONTEXT.other_context` (REQ-121) — before that the paths existed in the Web Template but the encoder dropped the data. | Deferred |
| **EVENT_CONTEXT optionals** — participations, `health_care_facility`, `end_time`, `location`. Listed here because a reader looking for them expects a deferred `ctx/` row, but that is the wrong shelf: they are not a `ctx/` spelling gap. | Not emitted on encode; the ITS `ctx/` sketches for them (`ctx/participation…`, `ctx/health_care_facility…`, `ctx/end_time`, `ctx/location`) are refused on decode (`ErrUnknownPath`). [ADR 0016](../../../docs/adr/0016-event-context-optionals-underscore-spelling.md) assigns them to the underscore grammar under the real `context` segment, so they arrive with that grammar rather than as new `ctx/` fields. | [REQ-140](../../../docs/specifications/wire.md#req-140--underscore-prefixed-rm-attributes) |
| Datatypes — **first-class** suffix form: `DV_TEXT`, `DV_CODED_TEXT`, `DV_DATE_TIME`, `DV_DATE`, `DV_TIME`, `DV_QUANTITY`, `DV_COUNT`, `DV_BOOLEAN`, `DV_DURATION`, `DV_URI`, `DV_EHR_URI`, `DV_ORDINAL`, `DV_PROPORTION`, `DV_IDENTIFIER`. Any other `DV_*`, a decorated instance of the above, or a **substituted subtype** (the value's dynamic type differs from the leaf type, e.g. `DV_EHR_URI` at a `DV_URI` leaf), rides `\|raw` — with **one carve-out** (REQ-053, amended 2026-08-05): a **fully-captured `DV_CODED_TEXT` at a `DV_TEXT`-typed leaf** rides the DV_CODED_TEXT suffix set (`\|code` + `\|value` + `\|terminology`, `\|formatting` where populated; **no bare key**), matching the reference's `dv_coded_text_as_dv_text` corpus shape; on decode, `\|code` at a `DV_TEXT` leaf re-selects the DV_CODED_TEXT builder under that type's own rules (`\|value` required). A *decorated* coded text whose extras the suffixes cannot carry (`mappings`, a `preferred_term`, …) still rides `\|raw` stamped `DV_CODED_TEXT`, and every **other** substituted subtype keeps riding `\|raw`. This changed emitted bytes for the fully-captured substituted value only (previously a `\|raw` fragment). | Both directions. | landed (Task 6; substitution carve-out 2026-08-05) |
| Optional `DV_ORDERED` / `DV_QUANTIFIED` / `DV_AMOUNT` attributes — **first-class** suffix form: `\|magnitude_status`, `\|normal_status`, `\|accuracy`, `\|accuracy_is_percent`, `\|precision`, `\|units_system`, `\|units_display_name`, and `DV_TEXT`'s `\|formatting`. | Supported both directions. **This changed emitted bytes:** a value carrying one of these previously rode `\|raw` as a whole and now rides the suffix form. An *undecorated* value is byte-identical to before — an absent attribute writes no suffix. `\|normal_status` carries the bare ordinal code and decode rebuilds it in the implied `openehr` terminology, so a status coded elsewhere still rides `\|raw`. On decode the pass-through suffixes are checked for **JSON kind** (`\|accuracy` a number, `\|accuracy_is_percent` a boolean, …) so a malformed value is refused naming the offending FLAT key rather than escaping as a canonical-path error from `canjson`; that refusal deliberately carries **no** sentinel — it is a payload defect, not a modelled gap. The check admits everything `canjson` admits and so adds no strictness: a **quoted** number (`\|accuracy: "0.5"`) still decodes, since `canjson` parses a numeric string into `Real` / `Integer`. The RM's own value constraints (`\|magnitude_status` against its code set, …) are left to the validation package. | landed (PROBE-086 ratchet) |
| Remaining `_`-prefixed / uncaptured RM attributes (`_uid`, `normal_range`, `other_reference_ranges`, `mappings`, …) — **first-class** suffix decomposition. | Not decomposed into suffixes; a value carrying them is emitted losslessly as `\|raw` instead (no data loss). First-class suffix form deferred. | Deferred |
| `\|raw` escape hatch (canonical fragment for exotic/decorated datatypes) | Supported both directions: encode emits `\|raw` for non-core or decorated `DV_*`; decode accepts a `\|raw` fragment that carries a string `_type` and is not combined with any other suffix; encode stamps the fragment with the value's **dynamic** type when it can classify it. On decode, `\|raw` is **not** checked for RM-type compatibility with the leaf constraint (an explicit bypass) — a documented relaxation. | landed (Task 6) |
| `\|other` open-value-set free text for `DV_CODED_TEXT` | Supported: an **undecorated** `DV_TEXT` at a `DV_CODED_TEXT` leaf whose Web Template input is `listOpen` encodes to `\|other`; decode maps `\|other` back to `DV_TEXT`, requiring `listOpen` and rejecting `\|other` combined with **any** other suffix. `\|other` carries the value alone, so a decorated `DV_TEXT` (a `formatting`, a `hyperlink`, …) is not expressible in this form and rides `\|raw` instead. | landed (Task 6) |
| `.schema`-suffixed media types on input | Not accepted. (Canonical types only; see [simplified.go](simplified.go).) | Deferred |
| `CODE_PHRASE` leaves (ENTRY `language` / `encoding`) — the reference emits these as leaves in their own right, under the same `\|code` + `\|terminology` pair a `DV_CODED_TEXT`'s `defining_code` uses. | Supported both directions. The **all-zero** value writes nothing (the field is non-pointer, so "unset" and "zero" coincide and an unconditional emit would put blank leaves on every `ctx/`-decoded composition); a `preferred_term` or decorated `TERMINOLOGY_ID` rides `\|raw`. A **partly-populated** value — an empty `code_string` beside a non-empty `TERMINOLOGY_ID` — also rides `\|raw`: the empty-code skip would otherwise drop it silently, so it is deliberately not treated as captured (PR #86 review round 3). | landed (PROBE-086 ratchet) |
| Other non-`DV_` leaves (party/`subject`, remaining RM leaves) on encode | Skipped on encode (source value dropped). `subject` is **defaulted** on `WithTemplate` decode (PARTY_SELF) so the result validates; a non-default source value is not round-tripped. | Deferred |

## Deviations

- **`LOCATABLE.name` on decode** — the FLAT/STRUCTURED formats do not carry names, and the
  Web Template collapses the HISTORY / ITEM_STRUCTURE wrappers, so decode cannot name every
  node from the WT alone. Passing [`WithTemplate(compiled)`](simplified.go) repopulates the
  mandatory `name` on every reconstructed node from the archetype terminology (keyed by the
  compiled aqlPath); without it, nodes are unnamed and the round-trip is merely
  **format-idempotent**. Names never leak into FLAT, so idempotence is preserved either way.

- **RM-mandatory attributes not carried by FLAT — completed on `WithTemplate` decode.** The
  formats omit several RM-mandatory attributes that are neither clinical-data leaves nor names
  (`HISTORY.origin`, `EVENT.time`, `ENTRY.language`/`.encoding`/`.subject`,
  `EVENT_CONTEXT.setting`, `COMPOSITION.category`/`.composer`, `INTERVAL_EVENT.math_function`/
  `.width`). With `WithTemplate`, decode now completes them from `ctx/` defaults + RM
  conventions (`rminfo.RequiredAttributes` drives the walk), so the decoded composition
  **validates against the OPT** (verified over the vendored corpus by PROBE-076's conformance
  leg and `names_test.go`). These are **synthesised defaults**, not recovered data — the
  formats never carried them, so e.g. every `EVENT.time`/`HISTORY.origin` takes the context
  `start_time` and `subject` becomes `PARTY_SELF`. **Qualifier:** `EVENT.time` and
  `HISTORY.origin` have no source other than `ctx/time` — a payload without `ctx/time`
  decodes successfully but does not validate when the template carries HISTORY/EVENT nodes
  (pinned by `names_test.go`). Without `WithTemplate`, decode omits names and defaults
  entirely (format-idempotent only). **One completion is visible on re-encode** (since
  2026-08-05, the amended REQ-053): the synthesised `EVENT_CONTEXT.setting` default
  (`238 other care`) re-encodes as `ctx/setting\|code` + `\|value` like any populated
  setting, so a `WithTemplate` decode of a body that carried no setting re-encodes with
  exactly those two keys gained — a *faithful* encoding of the completed composition, not
  drift; an OPT-free decode synthesises nothing and stays byte-identical (both pinned by
  `TestSettingWithTemplateDefaultRoundTrip`). The other completions stay invisible on
  re-encode: names never leak into FLAT, and origin/subject/encoding have no emitted
  surface. (The former deviation here — "`EVENT_CONTEXT.setting` is dropped on encode",
  once the one in-context attribute this codec lost outright, and the PROBE-086 census's
  one documented waiver — was closed by that same amendment: encode emits the pair, decode
  accepts either spelling, and the real path `…/context/setting` stays deliberately
  unresolvable in `rmpath` so the value cannot double-spell.)

- **Empty repeat instances are not representable in FLAT** — an instance of a repeatable
  node whose subtree contributes no FLAT keys (all leaves below it absent) is omitted on
  encode and does **not** consume an `:index`; later instances close ranks. Stamping the
  index by RM list position instead would emit a sparse sequence (`:0`,`:2`) that the
  decoder rejects as phantom gap-fill, breaking `MarshalFlat → UnmarshalFlat` on a valid
  composition. Consequence: a composition with such an empty instance round-trips
  **minus that instance** — the one place encode narrows the composition rather than
  erroring, because the format simply has no spelling for "an instance with no values".

- **`ITEM_TREE` vs `ITEM_LIST` on decode** — the Web Template collapses `ITEM_STRUCTURE`
  nodes, so the concrete subtype is inferred from the child aqlPath attribute:
  `item` → `ITEM_SINGLE`, `rows` → `ITEM_TABLE`, `items` → `ITEM_TREE`. `ITEM_TREE` and
  `ITEM_LIST` both use `items` and are indistinguishable from the path alone, so `items`
  defaults to `ITEM_TREE`. This is round-trip-preserving (the leaf values and their
  paths are identical); it can differ from an upstream canonical that used `ITEM_LIST`.

## Implementation notes (not deviations, but worth recording)

- **Integer precision** — FLAT/STRUCTURED JSON is decoded with `json.Number`
  (`UseNumber`), so a `DV_COUNT` magnitude above 2^53 is preserved exactly through
  decode and through OPT-free interconversion rather than being rounded via `float64`.
- **Zero-valued mandatory in-context attributes emit empty leaves** — an in-context RM
  attribute that is RM-**mandatory** and value-typed (a non-pointer struct, e.g. `EVENT.time`)
  always resolves, so a composition that left it at its zero value emits the leaf with an empty
  value rather than omitting it: the encoder cannot tell "unset" from "set to the zero value"
  for a non-pointer field. Only reachable with RM-invalid input — a valid composition populates
  it, and a `WithTemplate` decode synthesises it (§ Deviations, RM-mandatory attributes).

- **`:index` strictness** — a FLAT `:index` must be canonically spelled (`0`, `1`, …;
  negative, `+`, or zero-padded spellings are rejected — they would collide with other
  keys), must not be sparse (a gap would fabricate a phantom empty instance), and is capped
  at `maxRepeatIndex` (see `flat_decode.go`) with a total decoded-node budget on top, so a
  hostile key cannot force an unbounded allocation. Violations are `ErrUnknownPath`.

- **OPT-free `FlatToStructured` → `StructuredToFlat` normalises `:index`** — STRUCTURED is
  arrays-always (spec), and interconversion has no OPT, so the back-conversion cannot tell
  a single-cardinality leaf (no `:index` in FLAT) from a one-element repeatable (`:0`); it
  emits `:0` on both. The result is valid-but-verbose FLAT that decodes to the same
  composition (the redundant `:0` on a max=1 node is ignored on decode), so interconversion
  is **semantics-preserving, not byte-identical**. PROBE-076 asserts the semantic form
  (decode + re-encode equals the original FLAT).

## Conformance

**PROBE-076** (landed) exercises the codec over the vendored EHRbase `Test_dv_*` corpus
(OPT + canonical composition) — 24 pass, 1 skip. It asserts **round-trip idempotence**
(FLAT/STRUCTURED/interconversion) **and OPT-conformance**: when the source composition is
itself OPT-valid, a `WithTemplate` decode must also validate against the OPT. The conformance
leg catches dropped/mistyped leaves that idempotence alone (a symmetric omission) would miss.
It does **not** compare emitted FLAT/STRUCTURED against vendored upstream simplified
output — that comparison is **PROBE-086** (Implemented, Sandbox): its harness at
`testkit/conformance/webtemplate/` round-trips the corpus vendored at
`testkit/cassettes/flat-conformance/` over the modelled subset, with the refusal
inventory in that package's `SKIPPED.md`.

**Archetype-reuse siblings (REQ-116 residual).** REQ-116 made templates that reuse one
archetype among siblings *exportable* (distinct WebTemplate `id`s via the pinned
template-level name; name-predicated `aqlPath`s), and this codec handles the pinned
predicate on the paths it consumes: decode strips it when rebuilding lookup keys
(`bareAQLPath`, keyed consistently across `resolveLeaf` / `placeLeaf` / the name index),
and encode strips it before rmpath resolution. A **sole** pinned claimant of its bare
path — every pinned node in `GECCO_Diagnose`, and pinned ELEMENTs generally — round-trips,
with `WithTemplate` decode repopulating the **pinned** name (`flat_pinned_test.go`). What
does **not** round-trip yet is *reused siblings*: several pinned siblings sharing one bare
path (corona's four `SECTION.adhoc.v1`).

- **Encode** — the bare relPath answers to *every* reused sibling: with several
  instances present a `Max==1` node fails on the rmpath ambiguity, but with **one**
  instance present (the common partially-filled composition) each sibling's Web Template
  node would resolve that same instance and emit its data under **every** sibling's FLAT
  id — silent misattribution, no error. Encode therefore **refuses** whenever data
  resolves at a reused sibling (`refuseReusedSibling`, wrapping
  `rmpath.ErrPathAmbiguous`; pinned in `flat_pinned_test.go`, which also pins that a
  composition with no data in the reused region still encodes). Disambiguating needs
  the runtime `name/value` carried into resolution — exactly what the stripped
  predicate expressed — restricted to genuinely ambiguous nodes.
- **Decode** — sibling FLAT ids map to one bare path, so their instances would collapse
  onto one list slot (both default to index 0), landing different siblings' leaves in
  **one** RM instance. Decode therefore **refuses** any key that walks through a reused
  sibling (`ErrUnknownPath`, "reused siblings"; `ambiguousBarePaths` precomputes the
  affected paths, `flat_pinned_test.go` pins the refusal on the corona oracle) — a loud
  no rather than a silent merge. The name index likewise falls back to the shared
  archetype rubric for the bare spelling. Fixing both needs the FLAT segment identity
  (which sibling's id the key walked through) carried into placement — the information
  the stripped predicate expressed.

Reused-sibling FLAT is therefore **fail-loud in both directions** — nothing silently
misattributes data — and closing it is owned by the PROBE-086 adapter work (the upstream
corpus template pins no names, so its parity does not depend on this residual).
