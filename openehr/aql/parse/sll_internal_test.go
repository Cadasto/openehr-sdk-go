package parse

import (
	"errors"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse/gen"
)

// TestParseSLLAnswersOrDeclines pins the first of Parse's two parses (REQ-109:
// Parse validates a query against the grammar profile). The SLL parse answers
// for a query the grammar accepts, so that query is parsed once; it declines,
// without a panic reaching the caller, both for a query the parser refuses
// and for one whose only fault is a character the lexer drops, which the
// default parse reports. Parse then parses the query again and reports the
// error itself. Let the bail-out panic escape and the refused rows panic;
// leave out the lexer's errors and the dropped-character row answers.
func TestParseSLLAnswersOrDeclines(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		query  string
		answer bool
	}{
		{name: "accepted", query: `SELECT e/ehr_id/value FROM EHR e CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1] WHERE c/name/value = 'x'`, answer: true},
		{name: "refused by the parser", query: `SELECT e/ehr_id/value FORM EHR e`},
		{name: "a character the lexer drops", query: "SELECT e/ehr_id/value FROM EHR e §"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stream, lexErrors := lex(tc.query)
			tree, ok := parseSLL(stream, len(lexErrors))
			if ok != tc.answer || (tree != nil) != tc.answer {
				t.Errorf("parseSLL(lex(%q)) = (tree %t, %t), want (tree %t, %t)", tc.query, tree != nil, ok, tc.answer, tc.answer)
			}
			if _, err := Parse(tc.query); (err == nil) != tc.answer {
				t.Errorf("Parse(%q) error = %v, want an error %t", tc.query, err, !tc.answer)
			}
		})
	}
}

// TestREQ109_NestingRefusedBeforeParsing pins REQ-109 § Nesting limit's
// "before parsing": a query nested past the limit, closed or not, leaves the
// parser's shared prediction state as it found it, so no caller can grow it
// past what the limit admits. The state is counted as the DFA states of
// every decision, which every parser shares. The test runs serially: a
// parallel test parsing a new shape meanwhile would add states of its own.
func TestREQ109_NestingRefusedBeforeParsing(t *testing.T) {
	p := gen.NewAqlParser(antlr.NewCommonTokenStream(gen.NewAqlLexer(antlr.NewInputStream("")), antlr.TokenDefaultChannel))
	states := func() (n int) {
		for _, d := range p.GetInterpreter().DecisionToDFA() {
			n += d.Len()
		}
		return n
	}
	deep := "SELECT e/ehr_id/value FROM EHR e WHERE " + strings.Repeat("(", maxNesting+64) + "e/ehr_id/value = 'x'"
	for _, q := range []string{deep, deep + strings.Repeat(")", maxNesting+64)} {
		before := states()
		if _, err := Parse(q); !errors.Is(err, aql.ErrLimitExceeded) {
			t.Fatalf("Parse(nested %d deep) error = %v; want a refusal matching aql.ErrLimitExceeded", maxNesting+64, err)
		}
		if after := states(); after != before {
			t.Errorf("Parse(nested %d deep) grew the shared DFA from %d to %d states; want it refused before parsing", maxNesting+64, before, after)
		}
	}
}
