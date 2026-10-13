package rmaccess_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestRMAccessImports guards the package's place below the validators
// (ADR 0028): its own non-test files import only the standard library,
// openehr/rm and openehr/rm/rminfo, so it never reaches the validation,
// template, serialization or client packages it is read by, and nothing it
// pulls in reaches transport, auth or openehr/client (REQ-013).
func TestRMAccessImports(t *testing.T) {
	t.Parallel()
	allowed := map[string]bool{
		"github.com/cadasto/openehr-sdk-go/openehr/rm":        true,
		"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo": true,
	}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(imports) == 0 {
		t.Fatal("importguard.Imports(.) found no import; the scan has gone blind")
	}
	for _, imp := range imports {
		if !importguard.Standard(imp) && !allowed[imp] {
			t.Errorf("internal/rmaccess MUST NOT import %q in its own files: it imports only the standard library, openehr/rm and openehr/rm/rminfo (ADR 0028)", imp)
		}
	}
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("internal/rmaccess MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013)", v.Import, v.Importer, v.Prefix)
	}
}
