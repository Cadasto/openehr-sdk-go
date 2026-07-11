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
	// Same accessor on a catalogue-gap query returns the gap error.
	gapDoc, err := parse.Parse("SELECT 1 FROM EHR e")
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
