# ADR 0017 — AQL semantic layer: derived containment relation, overlays, opt-in enforcement

- **Status:** Accepted, 2026-08-22.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** —
- **Introduces:** [REQ-160](../specifications/clinical-modeling.md#req-160--aql-containment-admissibility-relation) / [REQ-161](../specifications/clinical-modeling.md#req-161--aql-semantic-and-portability-lint) / [REQ-162](../specifications/clinical-modeling.md#req-162--builder-containment-verification). **Amends:** [REQ-109](../specifications/clinical-modeling.md#req-109--aql-static-lint) (Layer 2 gains the REQ-161 check groups; the out-of-scope list narrows to version-predicate *value* semantics — predicate presence is REQ-161's).
- **Plan:** [2026-08-21-aql-semantic-layer.md](../plans/2026-08-21-aql-semantic-layer.md).
- **Related:** [ADR 0007](0007-aql-antlr-grammar-profile.md) (the permissive grammar profile this layer sits above); [REQ-048](../specifications/bmm-conformance.md#req-048--rm-meta-model-introspection-surface) (the `rminfo` class graph it consumes); [REQ-120](../specifications/rm-functions.md#req-120--rm-identifier-parsing-and-derivation) (the canonical HRID parser it delegates to).

## Context

The SDK's AQL surface is syntactically complete (REQ-109/113/117/119) but carries no Reference
Model knowledge: `OBSERVATION o CONTAINS COMPOSITION c` parses, lints clean, and builds, yet can
never match data under the RM. Server-side validation of RM structural legality is not common
practice in current engines — EHRbase, for example, checks containment at storage-root
granularity, so such queries are accepted and return zero rows (observed behaviour;
maintainer's knowledge base, `openehr-kb/notes/ecosystem/ehrbase-aql.md` §4.1.2) — which makes
an empty result set indistinguishable from an impossible query. The QUERY specification is silent on whether an
engine must reject an RM-impossible containment (registered gap AQL-C-009(d) in the maintainer's
knowledge base), on the `VERSION` default predicate (SPECPR-481), and on row semantics for
sibling multiplicity (SPECQUERY-9, open since 2018).

`openehr/rm/rminfo` (REQ-048) already ships the BMM-derived class graph — attribute RM types,
abstractness, conformance, concrete descendants — with AQL CONTAINS conformance named as an
intended consumer. The question is how to wire that knowledge to the AQL surface without
breaking the permissive parser (ADR 0007), the builder's contract, or portability to EHRbase
and other conformant CDRs. Dialects also extend containment beyond the RM — reference-hop
containments such as `FOLDER CONTAINS COMPOSITION`, and Cadasto's demographic extension
(`FROM PERSON … AND EHR e CONTAINS …`, AQL-C-010) — so a closed rule table would misjudge
queries that are meaningful on some targets.

## Decision

**Add one semantic layer — a containment admissibility relation derived at runtime from the
pinned BMM, extended by overlay data, consumed opt-in — and keep every existing surface's
contract unchanged.**

- **Derived, not hand-written.** The relation (REQ-160, `openehr/aql/contain`) is computed from
  `rminfo` at first use (memoized) — the reachability rules themselves are REQ-160
  § Reachability semantics, not restated here. The decision this bullet records is *derivation
  over generation*: no new code generation; a `bmmgen`-emitted table is the rejected
  alternative, revisited only if initialization cost measurably matters.
- **BMM core + overlay edges.** Facts the BMM cannot express — for example the `EHR` root's
  reference-based containment, the family-agnostic `VERSION` tier, or
  `FOLDER → COMPOSITION` as a reference hop — are overlay data with citations, not code
  branches. Consumers can extend the relation with their own overlay edges (the demographic
  dialect safeguard); the default relation expresses the openEHR RM and specifications only,
  never one vendor's storage model. The authoritative edge table, the payload-family rule, and
  the citation carried by each row live in
  [REQ-160 § Overlay edges](../specifications/clinical-modeling.md#req-160--aql-containment-admissibility-relation).
- **Three diagnostic categories, three forces** (REQ-161): RM-impossible, provable from the
  pinned BMM → **Error**; spec-gap portability hazard, citing the open specification question or community
  source where one exists → **Warning**; engine-capability difference → out of scope for the default checks, deferred to
  per-engine profiles. Error-on-impossible is a documented SDK position on the open gap
  AQL-C-009(d), not enforcement of spec text; the flagging policy is conservative and owned by
  REQ-161 § Flagging policy — an unknown name raises the `aql_unknown_rm_class` Warning, never
  silence and never an Error.
- **Row semantics stay out of scope.** The result-shape question for sibling multiplicity is
  unresolved at specification level; the layer may carry one conservative advisory Warning and
  must never adjudicate, deduplicate, or refuse a row shape.
- **Enforcement is opt-in; nothing existing tightens.** The parser keeps accepting everything
  the grammar profile admits (a linter must read foreign, broken queries); `Builder.Build`
  keeps its exact validation set; the semantic checks run as additional lint findings (REQ-161)
  and an explicit builder verification entry point (REQ-162). EHRbase remains a first-class
  target: an RM-valid query that runs on EHRbase must never draw an Error from the default
  relation.

## Consequences

- Statically-impossible containments become visible before a query reaches any CDR — closing
  the "empty result or impossible query?" ambiguity at the client — at the cost of one new
  sub-package and an `openehr/aql → openehr/aql/contain → {rminfo, openehr/rm}` import edge
  (REQ-013-safe; `contain`'s direct imports are `rminfo`, `openehr/rm` — REQ-120's canonical
  `ParseArchetypeID`, no duplicate lexical logic — and stdlib, and it sits below both `aql`
  and `lint` because `lint` already imports `aql`).
- The relation's default verdicts can disagree with a given engine's admissibility in both
  directions; the differences are asserted as neutral, cited, executable documentation
  (PROBE-097 / the REQ-160 acceptance tests), including a compatibility guard for EHRbase.
- A BMM pin bump can change derived verdicts; the REQ-160 acceptance table pins the
  representative rows, so a bump that moves a verdict fails a named test and is triaged like
  any other generated-surface drift (ADR 0001 spirit).
- New lint codes extend the REQ-109 catalogue additively; PROBE-028's corpus gains REQ-161
  codes only where a corpus query genuinely carries a newly-checked defect, as a deliberate,
  recorded re-baseline.
- Later consumers (typed result columns, a template-derived FROM builder, per-engine capability
  profiles, the demographic overlay data) can build on the same relation without reopening this
  decision; each needs its own requirement before implementation.
