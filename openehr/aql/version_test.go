package aql_test

// version_test.go pins the REQ-163 version-predicate carrier: the three
// `versionPredicate` alternatives the builder can now spell, the canonical
// bracket text each emits, the refusals, and the end-to-end consequence the
// carrier exists for — a builder-emitted `[LATEST_VERSION]` raises no
// aql_version_no_predicate.
//
// The canonical strings asserted here MUST equal what openehr/aql/parse emits
// for the same query; that is held mechanically by the REQ-163 rows in
// containment_roundtrip_test.go, which re-parse and re-emit every one of them.
// PROBE-088 · PROBE-097

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
)

// versionQuery is the shared body of the REQ-163 fixtures: the compositions of
// one EHR reached through a VERSION containment carrying pred (nil for the
// predicate-less form). It is the shape containment.go already names as
// ordinary AQL both the parser and Emit round-trip.
func versionQuery(pred aql.VersionPredicate) (aql.Query, error) {
	return aql.Select(aql.Col("c/uid/value")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Version("v", pred).Contains(aql.Class("COMPOSITION", "c"))).
		Build()
}

// TestVersionPredicateEmission tables the canonical bracket of every shape the
// sealed vocabulary has — the two keywords uppercase with no padding, and the
// comparison with one space each side of its operator, which is what
// aql.Comparison already renders in WHERE (REQ-163 § Canonical spellings).
func TestVersionPredicateEmission(t *testing.T) {
	tests := []struct {
		name string
		pred aql.VersionPredicate
		want string
	}{
		{
			name: "latest version keyword",
			pred: aql.LatestVersion(),
			want: "VERSION v[LATEST_VERSION]",
		},
		{
			name: "all versions keyword",
			pred: aql.AllVersions(),
			want: "VERSION v[ALL_VERSIONS]",
		},
		{
			name: "standing comparison against a parameter",
			pred: aql.VersionCompare("commit_audit/time_committed/value", aql.OpGt, aql.Param("since")),
			want: "VERSION v[commit_audit/time_committed/value > $since]",
		},
		{
			name: "standing comparison against a literal",
			pred: aql.VersionCompare("commit_audit/change_type/value", aql.OpEq, aql.String("creation")),
			want: "VERSION v[commit_audit/change_type/value = 'creation']",
		},
		{
			// Absence stays legal and emits the pre-REQ-163 bytes.
			name: "no predicate",
			pred: nil,
			want: "VERSION v",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").
				Contains(aql.Version("v", tc.pred)).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			want := "SELECT x FROM EHR e CONTAINS " + tc.want
			if q.String() != want {
				t.Fatalf("version emission mismatch:\n got: %q\nwant: %q", q.String(), want)
			}
		})
	}
}

// TestVersionWithoutPredicateEqualsClass pins the additivity half of REQ-163
// § The version-predicate carrier: `Version(alias, nil)` is the predicate-less
// form, and it MUST stay identical — in behaviour and in bytes — to the
// `Class("VERSION", …)` spelling that was the builder's only VERSION shape
// before this REQ. A nil predicate means the bracket is ABSENT, never an error.
func TestVersionWithoutPredicateEqualsClass(t *testing.T) {
	viaVersion, err := versionQuery(nil)
	if err != nil {
		t.Fatalf("Version(\"v\", nil): %v", err)
	}
	viaClass, err := aql.Select(aql.Col("c/uid/value")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Class("VERSION", "v").Contains(aql.Class("COMPOSITION", "c"))).
		Build()
	if err != nil {
		t.Fatalf("Class(\"VERSION\", \"v\"): %v", err)
	}
	if viaVersion.String() != viaClass.String() {
		t.Fatalf("the predicate-less form is not the landed spelling:\n Version: %q\n   Class: %q",
			viaVersion.String(), viaClass.String())
	}
}

// TestVersionPredicateMatchesGolden pins the committed canonical form of each
// predicate shape. The goldens are new files, so no existing fixture is
// re-baselined (REQ-163 § Additivity).
func TestVersionPredicateMatchesGolden(t *testing.T) {
	for name, pred := range map[string]aql.VersionPredicate{
		"version_latest_version": aql.LatestVersion(),
		"version_all_versions":   aql.AllVersions(),
		"version_compare": aql.VersionCompare(
			"commit_audit/time_committed/value", aql.OpGt, aql.Param("since")),
	} {
		t.Run(name, func(t *testing.T) {
			q, err := versionQuery(pred)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if golden := readGolden(t, name+".aql"); q.String() != golden {
				t.Fatalf("built query does not match golden:\n   got: %q\n golden: %q", q.String(), golden)
			}
		})
	}
}

// TestVersionPredicateRefusals pins the build-time refusals of the carrier.
// Each MUST wrap aql.ErrInvalidQuery rather than emit text the parser rejects.
//
// The non-VERSION node is NOT here: [aql.Version] fixes the RM type, so no
// public route reaches that state and the guard is pinned through the internal
// seam instead (version_internal_test.go).
func TestVersionPredicateRefusals(t *testing.T) {
	sel := func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e")
	}
	refused := map[string]func() (aql.Query, error){
		// A malformed comparison, one refusal per malformation the carrier
		// itself can see (aql.Comparison.validate).
		"comparison with an unknown operator": func() (aql.Query, error) {
			return sel().Contains(aql.Version("v", aql.VersionCompare("commit_audit/x", "==", aql.Int(1)))).Build()
		},
		"comparison with an empty path": func() (aql.Query, error) {
			return sel().Contains(aql.Version("v", aql.VersionCompare("", aql.OpEq, aql.Int(1)))).Build()
		},
		"comparison with a nil value": func() (aql.Query, error) {
			return sel().Contains(aql.Version("v", aql.VersionCompare("commit_audit/x", aql.OpEq, nil))).Build()
		},
		// The witness that keeps the CARRIER's own check honest. A value with
		// no AQL spelling renders as its Go text (`+Inf`), which is a perfectly
		// shaped `<path> <op> <operand>` to the text guard below and so passes
		// it — only [aql.Comparison.validate] refuses it. Remove the carrier
		// check and this row is the one that fails.
		"comparison against a value with no AQL spelling": func() (aql.Query, error) {
			return sel().Contains(aql.Version("v",
				aql.VersionCompare("commit_audit/x", aql.OpGt, aql.Real(math.Inf(1))))).Build()
		},
		// …and one the carrier cannot: a path that closes the emitter's own
		// bracket early re-parses as a DIFFERENT query, which is what
		// ValidateVersionPredicate — the guard (*parse.Query).Emit already
		// applies here — refuses. Without that call this text builds.
		"comparison whose path escapes the bracket": func() (aql.Query, error) {
			return sel().Contains(aql.Version("v",
				aql.VersionCompare("x] CONTAINS COMPOSITION c[y", aql.OpEq, aql.Int(1)))).Build()
		},
		// The landed archetype-on-VERSION refusal is unchanged by the new
		// bracket beside it: the VERSION alternative has no archetype slot.
		"archetype predicate on a VERSION class": func() (aql.Query, error) {
			return sel().Contains(aql.Archetype("VERSION", "v", "openEHR-EHR-COMPOSITION.report.v1")).Build()
		},
		// The junction receiver: a VERSION operand is a CONTAINS child like
		// any other, so nesting it below a junction is refused by the landed
		// rule — the grammar admits no `(A OR B) CONTAINS C`.
		"version below a containment junction": func() (aql.Query, error) {
			return sel().Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")).
				Contains(aql.Version("v", aql.LatestVersion()))).Build()
		},
	}
	for name, build := range refused {
		t.Run(name, func(t *testing.T) {
			q, err := build()
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v (query %q), want ErrInvalidQuery", err, q.String())
			}
		})
	}
}

// TestVersionPredicateDiagnosticsAreValueFree pins that a refused bracket is
// reported through [aql.RedactPredicateValues] — the class predicate is where
// openEHR carries the identifiable root, and a Build error is what a consuming
// CDR logs (REQ-119 § the redaction rule, which ValidateVersionPredicate
// already applies; this asserts the write side reaches it).
func TestVersionPredicateDiagnosticsAreValueFree(t *testing.T) {
	// The witness has to be a predicate that BOTH refuses and carries a value:
	// the bracket-escaping path is what earns the refusal, the string literal
	// is what must not come back.
	const secret = "9d3d1b4e-0000-0000-0000-000000000000"
	_, err := aql.NewBuilder().Select(aql.Col("x")).From("EHR", "e").
		Contains(aql.Version("v", aql.VersionCompare(
			"uid/value] CONTAINS COMPOSITION c[name/value", aql.OpEq, aql.String(secret)))).
		Build()
	if !errors.Is(err, aql.ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery — the witness no longer refuses, so it proves nothing", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("refusal reproduced the predicate value: %v", err)
	}
}

// TestBuiltVersionPredicateSuppressesAdvisory is the REQ-163 § Acceptance row
// the whole carrier exists for: a builder-emitted `… CONTAINS VERSION
// v[LATEST_VERSION]` raises NO aql_version_no_predicate, the advisory the
// builder could not previously satisfy at all.
//
// The predicate-less control is not decoration — it pins that REQ-161's
// advisory still FIRES on the shape it was written for (REQ-163 § Additivity:
// no REQ-161 code, severity or Detail text changes), so a linter that had
// simply stopped raising the code could not pass this test.
func TestBuiltVersionPredicateSuppressesAdvisory(t *testing.T) {
	const code = "aql_version_no_predicate"
	counts := func(t *testing.T, pred aql.VersionPredicate) int {
		t.Helper()
		q, err := versionQuery(pred)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		n := 0
		for _, issue := range lint.LintString(q.String(), nil).Issues {
			if issue.Code == code {
				n++
			}
		}
		return n
	}
	for name, pred := range map[string]aql.VersionPredicate{
		"latest version": aql.LatestVersion(),
		"all versions":   aql.AllVersions(),
		"comparison":     aql.VersionCompare("commit_audit/time_committed/value", aql.OpGt, aql.Param("since")),
	} {
		t.Run(name, func(t *testing.T) {
			if n := counts(t, pred); n != 0 {
				t.Fatalf("%s fired %d times on an explicit version predicate, want 0", code, n)
			}
		})
	}
	t.Run("predicate-less control still fires", func(t *testing.T) {
		if n := counts(t, nil); n != 1 {
			t.Fatalf("%s fired %d times on a bare VERSION, want 1 — REQ-161's advisory is unchanged", code, n)
		}
	})
}
