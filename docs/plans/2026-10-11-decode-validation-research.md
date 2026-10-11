---
kind: analysis
---

# Research report — decode leniency, RM validation and interval defaults

| | |
|---|---|
| Repository | `Cadasto/openehr-sdk-go` |
| Read against | `origin/main` at `f5a0f9f` (release v0.32.0) |
| Period | 2026-10-08 → 2026-10-11 |
| Prepared by | a Claude Code session, as a hand-off to an implementing agent |
| Decisions by | the maintainer (@sebastian-iancu) |
| Out of scope | PR #172 and its review. Do not act on anything from it. |

## 0. How to use this report

- **§2 is binding.** These are the maintainer's decisions. Do not re-open them in code.
- **§3 is evidence.** Every repository fact has a path and line on `origin/main` at `f5a0f9f`.
- **§5 is the plan:** work packages in a suggested order. **§9 (verified second opinion) supersedes §5 wherever they differ:** see §9.4 for the revised order and the work-package changes.
- **§6 lists open questions.** Do not settle them silently in code. Ask the maintainer, or record them as an ADR or a STRAND.
- **Repository rules apply** (`AGENTS.md`):
  - Normative text goes into `docs/specifications/` first.
  - Cite `REQ-NNN` / `PROBE-NNN` in tests and `doc.go`.
  - Update `docs/specifications/traceability.yaml` in the same change.
  - Every guard needs a test that fails when the guard is removed.
  - Never hand-edit vendored fixtures (`resources/`, `testkit/cassettes/`) or `*_gen.go` files.
  - Load `go-coding:go-coding` plus the focused skill for the diff.
  - Use `/sdd-specify` for spec, ADR and REQ work and `/sdd-deliver` for delivery.
  - Finish with `make spec-check`.

## 1. The issues and how they relate

| Issue | Type | Summary | Area |
|---|---|---|---|
| [#238](https://github.com/Cadasto/openehr-sdk-go/issues/238) | feature | Template validation should also report the RM floor's findings (STRAND-14). | `openehr/validation`; REQ-102, REQ-110, REQ-112 |
| [#242](https://github.com/Cadasto/openehr-sdk-go/issues/242) | feature | Opt-in strict decode that refuses members the RM does not define. Its scope is now widened by D5. | codecs; REQ-052, REQ-053, REQ-056, REQ-140 |
| [#248](https://github.com/Cadasto/openehr-sdk-go/issues/248) | bug | Template validation rejects an RM subtype, e.g. `DV_CODED_TEXT` where the OPT says `DV_TEXT`. | `openehr/validation`; REQ-102, REQ-110 |
| [#249](https://github.com/Cadasto/openehr-sdk-go/issues/249) | bug ×4 | **A** missing interval flags · **B** leap second · **C** indexed FLAT metadata key · **D** `ParseFileStrict` keeps `CONSTRAINT_REF`. | A: rm/canjson; B: rm; C: simplified; D: template |

How they relate:

- **#238 and #248** live in the same walker (`openehr/validation/walk_composition.go`). Doing #248 first avoids touching that file twice.
- **#242 and #249-A** are both about how strict decoding is. D1, D2 and D5 turn them into one cross-format rule.
- **#249 B, C and D** are independent of the rest and of each other.
- **Issue state:** all four were open at the time of writing, with no linked PR.

## 2. Maintainer decisions (binding)

- **D1 — Interval flag defaults (#249-A).**
  - Canonical decode reads an absent `*_unbounded` as `false`.
  - It reads an absent `*_included` as `true` on a bounded side and `false` on an unbounded side.
  - This is the rule the FLAT codec already has (`wire.md` REQ-140, *Interval boundary flags*). It now also binds canonical JSON and canonical XML; XML has the same gap (Q8).
  - Recorded on [#249 (decision)](https://github.com/Cadasto/openehr-sdk-go/issues/249#issuecomment-6064709367) and [#249 (correction)](https://github.com/Cadasto/openehr-sdk-go/issues/249#issuecomment-6107472505).
- **D2 — Included open side** (`*_included: true` beside `*_unbounded: true`). D2 covers only this explicit contradiction. An absent `*_included` is D1: a default applies and it is **never** an error in the default mode. Only the strict mode might refuse an absent flag, and that is still open (Q6).

  | Input for one side | Decoded as | Error? |
  |---|---|---|
  | bounded, `*_included` absent | `true` (closed end) | no |
  | unbounded, `*_included` absent | `false` | no |
  | bounded, `*_included: false` | `false` (open end at the bound) | no |
  | `*_unbounded: true` with `*_included: true` | as written | no decode error; RM validation reports `Lower_included_valid` / `Upper_included_valid` |

  - Canonical decode reads it as written.
  - **RM validation** reports it as a breach of BASE `Lower_included_valid` / `Upper_included_valid`.
  - It is not a decode refusal in the canonical formats.
- **D3 — Decode and validation stay separate calls.**
  - **Canonical JSON and XML** decode without a template, because they are self-describing (`_type`, `xsi:type`). To validate against a template, the template can be found from `COMPOSITION.archetype_details.template_id`.
  - **FLAT and STRUCTURED cannot decode without a template.** `UnmarshalFlat` / `UnmarshalStructured` take a Web Template (`flat_decode.go:65`, `structured.go:38`), plus a compiled OPT for some features (`WithTemplate`).
    - FLAT carries no `archetype_details`; decode rebuilds them from the Web Template (`flat_decode.go`, doc of `UnmarshalFlat`).
    - The only template hint in the payload is the root segment of every key, which must equal the Web Template tree id (`flat_decode.go:1371`).
    - The tree id is **not** the template id. It is the sanitised name of the OPT root node, or its term text in the default language (`openehr/template/webtemplate/id.go:66`, `sanitizeID`). The two often coincide: `testkit/corpus/flat-conformance/templates/conformance_ehrbase.de.v0.opt` has template id and tree id `conformance-ehrbase.de.v0`. Nothing guarantees it, though.
    - Choosing the template from the payload therefore means matching the root segment against the tree id computed for each stored template. The SDK has no such resolver today; the caller passes the template (Q10).
  - RM-only validation (`ValidateRM` and its typed wrappers) remains the entry for roots with no template.
  - Decode errors mean "no typed value can be built". Validation issues mean "a value was built, and it breaks RM rules".
- **D4 — Template validation always includes RM validation.**
  - Template-driven validation also runs the RM checks in one fused walk (§5, WP5). This resolves STRAND-14 through an ADR.
  - **No template-only opt-out:** drop point 4 of #238's proposal. #238's example for it, a partial draft, fails the template's own `required` checks anyway, so the option doesn't help that case.
- **D5 — Unmatched content on decode is ignored, in all four formats.**
  - Applies to canonical JSON, canonical XML, FLAT and STRUCTURED.
  - "Unmatched" means content that matches no RM attribute. For FLAT and STRUCTURED it means no Web Template path or suffix.
  - An **opt-in strict mode** turns unmatched content into a typed error. This is #242, widened from canonical JSON only to all four codecs.
- **D6 — Round-trip expectations.**
  - When the input carries unmatched content, or omits members that have a decided default, the encoded output is expected to differ from the input.
  - Expected output = input minus unmatched content, plus defaulted members written out, with member order ignored.
- **D7 — Validation issue paths.**
  - A container item is identified by its `archetype_node_id` when it has one, e.g. `items[at0001]/value`.
  - A bare positional form such as `items[0]/value` is used **only** when the item has no `archetype_node_id`.
  - Forms to take into account: `items[at0001, 'name']` and `items[at0001, 1]`.
  - The details (index base, AQL validity, when to add a name or a position) are still open: Q1–Q3.
- **D8 — #248.** Decide RM subtype conformance from BMM ancestry (`rminfo`), not from the hand-kept table.
- **D9 — ITS-JSON does not block this work.** Where ITS-JSON disagrees with these decisions (all four interval flags `required`, `additionalProperties: false`; §9.1), the decisions win. The maintainer considers those schema points probably wrong.
- **D10 — One interval rule everywhere.** The D1 defaults and the D2 handling apply wherever the SDK reads an openEHR interval:
  - canonical JSON and canonical XML;
  - FLAT and STRUCTURED (already REQ-140);
  - the AOM 1.4 codecs (`occurrences`, `existence`, `cardinality.interval`, ranges);
  - the OPT XML parser (`openehr/template/parse.go` `xmlInterval`, which ignores `*_included` today, and the numeric ranges in `parse_primitives.go:332-333`).

  ADL is the semantic reference: a range is closed unless an end is marked. The SDK has no ADL or ODIN text parser, so there is no ADL code path to change. This answers Q13.
- **D11 — Matched FLAT keys with no carrier stay typed refusals for now** (Q11). This covers the composer `external_ref` forms, `composer/_identifier:N`, `_instruction_details`, `_wf_definition` and `other_details`. The maintainer expects to support them later, as valid FLAT shortcut keys. Keep the refusals separate from D5's "unmatched" handling, so that later support is an addition, not a reversal.
- **D12 — Strict mode is offline-only in this scope** (Q12). REST reads through `transport` keep using the default lenient decode, and no transport or client option is added now.

## 3. Evidence

### 3.1 openEHR specifications (BASE, RM, AM)

Sources:
- `openEHR/specifications-BASE` at `0479cd0`, `-RM` at `66d3ac4`, `-AM` at `da06d63` (default branches, read 2026-10-08).
- The openehr-assistant MCP's BMM data (`type_specification_get` for `Interval`, `Point_interval`, `Proper_interval`, `DV_INTERVAL`).
- The vendored `resources/bmm/openehr_base_1.3.0.bmm.json`.

Findings:
- **BASE `Interval<T>`** (`docs/UML/classes/org.openehr.base.foundation_types.interval.adoc`): `lower_unbounded`, `upper_unbounded`, `lower_included` and `upper_included` are all `1..1` with **no default**. Invariants:
  - `Lower_included_valid`: `lower_unbounded implies not lower_included`
  - `Upper_included_valid`: the same for the upper side
  - `Limits_consistent` and `Limits_comparable`
- **`Point_interval<T>`** (`…point_interval.adoc`) redefines the four flags with defaults `*_unbounded = false` and `*_included = true`. A scan of the BMM pins (BASE 1.3.0, RM 1.2.0, AM 1.4.0, AM 2.4.0) found these to be the **only** defaults on interval flags.
- **BASE prose** (`docs/foundation_types/master05-interval.adoc:5`): where `Interval<X>` is a feature's type, a `Point_interval` or a `Proper_interval` is attached at run time. `Proper_interval` adds only `Inv_not_point`. So the `Point_interval` defaults are an analogy for `DV_INTERVAL`, not a rule for it.
- **RM `DV_INTERVAL`** (`docs/UML/classes/org.openehr.rm.data_types.dv_interval.adoc`) inherits `DATA_VALUE` and `Interval`, and adds only `Limits_consistent`. It declares no defaults.
- **AM / ADL** writes ranges as closed unless an end is marked. Example: `|0..<1000|` means `0 <= x < 1000` (`docs/ADL2/master01-preface.adoc` *Non-inclusive two-sided intervals*; `docs/ADL2/master04.5-cadl_primitive_types.adoc:302-316`).
- **No BASE, RM or AM text says how a serialisation treats an absent mandatory flag.** D1 is therefore an SDK decision backed by the points above. ITS-JSON was deliberately not consulted; the maintainer judged it unnecessary.
- **Paths** (`docs/architecture_overview/master11-paths.adoc`):
  - `[atNNNN]` is shorthand for `[@archetype_node_id='atNNNN']` (:74).
  - `[at0001 and name/value='x']` (:89), with the shortcut `[at0001, 'x']` (:95).
  - Positional XPath predicates like `events[1]` are **1-based**, and the chapter calls them "the only guaranteed unique paths" (:368-378).

### 3.2 Intervals in the SDK

- **The flags are plain `bool`** (`openehr/rm/foundation_types_interval_gen.go:38,44`), so after decode an absent flag and `false` look the same.
- **The interval codecs are generated:** `foundation_types_interval_{jsonunmar,jsonmar,xmlunmar,xmlmar}_gen.go`, produced by `internal/bmmgen` (ADR 0002). `interval_bound_gen.go` is an encoder helper for empty bounds, not a codec. For the JSON path, see §9. Changes go through the generator, or a deliberate hand-written override, followed by `make codegen-verify`.
- **The FLAT rule already exists** in `docs/specifications/wire.md:275` (REQ-140 *Interval boundary flags*). It covers the defaults, refusing an included open side at FLAT decode and encode, and a corpus note.
- **Known floor gap:** `docs/specifications/clinical-modeling.md:965` (*Known gap — an included open side*).
- **There is already a plan for the floor check.** `docs/plans/2026-09-30-validation-false-passes.md` (status Draft, lines 24 and 33) plans the floor check for an included open side. Coordinate with it rather than duplicating it.
- **Corpus** (all `*.json` under `testkit/`):
  - 39 `DV_INTERVAL` values in 9 files.
  - **25 have both bounds and no `*_included` members:**

    | File | Intervals |
    |---|---|
    | `testkit/corpus/compositions/Demonstration.v1.json` | 15 |
    | `testkit/corpus/compositions/TestPerson.v2.json` | 7 |
    | `testkit/corpus/crossformat/test_all_types/canonical.json` | 3 |

  - The last file is upstream EHRbase openEHR_SDK test data (`testkit/corpus/crossformat/MANIFEST.txt:38`), so another producer does omit these flags.
  - 14 intervals carry all four flags.
  - **On `main` the 25 decode as excluding both bounds,** which is a silent change of meaning.

### 3.3 Validation: the template walker and the RM floor

- **Two independent walkers.**
  - The template-driven `walker` lives in `openehr/validation/walk_composition.go`. The RM floor `rmFloorWalker` lives in `openehr/validation/rmfloor.go:148`.
  - The package doc says the two "compose but do not chain" (`openehr/validation/doc.go`).
  - STRAND-14 is Open (`docs/specifications/research-strands.md:265`). Its resolution form is an ADR, and the strand forbids settling the answer in code before that ADR.
- **They share the same readers.** Both use `openehr/rm/rminfo` and `openehr/validation/rmread`; see the imports of `walk_composition.go`.
- **What the template walker visits:**
  - Attributes the OPT declares, plus the RM-**mandatory** attributes the compiler injects (`internal/templatecompile/compile.go:239-282`). RM-optional attributes are not injected, e.g. `ELEMENT.null_flavour`, `LOCATABLE.uid` and `feeder_audit`.
  - At each node it checks the node id, the RM type, presence, and the OPT's existence, cardinality and occurrences.
  - It **never evaluates the RM per-type invariants.**
- **Where the template walker stops, so nothing below is checked:**
  - **Slot leaves** (`:68`). `IsSlot()` is true only for an unfilled `ARCHETYPE_SLOT` (`internal/templatecompile/node.go:118-120`).
  - **Primitive-constraint leaves** (`:79`; the code comment forbids descending further).
  - **Attributes with no OPT children** (`:129`, `:285`), which count as open.
  - **Items that bind to no OPT child**: an issue is emitted and there is no descent (`:299`).
- **The floor** walks every BMM attribute through `rminfo`. It runs the required-set check and the REQ-112 invariant catalogue (`clinical-modeling.md:884…`).
  - Node-level checks are `checkInvariants(value, rmType, path)` (`rmfloor.go:302`) and `checkCodedInvariants`. The coded rules are keyed by the owning class (`rmfloor_coded.go:24-56`).
- **Parent-to-child state in the floor.** The DV_SCALE blank-code exemption lives in a map keyed by the floor's own path strings (`rmfloor.go:151-154`, `:330`, `:426-437`). A fused walk must carry it down the recursion instead.
- **Path styles today:**

  | Producer | Container item segment | Where |
  |---|---|---|
  | Floor | `[i]`, 0-based | `rmfloor.go:251` |
  | Template walker | `[archetype_node_id]`, else `[@n]` (1-based) | `walk_composition.go:470-500` |
  | Template walker, missing OPT child | OPT node id or archetype id | `segmentForChild` |

  `Issue.Path` is documented as "the AQL path of the offending node" (`openehr/validation/issue.go:72`).
- **Subtype check (#248):**
  - `rmTypeIsSubtypeOf` looks up the hand-kept `bmmSubtypes` table (`walk_composition.go:722`, `:845`), which has rows only for abstract types. It is called at `:201` (when binding alternatives) and `:702` (`checkRMType`).
  - `rminfo.Default` already implements `rminfo.Hierarchy`, with `Ancestors`, `ConformsTo` and `ConcreteDescendants` (`openehr/rm/rminfo/hierarchy.go:67-201`).
  - `TestDataValueSubtypesMatchBMM` (`openehr/validation/data_value_subtypes_test.go:97`) already compares the `DATA_VALUE` row with it.
  - The composition generator's comments describe "lockstep admission" with the validator's table (`openehr/instance/generate.go:1324-1332`), so generator output must stay valid.
- **Timings, quoted from #238 and not re-measured:** BMI decode 106 µs, template pass 16 µs, floor 66 µs. Demonstration.v1: 1,837 µs, 147 µs and 507 µs.

### 3.4 Unmatched content today, per codec

| Codec | Today | Evidence |
|---|---|---|
| Canonical JSON (`canjson`) | **ignored** | A stray member decodes cleanly (checked in-session on an earlier commit; #242 reports the same at v0.31.0). `typereg` switches `RejectUnknownMembers` off for the `_type` look-ahead (`openehr/rm/typereg/streaming.go:412-424`). `canjson.Unmarshal` takes no options. `NewDecoder` has only `WithRelaxedTypeDispatch` (`canjson/decode.go:94`). |
| Canonical XML (`canxml`) | **ignored** | Generated `UnmarshalXML` methods "skip unrecognised elements" (`canxml/errors.go:28-36`). `ErrNamespace` is exported but unused, kept "for a future strict-namespace decode path". `Hash` attributes are skipped (`canxml/doc.go:103`). Option: `WithRelaxedTypeDispatch` (`canxml/decode.go:46`). |
| FLAT | **refused** (`ErrUnknownPath`) | `simplified/simplified.go:61`; `flat_decode.go:161`, `:187`; unknown `ctx/` fields at `:527`. Normative: `wire.md:271`, "REQ-053's fail-loud posture … never a silent drop". Option: `WithTemplate` (`simplified.go:32`). |
| STRUCTURED | **refused** (`ErrUnknownPath`) | `simplified/structured.go:110-130` |
| Definition/System responses (v1 codec) | kept in `Extras` | `wire.md:451` (REQ-144). These aren't RM content, so they are not in scope. |

- **No spec text covers canonical JSON's current behaviour.** `wire.md` has no rule on unknown members for canonical JSON; it only covers exact member names (`:113`) and duplicate members (`:131`).
- **D5 is a reversal for FLAT and STRUCTURED.** The default flips from fail-loud to ignore. Test files reference `ErrUnknownPath` 85 times across 15 files, and the PROBE-086 / PROBE-105 counts will move.

### 3.5 Round-trip tests compared with D6

- **PROBE-030** (`testkit/probes/serialize/probe_030_canjson_round_trip.go`) and **`TestRoundTripFixtures`** (`openehr/serialize/canjson/roundtrip_test.go:154`) compare decoded A with B and encode b1 with b2. They **never compare against the input**. The 25 intervals in §3.2 are not caught as a failure. PROBE-105 records them as a waived difference instead: `testkit/conformance/crossformat/recorded.go:364-366` says "canjson reads the absence as false … (6 keys)". WP3 must delete that record and regenerate `CENSUS.md`.
- **`TestRoundTripStructuralEquivalence`** (`roundtrip_test.go:113`) compares the normalised input with the output. It is the natural place for D6's expected-output rule.
- **PROBE-038** (`testkit/probes/serialize/probe_038_canjson_rm_polymorphic_decode.go:80`, `:138`) counts every `_type` in the **input**, including inside unmatched members. An input whose extension object carries a `_type` would fail it, even though the SDK behaves as D5 requires.

### 3.6 Validation paths compared with the AQL grammar

- The vendored AQL grammar defines `nodePredicate` in `resources/aql/grammar/active/AqlParser.g4`:
  - `(ID_CODE | AT_CODE) (, STRING | PARAMETER | TERM_CODE | AT_CODE | ID_CODE)?`
  - `ARCHETYPE_HRID (, …)?`
  - comparisons, and `AND` / `OR`
- **Neither `[at0001, 1]` nor a bare `[1]` is valid AQL.** The grammar allows no integer after the comma and has no positional predicate. `[at0001, 'name']` is valid AQL.
- `openehr/aql` already models `[at0001, 'x']` and `[at0001 and name/value=…]` predicates (`openehr/aql/predicate_model.go`, `openehr/aql/path.go`). It is a candidate for rendering issue paths.

### 3.7 #249 B, C and D (not changed by this research)

- **B:** `DVTime.ToTime` / `DVDateTime.ToTime` pass second 60 straight to `time.Date`, which normalises it into the next minute (`openehr/rm/temporal_funcs.go`, around line 218). Spec: REQ-123. Choosing between an error and clamping is still open (Q9).
- **C:** the indexed key `<root>/language:0|code` is dropped in favour of `ctx/language` (`simplified/flat_decode.go`, the metadata alias table around lines 266-311). The harness has the same blind spot (`testkit/conformance/webtemplate/case.go:167`, `IsCompositionMeta`).
  - This is **matched** content: it maps to `COMPOSITION.language`. D5 therefore does not apply. The key must be aliased and checked for conflicts with `ctx/`, not ignored.
- **D:** `template.ParseFileStrict` keeps a `CONSTRAINT_REF` node and drops its constraint (`openehr/template/parse.go:419`, per `docs/backlog.md:30`). The strict-mode rule in `clinical-modeling.md` has to be widened first.
  - "Strict" here means template parsing and is unrelated to D5's strict decode. Keep the names distinct.

## 4. Corrections made during the research

- **The #249 decision comment** said canonical JSON refuses an included open side at decode. That refusal exists only in FLAT. It was corrected [on the issue](https://github.com/Cadasto/openehr-sdk-go/issues/249#issuecomment-6107472505), and D2 records the decision.
- **"A fused walk gives one path style by itself"** is false. Path rendering has to be designed; see D7 and WP6.
- **"The floor's checks can be called as they are"** was overstated, because of the DV_SCALE exemption state (§3.3).
- **"An RM-driven walk cannot detect missing nodes"** was wrong. The template walker is still the preferred driver, because it already binds data to OPT alternatives, slots and occurrences.

## 5. Plan

Original order: WP1 → (WP2 + WP3) → WP4 → (WP6 + WP5) → WP7. WP0 can run at any time. **Superseded by §9.4.**

### WP0 — Issue housekeeping (needs the maintainer's approval for each action)
- **#249:** split into four issues (A–D) so each can close on its own. Otherwise reference it with `Refs #249`, not `Closes #249`.
- **#242:** update the title and body to the cross-format scope of D5. The existing [pointer comment](https://github.com/Cadasto/openehr-sdk-go/issues/242#issuecomment-6064710119) predates D5.

### WP1 — #248: subtype conformance from BMM ancestry (D8)
- **Spec:** probably none. Check whether REQ-102 or REQ-110 text names the table.
- **Code:**
  - Replace the body of `rmTypeIsSubtypeOf` (`walk_composition.go:722`) with `rminfo` `Hierarchy.ConformsTo`.
  - Strip generic parameters with `bmmtype.Class`.
  - Keep `intervalRMTypeMatches`.
  - Then remove `bmmSubtypes`, or reduce it to whatever is still used. `TestDataValueSubtypesMatchBMM` reads it.
  - Check that the generator's "lockstep" assumption (`openehr/instance/generate.go:1324-1332`) still holds.
- **Tests:**
  - The issue's repro: the BMI OPT with `DV_CODED_TEXT` under a `DV_TEXT` ELEMENT `value`, and the same for `name`.
  - `PARTY_REF` under `OBJECT_REF`.
  - The corpus sweep over 46 OPTs described in #248.
  - A can-fail test: restoring the table turns the new tests red.
- **Done when:** none of the cases in #248 reports `rm_type_mismatch`, and every composition the generator produces still validates clean.

### WP2 — Round-trip expectations (D6), delivered together with WP3
- **Spec:** state the normalisation once, in `conformance.md` or `wire.md` REQ-052, and cite it from PROBE-030 and PROBE-038.
- **Code:**
  - Add a `testkit` helper that builds the expected output from an input: drop members that are not attributes of the object's `_type` (via `rminfo`), and write out members that have a decided default (D1 flags).
  - Restrict PROBE-038's `_type` census to members the RM defines.
- **Tests:**
  - The corpus round trip compares the normalised input with the output, using `testkit/wireequiv`.
  - Can-fail: reverting the WP3 decode fix turns the 25 intervals red.
- **Note:** the new check fails on `main` until WP3 lands. Deliver WP2 and WP3 in one change. Never skip or quarantine a test.

### WP3 — #249-A: interval flag defaults (D1, D2)
- **Spec:**
  - Write the default rule **once** in `wire.md`, cited from REQ-052 (canonical JSON), REQ-056 (canonical XML; Q8 confirms the gap) and REQ-140, instead of restating it.
  - Remove any wording that makes an included open side a canonical **decode** refusal.
  - REQ-112 gains `Lower_included_valid` / `Upper_included_valid`, coordinated with `docs/plans/2026-09-30-validation-false-passes.md`.
- **Code:**
  - Make the canonical JSON decode of the four flags presence-aware. That means a generator change in `internal/bmmgen` (ADR 0002 rules apply) or a deliberate hand-written override, then `make codegen-verify`.
  - Do the same for canonical XML (Q8 confirms the gap) and for the AOM 1.4 codecs (§9).
  - Add the floor check for an included open side.
- **Tests:**
  - The 25 corpus intervals decode as closed.
  - An absent flag beside an unbounded side reads as `false`.
  - Explicit `included: true` beside `unbounded: true` decodes, and the floor reports it.
  - A can-fail test for each guard.

### WP4 — #242 widened: unmatched content and strict mode (D5)
- **Spec:**
  - Write one shared rule, "Unmatched content on decode", in `wire.md`, cited from REQ-052, REQ-056, REQ-053 and REQ-140.
  - Amend the fail-loud sentence at `wire.md:271` and `simplified/deviations.md`.
  - Update the conformance and probe text for PROBE-086 and PROBE-105.
  - Define "unmatched" for each format:
    - canonical JSON: a member name that is not an attribute of the decoded type
    - canonical XML: an element or attribute with no matching RM attribute, or a foreign namespace
    - FLAT and STRUCTURED: a path or suffix the Web Template does not resolve, an unknown `_`-segment, an unknown `ctx/` field
- **API:** add a strict option to each codec:
  - `canjson`: an options form of `Unmarshal` (or `UnmarshalStrict`) plus a `DecoderOption`
  - `canxml`: a `DecoderOption` (there is room to use `ErrNamespace`)
  - `simplified`: an `Option`
- **Errors:** a classified sentinel per codec, or one shared sentinel, carrying the path and the member or key name. Callers must be able to reach it with `errors.Is` / `errors.As`. Keep payload values out of messages.
- **Suggested split:**
  1. canonical JSON and XML. These are already lenient, so the work is adding strict mode.
  2. FLAT and STRUCTURED. The default flips, so the 85 `ErrUnknownPath` test references and the probe counts move.
- **Open:** Q4, Q5, Q6.

### WP5 — #238: one fused validation walk (D3, D4)
- **ADR first:** resolve STRAND-14 as chained by default, one walk, no template-only option. Then amend REQ-102, REQ-110 and REQ-112.
- **Design:**
  - The template walker drives.
  - At every RM node it binds to an OPT node, it also runs the floor's node checks.
  - It hands to a floor sub-walk every part it does not visit: open attributes, unfilled slots, the internals of primitive-constraint leaves, RM-optional attributes the OPT does not mention, and unbound or mismatched items (walked by their runtime type).
  - The template walker owns presence checks on attributes it models.
  - An explicit **equivalence table** between template issue codes and floor issue codes ensures a defect found by both is reported once. Remove duplicates per node during the visit; never by comparing path strings with indices stripped, because that merges siblings.
  - Carry the DV_SCALE exemption down the recursion, with a can-fail test.
  - Paths follow WP6.
- **Acceptance (from #238, minus the dropped option):**
  - The nine one-defect copies of BMI and the three corpus compositions are reported as not OK, with the floor's code at the node's path.
  - The seven duplicates in `clinical_notes.v0` appear once.
  - Generator output stays OK.
  - Benchmark before and after.

### WP6 — Path rendering (D7), part of or just before WP5
- **One path builder shared by both walkers.** This changes `ValidateRM` paths too, which is a behaviour change to note in the CHANGELOG at release.
- **Rule:**
  - Use the `archetype_node_id` predicate when the item has one.
  - Disambiguate siblings that share an id, by name or by position (Q2, Q3).
  - Use position only when the item has no id.
- **Rendering:** consider reusing `openehr/aql`'s predicate model so paths stay parseable where the chosen forms allow.

### WP7 — #249 B, C and D, each on its own
- **B:** decide between an error and clamping (Q9), then document it in REQ-123.
- **C:** alias the indexed metadata keys and refuse conflicts with `ctx/`. Fix the harness's `IsCompositionMeta` (the PROBE-086 counts move).
- **D:** widen the `ParseOPTStrict` rule in `clinical-modeling.md`, then refuse or model `CONSTRAINT_REF`.

## 6. Open questions (ask the maintainer; do not settle them in code)

1. **Q1 — Index base.** BASE positional predicates are 1-based XPath. The floor writes `[0]` (0-based) and the template walker writes `[@1]`. The maintainer's example used `[0]`. Pick one.
2. **Q2 — `items[at0001, 1]`.** Neither this nor a bare `[1]` is valid in the vendored AQL grammar, and the BASE paths chapter shows position only as a bare `[1]`. `Issue.Path` is documented as an AQL path. Options:
   - (a) use only AQL-valid predicates and accept that position cannot be expressed in AQL;
   - (b) adopt `[at0001, n]` as an SDK diagnostic convention and change the `Issue.Path` doc;
   - (c) render `[at0001 and name/value='…']` when names are unique and fall back to (b).
3. **Q3 — When to add a name or a position.** Always, or only when siblings share an `archetype_node_id`?
4. **Q4 — Unknown `_type` value** (a class the RM does not define). Recommendation: keep it an error. It is a dispatch failure, not unmatched content, and ignoring it would drop a whole subtree.
5. **Q5 — Should lenient decode report what it dropped?** For example, a non-fatal list or a callback. Recommendation: yes, optionally, especially for FLAT and STRUCTURED, where a dropped key can hold clinical data. A read–modify–write cycle also drops vendor extensions; that should be documented.
6. **Q6 — Should strict mode also refuse absent mandatory members** (e.g. the interval flags) in all four formats? The #242 pointer comment suggested this for canonical JSON.
7. **Q7 — FLAT and an included open side.** REQ-140 refuses it at FLAT decode. Given D2 and D3, should FLAT decode also defer to validation? Refusing at encode would stay either way.
8. **Q8 — Canonical XML. Answered: yes, the same gap.** `Interval[T].UnmarshalXML` leaves an absent flag at Go `false` (`openehr/rm/foundation_types_interval_xmlunmar_gen.go:30-62`). The ITS-XML XSD makes `lower_included` / `upper_included` `minOccurs="0"` and the `*_unbounded` pair mandatory (`openEHR/specifications-ITS-XML` `components/BASE/Release-1.2.0/BaseTypes.xsd`, type `Interval`). So valid XML may omit them, and D1 must cover XML.
9. **Q9 — #249-B.** Return an error (`rm.ErrTemporalConversion`) or clamp to `23:59:59.999999999`?
10. **Q10 — Template resolution for FLAT and STRUCTURED.** Keep requiring the caller to pass the Web Template, or add an optional resolver keyed by the root segment's tree id (for example `WithTemplateResolver(func(treeID string) (*webtemplate.WebTemplate, error))`)? The vendored REST pin (`resources/its-rest/ehr-validation.openapi.yaml`) defines no template-id parameter for FLAT or STRUCTURED payloads. Resolving the template is left to the server.

## 7. Sources and method

- **Code:** read on `origin/main` (`f5a0f9f`) with `git show` / `git grep`, read-only. The local checkout is a detached older commit, so always read `origin/main`.
- **GitHub:** issues #238, #242, #248 and #249, plus their comments, through the GitHub MCP.
- **openehr-assistant MCP:** BMM type specifications for `Interval`, `Point_interval`, `Proper_interval` and `DV_INTERVAL`; the guides `howto/spec-lookup`, `specs/base-foundation_types`, `specs/rm-data_types`, `specs/lang-bmm` and `specs/lang-bmm_persistence`.
- **openEHR specifications:** `specifications.openehr.org` is blocked by this environment's egress policy (proxy 403). The public GitHub spec repositories were cloned read-only instead (commit SHAs in §3.1).
- **Not done:**
  - No runtime reproduction of the issues; a proposal to add a repro test file was declined.
  - No benchmarks.
  - ITS-JSON not read.
  - The provenance of `Demonstration.v1.json` and `TestPerson.v2.json` not established.

## 8. What is recorded on GitHub

- [#249 — decision on item A](https://github.com/Cadasto/openehr-sdk-go/issues/249#issuecomment-6064709367)
- [#249 — correction, with the corpus evidence](https://github.com/Cadasto/openehr-sdk-go/issues/249#issuecomment-6107472505)
- [#242 — pointer to the #249 decision](https://github.com/Cadasto/openehr-sdk-go/issues/242#issuecomment-6064710119). Its scope predates D5; update it in WP0.

## 9. Second opinion (independent reviewer, verified)

An independent read-only reviewer (a separate model session) checked this report against `origin/main`. Every item below was **re-verified by me** unless it is marked *(reviewer, not re-verified)*. All `path:line` references are on `origin/main` at `f5a0f9f`.

### 9.1 Corrections to the report
- **PROBE-105 already records the interval defect** as a waived outcome. §3.5 is updated.
- **Q8 is answered: canonical XML has the same gap.** §6 is updated.
- **WP1 is not spec-free.**
  - REQ-110 names `bmmSubtypes` by identifier (`docs/specifications/clinical-modeling.md:726`), so this is a full-lane change.
  - `bmmSubtypes` also holds **AOM 1.4 primitive short-name rows**, e.g. `DURATION` → `DV_DURATION` and `DATE_TIME` → `DV_DATE_TIME` (`openehr/validation/walk_composition.go`, the end of `bmmSubtypes`).
  - `rminfo` `ConformsTo` answers `known=false` unless both names are BMM classes (`openehr/rm/rminfo/hierarchy.go`, `ConformsTo`).
  - So keep those rows, or map them through the template compiler's short-name handling. Do not drop the table wholesale.
- **Where the JSON change goes.**
  - `Interval[T]` has no `UnmarshalJSONFrom` of its own. `DV_INTERVAL` decodes its flags through `rawInterval[T]` embedded in `jsonWireDVInterval[T]` (`openehr/rm/data_types_quantity_jsonunmar_gen.go:40-44`).
  - The generator site is `internal/bmmgen/render_jsonunmar.go:113-148` (`shapeIntervalDerived` / `shapeIntervalBase`).
- **The same `rm.Interval[T]` backs AOM 1.4** `occurrences`, `existence` and `cardinality.interval` (`openehr/aom/aom14/archetype_constraint_model_gen.go:254,490,572`). D1 therefore reaches the AOM codecs, and the spec must say so (Q13).
- **The floor does not walk "every BMM attribute".** It stops at classes `rmread` does not model (`clinical-modeling.md` REQ-112, *Known gap — classes rmread does not model*). Any design that relies on a floor sub-walk inherits those gaps.
- **ITS-JSON, for the record.** It was not consulted for D1, at the maintainer's direction. `Interval.json` and `DV_INTERVAL.json` (ITS-JSON Release-1.1.0) list all four flags as `required`, with `additionalProperties: false`.
  - ITS-JSON and ITS-XML therefore disagree on absent `*_included`. D1 deliberately follows the lenient reading.
  - The JSON schema is a ready spec anchor for the **strict** mode in D5.
  - **Maintainer ruling (D9):** these ITS-JSON points do not override the decisions.

### 9.2 Impact the report missed
1. **D5 removes the mechanism the conformance harnesses measure with.**
   - The PROBE-086/105 harnesses reduce a body by its **refusals**. `conformance.md:620` requires "the PROBE-086 refusal-derived exclusion" for every FLAT and STRUCTURED decode.
   - With ignore-by-default there are no refusals to count. The harnesses must run in strict mode, or use a "report what was dropped" option (Q5), **before** the FLAT default flips.
2. **Some FLAT keys are matched but have no carrier, and are refused on purpose.** They are not "unmatched":
   - composer `external_ref` short forms and `composer/_identifier:N`
   - `_instruction_details`, `_wf_definition`, FEEDER_AUDIT_DETAILS `other_details`

   Sources: `openehr/serialize/simplified/deviations.md:93,106`; `wire.md:281`, "Refused on decode … never silently dropped"; ADR 0015 *(reviewer, not re-verified)*. Read literally, D5 would turn these into silent data loss (Q11). In XML the same class exists: `Hash` elements are skipped (`canxml/doc.go:103`).
3. **Versioning.**
   - A tightened validation is a breaking change (`docs/specifications/module-layout.md`, versioning table: "an input that passed now fails → major").
   - Before 1.0, a breaking change ships as a **minor** bump, with every break listed in the release notes (`docs/releases.md:34`).
   - The CHANGELOG keeps `### Added` only before 1.0 (`AGENTS.md`). Do not add `### Changed` or `### Fixed`.
   - Breaking changes in this plan:
     - D4 (WP5) and the new floor rule (WP3) make inputs that passed fail.
     - D7 changes `Issue.Path` for consumers that parse it.
     - D5 makes FLAT and STRUCTURED more permissive.
4. **Strict mode cannot be reached from REST reads today.** `transport.Decode[T]` calls `canjson.Unmarshal` with no options (`transport/client.go:691`). Several clients also call `canjson.Unmarshal` directly *(reviewer: `openehr/client/ehr/...`; not re-verified one by one)*. Strict mode is either offline-only, or needs a transport/client option (Q12).
5. **canxml has no channel for options into generated code.**
   - `WithRelaxedTypeDispatch` is documented as a no-op for that reason (`openehr/serialize/canxml/decode.go:40-55`).
   - Strict XML needs a generator change to the `default: _dec.Skip()` arm (seen in `foundation_types_interval_xmlunmar_gen.go`) plus a way to pass the policy in (ADR 0002 territory).
6. **Three interval parsers, two rules.**
   - The OPT numeric-range parser already reads an absent `*_included` as inclusive (`openehr/template/parse_primitives.go:332-333`). It does so even on an unbounded side.
   - The OPT `xmlInterval` used for occurrences, existence and cardinality ignores `*_included` altogether (`openehr/template/parse.go:316-321`).
   - Write the rule once and point all three at it (Q13).
7. **No corpus interval has an open side.** 0 of 39 carry `*_unbounded: true`. The open-side default (D1) and the floor rule (D2) need synthetic fixtures.
8. **Validation probes pin issue codes** *(reviewer, not re-verified)*: PROBE-025, PROBE-074, PROBE-027 and PROBE-076. D4 and the new floor rule mean re-baselining them, as a reviewed change, never by skipping.
9. **Go zero value.** `rm.Interval[T]{Lower: a, Upper: b}` built in Go is exclusive on both ends. D1 fixes the wire, not values built in code (Q16).

### 9.3 Design suggestions accepted into the plan
- **WP5: chain, then fuse only if benchmarks justify it.** I now prefer this to the fused walk I proposed in §5.
  - D4's contract is: one call, floor findings included, each defect reported once.
  - Chaining meets it this way: run the template walk, then the floor walk, then remove duplicates on the **exact** `(path, code)` pair using the equivalence table.
  - Exact matching is only safe once both walkers share one path builder (WP6) that gives every node a unique path. That is why WP6 must come first.
  - Chaining avoids threading DV_SCALE state through a shared recursion and moving the floor's checks into the template walker.
  - The ADR should bind the **contract**, not the mechanism (Q15).
- **canjson strict mode is cheap.**
  - `encoding/json/v2` `RejectUnknownMembers(true)` is the mechanism. `typereg` already exempts its `_type` look-ahead (`typereg/streaming.go:412-424`).
  - Add `opts ...DecoderOption` to `canjson.Unmarshal`, reusing the existing option type.
  - Classify the unknown-name error into its own sentinel next to `ErrInvalidShape` (housed in `typereg`), rather than widening `ErrInvalidShape`.
- **Q5 and strict mode share one mechanism:** an option that reports every unmatched path, where strict mode refuses at the first one. The same option keeps the harness census working under D5.
- **Proposed answer to Q6: no.** Strict mode covers unmatched content only. An absent mandatory flag still builds a value, so presence is a validation question (D3).
- **Proposed answer to Q1–Q3:** use BASE's own path forms.
  - `[atNNNN]` by default.
  - `[atNNNN, 'name']` only when siblings share an id and their names differ.
  - A 1-based `[n]` otherwise.
  - Re-document `Issue.Path` as an openEHR path that is AQL-valid when no position is needed. The vendored AQL grammar has no positional predicate.
- **The D6 helper needs declared types.** For objects whose `_type` is left out, the expected-output builder needs the parent attribute's declared type from `rminfo`.

### 9.4 Revised plan
- **Order:** WP1 → (WP2 + WP3) → WP4a → WP4b → WP4c → WP7-C → WP4d → WP6 → WP5 → the rest of WP7.
- **WP1:** full lane. Amend the REQ-110 text at `clinical-modeling.md:726`. Keep or re-route the AOM short-name rows. Can-fail test: going back to the table turns `DV_CODED_TEXT` under `DV_TEXT` red.
- **WP2 + WP3:**
  - Scope covers canonical JSON, canonical XML and the AOM 1.4 interval codecs (D10).
  - OPT parser (D10):
    - Give `xmlInterval` (`openehr/template/parse.go:316-321`) the two `*_included` fields with the D1 defaults, and carry them into `Multiplicity` (`intervalToMultiplicity`, which today models every occurrence, existence and cardinality interval as closed).
    - Make the numeric-range parser (`parse_primitives.go:332-333`) default an absent `*_included` to `not *_unbounded`. Today it defaults to inclusive even on an unbounded side.
  - Vendored OPTs spell all four flags on `<occurrences>` (e.g. `testkit/corpus/templates/BMI.opt:44-50`), so expect little corpus churn here. Use synthetic OPT fixtures for excluded or omitted ends.
  - **Spec note, do not act on it:** BASE `Multiplicity_interval` inherits `Proper_interval`, whose `Inv_not_point` requires `lower /= upper`. Yet `Multiplicity_interval.is_mandatory` is defined as `1..1`, which is a point (`specifications-BASE` `docs/UML/classes/org.openehr.base.foundation_types.multiplicity_interval.adoc`). Do not let that inconsistency drive code.
  - Delete the PROBE-105 recorded outcome and regenerate `CENSUS.md`.
  - Add synthetic fixtures for open sides.
  - Write the rule once in `wire.md`, cited from REQ-052, REQ-056, REQ-140 and the AOM codec REQ. Put the XSD `minOccurs` evidence in REQ-056.
  - List the new floor rule as a break in the release notes.
- **WP4** is split into four:
  - **4a** canjson strict, using the json/v2 option.
  - **4b** canxml strict: a generator change plus a policy channel. It also unblocks relaxed dispatch.
  - **4c** switch the PROBE-086/105 harnesses to strict mode or a dropped-content report.
  - **4d** flip the FLAT and STRUCTURED default. Only after 4c, after Q11 is answered, and after **WP7-C**; otherwise the indexed metadata keys become a silent drop of matched content.
- **WP5:** the ADR binds the contract. Implement chain + dedupe, benchmark both mechanisms, re-baseline the validation probes, and list the break in the release notes.
- **WP6** comes **before** WP5. The floor's `blankCodeAllowed` map is keyed on the floor's own path strings (`rmfloor.go:151-154`), so the exemption moves together with the path builder.

### 9.5 New open questions
11. **Q11 — answered by D11 (refusals stay; future support as shortcut keys).** Under D5, do FLAT keys that resolve but have no carrier stay typed refusals? These are the composer `external_ref` forms, `composer/_identifier:N`, `_instruction_details`, `_wf_definition` and `other_details`. Recommendation: yes, because they are matched content.
12. **Q12 — answered by D12 (offline-only for now).** Is strict mode offline-only, or also offered through `transport` and the client options?
13. **Q13 — answered by D10 (yes).** Does D1 also bind the AOM 1.4 codecs and the OPT parser's occurrences, existence and cardinality intervals?
14. **Q14:** In canxml strict mode, refuse or skip `Hash<K,V>` elements (matched but unsupported)?
15. **Q15:** Does the D4 ADR bind the fused walk as the mechanism, or only the contract? Recommendation: the contract.
16. **Q16:** Add a closed-interval constructor, so intervals built in Go do not repeat the wire's silent change of meaning?

### 9.6 Not re-verified by me
- ADR 0015's exact wording.
- The individual client call sites beyond `transport/client.go:691`.
- The issue-code pins in PROBE-025, -027, -074 and -076.
- The claim that FLAT `WithTemplate` synthesises RM-mandatory defaults.
- The reviewer's statements on `DecodePolymorphic` option propagation.
