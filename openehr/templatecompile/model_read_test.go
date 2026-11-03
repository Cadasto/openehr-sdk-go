package templatecompile_test

// model_read_test.go: the helpers the package's tests read an operational
// template into the model with, from a file or from inline XML, through the
// public reader alone.

import (
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
)

// mustReadModelFile reads the operational template at path into the model,
// stopping the test when it does not read.
func mustReadModelFile(t testing.TB, path string) *aom14.OperationalTemplate {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // read-only
	model, err := aom14.ReadOperationalTemplate(f)
	if err != nil {
		t.Fatalf("ReadOperationalTemplate(%s): %v", path, err)
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
