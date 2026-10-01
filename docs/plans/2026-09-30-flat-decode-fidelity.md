---
kind: plan
---

# Plan — Codec fidelity: archetype details on FLAT decode, and two silent drops

**Date:** 2026-09-30
**Status:** Done in PR 193; from the audit of the PR 181–189 review leftovers; decisions settled 2026-10-01 (below)
**Covers:** REQ-053 and REQ-140 ([wire.md § REQ-053](../specifications/wire.md#req-053), [§ REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes)), REQ-052 ([wire.md § REQ-052](../specifications/wire.md#req-052)); interacts with REQ-112 ([clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor))
**Probes:** PROBE-086, PROBE-089 (census and round trip), PROBE-030 (canonical JSON)
**Depends on:** nothing ([PR 191](https://github.com/Cadasto/openehr-sdk-go/pull/191), merged, rewrote the § REQ-140 bullet Phase 3 edits)
**Defers:** carrying `archetype_details` inside FLAT itself (the format has no key for it)

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

A composition decoded from FLAT or STRUCTURED passes the SDK's own template-less floor, and no encoder drops or invents data without saying so.

**Phase 1 blocks the next release.** PR 188 made the floor report a missing `archetype_details` on every archetype root. That rule is merged but not yet released. Measured on main (`bfa10c10`): `ValidateRM` rejects **24 of 24** decodable FLAT bodies of the vendored conformance corpus with `is_archetype_root` at the COMPOSITION and OBSERVATION roots, while `ValidateComposition` passes them. Releasing as is would ship a floor that refuses the SDK's own decode output.

## Evidence

| Symptom | Where |
|---|---|
| Decode builds the COMPOSITION root with `_type`, `archetype_node_id` and `name` only; no node gets `archetype_details` | `openehr/serialize/simplified/flat_decode.go:90-96`; nothing in the package mentions `archetype_details` |
| A canonical → FLAT → canonical round trip loses `template_id` silently; § REQ-140's list of classes encode drops does not name `archetype_details` | wire.md § REQ-140, the LOCATABLE row |
| A Web Template leaf whose `rmType` carries padding (` DV_INTERVAL<DV_QUANTITY>`) is skipped as structure, so encode drops all of its keys with no error | `flat_encode.go:249-251`; `isValueLeafType` checks `HasPrefix("DV_")` on the untrimmed name (`datatypes.go:229-231`) |
| Canonical JSON writes `"upper":{"_type":"DV_QUANTITY","magnitude":0,"units":""}` beside `"upper_unbounded":true` for a Go-built interval with one open side (a lab range "< 5") | `rm.Interval[T]` bounds are value types; `omitempty` does not omit a zero struct (`openehr/rm/foundation_types_interval_gen.go:34-47`) |
| The generator hard-codes `rm_version: "1.1.0"`, while the pinned BMM is RM 1.2.0 | `openehr/instance/generate.go:581`, `:751` |

## Decisions for the maintainer (before Phase 1)

1. **What decode writes.** Recommended: `archetype_id` from the Web Template node id on every archetype root (the nodes whose id is an archetype id), `template_id` from `WebTemplate.TemplateID` on the COMPOSITION root only (the RM makes it optional; Phase 0 confirms where the reference writes it), and `rm_version` from one SDK constant that the generator uses too. Phase 0 checks this against the reference before the code is written.
2. **Which `rm_version` string.** The pinned RM release (1.2.0), or the version the reference writes. Whichever is chosen, generator and decoder share it.
3. **Padded names.** Trim `rmType` once when a Web Template is parsed, so every consumer sees clean names (recommended), rather than trimming at each use.

**Settled (2026-10-01).** Phase 0 ran against a local EHRbase 2.36.0 (openEHR SDK 2.35.0): two corpus bodies posted as FLAT and read back as canonical JSON carry `archetype_details` on the COMPOSITION, the SECTION, the OBSERVATION and a slot-filled CLUSTER, with `archetype_id` equal to the node id, `template_id` on the COMPOSITION only, and `rm_version` `1.0.4`. The reference's FLAT decoder (`ToCompositionWalker`, openEHR_SDK at the corpus pin `e57511c6`) sets it the same way.

1. Decode writes what the reference writes, in both decode modes.
2. `rm_version` is the RM release the SDK is generated from, `rm.Release` (`1.2.0`), a constant bmmgen emits from the BMM's `rm_release` so that § REQ-041 holds; the generator uses it too. The reference's `1.0.4` is not copied, because `rm_version` records the release an object was built against.
3. The `webtemplate` package has no parse step (callers decode the struct), so the codec reads every leaf type through one normaliser instead.

Phase 2 also gained the STRING leaf: the encode backstop cannot land while encode silently skips ACTIVITY `action_archetype_id`, so § REQ-053's leaf grammar now carries it as a bare value, the reference's spelling.

## Phases

### Phase 0 — ground truth from the reference

**Tasks:**
- Post two corpus FLAT bodies to the local EHRbase (see `testkit/recordings/README.md` for the live setup) and read them back as canonical JSON. Record where `archetype_details` appears, with which `template_id` and `rm_version`.
- Write the answer into the PR description for Phase 1; no new fixture unless a recording is captured with `make probe-record`.

**Definition of done:** decisions 1 and 2 are settled with the observed output quoted.

### Phase 1 — decode rebuilds `archetype_details`

**Tasks:**
- In `decodeFlat` and the node reconstruction below it, attach `archetype_details` to the COMPOSITION root and to each archetype-root node, per decision 1. STRUCTURED goes through the same path.
- Add the round-trip facet to PROBE-089 or a new test: every decodable corpus body decodes with `WithTemplate` and then passes `ValidateRM`. Prove it can fail by removing the new attachment with `go test -overlay`.
- Amend § REQ-053 / § REQ-140 so the spec says decode reconstructs `archetype_details` from the Web Template, and name it among the attributes FLAT does not carry.
- Move the generator's `"1.1.0"` onto the shared constant.

**Definition of done:** 24 of 24 decodable corpus bodies pass `ValidateRM`; `make ci` green; the PR body names the consumer-visible change (decoded compositions now carry `archetype_details`).

### Phase 2 — no silent drop for padded leaf types

**Tasks:**
- Trim `rmType` when the Web Template is parsed (decision 3), and add a padded-name fixture test on both encode and decode.
- Make encode report a typed error when a populated value sits on a node it cannot classify, instead of skipping it, as § REQ-140's no-silent-loss rule asks.

**Definition of done:** the padded-name test round-trips all ten interval keys; removing the trim fails it.

### Phase 3 — no phantom zero bound in canonical JSON

**Tasks:**
- On encode, leave out a side's bound when that side is unbounded **and** the bound is its type's zero value. A real bound beside its open flag stays, per the ruling recorded in § REQ-112 (it is checked, not flagged).
- Check canonical XML for the same shape and treat it the same way.
- Extend the zero-bound statement in § REQ-140 (FLAT only today) to the canonical encoders, or state it once under § REQ-052 and point to it.

**Definition of done:** encoding a one-sided `DVInterval[DVQuantity]` emits no bound on the open side; PROBE-030 stays green; a test pins both the omitted zero and the kept real bound.

## Consumer-visible changes

- Decoded compositions gain `archetype_details` (Phase 1).
- A padded Web Template now encodes its leaves, or fails loudly (Phase 2).
- Canonical output drops a meaningless zero bound (Phase 3).
