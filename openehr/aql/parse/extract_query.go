package parse

// extract_query.go: REQ-113 Tier 2 — translate a validated ANTLR
// parse tree (gen.ISelectQueryContext) into the readable, generated-
// type-free [Query] AST. Pure recursive descent — no
// listeners, no shared mutable state between calls.
//
// Catalogue: since REQ-117 the supported shapes are the whole SDK grammar
// profile — the buildable grammar, the parser-only predicates (Not /
// Exists / Like / Matches incl. its TERMINOLOGY and {URI} operands),
// literal and star SELECT items, function calls on either side of a
// comparison, path-valued operands, boolean junctions at the FROM root
// and in WHERE, and — since REQ-118 — the deprecated `SELECT TOP` clause
// with its direction. The residual refusal is a numeric literal the AST
// cannot represent (see [aql.ErrIncompleteAST]); it — and any defensive gap a
// widened grammar would trip — surfaces as [aql.ErrIncompleteAST] from
// [ParseQuery] / [Document.QueryErr] so the loss is visible at parse time,
// not silently dropped at emit.

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/antlr4-go/antlr/v4"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse/gen"
)

// astExtractor threads incompleteness errors through a single extraction
// pass. Concrete methods append a reason via incomplete(); extractQuery
// joins the reasons into a single error wrapping ErrIncompleteAST.
type astExtractor struct {
	gaps []string
}

func (e *astExtractor) incomplete(format string, args ...any) {
	e.gaps = append(e.gaps, fmt.Sprintf(format, args...))
}

// err builds the joined ErrIncompleteAST when gaps were recorded.
func (e *astExtractor) err() error {
	if len(e.gaps) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", aql.ErrIncompleteAST, strings.Join(e.gaps, "; "))
}

// extractQuery turns a validated tree into a populated [Query]. Returns
// nil on a nil tree (no panic on a degenerate input). The clauses are
// populated independently — an absent WHERE/ORDER BY/LIMIT leaves the
// corresponding fields as their zero values (nil interface for Where /
// Limit / Offset; empty slice for OrderBy).
//
// Returns ([Query], ErrIncompleteAST) when extraction hit a catalogue
// gap; the [Query] is still populated best-effort for clauses that
// extracted cleanly. Caller decides whether the partial AST is useful.
func extractQuery(tree gen.ISelectQueryContext) (*Query, error) {
	if tree == nil {
		return nil, nil
	}
	ex := &astExtractor{}
	q := &Query{}
	if sc := tree.SelectClause(); sc != nil {
		q.Select = ex.extractSelectClause(sc)
	}
	if fc := tree.FromClause(); fc != nil {
		q.From = ex.extractFromClause(fc)
	}
	if wc := tree.WhereClause(); wc != nil {
		q.Where = ex.extractWhereClause(wc)
	}
	if oc := tree.OrderByClause(); oc != nil {
		q.OrderBy = ex.extractOrderBy(oc)
	}
	if lc := tree.LimitClause(); lc != nil {
		q.Limit, q.Offset = ex.extractLimit(lc)
	}
	err := ex.err()
	if err != nil {
		// Record the gap on the AST so [Query.Emit] refuses to
		// render an incomplete tree even when the caller ignored
		// this error return.
		q.incomplete = err
	}
	return q, err
}

// --- SELECT ----------------------------------------------------------

func (ex *astExtractor) extractSelectClause(c gen.ISelectClauseContext) SelectClause {
	out := SelectClause{Distinct: c.DISTINCT() != nil}
	// REQ-118: the deprecated `top` production
	// (`selectClause : SELECT DISTINCT? top? selectExpr …`) is IN-catalogue —
	// count and direction are both carried, since dropping the count turns a
	// bounded query into an unbounded one and dropping the direction selects
	// the opposite end of the result set.
	if top := c.Top(); top != nil {
		out.Top = ex.topClause(top)
	}
	columns := 0
	for _, item := range c.AllSelectExpr() {
		if item.SYM_ASTERISK() != nil {
			out.Star = true
			// REQ-117: record the star's POSITION as an item so a mixed
			// `SELECT *, col` projection stays order-preserving.
			out.Items = append(out.Items, SelectItem{Expr: StarExpr{}})
			continue
		}
		columns++
		out.Items = append(out.Items, ex.extractSelectItem(item))
	}
	// A bare `SELECT *` keeps its pre-REQ-117 shape — Star set, Items
	// empty — so consumers reading only the flag are unaffected. The
	// collapse is deliberately limited to a SINGLE star: `SELECT *, *` is
	// two projections and must keep both items, or the re-emitted query
	// silently changes arity.
	if out.Star && columns == 0 && len(out.Items) == 1 {
		out.Items = nil
	}
	return out
}

// topClause lifts the deprecated `top` production into the shared
// [aql.TopClause] vocabulary (REQ-118). A count outside Go `int` records the
// residual unrepresentable-numeric gap — the same treatment a `LIMIT` operand
// of that size already gets — rather than a top-specific one: truncating a
// row bound is silent data loss either way.
//
// Returns nil only after recording a gap, so a nil Top can never be read as
// "the source declared no bound": [Query.Emit] refuses the partial AST.
func (ex *astExtractor) topClause(c gen.ITopContext) *aql.TopClause {
	out := &aql.TopClause{}
	i := c.INTEGER()
	if i == nil {
		// Defensive: `top : TOP INTEGER direction=(FORWARD|BACKWARD)?` makes
		// the count mandatory, so this is unreachable against the current
		// profile.
		ex.incomplete("SELECT TOP %q carries no row count", c.GetText())
		return nil
	}
	n, err := strconv.Atoi(i.GetText())
	if err != nil {
		ex.incomplete("SELECT TOP integer literal %q out of range for int (%v)", i.GetText(), err)
		return nil
	}
	out.N = n
	if d := c.GetDirection(); d != nil {
		switch d.GetTokenType() {
		case gen.AqlParserFORWARD:
			out.Dir = aql.TopForward
		case gen.AqlParserBACKWARD:
			out.Dir = aql.TopBackward
		default:
			// Defensive: the label admits FORWARD | BACKWARD only. Record a
			// gap rather than emit a bound whose direction we dropped.
			ex.incomplete("SELECT TOP direction %q is outside the catalogue", d.GetText())
			return nil
		}
	}
	return out
}

func (ex *astExtractor) extractSelectItem(c gen.ISelectExprContext) SelectItem {
	item := SelectItem{}
	if id := c.IDENTIFIER(); id != nil {
		item.Alias = id.GetText()
	}
	if col := c.ColumnExpr(); col != nil {
		item.Expr = ex.extractColumnExpr(col)
	}
	return item
}

func (ex *astExtractor) extractColumnExpr(c gen.IColumnExprContext) SelectExpr {
	if ip := c.IdentifiedPath(); ip != nil {
		// REQ-117: a BARE `true` / `false` in a SELECT column position is a
		// literal projection, not a path — the SDK lexer lexes those keywords
		// as IDENTIFIER (the IDENTIFIER rule precedes BOOLEAN in AqlLexer.g4),
		// so they arrive here as an IDENTIFIER-only identifiedPath. This mirrors
		// the WHERE-side lift in [astExtractor.terminalAsValue]; the flat lint
		// view skips the identical shape via isKeywordLiteral.
		if v, ok := bareKeywordLiteral(ip); ok {
			return LiteralExpr{Value: v, Raw: sourceText(ip)}
		}
		return PathExpr{IdentifiedPath: extractIdentifiedPath(ip, ClauseSelect)}
	}
	if afc := c.AggregateFunctionCall(); afc != nil {
		return ex.extractAggregateFunctionCall(afc)
	}
	if fc := c.FunctionCall(); fc != nil {
		return ex.extractFunctionCall(fc)
	}
	if p := c.Primitive(); p != nil {
		// REQ-117: a primitive literal projection carries the shared
		// value vocabulary.
		return ex.primitiveAsSelectExpr(p)
	}
	// Defensive: every columnExpr alternative in the SDK grammar profile is
	// handled above, so this is unreachable today — record a gap rather
	// than drop a projection silently if the grammar ever widens.
	ex.incomplete("SELECT projection %q is outside the catalogue", c.GetText())
	return nil
}

// primitiveAsSelectExpr lifts a Primitive into a [LiteralExpr] (REQ-117).
// A primitive the value vocabulary cannot represent — an integer literal
// beyond int64 — records a gap rather than degrading the literal to a
// float, so the loss is never silently emitted.
func (ex *astExtractor) primitiveAsSelectExpr(p gen.IPrimitiveContext) SelectExpr {
	v := primitiveAsValue(p)
	if v == nil {
		ex.incomplete("primitive literal %q is out of range for the value vocabulary", p.GetText())
		return nil
	}
	// REQ-118: keep the source text — the typed value is canonical, and the
	// result schema names an unaliased literal column by what was written.
	return LiteralExpr{Value: v, Raw: sourceText(p)}
}

// sourceText returns a rule context's VERBATIM source span, taken from the
// character stream rather than from GetText() (REQ-118). GetText concatenates
// tokens and so drops interior whitespace — `- 5` would come back `-5`, which
// is a canonicalisation, exactly what the source-text field exists to avoid.
// Falls back to GetText when the context carries no usable stream (a
// hand-built tree in a test).
func sourceText(c antlr.ParserRuleContext) string {
	start, stop := c.GetStart(), c.GetStop()
	if start == nil || stop == nil {
		return c.GetText()
	}
	if in := start.GetInputStream(); in != nil {
		return in.GetTextFromInterval(antlr.NewInterval(start.GetStart(), stop.GetStop()))
	}
	return c.GetText()
}

func (ex *astExtractor) extractAggregateFunctionCall(c gen.IAggregateFunctionCallContext) FunctionCall {
	out := FunctionCall{Name: aggregateName(c)}
	if c.DISTINCT() != nil {
		out.Distinct = true
	}
	if c.SYM_ASTERISK() != nil {
		out.Star = true
		return out
	}
	if ip := c.IdentifiedPath(); ip != nil {
		out.Args = []SelectExpr{PathExpr{IdentifiedPath: extractIdentifiedPath(ip, ClauseSelect)}}
	}
	return out
}

func (ex *astExtractor) extractFunctionCall(c gen.IFunctionCallContext) FunctionCall {
	// Grammar alternative `functionCall : terminologyFunction` — the
	// TERMINOLOGY call has no `name` token and no Terminal children; its
	// three STRING arguments are modelled as literal operands (REQ-117).
	if tf := c.TerminologyFunction(); tf != nil {
		out := FunctionCall{Name: aql.TerminologyFunc}
		// REQ-118: each argument keeps its source text (the STRING token as
		// written, quotes included) alongside the unquoted typed value.
		for i, s := range terminologyArgs(tf) {
			lit := LiteralExpr{Value: s}
			if strs := tf.AllSTRING(); i < len(strs) {
				lit.Raw = strs[i].GetText()
			}
			out.Args = append(out.Args, lit)
		}
		return out
	}
	out := FunctionCall{Name: functionName(c)}
	for _, t := range c.AllTerminal() {
		// INVARIANT: terminalAsSelectExpr returns nil only after recording a
		// gap (an unrepresentable primitive, or a defensively-unhandled
		// alternative), so skipping the argument here can never be a silent
		// drop — [Query.Emit] refuses the resulting partial AST.
		if expr := ex.terminalAsSelectExpr(t); expr != nil {
			out.Args = append(out.Args, expr)
		}
	}
	return out
}

// terminalAsSelectExpr lifts a Terminal context into a SelectExpr — a
// PathExpr (identifiedPath), a nested FunctionCall, or a LiteralExpr for
// a parameter / primitive argument (`CONCAT('a', $p, LENGTH(x/y))` —
// REQ-117). It returns nil ONLY after recording a gap: either the terminal
// carries a primitive the value vocabulary cannot represent, or (defensively)
// it matches no grammar alternative at all.
func (ex *astExtractor) terminalAsSelectExpr(t gen.ITerminalContext) SelectExpr {
	if ip := t.IdentifiedPath(); ip != nil {
		// A bare `true` / `false` keyword lexes as IDENTIFIER, so it arrives
		// here as an identifiedPath. Lift it to a literal exactly as the
		// WHERE-side terminalAsValue does, or the same construct would model
		// as a path in SELECT and as a literal in WHERE (REQ-117).
		if v, ok := bareKeywordLiteral(ip); ok {
			return LiteralExpr{Value: v, Raw: sourceText(ip)}
		}
		return PathExpr{IdentifiedPath: extractIdentifiedPath(ip, ClauseSelect)}
	}
	if fc := t.FunctionCall(); fc != nil {
		return ex.extractFunctionCall(fc)
	}
	if p := t.PARAMETER(); p != nil {
		return LiteralExpr{Value: aql.ParamValue{Name: strings.TrimPrefix(p.GetText(), "$")}, Raw: p.GetText()}
	}
	if p := t.Primitive(); p != nil {
		return ex.primitiveAsSelectExpr(p)
	}
	// Defensive: `terminal : primitive | PARAMETER | identifiedPath |
	// functionCall` is fully covered above, so this is unreachable today —
	// record a gap rather than drop a function argument silently if the grammar
	// ever widens.
	ex.incomplete("function argument %q is outside the catalogue", t.GetText())
	return nil
}

// terminologyArgs lifts a terminologyFunction's three STRING arguments
// into the shared value vocabulary, in grammar order (operation, api,
// params) — REQ-117.
func terminologyArgs(c gen.ITerminologyFunctionContext) []aql.Value {
	strs := c.AllSTRING()
	out := make([]aql.Value, 0, len(strs))
	for _, s := range strs {
		out = append(out, unquoteAQLString(s.GetText()))
	}
	return out
}

func aggregateName(c gen.IAggregateFunctionCallContext) string {
	if tok := c.GetName(); tok != nil {
		return strings.ToUpper(tok.GetText())
	}
	// Walk children for a function-keyword terminal.
	for _, child := range c.GetChildren() {
		if term, ok := child.(antlr.TerminalNode); ok {
			text := term.GetText()
			if text != "" && text != "(" && text != ")" {
				return strings.ToUpper(text)
			}
		}
	}
	return ""
}

func functionName(c gen.IFunctionCallContext) string {
	if tok := c.GetName(); tok != nil {
		return strings.ToUpper(tok.GetText())
	}
	if id := c.IDENTIFIER(); id != nil {
		return strings.ToUpper(id.GetText())
	}
	if t := c.STRING_FUNCTION_ID(); t != nil {
		return strings.ToUpper(t.GetText())
	}
	if t := c.NUMERIC_FUNCTION_ID(); t != nil {
		return strings.ToUpper(t.GetText())
	}
	if t := c.DATE_TIME_FUNCTION_ID(); t != nil {
		return strings.ToUpper(t.GetText())
	}
	return ""
}

// --- FROM + CONTAINS -------------------------------------------------

func (ex *astExtractor) extractFromClause(c gen.IFromClauseContext) FromClause {
	out := FromClause{}
	fe := c.FromExpr()
	if fe == nil {
		return out
	}
	ce := fe.ContainsExpr()
	if ce == nil {
		return out
	}
	root := ex.extractContainment(ce)
	if root == nil {
		return out
	}
	// FromClause.Root captures the class at the FROM root; Contains
	// captures the chain BELOW it. A junction at the very root has no
	// single class, so it lands on FromClause.Junction instead — the
	// same containment tree the nested side uses (REQ-117) — and Root /
	// Contains stay zero.
	if root.Class.RMType == "" && len(root.Children) > 0 {
		out.Junction = root
		return out
	}
	out.Root = root.Class
	switch {
	case len(root.Children) == 1 && !root.Negated && root.ChildJoin == ContainsAnd:
		// `FROM <class> CONTAINS <subtree>` — promote the single
		// child to the From.Contains slot so the chained class is
		// directly readable.
		child := root.Children[0]
		out.Contains = &child
	case len(root.Children) > 0 || root.Negated:
		// Multi-child or negated subtree — preserve the synthetic
		// node so the operator and operands are both visible.
		out.Contains = &Containment{
			Children:  root.Children,
			ChildJoin: root.ChildJoin,
			Negated:   root.Negated,
		}
	}
	return out
}

// extractContainment turns a ContainsExpr into a Containment node.
//
// Grammar: containsExpr is one of
//   - classExprOperand
//   - classExprOperand (NOT? CONTAINS containsExpr)?   — the chain form
//   - containsExpr AND containsExpr | containsExpr OR containsExpr
//   - '(' containsExpr ')'
//
// The NOT belongs to the CONTAINS chain (the LHS classExprOperand is
// at the SAME node as the NOT token); the negation applies to the
// chained sub-tree, not to the LHS class.
func (ex *astExtractor) extractContainment(c gen.IContainsExprContext) *Containment {
	if c == nil {
		return nil
	}
	// Boolean junction: two operands with AND or OR between them.
	if c.AND() != nil || c.OR() != nil {
		join := ContainsAnd
		if c.OR() != nil {
			join = ContainsOr
		}
		operands := c.AllContainsExpr()
		out := &Containment{ChildJoin: join}
		for _, op := range operands {
			child := ex.extractContainment(op)
			if child == nil {
				// Defensive: a dropped operand would emit `A` where the
				// source said `A OR B` — one arm of the junction silently
				// gone, the substitution class this file must surface as a
				// gap instead (REQ-119).
				ex.incomplete("containment operand %q is outside the catalogue", op.GetText())
				continue
			}
			// REQ-117: flatten a same-operator operand so `A OR B OR C`
			// is ONE junction with three operands rather than the
			// parser's left-nested pair-of-pairs. Emission is unaffected
			// (a same-operator group needs no parentheses).
			if child.Class.RMType == "" && len(child.Children) > 0 && child.ChildJoin == join && !child.Negated {
				out.Children = append(out.Children, child.Children...)
				continue
			}
			out.Children = append(out.Children, *child)
		}
		return out
	}
	// Parenthesised inner contains: pass through.
	if c.SYM_LEFT_PAREN() != nil {
		kids := c.AllContainsExpr()
		if len(kids) > 0 {
			return ex.extractContainment(kids[0])
		}
	}
	// CONTAINS chain at this level — the LHS class is on THIS node
	// (via ClassExprOperand), the chained sub-tree is the inner
	// ContainsExpr. NOT at this level negates the chained sub-tree.
	if c.CONTAINS() != nil {
		node := &Containment{}
		if op := c.ClassExprOperand(); op != nil {
			node.Class = ex.extractClassExprOperand(op)
		}
		kids := c.AllContainsExpr()
		if len(kids) > 0 {
			child := ex.extractContainment(kids[0])
			if child != nil {
				if c.NOT() != nil {
					child.Negated = !child.Negated
				}
				node.Children = []Containment{*child}
			}
		}
		return node
	}
	// Bare class operand at this level (no CONTAINS chain).
	node := &Containment{}
	if op := c.ClassExprOperand(); op != nil {
		node.Class = ex.extractClassExprOperand(op)
	}
	return node
}

func (ex *astExtractor) extractClassExprOperand(c gen.IClassExprOperandContext) ClassExpr {
	switch v := c.(type) {
	case *gen.ClassExpressionContext:
		ce := ClassExpr{Pos: posOf(v.GetStart())}
		if ids := v.AllIDENTIFIER(); len(ids) > 0 {
			ce.RMType = ids[0].GetText()
		}
		if vv := v.GetVariable(); vv != nil {
			ce.Alias = vv.GetText()
		}
		if pp := v.PathPredicate(); pp != nil {
			ce.HasPredicate = true
			switch {
			case pp.ArchetypePredicate() != nil:
				ap := pp.ArchetypePredicate()
				if hrid := ap.ARCHETYPE_HRID(); hrid != nil {
					ce.Archetype = hrid.GetText()
				} else if p := ap.PARAMETER(); p != nil {
					// `[$name]` archetype predicate — store the
					// placeholder verbatim (with the leading `$`)
					// so the emitter re-emits the exact source
					// token; ParamArchetype is the typed signal.
					ce.Archetype = p.GetText()
					ce.ParamArchetype = true
				} else {
					// Defensive: `archetypePredicate : ARCHETYPE_HRID |
					// PARAMETER` — both handled. Leaving Archetype empty with
					// HasPredicate set would silently drop the predicate, a
					// row filter, from emission.
					ex.incomplete("archetype predicate %q is outside the catalogue", ap.GetText())
				}
			default:
				// Standing predicate (e.g. `[ehr_id/value=$x]`) — capture
				// verbatim so the emitter round-trips it, and expose a
				// structured {path, op, value} when it is a simple
				// comparison (REQ-113).
				ce.Predicate = trimBrackets(sourceText(pp))
				ce.PredicateComparison = standingComparison(pp.StandardPredicate())
			}
		}
		return ce
	case *gen.VersionClassExprContext:
		ce := ClassExpr{RMType: "VERSION", Version: true, Pos: posOf(v.GetStart())}
		if vv := v.GetVariable(); vv != nil {
			ce.Alias = vv.GetText()
		}
		if vp := v.VersionPredicate(); vp != nil {
			ce.HasPredicate = true
			// `versionPredicate` excludes its brackets (they belong to
			// classExprOperand), unlike `pathPredicate` which includes them —
			// trimBrackets is a no-op here and is kept off deliberately.
			ce.Predicate = sourceText(vp)
		}
		return ce
	}
	// Defensive: both classExprOperand alternatives are handled above. The
	// zero ClassExpr is refused loudly at Emit (validateContainmentTree), but
	// the gap belongs at PARSE time, where every sibling records it.
	ex.incomplete("class expression %q is outside the catalogue", c.GetText())
	return ClassExpr{}
}

// --- WHERE -----------------------------------------------------------

func (ex *astExtractor) extractWhereClause(c gen.IWhereClauseContext) aql.WhereExpr {
	if c == nil {
		return nil
	}
	for _, child := range c.GetChildren() {
		if we, ok := child.(gen.IWhereExprContext); ok {
			return ex.extractWhereExpr(we)
		}
	}
	// Defensive: a whereClause always carries a whereExpr child against the
	// current grammar. Returning nil WITHOUT the gap would silently emit the
	// query with its WHERE gone — a wider result set, err == nil — which is
	// the exact failure the file header promises cannot happen.
	ex.incomplete("WHERE clause %q carries no expression the catalogue models", c.GetText())
	return nil
}

func (ex *astExtractor) extractWhereExpr(c gen.IWhereExprContext) aql.WhereExpr {
	if c == nil {
		return nil
	}
	if c.NOT() != nil {
		// NOT applies to the next WhereExpr operand.
		ops := c.AllWhereExpr()
		if len(ops) == 0 {
			// Defensive: `NOT whereExpr` always carries its operand. A nil
			// return here erased the whole predicate silently (see
			// extractWhereClause above).
			ex.incomplete("NOT in %q carries no operand", c.GetText())
			return nil
		}
		return aql.Not(ex.extractWhereExpr(ops[0]))
	}
	if c.AND() != nil || c.OR() != nil {
		join := aql.OpAnd
		if c.OR() != nil {
			join = aql.OpOr
		}
		ops := c.AllWhereExpr()
		terms := make([]aql.WhereExpr, 0, len(ops))
		for i, op := range ops {
			t := ex.extractWhereExpr(op)
			if t == nil {
				ex.incomplete("AND/OR junction dropped operand %d (unsupported shape)", i)
				continue
			}
			// REQ-117: flatten a same-operator operand so `a AND b AND c`
			// is ONE [aql.Junction] with three terms — the documented
			// shared-vocabulary contract — rather than the parser's
			// left-nested pair-of-pairs. Emission is unaffected (a nested
			// same-operator junction needs no parentheses). The extractor
			// only ever builds value shapes, so DerefWhere is a pass-through
			// here — used anyway so every shape decision in the subsystem
			// goes through one door (the dispatch-site tripwire checks that).
			if norm, ok := aql.DerefWhere(t); ok {
				if inner, isJunction := norm.(aql.Junction); isJunction && inner.Op == join {
					terms = append(terms, inner.Terms...)
					continue
				}
			}
			terms = append(terms, t)
		}
		if join == aql.OpAnd {
			return aql.And(terms...)
		}
		return aql.Or(terms...)
	}
	if c.SYM_LEFT_PAREN() != nil {
		// Parenthesised: unwrap to the single inner WhereExpr.
		ops := c.AllWhereExpr()
		if len(ops) > 0 {
			return ex.extractWhereExpr(ops[0])
		}
	}
	if ie := c.IdentifiedExpr(); ie != nil {
		return ex.extractIdentifiedExpr(ie)
	}
	// Defensive: every whereExpr alternative is handled above. A silent nil
	// here would drop the predicate (or one junction arm) from the model.
	ex.incomplete("WHERE expression %q is outside the catalogue", c.GetText())
	return nil
}

func (ex *astExtractor) extractIdentifiedExpr(c gen.IIdentifiedExprContext) aql.WhereExpr {
	if c == nil {
		return nil
	}
	// EXISTS path
	if c.EXISTS() != nil {
		if ip := c.IdentifiedPath(); ip != nil {
			path := pathRaw(ip)
			return aql.Exists(path)
		}
		ex.incomplete("EXISTS operand %q is outside the catalogue", c.GetText())
		return nil
	}
	// Parenthesised inner identifiedExpr.
	if c.SYM_LEFT_PAREN() != nil {
		if inner := c.IdentifiedExpr(); inner != nil {
			return ex.extractIdentifiedExpr(inner)
		}
	}
	// LIKE / MATCHES forms (require a path on the left).
	if ip := c.IdentifiedPath(); ip != nil {
		path := pathRaw(ip)
		if c.LIKE() != nil {
			if op := c.LikeOperand(); op != nil {
				if v := likeOperandValue(op); v != nil {
					return aql.Like(path, v)
				}
			}
			// Defensive: likeOperand is `STRING | PARAMETER`, both
			// modelled — a gap here can only mean a widened grammar.
			ex.incomplete("LIKE operand in %q is outside the catalogue", c.GetText())
			return nil
		}
		if c.MATCHES() != nil {
			if op := c.MatchesOperand(); op != nil {
				return ex.matchesExpr(path, op)
			}
			ex.incomplete("MATCHES operand in %q is outside the catalogue", c.GetText())
			return nil
		}
		// path <op> terminal — the comparison form.
		if cmp := c.COMPARISON_OPERATOR(); cmp != nil {
			opStr := cmp.GetText()
			if t := c.Terminal(); t != nil {
				v, gap := ex.terminalAsValue(t)
				if gap != "" {
					ex.incomplete("comparison RHS terminal %q is outside the catalogue (%s)", t.GetText(), gap)
					return nil
				}
				if v != nil {
					// REQ-113: carry the structured path (alias +
					// segments) alongside the raw string so a consumer
					// reads it without re-splitting.
					parsed := extractIdentifiedPath(ip, ClauseWhere)
					return aql.Comparison{Path: path, Op: aql.Operator(opStr), Val: v, ParsedPath: &parsed.IdentifiedPath}
				}
			}
			ex.incomplete("comparison %q is outside the catalogue", c.GetText())
			return nil
		}
	}
	// Function-call LHS in WHERE (e.g. `LENGTH(x) > 5`) — grammar
	// alternative `functionCall COMPARISON_OPERATOR terminal`. The left
	// operand is a structured [aql.FuncCall] value (REQ-117).
	if fc := c.FunctionCall(); fc != nil {
		cmp, t := c.COMPARISON_OPERATOR(), c.Terminal()
		if cmp == nil || t == nil {
			// Defensive: the grammar's only functionCall alternative is
			// `functionCall COMPARISON_OPERATOR terminal`, so both are present
			// after a successful parse — record a gap rather than drop the
			// predicate silently if the grammar ever widens.
			ex.incomplete("function-call comparison %q is outside the catalogue (incomplete operator or right operand)", c.GetText())
			return nil
		}
		v, gap := ex.terminalAsValue(t)
		if gap != "" {
			ex.incomplete("comparison RHS terminal %q is outside the catalogue (%s)", t.GetText(), gap)
			return nil
		}
		left, gap := ex.functionCallAsValue(fc)
		if gap != "" {
			ex.incomplete("function-call WHERE LHS %q is outside the catalogue (%s)", fc.GetText(), gap)
			return nil
		}
		if left == nil || v == nil {
			// Defensive: both lifts return a non-empty gap for anything they
			// cannot model, so a nil-without-gap can only mean a widened
			// grammar handed us an operand shape neither recognises.
			ex.incomplete("function-call comparison %q is outside the catalogue (unrecognised operand)", c.GetText())
			return nil
		}
		return aql.Comparison{Op: aql.Operator(cmp.GetText()), Val: v, Left: left}
	}
	// Defensive: every identifiedExpr alternative in the SDK grammar profile is
	// handled above, so this is unreachable today — record a gap rather than
	// drop the predicate silently if the grammar ever widens.
	ex.incomplete("WHERE predicate %q is outside the catalogue", c.GetText())
	return nil
}

// functionCallAsValue lifts a functionCall context into an [aql.FuncCall]
// value — the shape used on both sides of a WHERE comparison and as a
// nested argument (REQ-117). Returns a non-empty gap reason when an
// argument is outside the value vocabulary.
func (ex *astExtractor) functionCallAsValue(c gen.IFunctionCallContext) (aql.Value, string) {
	// Grammar alternative `functionCall : terminologyFunction`.
	if tf := c.TerminologyFunction(); tf != nil {
		return aql.FuncCall{Name: aql.TerminologyFunc, Args: terminologyArgs(tf)}, ""
	}
	out := aql.FuncCall{Name: functionName(c)}
	for _, t := range c.AllTerminal() {
		v, gap := ex.terminalAsValue(t)
		if gap != "" {
			return nil, gap
		}
		if v == nil {
			return nil, fmt.Sprintf("unsupported argument %q", t.GetText())
		}
		out.Args = append(out.Args, v)
	}
	return out, ""
}

// --- ORDER BY + LIMIT ------------------------------------------------

func (ex *astExtractor) extractOrderBy(c gen.IOrderByClauseContext) []OrderTerm {
	terms := c.AllOrderByExpr()
	out := make([]OrderTerm, 0, len(terms))
	for _, t := range terms {
		ot := OrderTerm{Dir: OrderAsc}
		if ip := t.IdentifiedPath(); ip != nil {
			ot.Path = extractIdentifiedPath(ip, ClauseOrderBy)
		}
		if tok := t.GetOrder(); tok != nil {
			s := strings.ToUpper(tok.GetText())
			if s == "DESC" || s == "DESCENDING" {
				ot.Dir = OrderDesc
			}
		}
		out = append(out, ot)
	}
	return out
}

func (ex *astExtractor) extractLimit(c gen.ILimitClauseContext) (limit, offset LimitExpr) {
	limit = ex.limitValueAsExpr(c.GetLimit(), "LIMIT")
	offset = ex.limitValueAsExpr(c.GetOffset(), "OFFSET")
	return
}

func (ex *astExtractor) limitValueAsExpr(v gen.ILimitValueContext, clause string) LimitExpr {
	if v == nil {
		return nil
	}
	if t := v.INTEGER(); t != nil {
		text := t.GetText()
		n, err := strconv.Atoi(text)
		if err == nil {
			return IntLimit{N: n}
		}
		// Integer too large for int (overflow / out of range). Record a
		// catalogue gap so the clause isn't silently dropped — the
		// emit-on-partial-AST guard then refuses to render this AST.
		ex.incomplete("%s integer literal %q out of range for int (%v)", clause, text, err)
		return nil
	}
	if t := v.PARAMETER(); t != nil {
		return ParamLimit{Name: strings.TrimPrefix(t.GetText(), "$")}
	}
	// Defensive: `limitValue : INTEGER | PARAMETER` is fully covered above,
	// so this is unreachable today. Record a gap rather than drop the whole
	// clause if the grammar profile ever widens — a dropped LIMIT/OFFSET
	// silently returns more rows than the source asked for.
	ex.incomplete("%s value %q is outside the catalogue", clause, v.GetText())
	return nil
}

// --- shared helpers --------------------------------------------------

// extractIdentifiedPath mirrors the ast.go listener's path extraction
// shape — used by the structured astExtractor to produce identical
// IdentifiedPath values, so consumers can compare paths from
// Document.Paths and Query SELECT/WHERE/ORDER BY by equality.
func extractIdentifiedPath(c gen.IIdentifiedPathContext, clause Clause) IdentifiedPath {
	ip := IdentifiedPath{Pos: posOf(c.GetStart()), Clause: clause}
	ip.Raw = sourceText(c)
	if id := c.IDENTIFIER(); id != nil {
		ip.Alias = id.GetText()
	}
	if pp := c.PathPredicate(); pp != nil {
		ip.Predicate = trimBrackets(sourceText(pp))
	}
	if op := c.ObjectPath(); op != nil {
		ip.Segments = segmentsFromObjectPath(op)
	}
	return ip
}

// segmentsFromObjectPath decomposes a relative objectPath (the path steps after
// any alias root) into the shared path-segment vocabulary — the loop shared by
// extractIdentifiedPath (alias-qualified SELECT/WHERE/ORDER BY paths) and
// standingComparison (a class-predicate relative path). Returns nil for a nil
// objectPath.
func segmentsFromObjectPath(op gen.IObjectPathContext) []aql.PathSegment {
	if op == nil {
		return nil
	}
	var segs []aql.PathSegment
	for _, part := range op.AllPathPart() {
		seg := aql.PathSegment{}
		if id := part.IDENTIFIER(); id != nil {
			seg.Name = id.GetText()
		}
		if pp := part.PathPredicate(); pp != nil {
			seg.Predicate = trimBrackets(sourceText(pp))
		}
		segs = append(segs, seg)
	}
	return segs
}

func pathRaw(c gen.IIdentifiedPathContext) string {
	return sourceText(c)
}

// standingComparison lifts a class standing predicate's standardPredicate
// (`objectPath <op> operand`) into an [*aql.Comparison] for REQ-113, or
// nil when the predicate is absent or its RHS operand is not a scalar value
// (an objectPath / node-code operand). Path is the relative object path as
// written; ParsedPath carries its decomposed Segments with an empty Alias —
// the class-predicate path binds no FROM alias, but its steps are structured
// so a consumer need not re-split Path (the WHERE-side symmetry). The verbatim
// [ClassExpr.Predicate] text remains the round-trip source regardless.
func standingComparison(sp gen.IStandardPredicateContext) *aql.Comparison {
	if sp == nil {
		return nil
	}
	op, cmp, operand := sp.ObjectPath(), sp.COMPARISON_OPERATOR(), sp.PathPredicateOperand()
	if op == nil || cmp == nil || operand == nil {
		return nil
	}
	v := pathPredicateOperandValue(operand)
	if v == nil {
		return nil
	}
	raw := sourceText(op)
	parsed := aql.IdentifiedPath{Segments: segmentsFromObjectPath(op), Raw: raw}
	return &aql.Comparison{Path: raw, Op: aql.Operator(cmp.GetText()), Val: v, ParsedPath: &parsed}
}

// pathPredicateOperandValue lifts a standing-predicate RHS operand into an
// [aql.Value] — a primitive literal or a $parameter. A path-valued operand
// (objectPath) or a node code (ID_CODE / AT_CODE) is not a scalar value and
// returns nil, leaving [ClassExpr.PredicateComparison] nil.
func pathPredicateOperandValue(c gen.IPathPredicateOperandContext) aql.Value {
	if c == nil {
		return nil
	}
	if p := c.Primitive(); p != nil {
		return primitiveAsValue(p)
	}
	if t := c.PARAMETER(); t != nil {
		return aql.ParamValue{Name: strings.TrimPrefix(t.GetText(), "$")}
	}
	return nil
}

// terminalAsValue lifts a comparison terminal into an [aql.Value].
//
// Grammar: terminal is one of `primitive | PARAMETER | identifiedPath |
// functionCall`. Primitives and parameters map to the literal vocabulary;
// an identifiedPath whose text is the bare keyword `true` / `false` /
// `null` is normalised to the typed literal (the SDK grammar lexes those
// as IDENTIFIER because the IDENTIFIER rule precedes BOOLEAN in
// AqlLexer.g4), any other identifiedPath becomes an [aql.PathValue]
// (path-vs-path comparison, REQ-117), and a functionCall becomes an
// [aql.FuncCall]. A non-empty gap string reports a value the vocabulary
// cannot represent (an out-of-range integer literal) for the caller to
// record.
func (ex *astExtractor) terminalAsValue(c gen.ITerminalContext) (aql.Value, string) {
	if c == nil {
		return nil, ""
	}
	if t := c.PARAMETER(); t != nil {
		name := strings.TrimPrefix(t.GetText(), "$")
		return aql.ParamValue{Name: name}, ""
	}
	if p := c.Primitive(); p != nil {
		v := primitiveAsValue(p)
		if v == nil {
			return nil, fmt.Sprintf("literal %q is out of range for the value vocabulary", p.GetText())
		}
		return v, ""
	}
	if ip := c.IdentifiedPath(); ip != nil {
		if v, ok := bareKeywordLiteral(ip); ok {
			return v, ""
		}
		// REQ-117: an identified path in a value position — carried as
		// structured alias + segments, not raw text.
		parsed := extractIdentifiedPath(ip, ClauseWhere)
		return aql.PathValue{IdentifiedPath: parsed.IdentifiedPath}, ""
	}
	if fc := c.FunctionCall(); fc != nil {
		return ex.functionCallAsValue(fc)
	}
	return nil, ""
}

// keywordLiteralValue maps a bare literal keyword occupying a value position —
// a comparison `terminal` or a SELECT `columnExpr` — to its typed [aql.Value].
// The SDK lexer lexes `true` / `false` as IDENTIFIER (the IDENTIFIER rule
// precedes BOOLEAN in AqlLexer.g4), so they arrive as an IDENTIFIER-only
// identifiedPath rather than a primitive; `null` is included because the same
// rule order could shift. Reports ok=false for any other text.
//
// Reached through bareKeywordLiteral, which adds the shape gate. Shared by the
// structured extractor ([astExtractor.terminalAsValue],
// [astExtractor.extractColumnExpr]) and the flat lint view
// ([extractor.EnterIdentifiedPath]) so both agree these are literals, not
// paths (REQ-117).
func keywordLiteralValue(text string) (aql.Value, bool) {
	switch strings.ToLower(text) {
	case "true":
		return aql.BoolValue{B: true}, true
	case "false":
		return aql.BoolValue{B: false}, true
	case "null":
		return aql.NullValue{}, true
	}
	return nil, false
}

// primitiveAsValue lifts a Primitive to an [aql.Value] — STRING /
// numeric / BOOLEAN / DATE / TIME / DATETIME / NULL. Surface text
// canonicalisation: STRING strips outer quotes and resolves the
// grammar's escape sequences ([unescapeAQLString]);
// DATE/TIME/DATETIME strip outer single quotes from the lexer
// token (the lexer rule includes them); NULL maps to the typed
// [aql.NullValue] sentinel rather than a quoted string literal.
func primitiveAsValue(c gen.IPrimitiveContext) aql.Value {
	if t := c.STRING(); t != nil {
		return unquoteAQLString(t.GetText())
	}
	if t := c.BOOLEAN(); t != nil {
		return aql.BoolValue{B: strings.EqualFold(t.GetText(), "true")}
	}
	if np := c.NumericPrimitive(); np != nil {
		return numericPrimitiveAsValue(np)
	}
	if t := c.DATE(); t != nil {
		return aql.StringValue{S: stripSurroundingQuotes(t.GetText())}
	}
	if t := c.TIME(); t != nil {
		return aql.StringValue{S: stripSurroundingQuotes(t.GetText())}
	}
	if t := c.DATETIME(); t != nil {
		return aql.StringValue{S: stripSurroundingQuotes(t.GetText())}
	}
	if c.NULL() != nil {
		return aql.NullValue{}
	}
	return nil
}

// unquoteAQLString inverts [aql.StringValue.token]: strips outer
// quotes (single or double, the grammar admits both) and resolves the
// escape sequences the STRING token admits ([unescapeAQLString] —
// there is no SQL-style quote doubling in AQL). Falls back to the raw
// text when the input lacks recognised delimiters.
func unquoteAQLString(raw string) aql.Value {
	if len(raw) >= 2 {
		first, last := raw[0], raw[len(raw)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			return aql.StringValue{S: unescapeAQLString(raw[1 : len(raw)-1])}
		}
	}
	return aql.StringValue{S: raw}
}

// unescapeAQLString resolves the STRING token's escape forms into the runes
// they denote — the inverse of aql.StringValue.token.
//
// The lexer admits three escape shapes inside a quoted STRING
// (resources/aql/grammar/active/AqlLexer.g4): `ESCAPE_SEQ` (`'\\'
// ['"?abfnrtv\\*]`), `UTF8CHAR` (`'\\u' HEX HEX HEX HEX`), and `OCTAL_ESC`
// (one to three octal digits behind a backslash). It admits no SQL-style
// quote doubling — a doubled quote lexes as two adjacent STRING tokens
// (`'O'` then `'Brien'`), which is a syntax error, not one escaped literal,
// so a doubled quote can never reach this function inside one token and the
// SQL unescaping this function used to do was unreachable for grammar-valid
// input while being the wrong inverse for the escaped form that does occur.
//
// A trailing lone backslash cannot occur in a lexed token (it would escape the
// closing quote), but is passed through rather than dropped so a hand-built
// caller cannot lose a character silently.
func unescapeAQLString(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '\\' || i+1 >= len(s) {
			sb.WriteByte(s[i])
			i++
			continue
		}
		switch c := s[i+1]; {
		case c == 'u' && i+5 < len(s):
			// bitSize 16, not 32: four hex digits cannot exceed 0xFFFF, and
			// saying so makes the conversion to rune provably lossless rather
			// than merely unreachable (CodeQL flags the wider form).
			r, err := strconv.ParseUint(s[i+2:i+6], 16, 16)
			if err != nil {
				sb.WriteByte(s[i])
				i++
				continue
			}
			// UTF8CHAR is exactly four hex digits, so a non-BMP character has no
			// other spelling than a UTF-16 surrogate PAIR — which is how any
			// JSON/JavaScript-derived client writes an emoji or a CJK extension.
			// WriteRune substitutes U+FFFD for a lone surrogate, so combining the
			// pair here is what keeps such a literal from decoding to two
			// replacement characters, silently (REQ-119).
			if lo, ok := trailingSurrogate(s, i, rune(r)); ok {
				sb.WriteRune(utf16.DecodeRune(rune(r), lo))
				i += 12
				continue
			}
			// An unpaired half denotes no character. WriteRune renders it U+FFFD,
			// which is the lenient reading and the one the lexer's own input
			// decoding already applies to malformed bytes.
			sb.WriteRune(rune(r))
			i += 6
		case c >= '0' && c <= '7':
			// OCTAL_ESC is greedy up to three digits, but `\0`–`\3` may lead a
			// three-digit form while `\4`–`\7` may lead at most two.
			n := 1
			for n < 3 && i+1+n < len(s) && s[i+1+n] >= '0' && s[i+1+n] <= '7' {
				n++
			}
			if n == 3 && c > '3' {
				n = 2
			}
			// bitSize 8 matches the byte this writes: with the clamp above the
			// value cannot exceed 0o377, and pinning it means a regression in
			// that clamp takes the pass-through arm below instead of silently
			// truncating (bitSize 16 would yield 256, and byte(256) == 0).
			v, err := strconv.ParseUint(s[i+1:i+1+n], 8, 8)
			if err != nil {
				sb.WriteByte(s[i])
				i++
				continue
			}
			sb.WriteByte(byte(v))
			i += 1 + n
		default:
			if r, ok := aqlEscapeChar[c]; ok {
				sb.WriteByte(r)
				i += 2
				continue
			}
			// Not an ESCAPE_SEQ member: the lexer would not have produced it,
			// so keep both bytes rather than inventing a meaning for it.
			sb.WriteByte(s[i])
			i++
		}
	}
	return sb.String()
}

// trailingSurrogate reports the low half of a UTF-16 surrogate pair when hi is a
// high surrogate and the very next thing in s is a `\uXXXX` spelling a low one.
// i indexes the backslash of the escape that produced hi, so the candidate
// occupies s[i+6:i+12].
func trailingSurrogate(s string, i int, hi rune) (rune, bool) {
	if !utf16.IsSurrogate(hi) || hi > 0xDBFF || i+12 > len(s) ||
		s[i+6] != '\\' || s[i+7] != 'u' {
		return 0, false
	}
	lo, err := strconv.ParseUint(s[i+8:i+12], 16, 16)
	if err != nil || rune(lo) < 0xDC00 || rune(lo) > 0xDFFF {
		return 0, false
	}
	return rune(lo), true
}

// aqlEscapeChar maps the ESCAPE_SEQ suffixes to the bytes they denote.
// `\?` and `\*` are identity escapes the grammar admits (the latter is the
// SDK-AQL-004 profile addition); the rest are the C escapes.
var aqlEscapeChar = map[byte]byte{
	'\'': '\'', '"': '"', '?': '?', '*': '*', '\\': '\\',
	'a': '\a', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v',
}

// stripSurroundingQuotes peels one set of quote delimiters from a
// DATE/TIME/DATETIME lexer token (`'2026-01-01T00:00:00'` →
// `2026-01-01T00:00:00`), so the emitter's StringValue.token re-quotes
// cleanly instead of producing triple-quoted text.
//
// BOTH delimiters, because the three tokens admit both — `DATETIME :
// SYM_SINGLE_QUOTE … | SYM_DOUBLE_QUOTE ISO8601_DATE_TIME SYM_DOUBLE_QUOTE`
// (AqlLexer.g4). Peeling single quotes only made a double-quoted temporal
// literal round-trip with its quotes EMBEDDED in the value — `= "2026-…"`
// re-emitted as `'"2026-…"'`, a comparison that matches nothing, err == nil
// (REQ-119's silent class; the sibling unquoteAQLString always handled both).
// The token body is an ISO-8601 datetime, so it can contain neither delimiter
// and needs no unescaping.
func stripSurroundingQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"')) {
		return s[1 : len(s)-1]
	}
	return s
}

func numericPrimitiveAsValue(c gen.INumericPrimitiveContext) aql.Value {
	// `numericPrimitive : … | SYM_MINUS numericPrimitive` is RECURSIVE, so
	// descend while there are minuses to strip and let the parity decide the
	// sign — stopping after one level made the grammar-legal `- -5` arrive here
	// as the text `--5` and be reported as an out-of-range literal.
	//
	// The sign is then carried as TEXT rather than applied as a multiplier after
	// parsing the magnitude: int64's range is asymmetric, so `math.MinInt64`
	// has no positive counterpart and parsing its magnitude first overflows a
	// value that is exactly representable once signed.
	negative := false
	for c.SYM_MINUS() != nil {
		inner := c.NumericPrimitive()
		if inner == nil {
			break
		}
		negative = !negative
		c = inner
	}
	sign := ""
	if negative {
		sign = "-"
	}
	if t := c.INTEGER(); t != nil {
		if n, err := strconv.ParseInt(sign+t.GetText(), 10, 64); err == nil {
			return aql.IntValue{N: n}
		}
		return nil
	}
	// The three real-valued token shapes share one conversion; a switch keeps
	// the early-out and allocates nothing per literal.
	var t antlr.TerminalNode
	switch {
	case c.REAL() != nil:
		t = c.REAL()
	case c.SCI_INTEGER() != nil:
		t = c.SCI_INTEGER()
	case c.SCI_REAL() != nil:
		t = c.SCI_REAL()
	default:
		return nil
	}
	if f, err := strconv.ParseFloat(sign+t.GetText(), 64); err == nil {
		return aql.RealValue{F: f}
	}
	return nil
}

func likeOperandValue(c gen.ILikeOperandContext) aql.Value {
	if t := c.STRING(); t != nil {
		return unquoteAQLString(t.GetText())
	}
	if t := c.PARAMETER(); t != nil {
		return aql.ParamValue{Name: strings.TrimPrefix(t.GetText(), "$")}
	}
	return nil
}

// matchesExpr models a MATCHES predicate's right-hand side (REQ-117).
//
// Grammar: `matchesOperand : '{' valueListItem (',' valueListItem)* '}' |
// terminologyFunction | '{' URI '}'`. All three forms are structured: the
// value list into [aql.MatchesExpr.Values] (a `valueListItem` may itself
// be a terminologyFunction, modelled as an [aql.FuncCall]), the bare
// terminology function into Terminology, and the URI — carried verbatim,
// the lexer token is unquoted — into URI.
func (ex *astExtractor) matchesExpr(path string, c gen.IMatchesOperandContext) aql.WhereExpr {
	if tf := c.TerminologyFunction(); tf != nil {
		fc := aql.FuncCall{Name: aql.TerminologyFunc, Args: terminologyArgs(tf)}
		return aql.MatchesExpr{Path: path, Terminology: &fc}
	}
	if u := c.URI(); u != nil {
		return aql.MatchesExpr{Path: path, URI: u.GetText()}
	}
	items := c.AllValueListItem()
	out := make([]aql.Value, 0, len(items))
	for _, it := range items {
		if p := it.Primitive(); p != nil {
			v := primitiveAsValue(p)
			if v == nil {
				ex.incomplete("MATCHES value %q is out of range for the value vocabulary", p.GetText())
				continue
			}
			out = append(out, v)
			continue
		}
		if t := it.PARAMETER(); t != nil {
			out = append(out, aql.ParamValue{Name: strings.TrimPrefix(t.GetText(), "$")})
			continue
		}
		if tf := it.TerminologyFunction(); tf != nil {
			out = append(out, aql.FuncCall{Name: aql.TerminologyFunc, Args: terminologyArgs(tf)})
			continue
		}
		// Defensive: `valueListItem : primitive | PARAMETER |
		// terminologyFunction`, all handled above. Skipping an unmodelled
		// member would emit a NARROWER list than the source — valid AQL
		// matching fewer values, silently — so a widened grammar must land
		// here as a gap instead.
		ex.incomplete("MATCHES member %q is outside the catalogue", it.GetText())
	}
	if len(out) == 0 {
		// The grammar requires at least one valueListItem, so an empty
		// list means every member was refused above (each recorded a gap)
		// — never a silent drop.
		return nil
	}
	return aql.Matches(path, out...)
}
