package simplified_test

import (
	"os"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// readOPTFile reads the operational template file at path into the model.
func readOPTFile(path string) (*aom14.OperationalTemplate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return fixtures.ReadOperationalTemplate(raw)
}
