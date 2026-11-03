package fixtures

import (
	"bytes"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// ReadOperationalTemplate reads OPT XML into the operational template model
// with [aom14.ReadOperationalTemplate], within its size and depth limits. The
// model reads a root element of any name, so an <OPERATIONAL_TEMPLATE> root
// needs no renaming.
func ReadOperationalTemplate(b []byte) (*aom14.OperationalTemplate, error) {
	return aom14.ReadOperationalTemplate(bytes.NewReader(b))
}

// ParseOPTBytes parses OPT XML, normalising NightShift-style
// <OPERATIONAL_TEMPLATE> roots to the <template> shape expected by
// the SDK parser.
//
// Deprecated: Use [ReadOperationalTemplate], which reads the operational
// template model the compile step takes.
func ParseOPTBytes(b []byte) (*template.OperationalTemplate, error) {
	parsed, err := template.ParseOPT(bytes.NewReader(b))
	if err == nil {
		return parsed, nil
	}
	if !bytes.Contains(b, []byte("<OPERATIONAL_TEMPLATE")) {
		return nil, err
	}
	s := string(b)
	s = strings.Replace(s, "<OPERATIONAL_TEMPLATE", "<template", 1)
	if before, after, found := strings.CutLast(s, "</OPERATIONAL_TEMPLATE>"); found {
		s = before + "</template>" + after
	}
	return template.ParseOPT(bytes.NewReader([]byte(s)))
}
