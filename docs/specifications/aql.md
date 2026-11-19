---
kind: specification
---

# AQL

**Status:** Draft

Normative contract for the SDK's Archetype Query Language building blocks above the wire boundary: the parsed query tree, its canonical emission, the builder's write-side parity, and the static and semantic lint.
Covers REQ-109, REQ-113, REQ-119, REQ-160, REQ-161 and REQ-163.
The wire boundary, the executor and stored queries are in [wire.md § REQ-055](wire.md#req-055--wire-boundary) and [§ REQ-057](wire.md#req-057).

---

## REQ-113 — Execution-oriented parsed AQL AST

REQ-109's `openehr/aql/parse` returns a lint-shaped `Document` that flattens the FROM/CONTAINS tree to a class set and erases the WHERE expression structure: the lint contract reasons over the *set* of bound classes and the *set* of paths, not their containment shape or the operator tree.
An execution consumer (a server lowering AQL to SQL, a planner picking up CONTAINS nesting, a query rewriter) needs to read the *structure*.
The construction-side `aql.Builder` (REQ-055) is the write-side mirror of that need.

The SDK **MUST** expose a **stable, generated-type-free, readable** structured AQL AST: a `string → structured query` entry point whose result a consumer can traverse without importing `openehr/aql/parse/gen` or any `internal/` package.
The AST **MUST** preserve containment nesting, the WHERE operator/value tree, SELECT function/aggregate wrappers and aliases, ORDER BY direction, and LIMIT / OFFSET values.
The WHERE and Value vocabularies are shared with the construction side: `aql.Comparison`, `aql.Junction`, `aql.NotExpr`, `aql.ExistsExpr`, `aql.MatchesExpr`, `aql.LikeExpr`, `aql.ParamValue`, `aql.StringValue`, `aql.IntValue`, `aql.RealValue` and `aql.BoolValue` are populated by both Builder and Parse: one model, two directions.
The sealed `SelectExpr`, `WhereExpr` and `Value` sets grow additively, so a consumer type-switching over them treats an unrecognised case as out of catalogue rather than panicking, and each interface's godoc says so.

### Surface

```go
// Package github.com/cadasto/openehr-sdk-go/openehr/aql/parse

func ParseQuery(q string) (*Query, error)              // (*Query, aql.ErrIncompleteAST) on catalogue gap
func (d *Document) Query() *Query                       // best-effort partial AST
func (d *Document) QueryErr() error                     // aql.ErrIncompleteAST diagnostic, nil otherwise
func (d *Document) Dropped() []DroppedConstruct         // § Value-free structured drop records

type Query struct {
    Select  SelectClause
    From    FromClause
    Where   aql.WhereExpr  // nil when no WHERE clause
    OrderBy []OrderTerm
    Limit   LimitExpr      // nil when no LIMIT
    Offset  LimitExpr      // nil when no OFFSET
    Spans   ClauseSpans    // byte offsets of each clause in the source; zero when absent or built directly
}
func (q *Query) Emit() (string, error)                  // refuses (returns aql.ErrIncompleteAST) on an extractor-incomplete AST; ignores Spans

// Clause positions — half-open byte ranges [Start, End) into the parsed text.
type Offsets     struct { Start, End int }              // zero value: absent
func (o Offsets) IsZero() bool
type ClauseSpans struct {
    Select, From, Where, WhereExpr, OrderBy, Limit, Offset Offsets
}

// LIMIT / OFFSET — sealed union of literal and parameter forms.
type LimitExpr  interface { /* sealed */ }
type IntLimit   struct { N int }                        // `LIMIT 50`
type ParamLimit struct { Name string }                  // `LIMIT $rows`

// SELECT
type SelectClause struct {
    Distinct bool
    Star     bool
    Items    []SelectItem
    Top      *aql.TopClause                             // nil when no TOP; § The TOP carrier
}
type SelectItem  struct { Expr SelectExpr; Alias string }
type SelectExpr  interface{ isSelectExpr() }
type PathExpr    struct { IdentifiedPath }
type FunctionCall struct { Name string; Args []SelectExpr; Distinct, Star bool; Raw string }
type LiteralExpr struct { Value aql.Value; Raw string } // § Projection source text
type StarExpr    struct{}                               // a `*` beside column items

// FROM / CONTAINS — nested containment tree
type FromClause   struct { Root ClassExpr; Contains *Containment; Junction *Containment } // Junction: a FROM-root junction
type Containment  struct { Class ClassExpr; Children []Containment; ChildJoin ContainsJoin; Negated bool }
type ContainsJoin int  // ContainsAnd / ContainsOr

// ORDER BY
type OrderTerm struct { Path IdentifiedPath; Dir OrderDir }
type OrderDir  int  // OrderAsc / OrderDesc

// Drop records — § Value-free structured drop records
type DroppedConstruct struct { Kind ConstructKind; Clause Clause; Span Span }
```

For a query parsed from `src`, `src[Start:End]` of each `ClauseSpans` field is that clause's text: from the first byte of its keyword to just past its last token, with `WhereExpr` the condition after `WHERE`, `Limit` ending before any `OFFSET`, and surrounding whitespace, comments and a trailing `--` outside every span.

The shared `aql.Value` vocabulary also carries `aql.NullValue`, a typed NULL, so the unquoted `NULL` keyword round-trips without colliding with a `StringValue{"NULL"}`.
`aql.ErrIncompleteAST` is the sentinel `ParseQuery` and `Document.QueryErr` surface, and `(*Query).Emit` returns it for an AST from an incomplete extraction (§ Catalogue and residual).

`(*Document).Tree` is deprecated: it returns the generated ANTLR context (`gen.ISelectQueryContext`), which can change shape on any grammar regeneration, and stays as a raw-tree escape hatch; `Query` and `Query.Spans` are the stable read surface.

`aql.FormatWhere` renders a `WhereExpr` in canonical form, and `(*Query).Emit` renders `WHERE` through it.

### Structured path access

Two path-bearing sub-structures are exposed as parsed structure, not only raw text, so an execution consumer reads them without re-tokenizing AQL the SDK already parsed once:

- **`ClassExpr.PredicateComparison`**: a class *standing* predicate (e.g. `EHR e[ehr_id/value = $x]`) is exposed as an optional `*aql.Comparison` (`{path, operator, value}`, reusing the shared vocabulary) when it is a simple comparison.
  It **MUST** be nil for an archetype-HRID predicate (on `ClassExpr.Archetype`), a version predicate, or a non-scalar or complex standing predicate, so a comparison is distinguishable from a non-comparison.
  The comparison's `ParsedPath` carries the relative left-hand path's structured `Segments` with an **empty `Alias`** (a class-predicate path binds no FROM alias); the verbatim `ClassExpr.Predicate` text is retained.
- **`ParsedPath`**: `aql.Comparison`, `aql.ExistsExpr`, `aql.LikeExpr` and `aql.MatchesExpr`, the `identifiedExpr` alternatives whose operand is an `identifiedPath`, each carry the structured path (`*aql.IdentifiedPath`: alias + segments) beside the raw `Path` string, populated by the parser.
  A WHERE path sets the alias root; a class standing predicate's comparison sets `Alias == ""` (relative path).
  It is nil on the write side and for a path shape the parser does not structure.
  `ParsedPath.Raw` **MUST** equal `Path`.

The shared path vocabulary, `aql.IdentifiedPath` and `aql.PathSegment`, lives in `openehr/aql` so that `aql.Comparison` can carry it.
`parse.IdentifiedPath` embeds `aql.IdentifiedPath`, whose fields it promotes, and adds the parse-only `Clause` and source `Pos`; `parse.PathSegment` re-exports `aql.PathSegment`.

### Catalogue and residual

`parse.ParseQuery` **MUST** model every query the SDK grammar profile admits without `aql.ErrIncompleteAST`, apart from the one residual below.
`aql.ErrIncompleteAST` is fail-closed by design, so every gap is a query shape a downstream engine has to refuse wholesale even when its own execution layer could run it.
Among the shapes the catalogue models:

1. A primitive literal in SELECT (`SELECT 1, e/ehr_id/value FROM …`), as a typed literal item.
2. Mixed `SELECT *, col`, order-preserving.
3. A function call as the left operand of a WHERE comparison (`WHERE LENGTH(o/name/value) > 5`).
4. `MATCHES` with a `TERMINOLOGY(...)` or `{URI}` operand, modelled structurally (function name and three string arguments, or the URI), not as raw text.
5. A path-vs-path comparison (`WHERE a/x = b/y`).
6. A boolean junction at the FROM root (`FROM COMPOSITION c1 OR COMPOSITION c2`, with `AND` and grouping).
7. A parameter, primitive or nested function call as a function argument (`SELECT CONCAT('a', $p, LENGTH(x/y)) …`), in SELECT and in WHERE.
8. `AND` / `OR` junctions over any modelled operand.
9. The deprecated `SELECT TOP` clause (§ The `TOP` carrier).

**The residual is one condition: a numeric literal the value vocabulary cannot represent.**
That is one of:

- a `LIMIT` / `OFFSET` value or a `SELECT TOP` count beyond Go `int`;
- an INTEGER outside `int64` (the type `aql.IntValue` carries), or a REAL beyond `float64` in either direction (an overflow, or an underflow whose nearest representable value is zero, such as `1e-400`), in any value position: a SELECT literal, a comparison operand, a `MATCHES` member.

Such a literal **MUST** be refused, never degraded: `ParseQuery` and `Document.QueryErr` return `aql.ErrIncompleteAST`, and `(*Query).Emit` refuses the partial AST with the same error, because degrading it would be silent precision loss (`> 1e-400` emitted as `> 0.0` flips the predicate across the zero boundary).
The range of `int64` is asymmetric: its negative bound is exactly representable and is read, not refused, and a repeated unary minus is read at its parity (`numericPrimitive` is recursive), not reported as out of range.

Every other extractor branch is defensive: unreachable against the profile, and recording a gap rather than returning a zero value if a widened grammar ever reaches it.
No grammar change is licensed here.

### The `TOP` carrier

openEHR **QUERY Release-1.1.0 § 4.4.3** governs the clause.
Its syntax is `SELECT TOP integer [FORWARD|BACKWARD]`; from Release 1.1.0 the modifier is deprecated in favour of `LIMIT` with `ORDER BY`, and the specification announces its removal in a future major release; and `TOP` is not allowed beside a `LIMIT` clause in the same query.
A deprecated but legal construct is still one the SDK reads: until the removal release a client, a stored query or a conformance corpus may carry `TOP`.

- The `SELECT` clause **MUST** carry the clause as an optional value (`SelectClause.Top`, an `*aql.TopClause`) that models the row count and the direction, with an unspecified zero direction.
  The optional value keeps "no `TOP`" distinguishable from `TOP 0`: a dropped `TOP` turns a bounded query into an unbounded one, and an absent one read as `0` inverts it.
  The zero direction lets a bare `TOP n` round-trip without a direction the source did not write, and a `BACKWARD`, which selects the other end of the result set, is never lost.
- `TOP` and the `LIMIT` / `OFFSET` clause **MUST** each report what the source wrote, independently.
  The parser never normalises one into the other and never picks one when a query carries both: that query is spec-invalid (§ 4.4.3), and reporting it as written is what lets the lint name it (`aql_top_with_limit`, [§ REQ-109](#req-109--aql-static-lint)).
  `Emit` renders both as written, since the profile accepts the text and the emitter is not the diagnosis layer.

The profile's `top` production admits no `PARAMETER`, and the [`SDK-AQL-003`](../../resources/aql/grammar/DIVERGENCES.md#sdk-aql-003--limit--offset-parameter-operands) parameter relaxation does not extend to it: the SDK does not widen a construct the specification is removing.
The carrier therefore holds an integer count and does not reuse the `LIMIT` vocabulary (`parse.LimitExpr`), which has a parameter form and no direction.
A count outside Go `int` is the residual above.
The vocabulary lives in `openehr/aql` and is shared with the builder ([§ REQ-163](#req-163--aql-write-side-expressivity-parity)), which also fixes its canonical spelling.

### Projection source text

- `parse.LiteralExpr` **MUST** carry the literal's source text as written (`Raw`) beside its typed `aql.Value`, for a literal in a `SELECT` projection and for a literal in a function-call argument.
  The openEHR result schema names a projected column with no `AS` alias by its expression text, and re-rendering the typed value gives the canonical form, not the source form (`1.50` → `1.5`, `001` → `1`, `"x"` → `'x'`).
- `parse.FunctionCall` **MUST** carry the call's source text as written (`Raw`), for a function or aggregate call in a `SELECT` projection and for a call nested as a function-call argument, for the same reason.
  Rebuilding the text from the parts gives the upper-cased name and canonical spacing (`length( c/name/value )` → `LENGTH(c/name/value)`), which is neither what the client wrote nor how a path or literal column is named.
- Emission **MUST** render the typed value in canonical form and the call from its parts, never the source text: the canonical write form is normative ([wire.md § REQ-055](wire.md#req-055--wire-boundary)), and a `LiteralExpr` or `FunctionCall` a caller constructed has no source text, so a consumer treats `Raw` as optional.

A value-position call (`aql.FuncCall`) carries no source text, as a `WHERE` literal carries none: only a projection names a result column.

A consumer that needs a deterministic rendering of any `aql.Value`, parsed or constructed, uses `aql.FormatValue`; there is no second escaping implementation.

### Structured node predicates

§ Structured path access types exactly **one** bracketed predicate, the standing comparison at the class position, and leaves every other predicate as the raw text between the brackets, which [§ REQ-119](#req-119--re-parseable-canonical-aql-emission) keeps verbatim because a whitespace-collapsed predicate does not re-parse wherever the grammar separates its operands with a keyword.
A consumer comparing that text against a compiled-OPT node id or a name would have to model the lexer's trivia itself.
Every bracketed path predicate is therefore exposed as structure beside its text, at a path step, after a class and on an alias root (§ The class and alias-root brackets carry the same model).
**`Raw` is the spelling and stays authoritative for emission; the structured model is the value and is authoritative for comparison.** The model carries what `Raw` does not: where each component begins and ends.

#### The bracket is three productions, not one

A bracketed path predicate is `pathPredicate : SYM_LEFT_BRACKET (standardPredicate | archetypePredicate | nodePredicate) SYM_RIGHT_BRACKET`, and the three alternatives overlap:

| Production | Admits |
|---|---|
| `standardPredicate` | `objectPath COMPARISON_OPERATOR pathPredicateOperand` |
| `archetypePredicate` | `ARCHETYPE_HRID` or `PARAMETER` |
| `nodePredicate` | a node id or HRID with an optional name, `PARAMETER`, a comparison, `objectPath MATCHES CONTAINED_REGEX`, and `AND` / `OR` junctions over itself |

A comparison, a bare HRID and a bare `$param` are therefore each spelled by two productions, once as their own top-level alternative and once inside `nodePredicate`, so which parse-tree node carries a form depends on whether it sits at the top of the bracket or nests as a junction operand.
The structured kind **MUST** be a property of the form, not of the production that carried it: a form yields the same kind at the top of a bracket and as a junction operand.
The kind set is derived from the vendored grammar and checked against it mechanically ([PROBE-095](conformance.md#probe-095--aql-predicate-structuring)), never from a hand-written list of what the position is believed to admit.

#### The structured kinds

The model **MUST** be a sealed sum in `openehr/aql` covering these forms:

- a node id (`[at0001]`, `[id3]`);
- a node id with a name (`[at0001, 'Systolic']`);
- an archetype HRID;
- an archetype HRID with a name;
- a parameter (`[$param]`);
- a comparison, carrying `aql.Comparison`, so a WHERE comparison and a predicate comparison share one vocabulary;
- a regex match (`objectPath MATCHES CONTAINED_REGEX`, whose token also admits an optional `; 'label'` tail the model carries as its own component);
- an `AND` / `OR` junction over two structured predicates, with its operator and both operands.

The sum follows the package's `aql.Value` / `parse.SelectExpr` style.

#### The name slot

The grammar admits `SYM_COMMA (STRING | PARAMETER | TERM_CODE | AT_CODE | ID_CODE)` after both the node-id and the archetype-HRID alternatives.
The name carrier **MUST** discriminate all five spellings, on both alternatives, so a consumer resolving names against a template can tell a name deferred to bind time (`PARAMETER`) from a name it can resolve now.

#### Components are values, not spellings

Every component **MUST** be the value a reader compares or displays: delimiters removed (an at-code without its brackets, a name without its quotes), escapes resolved, and trivia (whitespace and `--` comments) absent, while `Raw` keeps the spelling verbatim.
A path-valued component follows the comparison vocabulary it reuses: the as-written `Path` beside the structured `ParsedPath`, whose `Segments` are the trivia-free form.
A regex component decomposes the `CONTAINED_REGEX` token: the braces, the interior whitespace, the `/` delimiters and the `;` are delimiters, so the model carries the pattern (with only the `\/` spelling resolved, which exists because `/` is the delimiter) and the optional label (unquoted, escapes resolved) as separate values; every other backslash sequence is regex syntax and stays as written.

#### An unstructured predicate is a statement, not a gap

A predicate the SDK does not structure **MUST** report itself as unstructured, a nil structured carrier beside a populated `Raw`, and never as a partial structure, which a consumer could not tell from a complete one at the type level.
The unstructured forms are exactly, at each of the three brackets: a comparison whose right-hand operand the shared value vocabulary does not carry (an `objectPath` operand, a node-code `ID_CODE` / `AT_CODE` operand, or a numeric literal in the residual of § Catalogue and residual), and a junction with an unstructured operand.
Every other alternative of every production the position admits is structured, so a reader can fail closed on exactly these forms.
Shrinking the set (a `PathValue` carrier for the path operand, a node-code value kind) widens the sealed value vocabulary and is its own change.

#### The class and alias-root brackets carry the same model

The bracket after a class in `FROM` / `CONTAINS` and the bracket on a path's alias root (`o[...]/data`) are the same `pathPredicate` production as a path step.
Each **MUST** carry the structured model beside its existing fields, derived as a path step's is, so one form yields one structure at all three: `parse.ClassExpr.ParsedPredicate` beside `Archetype`, `ParamArchetype`, `Predicate` and `PredicateComparison`, and `aql.IdentifiedPath.ParsedPredicate` beside `Predicate`.
Both extractors fill it, the flat lint view included; it is nil where the bracket is absent and on a `VERSION` class, whose bracket is `versionPredicate`.
`ClassExpr.Archetype` **MUST** carry the HRID of the named form `[hrid, name]` as well as of the bare one, so a reader of `Archetype` alone sees the archetype the class names.
`ClassExpr.Predicate` keeps the named form's whole bracket text, which emission renders ([§ REQ-119 § Emit-side structural parity](#emit-side-structural-parity)).

#### What the model leaves out

- A carrier for the `VERSION` class predicate keywords (`LATEST_VERSION` / `ALL_VERSIONS`): `versionPredicate` is a different production with a different accept set.
- Validating the predicate sub-grammar ([§ REQ-119 § Out of scope](#req-119--re-parseable-canonical-aql-emission)), since structuring what the parser accepted is not deciding what to accept.
- Rewriting or normalising `Raw`, which stays verbatim.
- The builder side: `aql.Builder` writes predicates from typed inputs ([§ REQ-163](#req-163--aql-write-side-expressivity-parity)).
- A disclosure policy over the components: each component is individually addressable so a consumer applies its own policy, and the SDK's own diagnostics follow [§ REQ-168](clinical-modeling.md#req-168--value-free-validation-diagnostics).

### Comparability

The shared vocabulary is not safely `==`-comparable.
`aql.PathValue` and `aql.FuncCall` carry slices, so `==` on either, on a `WhereExpr` or structured predicate holding one, or on an `aql.SelectField` carrying a call-shaped field, compiles and then panics at run time, and so does a map insertion keyed by one.
The SDK **MUST** ship a panic-free equality over `aql.Value` (`aql.EqualValues`) and over the structured predicate model (`aql.EqualPredicates`), across every value and pointer shape, a typed-nil pointer included, the predicate equality comparing components and a path through its structured form, so `[name / value = 'x']` equals `[name/value='x']`.
The compiler does not see the trap, so the godoc of each affected type states that `==` and map-key use are unsupported.
Equality over `WhereExpr` and `SelectExpr` is not offered: predicate-tree equality needs a normalisation policy first (is `a AND b` equal to `b AND a`?), which waits until a consumer needs it.
Callers compare built query strings.

### Value-free structured drop records

The drop channel names the offending construct by quoting its source text into `aql.ErrIncompleteAST`.
The drop record names it without quoting, under the two-class rule of [§ REQ-168 § The two field classes](clinical-modeling.md#the-two-field-classes): the construct kind, the clause and the source span are all value-free, and the record has no value-bearing field.

#### The drop record

Every construct the extractor drops **MUST** be recorded as a value-free `{kind, clause, span}`, reachable from the `Document` in source order through an accessor that runs the same lazy, once-only extraction as `Query()` / `QueryErr()`, so a `Document` nobody extracted never reads as "nothing dropped", and that returns a copy, so a caller cannot corrupt the shared `Document`.
`QueryErr()` keeps its text and its `aql.ErrIncompleteAST` sentinel.

#### The kind

The kind **MUST** be a closed enumeration of drop classes whose zero value means unclassified, so a reader can switch over it exhaustively and fail closed; adding a member is a consumer-visible change.
Its members are classes, not extractor call sites.
A reachable class, a legal query the profile admits carrying a construct the value vocabulary cannot represent (today the one residual of § Catalogue and residual), has its own member.
The defensive arms, unreachable while the catalogue and the profile agree, share one member meaning "a construct the catalogue does not model".

#### The clause

The clause **MUST** be the existing `parse.Clause`, extended append-only: its existing members keep their numeric values, since a consumer may have persisted or compared them, and `String()` gains an arm for each new member.
An existing exhaustive switch over `Clause` meets each new member as a case it does not handle.

#### The span

The span **MUST** be a start and an end `parse.Position` (`{Line, Col}`, the vocabulary of `SyntaxError.Pos` and of the `Pos` a class expression and an identified path carry) over the exact input handed to the parser, pointing at the offending construct rather than at its enclosing clause, and zero when the construct cannot be attributed.
A caller holding the input can then quote the construct under its own policy.

### Out of scope

The structured AST is **syntax-faithful for the catalogue**: it carries the source path text verbatim (`IdentifiedPath.Raw`), and function names are normalised to upper case (`count` → `COUNT`) so emission produces canonical AQL regardless of source casing.
It does **not** evaluate:

- archetype / template constraints (that is REQ-102);
- terminology binding;
- semantic validity beyond the SDK grammar profile (the server remains the execute-time authority, [PROBE-021](conformance.md#probe-021--aql-parse-error-mapping)).

Emission does not read the structured fields of § Structured path access or the model of § Structured node predicates ([§ REQ-119 § Emission ignores the structured model](#emission-ignores-the-structured-model)).
Structured diagnostics for the simplified-format codecs ([§ REQ-053](wire.md#req-053)) are a separate surface, and so is redaction policy: what counts as a value is the consumer's decision, and § Value-free structured drop records guarantees only that the value-free fields carry none.

- **Probes:** [PROBE-080](conformance.md#probe-080--aql-parseemit-round-trip-property) (the catalogue under the parse/emit round trip of [§ REQ-119](#round-trip-property)), [PROBE-082](conformance.md#probe-082--aql-structured-path-access-standing-predicate--where-path) (structured path access), [PROBE-087](conformance.md#probe-087--aql-structured-ast-catalogue-completeness) (the catalogue, the `TOP` carrier and projection source text), [PROBE-095](conformance.md#probe-095--aql-predicate-structuring) (predicate structuring), [PROBE-096](conformance.md#probe-096--aql-value-free-structured-diagnostics) (value-free diagnostics).

---

## REQ-119 — Re-parseable canonical AQL emission

[PROBE-021](conformance.md#probe-021--aql-parse-error-mapping) pins that the typed builders cannot emit syntactically invalid AQL (the write-side complement of [§ REQ-055](wire.md#req-055--wire-boundary)'s canonical form), and § Round-trip property makes `(*Query).Emit` of [§ REQ-113](#req-113--execution-oriented-parsed-aql-ast)'s AST a fixed point of the parse/emit round trip.
Both are stated over the *shapes* the catalogue models.
This requirement binds the value positions inside those shapes and the text a write path splices in verbatim, so that a consumer gets one guarantee, *what this SDK writes, it can read back*, rather than one that holds per clause and lapses per literal.

Two failure modes run through it.
Emitting text the parser **rejects** is loud: the caller gets an error the moment the query is read back.
Emitting text the parser **accepts as something else** is silent, invisible to every round-trip, golden and parser check downstream, and it is the only reason to refuse an operand that would otherwise be valid AQL.

### Round-trip property

For any AQL query the parser accepts and the catalogue of [§ REQ-113](#catalogue-and-residual) models:

```
Emit(ParseQuery(Emit(ParseQuery(x)))) == Emit(ParseQuery(x))
```

The first emit normalises whitespace, keyword casing, optional defaults (ASC), and clause ordering against the canonical write form; the second parse-emit **MUST** be a fixed point.
This is the buildable-grammar equivalent of [PROBE-020](conformance.md#probe-020--aql-builder-string-stability), held by [PROBE-080](conformance.md#probe-080--aql-parseemit-round-trip-property).

### Emission ignores the structured model

`Emit` **MUST NOT** read the structured predicate model of [§ REQ-113 § Structured node predicates](#structured-node-predicates), at a class or an alias root included: it renders `Raw`, so a consumer mutating the model cannot change what is emitted.
It renders the path of a comparison, `EXISTS`, `LIKE` and `MATCHES` from its `Path`, and a class bracket from the verbatim `ClassExpr.Predicate`, or from `ClassExpr.Archetype` when `Predicate` is empty, never from the structured fields of [§ REQ-113 § Structured path access](#structured-path-access), so those fields leave § Round-trip property unaffected.

### The emission closure property

The validating write paths are `aql.Builder.Build`, `aql.FormatWhere` and `(*parse.Query).Emit`.

- **Round-trip closure.** For every value a validating write path emits, in every value position it renders, the `SELECT` list included, `ParseQuery` of the emitted text **MUST** succeed and recover a value equal to it under `aql.EqualValues`; a value the path cannot read back is refused instead.
  The write paths apply the same guards, so one input gets one answer by any route: `Build` and `Emit` both render `WHERE` through `aql.FormatWhere`'s single validate-then-emit sequence.
- **One unvalidated formatter.** `aql.FormatValue` returns no error and so cannot refuse; it is the escape hatch for a value the caller has already checked, outside the closure.
  Being unvalidated does not license a panic: it **MUST** render a value with no wire form (a nil or typed-nil pointer, reachable through the public API as the zero `aql.MatchesExpr.Terminology`) as the empty string.
  The SDK **MUST** expose `aql.ValidateValue`, which refuses every value the closure refuses, a value with no wire form included, so a caller, or a write path outside `openehr/aql`, can hold the same line before calling `FormatValue`.
- **No silent substitution.** Where a caller-supplied operand is emitted verbatim because its grammar token is unquoted, the write side refuses operand text that would change the query's structure, even when the resulting text is valid AQL.
  The accept sets that bind it are the `MATCHES {uri}` operand's ([§ Canonical value spellings](#canonical-value-spellings)) and those of [§ Single-token identifier positions](#single-token-identifier-positions).
  The path positions REQ-055 rule 3 emits verbatim by contract are outside this rule: caller data belongs in `aql.Param` ([§ REQ-055](wire.md#req-055--wire-boundary), the injection guard).
- **Refuse, never re-spell.** A value the grammar cannot carry **MUST** fail with an error wrapping `aql.ErrInvalidQuery`, not be normalised into a spelling that parses, which would change what the query asks.
- **Verbatim text round-trips as identity.** Text the read side captures and the write side re-emits verbatim (the class and path predicates, `aql.IdentifiedPath.Raw`, `aql.Comparison.Path`) **MUST** come back unchanged, the caller's own spacing, line breaks and comments included.
  A projected literal's or function call's source text is also read verbatim, but it is never re-emitted ([§ REQ-113 § Projection source text](#projection-source-text)).
- **Every shape, and its pointer twin.** The sealed vocabularies `aql.Value`, `aql.WhereExpr`, `parse.SelectExpr` and `parse.LimitExpr` are each satisfied by the value and the pointer form of every shape, and a pointer is reachable without a caller writing `&` (`aql.MatchesExpr.Terminology` is a `*aql.FuncCall`).
  Every validator, emitter and diagnostic **MUST** give the two forms the same answer, and refuse a nil or typed-nil instead of panicking.
  The SDK **MUST** export each vocabulary's normaliser (`aql.DerefValue`, `aql.DerefWhere`, `parse.DerefSelectExpr`, `parse.DerefLimitExpr`), so a consumer type-switching over an additive sealed set can bind the same rule to both forms.
- **Absence is positional.** A predicate that denotes nothing (an untyped nil or a nil pointer) **MUST** be read as no clause at the top of a `WHERE`, including beside `FromEHR`'s implicit filter, and be refused as the operand of a `NOT` or a term of a junction, where an arm that quietly vanished changes the result set.
- **A narrower position takes its own accept set.** Where a rule narrows a position below the general `terminal` value set, the write side **MUST** accept only what that position can lex, rather than delegate to the shared value validator:
  - `likeOperand : STRING | PARAMETER`;
  - a braced `MATCHES` member (`valueListItem : primitive | PARAMETER | terminologyFunction`, where `primitive` names the `BOOLEAN` token but `BOOLEAN`, declared after `IDENTIFIER`, never lexes, so `MATCHES {true}` is refused);
  - the bare `matchesOperand : terminologyFunction`, which admits no other call;
  - and `limitValue : INTEGER | PARAMETER`.
- **Junctions.** A junction **MUST** carry at least one term and an operator from its vocabulary, checked as a comparison operator is, whether it came from a constructor or a struct literal.
  Where two functions decide the same question about a node (is it a class or a junction?) they agree, so a `VERSION` class expression is never reclassified as a grouping and dropped from the emitted chain.
- **Refusals of caller text are value-free.** The refusal of a `MATCHES {uri}` operand, of a class predicate, and of emitted text that fails verification (§ Emission verified after emission, `Build()`'s projection check included):
  - **MUST** name the coordinate and the structural reason (the path, the predicate, the clause or the item, the component or condition that failed, a value-free position such as a byte offset);
  - and **MUST NOT** reproduce value content: no URI text, in whole or in part, no projection text, no string, regex, comment or `TERM_CODE` display-name content of a predicate, and not the parser's own syntax message, which echoes source.

  These errors are the return value of `Build` and `Emit`, and a class predicate is where openEHR carries the identifiable root (`[ehr_id/value='…']`, `[subject/external_ref/id/value='…']`), so a verbatim body would move a patient identifier into whatever a consuming CDR does with an error.
  A refused predicate is named in an elided form that keeps every path, operator, node code, term code, numeric literal and delimiter, and replaces only the content of the regions the bracket scan tracks as content (§ The class predicate positions); an unterminated region keeps its opening delimiter, since which construct was left open is the finding.
  The renderer is exported as `aql.RedactPredicateValues`.

### Canonical value spellings

These value-level rules extend the canonical write form, whose home is [wire.md § REQ-055](wire.md#req-055--wire-boundary).
Each is derived from the vendored grammar profile (`resources/aql/grammar/active/`, ADR [0007](../adr/0007-aql-antlr-grammar-profile.md)), which is the authority ([§ REQ-109](#req-109--aql-static-lint)'s syntax floor); no rule here widens the profile.

- **String literals.** Emission **MUST** use the grammar's `ESCAPE_SEQ` (a backslash before the escaped character), never the SQL-style doubled quote, and spell these characters as follows:
  - a C0 control that has an `ESCAPE_SEQ` spelling (`\a \b \f \n \r \t \v`): escaped, so canonical AQL stays single-line and printable;
  - a byte that begins no valid UTF-8 sequence: `OCTAL_ESC` (`\NNN`), the grammar's only spelling for an arbitrary byte, since the lexer decodes its input to runes and a raw byte would come back as U+FFFD on the second pass;
  - every other character the token admits raw, `"`, `?`, `*` and the controls with no `ESCAPE_SEQ` spelling included: as it is.

  The `STRING` token admits only `ESCAPE_SEQ`, `UTF8CHAR`, `OCTAL_ESC`, or a character that is neither a backslash nor the delimiter, so a doubled quote ends the token and opens another.
  Decoding **MUST** be the exact inverse, over all three escape shapes and both delimiter forms, and combine an adjacent UTF-16 surrogate pair into the character it denotes, reading a lone half as U+FFFD.
  The `STRING`, `DATE`, `TIME` and `DATETIME` tokens all admit `"…"` beside `'…'`, and a delimiter that survives into the decoded value turns a comparison into one that matches nothing.
  `UTF8CHAR` is exactly four hex digits, so a character outside the BMP has no other spelling.
- **Real literals** **MUST** be emitted in decimal form with a fractional part, so the text re-lexes as `REAL : DIGIT* '.' DIGIT+` and never as `INTEGER : DIGIT+` (`2.0`, never `2`), or be refused where no such spelling exists (an infinity or NaN).
  Scientific notation (`SCI_REAL` / `SCI_INTEGER`) is read and re-emitted in decimal form, a canonicalisation and not a value change.
- **Integer literals** are bounded by `aql.IntValue`'s `int64`; the residual of [§ REQ-113](#req-113--execution-oriented-parsed-aql-ast) covers only a literal outside that type.
- **A value-position function name** **MUST** lex as the grammar's `IDENTIFIER` or one of its `*_FUNCTION_ID` tokens.
  Shadowing turns on declaration order in `AqlLexer.g4`, not on keyword-ness: a name spelled like a keyword token declared before `IDENTIFIER` is refused, notably the aggregates (`COUNT`, `MIN`, `MAX`, `SUM`, `AVG`), which are real AQL functions but belong to `aggregateFunctionCall`, admitted in `SELECT` alone.
  `true` / `false` (`BOOLEAN`, declared after `IDENTIFIER`) are accepted, because the grammar admits them as a function name and refusing them would make `Emit` reject an AST `ParseQuery` produced.
  - A **projected** function name in `SELECT`, and its call, **MUST** be held to the same accept set with the aggregates admitted, each call in the shape its rule admits:
    - `COUNT` takes `DISTINCT? path` or a bare `*`, never a star beside arguments or `DISTINCT`;
    - `MIN` / `MAX` / `SUM` / `AVG` take exactly one identified path;
    - and no other projected call carries `DISTINCT` or a star.

    A call outside its shape is refused, never resolved by dropping a part: `COUNT(*)` emitted where the AST said `COUNT(c/x)` counts rows instead of values.
- **`TERMINOLOGY`** **MUST** carry exactly three string-literal arguments and no `DISTINCT` or star argument, on both carriers: the read and write sides model a value-position call and a projected one as different types (`aql.FuncCall` and `parse.FunctionCall`), and checking one would leave the other emitting text the parser rejects.
- **A parameter placeholder** **MUST** spell the grammar's `PARAMETER : '$' IDENTIFIER_CHAR` (a leading letter, then letters, digits and underscores) in every position that carries one, the in-text `LIMIT` / `OFFSET` operand included.
  This is [§ REQ-055](wire.md#req-055--wire-boundary) rule 4's channel for caller data, so an unchecked name would let the injection guard's own recommendation emit a structure-changing query: `$p AND c/secret = 1` re-parses as two predicates where the caller wrote one.
  A parameter is not a projection: `columnExpr` has no `PARAMETER` alternative while a function argument (`terminal`) has, so a bare parameter as a `SELECT` item **MUST** be refused, while one inside a projected call's arguments stays legal.
  `aql.PathValue` is exempt from the closure: it is one of the identifier positions REQ-055 rule 3 emits verbatim by contract, developer-authored, and not the channel for caller data.
- **A `MATCHES {uri}` operand** **MUST** be refused unless, lexed alone between the braces, it is the grammar's `URI` token and no token declared before `URI` claims it; `TERM_CODE` (`<term>::<code>`) is the case in force.
  The check follows the token's own decomposition (`URI : URI_SCHEME ':' URI_HIER_PART ( '?' URI_QUERY )? ( '#' URI_FRAGMENT )?`), not a flat union of the delimiter sets, which is broader than any single position admits: `%` leads `URI_PCT_ENCODED` and needs two hex digits, `[` / `]` occur only in an `URI_IP_LITERAL` host, and `#` separates the fragment once and appears nowhere inside it.
  It is not an RFC 3986 parser: a spellable but structurally odd URI (an empty host, a dotted quad out of range) reaches the wire, where the backend stays the authority (PROBE-021).

#### Verbatim text on the read side

`aql.IdentifiedPath.Raw`, `aql.PathSegment.Predicate` and `parse.ClassExpr.Predicate` report what was written, trivia included.
The SDK **MUST** expose its own trivia model as `aql.StripPredicateTrivia`, for a consumer that compares that text rather than re-emitting it, so that the consumer does not model the lexer's skipped trivia a second time; the structured predicates of [§ REQ-113 § Structured node predicates](#structured-node-predicates) let most consumers compare components instead.

### Single-token identifier positions

Three positions carry exactly one `IDENTIFIER` token: a `SELECT` `AS` alias (`selectExpr : columnExpr (AS aliasName=IDENTIFIER)?`), and a class's alias and RM type (`classExprOperand : IDENTIFIER variable=IDENTIFIER? …`).
A fourth carries exactly one `ARCHETYPE_HRID` or `PARAMETER` (`archetypePredicate`).
All four are spliced verbatim into the emitted text, so a caller string carrying a delimiter re-parses as a different query with no error: `Alias: "x, c/y"` emits `SELECT c/x AS x, c/y FROM …`, which is two projections.

Each of these positions **MUST** accept exactly the strings that lex as its one token, with the `VERSION` rule below as the one carve-out, on every write path that renders it: `aql.Builder` and `(*parse.Query).Emit` refuse the same strings, the builder after trimming surrounding whitespace on intake, a trim that only removes padding and so cannot make a splice.
The test is over what the lexer does, not over an alphabet.
A keyword declared before `IDENTIFIER` is refused, and the refused set is wider here than for a function name, because `functionCall` lists the `*_FUNCTION_ID` tokens as alternatives: `LENGTH` may name a function but is not an identifier.
`true` / `false`, declared after `IDENTIFIER`, are accepted.
`at0001` and `id123` lex as `AT_CODE` / `ID_CODE` and are refused, while `AT0001` is a valid alias, because the `at` / `id` prefixes are lower-case literals rather than the case-insensitive letter fragments the keywords use.

`VERSION` is not an identifier but a separate alternative of `classExprOperand` (`VERSION variable=IDENTIFIER? …`).
It **MUST** be refused as an alias on every carrier and as the RM type of a `parse.ClassExpr` whose `Version` flag is unset, and accepted as the RM type where the class model has no flag (`aql.Builder` and `aql.Containment` model a class as an RM-type string).
On `parse.ClassExpr` the flag is the carrier, and an unflagged `RMType: "VERSION"` would emit text that re-parses with the flag set, an AST the caller did not write; on the builder the spelling is the carrier, so `FROM VERSION v` and `EHR e CONTAINS VERSION v CONTAINS COMPOSITION c` stay buildable.
A `VERSION` class takes no archetype predicate on either carrier, since `versionPredicate` has no archetype alternative (§ Emit-side structural parity).

### The class predicate positions

`parse.ClassExpr.Predicate`, the bracket text that is neither a bare archetype HRID nor a `$param`, is spliced verbatim as the identifier positions are, and admits the same substitution: `Predicate: "a/b='c'] CONTAINS OBSERVATION o[d/e='f'"` emits a whole extra containment term that re-parses cleanly.
One field feeds both `classExprOperand` alternatives, whose accept sets differ:

| Carrier | Grammar | Admits |
|---|---|---|
| a class | `pathPredicate` | `standardPredicate \| archetypePredicate \| nodePredicate` |
| a `VERSION` class | `versionPredicate` | `LATEST_VERSION \| ALL_VERSIONS \| standardPredicate` |

A write path **MUST** hold the field to the grammar of the position it renders, not to one accept set: validating it uniformly would both emit `VERSION v[at0001]`, which the parser rejects, and refuse `VERSION v[LATEST_VERSION]`, which the extractor produces.
`aql.ValidatePathPredicate` and `aql.ValidateVersionPredicate` expose the two accept sets.

#### The `VERSION` position is decided whole

`versionPredicate` is a fixed three-way choice that does not recurse into the position, so its shape is decidable in one pass, and the guard **MUST** accept exactly that shape:

- one of the two keywords, matched case-insensitively once every trivia rule the lexer skips is removed (whitespace, a `--` comment, a `UNICODE_BOM` wherever it stands);
- or one `standardPredicate`, with exactly one top-level comparison operator (`<=` and `>=` count as one operator each) and a non-blank operand on each side.

A top-level `AND`, `OR` or `MATCHES` is therefore refused here, where `versionPredicate` has no junction alternative, though the same text is lawful `nodePredicate` at a class; the keyword counts only as a whole word outside literals, comments and nested brackets, so `a/brand > 1` and `a/b_and_c = 1` are untouched.
The operands are left to the parser: `objectPath` recurses (`pathPart : IDENTIFIER pathPredicate?` reaches `nodePredicate`, which reaches `objectPath`), and judging them would take the sub-grammar validator this requirement does not build.
The position's shape is decided; its operands are not.

#### The class position is guarded on bracket escape

Emission writes `"[" + Predicate + "]"`, so the text can change the query's structure only by ending that bracket early.
The guard **MUST** refuse exactly the text that escapes its bracket, and nothing else: a malformation that stays inside the bracket is the parser's to reject (`Emit` surfaces that verdict, § Emission verified after emission), while a conservative `nodePredicate` approximation would refuse predicates `ParseQuery` accepts, since a node predicate lawfully carries `AND`, `OR`, quotes and spaces.
Escape is decided over the regions in which a bracket character is not a delimiter, each recognised as the lexer recognises its token:

- a string literal, `'…'` or `"…"`;
- a contained regex (`CONTAINED_REGEX`, whose body admits both brackets and the closing brace), matched as a whole token by the lexer's longest match, its optional `; 'label'` tail included, or not at all: `{/[0-9]+/}` and `{/a{2}/}` are content, and a `{` that completes no token is an ordinary character;
- a comment (`COMMENT` is skipped rather than channelled, so it survives into the text): `-- ` and a body running to a line break, or a bare `--` followed at once by a line break;
- a `TERM_CODE` display name, `|…|`, transparent to quotes, braces and dashes, so `at0001,SNOMED-CT::22298006|Barrett's oesophagus|` is ordinary clinical AQL;
- a balanced nested path predicate, as in `a[at0001]/b='c'`.

A region still open where the text ends escapes, because the end of the supplied text is not the end of the query and the region runs on through the emitted `]` into the clauses after it: an unterminated literal, an unclosed regex body and a `-- ` comment with no line break inside the text all escape.
Balance holds in both directions: an unclosed `[` lets the emitted `]` close the inner bracket and the outer one close on whatever `]` follows.
A bare `--` at the very end starts no comment, since the byte after it is the emitted `]` and not a line break, so `[a/b='c'--]` is a contained malformation while `[a/b='c'-- ]` is the substitution.

This accept set is a lexical condition, not a token, so it has no token names to walk.
At both bracket positions and in both directions, accepted text never re-parses as a different query, and refused text is never text the parser reads back unchanged; [PROBE-090](conformance.md#probe-090--aql-emission-round-trip-closure) checks both against a generated corpus.

### Emit-side structural parity

`(*parse.Query).Emit` **MUST** refuse, with an error wrapping `aql.ErrInvalidQuery`, every tree shape in the table below.
A shape is refused because the grammar cannot carry it as written, or because it carries a content field `Emit` would not render: a guard that passes a field the emitter then drops hides the loss.

| Refused tree shape | Reason |
|---|---|
| A containment junction anywhere but at the end of its chain | The grammar cannot carry it; `Build()` holds the same rule ([§ REQ-163](#req-163--aql-write-side-expressivity-parity)). |
| A class node with no RM type that is not a `VERSION` class expression, or with a child-join of its own | The grammar cannot carry it. |
| A class expression carrying both an archetype and a predicate that is not that archetype's named form | The grammar cannot carry it: the two are the mutually exclusive spellings of its one `[...]` position, and emitting `Predicate`, which emission renders first, drops the archetype, a row filter, so the query returns more rows than the AST asked for. |
| A `VERSION` class expression carrying an RM type other than `VERSION`, or an archetype predicate | Unrendered content: the alternative has neither slot. `RMType: "VERSION"` beside the flag is accepted, since the extractor sets the two together. |
| A `$param` archetype flag with no parameter | Unrendered content. |
| A non-zero FROM root beside a root junction | Unrendered content. |
| A blank standing predicate | Unrendered content: it would emit `[   ]`. |
| Class data on a containment junction node | Unrendered content. |
| A negation on the FROM-root junction or on a junction operand | Unrendered content: `NOT` has no spelling there. Negation on a chain child, `NOT CONTAINS`, stays legal. |
| A structured predicate comparison with no verbatim predicate text | Unrendered content. |

The one pair the archetype-and-predicate row accepts is the named form `ParseQuery` produces ([§ REQ-113 § The class and alias-root brackets carry the same model](#the-class-and-alias-root-brackets-carry-the-same-model)).
Its `Predicate` reads, token by token on the lexer's default channel, as exactly `ARCHETYPE_HRID SYM_COMMA` and one name token (`STRING | PARAMETER | TERM_CODE | AT_CODE | ID_CODE`), with the HRID equal to `Archetype`, on a class that is neither `VERSION` nor a `$param` archetype; emitting `Predicate` then drops nothing.
The same dual-operand rule as the archetype-and-predicate row binds `aql.Comparison` (`Path` and `Left`) and `aql.MatchesExpr` (its three operand forms).
`ClassExpr.HasPredicate` is exempt: it is a derived flag carrying no content, so no value of it adds or removes emitted text.

Completeness is checked before placement: the read side infers a node's kind from an absent RM type, so an incomplete class node checked the other way round would be reported as a misplaced junction the caller never wrote.
The parity with `Build()` is deliberately not total: `aql.Builder` also requires an alias, which `classExprOperand` takes as optional, so requiring one in `Emit` would refuse valid AQL `ParseQuery` had just accepted.
The extractor never builds a violating tree; these rules bind a consumer that assembles or rewrites an AST by hand.

### Emission verified after emission

Every rule above decides one position from the text handed to it, and that is not enough: a `CONTAINED_REGEX` body can complete a token inside the bracket text and still reach beyond it (`{/a\/}`), so which token the lexer forms depends on text the predicate does not contain.
No per-position validator decides that, and refusing `{/a\/}` outright would refuse text `ParseQuery` reads back unchanged in every ordinary query.

`(*parse.Query).Emit` **MUST** read the emitted text back with the parser before returning it, and refuse, with an error wrapping `aql.ErrInvalidQuery` (and `aql.ErrSyntax` as well where the parser refuses the text) and a diagnostic that follows § The emission closure property, text that does not re-parse or that re-parses as a different query.
`aql.Builder.Build` verifies the projection list it renders against the same definition ([§ REQ-163 § `Build()` verifies the projection it emitted](#build-verifies-the-projection-it-emitted)).
A different query is one that adds or removes a class expression, a projection, a predicate leaf or a whole clause, or that changes a containment connective, a negation, or the grouping of containment operands across operators.
A substitution confined to the text of one path position is outside the check, since REQ-055 rule 3 emits a path verbatim by contract.

`Emit` **MUST NOT** refuse text that re-parses as the same query by that definition, except a shape the skeleton below cannot model.
`Emit`'s comparison runs over an encoding-independent skeleton of what it renders, not over field-by-field equality.
It keeps the structure the text carries, including each containment junction's operator at its coordinate, leaves out the read-side carriers a constructed AST does not populate, and factors out the encodings the emitted text erases:

- same-operator junction nesting;
- one-term junctions;
- the bracketing of a containment chain;
- the two carriers of a comparison's path;
- edge whitespace on path text;
- a sole unaliased star item against the bare `SELECT *` flag.

A shape the skeleton cannot model makes `Emit` refuse rather than compare equal to itself.
There is no unverified `Emit`.

### Out of scope

- **Guarding path text against splices.** A path is emitted verbatim by REQ-055 rule 3's contract, so `aql.Exists("c/x AND c/y = 1")` emits text that re-parses as a junction rather than one `EXISTS`.
  Paths are developer-authored identifiers, and caller data goes through `aql.Param`, which is guarded.
  Guarding `aql.IdentifiedPath.Predicate` and `aql.PathSegment.Predicate` with the bracket-escape scan would be a scope change.
- **A full `nodePredicate` validator** at the class position.
  The guard decides bracket escape, which closes the silent class and refuses nothing `ParseQuery` accepts; a malformation inside the brackets (`[a b c]`) is left to the parser, and `Emit` surfaces its verdict.

### Acceptance

[PROBE-090](conformance.md#probe-090--aql-emission-round-trip-closure) checks that every value kind survives emit, parse and compare in every value position, on both carriers.
The guards derived from the grammar are checked against the vendored grammar by tests that read their cases from it, or generate them where a position has no token names to walk, rather than against a list a maintainer has to remember to update.
The accept direction has rows of its own, one per encoding the emitted text erases, so an over-refusal fails as surely as a missed splice.
String-literal closure is also fuzzed.

---

## REQ-163 — AQL write-side expressivity parity

The write side of `openehr/aql` **MUST** be able to construct the vocabularies its own read side already models at the class, projection and row-bound positions:

- the **containment algebra** (negated containment and sibling junctions) and **in-text paging**;
- the deprecated **`TOP`** clause;
- the **version predicate** on a `VERSION` class expression;
- the **standing comparison** in class position;
- and the **typed projection** (`DISTINCT`, `AS`, aggregate and function calls, literal items).

Each is already parsed, structured and re-emitted by [§ REQ-113](#req-113--execution-oriented-parsed-aql-ast)'s AST, so what this requirement adds is the carriers, their guards, and the canonical spellings that make the mirror byte-exact, not a new model.

Without these carriers a caller who needs one of the constructs abandons the typed builder for a hand-assembled string, the one construction route [§ REQ-055](wire.md#req-055--wire-boundary)'s injection guard cannot cover.
Two shapes the SDK itself recommends depend on them: the explicit version predicate `aql_version_no_predicate` asks for ([§ REQ-161 § `aql_version_no_predicate`](#aql_version_no_predicate), [SPECPR-481](https://openehr.atlassian.net/browse/SPECPR-481)), and REQ-161's suppression shape `… CONTAINS VERSIONED_COMPOSITION vo[uid/value=$vo] CONTAINS VERSION v[ALL_VERSIONS]`.

Every rule below is derived from the vendored grammar profile (`resources/aql/grammar/active/`, [ADR 0007](../adr/0007-aql-antlr-grammar-profile.md)), which stays the authority.

### Containment algebra and in-text paging

The builder expresses three forms the parse side models:

- **Negated containment**: `CONTAINS … NOT CONTAINS …`, per the grammar's `classExprOperand (NOT? CONTAINS containsExpr)?`.
- **Sibling containment junctions**: `AND` / `OR` over containment operands, parenthesised only where nesting departs from the default precedence (`NOT` binds tightest, then `AND`, then `OR`, mirroring the parse profile).
- **In-text `LIMIT n [OFFSET m]`**, as an explicit opt-in, so a bound survives stored-query registration; envelope paging (`Query.Fetch` / `Query.Offset`) stays the default.

The grammar takes a parenthesised group as a whole `containsExpr` alternative (`classExprOperand (NOT? CONTAINS containsExpr)? | '(' containsExpr ')'`), so no `CONTAINS` keyword may follow a containment junction, and deeper nesting is written inside the junction's operands.
`Build()` **MUST** refuse, with an error wrapping `aql.ErrInvalidQuery`, a junction placed anywhere but at the end of its chain, whether through `Containment.Contains` / `Containment.NotContains` or across repeated `Builder.Contains` / `Builder.NotContains` calls, which emit as one chain, and never re-shape the emission to make the placement expressible: distributing the chain tail under the junction's operands changes what the query asks.

### The `TOP` clause and the row-bound channels

`aql.Builder` constructs the deprecated `SELECT TOP n [FORWARD|BACKWARD]` in its plain and directed forms, so a consumer can author the shape for a corpus, a migration or a round-trip fixture without hand-assembling AQL text.
The vocabulary is the shared `aql.TopClause` ([§ REQ-113 § The `TOP` carrier](#the-top-carrier)), and its godoc states the upstream deprecation and names the in-text `LIMIT` with `ORDER BY` as the replacement.

- A negative `TOP` count **MUST** fail `Build()` with an error wrapping `aql.ErrInvalidQuery`: the profile's `top` production admits no sign, so a negative count could only emit text the parser rejects.
- A query has three row-bound channels: in-text `LIMIT` / `OFFSET`, the request envelope (`Query.Fetch` / `Query.Offset`), and `TOP`, itself an in-text row bound.
  `Build()` **MUST** refuse a query that sets more than one, with an error wrapping `aql.ErrInvalidQuery`, rather than combine them silently; QUERY Release-1.1.0 § 4.4.3 additionally forbids `TOP` beside `LIMIT`.

### The version-predicate carrier

`versionPredicate : LATEST_VERSION | ALL_VERSIONS | standardPredicate`.
The write side **MUST** carry that position as a **sealed sum** in `openehr/aql` with **exactly three** shapes: the two keywords, and one `standardPredicate` comparison carrying `aql.Comparison` rather than a second, structurally identical vocabulary.
The sum follows the package's existing `aql.Value` / `parse.SelectExpr` style, an unexported marker method.
The production is a fixed three-way choice that does not recurse into its own position, the property [§ REQ-119 § The class predicate positions](#the-class-predicate-positions) relies on to decide the position in one pass; a fourth shape would be a grammar change, not an extension.

The predicate reaches a class expression through a **`VERSION`-fixed constructor** (`aql.Version(alias, pred)`), so the RM type at that position is the SDK's spelling and never a caller string.
Absence stays legal: a `VERSION` class with no predicate **MUST** stay buildable, since REQ-161 advises on it with a Warning and does not refuse it.

- The rendered bracket text **MUST** be held to `aql.ValidateVersionPredicate`, the guard `(*parse.Query).Emit` applies at this position, so `Build` and `Emit` accept the same brackets.
- A version predicate on a class node **not** spelled `VERSION` **MUST** be refused: the grammar reaches `versionPredicate` from the `VERSION` alternative of `classExprOperand` alone.
- An **archetype** predicate on a `VERSION` node stays refused: the `VERSION` alternative has no archetype slot at all (§ REQ-119 § Single-token identifier positions states the same rule for the read-side carrier).

The combinators return a `Containment` rather than an error, so a misuse **MUST** surface at `Build()` as an error wrapping `aql.ErrInvalidQuery`, the route the containment algebra already uses, and never be absorbed into a shape that means something else.

### The standing-predicate carrier

`pathPredicate : '[' (standardPredicate | archetypePredicate | nodePredicate) ']'`.
The write side **MUST** carry a class-position standing predicate spelling the **`standardPredicate`** alternative, `objectPath COMPARISON_OPERATOR pathPredicateOperand`: **exactly one comparison**, never a junction, since that production has no junction alternative of its own.
The bound belongs to the **carrier**, not to the position: the bracket's third alternative, `nodePredicate`, is defined over `AND` / `OR`, so a junction class predicate is legal AQL that no builder carrier spells (§ Out of scope).

The comparison **MUST** be the `aql.Comparison` shape, so that a `WHERE` comparison, the parsed class predicate (`parse.ClassExpr.PredicateComparison`) and the built class predicate share **one** comparison model.
The left-hand path is relative (it binds no FROM alias), and the value renders through § REQ-119 § Canonical value spellings.

Four refusals bind, and each **MUST** wrap `aql.ErrInvalidQuery`:

- a **junction** receiver: a junction node carries no class of its own, so it has no bracket position (the content-free reasoning § REQ-119 § Emit-side structural parity applies to `parse.Containment`);
- a node that **already carries an archetype predicate**: archetype and standing are the two mutually exclusive spellings of the one `[...]` position, the dual-operand rule of [§ REQ-119 § Emit-side structural parity](#emit-side-structural-parity);
- a node that **already carries a standing predicate**: one bracket renders one predicate, and `standardPredicate` has no conjunction to carry a second, so the first comparison would be dropped.
  Two conditions on one node belong in `WHERE`, or, under `NOT CONTAINS`, where the node's alias binds no result row, in a restructured term;
- a **`VERSION`-spelled node**, routed as below.

The rendered bracket text **MUST** be held to `aql.ValidatePathPredicate`, the guard the position already has.

### One carrier per grammar position

`versionPredicate` admits `standardPredicate`, so a comparison **on a `VERSION` node is legal AQL** and **MUST** be expressible, through the version-predicate carrier's comparison shape (`aql.VersionCompare`) alone.
The standing-predicate carrier applied to a `VERSION`-spelled node **MUST** be refused with an error wrapping `aql.ErrInvalidQuery`, and the error **SHOULD** name `aql.VersionCompare`, the constructor which does carry it.
The two accept sets differ in both directions, `versionPredicate` carrying no node predicate and `pathPredicate` no `LATEST_VERSION`, so routing a class-position call to the version position silently would change the caller's accept set with no diagnostic (§ REQ-119 § The class predicate positions); refusing instead is loud and named.

Both spelling tests, this routing refusal and the version carrier's refusal of a node not spelled `VERSION`, **MUST** match the RM type ASCII-case-insensitively, so each accept set is exactly the spellings the lexer reads as the keyword.
`VERſION` (U+017F, Unicode-fold-equal to the keyword and unreadable to the lexer) is an ordinary class, and a Unicode fold would either answer a class-position call on it by naming a constructor that fixes the RM type to `VERSION`, or carry a `versionPredicate` bracket onto a node the lexer reads as an ordinary class.
In this carrier the spelling *is* the carrier, since the builder's class model has no `Version` flag ([§ REQ-119 § Single-token identifier positions](#single-token-identifier-positions)).

### The typed projection model

`parse.SelectClause` is the **reference shape**, and the write side **MUST** mirror it rather than grow a second projection vocabulary:

- a clause-level `Distinct` flag and star form;
- an ordered item list;
- and per item an expression plus an optional `AS` alias, over a sealed expression vocabulary of a path, a function or aggregate call (name, arguments, `DISTINCT` and star flags), a literal, and a star item.

`columnExpr : identifiedPath | primitive | aggregateFunctionCall | functionCall`, and as on the read side one call type models both call productions, so a shape rule written once binds both carriers.

The rules that already govern a projected item bind the builder unchanged:

- the projected function name, held by `aql.ValidateSelectFuncName`, the projected call shapes, `TERMINOLOGY`'s arity and argument type, and the bare-parameter refusal ([§ REQ-119 § Canonical value spellings](#canonical-value-spellings));
- the single-token rule for an `AS` alias, held by `aql.ValidateIdentifier` ([§ REQ-119 § Single-token identifier positions](#single-token-identifier-positions)).

`Build()` **MUST** refuse, with an error wrapping `aql.ErrInvalidQuery`, a typed construction the grammar does not admit at the position it renders:

- a call-shaped `Lit` (`literalColumn` is `columnExpr`'s `primitive` alternative, `Fn` being the call route);
- an aggregate-shaped argument inside `Fn` (aggregates are admitted as a top-level `columnExpr` alone);
- a function-shaped comparison operand in either bracket carrier (`pathPredicateOperand : primitive | objectPath | PARAMETER | ID_CODE | AT_CODE` has no `functionCall`);
- and a `VersionPredicate` shape outside the three § The version-predicate carrier seals, reachable by embedding the exported interface, which is refused rather than panicking ([idiom.md § No panics](idiom.md#no-panics)).

#### Read-side mirror duty

For **every** construct this requirement adds, `Build()` → `parse.ParseQuery` → `(*parse.Query).Emit` **MUST** be **byte-identity**, not merely re-parseable.
Identity is the right bar: the read side re-emits a class bracket verbatim and normalises a projected function name, so anything weaker would let the two sides disagree about the canonical spelling of a construct they both model, a disagreement invisible to a parseability check.
§ Canonical spellings for the new constructs is what makes identity reachable.

### `Col` stays lenient, and `Build()` verifies what it emitted

`aql.Col(path)` renders its argument into the projection list verbatim, with no check beyond emptiness, so `Col("COUNT(x) AS n")` builds, and `Col("a, b")` would build into two projections.
Two rules settle it, opposite in direction because the two failure modes are.

#### `Col` keeps its signature and its leniency

A `Col` whose text re-parses as a **single** projected item *and* introduces **no clause-level flag the builder did not record** (a function call, an aliased item, a path) **MUST** be accepted: it is loud, ordinary AQL that says what the caller wrote, and refusing it would break callers for no correctness gain.
This applies § REQ-119's own rule, that refusal is reserved for the silent-substitution mode.
The typed constructors above are the recommended route and their godoc says so; `Col` is not deprecated.

#### `Build()` verifies the projection it emitted

`Build()` **MUST** read the `SELECT` clause it emitted back before returning it, and refuse, with an error wrapping `aql.ErrInvalidQuery` and a diagnostic that follows [§ REQ-119 § The emission closure property](#the-emission-closure-property), a clause that does not re-parse or that re-parses as a different query ([§ REQ-119 § Emission verified after emission](#emission-verified-after-emission)).
It verifies its projection list only, because the rest of the builder's vocabulary takes no free text: `aql.Containment` holds an RM type, an alias and an archetype id, its brackets render from typed comparisons, and `WHERE` arrives structured.
For the projection, a different query is a `Col` whose text splits into two or more items, spills out of `SELECT` into another clause, or adds a clause-level flag: the silent mode, valid AQL asking a different question.
The compared structure **MUST** include the item count, each item's `AS` alias and the clause-level flags (`DISTINCT`, and the deprecated `TOP` with its `FORWARD` / `BACKWARD` direction, which `top : TOP INTEGER direction?` consumes into the clause as `selectClause` consumes `DISTINCT`), compared after factoring out the encodings the emitted text erases:

- a sole unaliased star item and the bare `SELECT *` flag are one encoding;
- path text enters its slot with edge whitespace trimmed;
- and a closed top-level comment run reads as whitespace.

A `Col` text that leaves a lexical region **open** where the clause ends (a string literal, a comment run, a bracket or a parenthesis) **MUST** be refused, even where it would read back unchanged.
What closes the region is the text after the projection, so the same text is a loud error in one query and a silent substitution in the next.
No typed constructor produces such a region, so the refusal reaches only a legacy `Col` carrying half a token.

So `Col("COUNT(c/uid/value) AS n")` and `Col("-- note\nc/uid/value")` are accepted, while `Col("DISTINCT c/uid/value")` (one item plus an unrecorded `DISTINCT`), `Col("a, b")` (two items), `Col("BACKWARD c/uid/value")` beside a recorded `Top(5)` (one item plus an unrecorded direction, a byte-stable query for the *last* five rows), `Col("-- x\n*")` (the parser reads the bare star form, the builder recorded one non-star item) and a `Col` leaving a region open are refused.
The refusal follows the value-free rule of [§ REQ-119 § The emission closure property](#the-emission-closure-property): it names the item index and the clause, never the projection text or the parser's own message.

### Canonical spellings for the new constructs

These spellings extend the canonical write form whose home is [§ REQ-055](wire.md#req-055--wire-boundary).
Each is what `(*parse.Query).Emit` produces at that position, and any other spelling would break the mirror duty's byte identity.
`Build()` and `Emit` **MUST** emit the spellings in this table:

| Construct | Canonical spelling |
|---|---|
| Containment junctions and negation | One space around `AND`, `OR` and `NOT CONTAINS`; parentheses only where precedence requires them. |
| In-text paging | `LIMIT n[ OFFSET m]`, emitted after `ORDER BY` in clause order. |
| `TOP` (deprecated) | `SELECT [DISTINCT ]TOP <n>[ FORWARD\| BACKWARD] <items>`: keywords upper-cased regardless of source casing, single spaces, the direction only where the source or the builder set one. |
| Projected function and aggregate names | **Uppercase ASCII.** Required for identity, not taste: the extractor upper-cases every projected name while the emitter renders the name as carried, so a lower-case builder spelling comes back upper-cased and the round trip is not identity. The upper-casing **MUST** be ASCII-only: a Unicode mapping folds some non-ASCII letters *into* the ASCII alphabet (`ı` → `I`) and so turns a spelling the lexer cannot tokenise into a legal-looking one, the rule the read side already states for itself. |

`Build()` **MUST** emit the spellings in the next table as well; `Emit` renders the same ones, except a class bracket it re-emits verbatim (§ REQ-119).

| Construct | Spelling |
|---|---|
| Version keywords | `VERSION v[LATEST_VERSION]` / `[ALL_VERSIONS]`: uppercase, no padding inside the brackets. The casing follows [§ REQ-055](wire.md#req-055--wire-boundary) rule 1's keyword *principle*, not its enumeration, which is closed over the clause keywords and names neither these nor `TOP`. The read side matches them case-insensitively and re-emits the bracket verbatim, so identity requires the write side to pick exactly one spelling. |
| A comparison in either bracket | `<path> <op> <value>`, **one space each side of the comparison operator**, no padding inside the brackets: `vo[uid/value = $vo]`, `v[commit_audit/time_committed > $since]`. This is what `aql.Comparison` already renders in `WHERE`, so the builder keeps **one** comparison renderer rather than two that can drift. The path is trimmed of EDGE whitespace on intake, as `aql.Builder` already trims its identifier positions (§ REQ-119 § Single-token identifier positions): the trim widens the accept set by one harmless equivalence and cannot manufacture a splice, and without it a padded path would be a second spelling of one bracket — padding this row forbids. |
| `DISTINCT` (clause) | Uppercase, directly after `SELECT` and **before** the deprecated `TOP` clause — `selectClause : SELECT DISTINCT? top? selectExpr …` — one space after. |
| `AS` | Uppercase, one space each side: `COUNT(c/uid/value) AS n`. |
| An aggregate's `DISTINCT` argument | `COUNT(DISTINCT c/uid/value)` — uppercase, one space after; arguments comma-separated with one space after each comma, the spelling REQ-055 rule 5 fixes for the item list. |
| Star | `COUNT(*)` with no padding; a sole unaliased star item emits as the bare `SELECT *` form, which is how it re-parses. |

### Acceptance

Each new construct is tested to build, emit and pass the identity round trip `Build → ParseQuery → Emit`, and the verb functions and the struct builder emit the same strings for it.
A builder-emitted `… CONTAINS VERSION v[LATEST_VERSION]` draws no `aql_version_no_predicate` from `lint.LintString`, and REQ-161's suppression shape `… CONTAINS VERSIONED_COMPOSITION vo[uid/value = $vo] CONTAINS VERSION v[ALL_VERSIONS]` builds end to end, draws no `aql_versioned_object_unreferenced`, and gets the same containment codes from `(*aql.Builder).VerifyContainment` as from the linter.
The `VersionPredicate` refusal is a returned `aql.ErrInvalidQuery`, not a panic.
Each `Col` shape above has its own row with its expected verdict, one per region kind for the open-region refusal, and the accept direction has rows of its own, one per encoding the emitted text erases, so an over-refusal fails as surely as a missed splice.

### Out of scope

- **A FROM-root bracket in any spelling: standing, archetype or version predicate.**
  `Builder.From` takes an RM type and an alias and nothing else, so `FROM VERSION v` builds and stays predicate-less, and `aql.Version` places its bracket on a `CONTAINS` operand.
  For the standing predicate, [§ REQ-055](wire.md#req-055--wire-boundary) rule 6 keeps the `WHERE <alias>/ehr_id/value = $param` form deliberately, noting the `EHR e[ehr_id/value=$param]` spelling is equally valid AQL and not what the builders emit; this requirement adds the standing predicate to `CONTAINS` operands and does not disturb that choice.
  [PROBE-097](conformance.md#probe-097--aql-semantic-and-portability-lint-corpus) records the root bracket as the residual unreachable shape.
- **A FROM-root containment junction** (`FROM COMPOSITION c1 OR COMPOSITION c2`, which the read side models via `parse.FromClause.Junction`): the builder keeps a single root class until a consumer needs write-side root junctions, so `Builder.From` / `Builder.FromEHR` take one class and a builder-emitted FROM root is never parenthesised.
- **A parameterised `TOP`.** `top : TOP INTEGER direction?` admits no `PARAMETER` ([§ REQ-113 § The `TOP` carrier](#the-top-carrier)), so there is nothing to add.
- **The class position's other bracket alternatives** — a `nodePredicate` junction, a node id, a name slot, a regex match.
  All are legal AQL that [§ REQ-113 § Structured node predicates](#structured-node-predicates) already structures on the read side and that no builder carrier spells.
  Adding them is a widening of the write vocabulary and rides its own requirement; until then the read/write parity this REQ establishes is bounded by the `standardPredicate` and `archetypePredicate` alternatives, and § The standing-predicate carrier says so.
- **Deprecating or narrowing `Col`.** Settled the other way above, deliberately.

- **Probes:** [PROBE-088](conformance.md#probe-088--aql-builder-containment-and-paging-stability) (the builder golden set and its refusals); [PROBE-097](conformance.md#probe-097--aql-semantic-and-portability-lint-corpus) arm (c) (the read/write parity corpus).

---

## REQ-109 — AQL static lint

The SDK **MUST** ship a building-block parse + lint pipeline for hand-written, imported, or `aql.NewQuery(literal)` AQL, so CI validators, MCP tools, and pre-flight checks can catch defects before a query reaches the CDR — without replacing the typed builders (REQ-055) or the CDR as the execute-time semantic authority (PROBE-021).

The lint runs three layers and **MUST** be collect-all (return every issue, not fail-fast), matching REQ-102.

### Syntax floor — the SDK grammar profile

The parse layer **MUST** validate against the **SDK-maintained grammar profile**, not a live pull from specifications.openehr.org: foundation openEHR AQL (QUERY Release-1.1.0) plus the documented `SDK-AQL-NNN` deltas in [`resources/aql/grammar/DIVERGENCES.md`](../../resources/aql/grammar/DIVERGENCES.md) (ADR [0007](../adr/0007-aql-antlr-grammar-profile.md)).
Deltas are classed **relaxation** (admits more than official AQL, e.g. `SELECT *`) or **correction** (fixes a foundation weak spot).

#### Lint-clean is not spec-conformance, and not execute-success

Because the profile deliberately admits relaxations and the CDR is the path authority, a query the SDK lints clean can still be rejected on execution; conversely the lint targets only the contract below.

The profile **MUST** admit every query the foundation grammar admits.
A query that a conformant engine answers and the profile refuses therefore lies outside foundation AQL, as an engine grammar extension, and [PROBE-107](conformance.md#probe-107--upstream-aql-suite-admissibility-corpus) records it by name, with that reason, as a decline; an unexplained refusal is a defect in the profile.
Recording a decline never widens the profile: admitting an engine extension is a separate `SDK-AQL-NNN` relaxation.

### Layer 1 — Syntax

- Empty / whitespace-only input **MUST** yield code `aql_empty` (before parse).
- Input that does not parse as `selectQuery` per the profile **MUST** yield code `aql_syntax`.
- The `aql_syntax` `Detail` **SHOULD** carry the parser's `line:column` when the parser reports a position, and **SHOULD NOT** invent one when it does not, because a reader takes a printed coordinate for a real one: a `parse.SyntaxError` whose `Pos` is zero prints no `0:0` from `Error()`, and its `Detail` carries no `L:C:` prefix.
- `parse.Parse` returns a `*parse.SyntaxError` wrapping the building-block sentinel `aql.ErrSyntax`; the `aqlvalidation.Validate` bridge maps `aql_syntax` / `aql_empty` to `validation.ErrAQLSyntax` via `validation.Issue.Err`.

### Layer 2 — Shape (AST walk, no CDR)

Each code in this table and in the § Layer 3 table **MUST** be raised exactly when its *Fires when* cell holds, at the severity the table gives.
`aql_select_star` is the one exception: it **SHOULD** be raised when its cell holds, and **MUST NOT** be raised for `COUNT(*)`.

| Check | Code | Severity | Fires when |
|---|---|---|---|
| Alias binding | `aql_unknown_alias` | Error | An identified path's root alias binds to no class in FROM / CONTAINS. |
| Attribute case | `aql_path_attribute_case` | Error | An attribute name in a path carries an ASCII upper-case letter: a step of an identified path, or a step of a relative path inside a structured predicate ([§ REQ-113 § Structured node predicates](#structured-node-predicates)) on an alias root, a path step or a FROM / CONTAINS class. Every attribute the pinned Reference Model declares is lower case, so the step names nothing. It fires once per such name. A predicate the parser does not structure is not read. |
| Identifiable scope | `aql_from_archetype` | Warning | FROM/CONTAINS names no archetype HRID, no `$param` archetype predicate, no `VERSION` operand and no `EHR` root in any letter case, so the query scans broadly. |
| Bound parameters | `aql_unbound_param` | Error | When linting an `aql.Query`, a `$name` the query references has no key in `Query.Parameters`. |
| Unused parameters | `aql_unused_param` | Warning | A `Query.Parameters` key is never referenced; the code is advisory. |
| Deprecated `TOP` | `aql_deprecated_top` | Warning | The query carries a `SELECT TOP` clause; it fires once per query. The remedy is `LIMIT` with `ORDER BY`: the modifier is deprecated from QUERY Release-1.1.0 and slated for removal ([§ REQ-113 § The `TOP` carrier](#the-top-carrier)). |
| `TOP` with `LIMIT` | `aql_top_with_limit` | Error | The query uses `TOP` and the `LIMIT` clause together, which QUERY Release-1.1.0 § 4.4.3 forbids. Fires in addition to `aql_deprecated_top`. |
| `TOP` with envelope `fetch` | `aql_top_with_fetch` | Error | The query carries `TOP` and the request envelope carries a row limit: the openEHR Query API's common parameters state that `fetch` cannot be combined with AQL-top. Reachable only when the caller supplies `Options.Query`, and keyed on its `Fetch` alone: an `Offset` does not fire it, the exclusion naming the row limit only. Fires in addition to `aql_deprecated_top`. `Build()` also refuses an envelope `Offset` beside a `TOP` ([§ REQ-163](#req-163--aql-write-side-expressivity-parity)), because it declines to author two row bounds; the lint reads a query it did not author, so it reports only the exclusion the Query API states. |
| Bare / mixed `SELECT *` | `aql_select_star` | Warning | A projection carries a `*` item; `COUNT(*)` does not fire it. A `*` item uses the [SDK-AQL-002](../../resources/aql/grammar/DIVERGENCES.md#sdk-aql-002--select-) relaxation, and official QUERY 1.1.0's only use of `*` is `COUNT(*)`. |

The three `TOP` codes **MUST** key on the `TOP` clause's presence in the source, not on a decoded count: a count too large to represent leaves no bound to read ([§ REQ-113](#req-113--execution-oriented-parsed-aql-ast)'s residual), and the query carrying it still draws them.
The other half of each pair is read from its own channel: the in-text `LIMIT` clause for `aql_top_with_limit`, the request envelope's `Fetch` for `aql_top_with_fetch`.

SELECT-present-with-≥1-projection and FROM-present are guaranteed by a successful parse (the grammar requires both), so they raise no Layer-2 issue.

The lint reads keywords, operators, function names and RM class names in any letter case, as the grammar's lexer and the [§ REQ-160](#reachability-semantics) relation read them.
Respelling only the letter case of one of them **MUST NOT** change which codes the lint raises, or their severities.
An attribute name in a path is not folded, which is why `aql_path_attribute_case` exists.
Its span covers the step when the step belongs to the identified path; otherwise it covers the path, or the class's RM type, whose predicate holds the step.

An alias is not folded either.
One declared in FROM / CONTAINS or by a SELECT `AS` may be written in lower, upper or mixed case, and every use **MUST** repeat that spelling: `aql_unknown_alias` matches an alias exactly.
This is stricter than the QUERY text, which calls variable names not case-sensitive.
When the unbound alias differs from a declared one only in letter case, the issue's `Detail` **SHOULD** name the declared spelling.

Layer 2 additionally carries the semantic, portability and path-shape check groups of [§ REQ-161](#req-161--aql-semantic-and-portability-lint), whose codes, severities and firing rules are specified once there.

Two grammar-admitted, server-executable shapes resolve without `aql_unknown_alias`:

- An `ORDER BY` key that is a bare identifier (no path tail, no predicate) and names no FROM alias **MUST** be resolved against the SELECT `AS` aliases (`SELECT x/y AS score … ORDER BY score`) before `aql_unknown_alias` is raised.
  FROM/CONTAINS is consulted first, and nothing else resolves through a SELECT alias: an identifier matching neither, an `AS` alias with a path tail (`score/magnitude`), and a `WHERE` path rooted on an `AS` alias each raise `aql_unknown_alias`.
- A bare `true` / `false` keyword in a value position (a comparison operand such as `WHERE s/is_queryable = true`, a SELECT item, a function-call argument) **MUST** be read as a boolean literal, not as a path root, by the lint and by the structured AST of [§ REQ-113](#req-113--execution-oriented-parsed-aql-ast) alike, so a consumer reads the construct the same way in every clause.
  With a predicate or a path tail (`true/nested`), or as an `ORDER BY` key (`orderByExpr` admits no literal), the keyword is a path and keeps its alias check.

### Layer 3 — Path & template (only when a compiled OPT is supplied)

The rule of § Layer 2 binds this table too: each code is raised exactly when its cell holds.

| Check | Code | Severity | Fires when |
|---|---|---|---|
| Archetype membership | `aql_archetype_not_in_template` | Error | A literal archetype HRID in FROM/CONTAINS is absent from the compiled OPT (`Compiled.AllByArchetypeID`); a `$param` archetype predicate is not a literal HRID and does not fire it. |
| Path in template | `aql_path_not_in_template` | Warning | An identified path resolves under none of the archetype subtrees its alias's class names, as the walk below decides. |

A `$param` archetype predicate (`[$name]`, `parse.ClassExpr.ParamArchetype`) anchors no `aql_path_not_in_template` resolution either: there is no literal subtree to walk, and the CDR resolves the bound scope at execution ([PROBE-021](conformance.md#probe-021--aql-parse-error-mapping)).

`aql_path_not_in_template` resolution walks the archetype-scoped compiled subtree (predicate-aware first-child descent) and warns only on high-confidence structural divergence: a path segment naming an attribute that does not exist on a node that has modelled attributes.
It stays silent on unmodelled RM-leaf attributes (e.g. `/value/magnitude`) and on descent below the modelled tree, because the CDR, not the OPT index, is the path authority (PROBE-021).
A path through a non-mandatory RM attribute the OPT did not constrain can still warn, which is why the check is a Warning.

### Issue model and entry points

- `openehr/aql/lint` owns its own `lint.Issue` / `lint.Result` / `lint.Severity`.
  `lint.Result.OK()` is true when no **Error**-severity issue is present (Warnings do not make a result not-OK).
- `lint.LintString(q, *Options)` is the raw-AQL entry; `lint.Lint(doc, *Options)` lints an already-parsed `*parse.Document`.
  `Options{Compiled, Query, Relation}` is nilable, but the three fields do not gate alike: nil `Compiled` drops Layer 3 and nil `Query` drops the parameter-binding checks, whereas a nil `Relation` switches nothing off, so the REQ-161 semantic group always runs ([§ REQ-161 § Relation supply](#relation-supply)).
- `aqlvalidation.Validate(q aql.Query, c *templatecompile.Compiled) validation.Result` is the seam: it parses `q.Q`, runs the layers, and maps `lint.Issue` → `validation.Issue` (code and severity carried verbatim) so callers already using `ValidateComposition` get one uniform `Result`.
  It lives in `openehr/validation/aqlvalidation`, apart from the instance validators, so that importing `openehr/validation` links no AQL parser ([module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence)).
- `aqlvalidation.ValidateWithTypeRelation(q, c, rel *contain.TypeRelation) validation.Result` is the same seam with the containment relation supplied by the caller, and `Validate(q, c)` is its nil call.
  [§ REQ-161 § Relation supply](#relation-supply) states the obligation to offer it and the bound on what a supplied relation changes.
- A published lint code **MUST NOT** be re-pointed at another rule: consumers filter and gate on `Code`, so a code keeps its meaning for as long as it exists.

### Value-free lint diagnostics

`lint.Issue` names a finding's location by quoting it: `Detail` is free text and `Path` is a path spelling.
Its fields follow the two-class rule of [§ REQ-168 § The two field classes](clinical-modeling.md#the-two-field-classes), classed as follows:

| Field | Class |
|---|---|
| `Code`, `Severity`, `Span` | **value-free**: never carries source text |
| `Detail`, `Path` | **value-bearing**: may embed query spellings |

`Path` is value-bearing deliberately: a path spelling carries its own predicates, so `o/data[at0001, 'Systolic']` is a path *and* a value.
A consumer that treats `Path` as safe because it looks structural is the failure this classification exists to prevent.
Naming `Detail` and `Path` value-bearing is what lets a disclosure boundary drop them deliberately instead of scrubbing them in hope.

`Issue.Span` is populated from the token positions lint already visits, and is zero when the issue is not attributable to a position.
It **MUST** be the same type as the drop record's span (`lint.Span` re-exports `parse.Span`), so a consumer compares a lint span with a drop-record span directly.

### Out of scope

- **Terminology** (`TERMINOLOGY()` / `MATCHES` value-set membership), function signatures, `ORDER BY` type checking, and version-predicate **value** semantics beyond parse.
  Predicate *presence* on a `VERSION` operand is [§ REQ-161](#req-161--aql-semantic-and-portability-lint)'s `aql_version_no_predicate`.
- **CDR-grade path resolution**: full AQL-path-to-canonical-path mapping (node-id-on-structural-attribute vs canonical placement) is PROBE-021 territory; Layer 3 is best-effort, hence `aql_path_not_in_template` is a Warning.
- **Re-emission / pretty-printing**: canonical re-emission from the structured AST is [§ REQ-113](#req-113--execution-oriented-parsed-aql-ast)'s `(*parse.Query).Emit`.

- **Probes:** PROBE-028 — lint fixed query strings against the grammar profile (+ a compiled OPT for Layer 3) and assert a stable issue-code multiset; [PROBE-107](conformance.md#probe-107--upstream-aql-suite-admissibility-corpus) — every query the vendored upstream suites send and an engine answers parses under the profile, or is a recorded decline; [PROBE-096](conformance.md#probe-096--aql-value-free-structured-diagnostics) — the lint issue's value-free fields carry no query text, and its span is the drop record's span type.

---

## REQ-160 — AQL containment admissibility relation

The SDK **MUST** ship a building-block relation that answers, for two AQL class expressions, whether a containment route under the pinned openEHR Reference Model connects them (§ Verdicts: route existence, not a claim of RM truth), so that the lint (REQ-161), the builder's verification ([§ REQ-161 § Builder adapter](#builder-adapter)) and future consumers judge `FROM … CONTAINS …` shapes from one shared RM-derived relation instead of each guessing.
Architecture and motivation: [ADR 0017](../adr/0017-aql-semantic-layer.md).

### Nil and zero relations

#### A nil relation is the default relation, on the receiver too

Every seam that *takes* a relation reads nil as the default ([§ REQ-161 § Relation supply](#relation-supply)).
The relation's own **methods MUST read a nil receiver the same way**, returning what the default relation would return rather than dereferencing it: nil is caller-constructible and documented, so panicking on it is what [idiom.md § No panics](idiom.md#no-panics) forbids, and a nil that means "the default" when passed but panics when used would be one spelling with two meanings.
This holds for the extension method as much as for the verdict methods: a consumer that conditionally extends a relation it might not hold (`var rel *TypeRelation; …; rel = rel.WithOverlay(e)`) writes the shape the nil convention invites.
A nil relation is therefore **not** the *zero* relation: the zero value is a real relation that knows no classes and answers UnknownClass for every one (§ Verdicts), a different answer rather than the same one reached differently.

### Verdicts

The relation answers two questions — **operand containability** (may this class be a `CONTAINS` operand at all?) and **pair admissibility** (does a containment route connect this ancestor/descendant pair?) — each yielding exactly one verdict from the table below; **ByReference** arises only on the pair question.

Beside the two verdict questions the relation answers one reachability query that yields no verdict: `Unavoidable(anc, via, desc)` reports whether every containment route from `anc` to `desc` passes through `via`.
It is computed over the same overlaid graph as the verdicts, so a consumer overlay changes its answer, and like every other method it reads a nil receiver as the default relation.
Its consumer is `aql_contains_redundant_step` ([§ REQ-161 § `aql_contains_redundant_step`](#aql_contains_redundant_step)).

#### Admissible is route-existence, not a claim of RM truth

A verdict is derived from the pinned BMM's by-value graph plus the overlay edges.
On the RM-faithful subgraph an Admissible verdict *is* genuine RM nesting; where the default relation is deliberately loose, at the family-agnostic `VERSION` hub, an Admissible verdict can cover a pair that cannot actually nest ([§ Overlay edges](#overlay-edges-default-relation)).
Consumers needing RM-exact verdicts use a stricter profile once one ships (§ Out of scope).

| Verdict | Meaning |
|---|---|
| **Admissible** | Pair: a route (by-value or overlay) connects the pair using only non-reference edges. Operand: a legal `CONTAINS` operand. |
| **Never** | Pair: no route connects the pair — a `CONTAINS` over it is always empty, and a `NOT CONTAINS` over it is trivially true. Operand: not a containment target at all. |
| **ByReference** | The pair is resolvable only through a reference hop that some engines implement as containment; portable behaviour is not guaranteed. |
| **UnknownClass** | The class is not one the relation knows (pinned BMM + overlays). Unknown is **not** wrong: future RM releases, demographic deployments, and dialects are legitimate sources of unknown names. |

The pair question is total: it answers for **every** ordered operand pair.
The pair verdict **MUST** be UnknownClass when either operand is UnknownClass, and otherwise Never when either operand's containability is Never; REQ-161 reports such operands once, through the operand-level codes, and suppresses the pair codes.
Neither short-circuit can pre-empt an overlay edge: every class an overlay edge of the relation in use names is thereby a known, containable class of that relation (§ Containable operands, § Extensibility).

### Reachability semantics

- `CONTAINS` admissibility **MUST** be **descendant-at-any-depth reachability** over the RM's by-value composition graph, derived from the pinned BMM through `openehr/rm/rminfo` (REQ-048).
  `COMPOSITION CONTAINS ELEMENT` is Admissible even though levels are skipped: level-skipping is the QUERY specification's own worked-example behaviour (its `COMPOSITION c CONTAINS OBSERVATION o` matches observations nested under `SECTION`s, against prose that reads the left operand as the direct "parent object") and the engine consensus, the depth question being part of the open containment gap.
  A direct-child rule would flag valid queries.
- The derivation crosses non-LOCATABLE by-value classes (e.g. `EVENT_CONTEXT` on the way from `COMPOSITION` to `ITEM_STRUCTURE`) while collecting only containable classes, and a reference-typed attribute (`OBJECT_REF` and its descendants) ends structural reachability: reference hops are overlay data, never derived.
- An attribute whose declaring class (`rminfo` `DeclaredOn`) is `LOCATABLE` or `PATHABLE` **MUST NOT** be a containment edge: AQL `CONTAINS` navigates archetyped content, not audit or reference metadata.
  The exclusion is read from the declaration site, not from a list of attribute names, so it covers a housekeeping attribute a later pin declares and spares a content attribute that happens to share a name.
  On the current pin it resolves to the six `LOCATABLE` housekeeping attributes (`name`, `uid`, `links`, `feeder_audit`, `archetype_details`, `archetype_node_id`) and to no `PATHABLE` property (the pin's `parent` back-reference is a BMM function, not an attribute).
  Without the exclusion an `ELEMENT` would reach `CLUSTER` through `LOCATABLE.feeder_audit` (a `FEEDER_AUDIT`) → `originating_system_audit` (a `FEEDER_AUDIT_DETAILS`) → `other_details` (an `ITEM_STRUCTURE`), a by-value path the RM permits but AQL containment does not, which is why `ELEMENT CONTAINS CLUSTER` is **Never** (§ Acceptance).
- **Abstract classes stand for their concrete kinds, on both operands:** a pair **MUST** be Admissible when *any* concrete-descendant combination is (`rminfo` `ConformsTo` / `ConcreteDescendants`).
  `ENTRY CONTAINS CLUSTER` is Admissible because a concrete entry admits it.
- **ByReference closes over routes, not edges:** a *route* is any edge sequence connecting the pair in the closure, derived and overlay edges alike.
  The pair verdict **MUST** be Admissible when at least one route crosses no ByReference edge, ByReference when routes exist but every one crosses at least one ByReference edge, and Never when no route connects the pair.
  `FOLDER CONTAINS OBSERVATION` is therefore ByReference: every route crosses one of `FOLDER`'s reference hops (§ Overlay edges).
- Class-name matching **MUST** be ASCII-case-insensitive against the BMM's canonical spelling (engines and client libraries round-trip lower-case class spellings; a case mismatch is not a semantic difference).

### Containable operands

A class **MUST** be a legal `CONTAINS` operand exactly when it:

- **conforms to** one of `LOCATABLE`, `VERSIONED_OBJECT`, or `VERSION` (conformance, not name equality, so every `VERSIONED_*` container the pin ships, `VERSIONED_FOLDER` and `VERSIONED_PARTY` included, and the concrete `ORIGINAL_VERSION` / `IMPORTED_VERSION` are containable);
- or is `EHR` (the non-LOCATABLE root);
- **or is named as an endpoint of an overlay edge of the relation in use**: naming a class in an edge makes it a known, containable class of that relation (§ Extensibility).

A data value (`DV_*`) or another known class outside that set therefore has containability **Never**: values are read through paths, not bound by `CONTAINS`.
Demographic classes are containable exactly when the pinned BMM declares them; their presence is a property of the pin, not of this contract.

**An engine that binds a class outside this set does so by an engine extension.**
An engine that answers `COMPOSITION c CONTAINS EVENT_CONTEXT ec` is an example: `EVENT_CONTEXT` conforms only to `PATHABLE` and is reached by value through `COMPOSITION.context`.
The default relation still answers Never for such a class: an engine extension is not an RM containment, and the compatibility guard (§ Overlay edges) does not reach it.
A deployment that relies on the extension **MAY** declare it as a consumer overlay edge (`COMPOSITION` → `EVENT_CONTEXT`, § Extensibility), which makes the class containable for that relation only.

### Overlay edges (default relation)

The default relation **MUST** carry the overlay edges below, the facts the BMM cannot express, each with its citation and each conformance-aware on both endpoints like any derived edge: an edge naming `VERSIONED_OBJECT` covers every class conforming to it.
They take part in the same reachability closure as the derived edges.

| Edge | Verdict contribution | Basis |
|---|---|---|
| `EHR` → `COMPOSITION`, `EHR_STATUS`, `FOLDER`, `EHR_ACCESS`, **and those four payload families' `VERSIONED_*` and `VERSION` tier** | Admissible | RM EHR IM: the EHR references its versioned objects — ehr_access, ehr_status, directory (folders) and compositions — so those four families are `EHR`'s **direct** overlay endpoints; the EHR IM versions no parties, so the demographic family is not a direct endpoint (a deployment that wants demographic containment adds a consumer edge, § Extensibility). `FROM EHR e CONTAINS …` is the specification's own canonical example shape and consensus engine behaviour. |
| For each **payload family** X in {`COMPOSITION`, `EHR_STATUS`, `EHR_ACCESS`, `FOLDER`, `PARTY`} — the payload classes of the concrete `VERSIONED_*` containers the pinned BMM ships — the pair of edges `VERSIONED_X` → `VERSION` and `VERSION` → `X` | Admissible | Version-tier mechanics are RM facts for every pin-shipped container (RM common IM version tier; the EHR IM's versioned objects; `VERSIONED_PARTY` is the demographic IM's, shipped by the pin), and the QUERY grammar admits the operands while CDR practice resolves them. `VERSION.data` is generic — in the pinned tables `ORIGINAL_VERSION.data` is literally typed `T`, and [REQ-048](reference-model.md#req-048--rm-meta-model-introspection-surface) leaves generic-parameter resolution out of scope — so the payload pairing is **cited overlay data reviewed on a BMM pin bump** ([ADR 0001](../adr/0001-bmm-version-bump-runbook.md) drift triage), not derivable from `rminfo`. |
| `FOLDER` → `COMPOSITION` | **ByReference** | `FOLDER.items: List<OBJECT_REF>`; engines that support it resolve the reference hop — engine-specific, verify before use. (`FOLDER → FOLDER` is by-value and needs no overlay.) |
| `FOLDER` → `VERSIONED_COMPOSITION` | **ByReference** | RM Common IM, directory package: a folder's items are "references to other (usually versioned) objects", `VERSIONED_COMPOSITION` its worked example. The hop lands on the version tier, so `FOLDER CONTAINS VERSION` and `FOLDER CONTAINS VERSIONED_COMPOSITION` resolve **ByReference** through the version-tier edges above, as the `FOLDER → COMPOSITION` shorthand above does for the payload; the onward routes are in § Acceptance. |

The `VERSION` node is **family-agnostic** in the default relation.
Because `VERSION.data` is generic (one RM class for every payload), a bare `VERSION` or `VERSIONED_OBJECT` operand stands for any payload family (abstract expansion, § Reachability semantics), and the shared node carries no payload-family state along a route.
The relation therefore **MAY** over-admit a rare cross-family version-tier pair rather than assert an impossibility it cannot cheaply prove.
Because the shared node offers a plain (non-reference) exit to *any* payload family, a route with no ByReference edge exists, and the route-closure rule (§ Reachability semantics) yields **Admissible** for both `VERSIONED_FOLDER CONTAINS COMPOSITION` (route `VERSIONED_FOLDER → VERSION → COMPOSITION`) and a hub-mediated `EHR … PARTY` pair.
This looseness is deliberate and bounded: it is a *missed defect*, never a false Error ([§ REQ-161 § Flagging policy](#flagging-policy)), and it never contradicts the EHRbase-compatibility guard below.
What waits for its own requirement is a **family-precise** profile that would refuse these pairs, and shipped demographic containment edges (§ Out of scope, § Extensibility).
The version-tier overlay set is pin-reviewed data that follows the pin: a pin that adds a versioned container fails a test until its family edges are carried.

Under the default relation nothing contains `EHR`: a pair with `EHR` as the descendant **MUST** verdict Never, and no reference hop beyond the table above is admitted.
The default relation expresses the openEHR RM and specifications only, never one vendor's storage model: **an RM-valid pair that a conformant engine (EHRbase among them) admits MUST NOT verdict Never.**
An engine admits a pair when it answers the query with the rows the pair selects.
A pair an engine accepts but answers with no rows is consistent with Never, and a class an engine binds by an engine extension is outside this guard (§ Containable operands).

### Archetype/class conformance

For a class expression carrying a literal archetype predicate, the HRID's type segment (`openEHR-<pkg>-<TYPE>.<concept>.vN`) **MUST** conform to the declared class (ConformsTo, case-insensitive): `ENTRY o[openEHR-EHR-OBSERVATION.…]` conforms; `EVALUATION o[openEHR-EHR-OBSERVATION.…]` does not.
The HRID is decomposed by [REQ-120](reference-model.md#req-120--rm-identifier-parsing-and-derivation)'s canonical parser, `rm.ParseArchetypeID`, with no second lexical logic here.
HRID token shape is grammar-guaranteed on parsed input; an unparseable HRID reaching the relation directly, or a declared class or type segment the relation does not know, **MUST** yield **UnknownClass**, never a false mismatch: a mismatch is only ever asserted between two known classes.

### Extensibility

An overlay edge, default and consumer alike, is a record of two class-name endpoints (`From`, `To`) and a by-reference marker: a plain edge contributes **Admissible** to routes through it, a marked edge contributes **ByReference** (§ Reachability semantics, the ByReference closure).
Consumers **MUST** be able to extend the relation with their own overlay edges without forking it, as an immutable copy (matching the `Containment` algebra's style) that leaves the default relation unaltered.
A consumer edge **MUST** be honoured as written, whether or not its endpoints are BMM-known, which is what lets a dialect deployment (e.g. demographic containment) verify its own queries without false findings:

- a BMM-known endpoint matches by conformance (like any derived edge, § Overlay edges: `WithOverlay(VERSIONED_OBJECT → PERSON)` covers every `VERSIONED_*`);
- an endpoint the pin does not know matches by exact name;
- the endpoints become known, containable classes of the extended relation (§ Containable operands);
- and a pair a consumer edge names takes that edge's verdict, since the UnknownClass / Never short-circuit (§ Verdicts) applies only to classes no edge of the relation names.

### Acceptance (representative rows — the full relation is derived)

| Pair | Verdict |
|---|---|
| `EHR CONTAINS COMPOSITION` · `EHR CONTAINS OBSERVATION` | Admissible (overlay + closure) |
| `COMPOSITION CONTAINS OBSERVATION` · `COMPOSITION CONTAINS ELEMENT` | Admissible |
| `SECTION CONTAINS SECTION` · `CLUSTER CONTAINS CLUSTER` · `INSTRUCTION CONTAINS ACTIVITY` | Admissible |
| `ENTRY CONTAINS CLUSTER` (abstract operand) · `EHR_STATUS CONTAINS ELEMENT` · `FOLDER CONTAINS FOLDER` | Admissible |
| `EHR CONTAINS VERSION` · `VERSIONED_OBJECT CONTAINS VERSION` · `VERSION CONTAINS COMPOSITION` | Admissible (overlay) |
| `VERSIONED_FOLDER CONTAINS VERSION` · `VERSION CONTAINS EHR_ACCESS` (version tier covers every pin-shipped container) | Admissible (overlay) |
| `VERSION CONTAINS PARTY` | Admissible (the pin ships `VERSIONED_PARTY`; PARTY payload family) |
| `FOLDER CONTAINS COMPOSITION` · `FOLDER CONTAINS OBSERVATION` (every route crosses the reference hop) | ByReference |
| `FOLDER CONTAINS VERSION` · `FOLDER CONTAINS VERSIONED_COMPOSITION` (the reference hop lands on the version tier) · `FOLDER CONTAINS EHR_STATUS` (onward through the family-agnostic `VERSION` node) | ByReference |
| `VERSIONED_FOLDER CONTAINS COMPOSITION` · `EHR CONTAINS PERSON` (deliberate over-admission via the family-agnostic `VERSION` hub, which offers a plain route to any payload *family* — a missed defect, never a false Error; § Overlay edges) | Admissible |
| `VERSIONED_FOLDER CONTAINS VERSIONED_COMPOSITION` (a folder version's items reference versioned compositions) · `VERSIONED_PARTY CONTAINS VERSIONED_COMPOSITION` (through the family-agnostic `VERSION` node; bounded over-admission) | ByReference |
| `EHR CONTAINS VERSIONED_PARTY` (no route reaches the demographic *container*: the hub over-admits payloads, reaches a `VERSIONED_*` container only through the `FOLDER → VERSIONED_COMPOSITION` reference hop, and no default edge leads to `VERSIONED_PARTY`) | Never |
| `OBSERVATION CONTAINS COMPOSITION` · `OBSERVATION CONTAINS SECTION` · `OBSERVATION CONTAINS EVALUATION` · `ELEMENT CONTAINS CLUSTER` | Never |
| `COMPOSITION CONTAINS COMPOSITION` · `EHR_STATUS CONTAINS EHR_STATUS` (engines accept both and answer with no rows) | Never |
| `COMPOSITION CONTAINS EVENT_CONTEXT` (`EVENT_CONTEXT` conforms only to `PATHABLE`; an engine answering it does so by an engine extension, § Containable operands) | Never |
| any class `CONTAINS EHR` (default relation) | Never |
| `DV_TEXT` as a `CONTAINS` operand (containability question) | Never |
| `COMPOSITION CONTAINS DV_TEXT` (pair question: a Never-containability operand) | Never |
| `FOO_BAR` as either operand — pair and containability questions alike | UnknownClass |

### Out of scope

- Row-semantics adjudication (REQ-161 carries the single advisory; the relation answers admissibility only);
- per-engine capability profiles (valid AQL an engine does not implement — deferred to its own requirement);
- a **family-precise profile** that would tighten the family-agnostic `VERSION` hub's over-admissions (§ Overlay edges) into RM-exact verdicts — deferred to its own requirement;
- and shipped **dialect containment edge data** (e.g. demographic containment) — the extension mechanism above is in scope, that edge data waits for its own requirement.

The version tier of every pin-shipped `VERSIONED_*` container, `VERSIONED_PARTY` included, is **not** deferred: it is default overlay data (§ Overlay edges).

- **Probes:** [PROBE-097](conformance.md#probe-097--aql-semantic-and-portability-lint-corpus) (the lint corpus and the builder verification's parity arm); [PROBE-107](conformance.md#probe-107--upstream-aql-suite-admissibility-corpus) (the compatibility guard's evidence gate over every clause family the vendored upstream suites query, the FROM/CONTAINS combinations included)

---

## REQ-161 — AQL semantic and portability lint

REQ-109's Layer 2 carries three additive check groups, each specified below:

- **semantic checks**, backed by the REQ-160 relation: RM-impossible shapes a CDR will typically accept and answer with zero rows;
- **portability checks**, for behaviours the QUERY specification leaves open, each citing the open specification question or community source where one exists;
- **path-shape checks**, for authoring rules the other groups do not reach: an identified path stepping through an unconstrained **repeating** RM attribute, a **row-bounded** query with no total order, an **unaliased** projection, the *path* source of engine-defined row multiplicity, and a **containment step that provably does nothing**.

The layer contract is preserved: AST walk, collect-all, no CDR, no OPT ([§ REQ-109 § Layer 2](#layer-2--shape-ast-walk-no-cdr)).
The relation and the RM facts the path-shape checks read are derived in-process from the pinned BMM, the latter through `openehr/rm/rminfo` ([REQ-048](reference-model.md#req-048--rm-meta-model-introspection-surface)), so the checks need no input beyond what Layer 2 already takes.
Each path-shape rule is stated as a rule by the openEHR authoring guidance and is decidable from the query text plus the pin; path shape is otherwise covered only by the OPT-gated Layer 3.
The codes reuse the REQ-109 issue model verbatim ([§ REQ-109 § Value-free lint diagnostics](#value-free-lint-diagnostics)), with `Span` on the offending class expression, path segment, projection item or path, as each code's entry states.
The SDK adjudicates **no row semantics**: it does not deduplicate, reason about expected row counts, or refuse a row shape.
The two fan-out advisories, [`aql_fanout_row_grain`](#aql_fanout_row_grain) and [`aql_fanout_path_grain`](#aql_fanout_path_grain), are the entire footprint.

### Checks

The semantic and portability catalogue, one entry per code:

#### `aql_impossible_containment`

- **Check:** Impossible containment; **severity:** Error.
- **Fires:** An adjacent FROM/CONTAINS pair of containable operands whose REQ-160 **pair verdict** is **Never** **MUST** raise this code — including junction operands (checked against the junction's enclosing parent) and `NOT CONTAINS` pairs (the exclusion is trivially true: a dead constraint is equally a defect).
- **Silent:** On a pair with an UnknownClass or Never-containability operand ([§ Pair-check suppression](#pair-check-suppression)).

#### `aql_contains_not_containable`

- **Check:** Non-containable target; **severity:** Error.
- **Fires:** A `CONTAINS` operand whose REQ-160 **containability verdict** is **Never** — a known class that is not a containment target, e.g. a `DV_*` type — **MUST** raise this code.
- **Silent:** `aql_contains_not_containable` is scoped to a `CONTAINS` operand, not to the FROM root.
  A **FROM root** whose REQ-160 containability verdict is Never (`FROM DV_TEXT t …`) **MUST** raise no containability code, in the single-root or the root-junction spelling: the catalogue above authorises that code for a `CONTAINS` operand only.
  The boundary binds that one code, not the root generally: a root's archetype predicate is checked by `aql_archetype_class_mismatch` exactly as a `CONTAINS` operand's would be, because that check is orthogonal to operand role, and an *unknown* root still raises `aql_unknown_rm_class` (unknown is not wrong).

#### `aql_archetype_class_mismatch`

- **Check:** Archetype/class mismatch; **severity:** Error.
- **Fires:** A literal archetype predicate whose HRID type segment does not conform to the declared class (REQ-160 § Archetype/class conformance) **MUST** raise this code.
- **Silent:** `$param` archetype predicates are skipped — the CDR resolves the bound scope at execution (PROBE-021), the same skip REQ-109 Layer 3 applies.
  An archetype predicate whose declared class or HRID type segment is UnknownClass raises no `aql_archetype_class_mismatch`, since the class expression already carries `aql_unknown_rm_class` through the class-token or the type-segment arm, and REQ-160 § Archetype/class conformance yields no mismatch on an unknown name: one finding per defect, and no Error built on an unknown name.

#### `aql_unknown_rm_class`

- **Check:** Unknown RM class; **severity:** Warning.
- **Fires:** A class token whose REQ-160 verdict is **UnknownClass**, **or** a literal archetype predicate whose HRID type segment is not a class the relation knows (`ArchetypeMatches` → **UnknownClass**, REQ-160 § Archetype/class conformance), **MUST** raise this code, once per class-expression occurrence.
- **Why:** Warning, because unknown is not wrong (future RM, demographic deployments, dialects).
  This is the code that fires when `aql_archetype_class_mismatch` is suppressed for an unknown type segment, so an unknown name is never silent.

#### `aql_containment_by_reference`

- **Check:** Containment by reference; **severity:** Warning.
- **Fires:** A pair whose REQ-160 pair verdict is **ByReference** **MUST** raise this code — Warning, because the hop is engine-specific: verify against the target CDR.
- **Silent:** On a pair with an UnknownClass or Never-containability operand ([§ Pair-check suppression](#pair-check-suppression)).

#### `aql_version_no_predicate`

- **Check:** `VERSION` without predicate; **severity:** Warning.
- **Fires:** A `VERSION` class expression carrying no version predicate **MUST** raise this code.
- **Why:** The default is unspecified ([SPECPR-481](https://openehr.atlassian.net/browse/SPECPR-481)), so a portable query states `[LATEST_VERSION]` or `[ALL_VERSIONS]` explicitly.

#### `aql_versioned_object_unreferenced`

- **Check:** Unreferenced `VERSIONED_OBJECT`; **severity:** Warning.
- **Fires:** An operand **MUST** raise this code exactly when its class conforms to `VERSIONED_OBJECT` (REQ-160 § Containable operands: conformance, not a name list), it carries **no class-position predicate** of its own, and its alias roots no identified path outside FROM/CONTAINS: the step is redundant unless container-level attributes are read ([Discourse #14186](https://discourse.openehr.org/t/aql-versionedobject-grammar/14186)).
- **Silent:** A **class-position predicate** is any bracket the operand carries in class position, a standing predicate or an archetype predicate, and any one of them suppresses the code, because each selects the container and so is itself a container-level attribute read.
  A class-position predicate is not an identified path, so "outside FROM/CONTAINS" answers *referenced*, never *redundant*.
- **Why:** The motivating shape is `… CONTAINS VERSIONED_COMPOSITION vo[uid/value=$vo] CONTAINS VERSION v[ALL_VERSIONS]`, the canonical *all versions of one composition* query, where the step does work although no identified path names its alias.

#### `aql_fanout_row_grain`

- **Check:** Engine-defined row grain; **severity:** Warning.
- **Fires:** The code **MUST** fire exactly when an `AND` containment junction that no inherited negation covers has two or more **projected** aliases among its class-expression **leaves**: the class expressions reached by flattening nested `AND` junctions, descending into neither an `OR` junction nor a negated node.
  An alias is **projected** when it roots at least one SELECT column, **or** when the projection carries a `*` item in any form (bare `SELECT *`, mixed `SELECT *, col`).
  A star projects **every** alias bound in FROM/CONTAINS, because the engine returns a column per alias, and a star roots no identified path, so a SELECT-column-only reading would exempt the shape where the question is most live.
  Negation is inherited downward and alternation is not: a `NOT CONTAINS` at any depth above an `AND` junction, including through an intervening un-negated `OR`, covers it, while an un-negated `AND` nested under an un-negated `OR` is covered by nothing and fires.
- **Silent:** The code stays silent on:
  - an `AND` junction with one projected alias;
  - an `OR` junction with two, under named columns and under `SELECT *` alike, since alternation selects rows and does not multiply them;
  - `NOT CONTAINS (A AND B)` and `NOT CONTAINS (B OR (D AND E))`.
- **Why:** A `NOT CONTAINS` collapses to a boolean filter on the parent row and multiplies nothing inside its excluded subtree.
  Possibility is sign-blind and row multiplicity is not, which is why `aql_impossible_containment` checks a `NOT CONTAINS` pair and this code does not.
- **Detail:** `Detail` states that row multiplicity for this shape is engine-defined (result-shape semantics are an open specification question, SPECQUERY-9).
  It covers the **junction** source of engine-defined row multiplicity (§ No double-reporting).

#### Pair-check suppression

An operand whose verdict is **UnknownClass** or whose containability is **Never** **MUST** suppress the pair checks (`aql_impossible_containment`, `aql_containment_by_reference`) for every pair it participates in.

### Path-shape checks

The path-shape catalogue, one entry per code:

#### `aql_path_repeating_unpredicated`

- **Check:** Unconstrained repeating segment; **severity:** Warning.
- **Fires:** An identified path segment **MUST** raise this code exactly when it types to a **multi-valued** RM attribute (`rminfo` `IsContainer`) and carries **no predicate** of any kind, as follows:
  - a node id, a name and a standing comparison all count as a predicate: presence suffices and content is **not** judged (§ The conservative segment walk);
  - the code fires once per offending segment, with `Span` on that segment;
  - it fires in **every** identified path the document records (`SELECT`, `WHERE` and `ORDER BY` alike, `parse.Document.Paths`).
- **Why:** A `WHERE` filter or an `ORDER BY` key over an unconstrained repeating segment leaves the same which-occurrence question to the engine as a projected path.
  Whether `[at0006]` is the *right* node id is a template question owned by Layer 3's `aql_path_not_in_template`.

#### `aql_paging_no_order_by`

- **Check:** Row bound without a total order; **severity:** Warning.
- **Fires:** A query **MUST** raise this code exactly when it carries a **row bound** and no `ORDER BY`, at most once per query, `Detail` naming the channel(s) that carried the bound.
  A row bound is the **in-text** `LIMIT` clause (`parse.Document.HasLimit`; the profile's `limitClause` admits no `OFFSET` without a `LIMIT`, so keying on `LIMIT` misses no in-text spelling) or a supplied `Options.Query` whose `Fetch` **or** `Offset` is non-zero.
- **Silent:** A nil `Options.Query` leaves the envelope arm unable to fire, as it leaves the parameter-binding checks ([§ REQ-109 § Issue model and entry points](#issue-model-and-entry-points)), and a deprecated `TOP` is not a row bound here ([§ No double-reporting](#no-double-reporting)).
- **Why:** Without a total order a page boundary is engine-defined, so successive pages can repeat or drop rows.
  The envelope arm reads `Offset` as well as `Fetch`, unlike `aql_top_with_fetch`'s fetch-only scope: that code reports an exclusion the Query API *states*, while this one reports an unstable page boundary, which an offset into an unordered result produces on its own.

#### `aql_select_no_alias`

- **Check:** Unaliased projection; **severity:** Warning.
- **Fires:** A `SELECT` item **MUST** raise this code exactly when it is not a `*` item and carries no `AS` alias, once per item, with:
  - `Span` on the item where the item has a position of its own (a bare path);
  - the **zero `Span`** otherwise (a function-call or literal item, which `Detail`'s item ordinal locates instead), REQ-109's answer for an issue not attributable to a position ([§ REQ-109 § Value-free lint diagnostics](#value-free-lint-diagnostics)).
- **Silent:** A `*` item is exempt ([§ No double-reporting](#no-double-reporting)); a **bare alias** item (`SELECT o`) is **not** exempt, because it names no column either.
- **Why:** A stored-query contract and every column-addressed result reader depend on stable column names, and an unaliased column's name is engine-defined.

#### `aql_fanout_path_grain`

- **Check:** Path row grain; **severity:** Warning.
- **Fires:** A query **MUST** raise this code exactly when two projected identified paths rooted on the **same** alias diverge after their longest common prefix, **each** passing at least one unpredicated multi-valued segment at or after the divergence, the Cartesian-product shape.
  Its row multiplicity is engine-defined (result-shape semantics are an open specification question, SPECQUERY-9), which `Detail` states as its sibling's does.
  It fires **once per alias**, not once per pair: the first offending pair in document order fixes the `Span` (the later path of that pair) and is named in `Detail`, so a wide projection yields one advisory rather than a quadratic report.
- **Silent:** It stays silent on:
  - two paths whose multi-valued segments are all in their **common prefix** (one repeating scope, no product);
  - a pair in which **either** path predicates every multi-valued segment at or after the divergence;
  - paths rooted on **different** aliases (the junction question, `aql_fanout_row_grain`);
  - a bare `SELECT *`, which projects no identified path (the star's row-grain question belongs to the junction code).

#### `aql_contains_redundant_step`

- **Check:** Redundant containment step; **severity:** Warning.
- **Fires:** A `CONTAINS` operand **MUST** raise this code exactly when **all** of the following hold:
  - it is **not** the FROM root;
  - it is **not** a leaf of the containment tree;
  - it carries **no class-position predicate** of any kind, archetype, standing or version (`parse.ClassExpr.HasPredicate` false);
  - its alias roots **no** identified path outside FROM/CONTAINS;
  - it sits inside no containment **junction** and under no **negation**;
  - its class does not conform to `VERSIONED_OBJECT` (§ No double-reporting);
  - its class is **unavoidable** under the REQ-160 relation, every containment route from its parent operand's class to its child operand's class passing through it.
- **Why:** Removing such a step provably changes no result: `EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o` with `c` unreferenced, where every EHR→OBSERVATION route passes `COMPOSITION`.
  The rule is narrower than the guidance sentence it comes from ("containment is minimal"), and the narrowing is its substance:
  - an **avoidable** intermediate narrows the result (removing `SECTION s` from `… CONTAINS SECTION s CONTAINS OBSERVATION o` admits observations outside sections);
  - an unreferenced **leaf** is an existence filter;
  - inside a junction or under a negation, removing the step changes the junction's arity or the excluded set, which the relation alone cannot prove inert.

### The conservative segment walk

The repeating-segment check, and the divergence test `aql_fanout_path_grain` builds on it, share one walk.
It starts at the class the path's alias binds (`parse.Document.Classes`), types each step through `rminfo`'s flattened attribute typing, and reads multiplicity from `IsContainer`.
Segment predicate presence is read from the verbatim `parse.PathSegment` predicate text ([§ REQ-113](#req-113--execution-oriented-parsed-aql-ast)): presence only, never content, so no predicate spelling has to be understood for the check to be sound.

The walk ends **silently**, with no issue on that segment and none on any segment beyond it, at the first step it cannot type, and never guesses or descends a sibling.
It stops:

- at a **class the pin does not know** (the alias's own class included);
- at an **attribute the pin does not declare** on the current class, own or inherited;
- at an attribute whose declared type is **not a class of the universe**, which includes a BMM **generic parameter** (on the current pin `EVENT.data` is typed `T`, exactly as `ORIGINAL_VERSION.data` is, and [REQ-048](reference-model.md#req-048--rm-meta-model-introspection-surface) leaves generic-parameter resolution out of scope; [§ REQ-160 § Overlay edges](#overlay-edges-default-relation) records the same fact);
- and at a **`$param` archetype scope**, whose extent the CDR resolves at execution, the skip [§ REQ-109 § Layer 3](#layer-3--path--template-only-when-a-compiled-opt-is-supplied) and § Checks apply for the same reason.

This is the Layer-3 false-positive policy applied to a check that needs no OPT: a false Warning on a legal path is worse than a missed one, because the consumer that runs this in CI cannot tell them apart.

**The walk's reach on the current pin is bounded, and the bound is a property of the pin, not of this contract.** `OBSERVATION/data` types to `HISTORY` (single-valued) and `HISTORY/events` to `EVENT` as a container, the anchor case, which fires when unpredicated; the walk then stops at `EVENT/data`'s generic `T`.
A path continuing `…/items/value/magnitude` therefore raises one issue, on `events`, and not one per repeating segment further down.
The bound narrows where `aql_fanout_path_grain` can be witnessed without disabling it: on that canonical shape the one reachable container (`events`) sits in the two paths' common prefix, so a firing pair is one whose repeating segments are typed before the stop.
`SELECT o/data/events/time, o/links/meaning/value` diverges at the alias with an unpredicated container on each branch (`HISTORY.events` and `LOCATABLE.links`) and fires.
Widening the reach means carrying the RM's generic constraints as cited data in the BMM band, not changing this check (§ Out of scope).

### Flagging policy

The checks are **conservative**: a false **Error** is worse than a missed defect, so the semantic checks flag only what the pinned BMM and overlays prove.
An **Error** code (`aql_impossible_containment`, `aql_contains_not_containable`, `aql_archetype_class_mismatch`) **MUST** fire only on a verdict the relation proves, so where the relation is deliberately loose (the family-agnostic `VERSION` hub, REQ-160 § Overlay edges) the layer raises no Error the relation does not support.
An unknown name is **not** an error: it surfaces as the [`aql_unknown_rm_class`](#aql_unknown_rm_class) **Warning**, never as silence and never as an Error.
Every path-shape code is a **Warning**, and where a fact is not provable from the query text plus the pin, a path-shape check **MUST** stay silent rather than guess (§ The conservative segment walk).

### Relation supply

`lint.Options` **MUST** carry a nilable relation field, typed `*contain.TypeRelation` (the relation is over RM **type names**, and the type says so).
A caller with dialect overlay edges (REQ-160 § Extensibility) lints without false findings.

Every entry point that runs the semantic group **MUST** offer this supply, or the escape hatch reaches only the callers who bypass the seams:

- the read side through `lint.Options`;
- the write side through `(*aql.Builder).VerifyContainment` (§ Builder adapter);
- and the validation seam through `aqlvalidation.ValidateWithTypeRelation` (REQ-109 § Issue model and entry points), whose no-relation form `aqlvalidation.Validate(q, c)` **MUST** be exactly the nil call.

A relation is supplied, never gated: nil **MUST** select the REQ-160 default at every one of them, and never switches a group off.

A supplied relation governs the containment questions only: the five containment codes of § Checks and `aql_contains_redundant_step` (§ Path-shape checks).
`aql_contains_redundant_step` asks a containment-route question, which is what an overlay edge states, so it **MUST** be asked of the relation **in use**: consulting the default instead would raise a false finding on the dialect deployment [§ REQ-160 § Extensibility](#extensibility) exists to serve.
An overlay **MUST NOT** retire a Layer-1 syntax finding, a Layer-2 shape or parameter-binding finding ([§ REQ-109 § Layer 2](#layer-2--shape-ast-walk-no-cdr)), or a Layer-3 template finding.
The other findings of this requirement are out of its reach as well, because their checks put no question to the relation:

- `aql_version_no_predicate` and `aql_versioned_object_unreferenced` ask class questions of the pinned BMM directly, such as `VERSIONED_OBJECT` conformance, which no supplied relation answers differently;
- `aql_fanout_row_grain` reads the query's own shape;
- the segment walk's two codes ask class questions of the pin;
- and `aql_paging_no_order_by` and `aql_select_no_alias` are parse-only.

The scope is by code, not by layer: the containment codes are themselves Layer 2 (REQ-109 § Layer 2).

### Always on, never gated

The path-shape codes are all Warnings, so none of them flips `Result.OK()` ([§ REQ-109 § Issue model and entry points](#issue-model-and-entry-points)).
They **MUST** run whenever Layer 2 runs, with no option to enable them, as the semantic and portability groups do: a caller who does not want one filters the result by `Code`, which is value-free and stable, and a caller who gates CI on Warnings has opted into exactly this class of advice.
An `Options` field switching the group on would buy nothing, since the group has no Error member to hold back, and would give a nilable `Options` field a third meaning beside `Relation`, which selects a default, and `Compiled` and `Query`, which gate checks that have no input without them.

### No double-reporting

The guidance rules and the **two** documented sources of engine-defined row multiplicity are each covered by exactly one code.
Where a shape could attract two, this section fixes the owner:

- `aql_versioned_object_unreferenced` **owns** the unreferenced version container: an operand whose class conforms to `VERSIONED_OBJECT` ([§ REQ-160 § Containable operands](#containable-operands): conformance, not a name list) keeps that code and draws no `aql_contains_redundant_step`, the specific case winning over the general one.
- `aql_fanout_row_grain` owns the **junction** source and `aql_fanout_path_grain` the **path** source, two projected paths on one alias descending through different repeating scopes.
  The two are disjoint by construction: the first needs two projected aliases under one `AND` junction, the second two projected paths under one alias.
- `aql_path_repeating_unpredicated` (multiplicity, no OPT) and `aql_path_not_in_template` (Layer-3 membership, OPT-gated) report **different** defects and can both fire on one path; neither suppresses the other.
- `aql_deprecated_top` / `aql_top_with_limit` / `aql_top_with_fetch` (REQ-109) own the deprecated `TOP` clause, including its `ORDER BY` remedy, so `aql_paging_no_order_by` stays off a `TOP`-only query.
- `aql_select_star` (REQ-109) owns the `*` item, so `aql_select_no_alias` stays off it.

### Builder adapter

The write side **MUST** offer the same semantic judgement as the read side, opt-in: `(*aql.Builder).VerifyContainment` walks the builder's own FROM root and containment algebra (junctions and `NOT CONTAINS` included), with no emission and no re-parse, and reports the containment findings of § Checks (`aql_impossible_containment`, `aql_contains_not_containable`, `aql_archetype_class_mismatch`, `aql_unknown_rm_class`, `aql_containment_by_reference`).
It takes a nilable REQ-160 relation (§ Relation supply), and each code's severity and firing rule are the catalogue's; this section adds none.

- A finding **MUST** carry exactly a value-free `Code` and a value-bearing `Detail` (which may quote class names and HRIDs), mirroring the REQ-109 field classification, and no `Span`, `Path` or severity field: a builder tree has no source text to point into, and severity is looked up per code in the catalogue.
- Verification **MUST** stay opt-in: `Build()` never runs it, so a query carrying a Never pair still builds.
- **Read/write parity:** for a query expressible through the builder, the verification's code multiset **MUST** equal the containment-check subset of `lint.LintString` over the emitted text: one rule engine shared by both adapters, so they cannot drift.

### Path-shape acceptance

The corpus carries three firing witnesses: the unpredicated repeating-segment projection over an `OBSERVATION` alias, a `LIMIT 50 OFFSET 100` query with no `ORDER BY`, and a repeating unpredicated segment whose path appears only in `WHERE`, which an implementation narrowed to the projection fails.
The silence direction has a row per near-miss, so an over-firing linter cannot pass:

- a predicated repeating segment;
- `ORDER BY` beside a `LIMIT`;
- a nil `Options.Query`;
- a `TOP`-only row bound;
- an aliased projection item;
- a `*` item;
- two projected paths whose repeating segments are all in their common prefix;
- two projected paths on different aliases;
- an avoidable unreferenced intermediate;
- an unreferenced leaf;
- a `VERSIONED_OBJECT`-conforming operand, which carries `aql_versioned_object_unreferenced` and not `aql_contains_redundant_step`.

[PROBE-099](conformance.md#probe-099--aql-path-shape-lint-corpus) carries the corpus, with the walk's silent stops (§ The conservative segment walk) pinned by name.

### Out of scope

Engine-capability findings (valid AQL a particular engine does not implement) are a deferred per-engine-profile concern; terminology, function signatures, and `ORDER BY` type checking remain out of scope as in REQ-109, and so do OPT-aware path checking and node-id correctness, which are Layer 3's.

Widening `aql_contains_not_containable` to the FROM root is also out of scope: a root-position finding would need its own catalogue entry, severity and span rule rather than a re-pointed existing code ([`aql_contains_not_containable`](#aql_contains_not_containable)).

- **Resolving a BMM generic parameter to its RM constraint.** The RM constrains the generic the pinned tables carry as `T`, but the pin does not express the constraint and REQ-048 does not resolve it, so the walk stops there (§ The conservative segment walk).
  Widening the reach would be **cited data in the BMM band**, reviewed on a pin bump like REQ-160's overlay table, and is its own requirement, not a change to this check.
- **Rewriting a query.** No suggested alias, no suggested predicate, no auto-fix: these codes advise, they never author.
  `Detail` names the defect and its location, and no lint code edits or emits AQL.
- **Any promotion of the path-shape checks to Error severity**, and any option gating them (§ Always on, never gated).

- **Probes:** [PROBE-097](conformance.md#probe-097--aql-semantic-and-portability-lint-corpus) (the semantic and portability corpus, its read/write parity arm included); [PROBE-099](conformance.md#probe-099--aql-path-shape-lint-corpus) (the path-shape corpus)
