package aql_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

// referenceQuery builds "all OBSERVATIONs of archetype body_temperature for a
// given EHR" — the PROBE-020 reference query whose canonical form is pinned in
// testdata/wire/observations_by_archetype.aql (REQ-055).
func referenceQuery() (aql.Query, error) {
	return aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("COMPOSITION", "c", "")).
		Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2")).
		Build()
}

func TestBuilderMatchesGolden(t *testing.T) {
	q, err := referenceQuery()
	if err != nil {
		t.Fatal(err)
	}
	golden := readGolden(t, "observations_by_archetype.aql")
	if q.String() != golden {
		t.Fatalf("built query does not match golden:\n got: %q\nwant: %q", q.String(), golden)
	}
}

func TestBuilderClauses(t *testing.T) {
	// FromEHR with a nil id emits a bare FROM EHR e, isolating this test to
	// the explicit WHERE / ORDER BY emission mechanics.
	q, err := aql.NewBuilder().
		Select(aql.Col("o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude"), aql.Col("o/name/value")).
		FromEHR("e", nil).
		Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2")).
		Where(aql.And(
			aql.Gt("o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude", aql.Real(37.5)),
			aql.Or(
				aql.Eq("o/name/value", aql.String("Temperature")),
				aql.Eq("o/name/value", aql.String("O'Brien")),
			),
		)).
		OrderBy("o/name/value", aql.Descending).
		Offset(10).
		Limit(20).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	const want = "SELECT o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude, o/name/value " +
		"FROM EHR e " +
		"CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] " +
		"WHERE o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude > 37.5 " +
		"AND (o/name/value = 'Temperature' OR o/name/value = 'O''Brien') " +
		"ORDER BY o/name/value DESC"
	if q.String() != want {
		t.Fatalf("clause emission mismatch:\n got: %q\nwant: %q", q.String(), want)
	}
	// Paging lives in the request envelope, not the AQL string.
	if q.Offset != 10 || q.Fetch != 20 {
		t.Fatalf("paging: Offset=%d Fetch=%d, want 10/20", q.Offset, q.Fetch)
	}
}

// TestFromEHRInjectsWhere locks the idiomatic ehr_id scoping: FromEHR emits the
// EHR-scope condition into WHERE (AND-combined with the explicit predicate),
// not as a standing predicate on the EHR class.
func TestFromEHRInjectsWhere(t *testing.T) {
	q, err := aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2")).
		Where(aql.Gt("o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude", aql.Real(37.5))).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	const want = "SELECT o " +
		"FROM EHR e " +
		"CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] " +
		"WHERE e/ehr_id/value = $ehr_id " +
		"AND o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude > 37.5"
	if q.String() != want {
		t.Fatalf("ehr_id injection mismatch:\n got: %q\nwant: %q", q.String(), want)
	}
}

// TestEmptyJunctionYieldsNoWhere locks the PROBE-021 structural guarantee: a
// vacuous predicate (And/Or with no surviving terms) emits no WHERE rather than
// a trailing, invalid one — an empty conjunction is logically true.
func TestEmptyJunctionYieldsNoWhere(t *testing.T) {
	q, err := aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", nil).
		Where(aql.And()).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q.String(), "WHERE") {
		t.Fatalf("expected no WHERE clause, got %q", q.String())
	}
}

// TestJunctionDropsNilTerms verifies nil terms are pruned so a one-survivor
// junction collapses to that term (no stray AND/OR, no parentheses).
func TestJunctionDropsNilTerms(t *testing.T) {
	q, err := aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", nil).
		Where(aql.And(aql.Eq("o/name/value", aql.String("Temperature")), aql.Or())).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	const want = "SELECT o FROM EHR e WHERE o/name/value = 'Temperature'"
	if q.String() != want {
		t.Fatalf("nil-term pruning mismatch:\n got: %q\nwant: %q", q.String(), want)
	}
}

func TestBuilderBindPopulatesParameters(t *testing.T) {
	q, err := aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).
		Bind("ehr_id", "7d44b88c-4199-4bad-97dc-d78268e01398").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if got := q.Parameters["ehr_id"]; got != "7d44b88c-4199-4bad-97dc-d78268e01398" {
		t.Fatalf("Parameters[ehr_id] = %v", got)
	}
	if !strings.Contains(q.String(), "$ehr_id") {
		t.Fatalf("expected placeholder in %q", q.String())
	}
}

func TestParamStripsLeadingDollar(t *testing.T) {
	q, err := aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", aql.Param("$ehr_id")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q.String(), "$$") {
		t.Fatalf("doubled dollar in %q", q.String())
	}
}

func TestBuilderBuildErrors(t *testing.T) {
	tests := map[string]*aql.Builder{
		"no select": aql.NewBuilder().FromEHR("e", nil),
		"no from":   aql.NewBuilder().Select(aql.Col("o")),
	}
	for name, b := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := b.Build(); !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

// TestFromClearsStaleEHRFilter locks the rescope/clear semantics: replacing or
// re-scoping the FROM source must not leave a dangling ehr_id WHERE condition.
func TestFromClearsStaleEHRFilter(t *testing.T) {
	t.Run("rescope to non-EHR drops filter", func(t *testing.T) {
		q, err := aql.NewBuilder().Select(aql.Col("o")).
			FromEHR("e", aql.Param("x")).From("COMPOSITION", "c").Build()
		if err != nil {
			t.Fatal(err)
		}
		if got := q.String(); got != "SELECT o FROM COMPOSITION c" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("FromEHR(nil) clears prior filter", func(t *testing.T) {
		q, err := aql.NewBuilder().Select(aql.Col("o")).
			FromEHR("e", aql.Param("x")).FromEHR("e", nil).Build()
		if err != nil {
			t.Fatal(err)
		}
		if got := q.String(); got != "SELECT o FROM EHR e" {
			t.Fatalf("got %q", got)
		}
	})
}

// TestBuildRejectsMalformedInput locks the PROBE-021 structural guarantee for
// consumer-supplied empties and nils — Build errors rather than emitting
// invalid AQL or panicking.
func TestBuildRejectsMalformedInput(t *testing.T) {
	tests := map[string]*aql.Builder{
		"empty select field": aql.NewBuilder().Select(aql.Col("")).FromEHR("e", nil),
		"empty from alias":   aql.NewBuilder().Select(aql.Col("o")).From("EHR", ""),
		"empty contains":     aql.NewBuilder().Select(aql.Col("o")).FromEHR("e", nil).Contains(aql.Archetype("", "o", "")),
		"nil comparison val": aql.NewBuilder().Select(aql.Col("o")).FromEHR("e", nil).Where(aql.Eq("o/x", nil)),
		"empty where path":   aql.NewBuilder().Select(aql.Col("o")).FromEHR("e", nil).Where(aql.Eq("", aql.Int(1))),
	}
	for name, b := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := b.Build(); !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

// TestBuildClonesParameters verifies the built query does not alias the
// builder's internal parameter map.
func TestBuildClonesParameters(t *testing.T) {
	b := aql.NewBuilder().Select(aql.Col("o")).FromEHR("e", aql.Param("x")).Bind("x", "v1")
	first, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	first.Parameters["x"] = "mutated"
	second, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	if second.Parameters["x"] != "v1" {
		t.Fatalf("parameter map aliased: got %v", second.Parameters["x"])
	}
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "wire", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(string(data), "\n")
}
