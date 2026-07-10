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

func TestParseSyntaxErrorPosition(t *testing.T) {
	_, err := parse.Parse("SELECT FROM EHR e") // missing projection
	if !errors.Is(err, aql.ErrSyntax) {
		t.Fatalf("err = %v, want ErrSyntax", err)
	}
	var se *parse.SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("expected *parse.SyntaxError, got %T", err)
	}
	if se.Pos.Line != 1 || se.Pos.Col < 1 {
		t.Fatalf("position = %+v", se.Pos)
	}
}
