# Plan — AQL path-shape lint: the authoring rules the linter does not cover

**Date:** 2026-08-26
**Status:** Done (2026-08-29) — all six phases executed; [REQ-164](../../specifications/clinical-modeling.md#req-164--aql-path-shape-and-paging-lint) `landed` with **five** codes (the cuttable Phase-4 check was kept), PROBE-099 Implemented (inline).
**Owner:** SDK maintainers
**Covers:** [REQ-164](../../specifications/clinical-modeling.md#req-164--aql-path-shape-and-paging-lint) — `landed`; taken from the AQL semantics band 160–169, prose homed in [clinical-modeling.md](../../specifications/clinical-modeling.md) beside REQ-161 and authored in Phase 0 via `sdd-specify`. Also landed one **honesty sentence** in the landed [REQ-161](../../specifications/clinical-modeling.md#req-161--aql-semantic-and-portability-lint) catalogue (the `aql_fanout_row_grain` scope limit), which is an implementation-aligned doc amendment, not a behaviour change.
**Probes:** [PROBE-099](../../specifications/conformance.md#probe-099--aql-path-shape-lint-corpus) — *AQL path-shape lint corpus*, in-repo, Implemented (inline), mirroring PROBE-097's per-code positive + negative-near-miss structure minus its read/write-parity arm. [PROBE-028](../../specifications/conformance.md#probe-028--aql-lint-stability) took a deliberate re-baseline, recorded in the PROBE-099 entry.
**Implementation:** landed
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
  ([`lookup.go`](../../../openehr/rm/rminfo/lookup.go)). *Any* predicate on the segment
  suppresses it — node-id, name, standing comparison — carried verbatim in
  `parse.PathSegment` predicate text; presence suffices, content is not judged. The walk is
  **conservative in the Layer-3 style** ([`resolve.go`](../../../openehr/aql/lint/resolve.go)'s
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
  This was the most cuttable phase: had review found the API not worth its weight, the Phase-0
  prose would have recorded the rule as out-of-scope with this reasoning and the plan would have
  shipped three checks here. **Review kept it** — the query landed as
  `contain.TypeRelation.Unavoidable` with the working name unchanged, and the resolved fork is
  recorded in REQ-164 § The redundant-step ruling rather than erased.

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
- [`traceability.yaml`](../../specifications/traceability.yaml) + REQ.md **Impl.** column updated.
- **The two indexes `make spec-check` cannot see:** a [roadmap.md](../../roadmap.md) row for
  REQ-164, and the [REQ.md § Numbering policy](../../specifications/REQ.md#numbering-policy) band
  table (164 taken).
- PROBE-099 Implemented (inline): every REQ-164 code fires on a corpus query built to carry
  exactly that defect, with severity and span; every code has a negative near-miss (predicated
  segment; ORDER BY present; aliased projection; an *avoidable* intermediate staying silent);
  the audit's two verified-silent queries from AQL-FIT-04 are corpus rows and now warn.
- PROBE-028's re-baseline recorded in the catalogue entry, per the additivity guard: `valid.aql`
  and `missing_archetype.aql` each gained exactly `aql_select_no_alias`, `bad_syntax.aql` nothing.
- The four guidance rules and the two row-multiplication sources are each covered by exactly
  one code — no double-reporting with `aql_versioned_object_unreferenced` (an operand
  conforming to `VERSIONED_OBJECT` keeps the REQ-161 code, and the REQ-164 redundant-step
  check skips it).
- `make spec-check` and `make ci` pass.
- Plan archived under [`docs/plans/archive/`](./).

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0 — REQ-164 prose + REQ-161 scope sentence + registry (`sdd-specify`) | ✅ landed 2026-08-27 (`bfa730d`, `fe6b387`, `3684951`) |
| Indexes `spec-check` misses (`roadmap.md` row, REQ.md numbering band) | ✅ numbering band in Phase 0; `roadmap.md` row in Phase 5 |
| Phase 1 — rminfo segment walker + `aql_path_repeating_unpredicated` | ✅ landed 2026-08-28 (`a727e86`) |
| Phase 2 — `aql_paging_no_order_by` + `aql_select_no_alias` | ✅ landed 2026-08-28 (`d775ec4`) — carries the PROBE-028 re-baseline |
| Phase 3 — `aql_fanout_path_grain` | ✅ landed 2026-08-28 (`ca22d61`, `c7735b8`) |
| Phase 4 — `aql_contains_redundant_step` (specified cuttable; **kept**) | ✅ landed 2026-08-28 (`c9b81e0` the `Unavoidable` query, `cf2980b` the check, `7318ba2` the doc-count corrections) |
| Phase 5 — PROBE-099 corpus + close-out | ✅ landed 2026-08-29 |
| `make spec-check` | ✅ OK at every phase |
| `make ci` | ✅ green on the host at the end of Phase 5 (`fmt-check`, `vet`, `go test ./... -count=1`, `golangci-lint run ./...` — 0 issues, `spec-check`, `flat-conformance-verify`, `build`) |

## Phases

### Phase 0 — Specify (REQ-164 + the REQ-161 scope sentence)

**Tasks:** author the REQ-164 section (proposed title: *AQL path-shape and paging lint*) with
the five catalogue rows above in the REQ-161 table style (Check / Code / Severity / Rule),
the conservative-walk policy, the always-on decision, the `VERSIONED_OBJECT` skip, and the
Phase-4 cuttability ruling; add the one-sentence scope boundary to the REQ-161
`aql_fanout_row_grain` row; registry row; traceability; numbering band.

**Definition of done:** `make spec-check` passes; no code changes.

### Phase 1 — The segment walker and the anchor check

**Tasks:** a walker in `openehr/aql/lint` (beside [`path.go`](../../../openehr/aql/lint/path.go))
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

**As landed:** that query warns **once**, on `events` alone. `EVENT.data` is the BMM generic
parameter `T` on the pinned tables and REQ-048 does not resolve it, so the conservative walk stops
there and nothing below it — `data`, `items` — is judged at all. The three-span expectation above
was written before the pin's reach was measured; the bound is now normative in REQ-164 § The
conservative segment walk, and widening it is a BMM-band change rather than a change to this
check.

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

**As landed:** the phase was **not** cut. Review judged the one new relation query worth its
weight, so `contain.TypeRelation.Unavoidable` shipped, `aql_contains_redundant_step` is allocated
and REQ-164 carries five codes; the out-of-scope branch is recorded as the road not taken in
REQ-164 § The redundant-step ruling and § Out of scope, and nothing moved to **Defers**.

### Phase 5 — PROBE-099 and close-out

**Tasks:** the PROBE-099 catalogue entry + corpus probe under `testkit/probes/aql/` (PROBE-097's
three-arm structure, minus the read/write-parity arm — these codes have no builder analogue);
PROBE-028 re-baseline if triggered, recorded; traceability/REQ.md/roadmap; archive.

**Definition of done:** `make spec-check` and `make ci` pass.

**As landed:** the probe is
[`testkit/probes/aql/probe_099_path_shape_lint.go`](../../../testkit/probes/aql/probe_099_path_shape_lint.go),
two arms — the per-code firing / near-miss corpus and the PROBE-028 additivity guard. Three firing
rows and fifteen negatives are claimed **by name** so the corpus cannot shrink below the wire
assertion, and each of those guards has an able-to-fail control that mutates the shipping corpus.
The re-baseline was triggered and is recorded in the PROBE-099 catalogue entry.

## Mapping to specs

- clinical-modeling.md § REQ-164 — normative contract (authored in Phase 0)
- [clinical-modeling.md § REQ-109](../../specifications/clinical-modeling.md#req-109--aql-static-lint) — layer contract + issue model + value-free diagnostics
- [clinical-modeling.md § REQ-161](../../specifications/clinical-modeling.md#req-161--aql-semantic-and-portability-lint) — catalogue style, flagging policy, additivity, the amended fan-out row
- [clinical-modeling.md § REQ-160](../../specifications/clinical-modeling.md#req-160--aql-containment-admissibility-relation) — the relation behind Phase 4
- [conformance.md § PROBE-097](../../specifications/conformance.md#probe-097--aql-semantic-and-portability-lint-corpus) — the corpus-probe pattern PROBE-099 mirrors
- [REQ.md](../../specifications/REQ.md) — registry row + numbering band
