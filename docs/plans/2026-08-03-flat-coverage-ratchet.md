# Plan — PROBE-086 coverage ratchet (upstream FLAT datatype coverage)

**Date:** 2026-08-03
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-053](../specifications/wire.md#req-053) (FLAT/STRUCTURED codecs — datatype coverage + the decode input surface amended by [ADR 0015](../adr/0015-flat-metadata-spelling.md)), [REQ-121](../specifications/rm-functions.md#req-121--locatable-path-read-access) (the rmpath resolution each new leaf needs). No new REQ: REQ-053 pins the grammar to the upstream *Simplified Formats* spec rather than enumerating a closed datatype set, so widening coverage is **conformance to an existing contract**, not new normative text — nothing here needs an `sdd-specify` gate.
**Probes:** [PROBE-086](../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) (the measurement this plan moves), PROBE-076 (must stay green)
**Implementation:** planned
**Depends on:** landed `openehr/serialize/simplified/`, `openehr/rm/rmpath/`, `openehr/template/webtemplate/`; the PROBE-086 harness and its census
**Defers:** `ctx/setting` emission (the one PROBE-086 hold-out that waives a real encode-side drop — Phase 3 settled the *spelling*, not this *emission* gap); `_`-prefixed RM attributes (1092 keys — its own REQ); PARTY_PROXY / DV_MULTIMEDIA / DV_PARSABLE / DV_INTERVAL<DV_QUANTITY> leaf mappings

## Goal

Raise the share of the pinned upstream EHRbase FLAT corpus that PROBE-086 compares **exactly**, from the 10.5% (192 of 1824 keys) the harness reported on landing. The ranked inventory already exists in [`SKIPPED.md` § What would move these numbers](../../testkit/conformance/webtemplate/SKIPPED.md); this plan executes the cheapest items and settles the one that needed a normative decision (Phase 3, [ADR 0015](../adr/0015-flat-metadata-spelling.md)).

Consumers: anyone decoding FLAT this SDK did not write — the whole point of PROBE-086 over PROBE-076.

## Scope correction against SKIPPED.md's ranking

`SKIPPED.md` ranks optional datatype suffixes first (cheapest to implement). Ranked by **risk** the order inverts, and this plan follows the risk order:

| Item | Keys | Risk | Why |
|---|---:|---|---|
| CODE_PHRASE leaves | 136 | **additive** | `leafToFlat` skips non-`DV_` leaves today, so these paths are emitted by nothing. New keys appear; no existing key changes spelling or value. |
| Optional datatype suffixes | 33 | **output-changing** | `capturedKeys` decides suffix-form vs `\|raw`. Admitting `\|magnitude_status` et al. re-routes a decorated value that rides `\|raw` today into the suffix form — a wire-visible change for existing users, and a `deviations.md` entry. |

Hence Phase 1 before Phase 2. Phase 3 is orthogonal to both — an input-surface change that moves no census number (see below).

## Definition of Ready

- **`Covers:`** names REQ-053 / REQ-121 (both landed, canonical prose exists) — satisfied.
- No irreversible fork in Phases 1–2. The one that did exist (metadata spelling) was gated on an ADR and is settled in Phase 3 by ADR 0015.
- Each phase names its verification command.

## Definition of Done

- Census coverage strictly above 10.5%, with the new figure recorded in `SKIPPED.md` and its per-fixture excluded ratchet re-pinned.
- `deviations.md` entries for the closed gaps deleted, not merely reworded.
- Stale `unserialisableIC` exemptions in `webtemplate.TestInContextLeavesResolveViaRmpath` deleted so the guard enforces the newly-resolvable leaves.
- `traceability.yaml` touched where package paths / tests changed.
- `make spec-check` + `make ci` green; PROBE-076 still green (the SDK's own round-trip must not regress).

## Implementation checklist

| Step | Status |
|---|---|
| Phase 1 — CODE_PHRASE leaf mapping (encode + decode + rmpath + guard) | **done** — 192 → 328 keys, 10.5% → 18.0% |
| Phase 2 — optional datatype suffixes | |
| Phase 3 — metadata spelling (ADR 0015) | **done** — census unchanged by design; REQ-115 unblocked |
| `SKIPPED.md` census refreshed, ratchet re-pinned | **done** (Phase 1) |
| `deviations.md` updated | **done** (Phase 1) |
| `make spec-check` | **green** |
| `make ci` | **green** |

## Phases

### Phase 1 — CODE_PHRASE leaves (136 keys)

Upstream spells these `<entry>/language|code` + `|terminology` (values `en` / `ISO_639-1`, `UTF-8` / `IANA_character-sets`) on all five ENTRY types.

**Tasks:**

1. `simplified/datatypes.go` — `capturedKeys["CODE_PHRASE"] = {code_string, terminology_id}`; teach `leafToFlat` to emit `|code` + `|terminology`. **Emit only when `code_string` is non-empty** — `Language` / `Encoding` are non-pointer `CodePhrase` fields, so an unconditional emit would write empty leaves on every composition decoded through the `ctx/` forms (the hazard `EVENT_CONTEXT.setting` documents).
2. `simplified/flat_decode.go` — `allowedSuffixes["CODE_PHRASE"] = {code, terminology}`; rebuild `{_type, code_string, terminology_id}` in `dvFromSuffixes`.
3. `openehr/rm/rmpath/walk.go` — resolve `language` / `encoding` on OBSERVATION / EVALUATION / INSTRUCTION / ACTION / ADMIN_ENTRY (REQ-121). Without this the encoder resolves nothing to hand the new leaf mapping.
4. Delete the ten matching `unserialisableIC` entries (language + encoding × 5 ENTRY types). Keep `subject` (PARTY_PROXY) and the two COMPOSITION `ctx/` respellings exempt.

**Verification:** `go test ./openehr/serialize/simplified/... ./openehr/rm/rmpath/... ./openehr/template/webtemplate/... ./testkit/...`; census coverage up by ~136 keys.

**Outcome — landed.** 192 → 328 compared keys (10.5% → 18.0%), +4 per fixture across all 34, and the `unsupported datatype: CODE_PHRASE` family is gone from the census. One thing the plan did not predict: the encoder's leaf test gated on `len(node.Inputs) > 0 || strings.HasPrefix(node.RMType, "DV_")`, and the reference emits the in-context CODE_PHRASE pair with **no input descriptors**, so those nodes were classed as structural and skipped before any datatype mapping was reached — the leaf mapping alone changed nothing. Leaf-ness is now `simplified.isValueLeafType`, shared with the `|raw` fallback. Two tests needed fixture repairs (not assertion changes): `TestEncodeSkipsEmptyRepeatInstances`, whose "empty" ENTRY now legitimately carries mandatory language/encoding, and `TestDecodeRejectsSparseIndex`, whose single-key mutation split a multi-suffix leaf so a datatype error masked the index check.

### Phase 2 — optional datatype suffixes (33 keys)

`|magnitude_status`, `|normal_status`, `|accuracy`, `|accuracy_is_percent`, `|precision`, `|units_system`, `|units_display_name`, `|formatting`, `|preferred_term` across DV_QUANTITY / PROPORTION / COUNT / DURATION / DATE / DATE_TIME / TIME / TEXT / CODED_TEXT.

**Tasks:**

1. Extend `allowedSuffixes` + `capturedKeys` + the `dvFromSuffixes` rebuilds per datatype, keeping every added suffix **optional** (absent ≠ coerced zero — the existing `requireSuffix` contract).
2. Record the `|raw` → suffix-form re-routing in `deviations.md`; it changes emitted bytes for decorated values.
3. Assert the round-trip both ways for each added suffix.

**Verification:** `make ci`; census coverage up by ~33 keys; PROBE-076 green.

### Phase 3 — the metadata spelling fork — **decided and landed**

Composition-metadata spelling was `ctx/` versus the reference's real paths. Resolved by **[ADR 0015](../adr/0015-flat-metadata-spelling.md): accept both on input, emit `ctx/` only.**

**Tasks (done):** ADR 0015; REQ-053 normative prose in `wire.md` (decode MUST accept either spelling, encode MUST emit `ctx/`, a disagreeing pair MUST fail); `siphonContext` + the `metadataAliases` / `metadataAliasTerminology` tables in `flat_decode.go`; six tests in `context_test.go`; `deviations.md` + `SKIPPED.md`.

**Outcome, stated against the expectation.** This **does not move the census** — coverage stays at **18.0%**, and the 318 keys stay held out. PROBE-086 decodes upstream FLAT and *re-encodes* it, so a real-path key still returns respelled as `ctx/`: one missing key plus one extra. Only emitting the reference's spelling would move the number, and ADR 0015 rejects that as a breaking output change for no interop gain. What the phase actually buys is the thing that was blocking: REQ-115 can now state its required-key set, and an EHRbase-authored body decodes instead of failing with `ErrUnknownPath`.

Two of the 318 turned out **not** to be respellings and stay refused rather than aliased — `context/setting|*` (102 keys; `ctx/setting` is unsupported on decode too, so it is an unimplemented field on both surfaces, not a spelling gap) and `composer|id*` (12 keys; an `external_ref` the `ctx/` forms structurally cannot carry, and dropping it would breach REQ-053). The `context/setting` waiver therefore survives this phase — it needs `ctx/setting` **emission**, which is its own slice.

## Mapping to specs

- [wire.md § REQ-053](../specifications/wire.md#req-053) — the codec contract (grammar pinned upstream; no closed datatype list)
- [rm-functions.md § REQ-121](../specifications/rm-functions.md#req-121--locatable-path-read-access) — locatable path read access
- [conformance.md § PROBE-086](../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) — the probe whose census this moves
- [`SKIPPED.md`](../../testkit/conformance/webtemplate/SKIPPED.md) — the ranked inventory (informative)
- [`simplified/deviations.md`](../../openehr/serialize/simplified/deviations.md) — package deviation register
