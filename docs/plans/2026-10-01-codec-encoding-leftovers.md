---
kind: plan
---

# Plan — Codec and RM encoding leftovers from PRs 190 to 193

**Date:** 2026-10-01
**Status:** Done — phases 1 to 4 implemented; decisions 1 and 2 took the recommended option, and decision 3 took the ADR route (ADR 0002 D4 amended, mapping row reworded)
**Covers:** REQ-053 and REQ-140 ([wire.md § REQ-053](../specifications/wire.md#req-053), [§ REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes)), REQ-052 and REQ-056 ([wire.md § REQ-052](../specifications/wire.md#req-052)), REQ-121 ([rm-functions.md](../specifications/rm-functions.md)), REQ-043 ([bmm-conformance.md](../specifications/bmm-conformance.md)); interacts with REQ-112 ([clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor))
**Probes:** PROBE-086 (census, must not move in phases 1 and 2), PROBE-030 and PROBE-033 (canonical JSON and XML round trip)
**Depends on:** nothing; the flat-decode fidelity work (PR 193) is merged
**Defers:** Web Template spelling of a slot-filled CLUSTER leaf, the in-context `ism_transition`, and demographic `rmpath` navigation (see Deferred)

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

Close the review suggestions that PRs 190 to 193 left behind in the FLAT codec, the canonical interval encoders and the BMM generator, so no later plan has to rediscover them. Nothing here blocks a release. Fifteen items were re-verified on main; all fifteen are still true, and nothing was found already fixed. The work is mostly tests and wording, with one real encoder gap (AOM 1.4 intervals).

## Evidence

Each line was reproduced or mutation-checked with `go test -overlay`, not read off the old review.

- `codedToFlat` writes `|code` and `|value` as blank keys for a zero `DV_CODED_TEXT`; only a nil pointer is skipped.
- A scalar already placed at a single-valued attribute is overwritten by the placement walk in `flat_decode.go` without an error.
- `string_leaf.go`, its test and `openehr/serialize/simplified/deviations.md` still say a STRING leaf on any attribute but `action_archetype_id` is refused; `emitStringLeaf` in fact accepts `INSTRUCTION_DETAILS.activity_id`, because `rmpath` resolves it. A whole encode is still stopped earlier by the `_instruction_details` refusal.
- Removing the STRING arm of `isValueLeafType` leaves the package green.
- `TestWebTemplatePathsResolveViaRmpath` run for one OPT fails on its stale-entry checks, and an OPT the parser refuses (today `social.opt`) is a silent skip.
- Removing `PointInterval`'s lower-side omission arm, or `ProperInterval`'s both-sides-open arm, from the generated JSON marshaller leaves every test green. The same removal on `DVInterval` is caught, so the harness works and only the cases are missing. The generator's own matching switch is unpinned too.
- An AOM 1.4 `occurrences` interval with an open upper side writes `"upper":0` beside `upper_unbounded: true` in JSON, and canonical XML writes Go-default PascalCase element names for it, because the abstract `Interval[T]` has no encoder of its own.
- `rmread/interval_void.go` holds nine bound predicates plus four helpers that duplicate generated `isZero*` functions. `rmread` imports only `openehr/rm`, so exporting one predicate from `rm` creates no import cycle.
- Three "no omit option" halves of the BMM mapping rules (container lower bound of 1 or more, mandatory generic, mandatory open parameter) are pinned only by `make codegen-verify`; flipping each in `render.go` leaves `go test ./internal/bmmgen/...` green.
- `CodeSetAccess` and `TerminologyAccess` are generated marker-only; the BMM declares 4 and 6 functions for them, and the `P_BMM_INTERFACE` row promises methods.
- The bmmgen package doc omits `interval_bound_gen.go`, and that file, unlike `release_gen.go`, gets no collision check against a BMM package file of the same name.

## Decisions for the maintainer

Decision 1 gates Phase 1 task 5, decision 2 gates Phase 2 task 1, and decision 3 gates Phase 4. Phase 1 tasks 1 to 4, the rest of Phase 2, and Phase 3 can start before any decision is made.

1. **Empty STRING error.** Decode refuses an empty STRING with `ErrUnsupportedDatatype`, the gap sentinel, and the package [deviations register](../../openehr/serialize/simplified/deviations.md) records that choice. The payload breaks an RM invariant, and the code comments in `string_leaf.go` and `flat_decode.go` give a malformed value a plain wrapped error without the gap sentinel; that convention is written nowhere else. The STRING row of § REQ-053 requires the refusal and names no error. Recommended: return a plain wrapped error, as the not-a-string case already does, and change the register line and the `string_leaf.go` godoc with it (Phase 1 task 5). The leaf is unreleased, so no consumer matches the sentinel yet. Alternative: keep the sentinel; the register already says so and nothing changes. Writing the sentinel into the § REQ-053 STRING row would be a full-lane edit, which this plan does not propose.
2. **Embedded flags of `PointInterval`.** Recommended: keep the struct shape and pin the existing "outer flags win" behaviour with a test. Dropping the re-declared flags is a Go API break and a regeneration for no consumer gain.
3. **Marker-only interfaces.** The `P_BMM_INTERFACE` mapping row in [bmm-conformance.md § REQ-043](../specifications/bmm-conformance.md#req-043--mapping-rules-p_bmm--go) promises methods translated from the BMM functions, and the generated code emits none, so code and spec disagree today. REQ-043 makes a deviation from its mapping rules depend on an ADR and an update to the section. Recommended: record the deviation in an ADR (an amendment of [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4, or a new one), then reword the row to say abstract interfaces are emitted without methods and name the two classes, since nothing consumes their methods. The ADR is accepted before Phase 4 is dispatched. Alternative: emit the methods, which conforms to the row as written and needs no ADR, but adds a generated API surface.

## Phases

### Phase 1 — FLAT codec fixes (maintenance lane)

**Tasks:**

1. A zero `DV_CODED_TEXT` writes no keys. No sentence governs this today: `codedToFlat` writes `|code` and `|value` unconditionally, and § REQ-053 covers only the empty STRING leaf and the all-zero `ctx/setting`. The PR adds one line to the package [deviations register](../../openehr/serialize/simplified/deviations.md) saying that an unset coded text writes no keys; that keeps the phase in the maintenance lane, and § REQ-053 is not amended. The PR notes the changed bytes for a consumer.
2. `walkAQL` in `flat_decode.go` refuses a second placement on an attribute that already holds a scalar, with `ErrUnknownPath`. This brings the code in line with the § REQ-053 rule on keys that reach two Web Template nodes for one single-valued RM attribute ([wire.md § REQ-053](../specifications/wire.md#req-053)). The overwrite is in the walk's `!ok` branch, where a slot that holds a scalar is replaced by a fresh map; the guard refuses a slot that is present and not a map, and keeps creating the map when the slot is absent. `placeLeaf`'s duplicate-placement check at the terminal slot stays as it is.
3. Reword the three stale STRING sentences to "an attribute `rmpath` does not resolve to an RM String". Keep the test name.
4. Add a table test for `isValueLeafType` covering `STRING`, a padded `" STRING"` and a non-leaf type, plus a STRING node with no inputs through `MarshalFlat`.
5. If decision 1 goes to a plain error, change `string_leaf.go` and the register line in `deviations.md`; otherwise leave both.

Each behaviour change is a red commit (failing test) before its fix.

**Definition of done:** `go test ./openehr/serialize/simplified/...`, then `make ci`. The PROBE-086 census is unchanged at 1467 keys. Removing the new guard in tasks 1, 2 and 4 with `go test -overlay` turns its test red.

### Phase 2 — Test pins and generator hygiene (maintenance lane)

**Tasks:**

1. Add `PointInterval[DVQuantity]` and `ProperInterval[DVQuantity]` cases to the canonical JSON and XML open-side tests: both sides open, lower only, upper only, and a flag set on the embedded interval (decision 2).
2. Pin the same switch in `internal/bmmgen/render_interval_bound_test.go` so removing an arm fails a bmmgen unit test, not only `codegen-verify`.
3. Add bmmgen render rows for the three "no omit option" halves, using real RM fields, each asserting the tag carries no omit option.
4. In `paths_rmpath_test.go`, the attribute-coverage guard of [rm-functions.md § REQ-121](../specifications/rm-functions.md#req-121--locatable-path-read-access) over the vendored templates, make the stale-entry checks run only when every subtest ran, or move them to a separate test. Pin the two skip sets separately, because they come from different branches of the test: the roots that are not a `COMPOSITION` (`Address.v2`, `TestPerson.v2`) and the OPT the parser refuses (`social.opt`), so a new parser refusal cannot pass as an expected skip.
5. Add the missing line to the bmmgen package doc and give `interval_bound_gen.go` the same collision check as `release_gen.go`, in a loop shared with the other fixed-name files.

**Definition of done:** `go test ./internal/bmmgen/... ./openehr/serialize/... ./openehr/template/webtemplate/...`, then `make ci` and `make codegen-verify`. The three mutations named in Evidence now fail a unit test.

### Phase 3 — AOM 1.4 interval encoding and one void predicate (full lane)

The AOM 1.4 gap and the duplicated predicates share one cause: the bound-emptiness rule lives in generated code that the abstract `Interval[T]` does not use. They move together.

**Tasks:**

1. Amend [§ REQ-052](../specifications/wire.md#req-052) so the open-side omission also binds `Interval[T]` fields of the AOM 1.4 types: delete the sentence that puts them outside the rule, and re-read the two sentences after it, which tie the rule to the § REQ-112 known gap on a bound beside its own open flag and to the § REQ-140 reading, so they still hold for the wider rule. Read through [clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) at that known gap and the [flat-decode fidelity plan](2026-09-30-flat-decode-fidelity.md)'s follow-up note, which carry the same reading. Widen the scope of [§ REQ-056](../specifications/wire.md#req-056), which today covers the RM surface only, to the same fields, so canonical XML element names follow its snake_case rule there too; this is a scope change, not a wording fix.
2. Generate `MarshalJSONTo` and the XML marshaller for `Interval[T]` with the same omission rule, from `internal/bmmgen`. Red tests first: an `occurrences` interval with an open upper side, in JSON and XML, plus a round trip through the vendored archetype corpus.
3. Add one generic predicate to `openehr/rm` that treats nil, a typed nil and a zero concrete bound as empty, and have `rmread` use it in place of its nine `isVoidDV*` predicates and four helpers (delete the parity test, or keep it if the helpers must stay). Exporting it is a public API shape, so the amended § REQ-052 names it in the sentence on an empty bound, which this phase edits anyway; the name is settled in the PR. If the maintainer would rather add no API, `rmread` keeps its own predicates and only the parity test pins them against the generated `isZero*` functions.
4. Update `interval_bound_gen.go` golden output and the REQ-052 and REQ-056 traceability rows with `make spec-gen`.

**Definition of done:** `make codegen-verify`, `make ci`. PROBE-030 and PROBE-033 stay green, but they pin RM roots only and never see an AOM 1.4 interval, so the phase adds its own test: the vendored ADL 1.4 corpus round-trips in JSON and XML with no `lower` or `upper` beside an open flag, and removing the omission arm for `Interval[T]` turns it red.

### Phase 4 — Marker-only interfaces (full lane)

**Tasks:** Apply decision 3. For the recommended reword: first the ADR that records the deviation, accepted; then change the `P_BMM_INTERFACE` row in [bmm-conformance.md § REQ-043](../specifications/bmm-conformance.md#req-043--mapping-rules-p_bmm--go) and add a bmmgen test that the two classes render as marker-only so the sentence stays true. For the alternative: emit the interface methods in `render_function.go` and regenerate; no ADR is needed, and the new methods are a consumer-visible addition.

**Definition of done:** `make codegen-verify` and `make ci`.

## Deferred

Three items stay out because each needs a consumer, a spec decision and a PROBE-086 census move, and each is already recorded in a deviations register:

- the slot-filled CLUSTER leaf: the corpus body `ehrbase_conformance_cluster` stays refused, because the Web Template spells `labresult` as a collapsed leaf where the reference emits a `text_value` child (recorded in `testkit/conformance/webtemplate/SKIPPED.md`);
- the in-context `ism_transition` leaves: the builder keeps the archetyped `transition` and `transition2` spelling (recorded in `openehr/template/webtemplate/deviations.md`);
- demographic `rmpath` navigation: PERSON, ADDRESS, PARTY_IDENTITY and CONTACT. Only needed if demographic FLAT encode is ever wanted. Phase 1 adds nothing here; a later PR confirms REQ-121 names it as a known gap.

## Consumer-visible changes

- Phase 1: FLAT encode no longer writes blank `|code` and `|value` keys for an unset coded text. FLAT decode refuses a second placement on a scalar-filled attribute.
- Phase 3: canonical JSON and XML of AOM 1.4 constraint intervals stop writing a zero bound beside an open flag, and XML uses the canonical element names. A new exported predicate in `openehr/rm`.
- Phases 2 and 4: none.
