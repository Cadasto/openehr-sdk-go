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

The template-less floor reports no finding for an EHR_STATUS (or COMPOSITION, EHR_ACCESS, PARTY) without
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
- Evaluate `Is_archetype_root` for the classes whose BMM declares it. Derive the set from the BMM class invariants if
  `rminfo` can expose them; otherwise a closed list with a test pinning it against the BMM, so a BMM bump that adds a
  root class fails loudly (ADR 0001).
- Walk `archetype_details` on every LOCATABLE where present: `archetype_id` / `archetype_id.value` / `rm_version`, with
  the codes and paths the spec rows fix.
- `…Bytes` entries decide `archetype_id` / `rm_version` presence from the JSON key set (the PROBE-081 mechanism);
  value-based entries report empties as the spec says.
- SHOULD: implement `LOCATABLE.is_archetype_root` so the generated methods in `openehr/rm/common_archetyped_gen.go`
  stop panicking.

**Definition of done:** every row of the Goal table produces the spec's finding; a complete `archetype_details`
produces none; a FOLDER without `archetype_details` produces none; a nested LOCATABLE with an incomplete `ARCHETYPED`
is reported at its own path; removing either evaluator fails a named test.

## Mapping to specs

- [clinical-modeling.md § REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) — normative contract
- [REQ.md](../specifications/REQ.md) — registry row
