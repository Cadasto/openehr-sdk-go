package ehr

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// HasResource reports whether a write returned a usable resource (REQ-094).
// False for a bare-nil interface, a typed-nil REGISTERED RM pointer, and an
// interface holding one; true otherwise. The contract is scoped to the RM
// registry: [rm.IsTypedNil] is a generated closed type switch whose default
// is false, so a typed nil of a type OUTSIDE the registry reads as present.
// Every write leaf returns a registered RM type, so callers on the write
// path never reach the gap.
//
// Comparing any(v) against a boxed zero value would panic for an
// uncomparable T, so this compares against untyped nil only and defers the
// typed-nil decision to the registry.
func HasResource[T any](v T) bool {
	a := any(v)
	if a == nil {
		return false
	}
	return !rm.IsTypedNil(a)
}
