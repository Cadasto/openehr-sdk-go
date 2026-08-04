package parse_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// representativeQuery exercises every structured-extraction surface: an EHR
// root with an identifying predicate ($ehr param), two archetype-bound
// containments, a parameterised WHERE, and identified paths across the SELECT,
// WHERE, and ORDER BY clauses.
const representativeQuery = "SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude AS sys " +
	"FROM EHR e[ehr_id/value=$ehr] " +
	"CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1] " +
	"CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
	"WHERE o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude > $threshold " +
	"ORDER BY c/uid/value"

func TestExtractClasses(t *testing.T) {
	doc, err := parse.Parse(representativeQuery)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Classes) != 3 {
		t.Fatalf("Classes = %d, want 3: %+v", len(doc.Classes), doc.Classes)
	}

	ehr := doc.Classes[0]
	if ehr.RMType != "EHR" || ehr.Alias != "e" || ehr.Archetype != "" || !ehr.HasPredicate {
		t.Errorf("EHR class = %+v", ehr)
	}
	comp := doc.Classes[1]
	if comp.RMType != "COMPOSITION" || comp.Alias != "c" ||
		comp.Archetype != "openEHR-EHR-COMPOSITION.encounter.v1" {
		t.Errorf("COMPOSITION class = %+v", comp)
	}
	obs := doc.Classes[2]
	if obs.RMType != "OBSERVATION" || obs.Alias != "o" ||
		obs.Archetype != "openEHR-EHR-OBSERVATION.blood_pressure.v1" {
		t.Errorf("OBSERVATION class = %+v", obs)
	}
}

func TestExtractPaths(t *testing.T) {
	doc, err := parse.Parse(representativeQuery)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Paths) != 3 {
		t.Fatalf("Paths = %d, want 3: %+v", len(doc.Paths), doc.Paths)
	}

	sel := doc.Paths[0]
	if sel.Alias != "o" || sel.Clause != parse.ClauseSelect {
		t.Errorf("select path alias/clause = %q/%v", sel.Alias, sel.Clause)
	}
	wantSegs := []struct{ name, pred string }{
		{"data", "at0001"},
		{"events", "at0006"},
		{"data", "at0003"},
		{"items", "at0004"},
		{"value", ""},
		{"magnitude", ""},
	}
	if len(sel.Segments) != len(wantSegs) {
		t.Fatalf("select segments = %d, want %d: %+v", len(sel.Segments), len(wantSegs), sel.Segments)
	}
	for i, w := range wantSegs {
		if sel.Segments[i].Name != w.name || sel.Segments[i].Predicate != w.pred {
			t.Errorf("segment %d = %+v, want {%q %q}", i, sel.Segments[i], w.name, w.pred)
		}
	}

	if doc.Paths[1].Clause != parse.ClauseWhere {
		t.Errorf("where path clause = %v", doc.Paths[1].Clause)
	}
	if got := doc.Paths[2]; got.Clause != parse.ClauseOrderBy || got.Alias != "c" {
		t.Errorf("order-by path = %+v", got)
	}
}

// TestExtractPathsSkipsBooleanKeywordOperand pins REQ-117: a bare
// `true` / `false` in a comparison-terminal position is a boolean literal, not
// a path, even though the SDK lexer hands it to the flat extractor as an
// IDENTIFIER-only identifiedPath (the IDENTIFIER rule precedes BOOLEAN in
// AqlLexer.g4). The flat view must agree with the structured extractor, which
// lifts the same shape to an aql.BoolValue.
func TestExtractPathsSkipsBooleanKeywordOperand(t *testing.T) {
	for _, q := range []string{
		"SELECT s/is_queryable FROM COMPOSITION s WHERE s/is_queryable = true",
		"SELECT s/is_queryable FROM COMPOSITION s WHERE s/is_queryable != FALSE",
	} {
		doc, err := parse.Parse(q)
		if err != nil {
			t.Fatalf("parse %q: %v", q, err)
		}
		// The two real paths are the SELECT and WHERE `s/is_queryable`.
		if len(doc.Paths) != 2 {
			t.Errorf("%q: Paths = %d, want 2: %+v", q, len(doc.Paths), doc.Paths)
		}
		for _, p := range doc.Paths {
			if p.Alias != "s" {
				t.Errorf("%q: unexpected path alias %q (%+v)", q, p.Alias, p)
			}
		}
	}

	// A path rooted at an identifier that merely CONTAINS a keyword, or a
	// keyword carrying a path tail, is a real path and stays recorded.
	doc, err := parse.Parse("SELECT s/x FROM COMPOSITION s WHERE s/x = true/nested")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Paths) != 3 {
		t.Errorf("keyword with a path tail must stay a path: Paths = %d, want 3: %+v",
			len(doc.Paths), doc.Paths)
	}
}

// TestExtractPathsSkipsBooleanKeywordSelectItem is the SELECT-side sibling of
// [TestExtractPathsSkipsBooleanKeywordOperand]: a bare `true` / `false`
// projected from SELECT is a literal, not a path root, so it contributes no
// entry to Document.Paths and the lint layer raises no aql_unknown_alias for
// it. A keyword carrying a path tail stays a path in this view too.
func TestExtractPathsSkipsBooleanKeywordSelectItem(t *testing.T) {
	for _, q := range []string{
		"SELECT true FROM EHR e",
		"SELECT FALSE FROM EHR e",
		"SELECT true, e/ehr_id/value FROM EHR e",
	} {
		doc, err := parse.Parse(q)
		if err != nil {
			t.Fatalf("parse %q: %v", q, err)
		}
		for _, p := range doc.Paths {
			if p.Alias == "true" || p.Alias == "false" || p.Alias == "FALSE" {
				t.Errorf("%q: keyword literal recorded as a path root: %+v", q, p)
			}
		}
	}

	// A keyword with a path tail in a SELECT column position is a real path.
	doc, err := parse.Parse("SELECT true/nested FROM EHR e")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Paths) != 1 || doc.Paths[0].Alias != "true" {
		t.Errorf("keyword with a path tail must stay a path: Paths = %+v", doc.Paths)
	}
}

func TestExtractParams(t *testing.T) {
	doc, err := parse.Parse(representativeQuery)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ehr", "threshold"}
	if len(doc.Params) != len(want) {
		t.Fatalf("Params = %v, want %v", doc.Params, want)
	}
	for i, w := range want {
		if doc.Params[i] != w {
			t.Errorf("Params[%d] = %q, want %q", i, doc.Params[i], w)
		}
	}
}

func TestExtractVersionClass(t *testing.T) {
	doc, err := parse.Parse(
		"SELECT v FROM EHR e CONTAINS VERSION v[all_versions] CONTAINS COMPOSITION c",
	)
	if err != nil {
		t.Fatal(err)
	}
	var ver *parse.ClassExpr
	for i := range doc.Classes {
		if doc.Classes[i].Version {
			ver = &doc.Classes[i]
		}
	}
	if ver == nil {
		t.Fatal("no VERSION class extracted")
	}
	if ver.RMType != "VERSION" || ver.Alias != "v" || !ver.HasPredicate {
		t.Errorf("VERSION class = %+v", *ver)
	}
}

// TestExtractParamArchetype covers a $param standing in for an archetype HRID
// in a containment predicate (SDK admits identifiable-by-param scope).
// Archetype carries the source placeholder verbatim (with the leading `$`)
// so emission can round-trip the exact placeholder name; ParamArchetype
// stays as the typed signal — aligned with the structured extractor
// (REQ-113 review).
func TestExtractParamArchetype(t *testing.T) {
	doc, err := parse.Parse("SELECT c FROM COMPOSITION c[$arch]")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Classes) != 1 {
		t.Fatalf("Classes = %d, want 1", len(doc.Classes))
	}
	c := doc.Classes[0]
	if !c.ParamArchetype || c.Archetype != "$arch" {
		t.Errorf("class = %+v, want ParamArchetype=true Archetype=$arch", c)
	}
	if len(doc.Params) != 1 || doc.Params[0] != "arch" {
		t.Errorf("Params = %v, want [arch]", doc.Params)
	}
}
