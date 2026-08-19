package lint_test

// span_test.go: PROBE-096 arms (b) and (c) on the lint surface —
// REQ-109 § Value-free lint diagnostics.
//
// The value-free assertion runs against the INPUT's value spans, never against
// expected strings, and it deliberately does NOT assert the value-BEARING
// fields clean: an arm that scrubbed Detail and Path too would pass a build
// that had quietly made them structural and would hide the point of the split.

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// spanCase is one query plus the source values a disclosure boundary must
// never see echoed in a value-free field.
type spanCase struct {
	name   string
	query  string
	values []string
}

func spanCases() []spanCase {
	return []spanCase{
		{
			name:   "unbound alias",
			query:  "SELECT zz/data[at0001, 'Systolic']/magnitude FROM COMPOSITION c",
			values: []string{"Systolic", "at0001"},
		},
		{
			name:   "syntax error",
			query:  "SELECT c FROM COMPOSITION c WHERE c/x = = 'secret-value'",
			values: []string{"secret-value"},
		},
		{
			name:   "unbound param",
			query:  "SELECT c FROM COMPOSITION c WHERE c/name/value = $missing",
			values: []string{"missing"},
		},
		{
			name:   "deprecated top",
			query:  "SELECT TOP 5 c FROM COMPOSITION c LIMIT 10",
			values: nil,
		},
	}
}

// TestValueFreeFieldsCarryNoSourceText is arm (b) on the lint surface.
func TestValueFreeFieldsCarryNoSourceText(t *testing.T) {
	t.Parallel()
	for _, tc := range spanCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := lint.LintString(tc.query, nil)
			if len(res.Issues) == 0 {
				t.Fatalf("LintString(%q) found nothing; the case no longer "+
					"exercises the diagnostic surface", tc.query)
			}
			for _, iss := range res.Issues {
				// Code and Severity render; Span is numeric. Together they are
				// the value-free surface.
				got := iss.Code + " " + iss.Severity.String()
				for _, v := range tc.values {
					if strings.Contains(got, v) {
						t.Errorf("value-free fields %q contain the source value %q", got, v)
					}
				}
			}
		})
	}
}

// TestSpanIsEitherAttributedOrZero pins the no-fallback rule: a span points at
// the finding or is zero, and is never an approximation of the whole query.
func TestSpanIsEitherAttributedOrZero(t *testing.T) {
	t.Parallel()
	for _, tc := range spanCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, iss := range lint.LintString(tc.query, nil).Issues {
				sp := iss.Span
				if sp.IsZero() {
					continue // not attributable — allowed, and stated as such
				}
				if sp.Start.Line < 1 || sp.Start.Col < 1 {
					t.Errorf("%s: span %+v has a non-positive start; positions are 1-based",
						iss.Code, sp)
				}
				if sp.End.Line < sp.Start.Line ||
					(sp.End.Line == sp.Start.Line && sp.End.Col < sp.Start.Col) {
					t.Errorf("%s: span %+v ends before it starts", iss.Code, sp)
				}
			}
		})
	}
}

// TestUnboundAliasSpanCoversTheAlias is arm (c): the span names the offending
// construct — the alias — not the enclosing clause or the whole path.
func TestUnboundAliasSpanCoversTheAlias(t *testing.T) {
	t.Parallel()
	const q = "SELECT zz/data[at0001]/magnitude FROM COMPOSITION c"
	var found bool
	for _, iss := range lint.LintString(q, nil).Issues {
		if iss.Code != "aql_unknown_alias" {
			continue
		}
		found = true
		if iss.Span.IsZero() {
			t.Fatal("aql_unknown_alias carries no span although the path has a position")
		}
		start, end := iss.Span.Start.Col-1, iss.Span.End.Col-1
		if start < 0 || end > len(q) || start >= end {
			t.Fatalf("span %+v does not address the %d-column source", iss.Span, len(q))
		}
		if got := q[start:end]; got != "zz" {
			t.Errorf("span covers %q; want the unbound alias %q — a span over the "+
				"whole path or clause does not localise the finding", got, "zz")
		}
	}
	if !found {
		t.Fatal("no aql_unknown_alias issue; the case stopped exercising the rule")
	}
}

// TestSyntaxSpanMarksThePosition pins the zero-width syntax span, and that it
// agrees with the line/column the Detail text already carried.
func TestSyntaxSpanMarksThePosition(t *testing.T) {
	t.Parallel()
	res := lint.LintString("SELECT c FROM COMPOSITION c WHERE c/x = = 1", nil)
	if len(res.Issues) != 1 || res.Issues[0].Code != "aql_syntax" {
		t.Fatalf("got %+v; want exactly one aql_syntax issue", res.Issues)
	}
	iss := res.Issues[0]
	if iss.Span.IsZero() {
		t.Fatal("aql_syntax carries no span although parse.SyntaxError has a position")
	}
	if iss.Span.Start != iss.Span.End {
		t.Errorf("span %+v is not zero-width; the parser reports where it stopped, "+
			"not how much text was at fault", iss.Span)
	}
	// The Detail text has carried "line:col" since REQ-109; the span must not
	// disagree with it, or a consumer reading both sees two answers.
	prefix := itoa(iss.Span.Start.Line) + ":" + itoa(iss.Span.Start.Col) + ":"
	if !strings.HasPrefix(iss.Detail, prefix) {
		t.Errorf("Detail %q does not start with the span's position %q; the two "+
			"positional channels must agree", iss.Detail, prefix)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// TestLintSpanIsTheParseSpanType is the one-type rule: lint.Span and
// parse.Span must be assignable both ways with no conversion, which is what
// makes correlating a lint issue with a dropped construct a comparison.
// takesParseSpan and takesLintSpan make the one-type rule a COMPILE-time
// check: passing each name's value to the other's parameter only compiles
// while lint.Span is an alias for parse.Span. A structurally-identical twin
// would fail to build here, which is the whole point — a runtime comparison
// could not tell the two apart.
func takesParseSpan(parse.Span) {}
func takesLintSpan(lint.Span)   {}

func TestLintSpanIsTheParseSpanType(t *testing.T) {
	t.Parallel()
	fromParse := parse.Span{
		Start: parse.Position{Line: 1, Col: 2},
		End:   parse.Position{Line: 1, Col: 5},
	}
	takesLintSpan(fromParse)

	var fromLint lint.Span
	takesParseSpan(fromLint)

	// And the round trip through an Issue keeps identity.
	iss := lint.Issue{Span: fromParse}
	if iss.Span != fromParse {
		t.Errorf("Issue.Span = %+v; want %+v", iss.Span, fromParse)
	}
}

// TestDropAndLintSpansAreComparable is the reason the type is shared: one
// query that both fails to model a construct and trips a lint rule yields two
// diagnostics whose spans can be compared directly.
func TestDropAndLintSpansAreComparable(t *testing.T) {
	t.Parallel()
	const q = "SELECT zz/x FROM COMPOSITION c LIMIT 99999999999999999999999999"
	doc, err := parse.Parse(q)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	drops := doc.Dropped()
	if len(drops) == 0 {
		t.Fatal("no dropped construct; the case stopped exercising the drop channel")
	}
	var aliasSpan lint.Span
	for _, iss := range lint.Lint(doc, nil).Issues {
		if iss.Code == "aql_unknown_alias" {
			aliasSpan = iss.Span
		}
	}
	if aliasSpan.IsZero() {
		t.Fatal("no aql_unknown_alias span; the case stopped exercising the lint surface")
	}
	// A direct comparison — no conversion. The two findings are at different
	// places in the same query, so they must not be equal.
	if aliasSpan == drops[0].Span {
		t.Errorf("the alias span and the drop span are equal (%+v); they mark "+
			"different constructs in the query", aliasSpan)
	}
	if aliasSpan.Start.Col >= drops[0].Span.Start.Col {
		t.Errorf("alias span %+v should start before the LIMIT drop %+v",
			aliasSpan, drops[0].Span)
	}
}
