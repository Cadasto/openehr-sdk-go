package sandbox_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestNoListenerImports guards REQ-082 (docs/specifications/conformance.md
// § REQ-082, Sandbox mode). The package's own non-test files must not import:
//
//   - "net" or "net/http/httptest", matched exactly. net/http is not on that
//     list, so net/http.ListenAndServe is not what this guard denies.
//   - every import of auth/ or transport/, including every package under
//     either path.
//
// importguard.Imports reads every non-test file that any build compiles,
// not only the ones this machine builds, and refuses a directory where
// none is left, so the guard cannot pass while checking nothing.
func TestNoListenerImports(t *testing.T) {
	t.Parallel()
	listeners := []string{"net", "net/http/httptest"}
	layers := []string{
		"github.com/cadasto/openehr-sdk-go/transport",
		"github.com/cadasto/openehr-sdk-go/auth",
	}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if slices.Contains(listeners, imp) {
			t.Errorf("sandbox/ MUST NOT import %q in its own files (REQ-082: no network listener)", imp)
		}
		if p, ok := importguard.Matches(imp, layers); ok {
			t.Errorf("sandbox/ MUST NOT import %q in its own files (forbidden entry %q; REQ-082: no auth or transport dependency)", imp, p)
		}
	}
}
