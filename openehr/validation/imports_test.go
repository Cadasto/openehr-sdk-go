package validation_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-013 § Building-block independence — openehr/validation MUST
// NOT depend on any of the wire / transport / auth / client
// layers. The validator operates on in-memory RM graphs; wire
// decoding, network calls, and authentication belong to callers.
// Pulling any of these into the validator's transitive deps would
// invert the dependency direction (validators feed into codecs and
// clients, not the reverse) and bloat any consumer that just wants
// in-memory checks.
//
// Forbidden prefixes — the full set enumerated by REQ-102 §
// Building-block independence in docs/specifications/clinical-modeling.md:
//
//   - openehr/serialize  (wire-byte codecs)
//   - openehr/client     (REST clients)
//   - transport          (HTTP transport)
//   - auth               (authentication)
//
// The check enumerates non-test files only — the test files are
// allowed to import these for fixture decode.
func TestValidationForbiddenImports(t *testing.T) {
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
				t.Errorf("openehr/validation MUST NOT import %q (REQ-013 building-block independence; matched forbidden prefix %q)", imp, bad)
			}
		}
	}
}
