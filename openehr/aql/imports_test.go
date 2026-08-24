package aql_test

import (
	"go/build"
	"strings"
	"testing"
)

// TestAQLForbiddenImports pins REQ-162 § Building-block independence (REQ-013):
// openehr/aql gained its first in-module dependency with REQ-162 — an import of
// openehr/aql/contain, whose own guard keeps that arrow pointing down at
// openehr/rm — and the package MUST stay importable without the wire layers, so
// a program that only CONSTRUCTS and verifies AQL still pulls in no transport,
// no auth, no client, and no serialisation.
//
// openehr/validation is forbidden for the same reason openehr/aql/lint forbids
// it: the arrow is validation → aql, never the reverse. openehr/aql/lint and
// openehr/aql/parse are not listed because Go refuses those itself — both import
// openehr/aql, so either would be an import cycle rather than a layering slip.
//
// A blacklist, deliberately, matching every other building-block guard in the
// repo (openehr/aql/lint, openehr/aql/parse): the rule is "no wire layers", not
// "no new dependencies", so a legitimate future dependency does not have to
// amend the guard to be allowed. Direct imports of the non-test files only —
// what this package itself names, not what its dependencies transitively pull.
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
