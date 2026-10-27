package parse

import "testing"

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
			tree, ok := parseSLL(tc.query)
			if ok != tc.answer || (tree != nil) != tc.answer {
				t.Errorf("parseSLL(%q) = (tree %t, %t), want (tree %t, %t)", tc.query, tree != nil, ok, tc.answer, tc.answer)
			}
			if _, err := Parse(tc.query); (err == nil) != tc.answer {
				t.Errorf("Parse(%q) error = %v, want an error %t", tc.query, err, !tc.answer)
			}
		})
	}
}
