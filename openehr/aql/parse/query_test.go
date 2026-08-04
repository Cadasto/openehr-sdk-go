package parse_test

// query_test.go pins REQ-113 Tier 2 end-to-end on simple
// AQL inputs: ParseQuery returns a populated *Query whose SELECT /
// FROM / WHERE / ORDER BY / LIMIT shapes match the source. The
// round-trip property (parsed → emit → byte-compare) is Phase 3e's
// concern; this file pins the EXTRACTION shape.

import (
	"errors"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// TestParseQueryReturnsStructuredAST asserts the canonical happy path.
func TestParseQueryReturnsStructuredAST(t *testing.T) {
	q, err := parse.ParseQuery("SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = $id ORDER BY e/time_created DESC")
	if err != nil {
		t.Fatal(err)
	}
	if q == nil {
		t.Fatal("ParseQuery returned nil for a valid query")
	}

	// SELECT
	if q.Select.Star {
		t.Errorf("Select.Star = true; want false")
	}
	if len(q.Select.Items) != 1 {
		t.Fatalf("Select.Items len = %d, want 1", len(q.Select.Items))
	}
	pe, ok := q.Select.Items[0].Expr.(parse.PathExpr)
	if !ok {
		t.Fatalf("Select.Items[0].Expr type = %T, want parse.PathExpr", q.Select.Items[0].Expr)
	}
	if pe.Alias != "e" {
		t.Errorf("Select path Alias = %q, want e", pe.Alias)
	}

	// FROM
	if q.From.Root.RMType != "EHR" {
		t.Errorf("From.Root.RMType = %q, want EHR", q.From.Root.RMType)
	}
	if q.From.Root.Alias != "e" {
		t.Errorf("From.Root.Alias = %q, want e", q.From.Root.Alias)
	}
	if q.From.Contains != nil {
		t.Errorf("From.Contains unexpectedly non-nil; want nil for FROM EHR e (no CONTAINS): %+v", q.From.Contains)
	}

	// WHERE
	cmp, ok := q.Where.(aql.Comparison)
	if !ok {
		t.Fatalf("Where type = %T, want aql.Comparison", q.Where)
	}
	if cmp.Op != aql.OpEq {
		t.Errorf("Where.Op = %q, want OpEq", cmp.Op)
	}
	pv, ok := cmp.Val.(aql.ParamValue)
	if !ok {
		t.Fatalf("Where.Val type = %T, want aql.ParamValue", cmp.Val)
	}
	if pv.Name != "id" {
		t.Errorf("Where.Val.Name = %q, want id", pv.Name)
	}

	// ORDER BY
	if len(q.OrderBy) != 1 {
		t.Fatalf("OrderBy len = %d, want 1", len(q.OrderBy))
	}
	if q.OrderBy[0].Dir != parse.OrderDesc {
		t.Errorf("OrderBy[0].Dir = %v, want OrderDesc", q.OrderBy[0].Dir)
	}

	// LIMIT/OFFSET — absent
	if q.Limit != nil {
		t.Errorf("Limit = %v, want nil", q.Limit)
	}
	if q.Offset != nil {
		t.Errorf("Offset = %v, want nil", q.Offset)
	}
}

// TestParseQuerySyntaxError mirrors Parse's error contract: an invalid
// query returns a *SyntaxError wrapping aql.ErrSyntax, with nil AST.
func TestParseQuerySyntaxError(t *testing.T) {
	q, err := parse.ParseQuery("SELEC e FROM EHR e")
	if err == nil {
		t.Fatal("ParseQuery: expected syntax error, got nil")
	}
	if !errors.Is(err, aql.ErrSyntax) {
		t.Errorf("error does not wrap aql.ErrSyntax: %v", err)
	}
	if q != nil {
		t.Errorf("ParseQuery returned non-nil *Query on syntax error: %+v", q)
	}
}

// TestDocumentQueryCachesExtraction asserts that repeated calls return
// the same pointer (the extractor runs once per document).
func TestDocumentQueryCachesExtraction(t *testing.T) {
	doc, err := parse.Parse("SELECT c FROM EHR e CONTAINS COMPOSITION c")
	if err != nil {
		t.Fatal(err)
	}
	q1 := doc.Query()
	q2 := doc.Query()
	if q1 == nil || q2 == nil {
		t.Fatal("Document.Query returned nil")
	}
	if q1 != q2 {
		t.Errorf("Document.Query returned different pointers on repeated calls (%p vs %p)", q1, q2)
	}
}

// TestDocumentQueryConcurrent asserts that concurrent callers of
// Document.Query() see a single stable *Query pointer — the sync.Once
// guard around lazy extraction. Run under -race for write-write
// detection; the assertion below also catches a non-Once
// implementation that double-builds.
func TestDocumentQueryConcurrent(t *testing.T) {
	doc, err := parse.Parse("SELECT c FROM EHR e CONTAINS COMPOSITION c WHERE c/uid/value = $id")
	if err != nil {
		t.Fatal(err)
	}
	const n = 32
	results := make([]*parse.Query, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			results[i] = doc.Query()
		})
	}
	wg.Wait()
	first := results[0]
	if first == nil {
		t.Fatal("concurrent Document.Query: first result is nil")
	}
	for i, r := range results {
		if r != first {
			t.Errorf("concurrent Document.Query: result %d (%p) != first (%p)", i, r, first)
		}
	}
}

// TestParseQueryContainmentChain pins the CONTAINS extraction shape
// — FROM root + a one-level CONTAINS subtree carrying its own class.
func TestParseQueryContainmentChain(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c FROM EHR e CONTAINS COMPOSITION c")
	if err != nil {
		t.Fatal(err)
	}
	if q.From.Root.RMType != "EHR" {
		t.Errorf("From.Root.RMType = %q, want EHR", q.From.Root.RMType)
	}
	if q.From.Contains == nil {
		t.Fatal("From.Contains unexpectedly nil")
	}
	if q.From.Contains.Class.RMType != "COMPOSITION" {
		t.Errorf("Contains.Class.RMType = %q, want COMPOSITION", q.From.Contains.Class.RMType)
	}
	if q.From.Contains.Class.Alias != "c" {
		t.Errorf("Contains.Class.Alias = %q, want c", q.From.Contains.Class.Alias)
	}
}

// TestParseQueryLimitOffset pins LIMIT/OFFSET extraction as
// IntLimit concrete shapes.
func TestParseQueryLimitOffset(t *testing.T) {
	q, err := parse.ParseQuery("SELECT e FROM EHR e LIMIT 50 OFFSET 100")
	if err != nil {
		t.Fatal(err)
	}
	lim, ok := q.Limit.(parse.IntLimit)
	if !ok || lim.N != 50 {
		t.Errorf("Limit = %v, want IntLimit{50}", q.Limit)
	}
	off, ok := q.Offset.(parse.IntLimit)
	if !ok || off.N != 100 {
		t.Errorf("Offset = %v, want IntLimit{100}", q.Offset)
	}
}

// TestParseQueryStarSelect pins the bare SELECT * extraction (Items
// empty, Star = true).
func TestParseQueryStarSelect(t *testing.T) {
	q, err := parse.ParseQuery("SELECT * FROM EHR e")
	if err != nil {
		t.Fatal(err)
	}
	if !q.Select.Star {
		t.Errorf("Select.Star = false; want true for SELECT *")
	}
	if len(q.Select.Items) != 0 {
		t.Errorf("Select.Items len = %d, want 0 for SELECT *", len(q.Select.Items))
	}
}

// TestParseQuerySelectPrimitiveLiteral pins REQ-117 catalogue shape 1: a
// primitive literal projected from SELECT is a [parse.LiteralExpr] carrying
// the shared [aql.Value] vocabulary, in source order alongside path items.
// PROBE-087
func TestParseQuerySelectPrimitiveLiteral(t *testing.T) {
	q, err := parse.ParseQuery("SELECT 1, e/ehr_id/value FROM EHR e")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if len(q.Select.Items) != 2 {
		t.Fatalf("Select.Items len = %d, want 2", len(q.Select.Items))
	}
	lit, ok := q.Select.Items[0].Expr.(parse.LiteralExpr)
	if !ok {
		t.Fatalf("Select.Items[0].Expr = %T, want parse.LiteralExpr", q.Select.Items[0].Expr)
	}
	iv, ok := lit.Value.(aql.IntValue)
	if !ok || iv.N != 1 {
		t.Errorf("LiteralExpr.Value = %#v, want aql.IntValue{1}", lit.Value)
	}
	if _, ok := q.Select.Items[1].Expr.(parse.PathExpr); !ok {
		t.Errorf("Select.Items[1].Expr = %T, want parse.PathExpr", q.Select.Items[1].Expr)
	}
	if out, err := q.Emit(); err != nil || out != "SELECT 1, e/ehr_id/value FROM EHR e" {
		t.Errorf("Emit = %q, %v; want the canonical input", out, err)
	}
}

// TestParseQuerySelectLiteralAlias covers an aliased string literal
// projection (`SELECT 'x' AS label`) — the literal carries the shared
// StringValue and the AS alias survives on the item (REQ-117).
// PROBE-087
func TestParseQuerySelectLiteralAlias(t *testing.T) {
	q, err := parse.ParseQuery("SELECT 'urgent' AS label FROM EHR e")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	item := q.Select.Items[0]
	if item.Alias != "label" {
		t.Errorf("SelectItem.Alias = %q, want label", item.Alias)
	}
	lit, ok := item.Expr.(parse.LiteralExpr)
	if !ok {
		t.Fatalf("Expr = %T, want parse.LiteralExpr", item.Expr)
	}
	if sv, ok := lit.Value.(aql.StringValue); !ok || sv.S != "urgent" {
		t.Errorf("LiteralExpr.Value = %#v, want aql.StringValue{urgent}", lit.Value)
	}
}

// TestParseQuerySelectKeywordLiteral pins the SELECT-side lift of a bare
// boolean / null keyword projection. The SDK lexer lexes `true` / `false` as
// IDENTIFIER (the IDENTIFIER rule precedes BOOLEAN in AqlLexer.g4), so
// `SELECT true` reaches the extractor as an IDENTIFIER-only identifiedPath —
// the same shape the WHERE side already lifts to a typed literal. The SELECT
// column position must agree: a bare keyword is a [parse.LiteralExpr], never a
// [parse.PathExpr] rooted at a pseudo-alias.
// PROBE-087
func TestParseQuerySelectKeywordLiteral(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		want     aql.Value
		emit     string
	}{
		{name: "true", in: "SELECT true FROM EHR e", want: aql.BoolValue{B: true}, emit: "SELECT true FROM EHR e"},
		{name: "false", in: "SELECT false FROM EHR e", want: aql.BoolValue{B: false}, emit: "SELECT false FROM EHR e"},
		// Keyword casing normalises to the canonical literal, matching the
		// WHERE-side treatment of the same keyword.
		{name: "uppercase", in: "SELECT TRUE FROM EHR e", want: aql.BoolValue{B: true}, emit: "SELECT true FROM EHR e"},
		// `null` reaches the extractor as the NULL token (a primitive), not an
		// IDENTIFIER — pinned here so both routes to a keyword literal agree.
		{name: "null", in: "SELECT null FROM EHR e", want: aql.NullValue{}, emit: "SELECT NULL FROM EHR e"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parse.ParseQuery(tc.in)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", tc.in, err)
			}
			if len(q.Select.Items) != 1 {
				t.Fatalf("Select.Items len = %d, want 1", len(q.Select.Items))
			}
			lit, ok := q.Select.Items[0].Expr.(parse.LiteralExpr)
			if !ok {
				t.Fatalf("Select.Items[0].Expr = %#v, want parse.LiteralExpr", q.Select.Items[0].Expr)
			}
			if lit.Value != tc.want {
				t.Errorf("LiteralExpr.Value = %#v, want %#v", lit.Value, tc.want)
			}
			if out, err := q.Emit(); err != nil || out != tc.emit {
				t.Errorf("Emit = %q, %v; want %q", out, err, tc.emit)
			}
		})
	}
}

// TestParseQuerySelectKeywordWithPathTailStaysPath is the negative of
// [TestParseQuerySelectKeywordLiteral]: only a BARE keyword is a literal. A
// keyword carrying a path tail (or a path predicate) is a real path whatever it
// is rooted at, in the SELECT column position exactly as in WHERE.
// PROBE-087
func TestParseQuerySelectKeywordWithPathTailStaysPath(t *testing.T) {
	for _, in := range []string{
		"SELECT true/nested FROM EHR e",
		"SELECT false[at0001] FROM EHR e",
	} {
		q, err := parse.ParseQuery(in)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", in, err)
		}
		if _, ok := q.Select.Items[0].Expr.(parse.PathExpr); !ok {
			t.Errorf("%q: Select.Items[0].Expr = %#v, want parse.PathExpr", in, q.Select.Items[0].Expr)
		}
	}
}

// TestParseQuerySelectStarWithColumns pins REQ-117 catalogue shape 2: a
// star mixed with column projections is an ORDER-PRESERVING item list
// carrying a [parse.StarExpr] at the star's position, while the
// query-level Star flag keeps its pre-REQ-117 meaning.
// PROBE-087
func TestParseQuerySelectStarWithColumns(t *testing.T) {
	const src = "SELECT *, c/uid/value FROM EHR e CONTAINS COMPOSITION c"
	q, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if !q.Select.Star {
		t.Errorf("Select.Star = false; want true (compatibility flag stays set)")
	}
	if len(q.Select.Items) != 2 {
		t.Fatalf("Select.Items len = %d, want 2 (star + column)", len(q.Select.Items))
	}
	if _, ok := q.Select.Items[0].Expr.(parse.StarExpr); !ok {
		t.Errorf("Select.Items[0].Expr = %T, want parse.StarExpr", q.Select.Items[0].Expr)
	}
	pe, ok := q.Select.Items[1].Expr.(parse.PathExpr)
	if !ok {
		t.Fatalf("Select.Items[1].Expr = %T, want parse.PathExpr", q.Select.Items[1].Expr)
	}
	if pe.Raw != "c/uid/value" {
		t.Errorf("Select.Items[1] path Raw = %q, want c/uid/value", pe.Raw)
	}
	out, err := q.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if out != src {
		t.Errorf("Emit\n  in:  %s\n  out: %s", src, out)
	}
}

// TestParseQuerySelectColumnAfterStarOrder pins that the star's POSITION
// is preserved — `SELECT col, *` keeps the column first (REQ-117).
// PROBE-087
func TestParseQuerySelectColumnAfterStarOrder(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c/uid/value, * FROM EHR e CONTAINS COMPOSITION c")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if len(q.Select.Items) != 2 {
		t.Fatalf("Select.Items len = %d, want 2", len(q.Select.Items))
	}
	if _, ok := q.Select.Items[0].Expr.(parse.PathExpr); !ok {
		t.Errorf("Select.Items[0].Expr = %T, want parse.PathExpr", q.Select.Items[0].Expr)
	}
	if _, ok := q.Select.Items[1].Expr.(parse.StarExpr); !ok {
		t.Errorf("Select.Items[1].Expr = %T, want parse.StarExpr", q.Select.Items[1].Expr)
	}
}

// TestParseQuerySelectFunctionArguments pins REQ-117 catalogue shape 7 on
// the SELECT side: primitive, parameter, and nested-function arguments in a
// function call all model structurally, in argument order.
// PROBE-087
func TestParseQuerySelectFunctionArguments(t *testing.T) {
	q, err := parse.ParseQuery("SELECT CONCAT('hello', $p, LENGTH(p/name)) FROM EHR e CONTAINS PERSON p")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	fc, ok := q.Select.Items[0].Expr.(parse.FunctionCall)
	if !ok {
		t.Fatalf("Select.Items[0].Expr = %T, want parse.FunctionCall", q.Select.Items[0].Expr)
	}
	if fc.Name != "CONCAT" {
		t.Errorf("FunctionCall.Name = %q, want CONCAT", fc.Name)
	}
	if len(fc.Args) != 3 {
		t.Fatalf("FunctionCall.Args len = %d, want 3", len(fc.Args))
	}
	lit, ok := fc.Args[0].(parse.LiteralExpr)
	if !ok {
		t.Fatalf("Args[0] = %T, want parse.LiteralExpr", fc.Args[0])
	}
	if sv, ok := lit.Value.(aql.StringValue); !ok || sv.S != "hello" {
		t.Errorf("Args[0].Value = %#v, want aql.StringValue{hello}", lit.Value)
	}
	param, ok := fc.Args[1].(parse.LiteralExpr)
	if !ok {
		t.Fatalf("Args[1] = %T, want parse.LiteralExpr", fc.Args[1])
	}
	if pv, ok := param.Value.(aql.ParamValue); !ok || pv.Name != "p" {
		t.Errorf("Args[1].Value = %#v, want aql.ParamValue{p}", param.Value)
	}
	nested, ok := fc.Args[2].(parse.FunctionCall)
	if !ok {
		t.Fatalf("Args[2] = %T, want parse.FunctionCall", fc.Args[2])
	}
	if nested.Name != "LENGTH" || len(nested.Args) != 1 {
		t.Errorf("nested call = %+v, want LENGTH with one argument", nested)
	}
	if _, ok := nested.Args[0].(parse.PathExpr); !ok {
		t.Errorf("nested Args[0] = %T, want parse.PathExpr", nested.Args[0])
	}
}

// TestParseQuerySelectTerminologyFunction pins the grammar's
// `functionCall : terminologyFunction` alternative in a SELECT column: the
// TERMINOLOGY call models as a named FunctionCall with its three string
// arguments, upper-cased canonically (REQ-117).
// PROBE-087
func TestParseQuerySelectTerminologyFunction(t *testing.T) {
	q, err := parse.ParseQuery("SELECT terminology('SNOMED-CT','near','12345') FROM EHR e")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	fc, ok := q.Select.Items[0].Expr.(parse.FunctionCall)
	if !ok {
		t.Fatalf("Select.Items[0].Expr = %T, want parse.FunctionCall", q.Select.Items[0].Expr)
	}
	if fc.Name != "TERMINOLOGY" {
		t.Errorf("FunctionCall.Name = %q, want TERMINOLOGY", fc.Name)
	}
	if len(fc.Args) != 3 {
		t.Fatalf("FunctionCall.Args len = %d, want 3", len(fc.Args))
	}
	out, err := q.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if want := "SELECT TERMINOLOGY('SNOMED-CT', 'near', '12345') FROM EHR e"; out != want {
		t.Errorf("Emit = %q, want %q", out, want)
	}
}

// TestParseQueryAggregateCountStar pins extraction shape for COUNT(*):
// FunctionCall with Star=true and empty Args.
func TestParseQueryAggregateCountStar(t *testing.T) {
	q, err := parse.ParseQuery("SELECT COUNT(*) FROM EHR e")
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Select.Items) != 1 {
		t.Fatalf("Select.Items len = %d, want 1", len(q.Select.Items))
	}
	fc, ok := q.Select.Items[0].Expr.(parse.FunctionCall)
	if !ok {
		t.Fatalf("Select.Items[0].Expr = %T, want parse.FunctionCall", q.Select.Items[0].Expr)
	}
	if fc.Name != "COUNT" {
		t.Errorf("FunctionCall.Name = %q, want COUNT", fc.Name)
	}
	if !fc.Star {
		t.Errorf("FunctionCall.Star = false; want true for COUNT(*)")
	}
	if len(fc.Args) != 0 {
		t.Errorf("FunctionCall.Args len = %d, want 0 for COUNT(*)", len(fc.Args))
	}
}

// TestParseQueryAggregateCountDistinct pins COUNT(DISTINCT path):
// FunctionCall with Distinct=true and the path operand in Args.
func TestParseQueryAggregateCountDistinct(t *testing.T) {
	q, err := parse.ParseQuery("SELECT COUNT(DISTINCT o/data) FROM EHR e CONTAINS OBSERVATION o")
	if err != nil {
		t.Fatal(err)
	}
	fc := q.Select.Items[0].Expr.(parse.FunctionCall)
	if !fc.Distinct {
		t.Errorf("FunctionCall.Distinct = false; want true for COUNT(DISTINCT ...)")
	}
	if len(fc.Args) != 1 {
		t.Fatalf("FunctionCall.Args len = %d, want 1", len(fc.Args))
	}
}

// TestParseQueryNotContains pins the NOT CONTAINS extraction shape:
// the chained child carries Negated=true so the parent emits
// `NOT CONTAINS` for the connector.
func TestParseQueryNotContains(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c FROM EHR e CONTAINS COMPOSITION c NOT CONTAINS SECTION s")
	if err != nil {
		t.Fatal(err)
	}
	if q.From.Contains == nil {
		t.Fatal("From.Contains unexpectedly nil")
	}
	if q.From.Contains.Class.RMType != "COMPOSITION" {
		t.Errorf("From.Contains.Class.RMType = %q, want COMPOSITION", q.From.Contains.Class.RMType)
	}
	if len(q.From.Contains.Children) != 1 {
		t.Fatalf("From.Contains.Children len = %d, want 1", len(q.From.Contains.Children))
	}
	child := q.From.Contains.Children[0]
	if !child.Negated {
		t.Errorf("Children[0].Negated = false; want true for `NOT CONTAINS`")
	}
	if child.Class.RMType != "SECTION" {
		t.Errorf("Children[0].Class.RMType = %q, want SECTION", child.Class.RMType)
	}
}

// TestParseQueryFromRootJunction pins REQ-117 catalogue shape 6: a boolean
// junction at the FROM root is carried by [parse.FromClause.Junction] — the
// same containment tree the nested side uses — with Root left zero because
// the clause has no single root class.
// PROBE-087
func TestParseQueryFromRootJunction(t *testing.T) {
	const src = "SELECT c1 FROM COMPOSITION c1 OR COMPOSITION c2"
	q, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if q.From.Junction == nil {
		t.Fatalf("From.Junction is nil; want the containment junction (Root=%+v)", q.From.Root)
	}
	if q.From.Root.RMType != "" {
		t.Errorf("From.Root.RMType = %q, want empty — a junction has no single root", q.From.Root.RMType)
	}
	if q.From.Contains != nil {
		t.Errorf("From.Contains = %+v, want nil for a junction root", q.From.Contains)
	}
	j := q.From.Junction
	if j.ChildJoin != parse.ContainsOr {
		t.Errorf("Junction.ChildJoin = %v, want ContainsOr", j.ChildJoin)
	}
	if len(j.Children) != 2 {
		t.Fatalf("Junction.Children len = %d, want 2", len(j.Children))
	}
	if j.Children[0].Class.RMType != "COMPOSITION" || j.Children[0].Class.Alias != "c1" {
		t.Errorf("Children[0].Class = %+v, want COMPOSITION c1", j.Children[0].Class)
	}
	if j.Children[1].Class.Alias != "c2" {
		t.Errorf("Children[1].Class = %+v, want COMPOSITION c2", j.Children[1].Class)
	}
	if out, err := q.Emit(); err != nil || out != src {
		t.Errorf("Emit = %q, %v; want the canonical input (no redundant parentheses)", out, err)
	}
}

// TestParseQueryFromRootJunctionFlattened pins that a same-operator FROM
// junction chain is ONE node with three operands (the parser's left-nesting
// is flattened), and that a nested junction of the OTHER operator keeps its
// grouping parentheses on emit (REQ-117).
// PROBE-087
func TestParseQueryFromRootJunctionFlattened(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c1 FROM COMPOSITION c1 OR COMPOSITION c2 OR EHR e")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if q.From.Junction == nil {
		t.Fatal("From.Junction is nil")
	}
	if got := len(q.From.Junction.Children); got != 3 {
		t.Errorf("Junction.Children len = %d, want 3 (flattened OR chain)", got)
	}

	const grouped = "SELECT c1 FROM (COMPOSITION c1 OR COMPOSITION c2) AND EHR e"
	q2, err := parse.ParseQuery(grouped)
	if err != nil {
		t.Fatalf("ParseQuery(grouped): %v", err)
	}
	if q2.From.Junction == nil {
		t.Fatal("From.Junction is nil for the grouped junction")
	}
	if q2.From.Junction.ChildJoin != parse.ContainsAnd {
		t.Errorf("outer ChildJoin = %v, want ContainsAnd", q2.From.Junction.ChildJoin)
	}
	inner := q2.From.Junction.Children[0]
	if inner.Class.RMType != "" || inner.ChildJoin != parse.ContainsOr || len(inner.Children) != 2 {
		t.Errorf("Children[0] = %+v, want the nested OR junction", inner)
	}
	if out, err := q2.Emit(); err != nil || out != grouped {
		t.Errorf("Emit = %q, %v; want the canonical input (precedence parentheses kept)", out, err)
	}
}

// TestParseQueryFromRootJunctionDuplicateAlias pins that the emitter's
// alias-uniqueness guard also walks a FROM-root junction (REQ-117).
// PROBE-087
func TestParseQueryFromRootJunctionDuplicateAlias(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c FROM COMPOSITION c OR OBSERVATION c")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	_, eerr := q.Emit()
	if !errors.Is(eerr, aql.ErrInvalidQuery) {
		t.Fatalf("Emit duplicate alias in junction: want ErrInvalidQuery, got %v", eerr)
	}
}

// TestParseQuerySingleRootUnaffectedByJunctionField pins the compatibility
// half of shape 6: an ordinary single-root FROM still populates Root (and
// Contains) with Junction nil (REQ-117).
// PROBE-087
func TestParseQuerySingleRootUnaffectedByJunctionField(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c FROM EHR e CONTAINS COMPOSITION c")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if q.From.Junction != nil {
		t.Errorf("From.Junction = %+v, want nil for a single-root FROM", q.From.Junction)
	}
	if q.From.Root.RMType != "EHR" || q.From.Contains == nil {
		t.Errorf("From = %+v, want EHR root with a CONTAINS child", q.From)
	}
}

// TestParseQueryBoolValue pins boolean WHERE extraction: the source
// keyword `true` / `false` (lexed as IDENTIFIER per lexer rule order)
// surfaces in Comparison.Val as aql.BoolValue.
func TestParseQueryBoolValue(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/active = true")
	if err != nil {
		t.Fatal(err)
	}
	cmp, ok := q.Where.(aql.Comparison)
	if !ok {
		t.Fatalf("Where = %T, want aql.Comparison", q.Where)
	}
	bv, ok := cmp.Val.(aql.BoolValue)
	if !ok {
		t.Fatalf("Comparison.Val = %T, want aql.BoolValue", cmp.Val)
	}
	if !bv.B {
		t.Errorf("BoolValue.B = false; want true")
	}
}

// TestParseQueryNullValue pins NULL extraction as the typed sentinel
// (not StringValue{"NULL"}).
func TestParseQueryNullValue(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/data = NULL")
	if err != nil {
		t.Fatal(err)
	}
	cmp := q.Where.(aql.Comparison)
	if _, ok := cmp.Val.(aql.NullValue); !ok {
		t.Errorf("Comparison.Val = %T, want aql.NullValue", cmp.Val)
	}
}

// TestParseQueryParamLimit pins the parameter-bound LIMIT/OFFSET
// extraction shape: ParamLimit concrete with the placeholder name.
func TestParseQueryParamLimit(t *testing.T) {
	q, err := parse.ParseQuery("SELECT e FROM EHR e LIMIT $rows OFFSET $skip")
	if err != nil {
		t.Fatal(err)
	}
	lim, ok := q.Limit.(parse.ParamLimit)
	if !ok || lim.Name != "rows" {
		t.Errorf("Limit = %v, want ParamLimit{rows}", q.Limit)
	}
	off, ok := q.Offset.(parse.ParamLimit)
	if !ok || off.Name != "skip" {
		t.Errorf("Offset = %v, want ParamLimit{skip}", q.Offset)
	}
}

// TestParseQueryStandingPredicate pins the standing class predicate
// extraction shape: HasPredicate true, Predicate carries the bracket
// body verbatim, Archetype empty.
func TestParseQueryStandingPredicate(t *testing.T) {
	q, err := parse.ParseQuery("SELECT e FROM EHR e[ehr_id/value=$id]")
	if err != nil {
		t.Fatal(err)
	}
	if !q.From.Root.HasPredicate {
		t.Errorf("From.Root.HasPredicate = false; want true")
	}
	if q.From.Root.Predicate != "ehr_id/value=$id" {
		t.Errorf("From.Root.Predicate = %q, want ehr_id/value=$id", q.From.Root.Predicate)
	}
	if q.From.Root.Archetype != "" {
		t.Errorf("From.Root.Archetype = %q, want empty for non-archetype predicate", q.From.Root.Archetype)
	}
}

// TestParseQueryParamArchetype pins the `[$name]` archetype predicate:
// the actual placeholder text (including the leading `$`) lives in
// Archetype, and the ParamArchetype flag is the typed signal.
func TestParseQueryParamArchetype(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c FROM EHR e CONTAINS COMPOSITION c[$template]")
	if err != nil {
		t.Fatal(err)
	}
	if q.From.Contains == nil {
		t.Fatal("From.Contains unexpectedly nil")
	}
	if !q.From.Contains.Class.ParamArchetype {
		t.Errorf("Contains.Class.ParamArchetype = false; want true")
	}
	if q.From.Contains.Class.Archetype != "$template" {
		t.Errorf("Contains.Class.Archetype = %q, want $template", q.From.Contains.Class.Archetype)
	}
}

// TestParseQueryVersionPredicate pins VERSION class predicate
// extraction + emission round-trip.
func TestParseQueryVersionPredicate(t *testing.T) {
	src := "SELECT v FROM EHR e CONTAINS VERSION v[all_versions]"
	q, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatal(err)
	}
	if q.From.Contains == nil {
		t.Fatal("From.Contains unexpectedly nil")
	}
	cls := q.From.Contains.Class
	if !cls.Version {
		t.Errorf("Contains.Class.Version = false; want true")
	}
	if cls.Predicate != "all_versions" {
		t.Errorf("Contains.Class.Predicate = %q, want all_versions", cls.Predicate)
	}
	out, err := q.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if out != src {
		t.Errorf("VERSION predicate round-trip\n  in:  %s\n  out: %s", src, out)
	}
}

// TestParseQueryWhereNotExpr pins the NotExpr shape extracted from a
// WHERE NOT predicate.
func TestParseQueryWhereNotExpr(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE NOT o/x = $a")
	if err != nil {
		t.Fatal(err)
	}
	ne, ok := q.Where.(aql.NotExpr)
	if !ok {
		t.Fatalf("Where = %T, want aql.NotExpr", q.Where)
	}
	if _, ok := ne.Operand.(aql.Comparison); !ok {
		t.Errorf("NotExpr.Operand = %T, want aql.Comparison", ne.Operand)
	}
}

// TestParseQueryWhereExistsExpr pins the ExistsExpr shape extracted
// from a WHERE EXISTS path predicate.
func TestParseQueryWhereExistsExpr(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE EXISTS o/data")
	if err != nil {
		t.Fatal(err)
	}
	ex, ok := q.Where.(aql.ExistsExpr)
	if !ok {
		t.Fatalf("Where = %T, want aql.ExistsExpr", q.Where)
	}
	if ex.Path != "o/data" {
		t.Errorf("ExistsExpr.Path = %q, want o/data", ex.Path)
	}
}

// TestParseQueryWhereLikeExpr pins the LikeExpr shape extracted from
// a WHERE LIKE pattern predicate; carries Pattern as a StringValue.
func TestParseQueryWhereLikeExpr(t *testing.T) {
	q, err := parse.ParseQuery("SELECT p FROM EHR e CONTAINS PERSON p WHERE p/name LIKE 'Dr%'")
	if err != nil {
		t.Fatal(err)
	}
	le, ok := q.Where.(aql.LikeExpr)
	if !ok {
		t.Fatalf("Where = %T, want aql.LikeExpr", q.Where)
	}
	if le.Path != "p/name" {
		t.Errorf("LikeExpr.Path = %q, want p/name", le.Path)
	}
	sv, ok := le.Pattern.(aql.StringValue)
	if !ok || sv.S != "Dr%" {
		t.Errorf("LikeExpr.Pattern = %v, want StringValue{Dr%%}", le.Pattern)
	}
}

// TestParseQueryWhereMatchesExpr pins the MatchesExpr shape extracted
// from a value-list MATCHES predicate; carries the value list in
// document order.
func TestParseQueryWhereMatchesExpr(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/status MATCHES {'active', 'archived'}")
	if err != nil {
		t.Fatal(err)
	}
	me, ok := q.Where.(aql.MatchesExpr)
	if !ok {
		t.Fatalf("Where = %T, want aql.MatchesExpr", q.Where)
	}
	if me.Path != "o/status" {
		t.Errorf("MatchesExpr.Path = %q, want o/status", me.Path)
	}
	if len(me.Values) != 2 {
		t.Fatalf("MatchesExpr.Values len = %d, want 2", len(me.Values))
	}
	if sv, ok := me.Values[0].(aql.StringValue); !ok || sv.S != "active" {
		t.Errorf("Values[0] = %v, want StringValue{active}", me.Values[0])
	}
	if sv, ok := me.Values[1].(aql.StringValue); !ok || sv.S != "archived" {
		t.Errorf("Values[1] = %v, want StringValue{archived}", me.Values[1])
	}
}

// TestParseQueryMatchesTerminologyOperand pins REQ-117 catalogue shape 4:
// a `MATCHES TERMINOLOGY('op','api','params')` operand models structurally
// (function name + three string arguments), not as raw text.
// PROBE-087
func TestParseQueryMatchesTerminologyOperand(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM EHR e CONTAINS OBSERVATION o " +
		"WHERE o/code MATCHES terminology('SNOMED-CT','near','12345')")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	me, ok := q.Where.(aql.MatchesExpr)
	if !ok {
		t.Fatalf("Where = %T, want aql.MatchesExpr", q.Where)
	}
	if me.Path != "o/code" {
		t.Errorf("MatchesExpr.Path = %q, want o/code", me.Path)
	}
	if len(me.Values) != 0 {
		t.Errorf("MatchesExpr.Values len = %d, want 0 for the terminology operand form", len(me.Values))
	}
	if me.Terminology == nil {
		t.Fatal("MatchesExpr.Terminology is nil; want the structured TERMINOLOGY call")
	}
	if me.Terminology.Name != aql.TerminologyFunc {
		t.Errorf("Terminology.Name = %q, want %s", me.Terminology.Name, aql.TerminologyFunc)
	}
	if len(me.Terminology.Args) != 3 {
		t.Fatalf("Terminology.Args len = %d, want 3", len(me.Terminology.Args))
	}
	for i, want := range []string{"SNOMED-CT", "near", "12345"} {
		sv, ok := me.Terminology.Args[i].(aql.StringValue)
		if !ok || sv.S != want {
			t.Errorf("Terminology.Args[%d] = %#v, want aql.StringValue{%s}", i, me.Terminology.Args[i], want)
		}
	}
	out, err := q.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	want := "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/code MATCHES TERMINOLOGY('SNOMED-CT', 'near', '12345')"
	if out != want {
		t.Errorf("Emit = %q, want %q", out, want)
	}
}

// TestParseQueryMatchesURIOperand pins the `{URI}` MATCHES operand form:
// the URI is carried verbatim on the structured predicate (REQ-117).
// PROBE-087
func TestParseQueryMatchesURIOperand(t *testing.T) {
	const src = "SELECT o FROM EHR e CONTAINS OBSERVATION o " +
		"WHERE o/code MATCHES {uri://terminology.hl7.org/CodeSystem/v3-ActCode}"
	q, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	me, ok := q.Where.(aql.MatchesExpr)
	if !ok {
		t.Fatalf("Where = %T, want aql.MatchesExpr", q.Where)
	}
	if me.URI != "uri://terminology.hl7.org/CodeSystem/v3-ActCode" {
		t.Errorf("MatchesExpr.URI = %q, want the source URI", me.URI)
	}
	if len(me.Values) != 0 || me.Terminology != nil {
		t.Errorf("MatchesExpr = %+v, want only the URI operand set", me)
	}
	if out, err := q.Emit(); err != nil || out != src {
		t.Errorf("Emit = %q, %v; want the canonical input", out, err)
	}
}

// TestParseQueryMatchesValueListTerminology pins the grammar's
// `valueListItem : terminologyFunction` alternative — a terminology call
// INSIDE the braces is one value of the list (REQ-117).
// PROBE-087
func TestParseQueryMatchesValueListTerminology(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM EHR e CONTAINS OBSERVATION o " +
		"WHERE o/code MATCHES {terminology('SNOMED-CT','near','12345'), 'other'}")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	me := q.Where.(aql.MatchesExpr)
	if me.Terminology != nil {
		t.Errorf("MatchesExpr.Terminology = %+v, want nil for the value-list form", me.Terminology)
	}
	if len(me.Values) != 2 {
		t.Fatalf("MatchesExpr.Values len = %d, want 2", len(me.Values))
	}
	fc, ok := me.Values[0].(aql.FuncCall)
	if !ok {
		t.Fatalf("Values[0] = %T, want aql.FuncCall", me.Values[0])
	}
	if fc.Name != aql.TerminologyFunc || len(fc.Args) != 3 {
		t.Errorf("Values[0] = %+v, want TERMINOLOGY with three arguments", fc)
	}
	if sv, ok := me.Values[1].(aql.StringValue); !ok || sv.S != "other" {
		t.Errorf("Values[1] = %#v, want aql.StringValue{other}", me.Values[1])
	}
}

// TestParseQueryWhereFunctionLHS pins REQ-117 catalogue shape 3: a
// function call as the LEFT operand of a WHERE comparison models as an
// [aql.Comparison] whose Left carries the structured [aql.FuncCall]
// (Path stays empty — the left operand is not a path).
// PROBE-087
func TestParseQueryWhereFunctionLHS(t *testing.T) {
	const src = "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE LENGTH(o/name/value) > 5"
	q, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	cmp, ok := q.Where.(aql.Comparison)
	if !ok {
		t.Fatalf("Where = %T, want aql.Comparison", q.Where)
	}
	if cmp.Path != "" {
		t.Errorf("Comparison.Path = %q, want empty for a function-call LHS", cmp.Path)
	}
	fc, ok := cmp.Left.(aql.FuncCall)
	if !ok {
		t.Fatalf("Comparison.Left = %T, want aql.FuncCall", cmp.Left)
	}
	if fc.Name != "LENGTH" {
		t.Errorf("FuncCall.Name = %q, want LENGTH", fc.Name)
	}
	if len(fc.Args) != 1 {
		t.Fatalf("FuncCall.Args len = %d, want 1", len(fc.Args))
	}
	pv, ok := fc.Args[0].(aql.PathValue)
	if !ok {
		t.Fatalf("FuncCall.Args[0] = %T, want aql.PathValue", fc.Args[0])
	}
	if pv.Raw != "o/name/value" || pv.Alias != "o" {
		t.Errorf("PathValue = {Raw:%q Alias:%q}, want {o/name/value o}", pv.Raw, pv.Alias)
	}
	if cmp.Op != aql.OpGt {
		t.Errorf("Comparison.Op = %q, want >", cmp.Op)
	}
	if iv, ok := cmp.Val.(aql.IntValue); !ok || iv.N != 5 {
		t.Errorf("Comparison.Val = %#v, want aql.IntValue{5}", cmp.Val)
	}
	if out, err := q.Emit(); err != nil || out != src {
		t.Errorf("Emit = %q, %v; want the canonical input", out, err)
	}
}

// TestParseQueryWhereFunctionRHS pins the grammar's `terminal :
// functionCall` alternative on the right of a comparison — the function
// call is a structured [aql.FuncCall] in the value position (REQ-117).
// PROBE-087
func TestParseQueryWhereFunctionRHS(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = LENGTH(o/y)")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	cmp := q.Where.(aql.Comparison)
	if cmp.Path != "o/x" {
		t.Errorf("Comparison.Path = %q, want o/x", cmp.Path)
	}
	fc, ok := cmp.Val.(aql.FuncCall)
	if !ok {
		t.Fatalf("Comparison.Val = %T, want aql.FuncCall", cmp.Val)
	}
	if fc.Name != "LENGTH" || len(fc.Args) != 1 {
		t.Errorf("FuncCall = %+v, want LENGTH with one argument", fc)
	}
}

// TestParseQueryPathVsPathComparison pins REQ-117 catalogue shape 5: an
// identified path as the RIGHT operand models as an introspectable
// [aql.PathValue] (alias + segments), not raw text.
// PROBE-087
func TestParseQueryPathVsPathComparison(t *testing.T) {
	const src = "SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = o/data[at0001]/value"
	q, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	cmp := q.Where.(aql.Comparison)
	pv, ok := cmp.Val.(aql.PathValue)
	if !ok {
		t.Fatalf("Comparison.Val = %T, want aql.PathValue", cmp.Val)
	}
	if pv.Alias != "o" {
		t.Errorf("PathValue.Alias = %q, want o", pv.Alias)
	}
	if len(pv.Segments) != 2 {
		t.Fatalf("PathValue.Segments len = %d, want 2 (%+v)", len(pv.Segments), pv.Segments)
	}
	if got := pv.Segments[0]; got.Name != "data" || got.Predicate != "at0001" {
		t.Errorf("Segments[0] = %+v, want {data at0001}", got)
	}
	if pv.Raw != "o/data[at0001]/value" {
		t.Errorf("PathValue.Raw = %q, want o/data[at0001]/value", pv.Raw)
	}
	if out, err := q.Emit(); err != nil || out != src {
		t.Errorf("Emit = %q, %v; want the canonical input", out, err)
	}
}

// TestParseQueryWhereFunctionArguments pins REQ-117 catalogue shape 7 on
// the WHERE side: parameter, primitive, and nested-function arguments
// inside a function call in a comparison.
// PROBE-087
func TestParseQueryWhereFunctionArguments(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE CONCAT('a', $p, LENGTH(o/y)) = 'abc'")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	cmp := q.Where.(aql.Comparison)
	fc, ok := cmp.Left.(aql.FuncCall)
	if !ok {
		t.Fatalf("Comparison.Left = %T, want aql.FuncCall", cmp.Left)
	}
	if len(fc.Args) != 3 {
		t.Fatalf("FuncCall.Args len = %d, want 3", len(fc.Args))
	}
	if sv, ok := fc.Args[0].(aql.StringValue); !ok || sv.S != "a" {
		t.Errorf("Args[0] = %#v, want aql.StringValue{a}", fc.Args[0])
	}
	if pv, ok := fc.Args[1].(aql.ParamValue); !ok || pv.Name != "p" {
		t.Errorf("Args[1] = %#v, want aql.ParamValue{p}", fc.Args[1])
	}
	nested, ok := fc.Args[2].(aql.FuncCall)
	if !ok {
		t.Fatalf("Args[2] = %T, want aql.FuncCall", fc.Args[2])
	}
	if nested.Name != "LENGTH" || len(nested.Args) != 1 {
		t.Errorf("nested call = %+v, want LENGTH with one argument", nested)
	}
}

// TestParseQueryWhereJunctionOverNewOperands pins REQ-117 catalogue
// shape 8: a junction is in-catalogue exactly when all its operands are,
// so an AND/OR over the newly modelled operands models without a gap and
// keeps its operand order.
// PROBE-087
func TestParseQueryWhereJunctionOverNewOperands(t *testing.T) {
	const src = "SELECT o FROM EHR e CONTAINS OBSERVATION o " +
		"WHERE o/x = $a AND LENGTH(o/name) > 5 AND o/p = o/q"
	q, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	j, ok := q.Where.(aql.Junction)
	if !ok {
		t.Fatalf("Where = %T, want aql.Junction", q.Where)
	}
	if j.Op != aql.OpAnd {
		t.Errorf("Junction.Op = %q, want AND", j.Op)
	}
	if len(j.Terms) != 3 {
		t.Fatalf("Junction.Terms len = %d, want 3 (%+v)", len(j.Terms), j.Terms)
	}
	if cmp, ok := j.Terms[0].(aql.Comparison); !ok || cmp.Path != "o/x" {
		t.Errorf("Terms[0] = %#v, want the o/x comparison", j.Terms[0])
	}
	if cmp, ok := j.Terms[1].(aql.Comparison); !ok {
		t.Errorf("Terms[1] = %T, want aql.Comparison", j.Terms[1])
	} else if _, ok := cmp.Left.(aql.FuncCall); !ok {
		t.Errorf("Terms[1].Left = %T, want aql.FuncCall", cmp.Left)
	}
	if cmp, ok := j.Terms[2].(aql.Comparison); !ok {
		t.Errorf("Terms[2] = %T, want aql.Comparison", j.Terms[2])
	} else if _, ok := cmp.Val.(aql.PathValue); !ok {
		t.Errorf("Terms[2].Val = %T, want aql.PathValue", cmp.Val)
	}
	if out, err := q.Emit(); err != nil || out != src {
		t.Errorf("Emit = %q, %v; want the canonical input", out, err)
	}
}

// TestDocumentQueryErrContract pins the QueryErr accessor: nil for a
// clean parse, an ErrIncompleteAST wrap for a catalogue-gap parse,
// stable across repeated calls (same sync.Once guard as Query).
func TestDocumentQueryErrContract(t *testing.T) {
	doc, err := parse.Parse("SELECT o FROM EHR e CONTAINS OBSERVATION o WHERE o/x = $a")
	if err != nil {
		t.Fatal(err)
	}
	if qerr := doc.QueryErr(); qerr != nil {
		t.Errorf("QueryErr on clean parse = %v, want nil", qerr)
	}
	// Same accessor on a catalogue-gap query returns the gap error. The
	// residual gap after REQ-117 is the LIMIT/OFFSET int-overflow guard.
	gapDoc, err := parse.Parse("SELECT e FROM EHR e LIMIT 9223372036854775808")
	if err != nil {
		t.Fatal(err)
	}
	qerr := gapDoc.QueryErr()
	if !errors.Is(qerr, aql.ErrIncompleteAST) {
		t.Errorf("QueryErr on incomplete-AST parse = %v, want ErrIncompleteAST", qerr)
	}
	// Repeated calls return the stable cached error (sync.Once guard).
	// errors.Is so we compare via the wrapped sentinel rather than
	// pointer identity, and the message stability check confirms the
	// same underlying error instance.
	qerr2 := gapDoc.QueryErr()
	if !errors.Is(qerr2, aql.ErrIncompleteAST) || qerr2.Error() != qerr.Error() {
		t.Errorf("QueryErr second call = %v, want stable %v", qerr2, qerr)
	}
}

// TestParseQueryLimitOverflow pins the integer-overflow gap: a LIMIT
// literal that overflows Go `int` surfaces ErrIncompleteAST rather
// than silently dropping the clause.
func TestParseQueryLimitOverflow(t *testing.T) {
	_, err := parse.ParseQuery("SELECT e FROM EHR e LIMIT 9223372036854775808")
	if !errors.Is(err, aql.ErrIncompleteAST) {
		t.Fatalf("ParseQuery overflow: want ErrIncompleteAST, got %v", err)
	}
}

// TestSelectFunctionArgLiftsBareKeyword pins that a bare `true` / `false` /
// `null` keyword in a SELECT function ARGUMENT is modelled as a literal, the
// same as in the WHERE-side terminal position. The SDK lexer hands these
// over as IDENTIFIER, so without the lift the argument would model as a
// PathExpr and typed consumers would read the same construct differently
// depending on the clause it sits in.
// REQ-117
func TestSelectFunctionArgLiftsBareKeyword(t *testing.T) {
	cases := map[string]struct {
		in       string
		want     aql.Value
		wantEmit string
	}{
		"concat_true": {
			"SELECT CONCAT(true, 'x') FROM COMPOSITION c",
			aql.BoolValue{B: true},
			"SELECT CONCAT(true, 'x') FROM COMPOSITION c",
		},
		"length_false": {
			"SELECT LENGTH(false) FROM COMPOSITION c",
			aql.BoolValue{B: false},
			"SELECT LENGTH(false) FROM COMPOSITION c",
		},
		// `null` is a grammar primitive rather than an IDENTIFIER, so it
		// already lifts; it is pinned here so the two spellings of a bare
		// keyword argument stay modelled the same way. Canonical emission
		// upper-cases it.
		"concat_null": {
			"SELECT CONCAT(null, 'x') FROM COMPOSITION c",
			aql.NullValue{},
			"SELECT CONCAT(NULL, 'x') FROM COMPOSITION c",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			q, err := parse.ParseQuery(tc.in)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			fc, ok := q.Select.Items[0].Expr.(parse.FunctionCall)
			if !ok {
				t.Fatalf("SELECT item is %T, want parse.FunctionCall", q.Select.Items[0].Expr)
			}
			lit, ok := fc.Args[0].(parse.LiteralExpr)
			if !ok {
				t.Fatalf("arg 0 is %T, want parse.LiteralExpr (a bare keyword is a literal, not a path)", fc.Args[0])
			}
			if lit.Value != tc.want {
				t.Errorf("arg 0 value = %#v, want %#v", lit.Value, tc.want)
			}
			// Emission is unaffected by the lift, so the round-trip and the
			// canonical form stay exactly as before.
			out, err := q.Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if out != tc.wantEmit {
				t.Errorf("Emit = %q, want %q", out, tc.wantEmit)
			}
		})
	}
}
