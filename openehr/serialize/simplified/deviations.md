# Simplified Formats — documented deviations & deferrals (REQ-053)

Parity with the openEHR *Simplified Formats* (ITS-REST, STABLE) is **structural, not
byte-exact**. This file records where `openehr/serialize/simplified` intentionally
deviates from, or has not yet implemented, part of the spec. Each entry says what the
current behaviour is and where the full behaviour lands.

Status legend: **Deviation** = deliberate, permanent-ish choice; **Deferred** = not yet
implemented, scheduled for a later phase of the
[Phase 3 plan](../../../docs/plans/2026-07-14-flat-structured-codecs.md).

## Strict, fail-loud posture

The codec never succeeds while silently losing or altering data (REQ-053 is
semantics-preserving). Concretely:

- **Encode** — a **clinical** datatype (`DV_*`) is emitted as its FLAT suffix form only
  when that form fully captures the value; a **decorated** value (carrying `normal_range`,
  `magnitude_status`, `accuracy`, `mappings`, … — anything outside the datatype's captured
  keys) and any datatype outside the core set are embedded as a lossless `|raw` canonical
  fragment rather than partially/silently dropped. A `DV_*` leaf the Web Template gives no
  input descriptors for (e.g. `DV_URI`, `DV_MULTIMEDIA`, `DV_PARSABLE`) is still emitted (as
  bare/suffixed or `|raw`), not skipped. A non-`DV_` leaf (party / context / other RM
  attribute) is a documented skip. A container node that resolves to a
  non-`Locatable` RM object (e.g. `EVENT_CONTEXT`) is recursed via the enclosing Locatable
  ancestor, not dropped. A typed-nil RM pointer is treated as an absent leaf (skipped).
- **Decode** — a key that does not resolve to a Web Template node returns
  [`ErrUnknownPath`](simplified.go); an unmapped datatype, a suffix outside the datatype's
  allowlist (e.g. a `\|unitt` typo), a misused `\|raw`/`\|other`, or a `\|other` on a closed
  value-set return `ErrUnsupportedDatatype`; a missing **required** suffix is an error, not
  a coerced zero value; trailing JSON after the object and an out-of-bound/over-budget
  `:index` are rejected.

Consequence: a payload that uses a not-yet-supported feature (below) is **rejected**,
not partially/silently accepted.

## Deferred features (Phase 6)

| Feature | Current behaviour | Lands in |
|---|---|---|
| `ctx/` context — **core supported**: `ctx/language`, `ctx/territory` (both mandatory on decode → `ErrMissingContext`), `ctx/composer_name` / `ctx/composer_self`, `ctx/time` (context `start_time`). | Emitted on encode; rebuilt on decode. | landed (Task 6) |
| `ctx/` context — **rest deferred**: participations, `health_care_facility`, `work_flow_id`, composer `external_ref` (`composer_id` / `id_namespace` / `id_scheme`), `end_time`, `location`, `other_context`. | Not emitted on encode (source values dropped); any such `ctx/*` key is rejected on decode (`ErrUnknownPath`). These are optional, so their absence does not break OPT-validity. `setting`, `category`, `composer` are RM-mandatory and **defaulted** on `WithTemplate` decode (see the RM-mandatory-completion deviation) — valid, but a non-default source value is not round-tripped. | Phase 6 |
| Datatypes — **first-class** suffix form: `DV_TEXT`, `DV_CODED_TEXT`, `DV_DATE_TIME`, `DV_DATE`, `DV_TIME`, `DV_QUANTITY`, `DV_COUNT`, `DV_BOOLEAN`, `DV_DURATION`, `DV_URI`, `DV_EHR_URI`, `DV_ORDINAL`, `DV_PROPORTION`, `DV_IDENTIFIER`. Any other `DV_*`, or a decorated instance of the above, rides `\|raw`. | Both directions. | landed (Task 6) |
| `_`-prefixed optional RM attributes (`_uid`, `_normal_range/…`, `\|magnitude_status`, `\|accuracy`) — **first-class** suffix decomposition. | Not decomposed into suffixes; a value carrying them is emitted losslessly as `\|raw` instead (no data loss). First-class suffix form deferred. | Phase 6 |
| `\|raw` escape hatch (canonical fragment for exotic/decorated datatypes) | Supported both directions: encode emits `\|raw` for non-core or decorated `DV_*`; decode accepts a `\|raw` fragment that carries a string `_type` and is not combined with any other suffix. `\|raw` is **not** checked for RM-type compatibility with the leaf constraint (an explicit bypass) — a documented relaxation. | landed (Task 6) |
| `\|other` open-value-set free text for `DV_CODED_TEXT` | Supported: a `DV_TEXT` at a `DV_CODED_TEXT` leaf whose Web Template input is `listOpen` encodes to `\|other`; decode maps `\|other` back to `DV_TEXT`, requiring `listOpen` and rejecting `\|other`+`\|code`. | landed (Task 6) |
| `.schema`-suffixed media types on input | Not accepted. (Canonical types only; see [simplified.go](simplified.go).) | Phase 6 |
| Non-`DV_` leaves (party/`subject`, `language`, `encoding`, other RM leaves) on encode | Skipped on encode (source value dropped). The RM-mandatory ones (`subject`, `language`, `encoding`) are **defaulted** on `WithTemplate` decode (PARTY_SELF / ctx language / UTF-8) so the result validates; a non-default source value is not round-tripped. | Phase 6 |

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
  `start_time` and `subject` becomes `PARTY_SELF`. Without `WithTemplate`, decode omits them
  (format-idempotent only).

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
- **`:index` bound** — a FLAT `:index` is capped at `maxRepeatIndex` (100 000) on
  decode/interconversion so a hostile key (`node:1000000000`) cannot force an unbounded
  allocation; an out-of-range index is `ErrUnknownPath`.

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
It does **not** yet compare emitted FLAT/STRUCTURED byte-for-byte against vendored upstream
simplified output — a documented follow-up needing those fixtures.
