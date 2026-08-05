# Plan — FLAT residual closure: substitution carve-out, `ctx/setting`, underscore RM attributes

> **For agentic workers:** execute task-by-task with TDD (failing test → minimal code → green → commit). Each phase names its files, interfaces, and verification commands. Workers see only their own phase — the **Interfaces** blocks and the Design constraints are the contract between phases.

**Date:** 2026-08-05
**Status:** Draft (approved design; Phase 0 authored)
**Owner:** SDK maintainers
**Covers:** [REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes) (new — underscore-prefixed RM attributes; opens the wire-extension band 140–149); amendments to [REQ-053](../specifications/wire.md#req-053) (DV_TEXT substitution carve-out; `ctx/setting` as the sixth respelled field; DV_MULTIMEDIA / DV_PARSABLE / DV_INTERVAL / ENTRY-`subject` leaf closure); [ADR 0016](../adr/0016-event-context-optionals-underscore-spelling.md) (EVENT_CONTEXT optionals ride the underscore grammar)
**Probes:** [PROBE-089](../specifications/conformance.md#probe-089--underscore-attribute-round-trip) (Draft — reserved); [PROBE-086](../specifications/conformance.md#probe-086--upstream-flat-serialisation-parity) census re-baseline; [PROBE-076](../specifications/conformance.md#probe-076--flat--structured-composition-round-trip) corpus extension
**Implementation:** planned
**Depends on:** landed REQ-053 codec (`openehr/serialize/simplified/`), REQ-106/111 (WebTemplate + compiled-template bridge), REQ-121 (`rmpath`), ADR 0014/0015; the PROBE-086 harness (`testkit/conformance/webtemplate/`) and pinned corpus (`testkit/cassettes/flat-conformance/`)
**Defers:** `_instruction_details` (ACTION) and `_wf_definition` (INSTRUCTION) — spec-named, corpus-unexercised, stay typed refusals; the composer `external_ref` / `composer/_identifier:N` surface (ADR 0015 boundary); accepting the ITS `ctx/` sketches for EVENT_CONTEXT optionals (ADR 0016 § Decision 3); FEEDER_AUDIT_DETAILS `other_details` (ITEM_STRUCTURE — no corpus fixture); `.schema` media types; reused-sibling FLAT (owned by the REQ-116 residual)

## Goal

Close the REQ-053 residual deferrals in one coordinated effort: (A) the DV_TEXT subtype-substitution carve-out, (B) `ctx/setting` emission, and (C) the full REQ-140 underscore-prefixed RM attribute grammar plus the leaf-datatype closures its machinery enables (DV_MULTIMEDIA, DV_PARSABLE, DV_INTERVAL, ENTRY `subject`). Consumers: every FLAT/STRUCTURED integrator round-tripping real EHRbase-authored payloads — at HEAD, **0 of 34** upstream corpus bodies decode end-to-end because all refuse on `_`-prefixed keys; after this plan all 34 decode and PROBE-086 coverage rises from **19.5% to ~80%** of upstream keys.

## Delivery shape — two PRs, one plan

- **PR 1** (branch `feat/req-053-flat-residuals`): Phase 0 SDD artefacts + Phases A and B. Small, quick review.
- **PR 2** (branch `feat/req-140-flat-rm-attributes`, stacked on PR 1): Phases C0–C4. The underscore grammar and datatype closures.

## Definition of Ready

- **Covers** lists REQ-140 + the REQ-053 amendments; canonical prose exists (wire.md § REQ-140, amended § REQ-053; registry row + band row in REQ.md). ✔ (Phase 0, this branch)
- ADR 0016 Accepted; ADR 0015 untouched. ✔
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
| Phase A — DV_TEXT substitution carve-out | |
| Phase B — `ctx/setting` emission + alias + harness waiver removal | |
| PR 1 opened (A + B + Phase 0) | |
| Phase C0 — underscore router (decode) + emission hook (encode) + simple families | |
| Phase C1 — value-decoration families (`_normal_range`, `_other_reference_ranges`, `_mapping`, `_null_flavour`, `_null_reason`) | |
| Phase C2 — party grammar (`_health_care_facility`, participations, `_identifier`, ENTRY `subject`) | |
| Phase C3 — DV_MULTIMEDIA / DV_PARSABLE / DV_INTERVAL leaves + `_feeder_audit` | |
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

**Interfaces — produces:** encode: a `*rm.DvCodedText` at a `DV_TEXT`-typed WT leaf whose populated fields are within DV_CODED_TEXT's captured-key set emits `<path>` (bare value), `<path>|code`, `<path>|terminology` (and `|formatting` etc. where populated). Decode: at a `DV_TEXT` leaf, presence of `|code` selects the DV_CODED_TEXT builder; `|code` without the bare value is an error (missing required); `|terminology` defaulting follows the existing DV_CODED_TEXT rules.

**Tasks:**
- [ ] Failing tests: encode `DvCodedText{value, defining_code}` at DV_TEXT leaf → suffix form (not `|raw`); decode those keys → `*rm.DvCodedText`; byte-exact round-trip; a *decorated* coded text (e.g. carrying `mappings` before Phase C1 lands) still rides `|raw`; `|other` behaviour unchanged.
- [ ] Minimal implementation: extend the substitution rule in `datatypes.go` with the single DV_CODED_TEXT-at-DV_TEXT carve-out; extend the DV_TEXT leaf decoder.
- [ ] Re-baseline the `dv_coded_text_as_dv_text` fixture pins (+4 compared: the 3 refused suffixes and the consequential `DV_TEXT missing required bare value`); regenerate census figures.
- [ ] Update `deviations.md` datatype row (substitution sentence); commit `fix(serialize/simplified): carry DV_CODED_TEXT at a DV_TEXT leaf in suffix form (REQ-053)`.

**Verify:** `go test ./openehr/serialize/simplified/... ./testkit/...` and `go test ./testkit/conformance/webtemplate/ -run TestCensus -census -v`.

## Phase B — `ctx/setting` emission *(PR 1)*

**Files:** `openehr/serialize/simplified/flat_encode.go` (`emitContext`), `flat_decode.go` (ctx family + alias table region, ~lines 186–210), `context_test.go`; `testkit/conformance/webtemplate/` (hold-out derivation + `TestHoldOutMatchesCodecAliases`; delete the named-waiver carve-out); `openehr/template/webtemplate/` guard test `TestInContextLeavesResolveViaRmpath` (exemption reason text); `deviations.md`, `SKIPPED.md` prose.

**Read first:** `flat_encode.go:56–100` (emitContext), the alias-table machinery in `flat_decode.go` (comments at lines ~186–210 name the setting exclusion), ADR 0015, `SKIPPED.md` § metadata hold-out, `probe_076_simplified_round_trip.go` + `roundtrip_test.go` (establish which legs decode `WithTemplate`).

**Interfaces — produces:** encode: `ctx/setting|code` + `ctx/setting|value` when `comp.Context != nil` and `Setting` non-zero (`defining_code.code_string` non-empty); all-zero writes nothing; a setting whose `defining_code.terminology_id` ≠ `openehr`, or carrying extras beyond code+value (mappings, formatting…), is `ErrUnsupportedDatatype` naming `ctx/setting`. Decode: `ctx/setting|code`+`|value` build `DvCodedText{DefiningCode: CodePhrase{openehr, code}, Value: value}`; one of the pair alone is an error naming the missing key; real-path `context/setting|code`/`|value` normalise onto the `ctx/` keys via the alias table with `context/setting|terminology` as an `openehr` witness; disagreement between spellings is an error (existing machinery). `WithTemplate` default (`238 other care`) when absent — unchanged.

**Tasks:**
- [ ] Failing tests: emit pair for populated setting; all-zero emits nothing; non-`openehr` setting errors; decode pair → exact `DvCodedText`; code-only / value-only errors; real-path spelling normalises; disagreement errors; witness mismatch errors.
- [ ] Implement `emitContext` + ctx decode + alias entries (extend `MetadataAliasSpellings` / `MetadataWitnessSpellings` — the exported accessors the harness derives from).
- [ ] **Round-trip interaction task:** determine whether PROBE-076's byte-idempotence legs decode `WithTemplate`. If yes, the synthesised default setting now re-emits — pin the resolved behaviour with an explicit test and record it in `deviations.md` (the added `ctx/setting` keys are a *faithful* encoding of the completed composition, not drift). If the idempotence legs are OPT-free, add the pinning test to `context_test.go` anyway (WithTemplate decode → re-encode gains exactly the two default keys).
- [ ] Harness: setting leaves the named-waiver path — the hold-out derives it from the alias accessors; `TestHoldOutMatchesCodecAliases` reverse direction must fail if a waiver reappears. Delete the waiver code path.
- [ ] Guard test: re-class the `setting` exemption in `TestInContextLeavesResolveViaRmpath` to the permanent double-spell class (like `start_time`) — `rmpath` still does not resolve `…/context/setting` (emission is via `ctx/`), the *reason* changes.
- [ ] Docs: `deviations.md` (drop the "`EVENT_CONTEXT.setting` is dropped on encode" deviation; move `setting` out of the deferred-`ctx/` row), `SKIPPED.md` § metadata (waiver → respelling; hold-out stays 306, census unchanged). Commit `feat(serialize/simplified): emit and accept ctx/setting (REQ-053, ADR 0015 gap closed)`.

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

**Files:** `rmattr.go` (+`rmattr_interval.go` if it grows), `datatypes.go` (captured-set interplay), tests; `deviations.md`.

**Read first:** `datatypes.go` capturedKeys + the decorated-value `|raw` test; `rm.DvInterval` / `rm.ReferenceRange` / `rm.TermMapping` / `rm.Element` (null flavour) generated shapes; corpus shapes: `_normal_range/lower|magnitude`, `_normal_range|lower_included`, `_other_reference_ranges:0/meaning|code`, `_mapping:0|match`, `_mapping:0/target|terminology`, `_mapping:0/purpose|value`.

**Interfaces — produces:** `intervalSuffixes(anchor dvKind)` — `/lower` + `/upper` subgroups decoded with the anchor datatype's own captured-key set (reuse `leafToFlat`/leaf builders — do **not** duplicate per-datatype logic), `|lower_included` `|upper_included` `|lower_unbounded` `|upper_unbounded` booleans. Consumed by C3's DV_INTERVAL leaf.

**Families:** `_normal_range` (DV_INTERVAL of the leaf's anchor type), `_other_reference_ranges:N` (REFERENCE_RANGE = interval + `/meaning` DV_TEXT/DV_CODED_TEXT), `_mapping:N` (TERM_MAPPING: `|match` single-char, `/target` CODE_PHRASE `|code`+`|terminology`, `/purpose` DV_CODED_TEXT `|code`+`|terminology`+`|value`), `_null_flavour` (`|code`+`|value`, `openehr` implied — legal beside an **absent** bare value: an `ELEMENT` with `NullFlavour` set and nil `Value` must emit the `_null_flavour` keys with no bare key, and decode back; today's "typed-nil pointer = skipped leaf" rule must not skip the attribute walk), `_null_reason` (bare DV_TEXT).

**The captured-set change:** a `DV_*` value whose only extras are `normal_range` / `other_reference_ranges` / `mappings` no longer rides `|raw` — the "fully captured" test consults the rmattr grammar. A value with extras *still* outside the grammar (e.g. non-`openehr` `normal_status` — unchanged rule) keeps riding `|raw`. Pin both directions in `datatypes_test.go`.

**Tasks:** failing round-trip test per family (fixture-shaped keys above) → implement → census pins → commit per family. The corpus's `_normal_range/lower|type` / `|ordinal` / `|denominator` shapes come from DV_PROPORTION / DV_ORDINAL anchors — cover at least QUANTITY, PROPORTION, ORDINAL anchors in tests.

## Phase C2 — party grammar + ENTRY `subject` *(PR 2)*

**Files:** `rmattr_party.go` + tests; `datatypes.go` (PARTY_PROXY leaf mapping for `subject`); `flat_decode.go` (composer refusal); `deviations.md`.

**Read first:** `rm.PartyIdentified` / `PartyRelated` / `PartySelf` / `Participation` / `DvIdentifier` shapes; corpus: `context/_health_care_facility|id|id_scheme|id_namespace|name`, `context/_participation:0|function|mode|name|id…`, `_other_participation:0|identifiers_id:0`, `_other_participation:0/relationship|code`, `composer/_identifier:0|id` (must refuse), `subject|id|id_scheme|id_namespace|name` in `party_identified`/`party_related` fixtures; ADR 0015 § Decision 5.

**Interfaces — produces:** `partySuffixes` (PARTY_IDENTIFIED/PARTY_RELATED: `|id` `|id_scheme` `|id_namespace` `|name`; nested `_identifier:N` DV_IDENTIFIER; PARTY_RELATED `/relationship` DV_CODED_TEXT) and `participationGroup` (`|function` DV_TEXT, `|mode` DV_CODED_TEXT, performer party suffixes inline, performer identifiers **inlined** as `|identifiers_id:N`/`|identifiers_issuer:N`/`|identifiers_assigner:N`/`|identifiers_type:N`). Consumed by C3's FEEDER_AUDIT details.

**Tasks:**
- [ ] **Inspect first:** read the actual `|mode` and `|function` values in `ehrbase_conformance_composition.json`. If `|mode` carries the bare `openehr` rubric (no code), vendor the small, stable `participation mode` group (code ↔ rubric) inside the package, rebuild `DvCodedText` from it, and refuse an unknown rubric loudly; record the vendored vocabulary in `deviations.md`. If it carries a code or a code+value pair, map directly. Update wire.md § REQ-140's PARTICIPATION row if the observed shape differs from the table — corpus wins (constraint 6).
- [ ] Failing round-trip tests: `_health_care_facility` (PARTY_IDENTIFIED), `context/_participation:N` (multi-instance), `_other_participation:N` on ENTRY (with relationship + inlined identifiers), nested `_identifier:N` on a standalone party.
- [ ] Composer boundary: `composer/_identifier:0|id` and `composer|id` refuse with typed errors (`// REQ-140` + ADR 0015 citation); pinned test.
- [ ] ENTRY `subject` leaf: `leafToFlat` maps PARTY_PROXY — PARTY_SELF emits nothing (the `WithTemplate` default; symmetric), PARTY_IDENTIFIED emits the party suffixes, PARTY_RELATED adds `/relationship`; decode rebuilds; `subject/_identifier:N` rides the nested form. Removes the "documented skip" for subject in `deviations.md`.
- [ ] Census pins; commits per family.

## Phase C3 — encapsulated leaves + `_feeder_audit` *(PR 2)*

**Files:** `datatypes.go` (DV_MULTIMEDIA, DV_PARSABLE, DV_INTERVAL leaf mappings), `rmattr_feeder.go` + tests; `deviations.md`.

**Read first:** corpus fixtures `ehrbase_conformance_data_types_dv_multimedia.json` (leaf suffix set: bare data + `|mediatype` `|size` `|alternatetext` `|integrity_check` `|integrity_check_algorithm` `|compression_algorithm`; `_charset`/`_language`/`_thumbnail` ride the C0 router), `…dv_parsable.json` (bare + `|formalism`), `…dv_interval_quantity…` (inspect exact spelling), `ehrbase_conformance_Element_feeder_audit.json` + `…feeder_audit_multimedia.json`; `rm.DvMultimedia` / `DvParsable` / `FeederAudit` / `FeederAuditDetails` shapes.

**Families / leaves:**
- [ ] DV_PARSABLE first-class leaf (bare value + `|formalism`); replaces the `|raw` ride for `ACTIVITY.timing` when fully captured — pin the boundary both ways.
- [ ] DV_MULTIMEDIA first-class leaf (suffix set above; `|integrity_check` base64; nested `_charset`/`_language` CODE_PHRASE and `_thumbnail` nested DV_MULTIMEDIA via the router).
- [ ] DV_INTERVAL leaf (reuse C1 `intervalSuffixes`; inspect the corpus spelling first and align wire.md § REQ-140 if needed).
- [ ] `_feeder_audit` (any LOCATABLE): `originating_system_item_id:N` / `feeder_system_item_id:N` (DV_IDENTIFIER suffix set), `originating_system_audit` / `feeder_system_audit` (FEEDER_AUDIT_DETAILS: `|system_id` `|version_id` `|time`; `/location` `/subject` `/provider` via C2 `partySuffixes`), `original_content` (DV_PARSABLE) **or** `original_content_multimedia` (DV_MULTIMEDIA) — choice by key name both directions; `other_details` refuses loudly (deferred).
- [ ] Census pins (the `_feeder_audit` family alone is 530 keys; expect most fixtures' `path not in web template` rows to collapse); commits per coherent slice.

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
