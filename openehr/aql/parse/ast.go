package parse

import (
	"reflect"
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse/gen"
)

// Clause identifies which top-level clause an [IdentifiedPath] appears in. It
// localises a path for diagnostics and lets the lint layer scope checks (e.g.
// SELECT vs WHERE).
type Clause int

const (
	// ClauseUnknown is the zero value: a path whose enclosing clause could
	// not be determined (should not occur for a well-formed query).
	ClauseUnknown Clause = iota
	// ClauseSelect is the SELECT projection list.
	ClauseSelect
	// ClauseWhere is the WHERE predicate.
	ClauseWhere
	// ClauseOrderBy is the ORDER BY list.
	ClauseOrderBy
	// ClauseFrom is the FROM / CONTAINS tree. It is appended after ClauseOrderBy
	// instead of inserted in clause order, so the numeric values of the members
	// above stay stable.
	ClauseFrom
	// ClauseLimit is the LIMIT value.
	ClauseLimit
	// ClauseOffset is the OFFSET value.
	ClauseOffset
	// ClauseTop is the deprecated SELECT TOP clause, distinct from ClauseSelect
	// so a diagnostic can name the row-count position instead of the whole
	// projection.
	ClauseTop
)

// String renders the clause name for diagnostics.
func (c Clause) String() string {
	switch c {
	case ClauseSelect:
		return "select"
	case ClauseWhere:
		return "where"
	case ClauseOrderBy:
		return "order by"
	case ClauseFrom:
		return "from"
	case ClauseLimit:
		return "limit"
	case ClauseOffset:
		return "offset"
	case ClauseTop:
		return "top"
	case ClauseUnknown:
		return "unknown"
	}
	return "unknown"
}

// endOf is the position just past tok's last character. A token whose text
// spans lines (a string literal carrying a newline) advances the line and
// restarts the column, so the span stays meaningful instead of reporting a
// column past the end of the first line.
func endOf(tok antlr.Token) Position {
	if tok == nil {
		return Position{}
	}
	text := tok.GetText()
	if before, last, found := strings.CutLast(text, "\n"); found {
		return Position{Line: tok.GetLine() + 1 + strings.Count(before, "\n"), Col: 1 + len([]rune(last))}
	}
	// ANTLR columns are 0-based; posOf exposes 1-based, and so does this.
	return Position{Line: tok.GetLine(), Col: tok.GetColumn() + 1 + len([]rune(text))}
}

// spanAt is the [Span] of whichever ANTLR shape the extractor holds when it
// records a drop — a rule context, a terminal node, or a bare token. The
// three share no interface, and a drop site should not have to pick a helper
// per shape.
func spanAt(node any) Span {
	// An absent node yields a zero span whether the nil arrives untyped or
	// typed — a typed nil would satisfy the interface cases below and panic
	// inside the accessor, and a diagnostics helper must not be able to
	// panic. The reflect call is a typed-nil check, one of the classes of
	// reflection the idiom rules (idiom.md) allow in library code.
	if node == nil {
		return Span{}
	}
	if rv := reflect.ValueOf(node); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return Span{}
	}
	switch n := node.(type) {
	case antlr.ParserRuleContext:
		return boundedSpan(posOf(n.GetStart()), endOf(n.GetStop()))
	case antlr.TerminalNode:
		return boundedSpan(posOf(n.GetSymbol()), endOf(n.GetSymbol()))
	case antlr.Token:
		return boundedSpan(posOf(n), endOf(n))
	}
	return Span{}
}

// boundedSpan keeps the documented [Start, End) contract when the two
// endpoints were computed independently: a missing endpoint (an epsilon rule
// match leaving GetStop nil) collapses the span to zero width at the known
// one rather than producing an inverted range.
func boundedSpan(start, end Position) Span {
	switch {
	case start == (Position{}) && end == (Position{}):
		return Span{}
	case start == (Position{}):
		return Span{Start: end, End: end}
	case end == (Position{}):
		return Span{Start: start, End: start}
	}
	return Span{Start: start, End: end}
}

// ClassExpr is one class expression bound in the FROM / CONTAINS tree
// (e.g. `OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]`). The
// containment tree is flattened to document order; nesting is not kept
// because the linter reasons over the set of bound classes, not their
// containment shape.
//
// ClassExpr is not comparable with `==` and cannot be a map key: it carries
// a slice ([ClassExpr.Archetypes]), and [ClassExpr.ParsedPredicate] can hold
// one.
type ClassExpr struct {
	// RMType is the reference-model class name (e.g. "OBSERVATION",
	// "COMPOSITION", "EHR"), or "VERSION" for a VERSION class expression.
	RMType string
	// Alias is the binding variable (e.g. "o"), or "" when anonymous.
	Alias string
	// Archetype is the literal archetype HRID the class bracket names (e.g.
	// "openEHR-EHR-OBSERVATION.blood_pressure.v1"), the `$param` placeholder
	// text when ParamArchetype is true (e.g. "$arch"), or "" when the bracket
	// names no archetype. The HRID is set for the bare form `[hrid]` and for
	// the named form `[hrid, 'name']` alike; for the named form
	// [ClassExpr.Predicate] also carries the whole bracket text, which is what
	// the emitter renders. Callers must consult ParamArchetype before treating
	// Archetype as a literal HRID.
	Archetype string
	// Archetypes lists each distinct literal archetype HRID the class bracket
	// names as an operand, in source order: the one HRID of the bare form
	// `[hrid]` and of the named form `[hrid, 'name']`, and each HRID operand
	// of an AND / OR junction, such as `[hrid1 or hrid2]` or
	// `[hrid and name/value='x']`. It is filled whether or not
	// [ClassExpr.ParsedPredicate] structures the bracket, so a junction left
	// unstructured, such as `[hrid and archetype_node_id=at0001]`, still lists
	// its HRID. A `$param` archetype is not listed (see
	// [ClassExpr.ParamArchetype]), and neither is an HRID in a path operand's
	// own predicate. It is nil when the bracket names no literal HRID, when
	// the class has no bracket, and for a VERSION class. Both views fill it.
	// Like ParsedPredicate it is a read-side derivation: emission renders
	// Archetype and Predicate and never reads it.
	Archetypes []string
	// ParamArchetype is true when the archetype predicate is a $param
	// placeholder (`[$arch]`) rather than a literal HRID; the
	// identifiable scope is deferred to bind time. When true, Archetype still carries the
	// placeholder text (not "").
	ParamArchetype bool
	// Version is true for a VERSION class expression (version machinery,
	// distinct from a clinical RM class).
	Version bool
	// HasPredicate is true when the class carries any path predicate
	// (`[...]`): an archetype, a standing predicate like `[ehr_id/value=$x]`,
	// or a version predicate. Distinguishes an identifiable EHR/VERSION root
	// from a bare one.
	HasPredicate bool
	// Predicate is the raw text inside the class predicate brackets when
	// HasPredicate is true and the predicate is not a bare literal archetype
	// HRID (which lives on [ClassExpr.Archetype] alone) or a `$param`
	// archetype (signalled by [ClassExpr.ParamArchetype]). Carries standing
	// predicates such as `ehr_id/value=$x`, and the named archetype form
	// `hrid, 'name'` beside its HRID on Archetype, so the emitter can
	// round-trip them, with brackets stripped and content verbatim from the
	// source. When Predicate is set the emitter renders it; otherwise it
	// renders Archetype.
	Predicate string
	// PredicateComparison is the standing class predicate parsed as a
	// `{path, operator, value}` comparison (e.g. `ehr_id/value = $x`),
	// reusing the shared [aql.Comparison] / [aql.Value] vocabulary. Non-nil
	// only when the predicate is a simple comparison; nil for an archetype
	// HRID (see [ClassExpr.Archetype]), a version predicate, a non-scalar /
	// complex standing predicate, or a comparison whose literal the value
	// vocabulary cannot represent (an out-of-range numeric). The verbatim
	// [ClassExpr.Predicate] text stays authoritative in every nil case, so
	// emission is lossless regardless.
	//
	// A fifth nil case depends on which view the [ClassExpr] was read from,
	// not on the predicate's shape: this field is always nil in the flat lint
	// view ([Parse] → [Document.Classes]), and is populated only by the
	// structured extraction ([ParseQuery] / [Document.Query]). The flat view
	// omits it on purpose, so linting does not pay for the structured parse;
	// the verbatim [ClassExpr.Predicate] text is authoritative there too, and
	// is populated identically in both views.
	//
	// The comparison's Path is the relative object path as written, and its
	// ParsedPath carries the same path's structured Segments with an empty
	// Alias (a relative predicate path binds no FROM alias), matching the
	// WHERE side for the class-predicate left-hand side.
	PredicateComparison *aql.Comparison
	// ParsedPredicate is the typed form of the class bracket, whichever field
	// carries its text (Archetype for a bare HRID or `$param`, Predicate
	// otherwise). It is derived as [PathSegment.Parsed] is, since the class
	// bracket is the same grammar production as a path step's, and both views
	// populate it. It is a read-side derivation: no write path reads it, so
	// mutating it cannot change what the emitter produces, and the text fields
	// stay authoritative for emission.
	//
	// nil means the SDK does not structure this form, and a reader may rely on
	// that and fail closed on it. There is no partial structure: a non-nil
	// ParsedPredicate is complete for its kind. It is nil when the class
	// carries no bracket, and for a VERSION class, whose bracket is a
	// different production. When a class bracket is present and
	// ParsedPredicate is nil, the predicate is one of the enumerated
	// unstructured forms listed on [PathSegment.Parsed].
	//
	// Not `==`-comparable and not usable as a map key; see the Comparability
	// section of [aql.SegmentPredicate]. Compare with [aql.EqualPredicates].
	ParsedPredicate aql.SegmentPredicate
	// Pos is the source position of the class expression.
	Pos Position
}

// PathSegment is one step of an identified path, re-exported from
// [aql.PathSegment], the shared path vocabulary.
type PathSegment = aql.PathSegment

// IdentifiedPath is an alias-qualified path referenced in SELECT, WHERE, or
// ORDER BY (e.g. `o/data[at0001]/events[at0006]/value/magnitude`). It embeds
// the shared [aql.IdentifiedPath] (Alias / Predicate / Segments / Raw), the
// same structured type an [aql.Comparison] carries on the WHERE side, without
// a package cycle, and adds the parse-only Clause and source Position. The
// embedded fields (Alias, Segments, …) are promoted.
type IdentifiedPath struct {
	aql.IdentifiedPath
	// Clause is the enclosing top-level clause.
	Clause Clause
	// Pos is the source position of the path.
	Pos Position
}

// extractor is an ANTLR listener that decorates the generated parse tree into
// the package's generated-type-free structures. It runs once per Parse.
type extractor struct {
	*gen.BaseAqlParserListener
	classes   []ClassExpr
	paths     []IdentifiedPath
	params    []string
	seenParam map[string]bool
}

func (d *Document) extract() {
	ex := &extractor{
		BaseAqlParserListener: &gen.BaseAqlParserListener{},
		seenParam:             map[string]bool{},
	}
	antlr.NewParseTreeWalker().Walk(ex, d.tree)
	d.Classes = ex.classes
	d.Paths = ex.paths
	d.Params = ex.params
}

// EnterClassExpression populates [Document.Classes]. Keep in lockstep
// with the structured extractor's extractClassExprOperand
// (extract_query.go) so a consumer reading the flat lint view and the
// structured Query sees identical raw [ClassExpr] fields for the same
// source — RMType / Alias / Archetype / Predicate / HasPredicate /
// ParsedPredicate, including the standing-predicate body and the
// param-archetype placeholder name carried verbatim in Archetype. The
// structured extractor additionally populates [ClassExpr.PredicateComparison],
// which this flat lint view intentionally omits — that field's own
// godoc is the canonical statement of the rule for consumers (REQ-113:
// the classification belongs on the type a consumer holds).
func (e *extractor) EnterClassExpression(c *gen.ClassExpressionContext) {
	ce := ClassExpr{Pos: posOf(c.GetStart())}
	if ids := c.AllIDENTIFIER(); len(ids) > 0 {
		ce.RMType = ids[0].GetText()
	}
	if v := c.GetVariable(); v != nil {
		ce.Alias = v.GetText()
	}
	if pp := c.PathPredicate(); pp != nil {
		ce.HasPredicate = true
		// REQ-113 § Structured node predicates: the class bracket is the same
		// production as a path step, so it gets the same typed form.
		ce.ParsedPredicate = structuredPathPredicate(pp)
		// REQ-113: every literal HRID the bracket names, structured or not.
		ce.Archetypes = bracketArchetypes(pp)
		switch {
		case pp.ArchetypePredicate() != nil:
			ap := pp.ArchetypePredicate()
			if hrid := ap.ARCHETYPE_HRID(); hrid != nil {
				ce.Archetype = hrid.GetText()
			} else if p := ap.PARAMETER(); p != nil {
				ce.Archetype = p.GetText()
				ce.ParamArchetype = true
			}
		default:
			ce.Predicate = trimBrackets(sourceText(pp))
			ce.Archetype = namedArchetypeHRID(pp)
		}
	}
	e.classes = append(e.classes, ce)
}

func (e *extractor) EnterVersionClassExpr(c *gen.VersionClassExprContext) {
	ce := ClassExpr{RMType: "VERSION", Version: true, Pos: posOf(c.GetStart())}
	if v := c.GetVariable(); v != nil {
		ce.Alias = v.GetText()
	}
	if vp := c.VersionPredicate(); vp != nil {
		ce.HasPredicate = true
		// Spanned over the ENCLOSING brackets — see bracketInterior. The
		// versionPredicate production excludes them, so a child-rule span drops
		// whatever padding, comment or line break sits between.
		ce.Predicate = bracketInterior(c.SYM_LEFT_BRACKET(), c.SYM_RIGHT_BRACKET(), vp)
	}
	e.classes = append(e.classes, ce)
}

func (e *extractor) EnterIdentifiedPath(c *gen.IdentifiedPathContext) {
	// REQ-109: a bare `true` / `false` in a value position is a literal, not a
	// path — the SDK lexer hands it to this listener as an IDENTIFIER-only
	// identifiedPath. Skipping it keeps the flat lint view from reporting a
	// spurious aql_unknown_alias for a grammar-admitted, server-executable
	// shape, and keeps it in lockstep with the structured extractor
	// (terminalAsValue / extractColumnExpr), which lifts the same shape to a
	// typed [aql.Value].
	if isKeywordLiteral(c) {
		return
	}
	ip := IdentifiedPath{
		Pos: posOf(c.GetStart()), Clause: clauseOf(c),
		Raw: sourceText(c),
	}
	if id := c.IDENTIFIER(); id != nil {
		ip.Alias = id.GetText()
	}
	if pp := c.PathPredicate(); pp != nil {
		ip.Predicate = trimBrackets(sourceText(pp))
		// REQ-113 § Structured node predicates: the alias-root bracket is the
		// same production as a path step, so it gets the same typed form.
		ip.ParsedPredicate = structuredPathPredicate(pp)
	}
	if op := c.ObjectPath(); op != nil {
		for _, part := range op.AllPathPart() {
			seg := PathSegment{}
			if id := part.IDENTIFIER(); id != nil {
				seg.Name = id.GetText()
			}
			if pp := part.PathPredicate(); pp != nil {
				seg.Predicate = trimBrackets(sourceText(pp))
				// REQ-113 § Structured node predicates: the typed form beside
				// the verbatim text. Both path extractors populate it, because
				// their outputs are compared for equality.
				seg.Parsed = structuredPathPredicate(pp)
			}
			ip.Segments = append(ip.Segments, seg)
		}
	}
	e.paths = append(e.paths, ip)
}

// isKeywordLiteral reports whether c is a bare literal keyword occupying one of
// the two grammar positions where a literal is admitted but the lexer yields an
// IDENTIFIER: a comparison `terminal` (the right-hand operand) and a
// `columnExpr` (a SELECT projection item). The structured extractor lifts both
// to a typed [aql.Value] — [astExtractor.terminalAsValue] and
// [astExtractor.extractColumnExpr] — so the flat view must skip exactly the
// same shapes.
//
// An ORDER BY key is deliberately NOT included: `orderByExpr` admits only an
// identifiedPath, the [OrderTerm] AST carries no literal alternative, and
// ordering by a constant is not a shape the REQ-113 catalogue models — so
// `ORDER BY true` stays a path and keeps its alias check.
func isKeywordLiteral(c *gen.IdentifiedPathContext) bool {
	if _, ok := bareKeywordLiteral(c); !ok {
		return false
	}
	switch c.GetParent().(type) {
	case *gen.TerminalContext, *gen.ColumnExprContext:
		return true
	}
	return false
}

// bareKeywordLiteral lifts an identifiedPath that is a BARE literal keyword —
// no path predicate, no object path — to its typed [aql.Value]. Anything
// qualified by a predicate or a path tail (`true/nested`, `false[at0001]`) is a
// real path whatever it is rooted at, and reports ok=false.
func bareKeywordLiteral(c gen.IIdentifiedPathContext) (aql.Value, bool) {
	if c == nil || c.PathPredicate() != nil || c.ObjectPath() != nil {
		return nil, false
	}
	return keywordLiteralValue(c.GetText())
}

// VisitTerminal collects $parameter references from anywhere in the tree,
// de-duplicated and in first-seen (document) order, with the leading `$`
// stripped to match [aql.Query.Parameters] keys.
func (e *extractor) VisitTerminal(node antlr.TerminalNode) {
	if node.GetSymbol().GetTokenType() != gen.AqlLexerPARAMETER {
		return
	}
	name := strings.TrimPrefix(node.GetText(), "$")
	if e.seenParam[name] {
		return
	}
	e.seenParam[name] = true
	e.params = append(e.params, name)
}

// clauseOf walks up the parse tree from a node to its enclosing top-level
// clause. Identified paths only ever appear under SELECT, WHERE, or ORDER BY
// (predicates carry relative objectPaths, not identifiedPaths).
func clauseOf(t antlr.Tree) Clause {
	for p := t.GetParent(); p != nil; p = p.GetParent() {
		switch p.(type) {
		case *gen.SelectClauseContext:
			return ClauseSelect
		case *gen.WhereClauseContext:
			return ClauseWhere
		case *gen.OrderByClauseContext:
			return ClauseOrderBy
		}
	}
	return ClauseUnknown
}

func posOf(tok antlr.Token) Position {
	if tok == nil {
		return Position{}
	}
	// ANTLR columns are 0-based; expose 1-based to match SyntaxError.
	return Position{Line: tok.GetLine(), Col: tok.GetColumn() + 1}
}

func trimBrackets(s string) string {
	return strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
}
