package aql

// projection_internal_test.go reaches the unexported clause-escape scan of
// REQ-163 § `Build()` verifies what it emitted, and pins it in the two
// directions the public surface can only sample: which characters the scan
// treats as DELIMITERS (its lexer states), and what structure it reads back out
// of a clause text (the reduction).
//
// Why this needs its own file: the scan is the hand-derived stand-in for a
// re-parse, and openehr/aql cannot re-parse (parse imports aql). Every row here
// therefore states what the parser would do, and the corpus in
// containment_roundtrip_test.go — which does parse — is what proves the
// statements right. If a row here and a row there ever disagree, THIS one is
// wrong.
// REQ-163 · PROBE-088

import (
	"errors"
	"strings"
	"testing"
)

// TestProjectionScanTracksLexerStates pins the states that make a character not
// the delimiter it looks like. Each row is text the emitter can legitimately
// produce, or text a caller can legitimately splice through [Col], so a scan
// that miscounted here would refuse valid AQL.
func TestProjectionScanTracksLexerStates(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		commas int
		words  []string
		flaw   bool
	}{
		{name: "a bare path has no delimiters", text: "c/uid/value"},
		{name: "top-level comma separates items", text: "c/a, c/b", commas: 1},
		{
			name: "a comma inside a string literal is content",
			text: "'a, b'",
		},
		{
			name: "a clause keyword inside a string literal is content",
			text: "'FROM EHR e'",
		},
		{
			name: "a comma inside a node predicate is content",
			text: "c/content[at0001,'Temp']/value",
		},
		{
			name: "a comma inside a function call is an argument separator",
			text: "CONCAT(c/a, c/b)",
		},
		{
			name: "a comma inside a contained regex is content",
			text: "c/x[a/b MATCHES {/[0-9]{2,3}/}]",
		},
		{
			// TERM_CODE_CHAR admits '-', so maximal munch takes both dashes
			// into the code and there is no comment here.
			name: "a double dash inside a term code is not a comment",
			text: "c/x[at0001,SNOMED-CT::1--2]",
		},
		{
			name:  "a clause keyword at the top level is an escape",
			text:  "c/uid/value FROM EHR e",
			words: []string{"FROM"},
		},
		{
			name:  "AS is reported so the alias boundary can be found",
			text:  "c/uid/value AS uid",
			words: []string{"AS"},
		},
		{
			name:  "a comment at the top level swallows the rest of the query",
			text:  "c/uid/value -- rest\n",
			words: []string{"--"},
		},
		{name: "an unterminated string literal is a flaw", text: "'abc", flaw: true},
		{name: "an unclosed node predicate is a flaw", text: "c/x[at0001", flaw: true},
		{name: "a stray closing bracket is a flaw", text: "c/x]", flaw: true},
		{name: "an unclosed function call is a flaw", text: "COUNT(c/x", flaw: true},
		{name: "a stray closing paren is a flaw", text: "c/x)", flaw: true},
		{name: "an unterminated comment is a flaw", text: "c/x -- rest", flaw: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sc := scanProjection(tc.text)
			if (sc.flaw != "") != tc.flaw {
				t.Fatalf("flaw = %q, want flaw=%t", sc.flaw, tc.flaw)
			}
			if tc.flaw {
				return
			}
			if len(sc.commas) != tc.commas {
				t.Errorf("top-level commas = %d, want %d", len(sc.commas), tc.commas)
			}
			var got []string
			for _, w := range sc.words {
				got = append(got, w.word)
			}
			if strings.Join(got, ",") != strings.Join(tc.words, ",") {
				t.Errorf("clause words = %v, want %v", got, tc.words)
			}
		})
	}
}

// TestProjectionKeywordMatchIsWordBounded pins the boundary rule: a path
// segment merely CONTAINING a keyword's letters is not that keyword. Without
// both boundaries the guard would refuse ordinary openEHR paths, which is the
// over-refusal direction REQ-163 § Acceptance requires rows for.
func TestProjectionKeywordMatchIsWordBounded(t *testing.T) {
	for _, text := range []string{
		"c/from_date", "c/topic", "c/as_of", "c/orderly", "c/whereabouts", "c/limits",
		"c/offsets", "c/distinctive", "c/selection", "c/contains_x", "c/x ASC",
	} {
		t.Run(text, func(t *testing.T) {
			if sc := scanProjection(text); len(sc.words) != 0 {
				t.Fatalf("%q read as carrying the clause word %q", text, sc.words[0].word)
			}
		})
	}
	// The control: with the boundaries, the real keyword still matches.
	if sc := scanProjection("c/x FROM y"); len(sc.words) != 1 || sc.words[0].word != "FROM" {
		t.Fatalf("the boundary rule stopped matching the real keyword: %+v", sc.words)
	}
}

// TestDeriveSelectShapeReadsTheClauseBack pins the reduction: what a reader of
// the emitted clause text carries. The `SELECT ` keyword itself is not part of
// the text — [ast.selectClause] renders the payload — so each row here is
// exactly the bytes that follow it.
func TestDeriveSelectShapeReadsTheClauseBack(t *testing.T) {
	tests := []struct {
		name    string
		clause  string
		want    selectShape
		escapes int
	}{
		{
			name:   "one path item",
			clause: "c/uid/value",
			want:   selectShape{items: 1, aliases: []string{""}},
		},
		{
			name:   "two items",
			clause: "c/a, c/b",
			want:   selectShape{items: 2, aliases: []string{"", ""}},
		},
		{
			name:   "an aliased item",
			clause: "COUNT(c/a) AS n",
			want:   selectShape{items: 1, aliases: []string{"n"}},
		},
		{
			// The sole-star reduction: the flag carries the projection and the
			// item list is empty, which is how `SELECT *` re-parses.
			name:   "the bare star form",
			clause: "*",
			want:   selectShape{star: true},
		},
		{
			// Mixed, so the item list is authoritative AND the flag is set —
			// exactly as parse.SelectClause carries it.
			name:   "a star mixed with a column",
			clause: "*, c/a",
			want:   selectShape{star: true, items: 2, aliases: []string{"", ""}},
		},
		{
			name:   "the clause-level DISTINCT",
			clause: "DISTINCT c/a",
			want:   selectShape{distinct: true, items: 1, aliases: []string{""}},
		},
		{
			name:   "DISTINCT before the deprecated TOP",
			clause: "DISTINCT TOP 5 c/a",
			want:   selectShape{distinct: true, top: true, items: 1, aliases: []string{""}},
		},
		{
			name:   "a directed TOP belongs to the clause",
			clause: "TOP 5 BACKWARD c/a",
			want:   selectShape{top: true, items: 1, aliases: []string{""}},
		},
		{
			// The keyword that ends the projection: the reader stops there, so
			// the clause the parser sees is not the one the builder wrote.
			name:    "a clause keyword inside an item",
			clause:  "c/a FROM EHR e",
			want:    selectShape{items: 1, aliases: []string{""}},
			escapes: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveSelectShape(tc.clause)
			if got.flaw != "" {
				t.Fatalf("unexpected flaw %q", got.flaw)
			}
			if len(got.escapes) != tc.escapes {
				t.Fatalf("escapes = %d, want %d (%+v)", len(got.escapes), tc.escapes, got.escapes)
			}
			if got.shape.distinct != tc.want.distinct || got.shape.top != tc.want.top ||
				got.shape.star != tc.want.star || got.shape.items != tc.want.items {
				t.Fatalf("shape = %+v, want %+v", got.shape, tc.want)
			}
			if strings.Join(got.shape.aliases, "|") != strings.Join(tc.want.aliases, "|") {
				t.Fatalf("aliases = %v, want %v", got.shape.aliases, tc.want.aliases)
			}
		})
	}
}

// TestRecordedAndDerivedReductionsAgree pins the two halves of the comparison
// against each other for the encoding they both have to erase. The recorded
// side runs over the builder's item LIST and the derived side over the emitted
// TEXT — two different functions over two different inputs — which is what
// makes a reduction dropped on either side alone visible as a self-refusal
// (TestSoleStarItemReducesToTheBareForm is the public witness).
func TestRecordedAndDerivedReductionsAgree(t *testing.T) {
	for _, sel := range [][]SelectField{
		{Star()},
		{Col("*")},
		{Star(), Col("c/a")},
		{Col("c/a")},
		{ColAs("c/a", "x"), Col("c/b")},
		{CountStar().As("n")},
	} {
		a := &ast{sel: sel}
		clause, rec, err := a.selectClause()
		if err != nil {
			t.Fatalf("selectClause(%v): %v", sel, err)
		}
		want, got := rec.shape(), deriveSelectShape(clause).shape
		if want.star != got.star || want.items != got.items {
			t.Errorf("clause %q: recorded {star:%t items:%d}, derived {star:%t items:%d}",
				clause, want.star, want.items, got.star, got.items)
		}
	}
}

// TestVerifySelectClauseRefusesADisagreement pins the comparison itself at the
// seam, by handing it a clause text that does NOT match the recorded structure —
// the state a defect in the renderer would produce and no public call can reach
// today. Without these rows the comparison arms are dead code a refactor could
// delete with every other test still green.
func TestVerifySelectClauseRefusesADisagreement(t *testing.T) {
	recorded := func(distinct, top bool, sel ...SelectField) recordedProjection {
		t.Helper()
		a := &ast{sel: sel, distinct: distinct}
		if top {
			a.top = &TopClause{N: 5}
		}
		_, rec, err := a.selectClause()
		if err != nil {
			t.Fatalf("selectClause: %v", err)
		}
		return rec
	}
	tests := map[string]struct {
		clause string
		rec    recordedProjection
		want   string
	}{
		"a DISTINCT the builder never recorded": {
			clause: "DISTINCT c/a",
			rec:    recorded(false, false, Col("c/a")),
			want:   "DISTINCT",
		},
		"a TOP the builder never recorded": {
			clause: "TOP 5 c/a",
			rec:    recorded(false, false, Col("c/a")),
			want:   "TOP",
		},
		"a DISTINCT the builder recorded and the text lost": {
			clause: "c/a",
			rec:    recorded(true, false, Col("c/a")),
			want:   "DISTINCT",
		},
		"an item list the text split": {
			clause: "c/a, c/b",
			rec:    recorded(false, false, Col("c/a")),
			want:   "changes the projection's structure",
		},
		"an item list the text collapsed": {
			clause: "c/a",
			rec:    recorded(false, false, Col("c/a"), Col("c/b")),
			want:   "changes the projection's structure",
		},
		"a clause keyword inside an item": {
			clause: "c/a FROM EHR e",
			rec:    recorded(false, false, Col("c/a")),
			want:   "spills out of the projection",
		},
		"an alias the builder did not record": {
			clause: "c/a AS m",
			rec:    recorded(false, false, ColAs("c/a", "n")),
			want:   "AS alias the builder did not record",
		},
		"an opaque region left open": {
			clause: "c/a['x",
			rec:    recorded(false, false, Col("c/a")),
			want:   "unterminated",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := verifySelectClause(tc.clause, tc.rec)
			if !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to name %q", err, tc.want)
			}
		})
	}
	// The control: the clause the builder actually emitted agrees with itself.
	t.Run("the emitted clause agrees with what was recorded", func(t *testing.T) {
		a := &ast{sel: []SelectField{ColAs("c/a", "n"), CountStar()}, distinct: true, top: &TopClause{N: 5}}
		clause, rec, err := a.selectClause()
		if err != nil {
			t.Fatalf("selectClause: %v", err)
		}
		if err := verifySelectClause(clause, rec); err != nil {
			t.Fatalf("the verification refused the clause the builder itself emitted (%q): %v", clause, err)
		}
	})
}

// TestMalformedTopDoesNotFoolTheVerification backs the ordering claim in
// [ast.selectClause]: the TOP operand is rendered BEFORE validatePaging runs, so
// a malformed one reaches the verification. It must be neither mis-diagnosed as
// a projection defect nor allowed to hide one — and the paging refusal must
// still land afterwards.
func TestMalformedTopDoesNotFoolTheVerification(t *testing.T) {
	for name, top := range map[string]*TopClause{
		"negative count":    {N: -5},
		"unknown direction": {N: 5, Dir: TopDir(99)},
	} {
		t.Run(name, func(t *testing.T) {
			a := &ast{sel: []SelectField{Col("c/a"), Col("c/b")}, top: top}
			clause, rec, err := a.selectClause()
			if err != nil {
				t.Fatalf("selectClause: %v", err)
			}
			if err := verifySelectClause(clause, rec); err != nil {
				t.Fatalf("a TOP defect was mis-diagnosed as a projection one (clause %q): %v", clause, err)
			}
			if err := a.validateTop(false); !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("validateTop = %v, want ErrInvalidQuery — the refusal must still land", err)
			}
		})
	}
}

// TestProjectionCoordinateNamesTheItem pins the diagnostic coordinate: a
// refusal has to say WHICH item is at fault, because it may not quote any of
// it (REQ-119 § the redaction rule) and a caller with a ten-item projection has
// nothing else to go on.
func TestProjectionCoordinateNamesTheItem(t *testing.T) {
	a := &ast{sel: []SelectField{Col("c/a"), Col("c/b, c/c"), Col("c/d")}}
	clause, rec, err := a.selectClause()
	if err != nil {
		t.Fatalf("selectClause: %v", err)
	}
	err = verifySelectClause(clause, rec)
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery", err)
	}
	if !strings.Contains(err.Error(), "SELECT item 1") {
		t.Fatalf("refusal does not name the item at fault: %v", err)
	}
}
