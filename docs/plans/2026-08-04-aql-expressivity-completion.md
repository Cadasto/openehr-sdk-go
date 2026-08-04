# Plan — AQL expressivity completion (structured-AST catalogue, lint gate, builder algebra)

**Date:** 2026-08-04
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-117](../specifications/clinical-modeling.md#req-117--aql-expression-catalogue-completion) (extends the landed REQ-113 / REQ-109 / REQ-055 surfaces)
**Probes:** PROBE-087 (catalogue completeness), PROBE-088 (builder stability)
**Implementation:** planned
**Depends on:** REQ-113 (structured AST, landed), REQ-109 (lint, landed), REQ-055 (builder, landed); no grammar-profile change (every target shape is already admitted by `resources/aql/grammar/active/`)
**Defers:** aggregate operands beyond `identifiedPath`/`*` (a grammar + upstream-spec question, not an extractor gap); grammar-level function positions the published grammar rejects (LIKE/MATCHES LHS, EXISTS operand, ORDER BY key, aggregate args — upstream AQL-change territory); query-client paging/header options (`openehr-ehr-id` header option is STRAND-09 item 1 adjacent); CDR-grade path resolution (REQ-109 out-of-scope list, unchanged)

## Goal

Close the declared expressivity gaps that force AQL-consuming engines and corpus tooling to refuse or work around grammar-admitted queries: complete the structured-AST catalogue (eight `ErrIncompleteAST` classes reduced to the single int-overflow guard), stop the lint gate over-rejecting two server-executable shapes, and give the builder the containment algebra and in-text paging the parse side already models. Consumers are AQL execution engines building on `parse.Query` (fail-closed on `ErrIncompleteAST`, so every catalogue gap is a wholesale refusal) and benchmark/conformance corpus authors building through `aql.Builder`.

## Definition of Ready

- **Covers** lists REQ-117; canonical prose exists ([clinical-modeling.md § REQ-117](../specifications/clinical-modeling.md#req-117--aql-expression-catalogue-completion), registry row in [REQ.md](../specifications/REQ.md)). ✔
- No irreversible fork: all vocabulary extensions are additive to sealed interface sets (documented consumer contract: unrecognised case = out-of-catalogue); no ADR needed. ✔
- PROBE-087 / PROBE-088 catalogued Draft in [conformance.md](../specifications/conformance.md). ✔
- Phases below name tasks and verification commands. ✔

## Definition of Done

- Code and tests land with `// REQ-117` / `// PROBE-087` / `// PROBE-088` citations.
- REQ-113's "v1 catalogue gaps" list in clinical-modeling.md is rewritten to the residual overflow guard, citing REQ-117; REQ-117 **Status** flips; [traceability.yaml](../specifications/traceability.yaml) `implementation` and REQ.md **Impl.** updated.
- wire.md § REQ-055 gains the one-line cross-reference to REQ-117 for the added canonical write forms (canonical home for the new constructs stays REQ-117).
- `make spec-check` and `make ci` pass; `make aqlgen-verify` untouched (no grammar change).
- Plan archived under `docs/plans/archive/`.

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry updated (`traceability.yaml`, REQ.md row, § REQ-117, PROBE-087/088) | done (Phase 0) |
| Phase 1 — vocabulary + extractor + emitter | |
| Phase 2 — lint acceptance | |
| Phase 3 — builder algebra + in-text paging | |
| Phase 4 — close-out (spec status, examples, CHANGELOG, archive) | |
| `make spec-check` + `make ci` | |

## Design constraints (binding for every phase)

1. **No grammar change.** Every target shape parses today (verified against `resources/aql/grammar/active/AqlParser.g4`: `columnExpr : … | primitive | functionCall`, `selectExpr : … | SYM_ASTERISK`, `identifiedExpr : functionCall COMPARISON_OPERATOR terminal | …`, `terminal : primitive | PARAMETER | identifiedPath | functionCall`, `matchesOperand : … | terminologyFunction | { URI }`, `containsExpr : … | containsExpr AND containsExpr | OR | ( … )`). The work is vocabulary + extractor + emitter + lint + builder only.
2. **Vocabulary lives in `openehr/aql`**, introspectable both directions (REQ-113 pattern). `openehr/aql` cannot import `openehr/aql/parse` (cycle); parse depends on the vocabulary, never the reverse.
3. **Additive sealed sets.** New `SelectExpr`/`WhereExpr`/`Value` implementations extend the sealed interfaces; the interface godoc must state that consumers type-switching must treat an unrecognised case as out-of-catalogue (never panic). Existing exported types, fields, and emitted strings are untouched — semver-minor.
4. **No silent loss** (unchanged): any AST shape `Emit` cannot render returns `aql.ErrIncompleteAST`; catalogue closure removes gaps by *modelling* them, never by widening what emit will improvise.
5. **Building-block independence (REQ-013)**: `TestAQLParseForbiddenImports` / `TestAQLLintForbiddenImports` stay green; nothing new imports `transport/`, `auth/`, `openehr/client/*`, `openehr/serialize/`.
6. **Canonicalisation is a semver contract** (wire.md § REQ-055): the PROBE-020 golden must stay byte-identical; new constructs get their own goldens (PROBE-088).
7. **TDD per task**: failing test → minimal implementation → green → commit (Conventional Commits, artefact-class messages).
8. Pure functions take no `context.Context`; builders stay construct-then-finalise, not goroutine-safe (idiom.md).

## Phases

### Phase 0 — SDD artefacts *(done in this branch's first commit)*

REQ.md row, clinical-modeling.md § REQ-117, conformance.md PROBE-087/088 (Draft), traceability.yaml row, this plan, plans README index row.

### Phase 1 — structured-AST catalogue completion (the eight closures)

**Packages:** `openehr/aql` (vocabulary), `openehr/aql/parse` (extractor `extract_query.go`, AST + emitter `query.go`, tests).

**Vocabulary additions (`openehr/aql`):**
- `LiteralSelect` (or extend `SelectExpr` set): a SELECT projection item wrapping a typed `Value` literal — carries the same `Value` vocabulary already used in WHERE (`String/Int/Real/Bool/Null` + date/time primitives as the parser yields them).
- `StarSelect`: an explicit star item so `SELECT *, col` is an ordered list `[Star, Path(col)]` (today `Star` is a query-level bool; keep that bool true for compatibility and additionally represent position).
- `FunctionComparison` / widen `Comparison`: a comparison whose LHS is a `FunctionCall` (name + args) instead of a path. Prefer one `Comparison` with an LHS sum (`Path | FunctionCall`) over a parallel type if it stays additive; otherwise a sibling `WhereExpr` implementation.
- `FunctionCall` argument vocabulary: args become `[]FuncArg` where `FuncArg` is `Path | Value | Param | FunctionCall` (nested). Existing path-only accessors keep working.
- `PathValue`: an identified path as a comparison RHS `Value` (path-vs-path), introspectable as alias+segments (reuse `aql.IdentifiedPath`).
- `MatchesTerminology` / `MatchesURI` operands: structured `TERMINOLOGY(op, api, params)` (three strings) and `{URI}` forms on the existing `Matches` vocabulary.
- FROM-root junction: `parse.FromClause` gains the same `ContainsJoin` tree the nested side already has (root becomes a containment *expression*, not a single `ClassExpr`; keep `From.Root` working for the single-root case — deprecate nothing).

**Extractor (`extract_query.go`):** replace each of the eight `ErrIncompleteAST` sites with faithful extraction; junction operands recurse; the int-overflow guard stays and gains a regression test proving it still fires.

**Emitter (`query.go`):** render every new shape canonically (upper-case function names, single spaces, parens only where precedence requires); extend the fixed-point property corpus.

**Tests (PROBE-087, inline):** per-shape structural pins in `query_test.go`; round-trip idempotence + canonical-preservation rows in `roundtrip_test.go` for all eight shapes (incl. nested functions, junctions mixing new operands, `SELECT COUNT(*), 1, e/x AS a`); the former 10-case incomplete-AST suite flips to asserting successful extraction (keep the overflow case as the lone remaining refusal); introspection rows in `openehr/aql/introspect_test.go`.

**Definition of done:** `go test ./openehr/aql/... ` green; forbidden-import tests green; PROBE-080 property corpus (34+11 cases) untouched and green; no `parse/gen` type appears in any exported signature.

### Phase 2 — lint gate acceptance *(after Phase 1; parallel with Phase 3)*

**Packages:** `openehr/aql/lint`, `openehr/aql/parse` (only if `Document` needs a `SelectAliases []string` field — additive).

- `ORDER BY <select-alias>`: resolution order becomes FROM aliases → SELECT `AS` aliases → `aql_unknown_alias`. Table tests: alias hit (no issue), unknown identifier (issue unchanged), alias shadowing a FROM alias (FROM wins, no issue), alias used with a path tail (`score/magnitude` — still unknown: an AS alias is not a path root).
- Boolean literal comparison operand (`WHERE s/is_queryable = true`): locate the rejection (parse-level `Document` handling or lint extract) and accept; `false` and parameterised comparisons covered.
- Regression: the full REQ-109 lint fixture corpus (`testkit/cassettes/aql/lint/`) byte-stable; PROBE-028 golden untouched.

**Definition of done:** `go test ./openehr/aql/lint/...` green; PROBE-028 probe green; no new lint codes introduced; `validation.ValidateAQL` bridge unchanged.

### Phase 3 — builder containment algebra + in-text paging *(after Phase 1; parallel with Phase 2)*

**Packages:** `openehr/aql` (builder), `testkit/probes/aql` (PROBE-088), `openehr/aql/testdata/wire/` (goldens), `docs/specifications/wire.md` (one-line REQ-055 cross-ref).

- Containment expression API (additive beside the existing flat `Contains(...)`): `ContainsExpr(expr ContainmentExpr)` with constructors `C(rmType, alias, archetypeID ...string) ContainmentExpr`, `(ContainmentExpr).Contains(child)`, `.NotContains(child)`, `ContainsAnd(a, b, ...)`, `ContainsOr(a, b, ...)` — names to be finalised against the package's existing vocabulary style (`Archetype`, `Col`); the parse-side `Containment`/`ContainsJoin` shapes are the semantic reference. Emission: `NOT` binds tightest, then `AND`, then `OR`; parens only when nesting departs from that precedence (mirrors Phase 1's emitter rule — reuse one canonicaliser if practical).
- In-text paging: `LimitInline(n)` / `LimitInlineParam(name)` + optional `OffsetInline` (grammar: `LIMIT limitValue (OFFSET limitValue)?`), emitted after ORDER BY. Mutually exclusive with envelope `Limit()`/`Offset()`: `Build()` returns an error when both channels are set.
- PROBE-088: goldens for `NOT CONTAINS`, sibling `AND`, `OR` with grouping, mixed nesting, in-text LIMIT/OFFSET (literal + param), plus a pin that the existing PROBE-020 golden is byte-identical.
- wire.md § REQ-055: one sentence — canonical forms for negated/sibling containment and in-text paging are specified by [clinical-modeling.md § REQ-117]; additive to this section's contract.

**Definition of done:** `go test ./openehr/aql/... ./testkit/probes/aql/...` green; PROBE-020 golden byte-identical; round-trip: every new builder output parses via `ParseQuery` with no `ErrIncompleteAST` and re-emits to the same bytes (ties Phase 1 and 3 together).

### Phase 4 — close-out

- clinical-modeling.md: REQ-113 "v1 catalogue gaps" list rewritten (residual = overflow guard, cite REQ-117); REQ-117 **Status** → implementation status; REQ-109 § notes the two lint acceptances.
- conformance.md: PROBE-087/088 **Status** → Implemented (inline / Sandbox); AQL matrix row updated.
- traceability.yaml: REQ-117 `implementation`, `probes: [PROBE-087, PROBE-088]`, `tests:` list; REQ.md **Impl.** → landed.
- `cmd/examples/aql-parse-structured` + `cmd/examples/aql-build` extended with one new-shape demo each; `docs/examples.md` rows updated.
- CHANGELOG: one artefact-class bullet (≤35 words).
- `make spec-check` + `make ci`; archive this plan (`git mv` + indexes).

## Mapping to specs

- [clinical-modeling.md § REQ-117](../specifications/clinical-modeling.md#req-117--aql-expression-catalogue-completion) — normative contract (canonical home for all new constructs)
- [clinical-modeling.md § REQ-113](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) / [§ REQ-109](../specifications/clinical-modeling.md#req-109--aql-static-lint) — the extended surfaces
- [wire.md § REQ-055](../specifications/wire.md#req-055--wire-boundary) — builder canonical write form (cross-reference only)
- [REQ.md](../specifications/REQ.md) — registry row
