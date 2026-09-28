package template_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestTemplateForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013): openehr/template, and every
// package of this module it pulls in, must not import transport, auth or
// openehr/client. openehr/serialize is not forbidden here: REQ-013 does not bar
// openehr/template from it.
func TestTemplateForbiddenImports(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		"github.com/cadasto/openehr-sdk-go/transport",
		"github.com/cadasto/openehr-sdk-go/auth",
		"github.com/cadasto/openehr-sdk-go/openehr/client",
	}
	if len(forbidden) == 0 {
		t.Fatal("forbidden list is empty; the guard is vacuous")
	}

	violations, err := importguard.Scan(".", forbidden)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/template MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}
}
