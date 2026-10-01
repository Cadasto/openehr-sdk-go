---
kind: plan
---

# Plan — Validation: close three false "valid" verdicts

**Date:** 2026-09-30
**Status:** Draft — from the audit of the PR 181–189 review leftovers
**Covers:** REQ-112 ([clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor)), REQ-102 and REQ-110 ([§ REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation), [§ REQ-110](../specifications/clinical-modeling.md#req-110--template-driven-validation-beyond-composition))
**Probes:** PROBE-081, PROBE-027
**Depends on:** [PR 191](https://github.com/Cadasto/openehr-sdk-go/pull/191), merged (Phase 1 removes the Known gap it added)
**Defers:** the other REQ-112 catalogue gaps (a bound beside its own open flag, `Limits_comparable`, `Other_reference_ranges_validity`, `Range_is_simple`, `Scheme_valid`, DV_AMOUNT `accuracy_is_percent`); they are recorded, not planned

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

The two validators stop passing data the RM or the template forbids, in the three places the audit found, and the spec stops claiming a limit the floor no longer has.

## Evidence (main `bfa10c10`)

| False pass | Evidence |
|---|---|
| An interval whose open side is marked included (`lower_unbounded` and `lower_included` both true) passes `ValidateRM`. BASE `Interval` `Lower_included_valid` / `Upper_included_valid` forbid it. Since PR 191, FLAT no longer produces the shape, but canonical JSON or XML, a FLAT `\|raw` fragment and Go-built values still reach the floor with it. | probe: `ValidateRM` on a DV_QUANTITY `normal_range` built that way reports nothing; `openehr/validation/rmfloor.go:511` (`checkDVInterval`) never reads the flags |
| A DV_ORDINAL whose `symbol` is empty, or does not match the template's (value, symbol) list, passes `ValidateComposition`. | `openehr/validation/primitive_dispatch.go` passes only the integer `value` to the constraint; `constraints.CDvOrdinal` (`openehr/template/constraints/quantity.go:137`) holds the pairs |
| § REQ-112 *Value-typed mandatory presence* says a value-typed mandatory attribute cannot be detected, naming EHR_STATUS.subject as the case. The floor does report zero-valued DV_ORDINAL / DV_SCALE `symbol` and REFERENCE_RANGE `range` as `required`. | `clinical-modeling.md:888` onward; `openehr/validation/rmfloor_ordered_test.go:44-50`, `:108-113` |

## Phases

### Phase 1 — the floor checks the included flags

**Tasks:**
- In the floor's interval evaluation, report an open side marked included under a rule-specific code, following the term-mapping precedent (for example `lower_included_valid` / `upper_included_valid`), at the interval's path. It covers every interval the readers reach: DV_INTERVAL values, typed `normal_range`, and REFERENCE_RANGE `range`.
- Add catalogue rows to § REQ-112 and delete the Known gap PR 191 added.
- A can-fail test per side, and one for a typed interval inside `normal_range`.

**Definition of done:** the probe case above is reported; `make ci` green; the PR body names it as tightened validation.

### Phase 2 — the template validator checks the ordinal symbol

**Tasks:**
- Pass the whole DV_ORDINAL (value and symbol code) to the ordinal constraint and check the pair against the template's list. Report an empty symbol and a symbol that does not match its value.
- Run the PROBE-027 corpus census (`TestREQ107_CorpusRatchet`): generated ordinals carry their listed symbols, so it must stay green under the new check.

**Definition of done:** a mismatched pair and an empty symbol are each reported; the vendored compositions with ordinals stay clean, or each new finding is explained in the PR.

### Phase 3 — the presence section says what the floor does

**Tasks:**
- Rewrite § REQ-112 *Value-typed mandatory presence* to separate the value-typed attributes the floor does judge by their zero value (DV_ORDINAL and DV_SCALE `symbol`, REFERENCE_RANGE `range`) from the ones it cannot, such as EHR_STATUS.subject.

**Definition of done:** the section and `rmfloor_ordered_test.go` agree; `make spec-check` green.

## Consumer-visible changes

Both validators report new findings on data they accepted before. That is tightened validation, so the release notes name it.
