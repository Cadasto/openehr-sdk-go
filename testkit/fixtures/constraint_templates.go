package fixtures

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ConstraintTemplateIDs returns template ids with vendored OPT + composition JSON
// used for REQ-103 primitive-constraint and REQ-102 validation cassette tests
// (ehrbase Robot Test_dv_* and clinical_content_validation).
func ConstraintTemplateIDs() ([]string, error) {
	dir := templatesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("fixtures: read %q: %w", dir, err)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".opt") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".opt")
		if !isConstraintTemplateID(id) {
			continue
		}
		if compositionJSONExcluded[id] {
			continue
		}
		jsonPath := filepath.Join(compositionsDir(), id+".json")
		if _, err := os.Stat(jsonPath); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

func isConstraintTemplateID(id string) bool {
	return id == "clinical_content_validation" || strings.HasPrefix(id, "Test_dv_")
}

// ConstraintExampleValueExcluded reports whether an OPT id is skipped in
// ExampleValue walk tests (REQ-107 gap on pattern subfields).
func ConstraintExampleValueExcluded(id string) bool {
	return constraintExampleValueExcluded[id]
}

// constraintExampleValueExcluded OPTs still validated via bundled compositions
// but skipped in ExampleValue walk tests.
var constraintExampleValueExcluded = map[string]bool{
	"Test_dv_identifier_pattern_constraint.v0": true, // pattern subfields: ExampleValue "example" vs "XYZ.*"
}
