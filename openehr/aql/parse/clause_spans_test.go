package parse_test

// clause_spans_test.go: REQ-113 § Surface — Query.Spans locates each clause
// of a parsed query in its source text, in bytes, so a consumer can splice
// the text (add a WHERE conjunct before ORDER BY / LIMIT) without the
// deprecated raw tree. Every expected clause text below is pinned as a
// literal: a grammar regeneration that moves a token boundary fails here.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// clauseTexts is the source text each clause span must slice out; an empty
// field means the clause is absent and its span must be zero.
type clauseTexts struct {
	sel, from, where, whereExpr, orderBy, limit, offset string
}

func TestREQ113_ClauseSpansSliceEachClause(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
		want clauseTexts
	}{
		{
			name: "select and from only",
			src:  "SELECT e/ehr_id/value FROM EHR e",
			want: clauseTexts{sel: "SELECT e/ehr_id/value", from: "FROM EHR e"},
		},
		{
			name: "every clause",
			src: "SELECT c/uid/value AS uid FROM EHR e CONTAINS COMPOSITION c " +
				"WHERE c/name/value = 'x' ORDER BY c/context/start_time/value DESC LIMIT 10 OFFSET 20",
			want: clauseTexts{
				sel:       "SELECT c/uid/value AS uid",
				from:      "FROM EHR e CONTAINS COMPOSITION c",
				where:     "WHERE c/name/value = 'x'",
				whereExpr: "c/name/value = 'x'",
				orderBy:   "ORDER BY c/context/start_time/value DESC",
				limit:     "LIMIT 10",
				offset:    "OFFSET 20",
			},
		},
		{
			name: "where alone",
			src:  "SELECT c/uid/value FROM COMPOSITION c WHERE c/name/value = 'x'",
			want: clauseTexts{
				sel:       "SELECT c/uid/value",
				from:      "FROM COMPOSITION c",
				where:     "WHERE c/name/value = 'x'",
				whereExpr: "c/name/value = 'x'",
			},
		},
		{
			name: "order by with several terms and no where",
			src:  "SELECT c/uid/value FROM COMPOSITION c ORDER BY c/name/value, c/uid/value ASC",
			want: clauseTexts{
				sel:     "SELECT c/uid/value",
				from:    "FROM COMPOSITION c",
				orderBy: "ORDER BY c/name/value, c/uid/value ASC",
			},
		},
		{
			name: "limit without offset",
			src:  "SELECT c/uid/value FROM COMPOSITION c WHERE c/name/value = 'x' LIMIT 50",
			want: clauseTexts{
				sel:       "SELECT c/uid/value",
				from:      "FROM COMPOSITION c",
				where:     "WHERE c/name/value = 'x'",
				whereExpr: "c/name/value = 'x'",
				limit:     "LIMIT 50",
			},
		},
		{
			name: "parameter limit and offset",
			src:  "SELECT c/uid/value FROM COMPOSITION c LIMIT $rows OFFSET $skip",
			want: clauseTexts{
				sel:    "SELECT c/uid/value",
				from:   "FROM COMPOSITION c",
				limit:  "LIMIT $rows",
				offset: "OFFSET $skip",
			},
		},
		{
			name: "select distinct",
			src:  "SELECT DISTINCT c/name/value FROM COMPOSITION c ORDER BY c/name/value",
			want: clauseTexts{
				sel:     "SELECT DISTINCT c/name/value",
				from:    "FROM COMPOSITION c",
				orderBy: "ORDER BY c/name/value",
			},
		},
		{
			name: "select top",
			src:  "SELECT TOP 5 BACKWARD c/uid/value FROM COMPOSITION c WHERE c/name/value = 'x'",
			want: clauseTexts{
				sel:       "SELECT TOP 5 BACKWARD c/uid/value",
				from:      "FROM COMPOSITION c",
				where:     "WHERE c/name/value = 'x'",
				whereExpr: "c/name/value = 'x'",
			},
		},
		{
			name: "parenthesised where expression",
			src: "SELECT c/uid/value FROM COMPOSITION c " +
				"WHERE (c/name/value = 'a' OR c/name/value = 'b') AND c/uid/value = $uid ORDER BY c/uid/value",
			want: clauseTexts{
				sel:       "SELECT c/uid/value",
				from:      "FROM COMPOSITION c",
				where:     "WHERE (c/name/value = 'a' OR c/name/value = 'b') AND c/uid/value = $uid",
				whereExpr: "(c/name/value = 'a' OR c/name/value = 'b') AND c/uid/value = $uid",
				orderBy:   "ORDER BY c/uid/value",
			},
		},
		{
			name: "lower-case keywords",
			src: "select c/uid/value from COMPOSITION c where c/name/value = 'x' " +
				"order by c/uid/value desc limit 5 offset 10",
			want: clauseTexts{
				sel:       "select c/uid/value",
				from:      "from COMPOSITION c",
				where:     "where c/name/value = 'x'",
				whereExpr: "c/name/value = 'x'",
				orderBy:   "order by c/uid/value desc",
				limit:     "limit 5",
				offset:    "offset 10",
			},
		},
		{
			name: "multi-line text with a comment line",
			src: "\n  SELECT\n\tc/uid/value,\n\tc/name/value\nFROM EHR e\n  CONTAINS COMPOSITION c\n" +
				"-- one name only\nWHERE c/name/value = 'x'\n  AND c/uid/value = $uid\n" +
				"ORDER BY\n  c/uid/value\nLIMIT 10\nOFFSET 5\n",
			want: clauseTexts{
				sel:       "SELECT\n\tc/uid/value,\n\tc/name/value",
				from:      "FROM EHR e\n  CONTAINS COMPOSITION c",
				where:     "WHERE c/name/value = 'x'\n  AND c/uid/value = $uid",
				whereExpr: "c/name/value = 'x'\n  AND c/uid/value = $uid",
				orderBy:   "ORDER BY\n  c/uid/value",
				limit:     "LIMIT 10",
				offset:    "OFFSET 5",
			},
		},
		{
			name: "trailing whitespace and double dash",
			src:  "SELECT c/uid/value FROM COMPOSITION c LIMIT 5 OFFSET 2 \t--",
			want: clauseTexts{
				sel:    "SELECT c/uid/value",
				from:   "FROM COMPOSITION c",
				limit:  "LIMIT 5",
				offset: "OFFSET 2",
			},
		},
		{
			name: "non-ASCII literal before where",
			src: "SELECT 'Blutdruck – Ü' AS label, o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude " +
				"FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
				"WHERE o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude > 140 LIMIT 3 OFFSET 1",
			want: clauseTexts{
				sel:       "SELECT 'Blutdruck – Ü' AS label, o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude",
				from:      "FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
				where:     "WHERE o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude > 140",
				whereExpr: "o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude > 140",
				limit:     "LIMIT 3",
				offset:    "OFFSET 1",
			},
		},
		{
			name: "non-ASCII archetype predicate and literals inside clauses",
			src: "SELECT o/data[at0001, 'Größe – 🩺']/value FROM EHR e " +
				"CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.height.v2, 'Körpergröße'] " +
				"WHERE o/name/value = 'Körpergröße' ORDER BY o/name/value LIMIT 1 OFFSET 1",
			want: clauseTexts{
				sel:       "SELECT o/data[at0001, 'Größe – 🩺']/value",
				from:      "FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.height.v2, 'Körpergröße']",
				where:     "WHERE o/name/value = 'Körpergröße'",
				whereExpr: "o/name/value = 'Körpergröße'",
				orderBy:   "ORDER BY o/name/value",
				limit:     "LIMIT 1",
				offset:    "OFFSET 1",
			},
		},
		{
			// The parser reads an invalid byte as one replacement character,
			// three bytes when re-encoded; the spans must count the byte the
			// caller wrote.
			name: "invalid UTF-8 byte in a literal before where",
			src:  "SELECT 'a\xffÜ' AS label, c/uid/value FROM COMPOSITION c WHERE c/name/value = 'x' LIMIT 1",
			want: clauseTexts{
				sel:       "SELECT 'a\xffÜ' AS label, c/uid/value",
				from:      "FROM COMPOSITION c",
				where:     "WHERE c/name/value = 'x'",
				whereExpr: "c/name/value = 'x'",
				limit:     "LIMIT 1",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q, err := parse.ParseQuery(tc.src)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", tc.src, err)
			}
			s := q.Spans
			for _, c := range []struct {
				clause string
				got    parse.Offsets
				want   string
			}{
				{"Select", s.Select, tc.want.sel},
				{"From", s.From, tc.want.from},
				{"Where", s.Where, tc.want.where},
				{"WhereExpr", s.WhereExpr, tc.want.whereExpr},
				{"OrderBy", s.OrderBy, tc.want.orderBy},
				{"Limit", s.Limit, tc.want.limit},
				{"Offset", s.Offset, tc.want.offset},
			} {
				if c.want == "" {
					if !c.got.IsZero() {
						t.Errorf("Spans.%s = %+v, want zero: the clause is absent", c.clause, c.got)
					}
					continue
				}
				if c.got.IsZero() {
					t.Errorf("Spans.%s is zero, want the span of %q", c.clause, c.want)
					continue
				}
				if c.got.Start < 0 || c.got.Start > c.got.End || c.got.End > len(tc.src) {
					t.Errorf("Spans.%s = %+v, not a range inside the %d-byte source", c.clause, c.got, len(tc.src))
					continue
				}
				if text := tc.src[c.got.Start:c.got.End]; text != c.want {
					t.Errorf("src[Spans.%s] = %q, want %q", c.clause, text, c.want)
				}
			}

			doc, err := parse.Parse(tc.src)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.src, err)
			}
			if got := doc.Query().Spans; got != s {
				t.Errorf("Document.Query().Spans = %+v, want ParseQuery's %+v", got, s)
			}
		})
	}
}

func TestREQ113_OffsetsIsZero(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		o    parse.Offsets
		want bool
	}{
		{parse.Offsets{}, true},
		{parse.Offsets{Start: 0, End: 6}, false},
		{parse.Offsets{Start: 7, End: 9}, false},
	} {
		if got := tc.o.IsZero(); got != tc.want {
			t.Errorf("%+v.IsZero() = %t, want %t", tc.o, got, tc.want)
		}
	}
}

// TestREQ113_ClauseSpansIgnoredByEmit pins that the spans describe the source
// only: a Query built directly carries none, and Emit renders the same text
// whatever they hold.
func TestREQ113_ClauseSpansIgnoredByEmit(t *testing.T) {
	t.Parallel()
	built := &parse.Query{
		Select: parse.SelectClause{Star: true},
		From:   parse.FromClause{Root: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}},
	}
	if built.Spans != (parse.ClauseSpans{}) {
		t.Errorf("a directly built Query has Spans %+v, want zero", built.Spans)
	}

	const src = "SELECT c/uid/value FROM COMPOSITION c WHERE c/name/value = 'x' ORDER BY c/uid/value LIMIT 10 OFFSET 5"
	q, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", src, err)
	}
	want, err := q.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	q.Spans = parse.ClauseSpans{Select: parse.Offsets{Start: 90, End: 3}, Where: parse.Offsets{Start: 1, End: 2}}
	got, err := q.Emit()
	if err != nil {
		t.Fatalf("Emit with altered Spans: %v", err)
	}
	if got != want {
		t.Errorf("Emit with altered Spans = %q, want %q", got, want)
	}
}

// TestREQ113_ClauseSpansInsertWhereConjunct is the use case the spans serve:
// a consumer extends the WHERE condition in the source text, in front of the
// ORDER BY and LIMIT that follow it, and the conjunct lands inside WHERE.
func TestREQ113_ClauseSpansInsertWhereConjunct(t *testing.T) {
	t.Parallel()
	const (
		src = "SELECT c/uid/value FROM COMPOSITION c\n" +
			"WHERE c/name/value = 'Blutdruck – Ü'\n" +
			"ORDER BY c/uid/value DESC\nLIMIT 10 OFFSET 5"
		conjunct = " AND c/archetype_details/template_id/value = $tid"
		want     = "SELECT c/uid/value FROM COMPOSITION c " +
			"WHERE c/name/value = 'Blutdruck – Ü' AND c/archetype_details/template_id/value = $tid " +
			"ORDER BY c/uid/value DESC LIMIT 10 OFFSET 5"
	)
	q, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", src, err)
	}
	at := q.Spans.WhereExpr.End
	edited := src[:at] + conjunct + src[at:]

	got, err := parse.ParseQuery(edited)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", edited, err)
	}
	j, ok := got.Where.(aql.Junction)
	if !ok || j.Op != aql.OpAnd || len(j.Terms) != 2 {
		t.Fatalf("WHERE of %q = %#v, want an AND of two comparisons", edited, got.Where)
	}
	if c, ok := j.Terms[1].(aql.Comparison); !ok || c.Path != "c/archetype_details/template_id/value" {
		t.Errorf("second WHERE term of %q = %#v, want the inserted template_id comparison", edited, j.Terms[1])
	}
	if len(got.OrderBy) != 1 || got.Limit != (parse.IntLimit{N: 10}) || got.Offset != (parse.IntLimit{N: 5}) {
		t.Errorf("ORDER BY / LIMIT / OFFSET of %q = %+v / %#v / %#v, want one term, 10 and 5", edited, got.OrderBy, got.Limit, got.Offset)
	}

	gotText, err := got.Emit()
	if err != nil {
		t.Fatalf("Emit of %q: %v", edited, err)
	}
	wantQ, err := parse.ParseQuery(want)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", want, err)
	}
	wantText, err := wantQ.Emit()
	if err != nil {
		t.Fatalf("Emit of %q: %v", want, err)
	}
	if gotText != wantText {
		t.Errorf("edited query emits %q, want %q", gotText, wantText)
	}
}
