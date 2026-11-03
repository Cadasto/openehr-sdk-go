// Package xmlschema holds what the openEHR ITS-XML schemas say about the
// canonical XML of the generated classes beyond their BMM names and shapes,
// for the generated packages and openehr/serialize/canxml, which import it:
// the type aliases the decoder reads, the root elements the encoder writes,
// and the children the encoder leaves out where a schema makes them optional.
// It is internal so that this wiring is not part of the SDK's public API.
package xmlschema

import "sync"

// names holds the canonical-XML names the classes of the generated packages
// take beyond their own BMM names: the type aliases the decoder reads and the
// root elements the encoder writes. The generated packages fill it from their
// init functions, through [RegisterTypeAlias] and [RegisterRootElement].
var names = struct {
	mu sync.RWMutex
	// aliases maps an alias to the type name it reads as.
	aliases map[string]string
	// roots maps a BMM class name to the local name of its root element.
	roots map[string]string
}{aliases: map[string]string{}, roots: map[string]string{}}

// RegisterTypeAlias makes the canonical-XML decoder read alias, written as the
// `xsi:type` of an element, as the class the type registry holds under name,
// such as `T_ARCHETYPE_ROOT`, which some tools write for the AOM 1.4
// `C_ARCHETYPE_ROOT`. The value decoded is a value of that class, so the
// encoder writes the class's own name. The alias is not registered in the type
// registry, which lists each class once, under its own name, and a name
// registered there is read as itself even when it is an alias too.
//
// The generated packages call it from their init functions. It panics when
// alias or name is empty and when alias is registered twice, as the type
// registry does on a duplicate: either is a programming error.
func RegisterTypeAlias(alias, name string) {
	if alias == "" || name == "" {
		panic("xmlschema.RegisterTypeAlias: an empty alias or type name")
	}
	names.mu.Lock()
	defer names.mu.Unlock()
	if _, dup := names.aliases[alias]; dup {
		panic("xmlschema.RegisterTypeAlias: duplicate registration for " + alias)
	}
	names.aliases[alias] = name
}

// RegisterRootElement names the root element the canonical-XML encoder writes
// for a value of the BMM class name: element, the global element the class's
// ITS-XML schema declares for its type, such as `template` for the AOM 1.4
// `OPERATIONAL_TEMPLATE`. A class with no registered root element is written
// under its lower-cased name. The decoder does not match the root element by
// name, so it reads either.
//
// The generated packages call it from their init functions. It panics when
// name or element is empty and when name is registered twice.
func RegisterRootElement(name, element string) {
	if name == "" || element == "" {
		panic("xmlschema.RegisterRootElement: an empty class name or element name")
	}
	names.mu.Lock()
	defer names.mu.Unlock()
	if _, dup := names.roots[name]; dup {
		panic("xmlschema.RegisterRootElement: duplicate registration for " + name)
	}
	names.roots[name] = element
}

// AliasOf returns the type name a registered alias reads as, and false when
// alias is registered as none.
func AliasOf(alias string) (string, bool) {
	names.mu.RLock()
	defer names.mu.RUnlock()
	name, ok := names.aliases[alias]
	return name, ok
}

// RootElement returns the local name of the root element registered for a
// value of the BMM class name, and false when none is registered.
func RootElement(name string) (string, bool) {
	names.mu.RLock()
	defer names.mu.RUnlock()
	element, ok := names.roots[name]
	return element, ok
}
