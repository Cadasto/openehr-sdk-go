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
// pointer parent. Two present attributes can hold nil: an interval bound
// typed by an interface, on a closed side, is present whatever it holds, and
// ORIGINAL_VERSION data, whose pointer is present when it is not nil, comes
// back as nil when that pointer holds a nil interface.
//
// A pointer, an interface and a primitive come back as the field holds them.
// A class held by value comes back as a pointer into parent, so reading it
// copies nothing and a write through it reaches parent; parent held by value
// is itself a copy, so such a write reaches only that copy. Two attributes
// of two classes come back in another shape: DV_PROPORTION precision as its
// Integer, and ORIGINAL_VERSION data as the value its pointer holds, so a
// walk can enter the payload of a version built in Go. A Hash attribute
// comes back as its Go map, or the pointer to it, whole; [Hash] reads its
// entries, with the same presence.
func Single(parent any, attr string) (value any, present bool) {
	v, present, _ := single(parent, attr, false)
	if !present {
		return nil, false
	}
	return v, true
}

// Peek reads the single-valued attribute attr on parent as [Single] does,
// except that a primitive leaf is not read: an attribute whose field holds a
// string, a boolean or a number, as a predeclared Go type or as rm.Real,
// rm.Integer, rm.Integer64 or rm.Character, comes back as (nil, present,
// true), its presence the one Single reads. Every other attribute comes back
// as Single gives it, with leaf false. A walk that only counts a primitive
// leaf reads it through Peek, so the value is not boxed.
func Peek(parent any, attr string) (value any, present, leaf bool) {
	v, present, _ := single(parent, attr, true)
	if _, leaf = v.(leafMarker); leaf {
		return nil, present, true
	}
	if !present {
		return nil, false, false
	}
	return v, true, false
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
	items, _, present, _ = multiple(parent, attr, true)
	if !present {
		return nil, false
	}
	return items, true
}

// Len returns the number of items of the container attribute attr on parent
// and whether the list is present, as [Multiple] reads them, without reading
// a single item: a list of a primitive type, such as DV_MULTIMEDIA data, an
// Array of Octet, is counted, not boxed one byte at a time. An absent list,
// and any pair [Multiple] reads as absent, reads as (0, false).
func Len(parent any, attr string) (n int, present bool) {
	_, n, present, _ = multiple(parent, attr, false)
	if !present {
		return 0, false
	}
	return n, true
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

// leafMarker is what a single reader called to peek hands back in place of a
// primitive leaf's value ([Peek]). It has no size, so boxing it allocates
// nothing.
type leafMarker struct{}

// leafValue hands back v, the value of a primitive leaf, or, when peek is
// set, leafMarker in its place, so the generated readers box v only when it
// is read.
func leafValue[T any](peek bool, v T) any {
	if peek {
		return leafMarker{}
	}
	return v
}

// boxPointers boxes, when box is set, each item of a list of values as a
// pointer into the backing array. It returns nil when box is not set and for
// an empty list.
func boxPointers[T any](box bool, xs []T) []any {
	if !box || len(xs) == 0 {
		return nil
	}
	out := make([]any, len(xs))
	for i := range xs {
		out[i] = &xs[i]
	}
	return out
}

// boxValues boxes, when box is set, each item of a list of interfaces or
// pointers as it is. It returns nil when box is not set and for an empty
// list.
func boxValues[T any](box bool, xs []T) []any {
	if !box || len(xs) == 0 {
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
