package parse_test

// query_test.go pins SDK-GAP-17 Tier 2 (REQ-113) end-to-end on simple
// AQL inputs: ParseQuery returns a populated *Query whose SELECT /
// FROM / WHERE / ORDER BY / LIMIT shapes match the source. The
// round-trip property (parsed → emit → byte-compare) is Phase 3e's
// concern; this file pins the EXTRACTION shape.

import (
	"errors"
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
		t.Errorf("Limit = %d, want nil", *q.Limit)
	}
	if q.Offset != nil {
		t.Errorf("Offset = %d, want nil", *q.Offset)
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

// TestParseQueryLimitOffset pins LIMIT/OFFSET extraction as *int.
func TestParseQueryLimitOffset(t *testing.T) {
	q, err := parse.ParseQuery("SELECT e FROM EHR e LIMIT 50 OFFSET 100")
	if err != nil {
		t.Fatal(err)
	}
	if q.Limit == nil || *q.Limit != 50 {
		t.Errorf("Limit = %v, want 50", q.Limit)
	}
	if q.Offset == nil || *q.Offset != 100 {
		t.Errorf("Offset = %v, want 100", q.Offset)
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
