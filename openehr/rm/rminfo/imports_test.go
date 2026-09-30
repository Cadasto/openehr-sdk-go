package rminfo_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// REQ-048 — the introspection surface MUST NOT load, parse, or resolve a BMM
// schema at runtime, and doc.go promises a stdlib-only building block. The
// data is compiled in, so the package needs no import beyond the standard
// library; any module-path import appearing here is what this tripwire
// exists for — most pointedly openehr/bmm, which would turn the compiled-in
// table back into a runtime reduction.
//
// It reads every non-test file that any build compiles, not only the ones this
// machine builds (importguard.Imports), and importguard.TestStandard is the
// can-fail control for the standard-library check. Test files may import
// anything: the PROBE-094 suite deliberately imports openehr/bmm to re-derive
// the table from the pinned schemas.
func TestRMInfoImportsAreStdlibOnly(t *testing.T) {
	t.Parallel()
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if !importguard.Standard(imp) {
			t.Errorf("openehr/rm/rminfo imports %q — the surface is stdlib-only (REQ-048: no runtime BMM dependency)", imp)
		}
	}
}
