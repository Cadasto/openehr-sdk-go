package composition_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-013 § Building-block independence — openehr/composition MUST
// NOT depend on wire / transport / auth / client layers. The
// composition builder is a building block above openehr/instance and
// openehr/template; it stays usable without constructing an
// authenticated client.
//
// Forbidden prefixes:
//
//   - openehr/serialize       (wire-byte codecs — caller imports separately)
//   - openehr/client          (REST clients)
//   - github.com/cadasto/openehr-sdk-go/transport
//   - github.com/cadasto/openehr-sdk-go/auth
//
// Non-test files only — test files are allowed to import openehr/serialize
// (canjson round-trip) and similar cross-package surfaces for assertions.
func TestCompositionForbiddenImports(t *testing.T) {
	pkg, err := build.Default.ImportDir("./", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	forbidden := []string{
		"openehr/serialize",
		"openehr/client",
		"github.com/cadasto/openehr-sdk-go/transport",
		"github.com/cadasto/openehr-sdk-go/auth",
	}
	for _, imp := range pkg.Imports {
		for _, bad := range forbidden {
			if strings.Contains(imp, bad) {
				t.Errorf("openehr/composition MUST NOT import %q (REQ-013 building-block independence; matched forbidden prefix %q)", imp, bad)
			}
		}
	}
}
