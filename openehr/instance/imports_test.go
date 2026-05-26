package instance_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-013 § Building-block independence — openehr/instance MUST
// NOT depend on wire / transport / auth / client layers, nor on
// the composition builder (REQ-101 consumes instance, not the
// reverse) or the validator (validation depends on instance only
// via cross-package probes/tests).
//
// Forbidden prefixes — REQ-107 § Building-block independence:
//
//   - openehr/serialize       (wire-byte codecs)
//   - openehr/client          (REST clients)
//   - openehr/composition     (REQ-101 builder)
//   - openehr/validation      (validator)
//   - github.com/cadasto/openehr-sdk-go/transport
//   - github.com/cadasto/openehr-sdk-go/auth
//
// Non-test files only — test files are allowed to import these for
// integration / cross-package probes (e.g. testkit/probes/instance/
// imports openehr/validation).
func TestInstanceForbiddenImports(t *testing.T) {
	pkg, err := build.Default.ImportDir("./", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	forbidden := []string{
		"openehr/serialize",
		"openehr/client",
		"openehr/composition",
		"openehr/validation",
		"github.com/cadasto/openehr-sdk-go/transport",
		"github.com/cadasto/openehr-sdk-go/auth",
	}
	for _, imp := range pkg.Imports {
		for _, bad := range forbidden {
			if strings.Contains(imp, bad) {
				t.Errorf("openehr/instance MUST NOT import %q (REQ-013 building-block independence; matched forbidden prefix %q)", imp, bad)
			}
		}
	}
}
