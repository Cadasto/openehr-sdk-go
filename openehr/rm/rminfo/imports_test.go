package rminfo_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-048 — the introspection surface MUST NOT load, parse, or resolve a BMM
// schema at runtime, and doc.go promises a stdlib-only building block. The
// data is compiled in, so the package needs no import beyond the standard
// library; any module-path import appearing here is what this tripwire
// exists for — most pointedly openehr/bmm, which would turn the compiled-in
// table back into a runtime reduction.
//
// Non-test files only: the PROBE-094 suite deliberately imports openehr/bmm
// to re-derive the table from the pinned schemas.
func TestRMInfoImportsAreStdlibOnly(t *testing.T) {
	pkg, err := build.Default.ImportDir("./", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	if len(pkg.GoFiles) == 0 {
		t.Fatal("no non-test Go files enumerated — the tripwire is vacuous")
	}
	for _, imp := range pkg.Imports {
		// A dot in the first path element marks a module path; standard
		// library import paths never carry one.
		first, _, _ := strings.Cut(imp, "/")
		if strings.Contains(first, ".") {
			t.Errorf("openehr/rm/rminfo imports %q — the surface is stdlib-only (REQ-048: no runtime BMM dependency)", imp)
		}
	}
}
