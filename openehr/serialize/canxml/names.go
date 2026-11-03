package canxml

import (
	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlschema"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// lookupType returns the constructor of the class an `xsi:type` of typeName
// reads as: the class registered in [typereg.Default] under typeName, or else
// the one registered under the name typeName is a type alias of, which a
// generated package registers for the names its schemas use beside the
// class's own.
func lookupType(typeName string) (func() any, bool) {
	if ctor, ok := typereg.Default.Lookup(typeName); ok {
		return ctor, true
	}
	name, ok := xmlschema.AliasOf(typeName)
	if !ok {
		return nil, false
	}
	return typereg.Default.Lookup(name)
}

// rootElementName returns the local name of the root element of a value of the
// BMM class name: the global element its package registered for it, or its
// lower-cased name.
func rootElementName(name string) string {
	if element, ok := xmlschema.RootElement(name); ok {
		return element
	}
	return ElementName(name)
}
