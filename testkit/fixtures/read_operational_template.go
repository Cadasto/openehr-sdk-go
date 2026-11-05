package fixtures

import (
	"bytes"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
)

// ReadOperationalTemplate reads OPT XML into the operational template model
// with [aom14.ReadOperationalTemplate], within its size and depth limits. The
// model reads a root element of any name, so an <OPERATIONAL_TEMPLATE> root
// needs no renaming.
func ReadOperationalTemplate(b []byte) (*aom14.OperationalTemplate, error) {
	return aom14.ReadOperationalTemplate(bytes.NewReader(b))
}
