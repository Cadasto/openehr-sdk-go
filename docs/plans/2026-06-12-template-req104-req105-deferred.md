# Plan — REQ-104 slot assertions and REQ-105 terminology bindings (deferred)

**Date:** 2026-06-12
**Status:** Draft (deferred — no committed delivery window)
**Owner:** SDK maintainers
**Covers:** REQ-104 (slot assertions); REQ-105 (terminology bindings)
**Implementation:** planned
**Depends on:** [archive/2026-05-22-template-req100-followups.md](archive/2026-05-22-template-req100-followups.md) Phases 1–6 (landed — compiled template, walker, REQ-103 primitives); REQ-101 composition builder and REQ-102 validation (landed)
**Defers:** External terminology lookup (live SNOMED/LOINC resolution); full Archie/Linker assertion parity; AOM 2 / ADL 2

## Goal

Land the remaining template-modelling REQs from the [REQ-100 follow-up plan](archive/2026-05-22-template-req100-followups.md) when consumers need stricter slot-fit checking or richer terminology surfacing. Phases 1–6 of that plan shipped the foundation; this plan tracks only the deferred tail.

## When to run

| Trigger | Action |
|---|---|
| Validator or builder surfaces a concrete slot-fit failure the RM-type-prefix fallback cannot express | Start **Phase 1 (REQ-104)** |
| Composition rendering, FHIR mapping, or UI export needs structured term bindings beyond raw OPT accessors | Start **Phase 2 (REQ-105)** |

## Phase 1 — REQ-104 slot assertion grammar

**Outcome:** Validators can determine whether a candidate archetype satisfies a slot's `includes` / `excludes` assertions, instead of falling back to RM-type prefix match only.

The OPT XSD exposes slot assertions as XML expression trees:

```xml
<archetype_slot rm_type_name="OBSERVATION" node_id="at0002">
  <includes>
    <expression><value>archetype_id matches {/openEHR-EHR-OBSERVATION\.body_weight\..*/}</value></expression>
  </includes>
</archetype_slot>
```

**Tasks:**

1. **Spec REQ-104** documenting the assertion grammar subset to be supported (initially just `archetype_id matches {regex-list}`).
2. **`SlotAssertion` typed AST** in `openehr/template/constraints/` with `MatchesArchetypeID(string) bool`.
3. **Parse** the expression sub-tree at compile time; cache the compiled regex per slot.
4. **Pragmatic default** — until REQ-104 lands, expose `Slot.RawIncludes() []string` (current behaviour) **and** add `Slot.AllowsRMType(rm string) bool` implementing the RM-type-prefix fallback. Validators use the prefix fallback unless a structured AST is available.

## Phase 2 — REQ-105 terminology bindings

**Outcome:** Consumers can resolve archetype-node-id (`at0001`) to display text in any of the OPT's languages, and follow `term_bindings` to external terminologies (SNOMED, LOINC, ICD-10).

**Tasks:**

1. **Spec REQ-105** documenting the `ArchetypeTerm` / `TermBinding` surface, the per-language map shape, and the fallback rule when the requested language is missing.
2. **Compile-time flattening** — already prescribed in the archived follow-up plan Phase 4; this REQ formalises the public accessor (`compiled.Term(nodeID, lang)`, `compiled.TermBindings(nodeID)`).
3. **External terminology lookup** is **out of scope** — REQ-105 only exposes the bindings the OPT carries.

## Implementation checklist

| Step | Status |
|---|---|
| Phase 1 — REQ-104 slot assertions | |
| Phase 2 — REQ-105 terminology bindings | |
| `make ci` green throughout | |

## References

- Historical delivery detail (Phases 1–6 landed): [archive/2026-05-22-template-req100-followups.md](archive/2026-05-22-template-req100-followups.md)
- Compiled template foundation: [ADR 0005](../adr/0005-compiled-template-foundation.md)
