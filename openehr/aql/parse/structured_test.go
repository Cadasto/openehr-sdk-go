package parse_test

// structured_test.go: PROBE-082 — REQ-113. The parser must
// expose the two path-bearing sub-structures as parsed structure, not only
// raw text: a class standing predicate as a {path, op, value} comparison,
// and the alias-qualified path of a WHERE comparison, EXISTS, LIKE or
// MATCHES as alias + segments — so a
// consumer reads them without re-tokenizing AQL grammar. Round-trip/emit is
// unaffected (the verbatim predicate/path text remains).

import (
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// standingPredicateQuery exercises both asks in one parse: a standing class predicate on
// the EHR root (`ehr_id/value=$ehr`) and a WHERE comparison over an
// alias-qualified path (`o/data[at0001]/events[at0006]/value/magnitude`).
const standingPredicateQuery = "SELECT o/data[at0001]/events[at0006]/value/magnitude " +
	"FROM EHR e[ehr_id/value=$ehr] " +
	"CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
	"WHERE o/data[at0001]/events[at0006]/value/magnitude > $threshold"

// TestStandingPredicateStructured is the PROBE-082 Ask #1 case: a class
// standing predicate is readable as a structured comparison, with the
// verbatim text retained for round-trip.
func TestStandingPredicateStructured(t *testing.T) {
	q, err := parse.ParseQuery(standingPredicateQuery)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	root := q.From.Root
	if root.PredicateComparison == nil {
		t.Fatalf("EHR standing predicate not structured; Predicate=%q", root.Predicate)
	}
	pc := root.PredicateComparison
	if pc.Path != "ehr_id/value" {
		t.Errorf("PredicateComparison.Path = %q, want ehr_id/value", pc.Path)
	}
	if pc.Op != aql.OpEq {
		t.Errorf("PredicateComparison.Op = %q, want =", pc.Op)
	}
	pv, ok := pc.Val.(aql.ParamValue)
	if !ok || pv.Name != "ehr" {
		t.Errorf("PredicateComparison.Val = %#v, want ParamValue{ehr}", pc.Val)
	}
	if root.Predicate == "" {
		t.Errorf("verbatim Predicate text must remain for round-trip, got empty")
	}
}

// TestWhereComparisonStructuredPath is the Ask #2 case: a WHERE comparison
// carries the structured alias + segments alongside the raw path string.
func TestWhereComparisonStructuredPath(t *testing.T) {
	q, err := parse.ParseQuery(standingPredicateQuery)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	cmp, ok := q.Where.(aql.Comparison)
	if !ok {
		t.Fatalf("Where = %T, want aql.Comparison", q.Where)
	}
	if cmp.ParsedPath == nil {
		t.Fatalf("WHERE comparison ParsedPath nil; Path=%q", cmp.Path)
	}
	pp := cmp.ParsedPath
	if pp.Alias != "o" {
		t.Errorf("ParsedPath.Alias = %q, want o", pp.Alias)
	}
	if len(pp.Segments) != 4 {
		t.Fatalf("ParsedPath.Segments len = %d, want 4 (%+v)", len(pp.Segments), pp.Segments)
	}
	if got := pp.Segments[0]; got.Name != "data" || got.Predicate != "at0001" {
		t.Errorf("Segments[0] = %+v, want {data at0001}", got)
	}
	if last := pp.Segments[3]; last.Name != "magnitude" {
		t.Errorf("Segments[3].Name = %q, want magnitude", last.Name)
	}
	if cmp.Path != pp.Raw {
		t.Errorf("Comparison.Path %q != ParsedPath.Raw %q (must agree)", cmp.Path, pp.Raw)
	}
}

// TestStandingPredicateParsedPath is the REQ-113 completeness case: a class
// standing predicate's relative left-hand path is exposed as structured
// segments with an empty Alias (a relative predicate path binds no FROM
// alias), so a consumer reads the segments without re-splitting the raw
// Comparison.Path string — the WHERE-side symmetry for the class-predicate LHS.
func TestStandingPredicateParsedPath(t *testing.T) {
	q, err := parse.ParseQuery(standingPredicateQuery)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	pc := q.From.Root.PredicateComparison
	if pc == nil {
		t.Fatalf("EHR standing predicate not structured; Predicate=%q", q.From.Root.Predicate)
	}
	if pc.ParsedPath == nil {
		t.Fatalf("class-predicate ParsedPath nil; Path=%q", pc.Path)
	}
	pp := pc.ParsedPath
	if pp.Alias != "" {
		t.Errorf("ParsedPath.Alias = %q, want empty (a relative predicate path binds no alias)", pp.Alias)
	}
	if len(pp.Segments) != 2 {
		t.Fatalf("ParsedPath.Segments len = %d, want 2 (%+v)", len(pp.Segments), pp.Segments)
	}
	if got := pp.Segments[0]; got.Name != "ehr_id" || got.Predicate != "" {
		t.Errorf("Segments[0] = %+v, want {ehr_id}", got)
	}
	if got := pp.Segments[1]; got.Name != "value" || got.Predicate != "" {
		t.Errorf("Segments[1] = %+v, want {value}", got)
	}
	if pc.Path != pp.Raw {
		t.Errorf("Comparison.Path %q != ParsedPath.Raw %q (must agree)", pc.Path, pp.Raw)
	}
}

// TestClassArchetypePredicateNotComparison confirms a non-comparison class
// predicate (an archetype HRID) is distinguishable: PredicateComparison is
// nil and the HRID lives on Archetype.
func TestClassArchetypePredicateNotComparison(t *testing.T) {
	q, err := parse.ParseQuery("SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	root := q.From.Root
	if root.Archetype != "openEHR-EHR-OBSERVATION.blood_pressure.v1" {
		t.Errorf("Archetype = %q, want the HRID", root.Archetype)
	}
	if root.PredicateComparison != nil {
		t.Errorf("archetype predicate must not yield a comparison; got %#v", root.PredicateComparison)
	}
}

// TestStandingPredicateRoundTrip confirms Ask #1/#2 are additive: emit still
// round-trips, the standing predicate survives a parse→emit→parse cycle, and
// emission is idempotent.
func TestStandingPredicateRoundTrip(t *testing.T) {
	q, err := parse.ParseQuery(standingPredicateQuery)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	emitted, err := q.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	q2, err := parse.ParseQuery(emitted)
	if err != nil {
		t.Fatalf("re-parse emitted %q: %v", emitted, err)
	}
	if q2.From.Root.PredicateComparison == nil {
		t.Errorf("standing predicate lost on round-trip; emitted=%q", emitted)
	}
	emitted2, err := q2.Emit()
	if err != nil {
		t.Fatalf("re-emit: %v", err)
	}
	if emitted != emitted2 {
		t.Errorf("emit not idempotent:\n  %q\n  %q", emitted, emitted2)
	}
}

// TestStandingPredicateLiteralValue exercises the literal-RHS operand branch
// (`primitiveAsValue`), distinct from the `$param` branch above: a standing
// predicate with a string literal yields a structured `aql.StringValue`.
func TestStandingPredicateLiteralValue(t *testing.T) {
	q, err := parse.ParseQuery("SELECT c FROM COMPOSITION c[name/value='Vital signs']")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	pc := q.From.Root.PredicateComparison
	if pc == nil {
		t.Fatalf("literal standing predicate not structured; Predicate=%q", q.From.Root.Predicate)
	}
	if pc.Path != "name/value" || pc.Op != aql.OpEq {
		t.Errorf("PredicateComparison = {%q %q}, want {name/value =}", pc.Path, pc.Op)
	}
	sv, ok := pc.Val.(aql.StringValue)
	if !ok || sv.S != "Vital signs" {
		t.Errorf("PredicateComparison.Val = %#v, want StringValue{Vital signs}", pc.Val)
	}
	if pc.ParsedPath == nil || len(pc.ParsedPath.Segments) != 2 ||
		pc.ParsedPath.Segments[0].Name != "name" || pc.ParsedPath.Segments[1].Name != "value" {
		t.Errorf("PredicateComparison.ParsedPath = %+v, want segments [name value]", pc.ParsedPath)
	}
}

// pathCarrier reads the as-written path and the structured path of a WHERE
// form whose operand is an identified path, or reports false for any other
// form.
func pathCarrier(w aql.WhereExpr) (path string, parsed *aql.IdentifiedPath, ok bool) {
	switch x := w.(type) {
	case aql.Comparison:
		return x.Path, x.ParsedPath, true
	case aql.ExistsExpr:
		return x.Path, x.ParsedPath, true
	case aql.LikeExpr:
		return x.Path, x.ParsedPath, true
	case aql.MatchesExpr:
		return x.Path, x.ParsedPath, true
	}
	return "", nil, false
}

// TestWherePathCarriersParsedPath is the PROBE-082 case for the other WHERE
// forms over an identified path (REQ-113 § Structured path access): EXISTS,
// LIKE and each MATCHES operand form carry the structured path beside Path,
// equal to a comparison's over the same path text. The node name keeps its
// escape in Path and is resolved in the segment, and inter-part whitespace
// stays in Path but not in the segments.
func TestWherePathCarriersParsedPath(t *testing.T) {
	const (
		named   = `c/content[at0001, 'O\'Brien']/name/value`
		spaced  = `c / content[at0001, 'O\'Brien'] / name / value`
		content = `c/content[at0001, 'O\'Brien']`
	)
	full := []string{"content", "name", "value"}
	cases := []struct {
		name  string
		path  string
		form  string
		want  aql.WhereExpr // the concrete shape, for the type check
		names []string
	}{
		{name: "EXISTS", path: content, form: "EXISTS " + content, want: aql.ExistsExpr{}, names: []string{"content"}},
		{name: "EXISTS spaced", path: spaced, form: "EXISTS " + spaced, want: aql.ExistsExpr{}, names: full},
		{name: "LIKE", path: named, form: named + " LIKE 'x%'", want: aql.LikeExpr{}, names: full},
		{name: "LIKE spaced", path: spaced, form: spaced + " LIKE $p", want: aql.LikeExpr{}, names: full},
		{name: "MATCHES value list", path: named, form: named + " MATCHES {'x'}", want: aql.MatchesExpr{}, names: full},
		{name: "MATCHES spaced", path: spaced, form: spaced + " MATCHES {'x', $p}", want: aql.MatchesExpr{}, names: full},
		{
			name: "MATCHES terminology", path: named,
			form: named + " MATCHES TERMINOLOGY('SNOMED-CT','near','12345')",
			want: aql.MatchesExpr{}, names: full,
		},
		{
			name: "MATCHES URI", path: named,
			form: named + " MATCHES {uri://terminology.hl7.org/CodeSystem/v3-ActCode}",
			want: aql.MatchesExpr{}, names: full,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "SELECT c/uid/value FROM COMPOSITION c WHERE " + tc.path + " = 'x' AND " + tc.form
			q, err := parse.ParseQuery(src)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", src, err)
			}
			j, ok := q.Where.(aql.Junction)
			if !ok || len(j.Terms) != 2 {
				t.Fatalf("Where = %#v, want a two-term junction", q.Where)
			}
			_, cmpParsed, _ := pathCarrier(j.Terms[0])
			if cmpParsed == nil {
				t.Fatalf("comparison ParsedPath nil in %q", src)
			}
			if got, want := reflect.TypeOf(j.Terms[1]), reflect.TypeOf(tc.want); got != want {
				t.Fatalf("second term = %v, want %v", got, want)
			}
			path, pp, _ := pathCarrier(j.Terms[1])
			if pp == nil {
				t.Fatalf("%s ParsedPath nil; Path=%q", tc.name, path)
			}
			if path != tc.path {
				t.Errorf("Path = %q, want the source spelling %q", path, tc.path)
			}
			if pp.Raw != path {
				t.Errorf("ParsedPath.Raw = %q, want Path %q", pp.Raw, path)
			}
			if pp.Alias != "c" {
				t.Errorf("ParsedPath.Alias = %q, want c", pp.Alias)
			}
			var names []string
			for _, s := range pp.Segments {
				names = append(names, s.Name)
			}
			if !reflect.DeepEqual(names, tc.names) {
				t.Errorf("segment names = %q, want %q", names, tc.names)
			}
			if !reflect.DeepEqual(*pp, *cmpParsed) {
				t.Errorf("ParsedPath = %+v, want the comparison's %+v", *pp, *cmpParsed)
			}
			if len(pp.Segments) == 0 {
				t.Fatal("no segments")
			}
			np, ok := pp.Segments[0].Parsed.(aql.NodeIDPredicate)
			if !ok {
				t.Fatalf("content segment Parsed = %#v, want aql.NodeIDPredicate", pp.Segments[0].Parsed)
			}
			want := aql.PredicateName{Kind: aql.NameString, Text: "O'Brien"}
			if np.ID != "at0001" || np.Name == nil || *np.Name != want {
				t.Errorf("content predicate = {%q %+v}, want {at0001 %+v}", np.ID, np.Name, want)
			}
		})
	}
}

// TestWherePathCarriersWriteSideNil pins REQ-113 § Structured path access on
// the write side: the constructors set only Path, never ParsedPath.
func TestWherePathCarriersWriteSideNil(t *testing.T) {
	built := map[string]aql.WhereExpr{
		"Exists":             aql.Exists("c/name/value"),
		"Like":               aql.Like("c/name/value", aql.StringValue{S: "x%"}),
		"Matches":            aql.Matches("c/name/value", aql.StringValue{S: "x"}),
		"MatchesTerminology": aql.MatchesTerminology("c/name/value", "SNOMED-CT", "near", "12345"),
		"MatchesURI":         aql.MatchesURI("c/name/value", "uri://example.org/x"),
	}
	for name, w := range built {
		path, pp, ok := pathCarrier(w)
		if !ok {
			t.Errorf("%s built %T, want a path-carrying WHERE form", name, w)
			continue
		}
		if path != "c/name/value" || pp != nil {
			t.Errorf("%s: Path = %q, ParsedPath = %+v; want c/name/value and nil", name, path, pp)
		}
	}
}

// TestWherePathCarriersEmitIgnoresParsedPath pins § Emission ignores the
// structured model for the four WHERE path carriers (REQ-113, PROBE-082):
// Emit gives the same text whether ParsedPath is as parsed, nil, or wrong.
func TestWherePathCarriersEmitIgnoresParsedPath(t *testing.T) {
	const src = `SELECT c/uid/value FROM COMPOSITION c
  WHERE c/content[at0001, 'O\'Brien']/name/value = 'x'
    AND EXISTS c/content[at0001, 'O\'Brien']
    AND c/content[at0001, 'O\'Brien']/name/value LIKE 'x%'
    AND c/content[at0001, 'O\'Brien']/name/value MATCHES {'x'}`
	q, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	want, err := q.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	j, ok := q.Where.(aql.Junction)
	if !ok || len(j.Terms) != 4 {
		t.Fatalf("Where = %#v, want a four-term junction", q.Where)
	}
	for i, w := range j.Terms {
		if _, pp, ok := pathCarrier(w); !ok || pp == nil {
			t.Fatalf("term %d = %#v, want a path carrier with ParsedPath set", i, w)
		}
	}
	wrong := &aql.IdentifiedPath{Alias: "z", Segments: []aql.PathSegment{{Name: "other"}}, Raw: "z/other"}
	for _, label := range []string{"nil", "wrong"} {
		var pp *aql.IdentifiedPath
		if label == "wrong" {
			pp = wrong
		}
		terms := make([]aql.WhereExpr, len(j.Terms))
		for i, w := range j.Terms {
			switch x := w.(type) {
			case aql.Comparison:
				x.ParsedPath = pp
				terms[i] = x
			case aql.ExistsExpr:
				x.ParsedPath = pp
				terms[i] = x
			case aql.LikeExpr:
				x.ParsedPath = pp
				terms[i] = x
			case aql.MatchesExpr:
				x.ParsedPath = pp
				terms[i] = x
			}
		}
		cp := *q
		cp.Where = aql.Junction{Op: j.Op, Terms: terms}
		got, err := cp.Emit()
		if err != nil {
			t.Fatalf("Emit with ParsedPath %s: %v", label, err)
		}
		if got != want {
			t.Errorf("Emit with ParsedPath %s = %q, want %q", label, got, want)
		}
	}
}
