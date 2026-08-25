package aql_test

import (
	"go/build"
	"strings"
	"testing"
)

// TestAQLForbiddenImports pins REQ-162 § Building-block independence (REQ-013):
// openehr/aql gained its first in-module dependencies with REQ-162 — TWO of
// them, openehr/aql/contain (whose own guard keeps that arrow pointing down at
// openehr/rm) and the Go-internal openehr/aql/internal/semcheck, the shared
// verdict→code engine the read-side linter also consumes (module-layout.md
// § REQ-013 names both) — and the package MUST stay importable without the wire
// layers, so a program that only CONSTRUCTS and verifies AQL still names no
// transport, no auth, and no client.
//
// openehr/validation is forbidden for the same reason openehr/aql/lint forbids
// it: the arrow is validation → aql, never the reverse. openehr/aql/lint and
// openehr/aql/parse are not listed because Go refuses those itself — both import
// openehr/aql, so either would be an import cycle rather than a layering slip.
//
// SCOPE — what this guard does and does not cover. It reads the DIRECT imports
// of this package's non-test files, so it says nothing about what those imports
// transitively pull, and the openehr/serialize entry below is a ban on NAMING
// that package here rather than a claim about the build closure. The
// distinction is live, not theoretical: since REQ-162,
// `go list -deps ./openehr/aql` reaches openehr/serialize/canxml and
// openehr/serialize/internal/poly through openehr/aql/contain → openehr/rm →
// openehr/rm/typereg, so "pulls in no serialisation" would be FALSE if read
// transitively — before REQ-162 the package had no in-module imports at all and
// the two readings coincided. REQ-013's actual MUST is unaffected and still
// holds: the package stays importable and useful without constructing an
// authenticated client or instantiating transport/ or auth/, and none of
// transport/, auth/, openehr/client/*, or openehr/validation appears anywhere in
// that closure. Serialisation is a peer building block under the same REQ, not
// a wire layer.
//
// A blacklist, deliberately, matching every other building-block guard in the
// repo (openehr/aql/lint, openehr/aql/parse): the rule is "no wire layers", not
// "no new dependencies", so a legitimate future dependency does not have to
// amend the guard to be allowed.
func TestAQLForbiddenImports(t *testing.T) {
	pkg, err := build.Default.ImportDir("./", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	if len(pkg.GoFiles) == 0 {
		t.Fatal("ImportDir enumerated no non-test Go files; the guard is vacuous")
	}
	forbidden := []string{
		"openehr/serialize",
		"openehr/client",
		"openehr/validation",
		"github.com/cadasto/openehr-sdk-go/transport",
		"github.com/cadasto/openehr-sdk-go/auth",
	}
	for _, imp := range pkg.Imports {
		for _, bad := range forbidden {
			if strings.Contains(imp, bad) {
				t.Errorf("openehr/aql MUST NOT import %q (REQ-013; matched %q)", imp, bad)
			}
		}
	}
}
