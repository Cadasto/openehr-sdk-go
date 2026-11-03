package canxml

import "github.com/cadasto/openehr-sdk-go/openehr/internal/xmlschema"

// rootElementName returns the local name of the root element of a value of the
// BMM class name: the global element its package registered for it, or its
// lower-cased name.
func rootElementName(name string) string {
	if element, ok := xmlschema.RootElement(name); ok {
		return element
	}
	return ElementName(name)
}
