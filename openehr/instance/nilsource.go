package instance

import (
	mrand "math/rand/v2"
	"reflect"
)

// isNilSource reports whether src is nil, or is not nil itself but holds a
// nil pointer, func, map, slice or chan, such as a nil *rand.PCG. Such a
// source passes a plain nil check and its Uint64 typically panics, so the
// package reads it as a nil source, even where a method happens to tolerate
// a nil receiver.
//
// The reflect call is a typed-nil check, one of the classes of reflection
// the idiom rules (idiom.md § API conventions) allow in library code; the
// package cannot share transport's, since a building block never imports
// transport (REQ-013).
func isNilSource(src mrand.Source) bool {
	if src == nil {
		return true
	}
	rv := reflect.ValueOf(src)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return rv.IsNil()
	default:
		return false
	}
}
