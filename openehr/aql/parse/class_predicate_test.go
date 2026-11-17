package parse_test

// class_predicate_test.go: REQ-113 § Structured node predicates at the class
// bracket and the alias-root bracket, which are the same `pathPredicate`
// production as a path step, and REQ-119's emission rules for the named
// archetype form those brackets carry.

import (
	"errors"
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

// emitClass emits `SELECT o/data FROM <class>` with the FROM root replaced by
// class: the direct-construction path a consumer reaches by assembling or
// rewriting an AST by hand, which is what the REQ-119 emission guards bind.
func emitClass(t *testing.T, class parse.ClassExpr) (string, error) {
	t.Helper()
	q, err := parse.ParseQuery("SELECT o/data FROM OBSERVATION o")
	if err != nil {
		t.Fatal(err)
	}
	q.From.Root = class
	return q.Emit()
}

// TestREQ119_NamedArchetypeFormEmits pins the one Archetype-and-Predicate
// pair Emit accepts: the named archetype form ParseQuery produces, with every
// name spelling the slot admits and with trivia inside the bracket. The
// emitted text is the verbatim Predicate, and it reads back as the same pair.
func TestREQ119_NamedArchetypeFormEmits(t *testing.T) {
	t.Parallel()
	for _, pred := range []string{
		bloodPressure + ", 'Blood pressure'",
		bloodPressure + `, 'O\'Brien'`,
		bloodPressure + ", $name",
		bloodPressure + ", SNOMED-CT::271649006|systolic|",
		bloodPressure + ", at0004",
		bloodPressure + ", id4",
		"  " + bloodPressure + " -- the archetype\n ,  'x' ",
	} {
		t.Run(pred, func(t *testing.T) {
			t.Parallel()
			src := "SELECT o/data FROM OBSERVATION o[" + pred + "]"
			pq, err := parse.ParseQuery(src)
			if err != nil {
				t.Fatalf("ParseQuery(%q) = %v", src, err)
			}
			if pq.From.Root.Archetype != bloodPressure || pq.From.Root.Predicate != pred {
				t.Fatalf("parsed Archetype %q, Predicate %q; want %q beside the verbatim text",
					pq.From.Root.Archetype, pq.From.Root.Predicate, bloodPressure)
			}
			out, err := pq.Emit()
			if err != nil {
				t.Fatalf("Emit refused the named form ParseQuery produced: %v", err)
			}
			if want := "SELECT o/data FROM OBSERVATION o[" + pred + "]"; out != want {
				t.Errorf("Emit = %q; want %q, the verbatim Predicate", out, want)
			}

			// The same pair, built by hand, emits the same text.
			built, err := emitClass(t, parse.ClassExpr{
				RMType: "OBSERVATION", Alias: "o", Archetype: bloodPressure, Predicate: pred,
			})
			if err != nil || built != out {
				t.Errorf("hand-built pair: Emit = %q, %v; want %q", built, err, out)
			}
		})
	}
}

// TestREQ119_NamedFormWithoutArchetypeStillEmits keeps the spelling ParseQuery
// produced before Archetype carried the named form's HRID: Predicate alone.
// It must still emit, and the verification that re-reads the emitted text
// must not take the HRID it now finds on Archetype for a different query.
func TestREQ119_NamedFormWithoutArchetypeStillEmits(t *testing.T) {
	t.Parallel()
	pred := bloodPressure + ", 'Blood pressure'"
	out, err := emitClass(t, parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Predicate: pred})
	if err != nil {
		t.Fatalf("Emit refused a named form carried on Predicate alone: %v", err)
	}
	if want := "SELECT o/data FROM OBSERVATION o[" + pred + "]"; out != want {
		t.Errorf("Emit = %q; want %q", out, want)
	}
}

// TestREQ119_ArchetypeBesideAnotherPredicateIsRefused pins every pair Emit
// still refuses. Emission renders Predicate when it is set, so a pair other
// than the archetype's own named form would drop the archetype, or emit a
// different one, with err == nil.
func TestREQ119_ArchetypeBesideAnotherPredicateIsRefused(t *testing.T) {
	t.Parallel()
	const labResult = "openEHR-EHR-OBSERVATION.lab_result.v1"
	named := bloodPressure + ", 'x'"
	cases := []struct {
		name  string
		class parse.ClassExpr
	}{
		// The named form of a different archetype: emission would query
		// lab_result's name under blood_pressure's Archetype.
		{"mismatched HRID", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: labResult, Predicate: named}},
		// Text that is not the named form at all.
		{"standing predicate", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: bloodPressure, Predicate: "name/value='x'"}},
		{"bare hrid, no name", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: bloodPressure, Predicate: bloodPressure}},
		{"node id with a name", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: bloodPressure, Predicate: "at0001, 'x'"}},
		{"named form in a junction", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: bloodPressure, Predicate: named + " and at0001"}},
		{"second name", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: bloodPressure, Predicate: named + ", 'y'"}},
		{"no comma", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: bloodPressure, Predicate: bloodPressure + " 'x'"}},
		{"path in the name slot", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: bloodPressure, Predicate: bloodPressure + ", name/value"}},
		{"unterminated name", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: bloodPressure, Predicate: bloodPressure + ", 'x"}},
		// The named form beside a $param archetype or on a VERSION class.
		{"$param archetype", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: bloodPressure, ParamArchetype: true, Predicate: named}},
		{"$param placeholder", parse.ClassExpr{RMType: "OBSERVATION", Alias: "o", Archetype: "$arch", ParamArchetype: true, Predicate: "$arch, 'x'"}},
		{"VERSION class", parse.ClassExpr{RMType: "VERSION", Version: true, Alias: "o", Archetype: bloodPressure, Predicate: named}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := emitClass(t, tc.class)
			if err == nil {
				t.Fatalf("Emit = %q with err == nil; Archetype %q beside Predicate %q must be refused",
					out, tc.class.Archetype, tc.class.Predicate)
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v; want it to wrap aql.ErrInvalidQuery", err)
			}
		})
	}
}

// TestREQ119_EmitIgnoresTheParsedPredicate shows Emit does not read the
// model at the class or alias-root bracket: replacing it, or clearing it,
// leaves the emitted text byte-identical.
func TestREQ119_EmitIgnoresTheParsedPredicate(t *testing.T) {
	t.Parallel()
	const src = "SELECT o[at0002, 'X']/data FROM OBSERVATION o[" + bloodPressure + ", 'Blood pressure']" +
		" CONTAINS CLUSTER k[at0003] WHERE o[at0002]/data/value = 1"
	pq, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatal(err)
	}
	want, err := pq.Emit()
	if err != nil {
		t.Fatal(err)
	}
	decoy := aql.NodeIDPredicate{ID: "at9999"}
	for name, set := range map[string]func(q *parse.Query, p aql.SegmentPredicate){
		"class": func(q *parse.Query, p aql.SegmentPredicate) {
			q.From.Root.ParsedPredicate = p
			q.From.Contains.Class.ParsedPredicate = p
		},
		"alias root": func(q *parse.Query, p aql.SegmentPredicate) {
			pe := q.Select.Items[0].Expr.(parse.PathExpr)
			pe.ParsedPredicate = p
			q.Select.Items[0].Expr = pe
			cmp := q.Where.(aql.Comparison)
			parsed := *cmp.ParsedPath
			parsed.ParsedPredicate = p
			cmp.ParsedPath = &parsed
			q.Where = cmp
		},
	} {
		for _, p := range []aql.SegmentPredicate{decoy, nil} {
			q, err := parse.ParseQuery(src)
			if err != nil {
				t.Fatal(err)
			}
			set(q, p)
			got, err := q.Emit()
			if err != nil || got != want {
				t.Errorf("%s ParsedPredicate = %v: Emit = %q, %v; want %q", name, p, got, err, want)
			}
		}
	}
}

// TestREQ119_JunctionRootIgnoresAParsedPredicate pins the same rule where the
// FROM root is a junction and Root itself is unused: a ParsedPredicate left
// on Root is not class data the emitter drops, since emission never reads it.
func TestREQ119_JunctionRootIgnoresAParsedPredicate(t *testing.T) {
	t.Parallel()
	q, err := parse.ParseQuery("SELECT c/uid/value FROM COMPOSITION c OR EHR e")
	if err != nil {
		t.Fatal(err)
	}
	want, err := q.Emit()
	if err != nil {
		t.Fatal(err)
	}
	q.From.Root.ParsedPredicate = aql.NodeIDPredicate{ID: "at0001"}
	got, err := q.Emit()
	if err != nil || got != want {
		t.Errorf("Emit = %q, %v; want %q: Root's ParsedPredicate is not emitted content", got, err, want)
	}
}

// TestREQ113_JunctionClassBracketNamesNoArchetype keeps Archetype to the
// named form itself: a junction whose first operand is a named archetype is
// not that form, so Archetype stays empty and the query still emits.
func TestREQ113_JunctionClassBracketNamesNoArchetype(t *testing.T) {
	t.Parallel()
	const src = "SELECT o/data FROM OBSERVATION o[" + bloodPressure + ", 'x' and at0001]"
	flat, structured := classesOf(t, src)
	for view, classes := range map[string]map[string]parse.ClassExpr{"Parse": flat, "ParseQuery": structured} {
		if a := classes["o"].Archetype; a != "" {
			t.Errorf("%s: Archetype = %q; want empty for a junction", view, a)
		}
		if _, ok := classes["o"].ParsedPredicate.(aql.JunctionPredicate); !ok {
			t.Errorf("%s: ParsedPredicate = %+v; want a junction", view, classes["o"].ParsedPredicate)
		}
	}
	pq, err := parse.ParseQuery(src)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := pq.Emit(); err != nil || out != src {
		t.Errorf("Emit = %q, %v; want %q", out, err, src)
	}
}
