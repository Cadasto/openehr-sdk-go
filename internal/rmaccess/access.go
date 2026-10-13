package rmaccess

import (
	"maps"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// Single returns the value of the single-valued attribute attr on parent and
// whether it is present, by the readings the package documentation lists. An
// absent attribute reads as (nil, false), and so does a pair parent's class
// does not declare as a single-valued attribute, a nil parent and a typed-nil
// pointer parent. One present attribute can hold nil: an interval bound typed
// by an interface, on a closed side, is present whatever it holds.
//
// A pointer, an interface and a value held by value come back as the field
// holds them, with six exceptions, of five classes, the validation walks read
// in another shape: ITEM_SINGLE item and REFERENCE_RANGE range come back as a
// pointer into parent, ISM_TRANSITION careflow_step and transition as the
// DV_CODED_TEXT they point to, DV_PROPORTION precision as its Integer, and
// CODE_PHRASE terminology_id as the string its TERMINOLOGY_ID holds. A Hash
// attribute comes back as its Go map, or the pointer to it, whole; [Hash]
// reads its entries, with the same presence.
func Single(parent any, attr string) (value any, present bool) {
	v, present, _ := single(parent, attr)
	if !present {
		return nil, false
	}
	return v, true
}

// Multiple returns the items of the container attribute attr on parent and
// whether the list is present: it is present when it is not nil, and empty
// when it is present and holds no item, which reads as (nil, true). An
// absent list reads as (nil, false), and so does a pair parent's class does
// not declare as a container, a nil parent and a typed-nil pointer parent;
// whether a class declares a pair at all is openehr/rm/rminfo's to say.
//
// The items of a list of values are pointers into its backing array, so a
// walk can descend into each and change it in place; the items of a list of
// interfaces or pointers come back as they are.
func Multiple(parent any, attr string) (items []any, present bool) {
	l, present, _ := multiple(parent, attr)
	if !present {
		return nil, false
	}
	return l.boxed(), true
}

// Len returns the number of items of the container attribute attr on parent
// and whether the list is present, as [Multiple] reads them, without reading
// a single item: a list of a primitive type, such as DV_MULTIMEDIA data, an
// Array of Octet, is counted, not boxed one byte at a time. An absent list,
// and any pair [Multiple] reads as absent, reads as (0, false).
func Len(parent any, attr string) (n int, present bool) {
	l, present, _ := multiple(parent, attr)
	if !present {
		return 0, false
	}
	return l.length(), true
}

// Hash returns the entries of a Hash attribute in ascending key order:
// keys[i] holds values[i]. present is false for a nil map, and for a pair the
// class does not declare as a Hash.
//
// A hash is present when its map is not nil, or, held through a pointer, when
// the pointer is not nil, whatever map it points to, as [Single] reads it; it
// is empty when it is present and holds no entry, which reads as
// (nil, nil, true). A nil parent and a typed-nil pointer parent read as
// absent too.
//
// A map's values are not addressable, so a value of an RM class or a
// primitive comes back as a pointer to a copy of the entry, the form
// [Multiple] gives an item of a list of values: a walk can descend into it,
// but a change made through it does not reach the map. A value held as an
// interface or a pointer comes back as it is.
func Hash(parent any, attr string) (keys []string, values []any, present bool) {
	keys, values, present, _ = hash(parent, attr)
	if !present {
		return nil, nil, false
	}
	return keys, values, true
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

// list is a container attribute as the generated access hands it over: the
// Go slice the field holds, its length read without touching an item, and its
// items boxed only when [Multiple] asks for them.
type list interface {
	length() int
	boxed() []any
}

// boxPointers hands over a list of values, whose items are boxed, when they
// are read, as pointers into the backing array.
func boxPointers[T any](xs []T) list { return valueList[T](xs) }

// boxValues hands over a list of interfaces or pointers, whose items are
// boxed, when they are read, as they are.
func boxValues[T any](xs []T) list { return refList[T](xs) }

// valueList is a list of values.
type valueList[T any] []T

func (xs valueList[T]) length() int { return len(xs) }

// boxed boxes each item as a pointer into the backing array. It returns nil
// for an empty list.
func (xs valueList[T]) boxed() []any {
	if len(xs) == 0 {
		return nil
	}
	out := make([]any, len(xs))
	for i := range xs {
		out[i] = &xs[i]
	}
	return out
}

// refList is a list of interfaces or pointers.
type refList[T any] []T

func (xs refList[T]) length() int { return len(xs) }

// boxed boxes each item as it is. It returns nil for an empty list.
func (xs refList[T]) boxed() []any {
	if len(xs) == 0 {
		return nil
	}
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// entryPointers returns the keys of a Hash of values in ascending order and,
// for each, a pointer to a copy of its value. It returns nil slices for an
// empty map.
func entryPointers[V any](m map[string]V) ([]string, []any) {
	if len(m) == 0 {
		return nil, nil
	}
	keys := slices.Sorted(maps.Keys(m))
	values := make([]any, len(keys))
	for i, k := range keys {
		v := m[k]
		values[i] = &v
	}
	return keys, values
}

// entryValues returns the keys of a Hash of interfaces or pointers in
// ascending order and, for each, its value as it is. It returns nil slices
// for an empty map.
func entryValues[V any](m map[string]V) ([]string, []any) {
	if len(m) == 0 {
		return nil, nil
	}
	keys := slices.Sorted(maps.Keys(m))
	values := make([]any, len(keys))
	for i, k := range keys {
		values[i] = m[k]
	}
	return keys, values
}
