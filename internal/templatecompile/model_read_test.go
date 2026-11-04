package templatecompile_test

// model_read_test.go: the helpers the package's tests read an operational
// template into the model with, from a file or from inline XML.

import (
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// readModelFile reads the operational template at path into the model.
func readModelFile(path string) (*aom14.OperationalTemplate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return fixtures.ReadOperationalTemplate(b)
}

// mustReadModelFile reads the operational template at path into the model,
// stopping the test when it does not read.
func mustReadModelFile(t testing.TB, path string) *aom14.OperationalTemplate {
	t.Helper()
	model, err := readModelFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return model
}

// mustReadModel reads the operational template xml into the model, stopping
// the test when it does not read.
func mustReadModel(t testing.TB, xml string) *aom14.OperationalTemplate {
	t.Helper()
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	return model
}

// mustCompileXML reads the operational template xml into the model and
// compiles it, stopping the test when either fails.
func mustCompileXML(t testing.TB, xml string, opts ...templatecompile.Options) *templatecompile.Compiled {
	t.Helper()
	c, err := templatecompile.Compile(mustReadModel(t, xml), opts...)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}
