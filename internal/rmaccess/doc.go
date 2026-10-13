// Package rmaccess reads the attributes of an openEHR Reference Model value
// by name, without reflection, through access the generator emits for every
// concrete RM class the type registry decodes.
//
// The API is small:
//
//	Single(parent any, attr string) (value any, present bool)
//	Multiple(parent any, attr string) (items []any, ok bool)
//	Covers(v any) bool
//	IsTypedNilPointer(v any) bool
//
// Each function dispatches on the Go type of parent, pointer and value form
// alike, through a type switch generated from the pinned BMMs
// (access_gen.go, written by internal/bmmgen). A reader answers every
// attribute the class declares, its own and those it inherits, by plain field
// access, so no registered class is a leaf for want of a reader. What the
// attributes are, their declared RM types, which are containers and which
// are RM-mandatory is openehr/rm/rminfo's to say; this package holds only the
// access.
//
// # Presence
//
// Whether an attribute is present is decided from its declared type and the
// Go value alone. A pointer is present when it is not nil; an interface when
// it holds a value that is not a nil pointer; a string when it is not empty;
// a number or a boolean always; a list when it is not nil. A value held by
// value is present when it is not empty: a DV_TEXT, held directly or through
// an interface, when its value is not empty; a DV_CODED_TEXT when its value
// or its defining code is not empty; a CODE_PHRASE when its code_string or
// its terminology is not empty; an OBJECT_REF or a PARTY_REF when its id,
// namespace and type are all there; any other class when it is not its zero
// value. Three readings are fixed: an interval bound is absent only when its
// side is marked unbounded and the bound is empty; EHR_STATUS subject is
// always present; ARCHETYPED archetype_id and rm_version are always present.
// The content rules and the fixed readings are in presence.go; the reading by
// Go kind and the zero tests are generated.
//
// # Dependencies
//
// The package imports the standard library and openehr/rm only. It is
// module-internal: it adds no API to the RM types.
package rmaccess
