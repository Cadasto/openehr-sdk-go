package parse

// extract_query.go: REQ-113 Tier 2 — translate a validated ANTLR
// parse tree (gen.ISelectQueryContext) into the readable, generated-
// type-free [Query] AST. Pure recursive descent — no
// listeners, no shared mutable state between calls.
//
// Catalogue: the v1 supported shapes are the buildable grammar plus the
// parser-only shapes (Not / Exists / Like / Matches). Inputs that parse
// cleanly but contain an out-of-catalogue shape surface as
// [aql.ErrIncompleteAST] from [ParseQuery] / [Document.QueryErr] so the
// loss is visible at parse time, not silently dropped at emit.

import (
	"fmt"
	"strconv"
	"strings"

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
	for _, item := range c.AllSelectExpr() {
		if item.SYM_ASTERISK() != nil {
			out.Star = true
			continue
		}
		out.Items = append(out.Items, ex.extractSelectItem(item))
	}
	// Star + columns mix: grammar permits it but the structured Query
	// has no carrier for both. Surface so emit doesn't silently drop
	// the column list.
	if out.Star && len(out.Items) > 0 {
		ex.incomplete("SELECT mixes `*` with column projections — only one form is supported per query")
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
		return PathExpr{IdentifiedPath: extractIdentifiedPath(ip, ClauseSelect)}
	}
	if afc := c.AggregateFunctionCall(); afc != nil {
		return ex.extractAggregateFunctionCall(afc)
	}
	if fc := c.FunctionCall(); fc != nil {
		return ex.extractFunctionCall(fc)
	}
	if p := c.Primitive(); p != nil {
		ex.incomplete("Primitive literal in SELECT projection (`%s`) is outside the v1 catalogue", p.GetText())
		return nil
	}
	return nil
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
	out := FunctionCall{Name: functionName(c)}
	for _, t := range c.AllTerminal() {
		if expr := ex.terminalAsSelectExpr(t); expr != nil {
			out.Args = append(out.Args, expr)
		}
	}
	return out
}

// terminalAsSelectExpr lifts a Terminal context into a SelectExpr —
// either a PathExpr (when the terminal carries an identifiedPath) or
// a nested FunctionCall. Parameter and Primitive terminals (e.g.
// `MAX(42)`, `COUNT($id)`) are outside the SELECT vocabulary today;
// the caller decides whether to record a catalogue gap.
func (ex *astExtractor) terminalAsSelectExpr(t gen.ITerminalContext) SelectExpr {
	if ip := t.IdentifiedPath(); ip != nil {
		return PathExpr{IdentifiedPath: extractIdentifiedPath(ip, ClauseSelect)}
	}
	if fc := t.FunctionCall(); fc != nil {
		return ex.extractFunctionCall(fc)
	}
	if t.PARAMETER() != nil || t.Primitive() != nil {
		ex.incomplete("Parameter or Primitive argument in function call (`%s`) is outside the v1 SELECT catalogue", t.GetText())
	}
	return nil
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
	// single class — the emitter requires one, so surface the gap
	// rather than silently emit `missing FROM root` at emit time.
	if root.Class.RMType == "" && len(root.Children) > 0 {
		ex.incomplete("FROM top-level boolean junction (`FROM A %s B`) is outside the v1 catalogue", root.ChildJoin.String())
		// Best-effort: hoist the first child's class as Root so a
		// caller inspecting the AST gets something rather than zero.
		first := root.Children[0]
		out.Root = first.Class
		if rest := root.Children[1:]; len(rest) > 0 {
			out.Contains = &Containment{Children: rest, ChildJoin: root.ChildJoin}
		}
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
			if child != nil {
				out.Children = append(out.Children, *child)
			}
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
				}
			default:
				// Standing predicate (e.g. `[ehr_id/value=$x]`) — capture
				// verbatim so the emitter round-trips it, and expose a
				// structured {path, op, value} when it is a simple
				// comparison (REQ-113).
				ce.Predicate = trimBrackets(pp.GetText())
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
			ce.Predicate = trimBrackets(vp.GetText())
		}
		return ce
	}
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
			return nil
		}
		return aql.Not(ex.extractWhereExpr(ops[0]))
	}
	if c.AND() != nil || c.OR() != nil {
		ops := c.AllWhereExpr()
		terms := make([]aql.WhereExpr, 0, len(ops))
		for i, op := range ops {
			t := ex.extractWhereExpr(op)
			if t == nil {
				ex.incomplete("AND/OR junction dropped operand %d (unsupported shape)", i)
				continue
			}
			terms = append(terms, t)
		}
		if c.AND() != nil {
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
			return nil
		}
		if c.MATCHES() != nil {
			if op := c.MatchesOperand(); op != nil {
				vs, ok := ex.matchesOperandValues(op)
				if !ok {
					return nil
				}
				if len(vs) > 0 {
					return aql.Matches(path, vs...)
				}
			}
			return nil
		}
		// path <op> terminal — the comparison form.
		if cmp := c.COMPARISON_OPERATOR(); cmp != nil {
			opStr := cmp.GetText()
			if t := c.Terminal(); t != nil {
				v, gap := ex.terminalAsValue(t)
				if gap != "" {
					ex.incomplete("comparison RHS terminal %q is outside the v1 catalogue (%s)", t.GetText(), gap)
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
		}
	}
	// Function-call LHS in WHERE (e.g. `LENGTH(x) > 5`) — grammar
	// alternative `functionCall COMPARISON_OPERATOR terminal`. Outside
	// the v1 catalogue; surface so the predicate isn't silently lost.
	if c.FunctionCall() != nil {
		ex.incomplete("function-call WHERE LHS (`%s`) is outside the v1 catalogue", c.GetText())
	}
	return nil
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
	return nil
}

// --- shared helpers --------------------------------------------------

// extractIdentifiedPath mirrors the ast.go listener's path extraction
// shape — used by the structured astExtractor to produce identical
// IdentifiedPath values, so consumers can compare paths from
// Document.Paths and Query SELECT/WHERE/ORDER BY by equality.
func extractIdentifiedPath(c gen.IIdentifiedPathContext, clause Clause) IdentifiedPath {
	ip := IdentifiedPath{Pos: posOf(c.GetStart()), Clause: clause}
	ip.Raw = c.GetText()
	if id := c.IDENTIFIER(); id != nil {
		ip.Alias = id.GetText()
	}
	if pp := c.PathPredicate(); pp != nil {
		ip.Predicate = trimBrackets(pp.GetText())
	}
	if op := c.ObjectPath(); op != nil {
		for _, part := range op.AllPathPart() {
			seg := PathSegment{}
			if id := part.IDENTIFIER(); id != nil {
				seg.Name = id.GetText()
			}
			if pp := part.PathPredicate(); pp != nil {
				seg.Predicate = trimBrackets(pp.GetText())
			}
			ip.Segments = append(ip.Segments, seg)
		}
	}
	return ip
}

func pathRaw(c gen.IIdentifiedPathContext) string {
	return c.GetText()
}

// standingComparison lifts a class standing predicate's standardPredicate
// (`objectPath <op> operand`) into an [*aql.Comparison] for REQ-113, or
// nil when the predicate is absent or its RHS operand is not a scalar value
// (an objectPath / node-code operand). Path is the relative object path as
// written; ParsedPath is left nil (a class predicate path has no alias to
// structure). The verbatim [ClassExpr.Predicate] text remains the round-trip
// source regardless.
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
	return &aql.Comparison{Path: op.GetText(), Op: aql.Operator(cmp.GetText()), Val: v}
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

// terminalAsValue lifts a comparison-RHS terminal into an [aql.Value].
//
// Grammar: terminal is one of `primitive | PARAMETER | identifiedPath |
// functionCall`. The first two map cleanly; an identifiedPath whose text
// is the bare boolean keyword `true` / `false` is normalised to a
// [BoolValue] (the SDK grammar lexes those as IDENTIFIER because the
// IDENTIFIER rule precedes BOOLEAN in AqlLexer.g4). Other identifiedPath
// shapes (path-vs-path comparisons like `a/x = b/y`) and functionCall
// terminals report a non-empty gap string for the caller to record.
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
			return nil, "unsupported primitive form"
		}
		return v, ""
	}
	if ip := c.IdentifiedPath(); ip != nil {
		// Boolean keyword lexed as IDENTIFIER (lexer rule order:
		// IDENTIFIER precedes BOOLEAN, so `true` / `false` parse as
		// an IDENTIFIER-only IdentifiedPath).
		txt := ip.GetText()
		switch strings.ToLower(txt) {
		case "true":
			return aql.BoolValue{B: true}, ""
		case "false":
			return aql.BoolValue{B: false}, ""
		case "null":
			return aql.NullValue{}, ""
		}
		return nil, "identifiedPath RHS (path-vs-path comparison)"
	}
	if c.FunctionCall() != nil {
		return nil, "functionCall RHS"
	}
	return nil, ""
}

// primitiveAsValue lifts a Primitive to an [aql.Value] — STRING /
// numeric / BOOLEAN / DATE / TIME / DATETIME / NULL. Surface text
// canonicalisation: STRING strips outer single quotes and undoes
// the AQL embedded-quote escape (two consecutive single quotes →
// one); DATE/TIME/DATETIME strip outer single quotes from the lexer
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
// quotes (single or double, the grammar admits both) and undoes the
// AQL embedded-quote escape for single-quoted literals (two
// consecutive single quotes → one). Falls back to the raw text when
// the input lacks recognised delimiters.
func unquoteAQLString(raw string) aql.Value {
	if len(raw) >= 2 {
		first, last := raw[0], raw[len(raw)-1]
		if first == '\'' && last == '\'' {
			inner := raw[1 : len(raw)-1]
			return aql.StringValue{S: strings.ReplaceAll(inner, "''", "'")}
		}
		if first == '"' && last == '"' {
			inner := raw[1 : len(raw)-1]
			return aql.StringValue{S: strings.ReplaceAll(inner, `""`, `"`)}
		}
	}
	return aql.StringValue{S: raw}
}

// stripSurroundingQuotes peels a single set of single-quote delimiters
// from a DATE/TIME/DATETIME lexer token (`'2026-01-01T00:00:00'` →
// `2026-01-01T00:00:00`). Used so the emitter's StringValue.token
// re-quotes cleanly instead of producing triple-quoted text.
func stripSurroundingQuotes(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

func numericPrimitiveAsValue(c gen.INumericPrimitiveContext) aql.Value {
	// Handle the optional unary minus by collecting the inner numeric.
	sign := 1
	if c.SYM_MINUS() != nil {
		sign = -1
		if inner := c.NumericPrimitive(); inner != nil {
			c = inner
		}
	}
	if t := c.INTEGER(); t != nil {
		if n, err := strconv.ParseInt(t.GetText(), 10, 64); err == nil {
			return aql.IntValue{N: int64(sign) * n}
		}
	}
	if t := c.SCI_INTEGER(); t != nil {
		if f, err := strconv.ParseFloat(t.GetText(), 64); err == nil {
			return aql.RealValue{F: float64(sign) * f}
		}
	}
	if t := c.REAL(); t != nil {
		if f, err := strconv.ParseFloat(t.GetText(), 64); err == nil {
			return aql.RealValue{F: float64(sign) * f}
		}
	}
	if t := c.SCI_REAL(); t != nil {
		if f, err := strconv.ParseFloat(t.GetText(), 64); err == nil {
			return aql.RealValue{F: float64(sign) * f}
		}
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

// matchesOperandValues collects the value-list members of a MATCHES
// operand. Returns ok=false when the operand uses the terminology-
// function or URI alternatives (grammar `matchesOperand: '{'
// valueListItem (',' valueListItem)* '}' | terminologyFunction |
// '{' URI '}'`), recording a catalogue gap so the predicate doesn't
// silently disappear.
func (ex *astExtractor) matchesOperandValues(c gen.IMatchesOperandContext) ([]aql.Value, bool) {
	items := c.AllValueListItem()
	if len(items) == 0 {
		// terminologyFunction or URI alternative — outside v1.
		ex.incomplete("MATCHES terminology-function / URI operand is outside the v1 catalogue")
		return nil, false
	}
	out := make([]aql.Value, 0, len(items))
	for _, it := range items {
		if p := it.Primitive(); p != nil {
			if v := primitiveAsValue(p); v != nil {
				out = append(out, v)
			}
			continue
		}
		if t := it.PARAMETER(); t != nil {
			out = append(out, aql.ParamValue{Name: strings.TrimPrefix(t.GetText(), "$")})
		}
	}
	return out, true
}
