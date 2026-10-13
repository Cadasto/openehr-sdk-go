---
kind: adr
id: ADR-0027
title: "Template admission precedes the RM rules at every node"
status: proposed
date: 2026-10-13
---

# ADR 0027 — Template admission precedes the RM rules at every node

- **Status:** Proposed, 2026-10-13, after [PR 256](https://github.com/Cadasto/openehr-sdk-go/pull/256); accepted when the maintainer confirms, before the single walk depends on it. Finding identity, which REQ-171 carries and which has landed, is additive and does not depend on this decision.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** none — a direct decision.
- **Amends:** [ADR 0025](0025-template-validation-includes-the-rm-floor.md) clause 1 (the floor's findings "over the whole root", which becomes "over every admitted node"); [REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation) (*RM floor*, *Contract*); [REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) (*Driver*, the shared-value sentence). **Introduces:** [REQ-171](../specifications/clinical-modeling.md#req-171--validation-engine), which carries the mechanics. **Applies:** [REQ-110](../specifications/clinical-modeling.md#req-110--template-driven-validation-beyond-composition), [REQ-168](../specifications/clinical-modeling.md#req-168--value-free-validation-diagnostics).
- **Related:** [ADR 0006](0006-composition-validation-walker-placement.md) placed the walker; [ADR 0028](0028-generated-rm-attribute-access.md) gives both layers one generated reader, which the single walk stands on.

## Context

[ADR 0025](0025-template-validation-includes-the-rm-floor.md) made template-driven validation include the Reference Model floor and left the mechanism open: a template walk followed by a floor walk, with exact repeats removed, or one fused walk. [PR 256](https://github.com/Cadasto/openehr-sdk-go/pull/256) shipped the first. The template walk runs, the floor then runs over the whole root, content the template refused included, and a floor finding is dropped when a template finding already carries the same code at the same path. To make that comparison hold, both walks were taught one path grammar.

Three things follow from that shape, and none of them is a bug in the implementation; they are properties of two walks joined after the fact.

- The floor walks into, and reports from, subtrees the template has refused. The findings inside are noise: the subtree has to go anyway. The maintainer's own objection to the two-walk build is this one, and no comparison after the fact can meet it, because the floor as a separate walk does not know what the template refused.
- The join sees only an exact code at an exact path, so one defect named under two codes is reported twice. An empty mandatory list is the recorded case: `required` from the template walk, `cardinality` from the floor, at one path.
- A finding does not say which layer produced it, and `rm_invariant` is one label for several rules. A server that wants to warn on template findings and reject on RM findings cannot tell them apart in one result, and a caller who needs the rule has to read `Detail`, which the package tells it not to do.

Archie, the openEHR Java reference library, checks the RM rules and the template in one walk over the instance, applying a template constraint wherever one exists. EHRbase runs the RM check and the template check as two stages, the second when the first finds nothing. The SDK keeps collect-all: one run reports every problem, whichever layer finds it.

The fork is irreversible for a consumer, not for the code. The set of issues a result carries is what consumers pin: the conformance probes hold issue-code multisets, and a server's validation policy is built on what a finding says about itself. Taking refused content out of the result changes that set for every consumer at once.

## Decision

At every node, the template's admission is decided before any Reference Model rule is evaluated. Content the template does not admit, an attribute it prohibits or a list item that binds to no child, is reported once and not entered: no RM rule runs on it and no finding comes from inside it. Content the template does not constrain is visited with the RM rules alone. [REQ-171](../specifications/clinical-modeling.md#req-171--validation-engine) carries the walk order and the union rule. The single walk relies on REQ-171's finding identity, the layer and rule every finding names, so it can be compared with the two walks finding by finding before they go.

Clause 1 of [ADR 0025](0025-template-validation-includes-the-rm-floor.md) reads, from this decision on, "over every admitted node, the nodes the template does not constrain included". Clauses 2 and 3 stand as written; clause 2, each defect once, is met by construction rather than by a merge, and the shared path rule of REQ-102 stays for the reader's sake, since one node still has one path whichever layer reports it.

Like ADR 0025, this record binds the contract and not the mechanism. One walk is the implementation it expects, because only one walk can ask the template before spending RM work; a two-walk build that recorded the template's refusals for the floor to honour would meet the contract too, at the cost of the bookkeeping this decision removes.

## Consequences

- A result no longer carries findings from inside content the template refused, and a defect both layers see is one finding by construction. Consumers that counted on either see a smaller result; before 1.0 that ships as a minor version and is named in the release notes as a break.
- The conformance probes that pin issue-code multisets (PROBE-025, PROBE-026, PROBE-074 among them) are re-baselined as a reviewed change, and each difference is explained as one of the kinds REQ-171's acceptance names.
- The merge by code and path, the "visited once" record of the floor and the pinned-name bookkeeping it needed go; the path rule stays.
- Whether an authoring tool may ask for the detail under a refused node is open and stays open until a consumer asks ([STRAND-16](../specifications/research-strands.md#strand-16--validator-policy-beyond-the-limits)); the default is to stop.

## Alternatives considered

- **Keep the two walks and fuse only if profiling demands it.** Cheapest now. Rejected: the reasons to fuse are ordering, finding identity and coverage, not speed; a second walk cannot honour a refusal it never saw.
- **Two public calls, the template walk and the floor, left to the caller to combine.** Rejected by [ADR 0025](0025-template-validation-includes-the-rm-floor.md) already: the gap is reached by default.
- **An option on the two-walk build that drops floor findings under refused subtrees.** Rejected: it needs the template walk to export its refusals and the floor to consult them, which is the fused walk with an extra data structure, and it still reports one defect under two codes.

## Traceability

- **Amends:** ADR-0025 (clause 1), REQ-102, REQ-112
- **Applies:** REQ-171
