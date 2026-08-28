package aql

// projection.go — REQ-163 § The typed projection model. The write side of the
// SELECT clause:
//
//	selectClause : SELECT DISTINCT? top? selectExpr (SYM_COMMA selectExpr)*
//	selectExpr   : columnExpr (AS aliasName=IDENTIFIER)? | SYM_ASTERISK
//	columnExpr   : identifiedPath | primitive | aggregateFunctionCall | functionCall
//
// [parse.SelectClause] is the REFERENCE SHAPE and this file mirrors it rather
// than growing a second projection vocabulary: a clause-level DISTINCT flag and
// star form, an ordered item list, and per item an operand plus an optional AS
// alias, over a sealed operand vocabulary of a path, a function/aggregate call,
// a literal, and a star.
//
// ONE type for two productions. `columnExpr` reaches both
// `aggregateFunctionCall` and `functionCall`, and the read side models the pair
// with the single [parse.FunctionCall]; [funcColumn] does the same, so a shape
// rule written once binds both carriers.
//
// Until this file `aql.Col` spliced its argument into the projection list
// verbatim and unchecked, which left SELECT the one clause REQ-119's write-path
// hardening did not reach. [Col] KEEPS that leniency — see its own doc and
// projection_verify.go for the two opposite-facing rules that settle it — and
// the constructors here are the recommended route beside it.

import (
	"fmt"
	"slices"
	"strings"
)

// SelectField is one entry in the SELECT projection list — the write-side
// mirror of [parse.SelectItem]: one operand plus an optional `AS` alias.
//
// Construct with [Col] (a verbatim path or alias, the legacy route), [ColAs],
// [Star], [Count], [CountDistinct], [CountStar], [Fn] or [Lit]; add an alias to
// any of them with [SelectField.As]. The zero value carries no operand and is
// refused by [Builder.Build].
//
// # Comparability
//
// A SelectField is NOT safe to compare with `==` and MUST NOT be used as a map
// key. A field built by [Fn] carries its argument list as a slice, so `==` on
// one panics with "comparing uncomparable type" — the same change [Containment]
// took in v0.18.0, and invisible to the compiler for the same reason: the slice
// is behind an interface field. Compare the built query strings instead.
type SelectField struct {
	// expr is the projected operand. Nil in the zero value, which Build
	// refuses rather than emitting an empty projection slot.
	expr selectOperand
	// alias is the AS alias, and aliasSet records that the CALLER asked for
	// one. The two are distinct because an alias the builder never recorded is
	// not compared against the emitted text: a legacy [Col] carrying its own
	// `AS` is tolerated (REQ-163 § `Col` stays lenient, rule 1) while a typed
	// item's alias is part of the structure Build verifies.
	alias    string
	aliasSet bool
}

// selectOperand is the SEALED SELECT-operand vocabulary — the write-side twin
// of [parse.SelectExpr], with one shape per `columnExpr` alternative plus the
// star.
//
// Sealed by the INTERFACE's own visibility, which is the stronger of the two
// sealing mechanisms and the one that applies here: selectOperand is unexported,
// so no declaration outside this package can name it as an embedded field or as
// a method's result, and the constructors in this file are therefore the only
// values that exist. [VersionPredicate] is the sibling carrier and seals
// differently — it is EXPORTED, so its closure rests on its methods being
// unexported instead. Either way the vocabulary closes with the grammar rather
// than with policy.
type selectOperand interface {
	// selectToken renders the operand's canonical text — WITHOUT the alias,
	// which the item renderer appends — or reports why it cannot be rendered.
	selectToken() (string, error)
	// kind names the shape, so a positional rule can be written without a type
	// switch over the vocabulary (REQ-119 § The emission closure property).
	kind() operandKind
	// literalValue is the projected literal's value, and nil for every other
	// shape.
	literalValue() Value
	// aggregateCall names the call and reports it AGGREGATE-shaped, and is
	// ("", false) for every other shape.
	//
	// It and literalValue are the fields a POSITIONAL rule needs to read, and
	// they are methods on the vocabulary rather than type assertions at the
	// rule for the reason kind is: an assertion binds the rule to one carrier,
	// and every shape here declares its methods on a value receiver, so a
	// pointer twin satisfies the interface beside the value (REQ-119 § The
	// emission closure property).
	aggregateCall() (string, bool)
}

// operandKind names a [selectOperand] shape for the positional rules that must
// distinguish them.
type operandKind int

const (
	// operandRaw is the legacy [Col] carrier, whose text is spliced verbatim.
	operandRaw operandKind = iota
	// operandPath is a typed projected path.
	operandPath
	// operandStar is the `*` item.
	operandStar
	// operandLiteral is a projected primitive.
	operandLiteral
	// operandFunc is a projected function or aggregate call.
	operandFunc
)

// rawColumn is the legacy [Col] carrier: text spliced into the projection list
// as written. Its leniency is deliberate (REQ-163 § `Col` stays lenient); what
// bounds it is the after-emission verification in projection_verify.go, which
// refuses text that changes the projection's recorded STRUCTURE.
type rawColumn struct{ text string }

func (r rawColumn) selectToken() (string, error) {
	if r.text == "" {
		return "", fmt.Errorf("%w: empty SELECT field", ErrInvalidQuery)
	}
	return r.text, nil
}

func (rawColumn) kind() operandKind { return operandRaw }

func (rawColumn) literalValue() Value { return nil }

func (rawColumn) aggregateCall() (string, bool) { return "", false }

// pathColumn is a typed projected `identifiedPath`. The text stays VERBATIM by
// contract (REQ-055 rule 3) — it is an openEHR identifier, never caller data —
// so the check here is the emptiness one every other path position has.
type pathColumn struct{ path string }

func (p pathColumn) selectToken() (string, error) {
	if p.path == "" {
		return "", fmt.Errorf("%w: SELECT path with empty path text", ErrInvalidQuery)
	}
	return p.path, nil
}

func (pathColumn) kind() operandKind { return operandPath }

func (pathColumn) literalValue() Value { return nil }

func (pathColumn) aggregateCall() (string, bool) { return "", false }

// starColumn is the `*` projection item — `selectExpr`'s own SYM_ASTERISK
// alternative (SDK-AQL-002), not a `columnExpr`. A SOLE unaliased one emits as
// the bare `SELECT *`, which is how it re-parses.
type starColumn struct{}

func (starColumn) selectToken() (string, error) { return "*", nil }

func (starColumn) kind() operandKind { return operandStar }

func (starColumn) literalValue() Value { return nil }

func (starColumn) aggregateCall() (string, bool) { return "", false }

// literalColumn is a projected primitive — `columnExpr`'s `primitive`
// alternative — carrying the shared [Value] vocabulary so a projected literal
// and a compared literal render through ONE renderer (REQ-119 § Canonical
// value spellings).
type literalColumn struct{ v Value }

func (l literalColumn) selectToken() (string, error) {
	// A CALL is not a primitive, and [Fn] is the route that carries one. The
	// shape is read through [DerefValue] rather than a bare type switch, so a
	// `*FuncCall` pointer twin is caught with the value shape (REQ-119
	// § The emission closure property).
	//
	// Refusing rather than rerouting is the package's own idiom (see
	// [versionRoutingRefusal]): [FormatValue] would render the call happily,
	// but a literal never reaches [SelectField.renderArgument], so a comma
	// inside an argument would come back as one more argument than the builder
	// recorded — the arity substitution the argument scan exists to close, one
	// route below it.
	if inner, ok := DerefValue(l.v); ok {
		if _, isCall := inner.(FuncCall); isCall {
			return "", fmt.Errorf("%w: a projected literal carrying a function call; `columnExpr`'s "+
				"`primitive` alternative is a primitive and the call has its own alternative — build it "+
				"with aql.Fn, which holds the name and every argument to the call's own rules",
				ErrInvalidQuery)
		}
	}
	// [FormatValue] cannot refuse — it has no error to return — so the
	// value-position guards must be applied HERE or the closure holds for
	// WHERE and not for SELECT. Exactly what parse's own SELECT literal arm
	// does (REQ-119).
	if err := ValidateValue(l.v); err != nil {
		return "", fmt.Errorf("SELECT literal: %w", err)
	}
	return FormatValue(l.v), nil
}

func (literalColumn) kind() operandKind { return operandLiteral }

func (l literalColumn) literalValue() Value { return l.v }

func (literalColumn) aggregateCall() (string, bool) { return "", false }

// funcColumn is a projected function or aggregate call — ONE type for
// `aggregateFunctionCall` and `functionCall` both, mirroring
// [parse.FunctionCall], so a shape rule written once binds both carriers.
type funcColumn struct {
	name     string
	args     []SelectField
	distinct bool
	star     bool
}

func (funcColumn) kind() operandKind { return operandFunc }

func (funcColumn) literalValue() Value { return nil }

// aggregateCall names the call in its canonical spelling and reports whether it
// is AGGREGATE-shaped — the property `terminal` has no alternative for.
//
// Three tests, because the shape shows itself three ways and each is the
// package's OWN classification rather than a second one: the STAR and the
// DISTINCT flags belong to `aggregateFunctionCall` alone ([funcColumn.validateShape]
// states that rule for the top-level position, where `functionCall` reaches
// neither), and the NAME is read through [IsAggregateFunc], the same ASCII-folded
// set the projected-name check admits and every value position refuses.
func (f funcColumn) aggregateCall() (string, bool) {
	name := asciiUpper(strings.TrimSpace(f.name))
	return name, f.star || f.distinct || IsAggregateFunc(name)
}

// selectToken renders `NAME(args…)` in the canonical spelling: an ASCII
// upper-cased name, `DISTINCT ` before the arguments when the aggregate
// carried it, `*` with no padding for `COUNT(*)`, and one space after each
// argument comma (REQ-055 rule 5).
//
// The name is upper-cased because IDENTITY is the bar (REQ-163 § Read-side
// mirror duty): the extractor upper-cases every projected name while the
// emitter renders the name as carried, so a lower-case builder spelling comes
// back upper-cased and the round trip is not identity.
func (f funcColumn) selectToken() (string, error) {
	// asciiUpper, never strings.ToUpper: the Unicode mapping folds some
	// non-ASCII letters INTO the ASCII alphabet (ı → I), turning a spelling
	// the lexer cannot tokenise into a legal-looking one.
	name := asciiUpper(strings.TrimSpace(f.name))
	// SELECT reaches `aggregateFunctionCall`, so the aggregates are admissible
	// here — which is why this is [ValidateSelectFuncName] and not the
	// value-position check.
	if err := ValidateSelectFuncName(name); err != nil {
		return "", fmt.Errorf("SELECT function call: %w", err)
	}
	if err := f.validateShape(name); err != nil {
		return "", err
	}
	if f.star {
		return name + "(*)", nil
	}
	parts := make([]string, 0, len(f.args))
	for i, a := range f.args {
		s, err := a.renderArgument(name, i)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	body := strings.Join(parts, ", ")
	if f.distinct {
		body = "DISTINCT " + body
	}
	return name + "(" + body + ")", nil
}

// validateShape holds the call's Star / Distinct flags and its argument list to
// the grammar rule that admits the NAME:
//
//	aggregateFunctionCall : COUNT '(' (DISTINCT? identifiedPath | '*') ')'
//	                      | (MIN|MAX|SUM|AVG) '(' identifiedPath ')'
//	functionCall          : terminologyFunction
//	                      | name '(' (terminal (',' terminal)*)? ')'
//
// while the general `functionCall` admits neither DISTINCT nor `*` at all. It
// is the builder-side statement of the rule [parse.Query.Emit] applies to a
// projected [parse.FunctionCall]; openehr/aql cannot call that one (the import
// runs the other way), so the two are held in agreement by test rather than by
// sharing code — see TestProjectionShapeRefusalsAgreeWithEmit in
// projection_test.go, which walks the shapes both sides can reach.
//
// The constructors make most of this unreachable by construction — [Count],
// [CountDistinct] and [CountStar] each build one admitted shape — so what it
// actually guards is [Fn], which takes a caller-supplied name.
func (f funcColumn) validateShape(name string) error {
	if !IsAggregateFunc(name) {
		if isTerminologyName(name) {
			return f.validateTerminology()
		}
		if f.star || f.distinct {
			return fmt.Errorf("%w: %s() admits neither DISTINCT nor a star argument", ErrInvalidQuery, name)
		}
		return nil
	}
	if f.star {
		// `COUNT(*)` is the only star form and it is the WHOLE argument list:
		// a star beside arguments or DISTINCT must be refused, never resolved
		// by dropping the loser.
		if name != "COUNT" {
			return fmt.Errorf("%w: %s(*) is not in the grammar; only COUNT takes a star", ErrInvalidQuery, name)
		}
		if f.distinct || len(f.args) > 0 {
			return fmt.Errorf("%w: COUNT(*) admits neither DISTINCT nor arguments beside the star; "+
				"emitting the star alone would silently drop them", ErrInvalidQuery)
		}
		return nil
	}
	if f.distinct && name != "COUNT" {
		return fmt.Errorf("%w: DISTINCT inside %s() is not in the grammar; only COUNT(DISTINCT path) is",
			ErrInvalidQuery, name)
	}
	if len(f.args) != 1 {
		return fmt.Errorf("%w: %s() takes exactly one path argument, got %d", ErrInvalidQuery, name, len(f.args))
	}
	if k := f.args[0].operandKind(); k != operandPath && k != operandRaw {
		return fmt.Errorf("%w: %s() takes an identified path, not a projected %s",
			ErrInvalidQuery, name, k)
	}
	return nil
}

// validateTerminology holds a projected call named TERMINOLOGY to its OWN
// grammar rule, `terminologyFunction : TERMINOLOGY '(' STRING ',' STRING ','
// STRING ')'` — not the general `functionCall`, so the arity and the argument
// type are both fixed and any other shape emits text the parser rejects.
//
// [Terminology] builds the value-position sibling correctly; this catches a
// hand-written [Fn] that borrowed the name.
func (f funcColumn) validateTerminology() error {
	if f.star || f.distinct {
		return fmt.Errorf("%w: %s() admits neither DISTINCT nor a star argument",
			ErrInvalidQuery, TerminologyFunc)
	}
	if len(f.args) != 3 {
		return fmt.Errorf("%w: %s() takes exactly 3 string arguments, got %d",
			ErrInvalidQuery, TerminologyFunc, len(f.args))
	}
	for i, a := range f.args {
		// Both carriers normalise before the shape check: the operand (is it a
		// literal at all?) and the [Value] inside it, since `*StringValue` is a
		// string literal too (REQ-119 § The emission closure property).
		lit, ok := DerefValue(a.literalValue())
		if !ok {
			return fmt.Errorf("%w: %s() argument %d is not a string literal; the grammar admits no other",
				ErrInvalidQuery, TerminologyFunc, i)
		}
		if _, isString := lit.(StringValue); !isString {
			return fmt.Errorf("%w: %s() argument %d is %T; the grammar admits only string literals",
				ErrInvalidQuery, TerminologyFunc, i, lit)
		}
	}
	return nil
}

// isTerminologyName reports the reserved TERMINOLOGY spelling. The fold is
// ASCII, held by length, for the reason [asciiKeyword] states.
func isTerminologyName(name string) bool { return asciiKeyword(name, TerminologyFunc) }

// String names an [operandKind] in a diagnostic. The vocabulary is the SDK's
// own, so naming it reproduces no caller text (REQ-119 § Emission verified
// after emission).
func (k operandKind) String() string {
	switch k {
	case operandRaw:
		return "column"
	case operandPath:
		return "path"
	case operandStar:
		return "star"
	case operandLiteral:
		return "literal"
	case operandFunc:
		return "function call"
	}
	return "unknown operand"
}

// Col is a projected path or alias, e.g. Col("o") or Col("o/data[at0001]").
//
// Col renders its argument into the projection list VERBATIM and is checked
// for emptiness alone, so `Col("COUNT(x) AS n")` builds. That leniency is
// deliberate and stays (REQ-163 § `Col` stays lenient): text that re-parses as
// a single projected item and introduces no clause-level flag the builder did
// not record is ordinary AQL saying what the caller wrote.
//
// What it does NOT survive is a change to the projection's STRUCTURE.
// `Col("a, b")` splits the list in two, and `Col("DISTINCT c/uid/value")`
// introduces a clause-level DISTINCT the builder never set — the keyword is
// consumed into the CLAUSE, not into the item — so both emit valid AQL asking
// a different question, invisible to every round-trip and golden check
// downstream. [Builder.Build] refuses them (REQ-163 § Build() verifies what it
// emitted).
//
// PREFER the typed constructors — [ColAs], [Count], [CountDistinct],
// [CountStar], [Fn], [Lit], [Star] and [SelectField.As] — which say the same
// things structurally and are checked at their identifier positions. Col is
// not deprecated.
func Col(path string) SelectField {
	return SelectField{expr: rawColumn{text: strings.TrimSpace(path)}}
}

// ColAs is a projected path carrying an `AS` alias —
// `ColAs("o/data[at0001]/value/magnitude", "temp")` emits
// `o/data[at0001]/value/magnitude AS temp`.
//
// The alias is a single-token identifier position, held to [ValidateIdentifier]
// at [Builder.Build] time: `selectExpr : columnExpr (AS aliasName=IDENTIFIER)?`
// is ONE token, so a spliced alias would otherwise re-parse as a second
// projection (REQ-119).
func ColAs(path, alias string) SelectField {
	return SelectField{
		expr:     pathColumn{path: strings.TrimSpace(path)},
		alias:    strings.TrimSpace(alias),
		aliasSet: true,
	}
}

// Star is the `*` projection item. As the SOLE unaliased item it emits the
// bare `SELECT *` form — which is how the parser reads it back — and beside
// other items it emits in place: `SELECT *, c/uid/value`.
//
// `selectExpr`'s star alternative carries no alias slot, so [SelectField.As] on
// a star item is refused at [Builder.Build] rather than emitted as text the
// parser rejects.
func Star() SelectField { return SelectField{expr: starColumn{}} }

// Count is the `COUNT(<path>)` aggregate.
func Count(path string) SelectField {
	return SelectField{expr: funcColumn{name: "COUNT", args: []SelectField{colPath(path)}}}
}

// CountDistinct is the `COUNT(DISTINCT <path>)` aggregate — the one projected
// call the grammar admits a DISTINCT inside (`aggregateFunctionCall`).
func CountDistinct(path string) SelectField {
	return SelectField{expr: funcColumn{name: "COUNT", args: []SelectField{colPath(path)}, distinct: true}}
}

// CountStar is the `COUNT(*)` aggregate — a row count, the only star form the
// grammar admits inside a call.
func CountStar() SelectField {
	return SelectField{expr: funcColumn{name: "COUNT", star: true}}
}

// Fn is a projected function or aggregate call — `Fn("MAX", Col("o/value"))`
// emits `MAX(o/value)`, `Fn("concat", Col("p/given"), Lit(String(" ")))` emits
// `CONCAT(p/given, ' ')`.
//
// The name is canonicalised to UPPER CASE over ASCII letters (never
// [strings.ToUpper], whose Unicode mapping would launder a name the lexer
// cannot tokenise) and held to [ValidateSelectFuncName] at [Builder.Build]
// time: SELECT reaches `aggregateFunctionCall`, so COUNT / MIN / MAX / SUM /
// AVG are admissible here and refused in every value position.
//
// The call's SHAPE is held to the rule that admits the name: COUNT takes
// `DISTINCT? identifiedPath` or a bare `*`, MIN / MAX / SUM / AVG take exactly
// one identified path, no other projected call carries DISTINCT or a star, and
// TERMINOLOGY keeps its fixed arity and argument type. Prefer [Count],
// [CountDistinct] and [CountStar] for the aggregates — each builds one admitted
// shape by construction.
//
// An argument carrying an `AS` alias is refused: `selectExpr` aliases the whole
// projected item, and `functionCall`'s arguments are `terminal`s with no alias
// slot at all. So is an AGGREGATE-shaped argument: `COUNT` / `MIN` / `MAX` /
// `SUM` / `AVG` reach `columnExpr` and no `terminal`, so `CONCAT(COUNT(*))` is
// text the parser rejects. A nested ordinary call — `Fn("CONCAT",
// Fn("LENGTH", Col("c/x")))` — is a `terminal` and stays admitted.
//
// The variadic slice is COPIED at intake, as [Builder.Select] copies its own:
// a `...SelectField` call site hands the callee the caller's own backing array
// when the arguments were spread from a slice, so retaining it would let a
// later write to that slice change what an already-recorded projection emits.
func Fn(name string, args ...SelectField) SelectField {
	return SelectField{expr: funcColumn{name: name, args: slices.Clone(args)}}
}

// Lit is a projected literal — `Lit(Int(1))` emits `1`, `Lit(String("x"))`
// emits `'x'` — rendered through the canonical value spellings a WHERE
// comparison uses, so the two positions cannot drift (REQ-119).
//
// A BARE parameter is not a projection: `columnExpr` has no PARAMETER
// alternative, so `Lit(Param("p"))` is refused at [Builder.Build] while
// `Fn("CONCAT", Lit(String("a")), Lit(Param("p")))` is admitted — a function
// argument is a `terminal`, which does carry one. The check is positional,
// which is why it is not on [Lit] itself.
//
// A FUNCTION CALL is not a literal in any position: `Lit(Func("CONCAT", …))`
// is refused at [Builder.Build] wherever it sits, because `primitive` and the
// call alternatives are different branches of `columnExpr` and only [Fn] holds
// a call to the rules of its own branch — the name, the argument shapes, and
// the per-argument escape scan that keeps a spliced comma from adding an
// argument the builder never recorded. Build the call with [Fn], nesting it
// where a nested call is what you mean.
func Lit(v Value) SelectField { return SelectField{expr: literalColumn{v: v}} }

// As returns the field carrying an `AS` alias — `Count("c/uid/value").As("n")`
// emits `COUNT(c/uid/value) AS n`. As everywhere in the builder the receiver is
// not modified; the result is a new value, and a later call replaces an earlier
// alias.
//
// The alias is held to [ValidateIdentifier] at [Builder.Build] time
// (`aliasName=IDENTIFIER` is one token). An alias on a star item is refused:
// `selectExpr`'s star alternative has no alias slot.
func (f SelectField) As(alias string) SelectField {
	f.alias = strings.TrimSpace(alias)
	f.aliasSet = true
	return f
}

// colPath is the typed path operand the aggregate constructors wrap their
// argument in, so `Count("c/uid/value")` records a PATH rather than the
// verbatim carrier — the aggregate rule is "exactly one identified path", and
// recording the shape is what lets Build say so.
func colPath(path string) SelectField {
	return SelectField{expr: pathColumn{path: strings.TrimSpace(path)}}
}

// operandKind reports the field's operand shape, and operandRaw for the zero
// value — which [SelectField.render] refuses before any rule reads this.
func (f SelectField) operandKind() operandKind {
	if f.expr == nil {
		return operandRaw
	}
	return f.expr.kind()
}

// literalValue reports the field's projected literal value, nil for every other
// shape and for the zero value.
func (f SelectField) literalValue() Value {
	if f.expr == nil {
		return nil
	}
	return f.expr.literalValue()
}

// renderedItem is one emitted SELECT item and the structure the builder
// RECORDED for it — the two halves the after-emission verification compares
// (projection_verify.go).
type renderedItem struct {
	// expr is the operand text, alias is the AS alias, and text is the whole
	// item as emitted (`expr` or `expr AS alias`).
	expr  string
	alias string
	text  string
	// aliasRecorded reports that the BUILDER fixed the alias, which is what
	// makes it part of the compared structure. It is false for a legacy [Col]
	// with no [SelectField.As] call: where the `AS` boundary falls inside
	// verbatim text is not something the builder recorded, so there is nothing
	// for the emitted text to disagree with (REQ-163 § `Col` stays lenient).
	aliasRecorded bool
	// start and end bound the item in the emitted clause text, so a refusal
	// can name the item a defect falls in without quoting any of it.
	start, end int
}

// render renders one projection item: the operand text, plus the canonical
// ` AS <alias>` when an alias was recorded.
//
// Every refusal it returns carries the ITEM INDEX, including the ones the
// operand raises for itself — a shape error, a name the grammar cannot lex, an
// argument that escapes its slot. The coordinate is the whole diagnostic here
// (REQ-119 § Emission verified after emission): the text may not be quoted, so
// a caller with a ten-item projection has nothing else to locate the defect by.
func (f SelectField) render(idx int) (renderedItem, error) {
	if f.expr == nil {
		return renderedItem{}, fmt.Errorf("%w: SELECT item %d carries no operand", ErrInvalidQuery, idx)
	}
	// A bare parameter is refused at the TOP of a projection and nowhere
	// deeper: `columnExpr` has no PARAMETER alternative while a function
	// ARGUMENT is a `terminal`, which has. The check is positional, so it lives
	// here rather than on the literal shape both positions share.
	if inner, ok := DerefValue(f.literalValue()); ok {
		if _, isParam := inner.(ParamValue); isParam {
			return renderedItem{}, fmt.Errorf("%w: SELECT item %d is a bare parameter, which is not a "+
				"projection; the grammar admits one only inside a function call", ErrInvalidQuery, idx)
		}
	}
	tok, err := f.expr.selectToken()
	if err != nil {
		// The operand's own diagnostics name the rule they applied — the shape,
		// the name, the argument slot — and none of them knows WHERE in the
		// projection it sat. The index is added once, here, so `%w` keeps
		// [ErrInvalidQuery] reachable as a sentinel and the message stays
		// value-free.
		return renderedItem{}, fmt.Errorf("SELECT item %d: %w", idx, err)
	}
	item := renderedItem{expr: tok, text: tok}
	if f.aliasSet {
		// `selectExpr : columnExpr (AS aliasName=IDENTIFIER)?` — one token, so
		// the exact guard is writable and the splice it otherwise admits is
		// invisible: an alias of `x, c/y` emits two projections.
		if err := ValidateIdentifier(f.alias); err != nil {
			return renderedItem{}, fmt.Errorf("SELECT item %d alias: %w", idx, err)
		}
		// `selectExpr`'s star alternative is SYM_ASTERISK alone — it is not a
		// `columnExpr`, so it reaches no `AS` at all.
		if tok == "*" {
			return renderedItem{}, fmt.Errorf("%w: SELECT item %d is a star carrying an AS alias; "+
				"`selectExpr`'s star alternative has no alias slot", ErrInvalidQuery, idx)
		}
		item.alias = f.alias
		item.text = tok + " AS " + f.alias
	}
	// A typed operand fixes its own alias — empty when the caller asked for
	// none — because its text cannot carry one: only the verbatim [Col] can.
	item.aliasRecorded = f.aliasSet || f.operandKind() != operandRaw
	return item, nil
}

// renderArgument renders one argument of a projected call. `functionCall`'s
// arguments are `terminal`s: no alias slot, no star, and no AGGREGATE.
//
//	functionCall : terminologyFunction | name '(' (terminal (',' terminal)*)? ')'
//	terminal     : primitive | PARAMETER | identifiedPath | functionCall
//
// `terminal` reaches an ordinary `functionCall` and NOT
// `aggregateFunctionCall`, which only `columnExpr` does — so a nested plain
// call is legal (`CONCAT(LENGTH(c/x))`) while an aggregate in the same slot is
// text the parser rejects (`CONCAT(COUNT(*))`, `CONCAT(MIN(c/x))`).
//
// The argument text is then held to the SAME escape scan the projection items
// are (projection_verify.go): a comma inside an argument re-parses as one more
// argument, changing the call's ARITY with nothing downstream able to see it —
// the silent-substitution mode one level below the item list.
func (f SelectField) renderArgument(fn string, idx int) (string, error) {
	if f.expr == nil {
		return "", fmt.Errorf("%w: %s() argument %d is empty", ErrInvalidQuery, fn, idx)
	}
	if f.aliasSet {
		return "", fmt.Errorf("%w: %s() argument %d carries an AS alias; `selectExpr` aliases the whole "+
			"projected item and a function argument is a `terminal`, which has no alias slot",
			ErrInvalidQuery, fn, idx)
	}
	if f.operandKind() == operandStar {
		return "", fmt.Errorf("%w: %s() argument %d is a star; the only star inside a call is COUNT(*), "+
			"which is the whole argument list", ErrInvalidQuery, fn, idx)
	}
	// `terminal` has no `aggregateFunctionCall` alternative, so an aggregate in
	// an argument slot emits text the parser rejects outright. Refused here
	// rather than left to the server, whose syntax message would point into
	// bytes the SDK wrote for the caller.
	if name, isAggregate := f.expr.aggregateCall(); isAggregate {
		return "", fmt.Errorf("%w: %s() argument %d is the aggregate-shaped call %s() — `terminal` reaches "+
			"an ordinary `functionCall` and not `aggregateFunctionCall`, which only `columnExpr` does, so "+
			"the emitted text has no reading; project the aggregate as its own SELECT item",
			ErrInvalidQuery, fn, idx, name)
	}
	tok, err := f.expr.selectToken()
	if err != nil {
		return "", err
	}
	if err := checkArgumentEscape(fn, idx, tok); err != nil {
		return "", err
	}
	return tok, nil
}
