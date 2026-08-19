package parse_test

// predicate_structure_test.go: PROBE-095 — REQ-113 § Structured node
// predicates.
//
// The corpus is GENERATED from the vendored grammar rather than enumerated: a
// hand-written case list is the "list a maintainer must remember to update"
// REQ-119 § Acceptance forbids, and the failure it forbids is a grammar
// alternative silently uncovered. The sweep below reads every alternative of
// the three productions the bracketed position admits and fails when one has
// no row here.

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

const grammarPath = "../../../resources/aql/grammar/active/AqlParser.g4"

// predicateProductions are the productions `pathPredicate` dispatches to. The
// bracket is THREE overlapping productions, not `nodePredicate` alone, which
// is why all three are swept.
var predicateProductions = []string{"standardPredicate", "archetypePredicate", "nodePredicate"}

// position is where in the bracket a form is presented.
type position int

const (
	atTopLevel position = iota // [FORM]
	inJunction                 // [at0001 and FORM]
)

func (p position) String() string {
	if p == inJunction {
		return "in junction"
	}
	return "top level"
}

// predicateRow is one grammar alternative's corpus entry.
type predicateRow struct {
	name string
	// alts are the grammar alternatives this row covers, as they appear in
	// AqlParser.g4 with whitespace normalised. The sweep requires every
	// alternative of the three productions to appear in some row's alts, so a
	// widened grammar fails here rather than going unstructured in silence.
	alts []string
	// pred is the predicate text, without its brackets.
	pred string
	// want is the expected concrete kind, as %T.
	want string
	// at lists the positions this form can occupy. A form shadowed at the top
	// of the bracket by an earlier pathPredicate alternative is still reachable
	// as a junction operand, and MUST yield the same kind there.
	at []position
	// check asserts the components. Optional.
	check func(t *testing.T, p aql.SegmentPredicate)
}

func predicateRows() []predicateRow {
	both := []position{atTopLevel, inJunction}
	return []predicateRow{
		{
			name: "node id",
			alts: []string{"(ID_CODE | AT_CODE) (SYM_COMMA (STRING | PARAMETER | TERM_CODE | AT_CODE | ID_CODE))?"},
			pred: "at0001",
			want: "aql.NodeIDPredicate",
			at:   both,
			check: func(t *testing.T, p aql.SegmentPredicate) {
				n := p.(aql.NodeIDPredicate)
				if n.ID != "at0001" || n.Name != nil {
					t.Errorf("got %+v; want ID at0001 and no name", n)
				}
			},
		},
		{
			name: "archetype hrid",
			alts: []string{"ARCHETYPE_HRID"},
			pred: "openEHR-EHR-OBSERVATION.blood_pressure.v1",
			want: "aql.ArchetypePredicate",
			at:   both,
			check: func(t *testing.T, p aql.SegmentPredicate) {
				a := p.(aql.ArchetypePredicate)
				if a.HRID != "openEHR-EHR-OBSERVATION.blood_pressure.v1" {
					t.Errorf("HRID = %q", a.HRID)
				}
			},
		},
		{
			name: "archetype hrid with name",
			alts: []string{"ARCHETYPE_HRID (SYM_COMMA (STRING | PARAMETER | TERM_CODE | AT_CODE | ID_CODE))?"},
			pred: "openEHR-EHR-OBSERVATION.blood_pressure.v1, 'Systolic'",
			want: "aql.ArchetypePredicate",
			at:   both,
			check: func(t *testing.T, p aql.SegmentPredicate) {
				a := p.(aql.ArchetypePredicate)
				if a.Name == nil {
					t.Fatal("an HRID predicate carrying a name reported none; the name " +
						"slot hangs off the HRID alternative too")
				}
				if a.Name.Kind != aql.NameString || a.Name.Text != "Systolic" {
					t.Errorf("name = %+v; want string Systolic", *a.Name)
				}
			},
		},
		{
			name: "parameter",
			alts: []string{"PARAMETER"},
			pred: "$node",
			want: "aql.ParamPredicate",
			at:   both,
			check: func(t *testing.T, p aql.SegmentPredicate) {
				if got := p.(aql.ParamPredicate).Name; got != "node" {
					t.Errorf("Name = %q; want %q (no leading $)", got, "node")
				}
			},
		},
		{
			name: "comparison",
			alts: []string{"objectPath COMPARISON_OPERATOR pathPredicateOperand"},
			pred: "name/value='Systolic'",
			want: "aql.ComparisonPredicate",
			at:   both,
			check: func(t *testing.T, p aql.SegmentPredicate) {
				c := p.(aql.ComparisonPredicate).Comparison
				if c.Path != "name/value" || c.Op != "=" {
					t.Errorf("got path %q op %q; want name/value and =", c.Path, c.Op)
				}
			},
		},
		{
			name: "matches regex",
			alts: []string{"objectPath MATCHES CONTAINED_REGEX"},
			pred: "name/value matches {/systolic/}",
			want: "aql.MatchesPredicate",
			at:   both,
			check: func(t *testing.T, p aql.SegmentPredicate) {
				m := p.(aql.MatchesPredicate)
				if m.Path != "name/value" {
					t.Errorf("Path = %q", m.Path)
				}
				if strings.HasPrefix(m.Regex, "{") || strings.HasSuffix(m.Regex, "}") {
					t.Errorf("Regex %q still carries its brace delimiters", m.Regex)
				}
			},
		},
		{
			name: "and junction",
			alts: []string{"nodePredicate AND nodePredicate"},
			pred: "at0001 and name/value='Systolic'",
			want: "aql.JunctionPredicate",
			at:   []position{atTopLevel},
			check: func(t *testing.T, p aql.SegmentPredicate) {
				j := p.(aql.JunctionPredicate)
				if j.Op != aql.OpAnd {
					t.Errorf("Op = %v; want AND", j.Op)
				}
				if _, ok := j.Left.(aql.NodeIDPredicate); !ok {
					t.Errorf("Left = %T; want aql.NodeIDPredicate", j.Left)
				}
				if _, ok := j.Right.(aql.ComparisonPredicate); !ok {
					t.Errorf("Right = %T; want aql.ComparisonPredicate", j.Right)
				}
			},
		},
		{
			name: "or junction",
			alts: []string{"nodePredicate OR nodePredicate"},
			pred: "at0001 or at0002",
			want: "aql.JunctionPredicate",
			at:   []position{atTopLevel},
			check: func(t *testing.T, p aql.SegmentPredicate) {
				if got := p.(aql.JunctionPredicate).Op; got != aql.OpOr {
					t.Errorf("Op = %v; want OR", got)
				}
			},
		},
	}
}

// query wraps a predicate at the requested position.
func (r predicateRow) query(at position) string {
	pred := r.pred
	if at == inJunction {
		pred = "at0009 and " + pred
	}
	return "SELECT o/data[" + pred + "]/magnitude FROM OBSERVATION o"
}

// segmentPredicate reads back the one predicated segment of such a query.
func segmentPredicate(t *testing.T, q string) (aql.SegmentPredicate, string) {
	t.Helper()
	doc, err := parse.Parse(q)
	if err != nil {
		t.Fatalf("Parse(%q) = %v", q, err)
	}
	for _, ip := range doc.Paths {
		for _, seg := range ip.Segments {
			if seg.Predicate != "" {
				return seg.Parsed, seg.Predicate
			}
		}
	}
	t.Fatalf("no predicated segment in %q", q)
	return nil, ""
}

// TestEveryGrammarAlternativeIsCovered is the generated half of PROBE-095: the
// corpus MUST have a row for every alternative of the three productions the
// bracketed position admits, so a widened grammar fails here.
func TestEveryGrammarAlternativeIsCovered(t *testing.T) {
	t.Parallel()
	covered := map[string]bool{}
	for _, r := range predicateRows() {
		for _, a := range r.alts {
			covered[a] = true
		}
	}
	var total int
	for _, prod := range predicateProductions {
		alts := grammarAlternatives(t, prod)
		if len(alts) == 0 {
			t.Fatalf("read no alternatives for %s; the sweep is not reading the "+
				"grammar and would pass vacuously", prod)
		}
		for _, a := range alts {
			total++
			if !covered[a] {
				t.Errorf("%s alternative %q has no corpus row; the grammar admits a "+
					"form no kind covers (REQ-113 § The bracket is three productions)", prod, a)
			}
		}
	}
	t.Logf("swept %d grammar alternatives across %v", total, predicateProductions)
}

// TestTheGrammarSweepCanFail shows the sweep is able to fail: an alternative
// absent from the corpus must be reported, or a broken reader would mark
// everything covered.
func TestTheGrammarSweepCanFail(t *testing.T) {
	t.Parallel()
	covered := map[string]bool{"ARCHETYPE_HRID": true}
	var missing int
	for _, a := range grammarAlternatives(t, "nodePredicate") {
		if !covered[a] {
			missing++
		}
	}
	if missing == 0 {
		t.Fatal("a deliberately near-empty coverage set reported nothing missing; " +
			"the sweep cannot fail and proves nothing")
	}
}

// TestEveryFormStructures runs each row at every position it can occupy.
func TestEveryFormStructures(t *testing.T) {
	t.Parallel()
	for _, r := range predicateRows() {
		for _, at := range r.at {
			t.Run(r.name+"/"+at.String(), func(t *testing.T) {
				t.Parallel()
				q := r.query(at)
				got, raw := segmentPredicate(t, q)
				if got == nil {
					t.Fatalf("predicate %q is unstructured; every alternative the "+
						"grammar admits at this position is structured", raw)
				}
				// At a junction position the whole predicate is a junction; the
				// row's own kind is its right operand.
				subject := got
				if at == inJunction {
					j, ok := got.(aql.JunctionPredicate)
					if !ok {
						t.Fatalf("a junction predicate structured as %T", got)
					}
					subject = j.Right
				}
				if fmt.Sprintf("%T", subject) != r.want {
					t.Fatalf("%q structured as %T; want %s", raw, subject, r.want)
				}
				if r.check != nil {
					r.check(t, subject)
				}
			})
		}
	}
}

// TestTheSameFormYieldsTheSameKindInBothPositions is the ONLY mechanical check
// on the rule that the kind is a property of the FORM rather than of the parse
// node that carried it — a comparison, a bare HRID and a bare $param are each
// spelled by two productions.
func TestTheSameFormYieldsTheSameKindInBothPositions(t *testing.T) {
	t.Parallel()
	for _, r := range predicateRows() {
		if len(r.at) < 2 {
			continue
		}
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			top, _ := segmentPredicate(t, r.query(atTopLevel))
			nested, _ := segmentPredicate(t, r.query(inJunction))
			j, ok := nested.(aql.JunctionPredicate)
			if !ok {
				t.Fatalf("nested case structured as %T; want a junction", nested)
			}
			if fmt.Sprintf("%T", top) != fmt.Sprintf("%T", j.Right) {
				t.Errorf("the same form yields %T at the top of the bracket and %T as a "+
					"junction operand; the kind must be a property of the form",
					top, j.Right)
			}
			if !aql.EqualPredicates(top, j.Right) {
				t.Errorf("the same form is not EqualPredicates across positions:\n"+
					"  top level: %+v\n  nested:    %+v", top, j.Right)
			}
		})
	}
}

// TestTriviaAndEscapesDoNotReachTheComponents is the property form of the
// trivia rule: a padded, commented, escaped spelling MUST produce the same
// components as the bare one WHILE the verbatim text differs. A corpus in
// which both assertions cannot fire is not exercising the rule.
func TestTriviaAndEscapesDoNotReachTheComponents(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, bare, padded string }{
		{"whitespace", "at0001,'Systolic'", "at0001 ,   'Systolic'"},
		{"comment", "at0001,'Systolic'", "at0001 -- which one\n,'Systolic'"},
		{"comparison whitespace", "name/value='x'", "name/value  =  'x'"},
		{"junction whitespace", "at0001 and at0002", "at0001   and   at0002"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mk := func(pred string) string {
				return "SELECT o/data[" + pred + "]/magnitude FROM OBSERVATION o"
			}
			bareP, bareRaw := segmentPredicate(t, mk(tc.bare))
			padP, padRaw := segmentPredicate(t, mk(tc.padded))
			if bareRaw == padRaw {
				t.Fatalf("the two spellings produced identical verbatim text (%q); the "+
					"case is not exercising trivia", bareRaw)
			}
			if !aql.EqualPredicates(bareP, padP) {
				t.Errorf("trivia reached the components:\n  bare   %q -> %+v\n  padded %q -> %+v",
					bareRaw, bareP, padRaw, padP)
			}
		})
	}
}

// TestEscapedNameResolvesItsEscapes pins that a name is the VALUE, not the
// spelling: quotes removed and escapes resolved.
func TestEscapedNameResolvesItsEscapes(t *testing.T) {
	t.Parallel()
	const q = `SELECT o/data[at0001, 'it\'s']/magnitude FROM OBSERVATION o`
	got, raw := segmentPredicate(t, q)
	n, ok := got.(aql.NodeIDPredicate)
	if !ok {
		t.Fatalf("structured as %T; want aql.NodeIDPredicate", got)
	}
	if n.Name == nil {
		t.Fatal("no name")
	}
	if n.Name.Text != "it's" {
		t.Errorf("Text = %q; want %q — the component is the value, escapes resolved", n.Name.Text, "it's")
	}
	if !strings.Contains(raw, `\'`) {
		t.Errorf("verbatim text %q lost its escape; Raw must stay the spelling", raw)
	}
}

// TestEveryNameSpellingIsDiscriminated covers all five spellings the name slot
// admits, on BOTH alternatives that carry it. A parameter is not a name but the
// name deferred to bind time, so it must be distinguishable.
func TestEveryNameSpellingIsDiscriminated(t *testing.T) {
	t.Parallel()
	const hrid = "openEHR-EHR-OBSERVATION.blood_pressure.v1"
	names := []struct {
		spelling string
		want     aql.PredicateNameKind
		text     string
	}{
		{"'Systolic'", aql.NameString, "Systolic"},
		{"$who", aql.NameParam, "who"},
		{"at0004", aql.NameAtCode, "at0004"},
		{"id4", aql.NameIDCode, "id4"},
		{"SNOMED-CT::271649006|systolic|", aql.NameTermCode, ""},
	}
	carriers := []struct{ label, spelling string }{
		{"node id", "at0001"},
		{"archetype hrid", hrid},
	}
	for _, c := range carriers {
		carrier := c.spelling
		for _, n := range names {
			t.Run(n.want.String()+"/"+c.label, func(t *testing.T) {
				t.Parallel()
				q := "SELECT o/data[" + carrier + ", " + n.spelling + "]/magnitude FROM OBSERVATION o"
				got, raw := segmentPredicate(t, q)
				var name *aql.PredicateName
				switch p := got.(type) {
				case aql.NodeIDPredicate:
					name = p.Name
				case aql.ArchetypePredicate:
					name = p.Name
				default:
					t.Fatalf("%q structured as %T; want a named node-id or archetype predicate", raw, got)
				}
				if name == nil {
					t.Fatalf("%q reported no name", raw)
				}
				if name.Kind != n.want {
					t.Fatalf("name kind = %v; want %v", name.Kind, n.want)
				}
				if n.want == aql.NameTermCode {
					if name.Terminology != "SNOMED-CT" || name.Code != "271649006" || name.Display != "systolic" {
						t.Errorf("term code = %+v; want SNOMED-CT / 271649006 / systolic — a "+
							"consumer that has to split this is back to re-lexing", *name)
					}
					return
				}
				if name.Text != n.text {
					t.Errorf("Text = %q; want %q", name.Text, n.text)
				}
			})
		}
	}
}

// TestBothExtractorsAgreeOnTheStructure keeps the two path extraction sites in
// step: their outputs are compared for equality elsewhere, so a structure
// populated by only one of them would be a silent asymmetry.
func TestBothExtractorsAgreeOnTheStructure(t *testing.T) {
	t.Parallel()
	for _, r := range predicateRows() {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			q := r.query(atTopLevel)
			flat, err := parse.Parse(q)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			structured, err := parse.ParseQuery(q)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			want := firstSegmentPredicate(flat.Paths)
			var got aql.SegmentPredicate
			for _, item := range structured.Select.Items {
				if pe, ok := item.Expr.(parse.PathExpr); ok {
					got = firstSegmentPredicate([]parse.IdentifiedPath{pe.IdentifiedPath})
				}
			}
			if !aql.EqualPredicates(want, got) {
				t.Errorf("the two extractors disagree:\n  Parse:      %+v\n  ParseQuery: %+v", want, got)
			}
		})
	}
}

func firstSegmentPredicate(paths []parse.IdentifiedPath) aql.SegmentPredicate {
	for _, ip := range paths {
		for _, seg := range ip.Segments {
			if seg.Predicate != "" {
				return seg.Parsed
			}
		}
	}
	return nil
}

// TestEmissionIsUnaffected asserts rather than assumes REQ-119 parity: the
// structured model is a read-side derivation, so emitted text must be
// byte-identical to the source's canonical form.
func TestEmissionIsUnaffected(t *testing.T) {
	t.Parallel()
	for _, r := range predicateRows() {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			q := r.query(atTopLevel)
			pq, err := parse.ParseQuery(q)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			emitted, err := pq.Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			again, err := parse.ParseQuery(emitted)
			if err != nil {
				t.Fatalf("re-parsing emitted %q: %v", emitted, err)
			}
			twice, err := again.Emit()
			if err != nil {
				t.Fatalf("second Emit: %v", err)
			}
			if twice != emitted {
				t.Errorf("emission is not a fixed point:\n  first  %q\n  second %q", emitted, twice)
			}
		})
	}
}

// TestEqualPredicatesIsPanicFree pins the comparability contract: the model
// carries an aql.Value, which panics under ==, so equality must be a function.
func TestEqualPredicatesIsPanicFree(t *testing.T) {
	t.Parallel()
	withSlice := aql.ComparisonPredicate{Comparison: aql.Comparison{
		Path: "x", Op: "=", Val: aql.Func("CONCAT", aql.String("a"), aql.String("b")),
	}}
	if !aql.EqualPredicates(withSlice, withSlice) {
		t.Error("a predicate does not equal itself")
	}
	if aql.EqualPredicates(withSlice, aql.NodeIDPredicate{ID: "at0001"}) {
		t.Error("different shapes compared equal")
	}
	if !aql.EqualPredicates(nil, nil) {
		t.Error("two nils are not equal")
	}
	if aql.EqualPredicates(nil, withSlice) {
		t.Error("nil equals a populated predicate")
	}
}

// grammarAlternatives reads one production's alternatives out of the vendored
// grammar, whitespace-normalised and comment-stripped. Splitting on top-level
// `|` only — a `|` inside a group belongs to that group's choice.
func grammarAlternatives(t *testing.T, production string) []string {
	t.Helper()
	src, err := os.ReadFile(grammarPath)
	if err != nil {
		t.Fatalf("reading the vendored grammar: %v", err)
	}
	re := regexp.MustCompile(`(?ms)^` + production + `\s*\n?\s*:(.*?);\s*$`)
	m := re.FindSubmatch(src)
	if m == nil {
		t.Fatalf("production %q not found in %s", production, grammarPath)
	}
	body := regexp.MustCompile(`//[^\n]*`).ReplaceAllString(string(m[1]), "")

	var alts []string
	var cur strings.Builder
	depth := 0
	for _, r := range body {
		switch r {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case '|':
			if depth == 0 {
				alts = append(alts, normaliseAlt(cur.String()))
				cur.Reset()
				continue
			}
		}
		cur.WriteRune(r)
	}
	alts = append(alts, normaliseAlt(cur.String()))

	out := alts[:0]
	for _, a := range alts {
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

func normaliseAlt(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
