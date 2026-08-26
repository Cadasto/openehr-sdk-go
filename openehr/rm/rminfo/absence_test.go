package rminfo_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// absenceUniverse is the synthetic class universe the absence tests reason
// over. Two members is enough: the questions here are about membership and the
// negative space behind a "no", not about attribute shapes.
func absenceUniverse() map[string]rminfo.ClassMeta {
	return map[string]rminfo.ClassMeta{
		"IN_UNIVERSE":      {},
		"ALSO_IN_UNIVERSE": {},
	}
}

// reporter asserts a Lookup carries the optional capability interface and
// returns it. Consumers do exactly this assertion, so a signature drift on
// AbsenceReporter must fail here rather than at some consumer's call site.
func reporter(t *testing.T, l rminfo.Lookup) rminfo.AbsenceReporter {
	t.Helper()
	r, ok := l.(rminfo.AbsenceReporter)
	if !ok {
		t.Fatal("Lookup does not implement AbsenceReporter")
	}
	return r
}

// TestAbsenceNoneIsTheDistinguishedZeroValue — REQ-049. The none answer is the
// reason type's zero value and every real reason differs from it, so the
// accessor cannot be misread as a membership test with inverted sense. The
// reasons are also pairwise distinct: collapsing two would re-fold facts the
// requirement exists to separate.
func TestAbsenceNoneIsTheDistinguishedZeroValue(t *testing.T) {
	var zero rminfo.AbsenceReason
	if zero != rminfo.AbsenceNone {
		t.Errorf("zero value = %v, want AbsenceNone", zero)
	}
	reasons := map[string]rminfo.AbsenceReason{
		"AbsenceUndeclared":      rminfo.AbsenceUndeclared,
		"AbsenceExcludedPackage": rminfo.AbsenceExcludedPackage,
		"AbsenceExcludedClass":   rminfo.AbsenceExcludedClass,
		"AbsencePrimitive":       rminfo.AbsencePrimitive,
		"AbsenceEnumeration":     rminfo.AbsenceEnumeration,
	}
	for name, r := range reasons {
		if r == rminfo.AbsenceNone {
			t.Errorf("%s == AbsenceNone: a real reason must not equal the none answer", name)
		}
		for otherName, other := range reasons {
			if name != otherName && r == other {
				t.Errorf("%s == %s: reasons must be pairwise distinct", name, otherName)
			}
		}
	}
}

// TestNewFallsBackToUndeclared — REQ-049 § Synthetic seam. A Lookup built
// without an absence table reports undeclared for every out-of-universe name,
// and none for every member. That is the documented fallback, not a defect.
func TestNewFallsBackToUndeclared(t *testing.T) {
	r := reporter(t, rminfo.New(absenceUniverse()))
	cases := []struct {
		rmType string
		want   rminfo.AbsenceReason
	}{
		{"IN_UNIVERSE", rminfo.AbsenceNone},
		{"ALSO_IN_UNIVERSE", rminfo.AbsenceNone},
		{"NOT_IN_UNIVERSE", rminfo.AbsenceUndeclared},
		{"", rminfo.AbsenceUndeclared},
	}
	for _, tc := range cases {
		if got := r.AbsenceReason(tc.rmType); got != tc.want {
			t.Errorf("AbsenceReason(%q) = %v, want %v", tc.rmType, got, tc.want)
		}
	}
}

// TestNewWithAbsenceReportsTheStoredReason — REQ-049 § The reason taxonomy.
// Each member of the closed set survives the round trip through the synthetic
// table, and a name in neither map still falls back to undeclared.
func TestNewWithAbsenceReportsTheStoredReason(t *testing.T) {
	r := reporter(t, rminfo.NewWithAbsence(absenceUniverse(), map[string]rminfo.AbsenceReason{
		"SKIPPED_PACKAGE_CLASS": rminfo.AbsenceExcludedPackage,
		"EXCLUDED_BY_NAME":      rminfo.AbsenceExcludedClass,
		"A_PRIMITIVE":           rminfo.AbsencePrimitive,
		"AN_ENUMERATION":        rminfo.AbsenceEnumeration,
		// Undeclared is computed, never stored — but a table that stores
		// it anyway must not be second-guessed into a different answer.
		"STORED_UNDECLARED": rminfo.AbsenceUndeclared,
	}))
	cases := []struct {
		rmType string
		want   rminfo.AbsenceReason
	}{
		{"SKIPPED_PACKAGE_CLASS", rminfo.AbsenceExcludedPackage},
		{"EXCLUDED_BY_NAME", rminfo.AbsenceExcludedClass},
		{"A_PRIMITIVE", rminfo.AbsencePrimitive},
		{"AN_ENUMERATION", rminfo.AbsenceEnumeration},
		{"STORED_UNDECLARED", rminfo.AbsenceUndeclared},
		{"IN_UNIVERSE", rminfo.AbsenceNone},
		{"IN_NEITHER_MAP", rminfo.AbsenceUndeclared},
	}
	for _, tc := range cases {
		if got := r.AbsenceReason(tc.rmType); got != tc.want {
			t.Errorf("AbsenceReason(%q) = %v, want %v", tc.rmType, got, tc.want)
		}
	}
}

// TestUniverseWinsOverAnAbsenceEntry — REQ-049 § Synthetic seam. Where the
// class data and the absence table name the same class the universe wins, so a
// member can never be reported absent — the two surfaces must not disagree
// about membership.
func TestUniverseWinsOverAnAbsenceEntry(t *testing.T) {
	r := reporter(t, rminfo.NewWithAbsence(absenceUniverse(), map[string]rminfo.AbsenceReason{
		"IN_UNIVERSE": rminfo.AbsencePrimitive,
	}))
	if got := r.AbsenceReason("IN_UNIVERSE"); got != rminfo.AbsenceNone {
		t.Errorf("AbsenceReason(IN_UNIVERSE) = %v, want AbsenceNone (the universe wins)", got)
	}
}

// TestAbsenceReasonString — REQ-049. The string form names the kind only: it
// never echoes the queried name, and a value outside the closed set formats
// numerically rather than masquerading as a member.
func TestAbsenceReasonString(t *testing.T) {
	cases := []struct {
		reason rminfo.AbsenceReason
		want   string
	}{
		{rminfo.AbsenceNone, "none"},
		{rminfo.AbsenceUndeclared, "undeclared"},
		{rminfo.AbsenceExcludedPackage, "excluded package"},
		{rminfo.AbsenceExcludedClass, "excluded class"},
		{rminfo.AbsencePrimitive, "primitive"},
		{rminfo.AbsenceEnumeration, "enumeration"},
		{rminfo.AbsenceReason(42), "absence reason 42"},
		{rminfo.AbsenceReason(-1), "absence reason -1"},
	}
	for _, tc := range cases {
		if got := tc.reason.String(); got != tc.want {
			t.Errorf("AbsenceReason(%d).String() = %q, want %q", int(tc.reason), got, tc.want)
		}
	}
}

// TestDefaultImplementsAbsenceReporter — REQ-049 § Surface shape. Default
// carries the capability; consumers reach it by exactly this assertion.
//
// The out-of-universe ANSWERS are deliberately not asserted here: Default's
// absence table is still nil at this point on the branch, so every such name
// reports undeclared. The generated table (REQ-042) lands in the next commit,
// and PROBE-098 is what checks the answers against the pinned BMM.
func TestDefaultImplementsAbsenceReporter(t *testing.T) {
	r := reporter(t, rminfo.Default)
	if got := r.AbsenceReason("COMPOSITION"); got != rminfo.AbsenceNone {
		t.Errorf("AbsenceReason(COMPOSITION) = %v, want AbsenceNone (a universe member)", got)
	}
}
