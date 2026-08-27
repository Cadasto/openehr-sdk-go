# Plan — AQL path-shape lint: the authoring rules the linter does not cover

**Date:** 2026-08-26
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-164 (proposed — AQL semantics band 160–169; prose homed in [clinical-modeling.md](../specifications/clinical-modeling.md) beside REQ-161, authored in Phase 0 via `sdd-specify`). Also lands one **honesty sentence** in the landed [REQ-161](../specifications/clinical-modeling.md#req-161--aql-semantic-and-portability-lint) catalogue (the `aql_fanout_row_grain` scope limit), which is an implementation-aligned doc amendment, not a behaviour change.
**Probes:** PROBE-099 (proposed — *AQL path-shape lint corpus*, in-repo, mirroring PROBE-097's per-code positive + negative-near-miss structure)
**Implementation:** planned
**Depends on:** landed REQ-109 (lint layer model + issue model), REQ-113 (structured paths), REQ-160 (relation, for Phase 4 only), `openehr/rm/rminfo` (multi-cardinality flags + flattened attribute typing — REQ-048/049 surface)
**Defers:** OPT-aware path checking (that is Layer 3, landed); terminology / function-signature checking (REQ-109 § Out of scope, unchanged)

## Goal

Close audit findings **AQL-FIT-04 and -05** (AQL alignment audit, 2026-08-26 — maintainer's
knowledge base, fit-gap report Part 2): the linter's coverage is thorough on containment,
parameters, template membership and the three spec-gap advisories, but **path shape** is an axis
it does not cover at all outside the OPT-gated Layer 3. Four rules that the guidance corpus
states as rules — each checkable from the query text plus the pinned BMM, no OPT, no CDR —
produce zero issues today (verified by execution against v0.22.0). Consumers are the same
callers REQ-109 § Layer 2 serves: anyone with a query string, most valuably in CI over stored
queries.

## The four checks

Proposed codes, all **Warning** severity (portable style / engine-defined behaviour — none is an
impossibility; the conservative-flagging policy of REQ-161 § Flagging policy carries over):

| Code | Fires when | Needs |
|---|---|---|
| `aql_path_repeating_unpredicated` | An identified path steps through a **multi-valued RM attribute** with no predicate on that segment — `o/data/events/data/items/value/magnitude` instead of `…/events[at0006]/…/items[at0004]/…` | `rminfo` walk (Phase 1) |
| `aql_paging_no_order_by` | A row-limited query has no `ORDER BY` — in-text `LIMIT`/`OFFSET`, and the envelope channel when `Options.Query` is supplied (`Fetch`/`Offset` set). Page boundaries are engine-defined without a total order | parse flags only |
| `aql_select_no_alias` | A SELECT item carries no `AS` alias (stored-query contracts depend on stable column names). Not raised for a `*` item — there is nothing to alias | parse only |
| `aql_contains_redundant_step` | An operand that is **unreferenced** (alias roots no path), **predicate-less** (no archetype, standing or version predicate), **not the FROM root, not a leaf**, and whose class is **unavoidable** on every containment path between its parent and its child under the REQ-160 relation — i.e. removing the step provably changes nothing | relation reachability (Phase 4) |

**Firing-rule details to be fixed normatively in Phase 0:**

- **Repeating-segment walk (the anchor check).** Start from the alias's class
  (`parse.Document.Classes`), type each path step through `rminfo`'s flattened
  `AttributeRMType`, test multiplicity with `IsContainer`
  ([`lookup.go`](../../openehr/rm/rminfo/lookup.go)). *Any* predicate on the segment
  suppresses it — node-id, name, standing comparison — carried verbatim in
  `parse.PathSegment` predicate text; presence suffices, content is not judged. The walk is
  **conservative in the Layer-3 style** ([`resolve.go`](../../openehr/aql/lint/resolve.go)'s
  false-positive policy): an unknown class, an attribute `rminfo` cannot type, or a `$param`
  archetype scope ends the walk **silently**. This is deliberately the check that needs no OPT:
  the flags come from the same pinned BMM REQ-160's relation is derived from.
- **Why not judge the predicate content:** whether `[at0006]` is the *right* at-code is a
  template question — Layer 3's job. This check is about the *shape* rule: unconstrained
  repeating segments are ambiguous and engine-dependent.
- **Paging check, envelope arm:** mirrors how the parameter-binding checks already use
  `Options.Query` — nil Query simply means the envelope arm cannot fire; the in-text arm keys
  on `Document.HasLimit && !Document.HasOrderBy`.
- **Redundant-step check:** the general case of REQ-161's `aql_versioned_object_unreferenced`,
  and the reason it must be **narrower** than the guidance sentence ("containment is minimal"):
  an unreferenced *leaf* is an existence filter, and an unreferenced *intermediate* narrows the
  result whenever it is avoidable (`… CONTAINS SECTION s CONTAINS OBSERVATION o` — removing
  `s` admits observations outside sections). Only the **unavoidable** intermediate — e.g.
  `EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o` with `c` unreferenced, where every
  EHR→OBSERVATION containment path passes COMPOSITION — does provably nothing. That is a
  reachability-avoiding-a-vertex question the REQ-160 relation can answer; it needs one new
  query on `contain.TypeRelation` (working name `Unavoidable(anc, via, desc string) bool`).
  This is the most cuttable phase: if review finds the API not worth its weight, the Phase-0
  prose records the rule as out-of-scope with this reasoning, and the plan ships three checks.

**The fan-out path axis (AQL-FIT-05).** The landed `aql_fanout_row_grain` covers one of the two
documented sources of engine-defined row multiplicity (sibling `AND`-junction operands,
SPECQUERY-9). The second — multiple projected paths descending through **different** repeating
scopes — becomes checkable once the Phase-1 walker exists. New sibling code (additive; the landed
code's rule and Detail text stay byte-stable per REQ-161 § Additivity):

| Code | Fires when |
|---|---|
| `aql_fanout_path_grain` | Two projected paths rooted on one alias diverge after their longest common prefix, and **each** passes through at least one unpredicated multi-valued segment at or after the divergence — the Cartesian-product shape. Warning; Detail cites SPECQUERY-9, like its sibling |

Phase 0 also adds one sentence to the REQ-161 `aql_fanout_row_grain` catalogue row naming the
scope boundary ("covers the junction source; the path source is `aql_fanout_path_grain`,
REQ-164") so the boundary is recorded rather than inferred — the immediately-honest half of
AQL-FIT-05, useful even if later phases slip.

## Definition of Ready

- Phase 0 has landed REQ-164 (registry row, canonical prose, traceability `specified`,
  numbering-band note) and the REQ-161 scope sentence.
- The always-on-Warning decision is recorded in the prose: none of these codes flips
  `Result.OK()`, so they are safe-on-by-default; a caller filtering by `Code` opts out
  per-code. (Rejected alternative, recorded: an Options style gate — extra surface for no
  Error-severity risk.)
- The PROBE-028 additivity consequence is pre-computed and recorded: its three cassette queries
  **will** gain codes where they genuinely carry these defects (e.g. an unaliased projection) —
  that is the REQ-161 § Additivity "deliberate, recorded re-baseline" mechanism, applied as
  designed, and the re-baseline list goes in the PROBE-099 catalogue entry.

## Definition of Done

- Code and tests land with `// REQ-164` / `// PROBE-099` citations.
- [`traceability.yaml`](../specifications/traceability.yaml) + REQ.md **Impl.** column updated.
- **The two indexes `make spec-check` cannot see:** a [roadmap.md](../roadmap.md) row for
  REQ-164, and the [REQ.md § Numbering policy](../specifications/REQ.md#numbering-policy) band
  table (164 taken).
- PROBE-099 Implemented (inline): every REQ-164 code fires on a corpus query built to carry
  exactly that defect, with severity and span; every code has a negative near-miss (predicated
  segment; ORDER BY present; aliased projection; an *avoidable* intermediate staying silent);
  the audit's two verified-silent queries from AQL-FIT-04 are corpus rows and now warn.
- PROBE-028 re-baseline (if any) recorded in the catalogue entry, per the additivity guard.
- The four guidance rules and the two row-multiplication sources are each covered by exactly
  one code — no double-reporting with `aql_versioned_object_unreferenced` (an operand
  conforming to `VERSIONED_OBJECT` keeps the REQ-161 code, and the REQ-164 redundant-step
  check skips it).
- `make spec-check` and `make ci` pass.
- Plan archived under [`docs/plans/archive/`](archive/).

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0 — REQ-164 prose + REQ-161 scope sentence + registry (`sdd-specify`) | |
| Indexes `spec-check` misses (`roadmap.md` row, REQ.md numbering band) | |
| Phase 1 — rminfo segment walker + `aql_path_repeating_unpredicated` | |
| Phase 2 — `aql_paging_no_order_by` + `aql_select_no_alias` | |
| Phase 3 — `aql_fanout_path_grain` | |
| Phase 4 — `aql_contains_redundant_step` (cuttable) | |
| PROBE-099 corpus | |
| `make spec-check` / `make ci` | |

## Phases

### Phase 0 — Specify (REQ-164 + the REQ-161 scope sentence)

**Tasks:** author the REQ-164 section (proposed title: *AQL path-shape and paging lint*) with
the five catalogue rows above in the REQ-161 table style (Check / Code / Severity / Rule),
the conservative-walk policy, the always-on decision, the `VERSIONED_OBJECT` skip, and the
Phase-4 cuttability ruling; add the one-sentence scope boundary to the REQ-161
`aql_fanout_row_grain` row; registry row; traceability; numbering band.

**Definition of done:** `make spec-check` passes; no code changes.

### Phase 1 — The segment walker and the anchor check

**Tasks:** a walker in `openehr/aql/lint` (beside [`path.go`](../../openehr/aql/lint/path.go))
that types `lint.Path.Segments` from a class root via `rminfo.Default()` — reusing the flattened
`AttributeRMType` / `IsContainer` surface — and reports, per path, the multi-valued segments
lacking predicates; wire it as a Layer-2 check group (AST walk, no CDR, no OPT — the layer
contract of REQ-109 § Layer 2 holds); emit `aql_path_repeating_unpredicated` with `Span` on the
offending segment and a value-free `Code` (path text goes in `Path`/`Detail` per REQ-109
§ Value-free lint diagnostics). Import direction stays `lint → rm/rminfo` (already allowed —
REQ-161 § Relation supply consults rminfo directly).

**Definition of done:** the audit's verified-silent query
(`SELECT o/data/events/data/items/value/magnitude FROM EHR e CONTAINS OBSERVATION o[…]`) warns
with spans on `events`, `data`, `items`; the predicated spelling stays silent; unknown-class and
untypeable-attribute walks stay silent (pinned negatives); `go test ./openehr/aql/lint/...` green.

### Phase 2 — Paging and projection-alias checks

**Tasks:** `aql_paging_no_order_by` (both channels; the envelope arm plumbed the same way the
parameter-binding checks read `Options.Query`); `aql_select_no_alias` (per unaliased item;
`*` items exempt). Both Warning, both documented in the lint package doc.

**Definition of done:** the audit's silent paging query
(`… LIMIT 50 OFFSET 100` without ORDER BY) warns; the same query with ORDER BY is silent;
envelope-arm positive and negative pinned; alias check positive/negative pinned; `make ci` green.

### Phase 3 — The fan-out path axis

**Tasks:** `aql_fanout_path_grain` on the Phase-1 walker output: group projected paths by alias,
longest-common-prefix comparison, fire once per alias pair meeting the rule; never re-fire the
junction code's shapes (disjoint by construction: this code needs two *paths*, that one two
*aliases*).

**Definition of done:** positive (two paths through different repeating scopes), negatives
(common repeating prefix only; one path predicated at divergence; different aliases — the
junction code's territory) all pinned; `make ci` green.

### Phase 4 — The redundant-step check *(cuttable)*

**Tasks:** `contain.TypeRelation.Unavoidable(anc, via, desc) bool` (reachability recomputed with
`via` excluded — one BFS variant over the existing derived graph, exported with the same
nil-receiver-as-default convention the relation's other methods follow); the
`aql_contains_redundant_step` rule as specified, `VERSIONED_OBJECT`-conforming operands skipped.

**Definition of done:** `EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o` with `c`
unreferenced warns; `… CONTAINS SECTION s …` with `s` unreferenced stays silent (avoidable);
leaves and predicated steps stay silent; `make ci` green. If cut: the Phase-0 out-of-scope
prose ships instead, and this phase's rows move to the plan's **Defers** on archive.

### Phase 5 — PROBE-099 and close-out

**Tasks:** the PROBE-099 catalogue entry + corpus probe under `testkit/probes/aql/` (PROBE-097's
three-arm structure, minus the read/write-parity arm — these codes have no builder analogue);
PROBE-028 re-baseline if triggered, recorded; traceability/REQ.md/roadmap; archive.

**Definition of done:** `make spec-check` and `make ci` pass.

## Mapping to specs

- clinical-modeling.md § REQ-164 — normative contract (authored in Phase 0)
- [clinical-modeling.md § REQ-109](../specifications/clinical-modeling.md#req-109--aql-static-lint) — layer contract + issue model + value-free diagnostics
- [clinical-modeling.md § REQ-161](../specifications/clinical-modeling.md#req-161--aql-semantic-and-portability-lint) — catalogue style, flagging policy, additivity, the amended fan-out row
- [clinical-modeling.md § REQ-160](../specifications/clinical-modeling.md#req-160--aql-containment-admissibility-relation) — the relation behind Phase 4
- [conformance.md § PROBE-097](../specifications/conformance.md#probe-097--aql-semantic-and-portability-lint-corpus) — the corpus-probe pattern PROBE-099 mirrors
- [REQ.md](../specifications/REQ.md) — registry row + numbering band
