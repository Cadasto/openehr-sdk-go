package template_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-013 (docs/specifications/module-layout.md § REQ-013): openehr/template must be
// usable without an authenticated client, so its non-test files must not import
// transport, auth or openehr/client. Test files may import anything.
func TestTemplateForbiddenImports(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		"github.com/cadasto/openehr-sdk-go/transport",
		"github.com/cadasto/openehr-sdk-go/auth",
		"github.com/cadasto/openehr-sdk-go/openehr/client",
	}
	// matched returns the forbidden prefix that imp is, or is a sub-package of.
	matched := func(imp string) (string, bool) {
		for _, p := range forbidden {
			if imp == p || strings.HasPrefix(imp, p+"/") {
				return p, true
			}
		}
		return "", false
	}

	// Can-fail control: a matcher that catches nothing, or everything, must
	// fail here rather than pass the scan below.
	if _, ok := matched("github.com/cadasto/openehr-sdk-go/transport/retry"); !ok {
		t.Fatalf("matcher misses the forbidden import %q", "github.com/cadasto/openehr-sdk-go/transport/retry")
	}
	for _, allowed := range []string{
		"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg",
		"github.com/cadasto/openehr-sdk-go/authoring",
	} {
		if p, ok := matched(allowed); ok {
			t.Fatalf("matcher flags the allowed import %q under forbidden prefix %q", allowed, p)
		}
	}

	pkg, err := build.Default.ImportDir("./", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	if len(pkg.GoFiles) == 0 {
		t.Fatal("no non-test Go files enumerated; the guard is vacuous")
	}
	for _, imp := range pkg.Imports {
		if p, ok := matched(imp); ok {
			t.Errorf("openehr/template MUST NOT import %q (REQ-013 building-block independence; matched forbidden prefix %q)", imp, p)
		}
	}
}
