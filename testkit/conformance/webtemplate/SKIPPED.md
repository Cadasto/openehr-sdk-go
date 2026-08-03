# PROBE-086 — skipped and excluded surface

**Informative.** The normative probe definition is [conformance.md § PROBE-086](../../../docs/specifications/conformance.md#probe-086--upstream-flat-serialisation-parity); the plan is [2026-07-16-web-template-tests-conformance.md](../../../docs/plans/archive/2026-07-16-web-template-tests-conformance.md). This file records what the harness does **not** compare and why, so the number is auditable rather than a bare skip count.

Regenerate the figures below with:

```
go test ./testkit/conformance/webtemplate/ -run TestCensus -census -v
```

The census output is deterministic — diff it across commits to see the surface move.

## Position as of 2026-08-03

```
corpus: 34 fixtures, 1824 keys — 192 compared, 1314 excluded, 318 metadata held out
coverage: 10.5% of upstream keys are in the modelled subset
```

**10.5% is the honest number, and it is the point of this probe.** The corpus is upstream-authored FLAT written against a mature Java implementation; this SDK's REQ-053 codec models a deliberately narrow slice of it. Nothing here was known before the harness ran — [PROBE-076](../../../docs/specifications/conformance.md#probe-076--flat--structured-composition-round-trip) round-trips the SDK's *own* output and so reports 24/25 green over the same codec. The gap between those two numbers is exactly the value of feeding in FLAT this SDK did not write.

This file's first revision recorded 4.9% / 90 compared. That figure was wrong against the codec's actual behaviour: `category` was held out as composition metadata, when it is nothing of the kind (see below). Its 102 keys — 34 fixtures × 3 — were already round-tripping byte-for-byte and are now compared, which is the whole of the move from 90 to 192. No codec gap closed to earn it, and none of the excluded surface moved.

## How the exclusion list is produced

It is **not** hand-maintained. `Run` decodes each upstream body; wherever the codec refuses a key it fails loudly (REQ-053 never drops silently), and the harness records the refusal, removes what that refusal covers, and retries. The excluded set is therefore whatever the codec itself declines — closing a gap shrinks it automatically, with no list to update in lockstep.

**How much a refusal removes is itself a correctness question.** The codec names a *base* path (it groups a leaf's suffixes before decoding), so the scope has to be read off the error, and dropping wider than the refusal quietly withdraws keys the codec would have round-tripped. `dropRefused` distinguishes three shapes:

| Refusal | Removes | Why not more |
|---|---|---|
| a suffix the datatype does not map (`unexpected \|precision for DV_QUANTITY`) | that one entry | `\|magnitude` and `\|unit` on the same leaf are modelled; the next pass names the next unmodelled suffix, so the leaf is pared down rather than discarded |
| a leaf whose datatype is not modelled, including a container addressed as a value leaf (`unsupported datatype: EVENT` for `any_event:0\|sample_count`) | the leaf's own entries | an EVENT this codec cannot read *as a value* still has children it reads fine |
| a path that does not resolve at all (`path not in web template`) | the leaf and its subtree | nothing below an unresolvable node can resolve either |

Getting this wrong is not theoretical: the first cut dropped the whole leaf family plus subtree for every refusal, which cost 17 keys of real coverage and left `interval_event` comparing **nothing at all** — a fixture in the suite asserting zero.

Two things are treated differently:

- **Composition metadata** — 318 keys, held out of the comparison on *both* sides and not counted as a refusal. The hold-out is a narrow allow-list (`language`, `territory`, `composer`, `context/setting`, `context/start_time`) precisely so that `context/other_context`, which carries archetyped data, can never be swallowed by it. Two different things sit in that list, and calling them all "spelling differences" was itself a drift this file has now corrected:
  - **Respellings** — `language`, `territory`, `composer`, `context/start_time`. Upstream writes them as real paths (`<root>/language|code`, `<root>/composer|id`, `<root>/context/start_time`); REQ-053 reads and writes the `ctx/` short forms (`ctx/language`, `ctx/time`). Same information, different surface, and comparing across the two spellings would report every such key as both missing and extra.
  - **One waiver** — `context/setting`. Not a respelling: it decodes (a `WithTemplate` decode even defaults it to `238 other care`) and then re-encodes to *nothing at all*, because `ctx/setting` emission is deferred ([simplified/deviations.md](../../../openehr/serialize/simplified/deviations.md) § `ctx/` context). Holding it out **waives a real encode-side drop**. It is waived here rather than hidden, and only until the `ctx/`-versus-real-path decision (§ What would move these numbers, item 3) — whichever spelling wins, this key stops being a hold-out and starts being compared.

  `category` was on this list until 2026-08-03 and should never have been. It is not a `ctx/` field at all: it is a template-constrained Web Template leaf that rides its own FLAT path, spelled identically by upstream and by this codec, and all 102 of its corpus keys round-trip byte-for-byte. Holding it out withdrew coverage the codec had already earned.

  Note the asymmetry the hold-out creates on the **emitted** side: the `ctx/` test there is unbounded, so *every* `ctx/`-prefixed key the encoder writes is skipped and a bogus one — a `ctx/` key upstream would never write, or one carrying the wrong value — is invisible to this harness. [PROBE-076](../../../docs/specifications/conformance.md#probe-076--flat--structured-composition-round-trip)'s decode leg is the backstop: it feeds this SDK's own `ctx/` output back through decode.
- **Nothing else.** There is no tolerated-drop list. An upstream key that decodes and then fails to re-encode is a failure, always — see the section below for the one class that ever qualified and why the bucket that held it is gone.

Everything that survives into the comparison must round-trip exactly: a missing key, an extra key, or a changed value **fails** the test.

## Excluded families (1314 keys)

| Keys | Refusals | Reason | Owner |
|---:|---:|---|---|
| 1118 | 289 | `path not in web template` | overwhelmingly the `_`-prefixed RM attribute family, which REQ-053 does not model at all: 1092 of the corpus's 1824 keys carry an `_` segment — `_feeder_audit/…` alone is 530, then `_health_care_facility` 143, `_other_participation:N` 97, `_other_reference_ranges` 44, `_uid` 43, `_link:N` 39, `_participation:N` 31, `_normal_range` 30, `_mapping` 27, `_work_flow_id` 24, `_guideline_id` 20, `_identifier` 16, … Also `ism_transition/*` and the ACTION `time` leaf, neither of which the WebTemplate builder synthesizes — webtemplate/deviations.md § inContext coverage names `ism_transition` as the outstanding in-context set, which is the largest but not the only one; ACTION `time` is unemitted too. |
| 136 | 68 | `unsupported datatype: CODE_PHRASE` | ENTRY-level `language` / `encoding`, emitted by the reference as CODE_PHRASE leaves; the codec has no CODE_PHRASE leaf mapping. |
| 8 | 2 | `unsupported datatype: PARTY_PROXY` | `subject` / `composer` addressed as a leaf. |
| 7 | 1 | `unsupported datatype: DV_MULTIMEDIA` | datatype not modelled by the codec. |
| 7 | 7 | `unsupported \|suffix for DV_QUANTITY` | optional datatype suffixes (`\|precision`, `\|magnitude_status`, …) beside modelled ones that now stay in the comparison. |
| 6 | 2 | `unsupported datatype: DV_INTERVAL<DV_QUANTITY>` | datatype not modelled by the codec. |
| 5 | 5 | `unsupported \|suffix for DV_PROPORTION` | optional datatype suffixes. |
| 5 | 5 | `unsupported \|suffix for DV_TEXT` | ″ (`\|formatting`, `\|terminology`, …) |
| 4 | 2 | `unsupported datatype: DV_PARSABLE` | datatype not modelled by the codec. |
| 4 | 4 | `unsupported \|suffix for DV_COUNT` | optional datatype suffixes. |
| 4 | 4 | `unsupported \|suffix for DV_DURATION` | ″ |
| 2 | 2 | `unsupported \|suffix for DV_DATE` | ″ |
| 2 | 2 | `unsupported \|suffix for DV_DATE_TIME` | ″ |
| 2 | 2 | `unsupported \|suffix for DV_TIME` | ″ |
| 1 | 1 | `unsupported bare value for DV_PROPORTION` | the leaf written without a suffix. |
| 1 | 1 | `unsupported datatype: EVENT` | an `any_event:N` node addressed as a value leaf (`\|sample_count`); its children are compared. |
| 1 | 1 | `unsupported datatype: STRING` | ACTIVITY `action_archetype_id`. |
| 1 | 1 | `unsupported \|suffix for DV_CODED_TEXT` | optional datatype suffix. |

"Refusals" counts decode passes, not leaves — a leaf carrying three unmodelled suffixes is refused three times, once per suffix, which is exactly why the suffix rows are now small.

The `|suffix` rows are collapsed to the datatype deliberately: a leaf often carries several unmodelled suffixes and which one the codec names first depends on map iteration order, so naming it would make the census non-deterministic. The *totals* are stable regardless of that order, since each unmodelled suffix is refused and removed individually.

## Encode-side drops — **closed 2026-08-01** (was 32 keys / 30 fixtures)

Keys the codec decoded correctly and then dropped on re-encode. **None remain, and the bucket that tolerated them has been removed.**

The harness found three: `EVENT.time`, `INSTRUCTION.narrative`, `INSTRUCTION.expiry_time`. All shared one cause — `rmpath` resolved none of the affected in-context attributes, and `flat_encode` routes that `ErrPathNotFound` into `skipNotFound` alongside genuinely absent optionals, which is what kept the loss silent.

Fixed by adding the missing attributes to `rmpath`'s `childrenAt` switches (REQ-121): `time` on `POINT_EVENT` / `INTERVAL_EVENT`, `math_function` and `width` on `INTERVAL_EVENT`, `narrative` and `expiry_time` on `INSTRUCTION`. All 32 keys now round-trip.

The list that had tolerated them — `knownEncodeGaps`, matching the key suffixes `/time`, `/narrative`, `/expiry_time` — was deleted along with the `Report.KnownGaps` bucket it fed. An empty-but-armed matcher is a fail-open: a regression in precisely the area this probe was built to watch would have landed in a tolerated bucket, `Clean()` would still have reported true, and the probe would have passed. Verified by regressing `rmpath` on purpose: removing the `narrative` case now fails `TestUpstreamFlatParity/ehrbase_conformance_instruction` with *upstream key decoded but not re-emitted*, which is the whole point.

**The class is now guarded structurally**, not just these three instances: `webtemplate.TestInContextLeavesResolveViaRmpath` asserts that every in-context leaf the WebTemplate can emit is resolvable through `rmpath` unless it is deliberately exempted in that test's `unserialisableIC`, each exemption carrying its reason *and* its encode consequence. A new in-context leaf without rmpath support fails that test with instructions, instead of silently losing data.

### The hazard model, corrected

An earlier revision of this file justified the exemptions with "the encoder never reaches rmpath for a leaf it cannot write", and warned that resolving them would cause "a hard encode failure on nearly every composition". Both are wrong, and the mechanism matters because it decides what is safe to fix independently:

- **Resolution comes first, leaf mapping second.** `flat_encode.emitNode` resolves the node's path against the RM instance through `rmpath`, and only then hands the value to the leaf mapping. There is no look-ahead: rmpath is reached for every emitted node, writable or not.
- **An unwritable datatype does not fail the encode.** `simplified.leafToFlat` returns *nothing at all* for a non-`DV_` type (CODE_PHRASE, PARTY_PROXY, STRING — a documented skip), and writes an unmapped `DV_*` value as `|raw`. So "the codec cannot write it" is a reason for a leaf to stay **unemitted**, never a reason for an attribute to stay **unresolvable**, and the two changes do *not* have to land together.

What is actually exempt today, and why:

- ENTRY `language` / `encoding` / `subject`, COMPOSITION `language` / `territory`, ACTIVITY `action_archetype_id` — the loss is in `leafToFlat`, which skips these datatypes whatever rmpath does. A codec gap, not an rmpath gap.
- COMPOSITION `composer` — **resolves in rmpath** and always did; it is exempt from the leaf guard because the `ctx/` short forms carry it instead. Only its `external_ref` is lost (a known deviation); the name round-trips via `ctx/composer_name`.
- ACTIVITY `timing` — **resolved in this round.** DV_PARSABLE has no suffix mapping, so a populated `timing` now rides `|raw` rather than vanishing.
- EVENT_CONTEXT `start_time` / `setting` — deliberately still unresolved, and the only two where resolving would make the *output* worse today. `start_time` is already emitted as `ctx/time`, so resolving it would double-spell one value under two keys. `setting` is a non-pointer `DV_CODED_TEXT` on `EVENT_CONTEXT`, so resolving it would emit a zero-valued leaf on every composition decoded through the `ctx/` forms. A **non-default `setting` is dropped on encode today** — a known deviation ([simplified/deviations.md](../../../openehr/serialize/simplified/deviations.md) § `ctx/` context), and precisely the drop the `context/setting` hold-out above waives. Both clear when the metadata real-path decision lands.

## What would move these numbers

Ranked by keys unlocked, cheapest first:

1. **Optional datatype suffixes** (33 keys across DV_TEXT / QUANTITY / PROPORTION / CODED_TEXT / COUNT / DURATION / DATE / DATE_TIME / TIME) — accept and round-trip the suffixes the reference emits. Cheapest by far, and now a small number precisely because each unmodelled suffix costs only itself.
2. **CODE_PHRASE leaves** (136 keys) — one datatype mapping in `leafToFlat`, plus deleting the matching `unserialisableIC` entries once it lands. rmpath resolution is *not* a co-requisite; see the corrected hazard model above.
3. **Composition metadata real-path spelling** (318 keys, currently held out rather than counted against us) — accepting the reference's spelling alongside `ctx/` would let these be compared instead of held out. It is also what settles `context/setting`, the one hold-out that waives a real encode-side drop rather than a respelling, and what unblocks EVENT_CONTEXT `start_time` / `setting` in rmpath.
4. **`_`-prefixed RM attributes** (1092 keys, 60% of the corpus) — by far the largest, and a genuine feature rather than a deviation to close: `_feeder_audit` (530 on its own), `_health_care_facility`, participations, `_uid`, links, reference ranges, workflow ids. Its own REQ.
