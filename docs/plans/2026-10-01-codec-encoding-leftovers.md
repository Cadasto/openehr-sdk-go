---
kind: plan
---

# Plan — Codec and RM encoding leftovers from PRs 190 to 193

**Date:** 2026-10-01
**Status:** Draft — every item re-checked against main (b054065d); three decisions for the maintainer below
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

## Decisions for the maintainer (before Phase 3 and Phase 4)

1. **Empty STRING error.** Decode refuses an empty STRING with `ErrUnsupportedDatatype`, the gap sentinel, though the payload breaks an RM invariant and the package convention gives such a payload a plain wrapped error. Recommended: return a plain wrapped error, as the not-a-string case already does. The leaf is unreleased, so no consumer matches the sentinel yet. Alternative: keep the sentinel and say so in the REQ-053 STRING row.
2. **Embedded flags of `PointInterval`.** Recommended: keep the struct shape and pin the existing "outer flags win" behaviour with a test. Dropping the re-declared flags is a Go API break and a regeneration for no consumer gain.
3. **Marker-only interfaces.** Recommended: reword the `P_BMM_INTERFACE` row to say abstract interfaces are emitted without methods and name the two classes, since nothing consumes their methods. Alternative: emit the methods, which is a new generated API surface.

## Phases

### Phase 1 — FLAT codec fixes (maintenance lane)

**Tasks:**

1. A zero `DV_CODED_TEXT` writes no keys, in the same way an empty STRING and an all-zero `ctx/setting` already write nothing. The PR notes the changed bytes for a consumer.
2. The placement walk refuses a second placement on an attribute that already holds a scalar, with `ErrUnknownPath`, as `placeLeaf` already does for a slot conflict.
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
4. Make the stale-entry checks in `paths_rmpath_test.go` run only when every subtest ran, or move them to a separate test, and pin the expected skip set (the demographic roots and `social.opt`) so any other skip fails.
5. Add the missing line to the bmmgen package doc and give `interval_bound_gen.go` the same collision check as `release_gen.go`, in a loop shared with the other fixed-name files.

**Definition of done:** `go test ./internal/bmmgen/... ./openehr/serialize/... ./openehr/template/webtemplate/...`, then `make ci` and `make codegen-verify`. The three mutations named in Evidence now fail a unit test.

### Phase 3 — AOM 1.4 interval encoding and one void predicate (full lane)

The AOM 1.4 gap and the duplicated predicates share one cause: the bound-emptiness rule lives in generated code that the abstract `Interval[T]` does not use. They move together.

**Tasks:**

1. Amend [§ REQ-052](../specifications/wire.md#req-052) and the REQ-056 bullet so the open-side omission also binds `Interval[T]` fields of the AOM 1.4 types, and remove the sentence that puts them outside the rule. Name the canonical XML element spelling for the same fields.
2. Generate `MarshalJSONTo` and the XML marshaller for `Interval[T]` with the same omission rule, from `internal/bmmgen`. Red tests first: an `occurrences` interval with an open upper side, in JSON and XML, plus a round trip through the vendored archetype corpus.
3. Export one generic predicate from `openehr/rm` (name to settle in the PR) that treats nil, a typed nil and a zero concrete bound as empty. Replace the nine `isVoidDV*` predicates and four helpers in `rmread` and delete the parity test, or keep the parity test if the helpers must stay.
4. Update `interval_bound_gen.go` golden output and the REQ-052 and REQ-056 traceability rows with `make spec-gen`.

**Definition of done:** `make codegen-verify`, `make ci`, PROBE-030 and PROBE-033 green. The vendored ADL 1.4 corpus round-trips in JSON and XML with no `lower` or `upper` beside an open flag.

### Phase 4 — Marker-only interfaces (full lane)

**Tasks:** Apply decision 3. For the recommended reword: change the `P_BMM_INTERFACE` row in [bmm-conformance.md](../specifications/bmm-conformance.md), and add a bmmgen test that the two classes render as marker-only so the sentence stays true. For the alternative: emit the interface methods in `render_function.go` and regenerate.

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
