# Plan — RM floor: archetype roots and ARCHETYPED

**Date:** 2026-09-24
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-112 ([clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor), *Per-RM-type invariant catalogue*: the *Archetype roots* and *ARCHETYPED* rows) — no new id
**Probes:** PROBE-081 extended (value-typed presence now also covers `ARCHETYPED.archetype_id` / `rm_version`)
**Implementation:** planned
**Depends on:** the landed REQ-112 floor (`openehr/validation/rmfloor*.go`)
**Defers:** `ARCHETYPE_ID` grammar validation; rules needing stored history (`VERSIONED_COMPOSITION.Archetype_node_id_valid`); `ARCHETYPED.template_id` presence (optional in the RM)

## Goal

The template-less floor reports no finding for a COMPOSITION, EHR_STATUS, EHR_ACCESS, PARTY or ENTRY without
`archetype_details`, nor for an `archetype_details` missing `archetype_id` or with an empty `rm_version`. Measured at
v0.27.0, every one of these passes `ValidateRMEHRStatusBytes` clean:

| `archetype_details` | Floor result |
|---|---|
| absent | OK |
| `{"_type":"ARCHETYPED"}` | OK |
| `archetype_id.value` `""` | OK |
| `rm_version` absent | OK |
| `rm_version` `""` | OK |

The RM forbids all five (`Is_archetype_root` + `LOCATABLE.Archetyped_valid`; `ARCHETYPED` 1..1 attributes and
`Rm_version_valid`). They are invariants of the object itself, so the floor is the right layer. Reported by the
consuming CDR project, which carries a local check with the same codes and paths meanwhile and deletes it when this
lands.

## Definition of Ready

- [x] **Covers** lists REQ-112; the canonical rows exist in clinical-modeling.md § REQ-112 (this PR, spec-first).
- [x] REQ.md row and `traceability.yaml` flipped to `partial`.
- [x] No irreversible fork: the codes follow the term-mapping precedent (rule-specific stable codes).

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry updated (`traceability.yaml`, REQ.md row) | done (this PR) |
| Indexes `spec-check` misses (`roadmap.md` row; no new REQ id, so no band change) | at landing |
| Code | |
| Tests with `// REQ-112` / `// PROBE-081` comments | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 1 — the catalogue rows

**Tasks:**
- Evaluate `Is_archetype_root` as a closed list of the five BMM declarers (COMPOSITION, EHR_ACCESS, EHR_STATUS, PARTY,
  ENTRY) with their concrete descendants, pinned by a test against the vendored BMM's `Is_archetype_root` declarations
  so a BMM bump that adds a root class fails loudly (ADR 0001). `rminfo` exposes no invariants, so deriving the set
  from it is not an option (that would be REQ-048/049 surface).
- Surface `archetype_details` from the readers: today no `rmread` reader returns it and `rmread.Handles` omits
  `rm.Archetyped`, which is why the floor never descends there. Add the reader and the `Handles` entry beside the
  evaluators.
- Walk `archetype_details` on every LOCATABLE where present: `archetype_id` / `archetype_id.value` / `rm_version`, with
  the codes and paths the spec rows fix.
- `ValidateRMEHRStatusBytes` decides `archetype_id` / `rm_version` key presence for the root EHR_STATUS only (its
  existing `subject` scope), reporting an absent or `null` key as `required` at the attribute path and replacing the
  value walk's finding at that same path; value-based entries report empties as the spec says.

**Definition of done:** every row of the Goal table produces the spec's finding; a complete `archetype_details`
produces none; a FOLDER without `archetype_details` produces none; an ENTRY (for example an EVALUATION or OBSERVATION)
nested in a COMPOSITION with no `archetype_details` is reported at its own path; a nested LOCATABLE with an incomplete
`ARCHETYPED` is reported at its own path; removing either evaluator fails a named test.

## Vendored content

The rule matches the RM and is not weakened. Some EHRbase-origin samples under `testkit/cassettes/rm/` omit
`archetype_details` on a root or on a nested entry, so they gain a floor finding once this lands. Counted with a
throwaway walk over `testkit/cassettes/rm/**` and `testkit/cassettes/compositions/*.json`: the root `_type` for the
top-level rows, a recursive `_type` scan for nested entries, and a sample counts when the `archetype_details` key is
absent or `null`.

| Sample set | Count | Runs through PROBE-030's floor leg |
|---|---|---|
| Top-level EHR_STATUS with no `archetype_details` | 19 | 9 (the eight `ehr_status_valid_*` plus `ehr_status_other_details_simple.json`); the ten `ehr_status_invalid_*` are excluded from PROBE-030 as API-validation payloads |
| Top-level COMPOSITION with no `archetype_details` | 3 | 0 (all under `rm/polymorphic/`, a subdirectory PROBE-030 does not walk) |
| Nested EVALUATION with no `archetype_details` | 2 | 2 (`minimal_evaluation.json` and `compo_with_nested_party_related.json`, both loaded as COMPOSITIONs) |
| Nested OBSERVATION with no `archetype_details` | 3 | 0 (three under `rm/polymorphic/`, not walked) |

So 11 samples run through PROBE-030's `ValidateRM` floor leg and would carry a new finding. No vendored sample has an
incomplete ARCHETYPED (a missing `archetype_id`, an empty value, or a missing or empty `rm_version`), so no
ARCHETYPED-arm hold-out is needed.

Handling, at implementation, with no fixture content edited:

- The affected cassettes become named `SkipFloor` hold-outs carrying the finding, the mechanism PROBE-030's catalog
  entry already sanctions.
- The OK-asserting root-class unit fixtures gain `archetype_details` at implementation:
  `TestValidateRMEHRStatusBytes_BareSubjectOK` and `TestValidateRMEHRStatus_MinimallyValid`
  (`openehr/validation/rmfloor_test.go`) both assert OK on an EHR_STATUS with no `archetype_details`, and no floor test
  fixture sets it today; their facet is the subject, not the root.

## Not in this plan

The generated `IsArchetypeRoot()` methods on the RM types (`openehr/rm/common_archetyped_gen.go`) panic today and
nothing calls them. Implementing `LOCATABLE.is_archetype_root` so they stop panicking belongs to the RM
behavioural-functions surface (REQ-120 to REQ-123), an optional follow-up there rather than a task of this plan.

## Mapping to specs

- [clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) — normative contract
- [REQ.md](../specifications/REQ.md) — registry row
