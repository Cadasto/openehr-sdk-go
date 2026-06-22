package fixtures

import (
	"bytes"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// ParseOPTBytes parses OPT XML, normalising NightShift-style
// <OPERATIONAL_TEMPLATE> roots to the <template> shape expected by
// the SDK parser.
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
	if idx := strings.LastIndex(s, "</OPERATIONAL_TEMPLATE>"); idx >= 0 {
		s = s[:idx] + "</template>" + s[idx+len("</OPERATIONAL_TEMPLATE>"):]
	}
	return template.ParseOPT(bytes.NewReader([]byte(s)))
}
