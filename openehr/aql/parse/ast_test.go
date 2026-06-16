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
func TestExtractParamArchetype(t *testing.T) {
	doc, err := parse.Parse("SELECT c FROM COMPOSITION c[$arch]")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Classes) != 1 {
		t.Fatalf("Classes = %d, want 1", len(doc.Classes))
	}
	c := doc.Classes[0]
	if !c.ParamArchetype || c.Archetype != "" {
		t.Errorf("class = %+v, want ParamArchetype with empty Archetype", c)
	}
	if len(doc.Params) != 1 || doc.Params[0] != "arch" {
		t.Errorf("Params = %v, want [arch]", doc.Params)
	}
}
