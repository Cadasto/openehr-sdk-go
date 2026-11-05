package transport

import "reflect"

// isTypedNil reports whether v is not nil itself but holds a nil pointer,
// func, map, slice or chan, such as a nil *smart.Source stored in an
// auth.TokenSource. Such a value passes a plain nil check, and its methods
// panic or block when called. A plain nil reports false: callers test that
// with == nil, as it keeps its own meaning (an option left unset).
//
// The reflect call is a typed-nil check, one of the classes of reflection
// the idiom rules (idiom.md) allow in library code.
func isTypedNil(v any) bool {
	if v == nil {
		return false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return rv.IsNil()
	default:
		return false
	}
}
