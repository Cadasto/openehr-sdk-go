package rmaccess

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// Single returns the value of the single-valued attribute attr on parent and
// whether it is present, by the readings the package documentation lists. An
// absent attribute reads as (nil, false), and so does a pair parent's class
// does not declare as a single-valued attribute, a nil parent and a typed-nil
// pointer parent. One present attribute can hold nil: an interval bound typed
// by an interface, on a closed side, is present whatever it holds.
//
// A pointer, an interface and a value held by value come back as the field
// holds them, with five exceptions the validation walks read in another
// shape: ITEM_SINGLE item and REFERENCE_RANGE range come back as a pointer
// into parent, ISM_TRANSITION careflow_step and transition as the
// DV_CODED_TEXT they point to, DV_PROPORTION precision as its Integer, and
// CODE_PHRASE terminology_id as the string its TERMINOLOGY_ID holds.
func Single(parent any, attr string) (value any, present bool) {
	v, present, _ := single(parent, attr)
	if !present {
		return nil, false
	}
	return v, true
}

// Multiple returns the items of the container attribute attr on parent, and
// ok true whenever parent's class declares attr as a container, whether the
// list is nil, empty or holds items. A caller that needs "empty" reads
// len(items) == 0. A pair the class does not declare as a container, a nil
// parent and a typed-nil pointer parent read as (nil, false).
//
// The items of a list of values are pointers into its backing array, so a
// walk can descend into each and change it in place; the items of a list of
// interfaces or pointers come back as they are.
func Multiple(parent any, attr string) (items []any, ok bool) {
	items, _, ok = multiple(parent, attr)
	return items, ok
}

// Covers reports whether v's Go type is one the access reads: every concrete
// class the type registry decodes, in pointer or value form, and every
// instantiation of a generic class the reverse registry recognises. A
// typed-nil pointer to such a class is covered; nil is not.
func Covers(v any) bool {
	return covers(v)
}

// IsTypedNilPointer reports whether v is an interface value carrying a
// typed-nil pointer to a registered RM class. A nil interface and a value
// held by value report false. It delegates to [rm.IsTypedNil].
func IsTypedNilPointer(v any) bool {
	return rm.IsTypedNil(v)
}

// boxPointers boxes each element of a list of values as a pointer into the
// backing array. It returns nil for an empty list.
func boxPointers[T any](xs []T) []any {
	if len(xs) == 0 {
		return nil
	}
	out := make([]any, len(xs))
	for i := range xs {
		out[i] = &xs[i]
	}
	return out
}

// boxValues boxes each element of a list of interfaces or pointers as it is.
// It returns nil for an empty list.
func boxValues[T any](xs []T) []any {
	if len(xs) == 0 {
		return nil
	}
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}
