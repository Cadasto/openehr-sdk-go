package xmlunmatched

import "reflect"

// The reflection in this file never chooses a Go type from the document. It
// tells a typed-nil pointer from a value, and it allocates the pointer a
// pointer to a pointer leaves nil, as encoding/xml does before it decodes, so
// that the pointer-receiver UnmarshalText method of what it points to can be
// reached and its refusal marked.

// isNilPointer reports whether v holds a nil pointer of some type: an
// interface value that is not nil itself, yet points to nothing.
func isNilPointer(v any) bool {
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

// pointee returns *v when v is a non-nil pointer to a pointer, after
// allocating *v when it is nil, as encoding/xml allocates a nil pointer it
// decodes into. ok is false, and nothing is allocated, for any other v.
func pointee(v any) (p any, ok bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil, false
	}
	e := rv.Elem()
	if e.Kind() != reflect.Pointer {
		return nil, false
	}
	if e.IsNil() {
		e.Set(reflect.New(e.Type().Elem()))
	}
	return e.Interface(), true
}
