---
kind: adr
id: ADR-0025
title: "Template-driven validation includes the RM floor"
status: accepted
date: 2026-10-11
---

# ADR 0025 — Template-driven validation includes the RM floor

- **Status:** Accepted, 2026-10-11 — maintainer decision, taken with the decode-leniency and validation work for issues 238, 242, 248 and 249.
- **Supersedes:** —
- **Superseded by:** —
- **Amended by:** [ADR 0027](0027-template-admission-before-rm-rules.md) (proposed): clause 1's "over the whole root" becomes "over every admitted node" once that decision is accepted.
- **Strand:** resolves [STRAND-14](../specifications/research-strands.md#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants).
- **Introduces:** —
- **Amends:** [REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation) (*RM floor*), [REQ-110](../specifications/clinical-modeling.md#req-110--template-driven-validation-beyond-composition), [REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) (the composition sentence). **Applies:** REQ-102 *Issue paths* (one path per node, which the single-report rule depends on), [REQ-168](../specifications/clinical-modeling.md#req-168--value-free-validation-diagnostics).
- **Related:** [ADR 0006](0006-composition-validation-walker-placement.md) placed the template walker in `openehr/validation`, beside the floor.

## Context

The template-driven validators (`ValidateComposition`, `Validate` and the other REQ-110 entry points) check what the operational template constrains, and the RM floor (`ValidateRM`, REQ-112) checks what the Reference Model requires. Until now the two composed but did not chain. A caller who ran only the template-driven pass could accept an instance that breaks an RM invariant, and the floor's findings were reached only by a second call. Issue 238 measured the gap on the SDK's own corpus: a one-defect copy of BMI per floor rule passes template validation, three corpus compositions carry RM breaches the template pass misses, and of the 26 floor findings across 48 corpus compositions, 7 repeat a template finding on the same node under the same code.

Three answers were open: chain the floor into the template pass, keep them apart and document both calls, or chain behind an option. An option that is off by default leaves the gap reachable by default, and one that is on by default only exists to reproduce the gap.

## Decision

Template-driven validation includes the RM floor, by contract:

1. **One call.** Every template-driven entry point returns, in one `Result`, both the template findings and the floor's findings over the whole root, the nodes the OPT does not model included.
2. **Each defect once.** Where the two layers report the same code at the same path, the `Result` carries it once. The shared path rule of REQ-102 *Issue paths* makes "the same node" and "the same path" one thing, so no comparison strips predicates or indexes, which would merge siblings.
3. **No template-only option.** Nothing turns the floor off on these entry points. A caller who wants the floor alone still calls `ValidateRM`.

The decision binds the contract, not the mechanism. Running the template walk and then the floor walk and removing exact duplicates meets it, and so does a single fused walk; which one ships is an implementation choice measured against the benchmark harness, and either may replace the other without a new decision. The binding sentences are REQ-102 *RM floor* and the REQ-112 composition sentence.

## Consequences

- A template-valid but RM-invalid instance now fails template validation. That tightens validation, so it is listed as a break in the release notes; before 1.0 it ships as a minor version.
- `Result` grows for an instance that breaks an RM rule; it does not change for one that keeps them.
- The SDK's own generator output must stay valid under the combined check (REQ-107), which the corpus ratchet enforces.
- Validation probes that pin issue-code multisets (PROBE-025, PROBE-026, PROBE-027, PROBE-074 and PROBE-076 among them) are re-baselined as a reviewed change, never skipped.
- The two walks cost less than the decode in front of them at the measured sizes (BMI: decode 106 µs, template 16 µs, floor 66 µs), so the combined check is affordable without fusing the walks.
