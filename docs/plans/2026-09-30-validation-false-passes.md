---
kind: plan
---

# Plan — Validation: close the two false "valid" verdicts left after v0.33.0

**Date:** 2026-09-30
**Status:** Draft — from the audit of the PR 181–189 review leftovers. Re-checked on `main` at `6ba94eaf` (v0.33.0): the floor now reports an open interval side marked included, and template validation runs the floor, so an empty DV_ORDINAL `symbol` is reported ([ADR 0025](../adr/0025-template-validation-includes-the-rm-floor.md)). Re-checked again at `aad783b7` (after v0.34.0 and PR 271, which made the two validators one walk): neither phase has landed. The one walk still hands the ordinal constraint the integer value alone (`openehr/validation/primitive_dispatch.go:85-90`), and § REQ-112 *Value-typed mandatory presence* is unchanged, though § REQ-171 *Structural access* now states the presence reading the floor uses, so Phase 2 can cite it.
**Covers:** REQ-112 ([clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor)), REQ-102 and REQ-110 ([§ REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation), [§ REQ-110](../specifications/clinical-modeling.md#req-110--template-driven-validation-beyond-composition))
**Probes:** PROBE-081, PROBE-027
**Defers:** the other REQ-112 catalogue gaps (a bound beside its own open flag, `Limits_comparable`, `Other_reference_ranges_validity`, `Range_is_simple`, `Scheme_valid`, DV_AMOUNT `accuracy_is_percent`); they are recorded, not planned

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

The template validator stops passing a DV_ORDINAL whose symbol does not match its value, and the spec stops claiming a limit the floor no longer has.

## Evidence (main `6ba94eaf`, line numbers as at `aad783b7`)

| False pass | Evidence |
|---|---|
| A DV_ORDINAL whose `symbol` does not match the template's (value, symbol) list passes `ValidateComposition`. An empty `symbol` is already reported, by the floor, as `required` at `…/symbol`. | `openehr/validation/primitive_dispatch.go:85-90` passes only the integer `value` to the constraint; `constraints.CDvOrdinal` (`openehr/template/constraints/quantity.go:140`) holds the pairs, and its `Validate` already accepts a `constraints.OrdinalSymbol` that nothing sends |
| § REQ-112 *Value-typed mandatory presence* says a value-typed mandatory attribute cannot be detected, naming EHR_STATUS.subject as the case. The floor does report zero-valued DV_ORDINAL / DV_SCALE `symbol` and REFERENCE_RANGE `range` as `required`, because § REQ-171 *Structural access* reads a by-value attribute holding its type's zero value as absent; EHR_STATUS `subject` is one of the exceptions that section fixes. | `clinical-modeling.md:1015` onward; [§ REQ-171](../specifications/clinical-modeling.md#req-171--validation-engine) *Structural access*; `openehr/validation/rmfloor_ordered_test.go:44-53`, `:107-113` |

## Phases

### Phase 1 — the template validator checks the ordinal symbol

**Tasks:**
- Pass the whole DV_ORDINAL (value and symbol code) to the ordinal constraint as a `constraints.OrdinalSymbol` and check the pair against the template's list. Report a symbol that does not match its value; leave the empty symbol to the floor, so it is reported once.
- Run the PROBE-027 corpus census (`TestREQ107_CorpusRatchet`): generated ordinals carry their listed symbols, so it must stay green under the new check.

**Definition of done:** a mismatched pair is reported, and an empty symbol is still reported once; the vendored compositions with ordinals stay clean, or each new finding is explained in the PR.

### Phase 2 — the presence section says what the floor does

**Tasks:**
- Rewrite § REQ-112 *Value-typed mandatory presence* to separate the value-typed attributes the floor does judge by their zero value (DV_ORDINAL and DV_SCALE `symbol`, REFERENCE_RANGE `range`) from the ones it cannot, such as EHR_STATUS.subject, citing § REQ-171 *Structural access* for the reading rather than restating it.
- The resolution of [STRAND-15](../specifications/research-strands.md#strand-15--decode-time-presence-and-unmatched-member-capture-on-the-generated-types) rewrites the same section when decode-time presence arrives. This phase is the smaller correction for the meantime; whichever lands second rebases on the first.

**Definition of done:** the section and `rmfloor_ordered_test.go` agree; `make spec-check` green.

## Consumer-visible changes

The template validator reports new findings on data it accepted before. That is tightened validation, so the release notes name it.
