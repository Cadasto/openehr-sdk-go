package canxml

import (
	"sync"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// schemaNames holds the canonical-XML names the classes of the generated
// packages take beyond their own BMM names: the type aliases the decoder reads
// and the root elements the encoder writes. The generated packages fill it from
// their init functions, through [RegisterTypeAlias] and [RegisterRootElement].
var schemaNames = struct {
	mu sync.RWMutex
	// aliases maps an alias to the type name it reads as.
	aliases map[string]string
	// roots maps a BMM class name to the local name of its root element.
	roots map[string]string
}{aliases: map[string]string{}, roots: map[string]string{}}

// RegisterTypeAlias makes the decoder read alias, written as the `xsi:type` of
// an element, as the class registered in [typereg.Default] under name, such as
// `T_ARCHETYPE_ROOT`, which some tools write for the AOM 1.4
// `C_ARCHETYPE_ROOT`. The value decoded is a value of that class, so the
// encoder writes the class's own name. The alias is not registered in
// [typereg.Default], which lists each class once, under its own name, and a
// name registered there is read as itself even when it is an alias too.
//
// The generated packages call it from their init functions; other code has no
// need to. It panics when alias or name is empty and when alias is registered
// twice, as [typereg.Registry.Register] does on a duplicate: either is a
// programmer error.
func RegisterTypeAlias(alias, name string) {
	if alias == "" || name == "" {
		panic("canxml.RegisterTypeAlias: an empty alias or type name")
	}
	schemaNames.mu.Lock()
	defer schemaNames.mu.Unlock()
	if _, dup := schemaNames.aliases[alias]; dup {
		panic("canxml.RegisterTypeAlias: duplicate registration for " + alias)
	}
	schemaNames.aliases[alias] = name
}

// RegisterRootElement names the root element [Marshal] and [MarshalIndent]
// write for a value of the BMM class name: element, the global element the
// class's ITS-XML schema declares for its type, such as `template` for the AOM
// 1.4 `OPERATIONAL_TEMPLATE`. A class with no registered root element is
// written under its lower-cased name ([ElementName]). The decoder does not
// match the root element by name, so it reads either.
//
// The generated packages call it from their init functions; other code has no
// need to. It panics when name or element is empty and when name is registered
// twice.
func RegisterRootElement(name, element string) {
	if name == "" || element == "" {
		panic("canxml.RegisterRootElement: an empty class name or element name")
	}
	schemaNames.mu.Lock()
	defer schemaNames.mu.Unlock()
	if _, dup := schemaNames.roots[name]; dup {
		panic("canxml.RegisterRootElement: duplicate registration for " + name)
	}
	schemaNames.roots[name] = element
}

// lookupType returns the constructor of the class an `xsi:type` of typeName
// reads as: the class registered in [typereg.Default] under typeName, or else
// the one registered under the name typeName is an alias of.
func lookupType(typeName string) (func() any, bool) {
	if ctor, ok := typereg.Default.Lookup(typeName); ok {
		return ctor, true
	}
	schemaNames.mu.RLock()
	name, ok := schemaNames.aliases[typeName]
	schemaNames.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return typereg.Default.Lookup(name)
}

// rootElementName returns the local name of the root element of a value of the
// BMM class name: the one registered for it, or its lower-cased name.
func rootElementName(name string) string {
	schemaNames.mu.RLock()
	element, ok := schemaNames.roots[name]
	schemaNames.mu.RUnlock()
	if ok {
		return element
	}
	return ElementName(name)
}
