package parse_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// TestDocumentTreeReturnsValidatedTree pins REQ-113 Tier 1:
// a successful Parse exposes the validated ANTLR tree via the
// Document.Tree() accessor — non-nil, with a SelectClause reachable.
// The accessor's *type* is an unstable consumer contract per its doc;
// this test asserts only that the validated tree is reachable, not the
// shape of its typed children.
func TestDocumentTreeReturnsValidatedTree(t *testing.T) {
	doc, err := parse.Parse("SELECT e/ehr_id/value FROM EHR e")
	if err != nil {
		t.Fatal(err)
	}
	tree := doc.Tree()
	if tree == nil {
		t.Fatal("Document.Tree() returned nil on a parsed query")
	}
	if tree.SelectClause() == nil {
		t.Errorf("Document.Tree().SelectClause() unexpectedly nil")
	}
	// Smoke: the zero document's Tree() must not panic and must return nil.
	var zero parse.Document
	if zero.Tree() != nil {
		t.Errorf("zero Document.Tree() = %v, want nil", zero.Tree())
	}
}

// TestParseGrammarFixtures drives every fixture under testdata/grammar: `.aql`
// files MUST parse, `.reject` files MUST fail with aql.ErrSyntax. These double
// as the regression suite for the SDK-AQL-NNN grammar deltas (ADR 0007).
//
// REQ-109: the parse layer holds the SDK grammar profile; accept fixtures
// parse and reject fixtures fail with aql.ErrSyntax.
func TestParseGrammarFixtures(t *testing.T) {
	dir := filepath.Join("testdata", "grammar")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no grammar fixtures found")
	}
	for _, e := range entries {
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			_, perr := parse.Parse(string(body))
			switch {
			case strings.HasSuffix(name, ".reject"):
				if perr == nil {
					t.Fatalf("expected syntax error, got nil")
				}
				if !errors.Is(perr, aql.ErrSyntax) {
					t.Fatalf("error does not wrap aql.ErrSyntax: %v", perr)
				}
			default: // .aql — must parse
				if perr != nil {
					t.Fatalf("unexpected error: %v", perr)
				}
			}
		})
	}
}

func TestParseSelectStar(t *testing.T) {
	doc, err := parse.Parse("SELECT * FROM EHR e")
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Star {
		t.Fatal("Star = false, want true for SELECT *")
	}
}

func TestParseClausePresence(t *testing.T) {
	doc, err := parse.Parse(
		"SELECT DISTINCT c FROM EHR e CONTAINS COMPOSITION c " +
			"WHERE c/name/value = 'x' ORDER BY c/uid/value LIMIT 10 OFFSET 5",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Distinct || doc.Star || doc.NumSelect != 1 || !doc.HasWhere || !doc.HasOrderBy || !doc.HasLimit {
		t.Fatalf("clause flags: %+v", doc)
	}
}

// TestParseSelectAliases pins REQ-109: the flat view records the SELECT `AS`
// aliases in document order so the lint layer can resolve an ORDER BY key
// against them once FROM/CONTAINS has had its turn. Projection items without
// an AS clause contribute nothing.
func TestParseSelectAliases(t *testing.T) {
	doc, err := parse.Parse(
		"SELECT o/data[at0001]/value/magnitude AS score, o/name/value, COUNT(*) AS n " +
			"FROM OBSERVATION o ORDER BY score DESC",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"score", "n"}
	if len(doc.SelectAliases) != len(want) {
		t.Fatalf("SelectAliases = %v, want %v", doc.SelectAliases, want)
	}
	for i, w := range want {
		if doc.SelectAliases[i] != w {
			t.Errorf("SelectAliases[%d] = %q, want %q", i, doc.SelectAliases[i], w)
		}
	}

	// A projection list with no AS clause records none.
	bare, err := parse.Parse("SELECT o/name/value FROM OBSERVATION o")
	if err != nil {
		t.Fatal(err)
	}
	if len(bare.SelectAliases) != 0 {
		t.Errorf("SelectAliases = %v, want none", bare.SelectAliases)
	}
}

// TestParseREQ055Golden parses the REQ-055 builder reference query, tying the
// parse front-end to the canonical AQL the builders emit.
func TestParseREQ055Golden(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "testdata", "wire", "observations_by_archetype.aql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parse.Parse(string(body)); err != nil {
		t.Fatalf("golden query must parse: %v", err)
	}
}

// REQ-109: a syntax error is a *parse.SyntaxError wrapping aql.ErrSyntax and
// carrying the parser's position.
func TestParseSyntaxErrorPosition(t *testing.T) {
	_, err := parse.Parse("SELECT FROM EHR e") // missing projection
	if !errors.Is(err, aql.ErrSyntax) {
		t.Fatalf("err = %v, want ErrSyntax", err)
	}
	se, ok := errors.AsType[*parse.SyntaxError](err)
	if !ok {
		t.Fatalf("expected *parse.SyntaxError, got %T", err)
	}
	if se.Pos.Line != 1 || se.Pos.Col < 1 {
		t.Fatalf("position = %+v", se.Pos)
	}
}

// nestedQuery is a query whose WHERE operand opens depth parentheses, all
// closed when closed is true.
func nestedQuery(depth int, closed bool) string {
	q := "SELECT e/ehr_id/value FROM EHR e WHERE " + strings.Repeat("(", depth) + "e/ehr_id/value = 'x'"
	if closed {
		q += strings.Repeat(")", depth)
	}
	return q
}

// bracketQuery is a query whose WHERE operand nests depth path predicates,
// one inside the other, after parens parentheses.
func bracketQuery(parens, depth int) string {
	return "SELECT e/ehr_id/value FROM EHR e WHERE " + strings.Repeat("(", parens) + "e/a" +
		strings.Repeat("[b", depth) + strings.Repeat("='x']", depth) + " = 'x'" + strings.Repeat(")", parens)
}

// TestREQ109_ParseRefusesNestingPastTheLimit pins REQ-109 § Nesting limit:
// Parse admits parentheses and brackets nested 128 levels deep, counted
// together, and refuses 129, at the parenthesis or bracket that opens level
// 129, with a SyntaxError that matches both aql.ErrSyntax and
// aql.ErrLimitExceeded. The limit refusal is told apart from the parser's
// own: an unclosed query nested 128 deep is a syntax error that does not
// match aql.ErrLimitExceeded, the same query 129 deep is the limit's. A
// parenthesis inside a string literal does not count, and a closing
// parenthesis with none open lends no depth to the ones after it.
func TestREQ109_ParseRefusesNestingPastTheLimit(t *testing.T) {
	t.Parallel()
	const prefix = "SELECT e/ehr_id/value FROM EHR e WHERE "
	limitErr := func(t *testing.T, q string, col int) {
		t.Helper()
		_, err := parse.Parse(q)
		se, ok := errors.AsType[*parse.SyntaxError](err)
		if !ok || se == nil {
			t.Fatalf("Parse(%d bytes) error = %v; want a *parse.SyntaxError", len(q), err)
		}
		if !errors.Is(err, aql.ErrSyntax) || !errors.Is(err, aql.ErrLimitExceeded) {
			t.Errorf("Parse(%d bytes) error = %v; want it to match aql.ErrSyntax and aql.ErrLimitExceeded", len(q), err)
		}
		if want := (parse.Position{Line: 1, Col: col}); se.Pos != want {
			t.Errorf("Parse(%d bytes) error at %+v; want %+v, the parenthesis that opens level 129", len(q), se.Pos, want)
		}
		if !strings.Contains(se.Msg, "128") {
			t.Errorf("Parse(%d bytes) message %q; want it to name the limit, 128", len(q), se.Msg)
		}
	}

	if _, err := parse.Parse(nestedQuery(128, true)); err != nil {
		t.Errorf("Parse(nested 128 deep) = %v; want nil", err)
	}
	limitErr(t, nestedQuery(129, true), len(prefix)+129)
	limitErr(t, nestedQuery(129, false), len(prefix)+129)
	stray := "SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = 'x' ))) AND " + strings.Repeat("(", 129) + "e/ehr_id/value = 'x'" + strings.Repeat(")", 129)
	limitErr(t, stray, strings.Index(stray, "AND ")+len("AND ")+129)

	// Brackets count as parentheses do, and together with them.
	bracketAt := func(q string, n int) int { // the column of the n-th '[' after "e/a"
		return strings.Index(q, "e/a[") + len("e/a") + 2*(n-1) + 1
	}
	for _, q := range []string{bracketQuery(0, 128), bracketQuery(64, 64)} {
		if _, err := parse.Parse(q); err != nil {
			t.Errorf("Parse(%d bytes, nested 128 deep) = %v; want nil", len(q), err)
		}
	}
	limitErr(t, bracketQuery(0, 129), bracketAt(bracketQuery(0, 129), 129))
	limitErr(t, bracketQuery(64, 65), bracketAt(bracketQuery(64, 65), 65))

	_, err := parse.Parse(nestedQuery(128, false))
	if !errors.Is(err, aql.ErrSyntax) || errors.Is(err, aql.ErrLimitExceeded) {
		t.Errorf("Parse(unclosed, nested 128 deep) error = %v; want a syntax error that does not match aql.ErrLimitExceeded", err)
	}
	if _, err := parse.Parse(prefix + "e/ehr_id/value = '" + strings.Repeat("(", 200) + "'"); err != nil {
		t.Errorf("Parse(200 parentheses inside a string literal) = %v; want nil", err)
	}
	var nilErr *parse.SyntaxError
	if nilErr.Is(aql.ErrLimitExceeded) || (&parse.SyntaxError{}).Is(aql.ErrLimitExceeded) {
		t.Error("a nil or zero SyntaxError matches aql.ErrLimitExceeded; want neither")
	}
}
