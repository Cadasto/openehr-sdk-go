# Plan — AQL write-side parity: the builder can express what the parser can read

**Date:** 2026-08-26
**Status:** Complete
**Owner:** SDK maintainers
**Covers:** [REQ-163](../../specifications/clinical-modeling.md#req-163--aql-write-side-expressivity-parity) — `landed`; 163 taken from the AQL semantics band 160–169, prose homed in [clinical-modeling.md](../../specifications/clinical-modeling.md) beside REQ-160..162 and authored in Phase 0 via `sdd-specify`. Amends nothing normative in landed REQs — but see § One divergence worth recording below, where REQ-163 amended its own section; extends the fixture sets of [PROBE-088](../../specifications/conformance.md#probe-088--aql-builder-containment-and-paging-stability) and the arm-(c) corpus of [PROBE-097](../../specifications/conformance.md#probe-097--aql-semantic-and-portability-lint-corpus).
**Probes:** no new probe id — PROBE-088 (builder golden stability) gained fourteen goldens and four refusal rows, PROBE-097 arm (c) (read/write parity) four rows for the new constructs; PROBE-020's golden stayed untouched.
**Implementation:** landed
**Depends on:** landed REQ-055 / 117 / 118 / 119 (builder + canonicalisation), REQ-113 (the read-side vocabulary being mirrored), REQ-160..162 (relation + verification)
**Defers:** a FROM-root standing predicate (REQ-055 rule 6 keeps the `WHERE e/ehr_id/value = $param` form deliberately); a FROM-root archetype predicate (separately unreachable, recorded in PROBE-097); a `TOP $n` parameter (the grammar admits none, REQ-118)

## Goal

Close audit findings **AQL-FIT-01, -02, -03** (AQL alignment audit, 2026-08-26 — maintainer's
knowledge base, ecosystem fit-gap report Part 2): give `openehr/aql`'s write side the three
carriers its own read side already models, so the whole version-query family stops being
string-only. Consumers are builder callers who today must abandon the typed builder for any
query with a `VERSION` predicate, a standing class predicate, or a typed projection
(`DISTINCT` / `AS` / aggregates).

## Why this is one plan

The three gaps are one defect seen from three clauses: **the write side is narrower than the
read side.** Every carrier this plan adds mirrors a vocabulary that already exists and is
already tested on the read side (`parse.ClassExpr.Predicate` / `.PredicateComparison`,
`parse.SelectClause` / `SelectItem` / `SelectExpr`), so the design work is mostly done. The
concrete failure today, verified by execution against v0.22.0:

- `aql.Class("VERSION", "v")` is the **only** VERSION shape the builder can emit — exactly the
  shape the SDK's own `aql_version_no_predicate` warns on (SPECPR-481). All three routes to the
  predicate are refused ([`containment.go`](../../../openehr/aql/containment.go) rejects an
  archetype predicate on VERSION; identifier validation rejects smuggling via alias or RM type).
- REQ-161's own documented suppression shape —
  `… CONTAINS VERSIONED_COMPOSITION vo[uid/value=$vo] CONTAINS VERSION v[ALL_VERSIONS]`,
  the canonical *all versions of one composition* query — cannot be built: `Containment`
  carries only an archetype HRID (or `$param`) in its bracket.
- `Col(path)` writes the projection **verbatim and unchecked**
  ([`builder.go:215-218`](../../../openehr/aql/builder.go)); SELECT is the one clause REQ-119's
  write-path hardening did not reach. `DISTINCT`, `AS` and aggregates are expressible only by
  smuggling text through `Col`.

## Definition of Ready

Implementation may start when:

- **Phase 0 has landed REQ-163**: registry row in [REQ.md](../../specifications/REQ.md), canonical
  prose in [clinical-modeling.md](../../specifications/clinical-modeling.md), a
  `traceability.yaml` entry (`status: specified`), and the numbering-policy band note
  (163 consumed from the AQL semantics band).
- The two API forks below are settled in the REQ prose (not silently in code):
  1. **`Col` leniency.** `Col("COUNT(x) AS n")` works today. Refusing it is a behaviour break;
     keeping it unchecked leaves REQ-119's substitution class open. Proposal: keep `Col` and add
     build-time **structural verification** — the built query is re-parsed once and the SELECT
     item count and clause structure must match what the builder recorded (a `Col` whose text
     splits into two items, or spills into another clause, is refused wrapping
     `aql.ErrInvalidQuery`); a `Col` that re-parses as a *single* function call or aliased item
     is tolerated as legacy. This applies REQ-119's own rule: refusal is reserved for the
     silent-substitution mode, loud malformation stays loud.
  2. **Standing predicates on VERSION nodes.** `versionPredicate` admits `standardPredicate`,
     so a comparison on a VERSION node is legal AQL. Proposal: route it through the
     version-predicate carrier (`VersionCompare`, below) and refuse `Predicated` on a
     VERSION-spelled node with an error naming the right constructor — one carrier per grammar
     position, mirroring the two-guard split [`predicate.go`](../../../openehr/aql/predicate.go)
     already documents.

## Definition of Done

- Code and tests land with `// REQ-163` citations (and `// PROBE-088` / `// PROBE-097` where
  fixtures grow).
- [`traceability.yaml`](../../specifications/traceability.yaml) and the REQ.md **Impl.** column
  reflect the implementation. What landed: REQ-163's entry moved `planned` → `landed`, gained a
  nine-file test list and `testkit/probes/aql` beside `openehr/aql` in `packages:`, and its notes
  record the fourth standing-carrier refusal and the two called-out behaviour changes; REQ.md's
  row moved `planned` → `landed`. No other REQ entry moved.
- **The two indexes `make spec-check` cannot see:** a [roadmap.md](../../roadmap.md) row for
  REQ-163, and the [REQ.md § Numbering policy](../../specifications/REQ.md#numbering-policy) band
  table updated (163 taken in the 160–169 AQL semantics band).
- The canonical builder golden set under
  [`openehr/aql/testdata/wire/`](../../../openehr/aql/testdata/wire/) covers every new construct;
  PROBE-020's existing golden is byte-unchanged (additivity). Fourteen new files —
  `version_*` ×3, `standing_*` ×3, `select_*` ×8 — all of them new names, so nothing was
  re-baselined.
- A builder-emitted `CONTAINS VERSION v[LATEST_VERSION]` query round-trips:
  `Build → parse.ParseQuery → Emit` is byte-identity, and `lint.LintString` raises **no**
  `aql_version_no_predicate` on it.
- The REQ-161 suppression shape is built end-to-end and PROBE-097 arm (c) exercises it —
  read/write parity now covers rows that were "builder-inexpressible" in the landed corpus.
- `make spec-check` and `make ci` pass (run `golangci-lint` on the touched packages directly if
  the Docker image is unavailable).
- Plan archived under [`docs/plans/archive/`](./).

**One divergence worth recording.** The plan's Covers line promised the section would amend
nothing normative, and it amended **itself**: Phase 2 built a fourth standing-carrier refusal —
a second `Predicated` call on one node — that the Phase-0 prose had not enumerated, so Phase 4
corrected § The standing-predicate carrier from three refusals to four and added the matching
§ Acceptance row. The correction runs implementation → spec, which is the direction this repo
prefers for a rule the code was always going to need; no landed REQ outside REQ-163 moved.
A related fork in the draft's own text resolved the same way: Phase 0's prose had suggested the
[REQ-055](../../specifications/wire.md#req-055--wire-boundary) canonicalisation rule set would
gain the new spellings, and a main-branch correction to the draft recorded that reading — the
execution instead homed every new canonical spelling in REQ-163 itself (the controller's
pre-flight ruling: one home per fact, beside the construct), leaving REQ-055 untouched apart
from one non-normative pointer sentence in wire.md.

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0 — REQ-163 prose + registry + traceability (`sdd-specify`) | ✅ landed 2026-08-27 (`9193231`, `a50921a`) |
| Indexes `spec-check` misses (`roadmap.md` row, REQ.md numbering band) | ✅ numbering band in Phase 0; `roadmap.md` row in Phase 4 |
| Phase 1 — version-predicate carrier | ✅ landed 2026-08-27 (`5bd9f10`, `901feec`) |
| Phase 2 — standing-predicate carrier | ✅ landed 2026-08-27 (`84c995c`, `5ea9183`) |
| Phase 3 — typed projection + `Distinct()` + structural verification | ✅ landed 2026-08-27 (`8e35c03`, `d79d313`) |
| Phase 4 — probes, goldens, docs | ✅ landed 2026-08-27 — PROBE-088 +14 goldens / +3 refusals, PROBE-097 arm (c) +4 rows, `doc.go`, conformance.md, the fourth-refusal spec correction, CHANGELOG + indexes |
| `make spec-check` | ✅ OK at every phase |
| `make ci` | ✅ green on the host at the end of Phase 4 (`fmt-check`, `mod-tidy-check`, `vet`, `go test ./... -count=1`, `golangci-lint run ./...` — 0 issues, `spec-check`, `flat-conformance-verify`, `build`) |

## Phases

### Phase 0 — Specify (REQ-163)

**Tasks:** author the REQ-163 section in clinical-modeling.md (proposed title: *AQL write-side
expressivity parity*), covering: the sealed `VersionPredicate` vocabulary; the standing-predicate
carrier and its one-comparison bound (`standardPredicate` admits exactly one comparison — no
junction alternative); the typed projection model and its read-side mirror duty
(`parse.SelectClause` is the reference shape); the `Col` leniency ruling and the structural
verification rule (DoR fork 1); the VERSION routing ruling (DoR fork 2); canonicalisation
additions to the REQ-055 rule set (bracket spelling: keywords uppercase, one space around the
comparison operator — matching what `parse.Query.Emit` already produces so round-trip identity
holds). Registry row, traceability entry, numbering-band note.

**Definition of done:** `make spec-check` passes; no code changes.

### Phase 1 — Version-predicate carrier

**Tasks:**

- New sealed type in `openehr/aql`: `VersionPredicate` (unexported marker method), with
  constructors `LatestVersion()`, `AllVersions()`, and
  `VersionCompare(path string, op Operator, v Value)` — the three alternatives of the grammar's
  `versionPredicate : LATEST_VERSION | ALL_VERSIONS | standardPredicate`, reusing the landed
  `aql.Operator` / `aql.Value` vocabulary.
- New constructor `aql.Version(alias string, pred VersionPredicate) Containment` (fixed RM type
  `VERSION`; `Version("v", nil)` ≡ `Class("VERSION", "v")`, the predicate-less form staying
  legal). `Containment` gains an unexported version-predicate field beside `archetypeID`.
- `validateTree` ([`containment.go`](../../../openehr/aql/containment.go)): the field is refused on
  a non-VERSION node; the existing VERSION-archetype refusal stays; the rendered bracket text is
  belt-and-braces validated through the landed `aql.ValidateVersionPredicate`
  ([`predicate.go`](../../../openehr/aql/predicate.go)) — which finally gains its write-side caller.
- Emission: after the alias, `[` + canonical spelling + `]`.
- Tests: all three predicate forms build, emit, re-parse to identity; the refusals
  (junction receiver, non-VERSION node, malformed comparison) each pinned; the built
  `[LATEST_VERSION]` query produces zero `aql_version_no_predicate` findings via
  `lint.LintString`.

**Definition of done:** `go test ./openehr/aql/...` green; new goldens under
`openehr/aql/testdata/wire/`; PROBE-020 golden unchanged.

### Phase 2 — Standing-predicate carrier

**Tasks:**

- New method `(c Containment) Predicated(path string, op Operator, v Value) Containment` —
  one `standardPredicate` comparison in class position, stored as the landed `aql.Comparison`
  shape (`{Path, Op, Val}`) so the WHERE clause, the parsed class predicate
  (`parse.ClassExpr.PredicateComparison`) and the built class predicate share **one** comparison
  model.
- Refusals via the `invalid`-field pattern `withChild` already uses: junction receiver; a node
  that already carries an archetype predicate (`pathPredicate` is one alternative, not a
  conjunction); a VERSION-spelled node (routed to `Version(...)` per the Phase-0 ruling).
- Path position validated with the same guard class the predicate positions already have
  (`ValidatePathPredicate` on the rendered bracket text); the value renders through the
  REQ-119 canonical value spellings.
- Tests: the REQ-161 motivating shape builds —
  `Class("VERSIONED_COMPOSITION","vo").Predicated("uid/value", aql.OpEq, /* $vo */ …)`
  `.Contains(Version("v", AllVersions()))` — emits the canonical text, re-parses to identity,
  and `lint.LintString` stays silent on `aql_versioned_object_unreferenced` (the suppression
  now reachable from the builder); `(*Builder).VerifyContainment` agrees with the linter on the
  containment-code subset for the same query (REQ-162 § Contract unchanged — portability codes
  stay lint-only).

**Definition of done:** the end-to-end suppression test passes; refusals pinned; `make ci` green.

### Phase 3 — Typed projection

**Tasks:**

- `SelectField` grows constructors mirroring `parse.SelectExpr`:
  `ColAs(path, alias string)`, `Count(path string)` / `CountStar()` /
  `Fn(name string, args ...SelectField)` (aggregate + function calls, `FunctionCall` mirror),
  `Lit(v Value)` (literal projection, `LiteralExpr` mirror), and `(SelectField) As(alias string)`
  for aliasing any item. `(*Builder) Distinct()` sets the clause flag
  (`SELECT DISTINCT? top? selectExpr …` — flag precedes the existing TOP emission).
- `Col(path)` unchanged in signature and leniency (Phase-0 ruling); typed constructors validate
  their identifier positions the way FROM/CONTAINS/ORDER BY already do.
- **Structural verification at `Build()`** (the Phase-0 rule): one re-parse of the emitted
  string; refuse on SELECT item-count mismatch or clause spill, wrapping `aql.ErrInvalidQuery`.
  This closes the last REQ-119 write path for *every* projection, `Col` included.
- Canonicalisation: `DISTINCT` emitted directly after `SELECT`; `AS` uppercase, one space each
  side; function names emitted uppercase (matching the read-side `FunctionCall.Name` contract).
- Tests: each constructor builds/emits/re-parses to identity; `Distinct()` + `Top(n)` compose
  (grammar order pinned); the smuggling shapes from the audit (`Col("COUNT(c/uid/value) AS n")`,
  `Col("DISTINCT c/uid/value")`) now either round-trip as recorded structure or are refused per
  the verification rule — pinned either way; a `Col("a, b")` split is refused.

**Definition of done:** `go test ./openehr/aql/...` green; goldens extended; verb-functions
(`aql.Select(...)`) and struct-builder emit byte-identical strings for the new constructs
(PROBE-020's property held over the additions).

### Phase 4 — Probes, goldens, docs

**Tasks:** extend PROBE-088's fixture set with the new constructs (its Preconditions already
anticipate "golden fixtures … covering the new constructs"); extend PROBE-097 arm (c)'s corpus
with the now-buildable rows and update its catalogue prose (the "builder-inexpressible" caveat
narrows); update [`openehr/aql/doc.go`](../../../openehr/aql/doc.go) examples; conformance.md
catalogue rows re-worded where they state the old unreachability; traceability + REQ.md Impl.
column + roadmap row.

**Definition of done:** `make spec-check` and `make ci` pass; plan archived.

## Mapping to specs

- clinical-modeling.md § REQ-163 — normative contract (authored in Phase 0)
- [clinical-modeling.md § REQ-113](../../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) — the read-side vocabulary mirrored
- [clinical-modeling.md § REQ-119](../../specifications/clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) — the write-path closure rule extended to SELECT
- [clinical-modeling.md § REQ-161](../../specifications/clinical-modeling.md#req-161--aql-semantic-and-portability-lint) / [REQ-162](../../specifications/clinical-modeling.md#req-162--builder-containment-verification) — the suppression shape and parity contract this makes reachable
- [wire.md § REQ-055](../../specifications/wire.md#req-055--wire-boundary) — canonicalisation rule set gaining the new spellings
- [REQ.md](../../specifications/REQ.md) — registry row + numbering band
