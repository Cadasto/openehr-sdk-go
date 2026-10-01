package rminfo_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// REQ-048 — the introspection surface MUST NOT load, parse, or resolve a BMM
// schema at runtime, and the package's own non-test files MUST import only
// the standard library; doc.go promises the same. The data is compiled in, so
// the package needs no import beyond the standard library; any module-path
// import appearing here is what this tripwire exists for — most pointedly
// openehr/bmm, which would turn the compiled-in table back into a runtime
// reduction.
//
// It reads every non-test file that any build compiles, not only the ones this
// machine builds (importguard.Imports), and importguard.TestStandard is the
// can-fail control for the standard-library check. Test files may import
// anything: the PROBE-094 suite deliberately imports openehr/bmm to re-derive
// the table from the pinned schemas.
//
// nonStdlibImportMessage names both rules a failing import violates. The
// cases below pin that text: deleting "stdlib-only" or "no runtime BMM" from
// its format fails this test.
func TestRMInfoImportsAreStdlibOnly(t *testing.T) {
	t.Parallel()

	const sample = "github.com/cadasto/openehr-sdk-go/openehr/bmm"
	msg := nonStdlibImportMessage(sample)
	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "REQ-048 stdlib-only", want: "stdlib-only"},
		{name: "REQ-048 no runtime BMM", want: "no runtime BMM"},
		{name: "REQ-048 import path", want: sample},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(msg, tc.want) {
				t.Errorf("message %q missing %q", msg, tc.want)
			}
		})
	}

	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if !importguard.Standard(imp) {
			t.Error(nonStdlibImportMessage(imp))
		}
	}
}

// nonStdlibImportMessage reports a non-stdlib import of openehr/rm/rminfo.
// REQ-048 forbids that import for two reasons: the package's own non-test
// files are stdlib-only, and the surface has no runtime BMM dependency.
func nonStdlibImportMessage(imp string) string {
	return fmt.Sprintf("openehr/rm/rminfo imports %q — own non-test files are stdlib-only and have no runtime BMM dependency (REQ-048)", imp)
}
