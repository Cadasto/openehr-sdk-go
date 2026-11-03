package templatecompile

// model_internal_test.go: the helpers the package's internal tests compile
// an operational template with, read into the model from a file or from
// inline XML.

import (
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// compileModelFile reads the operational template at path into the model and
// compiles it, stopping the test when either fails.
func compileModelFile(t testing.TB, path string) *Compiled {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	model, err := fixtures.ReadOperationalTemplate(b)
	if err != nil {
		t.Fatalf("ReadOperationalTemplate(%s): %v", path, err)
	}
	c, err := Compile(model)
	if err != nil {
		t.Fatalf("Compile(%s): %v", path, err)
	}
	return c
}

// compileModelXML reads the operational template xml into the model and
// compiles it, stopping the test when either fails.
func compileModelXML(t testing.TB, xml string) *Compiled {
	t.Helper()
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	c, err := Compile(model)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}
