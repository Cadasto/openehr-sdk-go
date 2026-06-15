# ADR 0006 — Composition validation walker package placement

- **Status:** Accepted, 2026-06-11.
- **Supersedes:** —
- **Superseded by:** —
- **Tracks:** [`docs/plans/archive/2026-05-24-composition-validation-template-driven.md`](../plans/archive/2026-05-24-composition-validation-template-driven.md) § Deviations #1. Related: [ADR 0005](0005-compiled-template-foundation.md) (OPT-only walker at `internal/templatecompile/walk/`).

## Context

REQ-102 template-driven validation walks compiled OPT nodes in lockstep with composition RM values — emitting issue codes for existence, cardinality, alternatives, identity pinning, and REQ-103 primitive checks at each step.

The REQ-100 follow-up plan introduced a generic OPT walker at `internal/templatecompile/walk/` (`Walk`, `WalkSubtree`, reference visitors). The composition-validation plan initially proposed placing the lockstep composition walker alongside it.

At implementation time the lockstep machinery proved tightly coupled to validation issue emission (alternative disambiguation, `Issue.Path` from compiled AQL paths, primitive dispatch). No other landed consumer needed the same abstraction.

## Decision

The lockstep composition validator **MUST** live in `openehr/validation/` (`walk_composition.go` and related files), **not** in `internal/templatecompile/walk/`.

- `internal/templatecompile/walk/` stays **OPT-only** — compile-time traversal, debug dumps, future tooling that does not import validation semantics ([ADR 0005](0005-compiled-template-foundation.md) Phase 5).
- `WalkComposition` on the internal OPT walker remains **deferred** (`internal/templatecompile/walk/doc.go`) until a second consumer needs shared lockstep machinery.
- If the composition builder or instance generator later need identical lockstep traversal, **extract then** — do not preemptively share before a concrete second call site exists.

Implicit-attribute existence policy (validate BMM-mandatory implicits; deviation #2) and `alternative_mismatch` vs `rm_type_mismatch` disambiguation (deviation #3) are normative in [`clinical-modeling.md` § REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation) — not repeated here.

## Consequences

- **Positive:** Validation concerns and forbidden-import rules (REQ-013) stay in `openehr/validation/`; templatecompile does not depend on validation types.
- **Positive:** OPT walk tooling stays usable without pulling in composition validation.
- **Negative:** Two traversal implementations coexist (OPT walk vs composition lockstep) until an optional future extraction.
