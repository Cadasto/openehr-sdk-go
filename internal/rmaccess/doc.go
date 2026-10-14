// Package rmaccess reads the attributes of an openEHR Reference Model value
// by name, without reflection, through access the generator emits for every
// concrete RM class the type registry decodes.
//
// The API is small:
//
//	Single(parent any, attr string) (value any, present bool)
//	Multiple(parent any, attr string) (items []any, present bool)
//	Len(parent any, attr string) (n int, present bool)
//	Hash(parent any, attr string) (keys []string, values []any, present bool)
//	Covers(v any) bool
//	IsTypedNilPointer(v any) bool
//
// Each function dispatches on the Go type of parent, pointer and value form
// alike, through a type switch generated from the pinned BMMs
// (access_gen.go, written by internal/bmmgen). A reader answers every
// attribute the class declares, its own and those it inherits, by plain field
// access, so no registered class is a leaf for want of a reader, and Hash
// hands over the entries of every Hash attribute in ascending key order, so a
// walk can go below them. What the attributes are, their declared RM types,
// which are containers and which are RM-mandatory is openehr/rm/rminfo's to
// say, and so is whether a class declares an attribute at all: each function
// reads an undeclared pair as absent. This package holds only the access.
//
// # Presence
//
// Whether an attribute is present is decided from its declared type and the
// Go value alone. A pointer is present when it is not nil; an interface when
// it holds a value that is not a nil pointer; a string when it is not empty;
// a number or a boolean always; a list or a hash when it is not nil, and it
// is empty when it is present and holds no item or entry. A value held by
// value is present when it is not its type's zero value, an interface field
// holding a nil pointer counting as zero. At an attribute the RM marks
// mandatory, three types are also empty when their content is missing: a
// DV_TEXT whose value is empty, whether held directly or through an
// interface; a DV_CODED_TEXT with an empty value and an empty defining code;
// a CODE_PHRASE with an empty code_string and an empty terminology. At an
// optional attribute such a value is present, so a walk enters it and reports
// what it lacks; an object reference held by value is likewise present unless
// it is its zero value. Three readings are fixed: an interval bound is absent
// only when its side is marked unbounded and the bound is empty; EHR_STATUS
// subject is always present; ARCHETYPED archetype_id and rm_version are always
// present. The content rules and the fixed readings are in presence.go; the
// reading by Go kind and the zero tests are generated.
//
// # Dependencies
//
// The package imports the standard library and openehr/rm only. It is
// module-internal: it adds no API to the RM types.
package rmaccess
