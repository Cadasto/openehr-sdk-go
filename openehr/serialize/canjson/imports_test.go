package canjson_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestCanJSONForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013): openehr/serialize/canjson,
// and every package of this module it pulls in, must not import transport,
// auth or openehr/client. openehr/serialize is not forbidden here: canjson is
// part of it and imports openehr/serialize/internal/poly.
func TestCanJSONForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/serialize/canjson MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}
}

// TestCanJSONDoesNotImportTheRM guards the dependency direction
// docs/specifications/module-layout.md § Dependency direction draws:
// openehr/serialize/canjson reaches openehr/rm/typereg and never imports
// openehr/rm, directly or through a package it pulls in, such as the
// unmatched-member walker. Through openehr/rm it would also pull in
// openehr/serialize/canxml, which is forbidden here too. The walk covers
// every package of this module in canjson's import closure; reaching
// openehr/rm/typereg is the control that it walks at all.
func TestCanJSONDoesNotImportTheRM(t *testing.T) {
	t.Parallel()
	const module = "github.com/cadasto/openehr-sdk-go"
	forbidden := []string{module + "/openehr/rm", module + "/openehr/serialize/canxml"}
	importers := map[string]string{} // in-module import path → a package that imports it
	queue := []string{module + "/openehr/serialize/canjson"}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		rel := strings.TrimPrefix(strings.TrimPrefix(pkg, module), "/")
		imports, err := importguard.Imports(filepath.Join("..", "..", "..", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read the imports of %s: %v", pkg, err)
		}
		for _, imp := range imports {
			if _, seen := importers[imp]; seen || !strings.HasPrefix(imp, module+"/") {
				continue
			}
			importers[imp] = pkg
			queue = append(queue, imp)
		}
	}
	if _, ok := importers[module+"/openehr/rm/typereg"]; !ok {
		t.Fatalf("the walk of canjson's imports never reached openehr/rm/typereg; it checks nothing (reached %d packages)", len(importers))
	}
	for _, f := range forbidden {
		if importer, ok := importers[f]; ok {
			t.Errorf("openehr/serialize/canjson MUST NOT pull in %q: %s imports it (module-layout.md § Dependency direction: neither canjson nor canxml imports openehr/rm)", f, importer)
		}
	}
}
