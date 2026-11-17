package parse_test

// class_predicate_test.go: REQ-113 § Structured node predicates at the class
// bracket and the alias-root bracket, which are the same `pathPredicate`
// production as a path step, and REQ-119's emission rules for the named
// archetype form those brackets carry.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// issue450Query is the reproduction from issue 450: a name with an escape at a
// path step, at a class bracket with an archetype HRID, and at a class bracket
// with a node id.
const issue450Query = `SELECT o/data[at0001, 'O\'Brien']/origin/value
  FROM EHR e CONTAINS COMPOSITION c
  CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1, 'O\'Brien']
  CONTAINS CLUSTER k[at0003, 'A\\B']`

const bloodPressure = "openEHR-EHR-OBSERVATION.blood_pressure.v1"

// classesOf returns the class expressions of q from both views: the flat lint
// view and the structured query, keyed by alias.
func classesOf(t *testing.T, q string) (flat, structured map[string]parse.ClassExpr) {
	t.Helper()
	doc, err := parse.Parse(q)
	if err != nil {
		t.Fatalf("Parse(%q) = %v", q, err)
	}
	flat = map[string]parse.ClassExpr{}
	for _, ce := range doc.Classes {
		flat[ce.Alias] = ce
	}
	pq, err := parse.ParseQuery(q)
	if err != nil {
		t.Fatalf("ParseQuery(%q) = %v", q, err)
	}
	structured = map[string]parse.ClassExpr{}
	var walk func(c *parse.Containment)
	walk = func(c *parse.Containment) {
		if c == nil {
			return
		}
		if c.Class.Alias != "" {
			structured[c.Class.Alias] = c.Class
		}
		for i := range c.Children {
			walk(&c.Children[i])
		}
	}
	if pq.From.Root.Alias != "" {
		structured[pq.From.Root.Alias] = pq.From.Root
	}
	walk(pq.From.Contains)
	walk(pq.From.Junction)
	return flat, structured
}

// TestREQ113_ClassBracketCarriesTheParsedPredicate is the issue 450
// reproduction at the class bracket: the name's escapes are resolved in the
// parsed form, as they are at a path step, and the named archetype form sets
// Archetype, in both views.
func TestREQ113_ClassBracketCarriesTheParsedPredicate(t *testing.T) {
	t.Parallel()
	flat, structured := classesOf(t, issue450Query)

	// The same name at the path step and at the class bracket is the same
	// value, so a consumer matches the same node in both places.
	doc, err := parse.Parse(issue450Query)
	if err != nil {
		t.Fatal(err)
	}
	step, ok := doc.Paths[0].Segments[0].Parsed.(aql.NodeIDPredicate)
	if !ok || step.Name == nil {
		t.Fatalf("path step parsed as %+v; want a named node id", doc.Paths[0].Segments[0].Parsed)
	}
	if named, isArchetype := flat["o"].ParsedPredicate.(aql.ArchetypePredicate); !isArchetype ||
		named.Name == nil || named.Name.Text != step.Name.Text {
		t.Errorf("class o name = %+v; want the path step's name %q", flat["o"].ParsedPredicate, step.Name.Text)
	}

	views := []struct {
		name    string
		classes map[string]parse.ClassExpr
	}{{"Parse", flat}, {"ParseQuery", structured}}
	for _, v := range views {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()
			o := v.classes["o"]
			if o.Archetype != bloodPressure || o.ParamArchetype {
				t.Errorf("class o: Archetype = %q, ParamArchetype = %v; want %q, false: "+
					"the named form names the archetype too", o.Archetype, o.ParamArchetype, bloodPressure)
			}
			if want := bloodPressure + `, 'O\'Brien'`; o.Predicate != want {
				t.Errorf("class o: Predicate = %q; want the verbatim bracket text %q", o.Predicate, want)
			}
			want := aql.ArchetypePredicate{
				HRID: bloodPressure,
				Name: &aql.PredicateName{Kind: aql.NameString, Text: "O'Brien"},
			}
			if !aql.EqualPredicates(o.ParsedPredicate, want) {
				t.Errorf("class o: ParsedPredicate = %+v; want %+v, escapes resolved", o.ParsedPredicate, want)
			}

			k := v.classes["k"]
			if k.Archetype != "" {
				t.Errorf("class k: Archetype = %q; want empty for a node-id bracket", k.Archetype)
			}
			wantK := aql.NodeIDPredicate{
				ID:   "at0003",
				Name: &aql.PredicateName{Kind: aql.NameString, Text: `A\B`},
			}
			if !aql.EqualPredicates(k.ParsedPredicate, wantK) {
				t.Errorf("class k: ParsedPredicate = %+v; want %+v, escapes resolved", k.ParsedPredicate, wantK)
			}

			for _, c := range []string{"e", "c"} {
				if p := v.classes[c].ParsedPredicate; p != nil {
					t.Errorf("class %s carries no bracket; ParsedPredicate = %+v, want nil", c, p)
				}
			}
		})
	}
}

// TestREQ113_AliasRootBracketCarriesTheParsedPredicate is the alias-root half
// of the reproduction: `o[...]/data` gets the same parsed form as a path step,
// from both extractors.
func TestREQ113_AliasRootBracketCarriesTheParsedPredicate(t *testing.T) {
	t.Parallel()
	const q = `SELECT o[at0002, 'X\'Y']/data FROM OBSERVATION o WHERE o[at0002, 'X\'Y']/data/value = 1`
	want := aql.NodeIDPredicate{
		ID:   "at0002",
		Name: &aql.PredicateName{Kind: aql.NameString, Text: "X'Y"},
	}

	doc, err := parse.Parse(q)
	if err != nil {
		t.Fatalf("Parse(%q) = %v", q, err)
	}
	if len(doc.Paths) != 2 {
		t.Fatalf("Parse: %d paths; want 2", len(doc.Paths))
	}
	for _, ip := range doc.Paths {
		if ip.Predicate != `at0002, 'X\'Y'` {
			t.Errorf("Parse %v path: Predicate = %q; want the verbatim text", ip.Clause, ip.Predicate)
		}
		if !aql.EqualPredicates(ip.ParsedPredicate, want) {
			t.Errorf("Parse %v path: ParsedPredicate = %+v; want %+v", ip.Clause, ip.ParsedPredicate, want)
		}
	}

	pq, err := parse.ParseQuery(q)
	if err != nil {
		t.Fatalf("ParseQuery(%q) = %v", q, err)
	}
	pe, ok := pq.Select.Items[0].Expr.(parse.PathExpr)
	if !ok {
		t.Fatalf("SELECT item is %T; want parse.PathExpr", pq.Select.Items[0].Expr)
	}
	if !aql.EqualPredicates(pe.ParsedPredicate, want) {
		t.Errorf("ParseQuery SELECT path: ParsedPredicate = %+v; want %+v", pe.ParsedPredicate, want)
	}
	cmp, ok := pq.Where.(aql.Comparison)
	if !ok || cmp.ParsedPath == nil {
		t.Fatalf("WHERE is %T with ParsedPath %v; want a comparison carrying its parsed path", pq.Where, cmp.ParsedPath)
	}
	if !aql.EqualPredicates(cmp.ParsedPath.ParsedPredicate, want) {
		t.Errorf("ParseQuery WHERE path: ParsedPredicate = %+v; want %+v", cmp.ParsedPath.ParsedPredicate, want)
	}

	// No bracket on the alias root: nil, not an empty structure.
	plain, err := parse.Parse("SELECT o/data FROM OBSERVATION o")
	if err != nil {
		t.Fatal(err)
	}
	if p := plain.Paths[0].ParsedPredicate; p != nil {
		t.Errorf("an alias root with no bracket has ParsedPredicate %+v; want nil", p)
	}
}

// TestREQ113_VersionClassCarriesNoParsedPredicate pins the VERSION exclusion:
// its bracket is `versionPredicate`, not `pathPredicate`, so even a standing
// comparison there is not structured into the model.
func TestREQ113_VersionClassCarriesNoParsedPredicate(t *testing.T) {
	t.Parallel()
	for _, q := range []string{
		"SELECT v/commit_audit FROM VERSION v[LATEST_VERSION]",
		"SELECT v/commit_audit FROM VERSION v[commit_audit/time_committed/value > '2020-01-01']",
	} {
		flat, structured := classesOf(t, q)
		for view, classes := range map[string]map[string]parse.ClassExpr{"Parse": flat, "ParseQuery": structured} {
			if p := classes["v"].ParsedPredicate; p != nil {
				t.Errorf("%s(%q): VERSION class ParsedPredicate = %+v; want nil", view, q, p)
			}
		}
	}
}
