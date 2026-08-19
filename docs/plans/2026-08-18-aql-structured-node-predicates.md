# Plan — AQL structured node predicates (typed path-segment predicate model)

**Date:** 2026-08-18
**Status:** Complete
**Owner:** SDK maintainers
**Covers:** **[REQ-113](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) § Structured node predicates** — the spec text is written; the code is not. No new requirement id: this extends the landed REQ-113 read AST, which already owns `PathSegment` and the class-predicate structuring.
**Verifies / builds on:** landed [REQ-113](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) (structured read AST; `aql.IdentifiedPath`/`PathSegment`, `ClassExpr.PredicateComparison`), [REQ-117](../specifications/clinical-modeling.md#req-117--aql-expression-catalogue-completion) (expression-catalogue completion), [REQ-119](../specifications/clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) (re-parseable emission; verbatim predicate text; `aql.StripPredicateTrivia`)
**Probes:** **[PROBE-095](../specifications/conformance.md#probe-095--aql-predicate-structuring)** (Draft) — a corpus generated from the grammar's `pathPredicate` alternatives
**Implementation:** landed
**Depends on:** `openehr/aql`, `openehr/aql/parse` (extractor), the REQ-119 verbatim-text guarantee (unchanged by this plan)
**Defers:** the **class position** — `parse.ClassExpr` keeps its landed REQ-113 carriers (`Archetype` / `ParamArchetype` / `Predicate` / `PredicateComparison`) and gains no parallel one; a Phase 0 decision, with the reasoning in the canonical section. A carrier for VERSION class predicates (`LATEST_VERSION` / `ALL_VERSIONS`) — a version-query-shaped ask that should ride a version-query slice. Full `nodePredicate` sub-grammar *validation* (the REQ-119 § Out of scope deferral for issue #99 stands untouched: structuring what the parser already accepted is not deciding what to accept). Rewriting or normalising `Raw` (REQ-119 keeps it verbatim). A disclosure policy over the components — the point is that each consumer applies its own; the SDK's own diagnostics are [REQ-113 § Value-free structured drop records](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast)'s.

## Goal

Give every bracketed **path-segment** predicate the treatment the standing class predicate got in
REQ-113: a **typed model beside the raw text**, so a reader never has to lex predicate text again.
Today `aql.PathSegment.Predicate` is a raw string, and the only structured predicate anywhere is
the standing comparison at the class position (`ClassExpr.PredicateComparison`). Everything else —
`[at0001]`, `[id3]`, `[at0001, 'Systolic']`, `[at0001 and name/value=$name]`, an archetype HRID in
a path segment, a `$param` — arrives as text the reader must re-tokenize. The class position stays
as it is (see **Defers:**): it is already decomposed into three landed fields, so it does not carry
the burden this plan removes.

## Motivation

Three converging pieces of evidence, all from this repository's own history:

1. **v0.20.0 (REQ-119) made predicate text verbatim, and every downstream
   comparison of that text is exposed to trivia.** An engine matching segment
   predicates against compiled-OPT node ids that coped with collapsed text via
   `strings.TrimSpace` breaks on a predicate carrying a `-- comment`: the at-code
   arrives with the comment attached and matches no node — a wire-visible answer
   change caused by nothing but a toolchain bump. `aql.StripPredicateTrivia` was
   exported for exactly this reason: *"every consumer that compares that text
   against something needs the same trivia model."* The stronger conclusion is
   that no trivia model should be needed at all: the lexer already knows where
   the at-code ends; the AST should say so.
2. **`aql.RedactPredicateValues` cannot decide what a value is for every reader.**
   It treats numeric literals as structure; a disclosure rule that treats a bare
   magnitude as a value cannot adopt it. With predicate *components* typed, each
   reader applies its own disclosure policy per component — no shared text-level
   helper has to guess.
3. **The class-predicate splice work (issue #99, archived plan 2026-08-08) already
   treats the bracketed predicate as a sub-grammar** — guarded positions, verbatim
   round-trip — establishing that the position *has* a structure worth naming. It
   deliberately stopped short of a full `nodePredicate` **validator**, and this
   plan does **not** close that deferral: structuring what the parser already
   accepted is a different job from deciding what to accept, and the deferral is
   recorded at the class position, which this plan leaves alone. What carries over
   is the precedent that the position is a sub-grammar rather than an opaque
   string.

Every future widening of downstream predicate support (name predicates,
`name/value` standing forms, terminology-coded names) multiplies the re-lexing
sites if the carrier stays raw text.

## Architecture

Follow the package's sealed-interface style (`aql.Value`, `parse.SelectExpr`). The normative
rules — the kinds, the trivia/delimiter contract, the unstructured-set enumeration, the
comparability restriction, the class-position scope, and the REQ-119 non-interference clause —
live in the [canonical section](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast)
and are **not** restated here. What this section carries is the implementation shape and the two
grammar facts Phase 0 surfaced, which change the sketch this plan was drafted with.

### The bracket is three productions (Phase 0 finding)

The draft sketch enumerated the kinds against `nodePredicate`. That production is only one of
three alternatives the bracketed position admits:

```antlr
pathPredicate      : SYM_LEFT_BRACKET (standardPredicate | archetypePredicate | nodePredicate) SYM_RIGHT_BRACKET ;
standardPredicate  : objectPath COMPARISON_OPERATOR pathPredicateOperand ;
archetypePredicate : ARCHETYPE_HRID | PARAMETER ;
nodePredicate      : (ID_CODE | AT_CODE) (SYM_COMMA (STRING | PARAMETER | TERM_CODE | AT_CODE | ID_CODE))?
                   | ARCHETYPE_HRID      (SYM_COMMA (STRING | PARAMETER | TERM_CODE | AT_CODE | ID_CODE))?
                   | PARAMETER
                   | objectPath COMPARISON_OPERATOR pathPredicateOperand
                   | objectPath MATCHES CONTAINED_REGEX
                   | nodePredicate AND nodePredicate
                   | nodePredicate OR  nodePredicate ;
```

A comparison, a bare HRID and a bare `$param` are each spelled **twice** — once as their own
top-level alternative, once inside `nodePredicate` — so which parse-tree node carries a form
depends on whether it sits at the top of the bracket or nested as a junction operand. The
extractor therefore populates the same kind from more than one context type, and PROBE-095
presents every form in **both** positions. The class position already splits this way in landed
code ([`extract_query.go`](../../openehr/aql/parse/extract_query.go) switches on
`pp.ArchetypePredicate()` then falls through to `pp.StandardPredicate()`); the segment position
([`ast.go`](../../openehr/aql/parse/ast.go)) takes raw text and is what this plan structures.

### The name slot is wider than the sketch (Phase 0 finding)

`(SYM_COMMA (STRING | PARAMETER | TERM_CODE | AT_CODE | ID_CODE))?` hangs off **both** the node-id
and the archetype-HRID alternatives. So the draft's `ArchetypePredicate struct{ HRID string }`
cannot carry `[openEHR-EHR-OBSERVATION.blood_pressure.v1, 'Systolic']`, and a name carrier
modelling only "plain string vs coded" drops three of five spellings — one of which, `PARAMETER`,
is not a name but a deferral of the name to bind time, which a consumer resolving names against a
template has to tell apart from a name it can resolve now.

### Shape

```go
// On aql.PathSegment only — the class position keeps its landed REQ-113
// carriers (Phase 0 decision, recorded in the canonical section).
//   Parsed SegmentPredicate   // nil only for a form the spec enumerates
// Raw stays verbatim (REQ-119); Parsed is a read-side derivation.

type SegmentPredicate interface{ segmentPredicate() }

type NodeIDPredicate     struct{ ID string; Name *PredicateName }   // [at0001] / [at0001, 'Systolic']
type ArchetypePredicate  struct{ HRID string; Name *PredicateName } // name slot per the finding above
type ParamPredicate      struct{ Name string }                      // [$param]
type ComparisonPredicate struct{ Comparison Comparison }            // the standing form
type MatchesPredicate    struct{ Path string; Regex string }        // objectPath MATCHES {regex}
type JunctionPredicate   struct{ Op JunctionOp; Left, Right SegmentPredicate }

// PredicateName discriminates all five spellings the name slot admits,
// including the $param deferral.
type PredicateName struct{ Kind PredicateNameKind; Text string /* + term-code parts */ }
```

`ComparisonPredicate` embeds `aql.Comparison`, whose `Val` is an `aql.Value` that panics under
`==` — so the sum and every kind embedding it are not `==`-comparable and not map keys, and the
package ships `EqualPredicates` beside the landed `aql.EqualValues`. The builder side
(`aql.Builder`) is untouched: it already writes predicates from typed inputs.

## Definition of Ready

**Phase 0 has landed, so implementation (Phase 1+) may start.** Each item below is satisfied:

- ✅ Canonical normative prose exists — the kinds, the trivia/delimiter rules, the unstructured-set enumeration, the comparability restriction, the class-position scope, and the REQ-119 non-interference clause: [clinical-modeling.md § REQ-113 § Structured node predicates](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast). The kinds were enumerated **against the vendored grammar**, which is where the two findings in § Architecture came from.
- ✅ [PROBE-095](../specifications/conformance.md#probe-095--aql-predicate-structuring) defined (`Status: Draft`): a corpus **generated from** the `pathPredicate` alternatives, every form presented at bracket top level *and* as a junction operand, trivia/escape independence asserted as a property, the five name spellings on both carrying alternatives, and a sweep that fails when the grammar admits a form no kind covers.
- ✅ Negative space pinned: an unstructured form reports itself unstructured with `Raw` intact and **never** a partial structure; the enumerated unstructured set is empty for the segment position; the class position is unchanged.
- ✅ Each phase names its verification command.

## Definition of Done

- `openehr/aql` + `openehr/aql/parse` land the model with `// REQ-` citations;
  the extractor populates `Parsed` on every **path-segment** predicate, in every
  clause whose paths carry one (SELECT / WHERE / ORDER BY, and a predicate on the
  alias root). The class position is untouched (§ Defers).
- `aql.StripPredicateTrivia` and `aql.RedactPredicateValues` docs updated to point
  at the structured model as the preferred consumption path (both stay — text
  consumers exist).
- `traceability.yaml` + REQ.md **Impl.** column; `roadmap.md` row; CHANGELOG (a
  consumer-visible additive surface).
- REQ-119 emission parity asserted, not assumed: emitted text byte-identical.
- `make spec-check` and `make ci` green; plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| Spec text in REQ-113 + PROBE-095 (Phase 0) | ✅ |
| Sum type + `EqualPredicates` + extractor population | ✅ |
| Tests with `// REQ-` / `// PROBE-` comments (generated corpus; trivia + escape + both positions) | ✅ |
| `make spec-check` | ✅ |
| `make ci` | ✅ |

## Phases

### Phase 0 — Spec text — **done**

Wrote the rules into REQ-113 as § Structured node predicates, defined PROBE-095 (Draft), and marked
REQ-113 `partial`. Checking the kinds against `AqlParser.g4` — rather than against this plan's draft
sketch — found the two errors in § Architecture: the three-production bracket and the five-spelling
name slot. The class position was decided (left alone) rather than left open.

**Verification:** `make spec-check` — OK.

### Phase 1 — Model + extractor

1. Add the sealed sum + `EqualPredicates` to `openehr/aql`; add `Parsed` to
   `PathSegment`. **Not** to the class position (§ Defers).
2. Populate it in the extractor from lexer-level tokens — never by re-lexing the
   verbatim text downstream of the parser, which is the burden the REQ removes.
   Both context types that can carry a form must map to one kind (§ Architecture),
   so the population is keyed on the form, not on the context.
3. Corpus tests per the probe definition — the corpus is **generated** from the
   grammar's `pathPredicate` alternatives, not enumerated by hand.

**Definition of done:** `go test ./openehr/aql/...` green; PROBE implemented;
REQ-119's round-trip suite untouched and green.

### Phase 2 — Helper alignment — **done, with one carry-over**

1. ✅ `StripPredicateTrivia` and `RedactPredicateValues` godoc now point at
   `PathSegment.Parsed` as the preferred consumption path, and say why each
   still exists: the first for consumers holding predicate TEXT (the class
   position, a hand-assembled predicate), the second because it cannot decide
   what a value is for every reader — it treats a numeric literal as structure,
   which a disclosure rule counting a bare magnitude as a value cannot adopt.
2. **Carried over:** the worked example. `docs/examples.md` documents real
   programs under [`cmd/examples/`](../../cmd/examples/) and must stay in sync
   with them in the same PR, so adding one is a new example program rather than
   a prose block — its own small change, not a footnote to this one.

**Definition of done:** godoc updated; `make ci` green.

## Mapping to specs

- [clinical-modeling.md § REQ-113 § Structured node predicates](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) — the canonical normative contract (authored in Phase 0; this plan restates none of it)
- [conformance.md § PROBE-095](../specifications/conformance.md#probe-095--aql-predicate-structuring) — the corpus construction and its oracle
- REQ-113 / REQ-117 / REQ-119 sections — the landed vocabulary this model completes
