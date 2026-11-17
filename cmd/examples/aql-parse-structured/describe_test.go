package main

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// TestDescribeClassExprKeepsTheName holds the printed bracket to the source:
// a named archetype carries its id on Archetype and the whole bracket on
// Predicate, so printing Archetype first would drop the name.
func TestDescribeClassExprKeepsTheName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ query, want string }{
		{
			query: "SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1, 'Systolic']",
			want:  "OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1, 'Systolic']",
		},
		{
			query: "SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
			want:  "OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		},
		{
			query: "SELECT e FROM EHR e[ehr_id/value=$id]",
			want:  "EHR e[ehr_id/value=$id]",
		},
	} {
		q, err := parse.ParseQuery(tc.query)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", tc.query, err)
		}
		if got := describeClassExpr(q.From.Root); got != tc.want {
			t.Errorf("describeClassExpr = %q, want %q", got, tc.want)
		}
	}
}
