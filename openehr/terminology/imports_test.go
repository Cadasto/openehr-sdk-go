package terminology_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-034 § openEHR terminology vocabulary — the accessor MUST be a
// stdlib-only building block, so it sits *below* openehr/rm and RM-level
// consumers can adopt it without a dependency cycle. doc.go promises the
// same thing, and it joins the REQ-013 building-block-independence set on
// those terms.
//
// The rule enforced here is stdlib-only, not merely in-module-free: any
// import whose first path element contains a dot is a module path (standard
// library import paths never carry one), so it fails — this module's own
// packages (most pointedly openehr/rm, which would invert the dependency
// direction) and third-party modules alike. Enumerating a forbidden subset
// would let a new dependency slip in under a name nobody thought to list.
// Same rule as the sibling stdlib-only block, openehr/rm/rminfo.
//
// Non-test files only: a test file may import whatever it needs (Task 3's
// drift check re-hashes the pinned XML, for instance).
func TestTerminologyForbiddenImports(t *testing.T) {
	t.Parallel()
	pkg, err := build.Default.ImportDir("./", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	if len(pkg.GoFiles) == 0 {
		t.Fatal("no non-test Go files enumerated — the tripwire is vacuous")
	}
	for _, imp := range pkg.Imports {
		if isModulePath(imp) {
			t.Errorf("openehr/terminology imports %q — the package is stdlib-only so openehr/rm can import it (REQ-034; REQ-013 building-block independence)", imp)
		}
	}
}

// isModulePath reports whether imp is a module import path rather than a
// standard-library one: standard-library paths never carry a dot in their
// first element, so any first element containing one is a module (this
// module's own packages and third-party modules alike).
func isModulePath(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return strings.Contains(first, ".")
}

// TestIsModulePath is the tripwire's can-fail control: the reject arm of
// [TestTerminologyForbiddenImports] never fires on the package's real imports
// (iter, slices), so an inverted predicate would leave that test green while
// silently disabling the REQ-013 guard. This pins the classifier directly —
// stdlib paths pass, module paths (openehr/rm most pointedly) are caught.
func TestIsModulePath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		imp  string
		want bool
	}{
		{"iter", false},
		{"slices", false},
		{"go/build", false},
		{"encoding/xml", false},
		{"github.com/cadasto/openehr-sdk-go/openehr/rm", true},
		{"golang.org/x/text/unicode/norm", true},
		{"example.com/foo", true},
	}
	for _, tc := range cases {
		if got := isModulePath(tc.imp); got != tc.want {
			t.Errorf("isModulePath(%q) = %v, want %v", tc.imp, got, tc.want)
		}
	}
}
