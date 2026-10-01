---
kind: plan
---

# Plan — Generator and validator follow-ups from PR 196

**Date:** 2026-10-01
**Status:** Done — phases 1 to 4 landed; decisions 1 to 3 took the recommended option (whole-string pattern, no child for an optional attribute the OPT leaves empty, uncomparable pairs left as generated)
**Covers:** REQ-107 ([clinical-modeling.md § REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator)), REQ-103 ([§ REQ-103](../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection)), REQ-102 ([§ REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation)), REQ-112 ([§ REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor))
**Probes:** PROBE-027 (the corpus census; its nine rows are the baseline)
**Depends on:** PR 196 merged
**Defers:** the slot fills the census still refuses (`family_history.v.1.2.3`, `Corona_Anamnese`) and `ITEM_TABLE.rotated`

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

Close the four gaps PR 196 left open on purpose, so generated values fill every constrained text field, the generator and validator read a C_STRING pattern the same way, generated entries pass the RM floor without a tolerated issue, and interval ordering has a stated rule for pairs it cannot compare. Each item below was reported by a worker or reviewer during the PR 196 triage and is recorded as reported; the first task of each phase is to reproduce it on main.

## Evidence (as reported, not re-checked)

- **Optional text fields stay unset.** The generator recognises a C_STRING on `DV_TEXT.formatting` and on `DV_IDENTIFIER` `issuer`, `type` and `assigner` (also `preferred_term`, `alternate_text`, `magnitude_status`), but writes nothing, because the validator cannot read them: `primitiveValueMatchesShortName` (`openehr/validation/walk_composition.go`, around line 678) accepts only `string` for STRING while rmread returns `*string` for optional fields, and the DV_TEXT reader in `openehr/validation/rmread/read.go` (around line 1255) has no `formatting` arm. Filling them today adds 15 `rm_type_mismatch` census rows on `Test_dv_identifier_pattern_constraint.v0`, fails `TestREQ107_FloorGapsFilled`, and makes `ValidateComposition` report `formatting` as absent. The generator change is one line in `optionalString` (`openehr/instance/generate.go`): return the writer `func(s string) { *f = &s }` instead of `nil`.
- **Two readings of a pattern.** The generator accepts a value only when the C_STRING pattern matches the whole string; `CString.Validate` (`openehr/template/constraints/string.go`, around line 79) accepts a match anywhere in it. § REQ-103 does not say which reading binds.
- **A tolerated floor issue.** `materialiseImplicitMultiple` (`openehr/instance/generate.go`, around line 825) builds an archetype-root ENTRY for OPT-silent `COMPOSITION.content` without `archetype_details`, so `ValidateRM` reports `is_archetype_root @ /content[0]/archetype_details`. The two BMM-built cases in `openehr/instance/rm_defaults_test.go` accept exactly that issue.
- **Pairs left unordered.** Interval ordering now compares date-times as instants and leaves alone any pair it cannot compare (a zoned value against a zoneless one, a partial value against a full one, such as DV_DATE `2026-10` against `2026-01-01`). The old code swapped those by text. The generator never writes such pairs itself; they come only from other sources.

## Decisions for the maintainer

1. **Pattern reading (Phase 2).** Recommended: the whole-string reading, which is how a C_STRING pattern constrains a value in the AOM; check that against the AOM 1.4 specification first, then make `CString.Validate` anchor the match and state the reading in § REQ-103. Alternative: keep the substring reading and loosen the generator to it.
2. **Archetype-root ENTRY with no OPT node (Phase 3).** Recommended: do not build a child for an optional attribute the OPT leaves empty, so nothing archetype-rooted appears without an archetype. Alternative: stamp an `openEHR-EHR-<TYPE>.example.v1` root with `archetype_details`, as slot fills already do.
3. **Pairs that cannot be compared (Phase 4).** Recommended: keep leaving them alone and say so in one § REQ-107 sentence. Alternative: order them by their earliest instant.

## Phases

### Phase 1 — The validator reads optional text fields; the generator fills them

**Tasks:**

1. Reproduce: a filled `DV_TEXT.formatting` and a filled `DV_IDENTIFIER.issuer` against a template that constrains them.
2. Make `primitiveValueMatchesShortName` accept a `*string` for STRING (nil as absent), and add the `formatting` arm to the rmread DV_TEXT reader; check the other `*string` RM fields the readers skip in the same pass.
3. Apply the one-line `optionalString` change, then tighten `TestREQ107_StringLeafDispatchesOnAttribute` to require `formatting` to be a list member.
4. Run the census; it keeps its nine rows.

**Definition of done:** `go test ./openehr/instance/... ./openehr/validation/... ./testkit/probes/instance/...` and `make ci`; reverting each reader change with `go test -overlay` turns a test red.

### Phase 2 — One reading of a C_STRING pattern (decision 1)

**Tasks:** settle decision 1 in § REQ-103 (full lane); align `CString.Validate` or the generator; add a test with a pattern that matches inside a string but not the whole of it, run through both the generator and the validator.

**Definition of done:** `make ci`; the census keeps its nine rows, or the PR names every row that moves.

### Phase 3 — No tolerated floor issue (decision 2)

**Tasks:** apply decision 2 in `materialiseImplicitMultiple`; remove the tolerated `is_archetype_root` issue from the two BMM-built tests so they require a clean `ValidateRM`; if decision 2 takes the stamping option, cover it with the slot-fill tests' assertions.

**Definition of done:** `go test ./openehr/instance/...` and `make ci`, with no test accepting a floor issue.

### Phase 4 — A rule for pairs that cannot be compared (decision 3)

**Tasks:** write the § REQ-107 sentence decision 3 picks, and pin it with the mixed-zone and partial-date cases already in `openehr/instance/interval_temporal_internal_test.go`.

**Definition of done:** `make spec-check` and `go test ./openehr/instance/...`.

## Related, not in scope

These came up during the same triage and stay out unless the maintainer pulls them in:

- `rmwrite.EnsureSingle` has no case for `DV_PARSABLE`, `DV_IDENTIFIER`, `DV_URI` or `DV_EHR_URI`, so `writeBMMString` cannot write their values and its error is ignored.
- `ValidateRM` accepts `DV_DATE_TIME.value = "example"`; the floor has no ISO 8601 check.
- `fillCurrentState`'s `firstCodedExample` path looks unreachable.
- The fourteen suggestions in the PR 196 findings file.

## Consumer-visible changes

- Phase 1: generated values gain the optional text fields a template constrains; `ValidateComposition` checks them.
- Phase 2: depending on decision 1, `CString.Validate` refuses a value the pattern matches only in part.
- Phase 3: depending on decision 2, generated compositions lose an OPT-silent archetype-root entry, or gain an example archetype id on it.
- Phase 4: none, or reordered interval bounds for mixed or partial values (decision 3).
