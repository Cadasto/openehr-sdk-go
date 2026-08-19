package parse_test

// dropped_test.go: PROBE-096 arm (a) and the value-free half of arm (b) —
// REQ-113 § Value-free structured drop records.
//
// The oracle is deliberately NOT a list of expected strings: a test comparing
// a record against an expectation passes a record that echoed a value the
// author happened to expect. Instead the assertions are (1) a record exists
// per drop, held over the extractor SOURCE so a site added later is covered
// the day it lands, and (2) no field of a record contains any substring of
// the input's value spans.

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// hugeInt is beyond Go's int on every platform the module supports, so the
// value vocabulary cannot represent it. This is the ONE drop class a legal
// query reaches (REQ-119 § Amends REQ-117).
const hugeInt = "99999999999999999999999999"

// dropCase is one legal query carrying a construct the AST cannot hold.
type dropCase struct {
	name   string
	query  string
	clause parse.Clause
	// values are the source spans a disclosure boundary must never see
	// echoed back. Every one is asserted absent from every record field.
	values []string
}

func reachableDropCases() []dropCase {
	return []dropCase{
		{
			name:   "limit",
			query:  "SELECT c FROM COMPOSITION c LIMIT " + hugeInt,
			clause: parse.ClauseLimit,
			values: []string{hugeInt},
		},
		{
			name:   "offset",
			query:  "SELECT c FROM COMPOSITION c LIMIT 5 OFFSET " + hugeInt,
			clause: parse.ClauseOffset,
			values: []string{hugeInt},
		},
		{
			name:   "top",
			query:  "SELECT TOP " + hugeInt + " c FROM COMPOSITION c",
			clause: parse.ClauseTop,
			values: []string{hugeInt},
		},
		{
			name:   "select literal",
			query:  "SELECT " + hugeInt + " FROM COMPOSITION c",
			clause: parse.ClauseSelect,
			values: []string{hugeInt},
		},
		{
			name:   "where operand",
			query:  "SELECT c FROM COMPOSITION c WHERE c/x = " + hugeInt,
			clause: parse.ClauseWhere,
			values: []string{hugeInt},
		},
		{
			name:   "matches member",
			query:  "SELECT c FROM COMPOSITION c WHERE c/x MATCHES {" + hugeInt + "}",
			clause: parse.ClauseWhere,
			values: []string{hugeInt},
		},
	}
}

// TestEveryReachableDropRecordsAConstruct is PROBE-096 arm (a) at the
// behavioural level: a query that drops a construct reports it, by kind and
// clause, beside the unchanged error.
func TestEveryReachableDropRecordsAConstruct(t *testing.T) {
	t.Parallel()
	for _, tc := range reachableDropCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc, err := parse.Parse(tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) = %v; the query must be syntactically legal, "+
					"or it exercises the syntax path instead of the drop channel", tc.query, err)
			}
			// The existing channel is unchanged: still ErrIncompleteAST.
			if qerr := doc.QueryErr(); !errors.Is(qerr, aql.ErrIncompleteAST) {
				t.Fatalf("QueryErr() = %v; want ErrIncompleteAST — without it this "+
					"case no longer reaches the drop channel", qerr)
			}
			drops := doc.Dropped()
			if len(drops) == 0 {
				t.Fatalf("Dropped() is empty although QueryErr() reported a gap: " +
					"a drop with no record is a defect (REQ-113)")
			}
			var found bool
			for _, d := range drops {
				if d.Kind == parse.KindNumericOutOfRange && d.Clause == tc.clause {
					found = true
				}
				if d.Kind == parse.KindUnclassified {
					t.Errorf("record %v carries KindUnclassified; the extractor "+
						"must never record the zero value", d)
				}
				if d.Span.IsZero() {
					t.Errorf("record %v carries a zero span; want the offending "+
						"construct's position", d)
				}
			}
			if !found {
				t.Errorf("Dropped() = %v; want a KindNumericOutOfRange in %v",
					drops, tc.clause)
			}
		})
	}
}

// TestDropRecordsCarryNoSourceText is PROBE-096 arm (b): every field of every
// record is asserted against the INPUT's value spans, not against an expected
// string. The value-BEARING surface (QueryErr's text) is deliberately not
// asserted clean — an arm that scrubbed it too would pass a build that had
// quietly made the error value-free and would hide the point of the split.
func TestDropRecordsCarryNoSourceText(t *testing.T) {
	t.Parallel()
	for _, tc := range reachableDropCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc, err := parse.Parse(tc.query)
			if err != nil {
				t.Fatalf("Parse(%q) = %v", tc.query, err)
			}
			for _, d := range doc.Dropped() {
				// The record's whole rendered form stands in for its fields:
				// Kind and Clause render through String(), Span is numeric.
				rendered := d.String() + " " + d.Kind.String() + " " + d.Clause.String()
				for _, v := range tc.values {
					if strings.Contains(rendered, v) {
						t.Errorf("record %q contains the source value %q; every "+
							"value-free field MUST NOT carry source text", rendered, v)
					}
				}
			}
			// Positive control: the human-readable error DOES quote the value.
			// If this stops holding, the arm above has stopped proving anything.
			if got := doc.QueryErr().Error(); !containsAny(got, tc.values) {
				t.Errorf("QueryErr() = %q; expected it to quote the source value — "+
					"the value-free assertion above is only meaningful while the "+
					"text channel is value-bearing", got)
			}
		})
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// TestDroppedTriggersItsOwnExtraction pins the accessor property that a
// document nobody called Query() on must not read as "nothing dropped".
func TestDroppedTriggersItsOwnExtraction(t *testing.T) {
	t.Parallel()
	doc, err := parse.Parse("SELECT c FROM COMPOSITION c LIMIT " + hugeInt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Dropped() is the FIRST call — no Query(), no QueryErr() before it.
	if got := doc.Dropped(); len(got) == 0 {
		t.Fatal("Dropped() on a fresh document is empty; it must trigger " +
			"extraction itself rather than reporting the un-extracted state")
	}
}

// TestDroppedReturnsACopy pins that a caller cannot corrupt the shared document.
func TestDroppedReturnsACopy(t *testing.T) {
	t.Parallel()
	doc, err := parse.Parse("SELECT c FROM COMPOSITION c LIMIT " + hugeInt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	first := doc.Dropped()
	if len(first) == 0 {
		t.Fatal("no drops to mutate")
	}
	want := first[0]
	first[0] = parse.DroppedConstruct{}
	if got := doc.Dropped()[0]; got != want {
		t.Errorf("Dropped()[0] = %v after a caller zeroed its copy; want %v — "+
			"the accessor must return a copy", got, want)
	}
}

// TestCleanQueryDropsNothing is the negative control: the assertions above are
// only meaningful if a well-formed query records nothing.
func TestCleanQueryDropsNothing(t *testing.T) {
	t.Parallel()
	doc, err := parse.Parse("SELECT c/uid/value FROM COMPOSITION c WHERE c/name/value = 'x' LIMIT 5")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.QueryErr(); err != nil {
		t.Fatalf("QueryErr() = %v; want nil for a fully modelled query", err)
	}
	if got := doc.Dropped(); len(got) != 0 {
		t.Errorf("Dropped() = %v; want empty when QueryErr() is nil", got)
	}
}

// TestUnmodelledConstructIsUnreachableByALegalQuery holds the claim REQ-113
// makes about the defensive arms: KindUnmodelledConstruct exists so a widened
// grammar fails loudly, and no legal query reaches it today. Asserted at the
// unit level because no corpus can reach it — which is exactly why the kind
// set enumerates classes rather than call sites.
func TestUnmodelledConstructIsUnreachableByALegalQuery(t *testing.T) {
	t.Parallel()
	for _, tc := range reachableDropCases() {
		doc, err := parse.Parse(tc.query)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.query, err)
		}
		for _, d := range doc.Dropped() {
			if d.Kind == parse.KindUnmodelledConstruct {
				t.Errorf("%s: recorded KindUnmodelledConstruct %v; the catalogue "+
					"covers the whole grammar profile (REQ-117), so a legal "+
					"query must not reach a defensive arm", tc.name, d)
			}
		}
	}
}

// TestEveryIncompleteSiteRecordsAKind is PROBE-096 arm (a) held over the
// SOURCE rather than over a list: every ex.incomplete(...) call in the
// extractor MUST pass a ConstructKind as its first argument. A site added
// later is covered the day it lands, which a hand-maintained enumeration of
// sites would not be.
func TestEveryIncompleteSiteRecordsAKind(t *testing.T) {
	t.Parallel()
	const src = "extract_query.go"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, src, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", src, err)
	}
	var sites, bad int
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "incomplete" {
			return true
		}
		sites++
		if !firstArgIsAKind(call) {
			bad++
			t.Errorf("%s: ex.incomplete(...) does not pass a ConstructKind as its "+
				"first argument; a drop that records a reason but no kind is a "+
				"defect (REQ-113 § The kind set enumerates CLASSES)",
				fset.Position(call.Pos()))
		}
		return true
	})
	if sites == 0 {
		t.Fatal("found no ex.incomplete(...) calls; the sweep is not reading the " +
			"extractor and would pass vacuously")
	}
	t.Logf("checked %d drop sites, %d without a kind", sites, bad)
}

// TestTheKindSweepCanFail shows the sweep above is able to fail: the same
// predicate it uses must reject a call whose first argument is not a kind.
// Without this, a broken matcher would report every site as compliant.
func TestTheKindSweepCanFail(t *testing.T) {
	t.Parallel()
	const bad = `package p
func f() { ex.incomplete("no kind here", x.GetText()) }`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bad.go", bad, 0)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	var seen, rejected int
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "incomplete" {
			return true
		}
		seen++
		if !firstArgIsAKind(call) {
			rejected++
		}
		return true
	})
	if seen != 1 || rejected != 1 {
		t.Fatalf("saw %d calls, rejected %d; want 1 and 1 — the sweep's predicate "+
			"does not actually reject a kindless call", seen, rejected)
	}
}

// firstArgIsAKind reports whether a call's first argument names a
// ConstructKind member. Kept in one place so the sweep and its
// able-to-fail control cannot drift apart.
func firstArgIsAKind(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	switch a := call.Args[0].(type) {
	case *ast.Ident:
		// A literal member: ex.incomplete(KindNumericOutOfRange, …)
		return strings.HasPrefix(a.Name, "Kind")
	case *ast.SelectorExpr:
		// A kind carried in from a helper: ex.incomplete(gap.Kind, …)
		return a.Sel.Name == "Kind"
	}
	return false
}

// TestConstructKindStringIsTotal keeps String() honest across the closed set,
// including the zero value a reader must be able to render while failing closed.
func TestConstructKindStringIsTotal(t *testing.T) {
	t.Parallel()
	for _, k := range []parse.ConstructKind{
		parse.KindUnclassified,
		parse.KindNumericOutOfRange,
		parse.KindUnmodelledConstruct,
	} {
		if got := k.String(); got == "" {
			t.Errorf("ConstructKind(%d).String() is empty", int(k))
		}
	}
	// An out-of-set value must render, not panic or return "".
	if got := parse.ConstructKind(99).String(); got == "" {
		t.Error("an unknown ConstructKind renders as empty; want a fail-closed label")
	}
}

// TestClauseValuesAreStable pins the append-only rule: the four published
// members keep their numeric identity, so a consumer that persisted or
// compared them is unaffected by the four appended for the drop channel.
func TestClauseValuesAreStable(t *testing.T) {
	t.Parallel()
	for want, c := range []parse.Clause{
		parse.ClauseUnknown,
		parse.ClauseSelect,
		parse.ClauseWhere,
		parse.ClauseOrderBy,
	} {
		if int(c) != want {
			t.Errorf("Clause %v = %d; want %d — the published members MUST NOT "+
				"be renumbered (REQ-113 § The clause axis reuses the landed enum)",
				c, int(c), want)
		}
	}
	for _, c := range []parse.Clause{
		parse.ClauseFrom, parse.ClauseLimit, parse.ClauseOffset, parse.ClauseTop,
	} {
		if c.String() == "unknown" {
			t.Errorf("Clause(%d).String() = %q; every appended member needs its "+
				"own String() arm", int(c), c.String())
		}
	}
}

// TestSpanPointsAtTheConstruct is PROBE-096 arm (c): a span resolves to the
// offending construct, not to its enclosing clause.
func TestSpanPointsAtTheConstruct(t *testing.T) {
	t.Parallel()
	const q = "SELECT c FROM COMPOSITION c LIMIT " + hugeInt
	doc, err := parse.Parse(q)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	drops := doc.Dropped()
	if len(drops) == 0 {
		t.Fatal("no drops recorded")
	}
	d := drops[0]
	if d.Span.Start.Line != 1 {
		t.Errorf("span starts on line %d; the query is one line", d.Span.Start.Line)
	}
	// Columns are 1-based; slice the source back out and compare.
	start, end := d.Span.Start.Col-1, d.Span.End.Col-1
	if start < 0 || end > len(q) || start >= end {
		t.Fatalf("span %+v does not address %d source columns", d.Span, len(q))
	}
	if got := q[start:end]; got != hugeInt {
		t.Errorf("span covers %q; want the offending literal %q — a span pointing "+
			"at the enclosing clause does not support quoting under a caller's "+
			"own policy", got, hugeInt)
	}
}

// TestExtractorSourceIsReadable guards the sweep's premise: it reads the
// extractor from the package directory, which must be where the test runs.
func TestExtractorSourceIsReadable(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat(filepath.Join(".", "extract_query.go")); err != nil {
		t.Fatalf("extract_query.go not readable from the test's working "+
			"directory: %v — TestEveryIncompleteSiteRecordsAKind depends on it", err)
	}
}
