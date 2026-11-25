---
kind: adr
id: ADR-0025
title: "Template-driven validation includes the RM floor in one walk, template admission first"
status: accepted
date: 2026-10-11
---

# ADR 0025 — Template-driven validation includes the RM floor in one walk, template admission first

- **Status:** Accepted, 2026-10-11 — maintainer decision.
- **Amended:** 2026-11-25: takes in template admission before the RM rules and the place of the walk; states the one walk the engine runs.
- **Strand:** resolves [STRAND-14](../specifications/research-strands.md#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants).
- **Amends:** [REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation) (*RM floor*, *Contract*, and *Roots beyond COMPOSITION*, which holds the retired REQ-102), [REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) (the composition sentence, *Driver*). **Introduces:** [REQ-171](../specifications/clinical-modeling.md#req-171--validation-engine), which carries the mechanics. **Applies:** REQ-102 *Issue paths* (one path per node), [REQ-168](../specifications/clinical-modeling.md#req-168--value-free-validation-diagnostics).
- **Related:** [ADR 0028](0028-generated-rm-attribute-access.md) gives both layers one generated reader, which the one walk stands on.

## Context

The template-driven validators (`ValidateComposition`, `Validate` and the entry points for other roots) check what the operational template constrains, and the RM floor (`ValidateRM`, REQ-112) checks what the Reference Model requires.
Run apart, a caller who ran only the template pass could accept an instance that breaks an RM invariant.
Issue 238 measured the gap on the SDK's corpus: a one-defect copy of BMI per floor rule passed template validation, three corpus compositions carried RM breaches the template pass missed, and 7 of the 26 floor findings across 48 corpus compositions repeated a template finding on the same node under the same code.

Joining the two passes after the fact leaves two defects no comparison can remove.
The floor walks into subtrees the template has refused and reports noise from inside them, since a separate walk does not know what the template refused.
And one defect named under two codes is reported twice: an empty mandatory list draws `required` from the template and `cardinality` from the floor at one path.

Archie, the openEHR Java reference library, checks the RM rules and the template in one walk; EHRbase runs the RM check and then the template check when the first finds nothing.
The SDK keeps collect-all: one run reports every problem, whichever layer finds it.
The set of issues a result carries is what consumers pin, in conformance probes and in a server's validation policy, so the choice is irreversible for them.

## Decision

Template-driven validation includes the RM floor, by contract:

1. **One call.** Every template-driven entry point returns, in one `Result`, both the template findings and the RM findings over every node the template admits, the nodes the template does not constrain included.
2. **Each defect once.** A defect two checks find is reported once, by the check [REQ-171 *One finding per defect*](../specifications/clinical-modeling.md#one-finding-per-defect) names; every finding names its layer and rule.
   The shared path rule of REQ-102 *Issue paths* keeps one path per node, whichever layer reports it.
3. **No template-only option.** Nothing turns the floor off on these entry points; a caller who wants the floor alone calls `ValidateRM`.
4. **Admission first.** At every node the template's admission is decided before any RM rule runs.
   Content the template does not admit, an attribute it prohibits or a list item that binds to no child, draws its one admission finding and is not entered: no RM rule runs on it and no finding comes from inside it ([REQ-171 *Refused content*](../specifications/clinical-modeling.md#refused-content)).
   Content the template does not constrain is visited with the RM rules alone.
5. **One engine, one walk, in `openehr/validation`.** Template-driven validation and the floor run as one engine ([REQ-171 *One walk*](../specifications/clinical-modeling.md#one-walk)): `engine.go` drives the walk, and the template rules and the RM rules are the bodies it dispatches.
   The walk lives beside the rules it applies, not in `internal/templatecompile/walk`, which stays a walker over the compiled template alone and imports nothing of validation.

The binding sentences are REQ-102 *RM floor*, the REQ-112 composition sentence and REQ-171.
A two-walk build that recorded the template's refusals for the floor to honour would meet clauses 1 to 4, at the cost of the bookkeeping clause 5 removes.

## Consequences

- A template-valid but RM-invalid instance fails template validation.
- A result carries no finding from inside content the template refused, and a defect both layers see is one finding by construction.
- The SDK's own generator output stays valid under the combined check (REQ-107), which the corpus ratchet enforces.
- Validation probes that pin issue-code multisets (PROBE-025, PROBE-026, PROBE-074 among them) change only as a reviewed re-baseline, each difference explained as a kind REQ-171's acceptance names.
- Whether an authoring tool may ask for the detail under a refused node stays open until a consumer asks ([STRAND-16](../specifications/research-strands.md#strand-16--validator-policy-beyond-the-limits)); the default is to stop.

## Alternatives considered

- **Keep the template walk and the floor apart, and document both calls.** Rejected: the gap is reached by default.
- **Chain the floor behind an option.** Rejected: off by default it leaves the gap reachable by default, and on by default it exists only to reproduce the gap.
- **Two walks joined by code and path.** Rejected: a second walk cannot honour a refusal it never saw, and it reports one defect under two codes.
- **Two walks, the floor consulting the template's exported refusals.** Rejected: it is the one walk with an extra data structure, and it still reports one defect under two codes.

## Traceability

- Resolves: STRAND-14
- Amends: REQ-102, REQ-112
- Introduces: REQ-171
- Applies: REQ-168
