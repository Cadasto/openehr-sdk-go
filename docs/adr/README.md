# Architecture Decision Records

Closed architecture decisions for `openehr-sdk-go`. Each ADR is a numbered Markdown file with the standard headings (Status, Context, Decision, Consequences). Status reaches **Accepted** before the ADR is considered closed.

Open decisions (those that would be ADRs once resolved) currently live as research strands in the **Cadasto SDK Specification proposal** (private). When a strand is resolved, an ADR lands here.

| # | Title | Status |
|---|---|---|
| [0001](0001-bmm-version-bump-runbook.md) | BMM version-bump runbook | Accepted (2026-05-16) |
| [0002](0002-bmm-codegen-decisions.md) | BMM code generator structural decisions (D1–D6) | Accepted (2026-05-16) |
| [0003](0003-rm-event-polymorphism.md) | Codec polymorphism for abstract generic RM classes (EVENT, …) | Accepted (2026-05-16) |
| [0004](0004-numeric-wire-tolerance.md) | Strict-encode, permissive-decode for BMM `Real` and `Integer` | Accepted (2026-05-16) |
| [0005](0005-compiled-template-foundation.md) | Compiled OPT foundation (`rminfo` + `internal/templatecompile`) | Accepted (2026-05-22) |
| [0006](0006-composition-validation-walker-placement.md) | Composition validation walker package placement | Accepted (2026-06-11) |
| [0007](0007-aql-antlr-grammar-profile.md) | AQL parser: ANTLR + SDK grammar profile | Accepted (2026-06-15) |

See [docs/architecture.md § Open decisions](../architecture.md#open-decisions) for the strand-to-ADR mapping.
