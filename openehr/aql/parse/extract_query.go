package parse

// extract_query.go: SDK-GAP-17 Tier 2 — translate a validated ANTLR
// parse tree (gen.ISelectQueryContext) into the readable, generated-
// type-free [Query] AST (REQ-113). Pure recursive descent — no
// listeners, no shared mutable state between calls. The validated-tree
// input is assumed syntactically well-formed (Parse rejects malformed
// queries before reaching here); the extractor is forgiving about
// optional clauses (missing WHERE / ORDER BY / LIMIT).
//
// Reuses the AST helpers already in ast.go: posOf, trimBrackets, and
// the IdentifiedPath / ClassExpr / PathSegment types.

import (
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse/gen"
)

// extractQuery turns a validated tree into a populated [Query]. Returns
// nil on a nil tree (no panic on a degenerate input). The clauses are
// populated independently — an absent WHERE/ORDER BY/LIMIT leaves the
// corresponding fields as their zero values (nil for the *int LIMIT
// and OFFSET; nil interface for Where; empty slice for OrderBy).
func extractQuery(tree gen.ISelectQueryContext) *Query {
	if tree == nil {
		return nil
	}
	q := &Query{}
	if sc := tree.SelectClause(); sc != nil {
		q.Select = extractSelectClause(sc)
	}
	if fc := tree.FromClause(); fc != nil {
		q.From = extractFromClause(fc)
	}
	if wc := tree.WhereClause(); wc != nil {
		q.Where = extractWhereClause(wc)
	}
	if oc := tree.OrderByClause(); oc != nil {
		q.OrderBy = extractOrderBy(oc)
	}
	if lc := tree.LimitClause(); lc != nil {
		q.Limit, q.Offset = extractLimit(lc)
	}
	return q
}

// --- SELECT ----------------------------------------------------------

func extractSelectClause(c gen.ISelectClauseContext) SelectClause {
	out := SelectClause{Distinct: c.DISTINCT() != nil}
	for _, item := range c.AllSelectExpr() {
		if item.SYM_ASTERISK() != nil {
			out.Star = true
			continue
		}
		out.Items = append(out.Items, extractSelectItem(item))
	}
	return out
}

func extractSelectItem(c gen.ISelectExprContext) SelectItem {
	item := SelectItem{}
	if id := c.IDENTIFIER(); id != nil {
		item.Alias = id.GetText()
	}
	if col := c.ColumnExpr(); col != nil {
		item.Expr = extractColumnExpr(col)
	}
	return item
}

func extractColumnExpr(c gen.IColumnExprContext) SelectExpr {
	if ip := c.IdentifiedPath(); ip != nil {
		return PathExpr{IdentifiedPath: extractIdentifiedPath(ip, ClauseSelect)}
	}
	if afc := c.AggregateFunctionCall(); afc != nil {
		return extractAggregateFunctionCall(afc)
	}
	if fc := c.FunctionCall(); fc != nil {
		return extractFunctionCall(fc)
	}
	if p := c.Primitive(); p != nil {
		// A literal in SELECT position is rare but legal. Box it as a
		// FunctionCall-shaped projection so the AST stays uniform —
		// the value is captured in Args[0] as the inner PathExpr is
		// not applicable; consumers reading a Primitive in SELECT
		// can match on FunctionCall.Name == "" and the value form.
		// First-cycle compromise: emit a ValueExpr-wrapped path is
		// not in the v1 catalogue; defer Primitive-in-SELECT to a
		// follow-up. The lint pass already accepts/rejects this.
		_ = p
	}
	return nil
}

func extractAggregateFunctionCall(c gen.IAggregateFunctionCallContext) FunctionCall {
	out := FunctionCall{Name: aggregateName(c)}
	// Aggregate functions accept either `*` or a single identifiedPath
	// (e.g. COUNT(o), COUNT(*)). For the first cycle we capture the
	// path operand when present; the star form leaves Args empty.
	if ip := c.IdentifiedPath(); ip != nil {
		out.Args = []SelectExpr{PathExpr{IdentifiedPath: extractIdentifiedPath(ip, ClauseSelect)}}
	}
	return out
}

func extractFunctionCall(c gen.IFunctionCallContext) FunctionCall {
	out := FunctionCall{Name: functionName(c)}
	for _, t := range c.AllTerminal() {
		if expr := terminalAsSelectExpr(t); expr != nil {
			out.Args = append(out.Args, expr)
		}
	}
	return out
}

// terminalAsSelectExpr lifts a Terminal context into a SelectExpr —
// either a PathExpr (when the terminal carries an identifiedPath) or
// nil for shapes outside the SELECT vocabulary (parameters, primitives,
// nested function calls are not yet a SelectExpr).
func terminalAsSelectExpr(t gen.ITerminalContext) SelectExpr {
	if ip := t.IdentifiedPath(); ip != nil {
		return PathExpr{IdentifiedPath: extractIdentifiedPath(ip, ClauseSelect)}
	}
	if fc := t.FunctionCall(); fc != nil {
		return extractFunctionCall(fc)
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

func extractFromClause(c gen.IFromClauseContext) FromClause {
	out := FromClause{}
	fe := c.FromExpr()
	if fe == nil {
		return out
	}
	ce := fe.ContainsExpr()
	if ce == nil {
		return out
	}
	root := extractContainment(ce)
	if root == nil {
		return out
	}
	// FromClause.Root captures the class at the FROM root; Contains
	// captures the chain BELOW it. The simple chain (single child)
	// unwraps to a direct Containment so a consumer reads
	// `From.Contains.Class.RMType` directly; multi-child junctions
	// stay as a synthetic Containment carrying the operands.
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
// It mirrors the recursive grammar shape:
//
//   - bare class: Class set, Children empty.
//   - NOT child:  Negated=true; Class is from the inner ContainsExpr,
//     plus its children.
//   - parenthesised: pass through to the inner ContainsExpr.
//   - boolean junction (A AND/OR B): a synthetic Containment with
//     ChildJoin and Children=[A, B] (flattened on the same operator).
//   - CONTAINS chain: a single Containment with the child sub-tree.
func extractContainment(c gen.IContainsExprContext) *Containment {
	if c == nil {
		return nil
	}
	if c.NOT() != nil {
		// Skip past the NOT; the negation applies to the child.
		kids := c.AllContainsExpr()
		if len(kids) == 0 {
			return &Containment{Negated: true}
		}
		inner := extractContainment(kids[0])
		if inner == nil {
			return &Containment{Negated: true}
		}
		inner.Negated = !inner.Negated
		return inner
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
			child := extractContainment(op)
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
			return extractContainment(kids[0])
		}
	}
	// Bare class operand at this level.
	node := &Containment{}
	if op := c.ClassExprOperand(); op != nil {
		node.Class = extractClassExprOperand(op)
	}
	// CONTAINS chain: the second ContainsExpr is the subtree.
	if c.CONTAINS() != nil {
		kids := c.AllContainsExpr()
		if len(kids) > 0 {
			child := extractContainment(kids[0])
			if child != nil {
				node.Children = []Containment{*child}
			}
		}
	}
	return node
}

func extractClassExprOperand(c gen.IClassExprOperandContext) ClassExpr {
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
			if ap := pp.ArchetypePredicate(); ap != nil {
				if hrid := ap.ARCHETYPE_HRID(); hrid != nil {
					ce.Archetype = hrid.GetText()
				} else if ap.PARAMETER() != nil {
					ce.ParamArchetype = true
				}
			}
		}
		return ce
	case *gen.VersionClassExprContext:
		ce := ClassExpr{RMType: "VERSION", Version: true, Pos: posOf(v.GetStart())}
		if vv := v.GetVariable(); vv != nil {
			ce.Alias = vv.GetText()
		}
		if v.VersionPredicate() != nil {
			ce.HasPredicate = true
		}
		return ce
	}
	return ClassExpr{}
}

// --- WHERE -----------------------------------------------------------

func extractWhereClause(c gen.IWhereClauseContext) aql.WhereExpr {
	if c == nil {
		return nil
	}
	for _, child := range c.GetChildren() {
		if we, ok := child.(gen.IWhereExprContext); ok {
			return extractWhereExpr(we)
		}
	}
	return nil
}

func extractWhereExpr(c gen.IWhereExprContext) aql.WhereExpr {
	if c == nil {
		return nil
	}
	if c.NOT() != nil {
		// NOT applies to the next WhereExpr operand.
		ops := c.AllWhereExpr()
		if len(ops) == 0 {
			return nil
		}
		return aql.Not(extractWhereExpr(ops[0]))
	}
	if c.AND() != nil || c.OR() != nil {
		ops := c.AllWhereExpr()
		terms := make([]aql.WhereExpr, 0, len(ops))
		for _, op := range ops {
			if t := extractWhereExpr(op); t != nil {
				terms = append(terms, t)
			}
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
			return extractWhereExpr(ops[0])
		}
	}
	if ie := c.IdentifiedExpr(); ie != nil {
		return extractIdentifiedExpr(ie)
	}
	return nil
}

func extractIdentifiedExpr(c gen.IIdentifiedExprContext) aql.WhereExpr {
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
			return extractIdentifiedExpr(inner)
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
		}
		if c.MATCHES() != nil {
			if op := c.MatchesOperand(); op != nil {
				if vs := matchesOperandValues(op); len(vs) > 0 {
					return aql.Matches(path, vs...)
				}
			}
		}
		// path <op> terminal — the comparison form.
		if cmp := c.COMPARISON_OPERATOR(); cmp != nil {
			opStr := cmp.GetText()
			if t := c.Terminal(); t != nil {
				if v := terminalAsValue(t); v != nil {
					return aql.Comparison{Path: path, Op: aql.Operator(opStr), Val: v}
				}
			}
		}
	}
	return nil
}

// --- ORDER BY + LIMIT ------------------------------------------------

func extractOrderBy(c gen.IOrderByClauseContext) []OrderTerm {
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

func extractLimit(c gen.ILimitClauseContext) (limit, offset *int) {
	limit = limitValueAsInt(c.GetLimit())
	offset = limitValueAsInt(c.GetOffset())
	return
}

func limitValueAsInt(v gen.ILimitValueContext) *int {
	if v == nil {
		return nil
	}
	if t := v.INTEGER(); t != nil {
		if n, err := strconv.Atoi(t.GetText()); err == nil {
			return &n
		}
	}
	// PARAMETER form ($n) — we don't surface a numeric value; nil
	// signals "present but parameter-bound", which the first-cycle
	// consumer can interpret as "look at the query parameters".
	return nil
}

// --- shared helpers --------------------------------------------------

// extractIdentifiedPath mirrors the ast.go listener's path extraction
// shape — used by the structured extractor to produce identical
// IdentifiedPath values, so consumers can compare paths from
// Document.Paths and Query SELECT/WHERE/ORDER BY by equality.
func extractIdentifiedPath(c gen.IIdentifiedPathContext, clause Clause) IdentifiedPath {
	ip := IdentifiedPath{
		Raw:    c.GetText(),
		Pos:    posOf(c.GetStart()),
		Clause: clause,
	}
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

func terminalAsValue(c gen.ITerminalContext) aql.Value {
	if c == nil {
		return nil
	}
	if t := c.PARAMETER(); t != nil {
		name := strings.TrimPrefix(t.GetText(), "$")
		return aql.ParamValue{Name: name}
	}
	if p := c.Primitive(); p != nil {
		return primitiveAsValue(p)
	}
	return nil
}

// primitiveAsValue lifts a Primitive to an [aql.Value] — STRING /
// numeric / BOOLEAN. Date/time/datetime/null are out of the first-
// cycle vocabulary; they surface as StringValue (preserving the
// source text) so the round-trip property still holds.
func primitiveAsValue(c gen.IPrimitiveContext) aql.Value {
	if t := c.STRING(); t != nil {
		raw := t.GetText()
		// Strip surrounding quotes and undo embedded-quote doubling
		// so StringValue.S carries the canonical string content; the
		// emitter re-quotes uniformly on the way back.
		if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
			inner := raw[1 : len(raw)-1]
			return aql.StringValue{S: strings.ReplaceAll(inner, "''", "'")}
		}
		return aql.StringValue{S: raw}
	}
	if t := c.BOOLEAN(); t != nil {
		return aql.BoolValue{B: strings.EqualFold(t.GetText(), "true")}
	}
	if np := c.NumericPrimitive(); np != nil {
		return numericPrimitiveAsValue(np)
	}
	if t := c.DATE(); t != nil {
		return aql.StringValue{S: t.GetText()}
	}
	if t := c.TIME(); t != nil {
		return aql.StringValue{S: t.GetText()}
	}
	if t := c.DATETIME(); t != nil {
		return aql.StringValue{S: t.GetText()}
	}
	if c.NULL() != nil {
		return aql.StringValue{S: "NULL"}
	}
	return nil
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
		raw := t.GetText()
		if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
			inner := raw[1 : len(raw)-1]
			return aql.StringValue{S: strings.ReplaceAll(inner, "''", "'")}
		}
		return aql.StringValue{S: raw}
	}
	if t := c.PARAMETER(); t != nil {
		return aql.ParamValue{Name: strings.TrimPrefix(t.GetText(), "$")}
	}
	return nil
}

func matchesOperandValues(c gen.IMatchesOperandContext) []aql.Value {
	items := c.AllValueListItem()
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
	return out
}
