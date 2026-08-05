# Plan — FLAT residual closure: substitution carve-out, `ctx/setting`, underscore RM attributes

> **For agentic workers:** execute task-by-task with TDD (failing test → minimal code → green → commit). Each phase names its files, interfaces, and verification commands. Workers see only their own phase — the **Interfaces** blocks and the Design constraints are the contract between phases.

**Date:** 2026-08-05
**Status:** Draft (approved design; Phase 0 authored)
**Owner:** SDK maintainers
**Covers:** [REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes) (new — underscore-prefixed RM attributes; opens the wire-extension band 140–149); amendments to [REQ-053](../specifications/wire.md#req-053) (DV_TEXT substitution carve-out; `ctx/setting` as the sixth respelled field; DV_MULTIMEDIA / DV_PARSABLE / DV_INTERVAL / ENTRY-`subject` leaf closure — **prose not yet authored into `wire.md`; Phase C3 precondition, see Definition of Ready**); [ADR 0016](../adr/0016-event-context-optionals-underscore-spelling.md) (EVENT_CONTEXT optionals ride the underscore grammar)
**Probes:** [PROBE-089](../specifications/conformance.md#probe-089--underscore-attribute-round-trip) (Draft — reserved); [PROBE-086](../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) census re-baseline; [PROBE-076](../specifications/conformance.md#probe-076--flat--structured-composition-round-trip) corpus extension
**Implementation:** partial — Phases A and B landed (PR 1); Phases C0–C4 open (PR 2)
**Depends on:** landed REQ-053 codec (`openehr/serialize/simplified/`), REQ-106/111 (WebTemplate + compiled-template bridge), REQ-121 (`rmpath`), ADR 0014/0015; the PROBE-086 harness (`testkit/conformance/webtemplate/`) and pinned corpus (`testkit/cassettes/flat-conformance/`)
**Defers:** `_instruction_details` (ACTION) and `_wf_definition` (INSTRUCTION) — spec-named, corpus-unexercised, stay typed refusals; the composer `external_ref` / `composer/_identifier:N` / `composer/relationship` surface (ADR 0015 boundary); PARTICIPATION `time` (no channel in the reference's suffix set, corpus-unexercised); accepting the ITS `ctx/` sketches for EVENT_CONTEXT optionals (ADR 0016 § Decision 3); FEEDER_AUDIT_DETAILS `other_details` (ITEM_STRUCTURE — no corpus fixture); `.schema` media types; reused-sibling FLAT (owned by the REQ-116 residual)

## Goal

Close the REQ-053 residual deferrals in one coordinated effort: (A) the DV_TEXT subtype-substitution carve-out, (B) `ctx/setting` emission, and (C) the full REQ-140 underscore-prefixed RM attribute grammar plus the leaf-datatype closures its machinery enables (DV_MULTIMEDIA, DV_PARSABLE, DV_INTERVAL, ENTRY `subject`). Consumers: every FLAT/STRUCTURED integrator round-tripping real EHRbase-authored payloads — at HEAD, **0 of 34** upstream corpus bodies decode end-to-end because all refuse on `_`-prefixed keys; after this plan all 34 decode and PROBE-086 coverage rises from **19.7% (after Phase A) to ~80%** of upstream keys.

## Delivery shape — two PRs, one plan

- **PR 1** (branch `feat/req-053-flat-residuals`): Phase 0 SDD artefacts + Phases A and B. Small, quick review.
- **PR 2** (branch `feat/req-140-flat-rm-attributes`, stacked on PR 1): Phases C0–C4. The underscore grammar and datatype closures.

## Definition of Ready

- **Covers** lists REQ-140 + the REQ-053 amendments; canonical prose exists for REQ-140 (wire.md § REQ-140; registry row + band row in REQ.md) and for the Phase A/B REQ-053 amendments (DV_TEXT substitution, `ctx/setting`). ✔ (Phase 0, this branch)
- **Not yet authored:** the Phase C3 REQ-053 leaf-closure prose (DV_MULTIMEDIA / DV_PARSABLE / DV_INTERVAL suffix sets, ENTRY `subject`). Its grammar currently lives only in this plan's Phase C tables — a plan is not a canonical home, and wire.md § REQ-140 already dangles a reference to the DV_MULTIMEDIA suffix set it does not define. Authoring it into wire.md § REQ-053 is a **precondition of Phase C3**, not a side effect of it.
- ADR 0016 Accepted. ADR 0015's decision text stands unchanged; its Consequences carry a dated forward note that the `context/setting` waiver closed on 2026-08-05 (the decision itself is not reopened). ✔
- PROBE-089 catalogued Draft in conformance.md; PROBE-086 prose updated for the setting respelling. ✔
- traceability.yaml carries REQ-140 → this plan. ✔
- Phases below name tasks, files, and verification commands. ✔

## Definition of Done

- Code and tests land with `// REQ-140` / `// REQ-053` / `// PROBE-089` / `// PROBE-086` citations.
- REQ-140 **Impl.** flips `planned → landed` (REQ.md + traceability.yaml, tests enumerated); PROBE-089 Status flips to Implemented (Sandbox).
- `deviations.md` rewritten (rows below); `SKIPPED.md` regenerated (`-census`) with prose re-baselined; harness pins updated deliberately.
- roadmap.md, umbrella plan ([2026-06-23-simplified-formats.md](2026-06-23-simplified-formats.md)), plans README, CHANGELOG (one artefact-class bullet per PR merge) updated.
- `make ci` and `make spec-check` pass on each PR; plan archived under `docs/plans/archive/` after PR 2.

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0 — SDD artefacts (wire.md § REQ-140 + REQ-053 amendments, REQ.md row + band, conformance.md PROBE-089 + PROBE-086 prose, traceability.yaml, ADR 0016, this plan) | done |
| Phase A — DV_TEXT substitution carve-out | done |
| Phase B — `ctx/setting` emission + alias + harness waiver removal | done |
| PR 1 opened (A + B + Phase 0) | |
| Phase C0 — underscore router (decode) + emission hook (encode) + simple families | done |
| Phase C1 — value-decoration families (`_normal_range`, `_other_reference_ranges`, `_mapping`, `_null_flavour`, `_null_reason`) | done |
| Phase C2 — party grammar (`_health_care_facility`, participations, `_identifier`, ENTRY `subject`) | done |
| Phase C3 — DV_MULTIMEDIA / DV_PARSABLE / DV_INTERVAL leaves + `_feeder_audit` | done |
| Phase C4 — PROBE-089, census re-baseline, docs, status flips | |
| PR 2 opened (C0–C4) | |

## Design constraints (binding for every phase)

1. **Fail-loud, never silent** (REQ-053): an unrecognised `_`-segment is `ErrUnknownPath`; a recognised family with an unrecognised suffix is `ErrUnsupportedDatatype` naming the offending FLAT key; encode never drops a populated in-scope value — it emits, rides `|raw` where legal, or returns a typed error. No new error sentinels without updating `simplified.go`'s taxonomy godoc.
2. **Encode and decode land together per family.** The ratchet plan twice found decode-without-encode producing silent re-encode loss. Every family task carries a byte-exact FLAT → RM → FLAT round-trip test before it is done.
3. **The `|raw` boundary narrows deliberately**: a decorated value whose extras are now expressible rides suffixes + `_` keys instead of one `|raw` fragment. This changes emitted bytes for decorated values (precedent: the 2026-08-03 optional-suffix set); undecorated values stay byte-identical. Record in `deviations.md`.
4. **Building-block independence (REQ-013):** nothing new imports `transport/`, `auth/`, or `openehr/client/*`; `independence_test.go` stays green.
5. **Grammar vocabulary lives in `openehr/serialize/simplified`** (new file `rmattr.go` + siblings) — RM-type-keyed, recursive, one implementation for every position a type appears in (a PARTY_IDENTIFIED decomposes identically at `context/_health_care_facility`, inside `_participation:N`, and inside `_feeder_audit/…`).
6. **Reference spelling wins** (ADR 0014/0016): suffix names, `:N` indexing, the `original_content` vs `original_content_multimedia` choice-by-key-name, and PARTICIPATION's inlined `|identifiers_*:N` all match the vendored corpus byte-for-byte. When the corpus and this plan disagree, the corpus wins — inspect the fixture, then update this plan's table AND wire.md § REQ-140 in the same commit.
7. **TDD per task; Conventional Commits** (scope `serialize/simplified`, `testkit`, `docs`); pure functions take no `context.Context`.
8. **Census pins move deliberately**: every phase that lands corpus keys re-baselines `testkit/conformance/webtemplate/runner_test.go` pins in the same commit, with the delta stated in the commit body.

## Phase A — DV_TEXT substitution carve-out *(PR 1)*

**Files:** `openehr/serialize/simplified/datatypes.go` (encode rule), `flat_decode.go` (DV_TEXT leaf accepting coded suffixes), `datatypes_test.go`, `roundtrip_test.go`; `deviations.md` (datatype row); `testkit/conformance/webtemplate/runner_test.go` (pins).

**Read first:** `datatypes.go` lines ~55–90 (the substitution → `|raw` rule and its godoc; the one sanctioned substitution today is `|other`), the census rows in `SKIPPED.md` (`unsupported |suffix for DV_TEXT`, 3 keys + 1 consequential), fixture `ehrbase_conformance_data_types_dv_coded_text_as_dv_text.json`.

**Interfaces — produces:** encode: a `*rm.DvCodedText` at a `DV_TEXT`-typed WT leaf whose populated fields are within DV_CODED_TEXT's captured-key set emits the DV_CODED_TEXT suffix form — `<path>|code`, `<path>|value`, `<path>|terminology` (and `|formatting` etc. where populated), with **no bare key**. *(Corpus correction, 2026-08-05: the plan originally said "bare value + `|code` + `|terminology`", but the fixture carries the rubric under `|value` and no bare key — constraint 6, corpus wins; wire.md § REQ-053 amended in the same commit.)* Decode: at a `DV_TEXT` leaf, presence of `|code` selects the DV_CODED_TEXT builder, which then follows that type's own rules — `|code` without `|value` is an error (missing required), a bare value beside `|code` is refused, `|terminology` optional as for a genuine coded leaf.

**Tasks:**
- [x] Failing tests: encode `DvCodedText{value, defining_code}` at DV_TEXT leaf → suffix form (not `|raw`); decode those keys → `*rm.DvCodedText`; byte-exact round-trip; a *decorated* coded text (e.g. carrying `mappings` before Phase C1 lands) still rides `|raw`; `|other` behaviour unchanged.
- [x] Minimal implementation: extend the substitution rule in `datatypes.go` with the single DV_CODED_TEXT-at-DV_TEXT carve-out; extend the DV_TEXT leaf decoder.
- [x] Re-baseline the `dv_coded_text_as_dv_text` fixture pins (+4 compared: the 3 refused suffixes and the consequential `DV_TEXT missing required bare value`); regenerate census figures (356 → 360 compared, 1162 → 1158 excluded).
- [x] Update `deviations.md` datatype row (substitution sentence); commit `fix(serialize/simplified): carry DV_CODED_TEXT at a DV_TEXT leaf in suffix form (REQ-053)`.

**Verify:** `go test ./openehr/serialize/simplified/... ./testkit/...` and `go test ./testkit/conformance/webtemplate/ -run TestCensus -census -v`.

## Phase B — `ctx/setting` emission *(PR 1)*

**Files:** `openehr/serialize/simplified/flat_encode.go` (`emitContext`), `flat_decode.go` (ctx family + alias table region, ~lines 186–210), `context_test.go`; `testkit/conformance/webtemplate/` (hold-out derivation + `TestHoldOutMatchesCodecAliases`; delete the named-waiver carve-out); `openehr/template/webtemplate/` guard test `TestInContextLeavesResolveViaRmpath` (exemption reason text); `deviations.md`, `SKIPPED.md` prose.

**Read first:** `flat_encode.go:56–100` (emitContext), the alias-table machinery in `flat_decode.go` (comments at lines ~186–210 name the setting exclusion), ADR 0015, `SKIPPED.md` § metadata hold-out, `probe_076_simplified_round_trip.go` + `roundtrip_test.go` (establish which legs decode `WithTemplate`).

**Interfaces — produces:** encode: `ctx/setting|code` + `ctx/setting|value` when `comp.Context != nil` and `Setting` non-zero (`defining_code.code_string` non-empty); all-zero writes nothing; a setting whose `defining_code.terminology_id` ≠ `openehr`, or carrying extras beyond code+value (mappings, formatting…), is `ErrUnsupportedDatatype` naming `ctx/setting`. Decode: `ctx/setting|code`+`|value` build `DvCodedText{DefiningCode: CodePhrase{openehr, code}, Value: value}`; one of the pair alone is an error naming the missing key; real-path `context/setting|code`/`|value` normalise onto the `ctx/` keys via the alias table with `context/setting|terminology` as an `openehr` witness; disagreement between spellings is an error (existing machinery). `WithTemplate` default (`238 other care`) when absent — unchanged.

**Tasks:**
- [x] Failing tests: emit pair for populated setting; all-zero emits nothing; non-`openehr` setting errors; decode pair → exact `DvCodedText`; code-only / value-only errors; real-path spelling normalises; disagreement errors; witness mismatch errors.
- [x] Implement `emitContext` + ctx decode + alias entries (extend `MetadataAliasSpellings` / `MetadataWitnessSpellings` — the exported accessors the harness derives from).
- [x] **Round-trip interaction task:** determine whether PROBE-076's byte-idempotence legs decode `WithTemplate`. *(Resolved: the idempotence and interconversion legs are OPT-free; the conformance leg decodes `WithTemplate` but never re-encodes, and `names_test.go` compares only the key count — which the fixture's carried setting keeps stable.)* Pinned in `context_test.go` (`TestSettingWithTemplateDefaultRoundTrip`): WithTemplate decode → re-encode gains exactly the two default keys; OPT-free stays byte-identical. Recorded in `deviations.md` (§ RM-mandatory attributes — the one completion visible on re-encode).
- [x] Harness: setting leaves the named-waiver path — the hold-out derives it from the alias accessors; `TestHoldOutMatchesCodecAliases` reverse direction must fail if a waiver reappears (verified by mutation). Waiver code path deleted.
- [x] Guard test: re-class the `setting` exemption in `TestInContextLeavesResolveViaRmpath` to the permanent double-spell class (like `start_time`) — `rmpath` still does not resolve `…/context/setting` (emission is via `ctx/`), the *reason* changes.
- [x] Docs: `deviations.md` (drop the "`EVENT_CONTEXT.setting` is dropped on encode" deviation; move `setting` out of the deferred-`ctx/` row), `SKIPPED.md` § metadata (waiver → respelling; hold-out stays 306, census unchanged at 360/1158/306). Commit `feat(serialize/simplified): emit and accept ctx/setting (REQ-053, ADR 0015 gap closed)`.

**Verify:** as Phase A, plus `go test ./openehr/template/webtemplate/...`.

**PR 1 close-out:** plans README active-table row for this plan; `make ci` + `make spec-check`; open PR (base `main`) titled `feat(serialize/simplified): REQ-053 residuals — DV_TEXT substitution + ctx/setting (Phase 0 for REQ-140)`.

## Phase C0 — underscore router + emission hook + simple families *(PR 2)*

**Files:** create `openehr/serialize/simplified/rmattr.go`, `rmattr_encode.go`, `rmattr_test.go`; modify `flat_decode.go` (key splitter), `flat_encode.go` (post-node hook); `deviations.md`.

**Read first:** `flat_decode.go` (suffix grouping by base path; `resolveLeaf`/`placeLeaf`; the `:index` strictness rules — canonical spelling, no gaps, `maxRepeatIndex` budget **apply to `_family:N` indexes too**), `flat_encode.go` (`emitNode` — resolution first, leaf mapping second), `rmpath` (`ErrPathNotFound` / `skipNotFound` routing), `openehr/rm` generated types (`Locatable` embedding: `Uid`, `Links`, `FeederAudit`; `EventContext`; `ObjectRef`; `Link`).

**Interfaces — produces (later phases consume):**
- `rmattrDecode(owner rmattrOwner, family string, index int, group suffixGroup) error` — one entry point routing to per-family typed decoders; `rmattrOwner` wraps the resolved base (RM node or pending placement) + its RM kind.
- `rmattrEncode(owner any, base string, out map[string]any) error` — inspects the owner's RM type, emits every populated in-scope `_` key under `base`.
- The **key splitter** in decode: a path segment with `_` prefix (after `:N` strip) ends WT resolution at the preceding node; the tail (family, optional index, optional subpath, suffixes) is grouped per (basePath, family, index) and handed to `rmattrDecode`. `<root>/context/_*` resolves the owner as the composition's `EVENT_CONTEXT` whether or not the Web Template carries a `context` node.
- Suffix-set helpers reused by C1–C3: `objectRefSuffixes` (`|id`, `|id_scheme`, `|namespace`, `|type` ↔ `rm.ObjectRef` with `GenericId` carrying the scheme), bare-value families (`_uid` ↔ `rm.UidBasedId` via the existing identifier parsing; `_end_time` DV_DATE_TIME; `_location` String).

**Families landed here:** `_uid` (any LOCATABLE incl. root), `_link:N` (`|meaning`, `|type`, `|target` ↔ `rm.Link{Meaning DvText, Type DvText, Target DvEhrUri}`), `_work_flow_id` + `_guideline_id` (ENTRY, OBJECT_REF), `context/_end_time`, `context/_location`.

**Tasks (TDD each family):**
- [ ] Failing round-trip tests per family against a vendored OPT fixture (use `fixtures.FlatConformanceOpt` and hand-authored minimal FLAT maps); unknown family stays `ErrUnknownPath`; known family + bogus suffix (`_link:0|typo`) errors naming the key; `_link:1` without `_link:0` errors (existing sparse-index rule); STRUCTURED interconversion carries the keys (`structured.go` needs no OPT — add cases to `structured_test.go`).
- [ ] Implement splitter + hook + the five families; wire encode: `emitNode` calls `rmattrEncode` after leaf/locatable emission; composition root gets the call from the top-level marshal.
- [ ] Corpus movement: re-baseline pins for fixtures whose only unmodelled keys were these families; commit per family or per coherent pair.

**Verify:** package tests + census regeneration after the last family.

## Phase C1 — value-decoration families *(PR 2)*

**Files:** `rmattr_value.go` + `rmattr_value_encode.go` (new), `rmattr.go` (registry + router), `flat_decode.go` (the owner's anchor type), `flat_encode.go` (the absent-value leaf walk), `datatypes.go` (captured-set interplay), `rmattr_value_test.go` (new) + `datatypes_test.go` + `structured_test.go`; `deviations.md`, `SKIPPED.md`, `runner_test.go` pins.

**Read first:** `datatypes.go` capturedKeys + the decorated-value `|raw` test; `rm.DvInterval` / `rm.ReferenceRange` / `rm.TermMapping` / `rm.Element` (null flavour) generated shapes; corpus shapes: `_normal_range/lower|magnitude`, `_normal_range|lower_included`, `_other_reference_ranges:0/meaning|code`, `_mapping:0|match`, `_mapping:0/target|terminology`, `_mapping:0/purpose|value`.

**Interfaces — produces:** the interval grammar in one place, decode and encode, for C3's DV_INTERVAL leaf to reuse:

- decode — `intervalSuffixes(g rmattrGroup, ts rmattrTails, anchor string) (map[string]any, error)` in `rmattr_value.go`: `/lower` + `/upper` sub-objects decoded through `dvFromSuffixes(anchor, …)` (the clinical leaf's own builder — no per-datatype logic duplicated, `|raw` bypass included), plus the four boundary Booleans. `ts` comes from `splitRMAttrTails(g)`, which partitions a family instance's tails into own suffixes and one suffix map per sub-path segment and normalises the `:0` the STRUCTURED interconversion adds; `ts.check(g, own, sub)` is the per-family allowlist.
- encode — `intervalToFlat[T any](out map[string]any, base, anchor string, iv rm.Interval[T]) error` in `rmattr_value_encode.go`, bounds emitted through `emitLeafValue(out, path, v, rmType, listOpen, decorated bool) (string, error)` (the `leafToFlat` body, split out; the returned suffix type is `""` when the value rode `|raw`, which is also how `leafToFlat` knows whether the `_` decorations are still owed).

*(Corpus corrections, 2026-08-05 — constraint 6, wire.md § REQ-140 amended in the same commit: (1) REFERENCE_RANGE's RM `range` level is **elided** — bounds and boundary Booleans sit directly under `_other_reference_ranges:N`; (2) the boundary Booleans are asymmetric — absent `|*_unbounded` is `false`, absent `|*_included` is the **closed** endpoint `true`, and each is emitted only when it contradicts that default. Three corpus fixtures disagree about which flags they spell, and this is the only mapping under which all three round-trip byte-exactly; (3) `_null_flavour` is corpus-exercised after all — `ehrbase_conformance_Element_null_flavor.json` carries it with an explicit `|terminology`, so it rides the ordinary DV_CODED_TEXT suffix grammar rather than an `openehr`-implied pair.)*

**Families:** `_normal_range` (DV_INTERVAL of the leaf's anchor type), `_other_reference_ranges:N` (REFERENCE_RANGE = interval + `/meaning` DV_TEXT/DV_CODED_TEXT), `_mapping:N` (TERM_MAPPING: `|match` single-char, `/target` CODE_PHRASE `|code`+`|terminology`, `/purpose` DV_CODED_TEXT `|code`+`|terminology`+`|value`), `_null_flavour` (`|code`+`|value`, `openehr` implied — legal beside an **absent** bare value: an `ELEMENT` with `NullFlavour` set and nil `Value` must emit the `_null_flavour` keys with no bare key, and decode back; today's "typed-nil pointer = skipped leaf" rule must not skip the attribute walk), `_null_reason` (bare DV_TEXT).

**The captured-set change:** a `DV_*` value whose only extras are `normal_range` / `other_reference_ranges` / `mappings` no longer rides `|raw` — the "fully captured" test consults the rmattr grammar. A value with extras *still* outside the grammar (e.g. non-`openehr` `normal_status` — unchanged rule) keeps riding `|raw`. Pin both directions in `datatypes_test.go`.

**Tasks:**
- [x] `_normal_range` + `_other_reference_ranges:N` (they share the interval grammar): failing round-trip tests over the QUANTITY / COUNT / DATE_TIME / ORDINAL / PROPORTION anchors → implement → pins for the eight `data_types_dv_*` fixtures. Census 494 → 564 compared.
- [x] `_mapping:N`, composed with all three DV_TEXT shapes (plain, genuinely coded, Phase A substituted coded-at-text). Census 564 → 591.
- [x] `_null_flavour` + `_null_reason`, including the absent-value case in **both** directions (`emitNode` now runs the owner walk on a leaf whose value resolves to nothing). Census 591 → 595 (32.6%).
- [x] `|raw` boundary pinned both ways in `datatypes_test.go` (`TestNormalRangeNarrowsRawBoundary`, `TestDecoratedCodedTextAtTextLeaf`); STRUCTURED interconversion cases in `structured_test.go` for the nested-object and array shapes.

**Residue this phase leaves** (all recorded in `deviations.md` / `SKIPPED.md`): the DV_PROPORTION interval's four **derived** bare magnitudes join the leaf-level `unsupported bare value for DV_PROPORTION` refusal (1 → 5 keys) — 101 of the families' 105 corpus keys are compared; a bare-spelled leaf carrying an underscore family has no STRUCTURED representation (pre-existing, C0's `_uid` collides identically, now reachable from corpus bodies); and a null-flavoured instance of a *repeating* collapsed ELEMENT is not emitted, because the `:index` comes from the value-list enumeration.

## Phase C2 — party grammar + ENTRY `subject` *(PR 2)*

**Files:** `rmattr_party.go` + tests; `datatypes.go` (PARTY_PROXY leaf mapping for `subject`); `flat_decode.go` (composer refusal); `deviations.md`.

**Read first:** `rm.PartyIdentified` / `PartyRelated` / `PartySelf` / `Participation` / `DvIdentifier` shapes; corpus: `context/_health_care_facility|id|id_scheme|id_namespace|name`, `context/_participation:0|function|mode|name|id…`, `_other_participation:0|identifiers_id:0`, `_other_participation:0/relationship|code`, `composer/_identifier:0|id` (must refuse), `subject|id|id_scheme|id_namespace|name` in `party_identified`/`party_related` fixtures; ADR 0015 § Decision 5.

**Interfaces — produces:** in `rmattr_party.go` / `rmattr_party_encode.go`, decode and encode symmetric:

- decode — `partySuffixes(g rmattrGroup, ts rmattrTails, identifiers []any) (party map[string]any, populated bool, err error)` assembles the party from one position's tails (`|id` `|id_scheme` `|id_namespace` `|type` `|name`, PARTY_RELATED's `/relationship`); the identifier list is *passed in* because the two positions spell it differently, and `partyIdentifiers(g, ts)` decodes the nested `_identifier:N` form. `populated == false` is the no-party-key case the caller resolves (PARTY_SELF for a performer, absence for an ENTRY `subject`). `decodeRMAttrParticipation` adds `|function` / `|mode` and `takeInlineIdentifiers(g, ts)`.
- encode — `partyRMAttr(out map[string]any, path string, p any) error` (party + nested `_identifier:N`) and `partySuffixesToFlat(out, path, p) ([]rm.DVIdentifier, error)` (suffixes only, returning the list for the caller's own spelling); `participationsRMAttr(out, base, family string, ps []rm.Participation) error`.
- the C1 tail machinery grew a third position for this: `splitRMAttrTails(g, lists map[string]bool)` (existing callers pass `nil`) partitions an indexed sub-path segment into `rmattrTails.sublist`, with `listSlots` / `listValue` / `ownString` beside `value` / `boolTail`.

C3 consumes `partySuffixes` / `partyRMAttr` for FEEDER_AUDIT_DETAILS' `/location`, `/subject`, `/provider`.

**Tasks:**
- [x] **Inspect first:** *(Done 2026-08-05 — `|mode` carries the bare openEHR rubric with **no code**, `|function` a plain string.)* The `participation mode` group (codes 193–224) is vendored in `rmattr_party.go` as `participationModes`, inverted once for the decode direction; an unknown rubric is refused loudly and encode refuses a mode whose code/terminology/value the rebuild would not reproduce. wire.md § REQ-140 amended with both findings plus the `|type` rule below. Recorded in `deviations.md` § vendored vocabularies.
- [x] Failing round-trip tests: `_health_care_facility` (PARTY_IDENTIFIED + PARTY_RELATED + HIER_OBJECT_ID ref), `context/_participation:N` (multi-instance, one with a PARTY_SELF performer), `_other_participation:N` on ENTRY (relationship + inlined identifiers), nested `_identifier:N` on a standalone party; owner admission (`_participation` on an ENTRY, `_other_participation` on a SECTION, `_health_care_facility` on an ENTRY) refused via rminfo.
- [x] **Corpus correction (constraint 6), wire.md amended in the same commit:** the party `external_ref`'s `type` is **not** fixed. The reference writes no `|type` and hardcodes `PARTY`, but the vendored PROBE-076 fixture `clinical_content_validation.json` carries `PERSON` and `ORGANISATION` references at `composer`, `health_care_facility` and every participation performer — refusing them would have failed a green probe, and normalising them to `PARTY` is the silent loss REQ-053 forbids. So absent `|type` decodes as `PARTY` (byte-exact for every corpus body, which never gains the key) and encode emits `|type` only when the value differs. The suffix is the reference's own spelling of the same RM attribute in the OBJECT_REF families.
- [x] Composer boundary: `composer|id*` stays the PARTY_PROXY leaf refusal (12 corpus keys), and the composer's party *sub-structure* — `composer/_identifier:N` (8 keys) and `composer/relationship` (3) — is refused in `siphonContext` with a typed error naming the key and the ADR 0015 boundary; pinned in `context_test.go`.
- [x] ENTRY `subject` leaf: `leafToFlat` maps PARTY_PROXY — PARTY_SELF emits nothing (the `WithTemplate` default; symmetric), PARTY_IDENTIFIED emits the party suffixes, PARTY_RELATED adds `/relationship`; decode siphons the whole leaf (own suffixes + `/relationship` + `/_identifier:N`) as one party group *before* the `_` router, because those three key shapes address one RM value and the party's concrete subtype is only known once all three are in hand. `rmpath` gained `subject` on the five ENTRY subtypes (REQ-121 pattern) and the `TestInContextLeavesResolveViaRmpath` exemptions are deleted; `emitNode` gained an explicit `ctx/`-only leaf guard so the composer stays ctx-spelled now that PARTY_PROXY is emittable. Removes the "documented skip" for subject in `deviations.md`.
- [x] Census pins; commits per family. Census 595 → 885 compared, 923 → 633 excluded (32.6% → 48.5%); every one of the four families' 290 corpus keys compared, no residue. The PARTY_PROXY excluded row falls 20 → 12 (exactly the composer `external_ref`), and the 11 composer sub-structure keys move from `path not in web template` to their own named row.

**`_identifier` is not a router family.** The plan listed it as one; it is only ever reached *inside* a party (nested in `_health_care_facility`'s tails, at the `subject` leaf, or under a C3 FEEDER_AUDIT_DETAILS party), so the party implementation owns it and the router has no owner to judge it against — `<entry>/_identifier:0` stays `ErrUnknownPath`. The composition's composer is the one path that would have reached it as a family, and that is the ADR 0015 refusal above.

## Phase C3 — encapsulated leaves + `_feeder_audit` *(PR 2)*

**Files:** `datatypes.go` + `flat_decode.go` (DV_MULTIMEDIA, DV_PARSABLE, DV_INTERVAL leaf mappings, the composite-leaf siphon), `rmattr.go` (five new value families + the `attrType` gate), `rmattr_value.go` / `rmattr_value_encode.go`, `rmattr_feeder.go` + `rmattr_feeder_encode.go` (new), `rmattr_party.go` / `rmattr_party_encode.go` (the PARTY_PROXY `|_type` position), `structured.go` (the `"|"` member); `rmattr_encapsulated_test.go` + `rmattr_feeder_test.go` (new), `datatypes_test.go`, `structured_test.go`, `hardening_test.go`; `deviations.md`, `SKIPPED.md`, `runner_test.go` pins.

**Read first:** corpus fixtures `ehrbase_conformance_data_types_dv_multimedia.json`, `…dv_parsable.json`, `…interval_dv_quantity.json`, `ehrbase_conformance_Element_feeder_audit.json` + `…feeder_audit_multimedia.json` + `…party_self.json`; `rm.DVMultimedia` / `DVParsable` / `DVInterval` / `FeederAudit` / `FeederAuditDetails` shapes.

**Families / leaves:**
- [x] DV_PARSABLE first-class leaf (bare value + `|formalism`, both RM-mandatory); `ACTIVITY.timing` leaves the `|raw` ride when fully captured. Since the widened captured set covers **all four** of DV_PARSABLE's attributes, a DV_PARSABLE at a Web Template leaf never rides `|raw` again; the boundary that remains is at a *nested* position (`original_content`), where `charset` / `language` have no key and the value rides `|raw` whole — pinned both ways.
- [x] DV_MULTIMEDIA first-class leaf. **Corpus corrections (constraint 6), wire.md § REQ-140 amended in the same commit:** (1) the bare key is the **`uri`**, not the inline data — the fixture writes `dv_multimedia: "http://med.tube.com/sample"` and puts the octets under `|data` (its `_thumbnail` spells `|data` and no bare key at all), so this plan's "bare data" was wrong; (2) `|mediatype` and `|alternatetext` carry no underscore where the RM attribute does; (3) the three CODE_PHRASE-valued attributes travel as a **bare code** — `media_type` in the implied `IANA_media-types` (the identifier REQ-107's `rmwrite` already pins, corroborated by the vendored canonical `Test_dv_multimedia_open_constraint.v0` and `Demonstration.v1`), `|integrity_check_algorithm` / `|compression_algorithm` with **no** implied terminology this codec can source — and a value the rebuild would re-terminologise rides `|raw`, the `|normal_status` rule one attribute up; (4) `|data` *and* `|integrity_check` carry the base64 of the RM's `Byte[]`, which is what the canonical form uses.
- [x] DV_INTERVAL leaf — C1's `intervalSuffixes` / `intervalToFlat` verbatim. The Web Template names the bound datatype inside the angle brackets (`DV_INTERVAL<DV_QUANTITY>`, six of them on the corpus template's `conformance_interval` OBSERVATION), which is the only place the anchor can come from; the corpus spelling is exactly C1's (`/lower|magnitude`, `|lower_included`, `|upper_unbounded`), so wire.md needed only the leaf row. Its keys are siphoned by C2's party-leaf mechanism, generalised to `compositeLeafGroups` — a childless leaf whose FLAT form has sub-paths of its own rather than a suffix set.
- [x] `_charset` / `_language` / `_encoding` / `_thumbnail` / `_accuracy` join the router as **value families**, admitted by `rminfo` alone. That is what made three of C1's named residuals fall out almost free, and they are taken: DV_TEXT's `_language` (13 keys — including the corpus's `|preferred_term`, which the standalone CODE_PHRASE leaf grammar therefore gained) and `_encoding` (6), and the DV_TEMPORAL `_accuracy` (3, gated by a new `rmattrFamily.attrType` so the Real-typed `accuracy` on DV_QUANTITY / COUNT / DURATION / PROPORTION keeps its scalar `|accuracy` suffix and one attribute never has two channels at one owner).
- [x] `_feeder_audit` (any LOCATABLE — nine positions across 14 corpus bodies). `rmattrChildGroups` adds the level the C1 tail machinery cannot express (it admits a `:index` only on the leading sub-path segment, and this family needs `…/originating_system_audit/provider/_identifier:0|id`): it re-roots each sub-path segment into its own `rmattrGroup`, so a FEEDER_AUDIT_DETAILS party is decoded by C2's `partyLeafSuffixes` verbatim, an item id and an `original_content` by the datatype leaf builder. **Corpus correction (constraint 6), wire.md amended:** the FEEDER_AUDIT_DETAILS `subject` spells PARTY_SELF **explicitly**, as `subject|_type: "PARTY_SELF"` — it has to, because the attribute is RM-optional there, so the absence of every party key already means *absent* (unlike a PARTICIPATION performer or an ENTRY `subject`, both RM-mandatory, where absence *is* the PARTY_SELF spelling). `other_details` refuses loudly on both sides.
- [x] `_provider` taken with it (1 key — the last unclaimed underscore family the corpus writes): ENTRY.provider is the second RM-optional PARTY_PROXY position, so it is the same `partyProxyLeafSuffixes` / `partyProxyRMAttr` pair at one more owner, and neither `rmpath` nor the Web Template builder projects the attribute, so there is no double-spelling hazard.
- [x] **STRUCTURED** gained a spelling for a bare leaf value beside the same leaf's members — the `"|"` member, i.e. the `"|"+suffix` convention with the empty suffix. It had to: a DV_MULTIMEDIA leaf always carries a bare uri *and* mandatory suffixes, so without it the leaf could not interconvert at all and two green PROBE-076 legs would have turned red. This closes the residual C1 recorded as an expected refusal (`_uid` / `_normal_range` beside a bare leaf), reversibly and without an OPT — `|value` would not have been reversible, since DV_ORDINAL and DV_CODED_TEXT spell a real `|value`.
- [x] Census pins re-baselined: 885 → 1466 compared, 633 → 52 excluded (48.5% → 80.4%). Every `path not in web template` refusal bar 22 keys collapsed.

**Residue this phase leaves** (all in `deviations.md` / `SKIPPED.md`): 52 corpus keys, none of them an underscore family this phase claimed — the composer boundary 23 (ADR 0015: `external_ref` 12 + party sub-structure 11), the DV_PROPORTION derived bare magnitudes 5, the two deferred families `_instruction_details` 3 + `_wf_definition` 2, and 20 keys the **WebTemplate builder** does not project at all (the `ism_transition/*` set 10 including its `_reason:0`, INTERVAL_EVENT `math_function` 3 + `width` 1 + `|sample_count` 1, ACTION `time` 1, OBSERVATION `history_origin` 1, CLUSTER `labresult/text_value` 1, ACTIVITY `action_archetype_id` 1). Nothing in that list is reachable by widening the underscore grammar.


## Phase C4 — probe, census, docs, status flips *(PR 2 close-out)*

- [ ] PROBE-089 probe: `testkit/probes/serialize/probe_089_underscore_round_trip.go` (repo probe pattern — see `probe_086_upstream_flat_parity.go` wrapper style) + per-family fixture set; run by `TestProbe089`; conformance.md Status flip to Implemented (Sandbox), summary-table cell update.
- [ ] Census re-baseline: regenerate `SKIPPED.md` (`go test ./testkit/conformance/webtemplate/ -run TestCensus -census -v`); rewrite its prose — excluded-families table (expect residue: composer boundary 16 keys, derived DV_PROPORTION magnitude 1, `ism_transition/*` + ACTION `time` + `|sample_count` + STRING `action_archetype_id` as the non-underscore WebTemplate-builder residue), coverage headline (~80%), "What would move these numbers" rewritten.
- [ ] `deviations.md` full pass: deferred-features table rows for `_` attributes, subject, datatypes cleared/rewritten; `|raw` boundary text updated; vendored vocabularies (if any) recorded.
- [ ] Status flips: REQ.md REQ-140 row `planned → landed`; traceability.yaml `implementation: landed` + `tests:` enumerated; roadmap.md FLAT/STRUCTURED row + REQ-140 mention; umbrella plan residual paragraph rewritten (remaining: `.schema` acceptance, reused-sibling FLAT, deferred `_instruction_details`/`_wf_definition`/`other_details`, ITS `ctx/` sketches); plans README (this plan → archive note); CHANGELOG one bullet.
- [ ] `make ci` + `make spec-check`; archive plan (`git mv` to `docs/plans/archive/`, index row) — after PR 2 merges, per repo convention.

## Verification commands (all phases)

```
go test ./openehr/serialize/simplified/...        # package tests
go test ./testkit/conformance/webtemplate/...     # harness + pins
go test ./testkit/conformance/webtemplate/ -run TestCensus -census -v   # census
go test ./testkit/probes/serialize/...            # probes
make fmt && make ci                               # full gate (includes spec-check)
```
