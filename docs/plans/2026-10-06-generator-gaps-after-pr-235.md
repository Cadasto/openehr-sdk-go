---
kind: plan
---

# Plan: REQ-107 generator, the gaps PR 235 leaves

**Date:** 2026-10-06
**Status:** Draft, written when the maintainer stopped the PR 235 triage. Re-checked on `main` at `6ba94eaf` (v0.33.0): two Phase 2 rows moved to the backlog, which already held them, and the validator half of the ACTOR row landed in v0.32.0.
**Covers:** REQ-107 ([clinical-modeling.md § REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator)), REQ-102 ([§ REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation)), REQ-103 ([§ REQ-103](../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection))
**Probes:** PROBE-027
**Depends on:** [PR 235](https://github.com/Cadasto/openehr-sdk-go/pull/235), merged
**Defers:** the third structural level that § REQ-107 *Primitive-leaf value fill* already lists as deferred

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

Close the generator gaps the PR 235 reviews found but did not fix, and give REQ-107 a boundary that ends the review loop: a defect is one that a vendored template shows, and a template shape no vendored OPT has is a lead.

## Why the boundary comes first

PR 235 started as three spec commits stating the visit rule and the placeholders. Five review passes followed, by three reviewer setups. Each pass judged the generator against every sentence of § REQ-107. Each fix added code, tests and new binding sentences, and the next pass reviewed those. The contract speaks about any OPT, and the space of OPT shapes has no end, so every pass found another one. The last implementer round changed none of the 122 corpus outputs (61 OPTs under two policies) in either compile mode: the fixes closed shapes no vendored template has.

At the stop, every critical and important finding on PR 235 was fixed or declined. The commits after `b9e79fa1`, including the restructure of § REQ-107 in `f3353535`, have had no review pass.

## Phases

### Phase 1: bound the contract by the corpus

**Tasks:**
- Decide through `/sdd-specify` (an ADR, and the matching REQ-107 acceptance line) that REQ-107's acceptance is the vendored corpus: every OPT under both policies, both value fills, both compile modes and the builder passes `validation.ValidateRM` and the template validator. `TestREQ107_CorpusRatchet` in `testkit/probes/instance/corpus_ratchet_test.go` is the existing mechanism.
- In the same decision, say how a review rates a template shape no vendored OPT has: a suggestion for the backlog, unless it makes `Generate` panic or return a root the validators reject without an error.
- Shorten § REQ-107 where a case list only restates what the corpus test already shows. Keep the visit rule, the precedence order, the RM defaults and the error sentinels.
- Add a vendored OPT for each shape the plan below fixes, so the corpus shows the fix.

**Definition of done:** the decision is recorded and cited from § REQ-107; reviewers of the later phases are briefed with it; `make ci` is green.

### Phase 2: generator gaps

| Gap | Evidence |
|---|---|
| `PARTY_RELATED.relationship` has no writer. The BMM marks it mandatory, so a generated `PARTY_RELATED` subject fails the template validator with `required @ /subject/relationship`. It needs a writer and an RM default from the openEHR subject-relationship group. | `EnsureSingle` in `internal/templateinstance/rmwrite/write.go` has no `*rm.PartyRelated` case |
| The walk still builds the party the OPT names under `composer`, then replaces it with `Options.Composer`. An unsatisfiable constraint inside that party still makes `Generate` fail. Skip the walk of `composer` at a COMPOSITION root. | report on `1494f331` |
| The builder entry of `TestREQ107_CorpusRatchet` runs `ExampleFill` only. Add a `RandomFill` entry. | `testkit/probes/instance/corpus_ratchet_test.go` |
| An optional attribute the OPT names with no children and that has no writer is visited but left unset: DV_TEXT `language`, `accuracy` on DV_QUANTITY, DV_DATE_TIME and DV_COUNT, OBSERVATION `workflow_id`. `formatting` and `magnitude_status` are fixed (`7851404d`). | suggestion #6c61c on PR 235 |
| ACTOR `languages` and `roles` have no generator writer. This is part of § REQ-107's *Known gap: attributes the generator cannot write*. The validator side landed in v0.32.0: a list item without an `archetype_node_id` binds by RM type (§ REQ-102 *Items that are not a LOCATABLE*). | § REQ-107; `6558c9cc` |
| Unreachable code: the node-id branch of `fillPartyRelationship`, and the `finishNode` fallbacks for DV_EHR_URI, DV_ORDINAL and DV_SCALE. Keep them as named safety nets or remove them. | suggestions #ad9e3 and #3ef26 on PR 235 |
| No test may pin terminology `local` on a closed-list C_CODE_PHRASE that names no terminology. Check whether `TestREQ107_BareCodePhraseKeepsLocal` covers it. | suggestion #88b28 on PR 235 |

**Decision to confirm:** since `5482fcc5`, an attribute the OPT requires while prohibiting every child of it gets no member, where it used to get one default member. § REQ-107 *Exceptions and known gaps* reads this as an OPT that contradicts itself. Confirm it, or reverse it.

**Definition of done:** each gap is fixed with a reproduction test that fails first, or recorded as a Known gap; `make ci` is green.

### Phase 3: validator and parser gaps

These sit outside `openehr/instance`, but they limit what PROBE-027 can check the generator against.

| Gap | Evidence |
|---|---|
| `bindByRMType`, behind `matchSingleAlternative`, binds a single-valued attribute to the most specific alternative in three tiers, but it does not skip a prohibited child, and within a tier the first child in OPT order wins. So a valid slot fill after a prohibited ITEM_TREE can still bind to the prohibited one and be reported as `node_id_mismatch`. Verify with a fixture first. | `openehr/validation/walk_composition.go:191-226` |
| A slot's occurrences are not parsed: `template.Slot` carries none and compile copies none. This is § REQ-107's *Known gap: slot occurrences*. | `openehr/template/template.go`; suggestion #4e66a |
| A C_DATE_TIME, C_DATE or C_TIME range is dropped at parse, since `CDateTime` models only a pattern, so neither the validator nor `RandomFill` sees it. | `openehr/template/parse_primitives.go`; suggestion #3b121 |
| A generic `rm_type_name` other than a DV_INTERVAL instantiation, such as `POINT_EVENT<ITEM_TREE>`, cannot be built. This is § REQ-107's *Known gap: other generic types*. | § REQ-107 |

**Definition of done:** each gap is fixed with a can-fail test, and the matching Known gap is removed from § REQ-107; or it stays recorded there; `make ci` is green.
