package aql_test

import (
	"encoding/json"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

func TestNewQueryString(t *testing.T) {
	q := aql.NewQuery("  SELECT e/ehr_id/value FROM EHR e  ")
	if got := q.String(); got != "SELECT e/ehr_id/value FROM EHR e" {
		t.Fatalf("String() = %q", got)
	}
}

func TestQueryValidate(t *testing.T) {
	if err := (aql.Query{}).Validate(); err == nil {
		t.Fatal("expected error for empty query")
	}
	if err := aql.NewQuery("SELECT 1").Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestResultSetUnmarshal(t *testing.T) {
	const raw = `{
	  "meta": {"_type": "RESULTSET", "_executed_aql": "SELECT 1"},
	  "q": "SELECT 1",
	  "columns": [{"name": "#0", "path": "/ehr_id/value"}],
	  "rows": [["abc"]],
	  "extra_field": true
	}`
	var rs aql.ResultSet
	if err := json.Unmarshal([]byte(raw), &rs); err != nil {
		t.Fatal(err)
	}
	if rs.Meta.Type != "RESULTSET" {
		t.Fatalf("meta type = %q", rs.Meta.Type)
	}
	if len(rs.Rows) != 1 || rs.Rows[0][0] != "abc" {
		t.Fatalf("rows = %#v", rs.Rows)
	}
	if rs.Extras == nil {
		t.Fatal("expected extras")
	}
}
