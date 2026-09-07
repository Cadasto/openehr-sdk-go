package sandbox_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNoListenerImports guards REQ-082 (sandbox/ serves every
// backend-facing probe with no network listener and no credentials)
// and REQ-013 (building-block independence): the package's non-test
// source MUST NOT import net/http/httptest, net (the only listener
// API), transport, or auth — no matter what a future edit adds, since
// a grep-based check can be fooled by a renamed import but a parsed
// import path cannot.
//
// Parses with go/parser in ImportsOnly mode rather than go/build: it
// reads only the import declarations, so it is cheap, and it lets
// this test decide for itself which files count as "non-test" instead
// of trusting go/build's own filtering.
func TestNoListenerImports(t *testing.T) {
	forbidden := []string{
		"net/http/httptest",
		"net",
		"github.com/cadasto/openehr-sdk-go/transport",
		"github.com/cadasto/openehr-sdk-go/auth",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob sandbox/*.go: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no .go files found — check the working directory (must be sandbox/)")
	}

	fset := token.NewFileSet()
	seenNonTest := false
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		seenNonTest = true
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquote import %s: %v", name, imp.Path.Value, err)
			}
			for _, bad := range forbidden {
				if path == bad {
					t.Errorf("%s imports %q — sandbox/ MUST NOT depend on it (REQ-082 no listener, REQ-013 independence)", name, path)
				}
			}
		}
	}
	if !seenNonTest {
		t.Fatal("every file matched *.go was a _test.go file — glob pattern is wrong")
	}
}
