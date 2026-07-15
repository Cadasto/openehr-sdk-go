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

- **Encode** — a **clinical** datatype (`DV_*`) outside the core set is embedded as a
  `|raw` canonical fragment (lossless) rather than dropped; a non-`DV_` leaf (party /
  context / other RM attribute) is a documented skip. A container node that does not
  resolve to a `Locatable` is an error, not a skip.
- **Decode** — a FLAT/STRUCTURED key that does not resolve to a Web Template node
  returns [`ErrUnknownPath`](simplified.go); an unmapped datatype returns
  `ErrUnsupportedDatatype`; a missing **required** suffix is an error, never a coerced
  zero value.

Consequence: a payload that uses a not-yet-supported feature (below) is **rejected**,
not partially/silently accepted.

## Deferred features (Phase 6)

| Feature | Current behaviour | Lands in |
|---|---|---|
| `ctx/` context — **core supported**: `ctx/language`, `ctx/territory` (both mandatory on decode → `ErrMissingContext`), `ctx/composer_name` / `ctx/composer_self`, `ctx/time` (context `start_time`). | Emitted on encode; rebuilt on decode. | landed (Task 6) |
| `ctx/` context — **rest deferred**: `setting`, `category`, participations, `health_care_facility`, `work_flow_id`, composer `external_ref` (`composer_id` / `id_namespace` / `id_scheme`), `end_time`, `location`, `other_context`. | Not emitted; any such `ctx/*` key is rejected on decode (`ErrUnknownPath`). Setting/category are platform defaults or need terminology resolution. | Phase 6 |
| `_`-prefixed optional RM attributes (`_uid`, `_normal_range/…`) | Not emitted; rejected on decode. | Phase 6 |
| `\|raw` escape hatch (canonical fragment for exotic datatypes) | Supported both directions: encode emits `\|raw` for non-core `DV_*`; decode accepts any `\|raw` fragment (must carry `_type`). | landed (Task 6) |
| `\|other` open-value-set free text for `DV_CODED_TEXT` | Not implemented. | Phase 6 |
| `.schema`-suffixed media types on input | Not accepted. (Canonical types only; see [simplified.go](simplified.go).) | Phase 6 |
| Non-`DV_` leaves (party/`subject`, other RM leaves) on encode | Skipped (not an error), pending the `ctx/`/`_`-attr work. | Phase 6 |

## Deviations

- **`LOCATABLE.name` on decode** — reconstructed intermediate/leaf nodes carry `_type`
  and `archetype_node_id` only; the mandatory `name` is not repopulated from the Web
  Template. Round-trip does not depend on it (`rmpath` re-resolves by
  `archetype_node_id`). Full `name` population lands with the `ctx/`/name completion
  (Phase 6). Until then, decoded compositions are **format-idempotent**, not guaranteed
  canonically equal to an upstream canonical instance.

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

## Conformance

Structural conformance against a vendored upstream trio (**PROBE-076**) is **deferred**
to Phase 7; when it lands, any residual byte-level differences (id sanitisation,
`version`, field ordering) are recorded here.
