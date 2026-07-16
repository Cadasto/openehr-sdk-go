package simplified_test

// REQ-013 — building-block independence: the simplified-format codecs must
// not pull in the transport, auth, client, or cadasto layers.
import (
	"go/build"
	"strings"
	"testing"
)

func TestBuildingBlockIndependence(t *testing.T) {
	pkg, err := build.Import("github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified", "", 0)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	// Note: the module path is github.com/cadasto/openehr-sdk-go, so the
	// cadasto/ extras cut line is the "/openehr-sdk-go/cadasto/" subtree —
	// not a bare "/cadasto" (that would match the module owner).
	forbidden := []string{"/transport", "/auth", "/openehr/client", "/openehr-sdk-go/cadasto/"}
	for _, imp := range pkg.Imports {
		for _, f := range forbidden {
			if strings.Contains(imp, f) {
				t.Errorf("forbidden import %q (matches %q)", imp, f)
			}
		}
	}
}
