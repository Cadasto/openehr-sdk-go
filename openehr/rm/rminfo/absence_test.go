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

// TestStoredNoneIsTableSilence — REQ-049 § Surface shape. A synthetic absence
// entry whose value is the *none* reason is treated as table silence: the name
// is not in the class data, so reporting *none* for it would claim a
// membership KnownRMTypes() does not list, and the two surfaces must not
// disagree. The answer is *undeclared*, exactly as for a name in neither map.
func TestStoredNoneIsTableSilence(t *testing.T) {
	l := rminfo.NewWithAbsence(absenceUniverse(), map[string]rminfo.AbsenceReason{
		"STORED_NONE": rminfo.AbsenceNone,
	})
	r := reporter(t, l)
	if got := r.AbsenceReason("STORED_NONE"); got != rminfo.AbsenceUndeclared {
		t.Errorf("AbsenceReason(STORED_NONE) = %v, want AbsenceUndeclared (a stored none is table silence)", got)
	}
	for _, name := range l.KnownRMTypes() {
		if name == "STORED_NONE" {
			t.Error("KnownRMTypes() lists STORED_NONE: an absence entry must not add a name to the universe")
		}
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
// The answers come from the GENERATED table (absence_gen.go), so what this
// pins is the wiring — that Default was built WITH that table, one name per
// reason, rather than with the nil table that reports undeclared for
// everything. Whether the whole table agrees with the pinned BMM is
// PROBE-098's question; whether it is what the generator emits is
// `make codegen-verify`'s.
func TestDefaultImplementsAbsenceReporter(t *testing.T) {
	r := reporter(t, rminfo.Default)
	cases := map[string]rminfo.AbsenceReason{
		"COMPOSITION":     rminfo.AbsenceNone,            // a universe member
		"EXTRACT":         rminfo.AbsenceExcludedPackage, // org.openehr.rm.ehr_extract
		"TUPLE":           rminfo.AbsenceExcludedClass,
		"PROPORTION_KIND": rminfo.AbsenceEnumeration,
		"Interval":        rminfo.AbsencePrimitive,  // a Go type is emitted for it either way
		"NO_SUCH_CLASS":   rminfo.AbsenceUndeclared, // computed from the table's silence
	}
	for name, want := range cases {
		if got := r.AbsenceReason(name); got != want {
			t.Errorf("AbsenceReason(%s) = %v, want %v", name, got, want)
		}
	}
}
