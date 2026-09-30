package serialize_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestSerializeForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for the openehr/serialize
// package itself, which REQ-013 names as a building block: neither it nor any
// package of this module it pulls in may import transport, auth or
// openehr/client. The package holds only its documentation today, and no
// sub-package imports it, so the sub-package guards cannot catch an import
// added here.
func TestSerializeForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/serialize MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}
}
