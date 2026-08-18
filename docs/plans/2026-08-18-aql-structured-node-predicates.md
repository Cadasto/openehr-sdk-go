# Plan — AQL structured node predicates (typed segment/class predicate model)

**Date:** 2026-08-18
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** **REQ-160** (structured node predicates) — proposed; needs a **new band**: the clinical-modeling band (100–119) is exhausted per the [numbering policy](../specifications/REQ.md#numbering-policy), which requires the next band to be allocated there first. Proposed: **160–169 "AQL structured model & diagnostics"**, deliberately leaving 150–159 free for the transport-overflow band raised in PR #107's review (finding F2). Phase 0 settles both.
**Verifies / builds on:** landed [REQ-113](../specifications/clinical-modeling.md) (structured read AST; `aql.IdentifiedPath`/`PathSegment`, `ClassExpr.PredicateComparison`), [REQ-117](../specifications/clinical-modeling.md) (expression-catalogue completion), [REQ-119](../specifications/clinical-modeling.md) (re-parseable emission; verbatim predicate text; `aql.StripPredicateTrivia`)
**Probes:** **PROBE-095** (proposed; allocated in Phase 0) — predicate structuring corpus over the grammar's `nodePredicate` forms
**Implementation:** planned
**Depends on:** `openehr/aql`, `openehr/aql/parse` (extractor), the REQ-119 verbatim-text guarantee (unchanged by this plan)
**Defers:** a carrier for VERSION class predicates (`LATEST_VERSION` / `ALL_VERSIONS`) — a distinct, version-query-shaped ask that should ride a version-query slice, not this plan; full `nodePredicate` sub-grammar *validation* at the class position beyond the kinds structured here (the REQ-119 § archive deferral for issue #99 stands where a predicate is not one of the structured kinds); rewriting/normalising `Raw` (REQ-119 keeps it verbatim)

## Goal

Give every bracketed predicate the same treatment the standing class predicate got
in REQ-113: a **typed model beside the raw text**, so a reader never has to lex
predicate text again. Today `aql.PathSegment.Predicate` and `parse.ClassExpr.Predicate`
are raw strings; the only structured predicate is the standing comparison
(`ClassExpr.PredicateComparison`). Everything else — `[at0001]`, `[id3]`,
`[at0001, 'Systolic']`, `[at0001 and name/value=$name]`, an archetype HRID in a
path segment, a `$param` — arrives as text the reader must re-tokenize.

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
   treats the node predicate as a sub-grammar** — guarded positions, verbatim
   round-trip — but stopped short of structuring it, recording a full
   `nodePredicate` validator as deferred. A typed model subsumes most of that
   deferral for the forms that dominate real queries.

Every future widening of downstream predicate support (name predicates,
`name/value` standing forms, terminology-coded names) multiplies the re-lexing
sites if the carrier stays raw text.

## Architecture

Follow the package's sealed-interface style (`aql.Value`, `parse.SelectExpr`).
Sketch — Phase 0 / review material, not normative:

```go
// On aql.PathSegment and (for the class position) parse.ClassExpr:
//   Parsed SegmentPredicate   // nil when the SDK does not structure the form
// Raw stays verbatim (REQ-119); Parsed is a read-side derivation.

// SegmentPredicate is a sealed sum over the grammar's nodePredicate forms.
type SegmentPredicate interface{ segmentPredicate() }

// NodeIDPredicate: [at0001] / [id3] — trivia-free, delimiter-free.
type NodeIDPredicate struct{ ID string }

// NodeNamePredicate: [at0001, 'Systolic'] and the terminology-coded name
// forms; Name distinguishes plain string vs coded spelling.
type NodeNamePredicate struct {
    ID   string
    Name PredicateName
}

// ArchetypePredicate: an archetype HRID in a segment position.
type ArchetypePredicate struct{ HRID string }

// ParamPredicate: [$param].
type ParamPredicate struct{ Name string }

// ComparisonPredicate: the standing form — reuses the existing structured
// aql.Comparison (REQ-113/REQ-117 vocabulary), so WHERE and predicate
// comparisons share one model.
type ComparisonPredicate struct{ Comparison Comparison }
```

Rules the spec section must pin (Phase 0):

- **`Parsed` nil is a statement, not an accident**: it means "the SDK does not
  structure this form", and the set of unstructured forms is enumerated in the
  spec — never silently narrowed or widened (the REQ-113 structuring lesson: a
  reader must be able to fail closed on exactly the forms the SDK leaves raw).
- Components are **trivia-free and delimiter-free** (the at-code without brackets,
  the name without quotes, escapes resolved) — the values a reader compares or
  displays, not the spelling.
- **Emission ignores `Parsed`** — REQ-119's verbatim round-trip is untouched.
- The builder side (`aql.Builder`) is out of scope here; it already writes
  predicates from typed inputs.

## Definition of Ready

Implementation (Phase 1+) may start once **Phase 0 has landed the REQ**:

- The new band and the REQ id are allocated in `REQ.md` (numbering-policy table
  first, per its own rule; coordinate with the PR #107 F2 transport-overflow
  allocation).
- Canonical normative prose exists: the sum type's kinds, the trivia/delimiter
  rules, the nil-`Parsed` enumeration, and the REQ-119 non-interference clause.
- PROBE-095 (or the allocated id) is defined in `conformance.md` (Draft): a corpus
  over every `nodePredicate` alternative in `AqlParser.g4` — including
  trivia-carrying spellings (whitespace, `--` comments) and escaped names —
  asserting the structured components and the untouched `Raw`.
- Each phase names its verification command.

## Definition of Done

- `openehr/aql` + `openehr/aql/parse` land the model with `// REQ-` citations;
  extractor populates `Parsed` in every position that carries a predicate
  (path segments, SELECT/WHERE/ORDER BY paths, class expressions).
- `aql.StripPredicateTrivia` and `aql.RedactPredicateValues` docs updated to point
  at the structured model as the preferred consumption path (both stay — text
  consumers exist).
- `traceability.yaml` + REQ.md **Impl.** column; `roadmap.md` row.
- `make spec-check` and `make ci` green; plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| Band + REQ § + registry row (Phase 0) | |
| PROBE defined in `conformance.md` (Draft) | |
| Sum type + extractor population | |
| Tests with `// REQ-` / `// PROBE-` comments (trivia + escape corpus) | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 0 — Band, spec & registry (the specify gate)

1. Allocate the 160–169 band in the numbering-policy table (or whatever Phase 0
   agrees), then REQ-160's canonical section and registry row (**Impl.:** `planned`).
2. Enumerate the structured kinds against `AqlParser.g4`'s `nodePredicate`
   production — check every alternative against the grammar before writing
   acceptance criteria (the REQ-118 lesson: `TOP $n` looked plausible, and the
   grammar admits no parameter there).
3. Define the probe (Draft) + `traceability.yaml` row.

**Definition of done:** `make spec-check` passes with the new rows.

### Phase 1 — Model + extractor

1. Add the sealed sum to `openehr/aql`; add `Parsed` to `PathSegment` and the
   class position.
2. Populate it in the extractor from lexer-level tokens (not by re-lexing the
   verbatim text downstream of the parser).
3. Corpus tests per the probe definition.

**Definition of done:** `go test ./openehr/aql/...` green; PROBE implemented;
REQ-119's round-trip suite untouched and green.

### Phase 2 — Helper alignment

1. Doc updates on `StripPredicateTrivia` / `RedactPredicateValues`.
2. A worked example (`docs/examples.md`) showing predicate consumption without
   text comparison.

**Definition of done:** examples build; docs updated.

## Mapping to specs

- Phase 0 authors the canonical section — this plan holds proposals only.
- [docs/specifications/REQ.md](../specifications/REQ.md) — band + registry row (Phase 0)
- REQ-113 / REQ-117 / REQ-119 sections — the landed vocabulary this model completes
