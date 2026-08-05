# PROBE-086 — skipped and excluded surface

**Informative.** The normative probe definition is [conformance.md § PROBE-086](../../../docs/specifications/conformance.md#probe-086--upstream-flat-serialisation-parity); the plan is [2026-07-16-web-template-tests-conformance.md](../../../docs/plans/archive/2026-07-16-web-template-tests-conformance.md). This file records what the harness does **not** compare and why, so the number is auditable rather than a bare skip count.

Regenerate the figures below with:

```
go test ./testkit/conformance/webtemplate/ -run TestCensus -census -v
```

The census output is deterministic — diff it across commits to see the surface move.

## Position as of 2026-08-05 (DV_TEXT substitution carve-out + `ctx/setting` emission landed)

```
corpus: 34 fixtures, 1824 keys — 360 compared, 1158 excluded, 306 metadata held out
coverage: 19.7% of upstream keys are in the modelled subset
```

The metadata/excluded split moved on 2026-08-03 without the coverage figure moving: the composer hold-out became suffix-aware, so the 12 `composer|id`/`|id_scheme`/`|id_namespace` keys left the hold-out (318 → 306) and joined the refusals (1150 → 1162) where they belong. `compared` is untouched — those keys were never in the comparison; they were being counted as a spelling difference when they are a refusal.

`ctx/setting` emission (2026-08-05, the amended REQ-053) moved **none** of the three figures, by the same mechanism ADR 0015 predicted for the other respellings: the 102 `context/setting|*` keys stay held out on both sides — what changed is *why*. They were the suite's one documented **waiver** of a real encode-side drop; they are now ordinary respellings (`context/setting|code`/`|value` alias onto `ctx/setting|code` + `|value`, `|terminology` is an `openehr` witness), a populated setting round-trips, and the waiver class is empty — `TestHoldOutMatchesCodecAliases` fails if one reappears.

**19.7% is the honest number, and it is the point of this probe.** The corpus is upstream-authored FLAT written against a mature Java implementation; this SDK's REQ-053 codec models a deliberately narrow slice of it. Nothing here was known before the harness ran — [PROBE-076](../../../docs/specifications/conformance.md#probe-076--flat--structured-composition-round-trip) round-trips the SDK's *own* output and so reports 24/25 green over the same codec. The gap between those two numbers is exactly the value of feeding in FLAT this SDK did not write.

### How the number has moved

| Position | Compared | Coverage | Earned by |
|---|---:|---:|---|
| first revision | 90 | 4.9% | — |
| 2026-08-01 | 192 | 10.5% | **nothing** — `category` was wrongly held out as composition metadata (see below); its 102 keys were already round-tripping byte-for-byte. No codec gap closed, no excluded surface moved. |
| 2026-08-03 | 328 | 18.0% | **a real codec gap closing** — the CODE_PHRASE leaf mapping ([plan](../../../docs/plans/archive/2026-08-03-flat-coverage-ratchet.md) Phase 1). The `unsupported datatype: CODE_PHRASE` family, 68 refusals over 136 keys, is gone from the census: excluded fell 1314 → 1178 and every fixture gained exactly its ENTRY's `language\|code`, `language\|terminology`, `encoding\|code`, `encoding\|terminology`. |
| 2026-08-03 | 356 | 19.5% | **the optional datatype suffixes** ([plan](../../../docs/plans/archive/2026-08-03-flat-coverage-ratchet.md) Phase 2) — 28 keys: `\|magnitude_status`, `\|normal_status`, `\|accuracy`, `\|accuracy_is_percent`, `\|precision`, `\|units_system`, `\|units_display_name`, `\|formatting` across DV_QUANTITY / COUNT / PROPORTION / DURATION / DATE / DATE_TIME / TIME / TEXT / CODED_TEXT. Unlike Phase 1 this **changes emitted bytes**: `capturedKeys` decides suffix-form versus `\|raw`, so a value carrying one of these now rides suffixes where it previously rode a `\|raw` fragment. Undecorated values are byte-identical (an absent attribute writes no suffix). |
| 2026-08-05 | 360 | 19.7% | **the DV_TEXT substitution carve-out** ([plan](../../../docs/plans/2026-08-05-flat-rm-attributes.md) Phase A, REQ-053) — 4 keys, all in `dv_coded_text_as_dv_text`: a fully-captured `DV_CODED_TEXT` at a `DV_TEXT`-typed leaf now rides the DV_CODED_TEXT suffix set (`\|code` + `\|value` + `\|terminology` + `\|formatting`) instead of `\|raw`, and decode re-selects the coded builder from `\|code` at a `DV_TEXT` leaf. That cleared the 3 refused suffixes **and** the consequential `DV_TEXT missing required bare value` refusal (the surviving `\|formatting` had been left with nothing to rebuild from). A *decorated* coded text (mappings, preferred_term, …) still rides `\|raw`; every other substituted subtype is unchanged. |

The CODE_PHRASE round trip needed three things, and the third is the one worth remembering: the encoder's leaf test gated on *input descriptors or a `DV_` prefix*, and the reference emits these in-context leaves with **no inputs at all**, so every such node was classed as structural and skipped before any datatype mapping was consulted. A leaf type is now recognised by `simplified.isValueLeafType`; the reference's silence about inputs is not evidence that a node carries no value. `rmpath` also had to resolve `language` / `encoding` on the five ENTRY subtypes (REQ-121) — with the leaf mapping in place but resolution missing, the keys would have decoded and then silently failed to re-emit, which is exactly the shape of the encode-side drop closed on 2026-08-01.

## How the exclusion list is produced

It is **not** hand-maintained. `Run` decodes each upstream body; wherever the codec refuses a key it fails loudly (REQ-053 never drops silently), and the harness records the refusal, removes what that refusal covers, and retries. The excluded set is therefore whatever the codec itself declines — closing a gap shrinks it automatically, with no list to update in lockstep.

**How much a refusal removes is itself a correctness question.** The codec names a *base* path (it groups a leaf's suffixes before decoding), so the scope has to be read off the error, and dropping wider than the refusal quietly withdraws keys the codec would have round-tripped. `dropRefused` distinguishes three shapes:

| Refusal | Removes | Why not more |
|---|---|---|
| a suffix the datatype does not map (`unexpected \|normal_range for DV_QUANTITY`) | that one entry | `\|magnitude` and `\|unit` on the same leaf are modelled; the next pass names the next unmodelled suffix, so the leaf is pared down rather than discarded |
| a leaf whose datatype is not modelled, including a container addressed as a value leaf (`unsupported datatype: EVENT` for `any_event:0\|sample_count`) | the leaf's own entries | an EVENT this codec cannot read *as a value* still has children it reads fine |
| a path that does not resolve at all (`path not in web template`) | the leaf and its subtree | nothing below an unresolvable node can resolve either |

Getting this wrong is not theoretical: the first cut dropped the whole leaf family plus subtree for every refusal, which cost 17 keys of real coverage and left `interval_event` comparing **nothing at all** — a fixture in the suite asserting zero.

Two things are treated differently:

- **Composition metadata** — 306 keys, held out of the comparison on *both* sides and not counted as a refusal. The hold-out is a narrow allow-list (`language|*`, `territory|*`, `composer|name`, `composer_self`, `context/start_time`, `context/setting|*`) precisely so that `context/other_context`, which carries archetyped data, can never be swallowed by it. Since 2026-08-05 everything in that list is one kind of thing — a **respelling**: upstream writes real paths (`<root>/language|code`, `<root>/composer|name`, `<root>/context/start_time`, `<root>/context/setting|code`); REQ-053 reads and writes the `ctx/` short forms (`ctx/language`, `ctx/composer_name`, `ctx/time`, `ctx/setting|code` + `|value`). Same information, different surface, and comparing across the two spellings would report every such key as both missing and extra. `composer_self` is held out even though this corpus writes only the `ctx/` form of it (one key, in `party_self`) — the codec accepts the real-path spelling, so the harness must account for it whether or not a fixture exercises it.

  `context/setting` was the exception until 2026-08-05, and its history is worth keeping: it was this suite's one documented **waiver** — the real path decoded (a `WithTemplate` decode even defaulted it to `238 other care`) and then re-encoded to *nothing at all*, because `ctx/setting` emission was deferred. [ADR 0015](../../../docs/adr/0015-flat-metadata-spelling.md) settled the metadata *spelling* and deliberately left that **emission** gap open; the amended REQ-053 closed it — encode emits `ctx/setting|code` + `|value` for a populated setting, decode accepts either spelling under the same disagreement rules, and `context/setting|terminology` is a checked-then-discarded `openehr` witness ([simplified/deviations.md](../../../openehr/serialize/simplified/deviations.md) § `ctx/` context). **The waiver class is now empty and must stay empty**: `TestHoldOutMatchesCodecAliases`'s reverse direction fails on any hold-out with no accepted spelling behind it, and reintroducing a waiver is a spec decision (conformance.md § PROBE-086), not a harness edit.

  **The match is suffix-aware, and it has to be.** `language` and `territory` hold out *every* suffix, because their `|terminology` witness has no separate `ctx/` counterpart — one `ctx/language` stands for the whole CODE_PHRASE, so matching per-suffix would leave the witness keys in the comparison with nothing on the emitted side to compare them against; `context/setting` is base-matched on the same grounds (every suffix it carries is an accepted alias or the witness). The composer is the opposite case: its suffixes do not all mean the same thing, so only the exact spellings the codec respells are held out. `composer|id`, `|id_scheme` and `|id_namespace` are the PARTY_PROXY `external_ref`, which no `ctx/` short form can carry; ADR 0015 refuses them rather than dropping them silently, so they flow through to decode, are refused there, and are **counted as an excluded family** — 12 of the PARTY_PROXY row's 20 keys below.

  Until 2026-08-03 the matcher cut every key at its first `|` and matched the base, so all four composer suffixes were absorbed: 12 keys of real data loss counted as a spelling difference, while this file and the surrounding docs said they were refused and visible in the census. `TestHoldOutMatchesCodecAliases` now asserts both directions mechanically against `simplified.MetadataAliasSpellings` / `MetadataWitnessSpellings` — every spelling the codec accepts is held out, and every hold-out the harness applies over the corpus key universe maps back to an accepted spelling. The reverse direction is the one that would have caught this: a hold-out with no codec spelling behind it is absorbing a refusal.

  `category` was on this list until 2026-08-03 and should never have been. It is not a `ctx/` field at all: it is a template-constrained Web Template leaf that rides its own FLAT path, spelled identically by upstream and by this codec, and all 102 of its corpus keys round-trip byte-for-byte. Holding it out withdrew coverage the codec had already earned.

  Note the asymmetry the hold-out creates on the **emitted** side: the `ctx/` test there is unbounded, so *every* `ctx/`-prefixed key the encoder writes is skipped and a bogus one — a `ctx/` key upstream would never write, or one carrying the wrong value — is invisible to this harness. [PROBE-076](../../../docs/specifications/conformance.md#probe-076--flat--structured-composition-round-trip)'s decode leg is the backstop: it feeds this SDK's own `ctx/` output back through decode.
- **Nothing else.** There is no tolerated-drop list. An upstream key that decodes and then fails to re-encode is a failure, always — see the section below for the one class that ever qualified and why the bucket that held it is gone.

Everything that survives into the comparison must round-trip exactly: a missing key, an extra key, or a changed value **fails** the test.

## Excluded families (1158 keys)

| Keys | Refusals | Reason | Owner |
|---:|---:|---|---|
| 1118 | 289 | `path not in web template` | overwhelmingly the `_`-prefixed RM attribute family, which REQ-053 does not model at all: 1092 of the corpus's 1824 keys carry an `_` segment — `_feeder_audit/…` alone is 530, then `_health_care_facility` 143, `_other_participation:N` 97, `_other_reference_ranges` 44, `_uid` 43, `_link:N` 39, `_participation:N` 31, `_normal_range` 30, `_mapping` 27, `_work_flow_id` 24, `_guideline_id` 20, `_identifier` 16, … Also `ism_transition/*` and the ACTION `time` leaf, neither of which the WebTemplate builder synthesizes — webtemplate/deviations.md § inContext coverage names `ism_transition` as the outstanding in-context set, which is the largest but not the only one; ACTION `time` is unemitted too. |
| 20 | 6 | `unsupported datatype: PARTY_PROXY` | two owners, two different reasons. **ENTRY `subject`** addressed as a value leaf — `subject\|id` / `\|id_scheme` / `\|id_namespace` / `\|name` on the OBSERVATION, 8 keys over `party_identified` and `party_related`: `leafToFlat` maps no PARTY_PROXY at all, so it is a plain codec gap. **COMPOSITION `composer\|id` / `\|id_scheme` / `\|id_namespace`** — the composer's `external_ref`, 12 keys over `composition`, `party_identified`, `party_related`, `party_self`: refused deliberately (ADR 0015), because the `ctx/` short forms structurally cannot carry it and dropping it silently would breach REQ-053. The composer's `\|name` is **not** in this row — it respells to `ctx/composer_name` and is held out as metadata. |
| 7 | 1 | `unsupported datatype: DV_MULTIMEDIA` | datatype not modelled by the codec. |
| 6 | 2 | `unsupported datatype: DV_INTERVAL<DV_QUANTITY>` | datatype not modelled by the codec. |
| 4 | 2 | `unsupported datatype: DV_PARSABLE` | datatype not modelled by the codec. |
| 1 | 1 | `unsupported bare value for DV_PROPORTION` | the proportion written *also* as a bare value (`1.6532258064516128` beside its numerator/denominator) — the **derived** magnitude. DV_PROPORTION has no `magnitude` attribute; it is a computed function, so there is nothing to decode it into, and emitting it would mean reproducing the reference's exact float formatting. |
| 1 | 1 | `unsupported datatype: EVENT` | an `any_event:N` node addressed as a value leaf (`\|sample_count`); its children are compared. |
| 1 | 1 | `unsupported datatype: STRING` | ACTIVITY `action_archetype_id`. |

"Refusals" counts decode passes, not leaves — a leaf carrying three unmodelled suffixes is refused three times, once per suffix, which is exactly why the suffix rows are now small.

The per-datatype `|suffix` rows for DV_QUANTITY / PROPORTION / COUNT / DURATION / DATE / DATE_TIME / TIME / CODED_TEXT are **gone** as of the Phase 2 optional-suffix set, and the last one standing — `unsupported |suffix for DV_TEXT`, which was never an optional-attribute problem but the DV_CODED_TEXT-at-DV_TEXT **substitution** (plus its consequential `DV_TEXT missing required bare value` refusal) — cleared on 2026-08-05 with the REQ-053 carve-out in the substitution rule.

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
- **An unwritable datatype does not fail the encode.** `simplified.leafToFlat` returns *nothing at all* for a leaf datatype it does not map (PARTY_PROXY, STRING — a documented skip; CODE_PHRASE was one until this ratchet gave it a `|code` + `|terminology` mapping), and writes an unmapped `DV_*` value as `|raw`. So "the codec cannot write it" is a reason for a leaf to stay **unemitted**, never a reason for an attribute to stay **unresolvable**, and the two changes do *not* have to land together.

What is actually exempt today, and why:

- ENTRY `subject`, ACTIVITY `action_archetype_id` — the loss is in `leafToFlat`, which skips these datatypes whatever rmpath does. A codec gap, not an rmpath gap. (ENTRY `language` / `encoding` were in this bucket until the CODE_PHRASE leaf mapping landed; they now resolve and round-trip, and their guard exemptions are deleted.)
- COMPOSITION `language` / `territory` — no longer a datatype gap either, but still exempt: the `ctx/` short forms carry them, so resolving them would double-spell one value under two keys.
- COMPOSITION `composer` — **resolves in rmpath** and always did; it is exempt from the leaf guard because the `ctx/` short forms carry it instead. Only its `external_ref` is lost (a known deviation); the name round-trips via `ctx/composer_name`.
- ACTIVITY `timing` — **resolved in this round.** DV_PARSABLE has no suffix mapping, so a populated `timing` now rides `|raw` rather than vanishing.
- EVENT_CONTEXT `start_time` / `setting` — deliberately still unresolved, and both now for the same reason: the **permanent double-spell class**. `start_time` is emitted as `ctx/time` and `setting` as `ctx/setting|code` + `|value` (REQ-053, amended 2026-08-05 — the emission gap ADR 0015 had left open), so resolving either real path would double-spell one value under two keys, and ADR 0015's ctx/-only encode makes that permanent — no data loss sits behind either exemption. An earlier revision of this file paired the two as one deferred item and then corrected the pairing the other way (`setting` was the emission gap, `start_time` the double-spell); with `ctx/setting` emission landed the asymmetry is gone — closing it is what cleared this suite's one waiver (§ Composition metadata above).

## What would move these numbers

Ranked by keys unlocked, cheapest first. **~~CODE_PHRASE leaves (136 keys)~~ — done 2026-08-03**, and the ranking below now carries a risk column, because "cheapest to implement" and "safest to land" turned out to disagree.

1. ~~**Optional datatype suffixes**~~ — **done 2026-08-03** (28 keys, not the 33 the earlier count implied: 4 of those were the DV_TEXT substitution family and the derived DV_PROPORTION magnitude, neither an optional attribute, and one `|formatting` was double-counted). It was **not** additive, as predicted: `capturedKeys` decides suffix-form versus `|raw`, so a decorated value that rode `|raw` now rides suffixes — recorded in `deviations.md`.

   ~~**DV_TEXT subtype substitution** (3 keys + 1 consequential)~~ — the residue of that item, **done 2026-08-05** (REQ-053, [plan](../../../docs/plans/2026-08-05-flat-rm-attributes.md) Phase A): a carve-out in the substitution rule, not a suffix entry — a fully-captured `DV_CODED_TEXT` at a `DV_TEXT` leaf rides the DV_CODED_TEXT suffix set both ways; decorated coded texts and every other substituted subtype keep riding `|raw`.
2. **Composition metadata real-path spelling** (306 keys) — **decided 2026-08-03, [ADR 0015](../../../docs/adr/0015-flat-metadata-spelling.md): accept both spellings on input, emit `ctx/` only.** That unblocks REQ-115 and fixes a real rejection, but **it does not move these 306 keys and the coverage figure is unchanged.** Worth spelling out because the opposite is the natural assumption: this probe decodes upstream FLAT and *re-encodes* it, so a real-path key still comes back respelled as `ctx/` — one missing key plus one extra — and stays held out. Only emitting the reference's spelling would move them, which ADR 0015 rejects as a breaking output change.

   **What that rejection actually was**, since an earlier revision of this file got it wrong: it was not `ErrUnknownPath` over a respelling. The composition-level `language` / `territory` / `composer` leaves are CODE_PHRASE and PARTY_PROXY, and neither type had a suffix mapping, so an EHRbase-authored body failed as **`ErrUnsupportedDatatype`**. `ErrUnknownPath` was raised by `composer_self` alone, which reaches no Web Template node at all. The sharper problem came next: once the CODE_PHRASE leaf mapping landed (the row above), those keys stopped failing and started decoding **silently** through ordinary leaf placement, bypassing `ctx/` normalisation — the value landed straight on the RM attribute, where `applyContext` (which runs after content, assigning from the `ctx/` values) overwrote it, and a body carrying only the real path failed the mandatory-context check instead. Accepting the spelling is what closed that.

   Two families inside that count are **not** respellings, and they are no longer treated alike:

   - `composer|id` / `|id_scheme` / `|id_namespace` (12 keys) — the composer's `external_ref`, which the `ctx/` short forms structurally cannot carry. Since 2026-08-03 these are **genuinely refused and visible**: the hold-out is suffix-aware, so they reach decode, are refused as PARTY_PROXY, and appear in the excluded table above (12 of that row's 20 keys). They are therefore *outside* the 306 — which is why the hold-out fell from 318. Closing them means carrying `external_ref` on some new surface, not adding an alias.
   - `context/setting|*` (102 keys) — **was the one documented waiver until 2026-08-05**, when the amended REQ-053 landed `ctx/setting` emission and made it an ordinary respelling: encode writes `ctx/setting|code` + `|value` for a populated setting, decode aliases the real-path pair onto them with `|terminology` as an `openehr` witness, and a non-default setting now survives the round-trip. Its keys stay inside the 306 — held out on both sides exactly like `context/start_time` — so the census did not move; what cleared is the *loss* the hold-out used to waive, along with the waiver itself (the class is empty and `TestHoldOutMatchesCodecAliases` keeps it that way). The EVENT_CONTEXT `setting` rmpath exemption is now the same permanent double-spell as `start_time`'s.

   All 306 are therefore respellings: `language|code`/`|terminology`, `territory|code`/`|terminology`, `composer|name`, `context/start_time`, `context/setting|code`/`|value`/`|terminology`, plus the one `ctx/composer_self` the corpus writes.
3. **`_`-prefixed RM attributes** (1092 keys, 60% of the corpus) — by far the largest, and a genuine feature rather than a deviation to close: `_feeder_audit` (530 on its own), `_health_care_facility`, participations, `_uid`, links, reference ranges, workflow ids. Its own REQ.
