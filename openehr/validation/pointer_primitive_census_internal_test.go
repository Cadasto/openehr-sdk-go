package validation

// pointer_primitive_census_internal_test.go: REQ-102 § Primitive constraints —
// the optional primitive attributes the RM holds by pointer are the ones the
// validator reads as the value they point to. The validator keeps two
// hand-written lists of them, [rmbind.PrimitiveValue] (the value a pointer
// binds as) and [primitiveLeaf] (the pointers a walk takes for a leaf), and
// the fields of the RM classes are the truth both follow, so a census of
// those fields keeps the three from drifting apart.

import (
	"cmp"
	"reflect"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmbind"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// primitiveKind reports whether k is the kind of a primitive an RM attribute
// may hold: a boolean, a number or a string.
func primitiveKind(k reflect.Kind) bool {
	switch k {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// pointerPrimitiveFields returns the pointer-to-primitive types the fields of
// the registered RM classes have, each with the fields that have it, as
// "CLASS.Field" in class order.
func pointerPrimitiveFields(t *testing.T) map[reflect.Type][]string {
	t.Helper()
	names := typereg.Default.Names()
	if len(names) == 0 {
		t.Fatal("precondition: the type registry decodes no class")
	}
	held := map[reflect.Type][]string{}
	for _, name := range names {
		ctor, _ := typereg.Default.Lookup(name)
		class := reflect.TypeOf(ctor())
		for class.Kind() == reflect.Pointer {
			class = class.Elem()
		}
		if class.Kind() != reflect.Struct {
			continue
		}
		for _, field := range reflect.VisibleFields(class) {
			if field.Type.Kind() == reflect.Pointer && primitiveKind(field.Type.Elem().Kind()) {
				held[field.Type] = append(held[field.Type], name+"."+field.Name)
			}
		}
	}
	return held
}

// REQ-102 — the pointer types the validator reads as a primitive are exactly
// those the fields of the registered RM classes have. For each pointer type
// to a primitive, held by a field or one the validator could be taught by
// mistake, a non-nil value of it is a primitive leaf ([primitiveLeaf]) and
// binds as the value it points to ([rmbind.PrimitiveValue]) when a field
// holds it, and is neither when none does. A *string is the one that binds
// as itself: an optional String matches STRING alone.
//
// An RM that gains a pointer-held primitive, or a validator list that names a
// type no field holds, fails here by the type's name.
func TestPointerPrimitive_REQ102_ValidatorCoversTheRMFields(t *testing.T) {
	t.Parallel()
	held := pointerPrimitiveFields(t)
	// The pointer types a list could name beside the held ones: the named
	// primitives of the RM package and the Go primitives.
	candidates := []reflect.Type{
		reflect.TypeFor[*rm.Character](), reflect.TypeFor[*rm.Integer64](),
		reflect.TypeFor[*int](), reflect.TypeFor[*int64](), reflect.TypeFor[*float64](),
	}
	for typ := range held {
		if !slices.Contains(candidates, typ) {
			candidates = append(candidates, typ)
		}
	}
	slices.SortFunc(candidates, func(a, b reflect.Type) int {
		return cmp.Compare(a.String(), b.String())
	})
	leaf := walkedAttr(attributeFacts{Name: "value", RMType: "String", Typed: true}, nil)
	for _, typ := range candidates {
		fields := held[typ]
		ptr := reflect.New(typ.Elem()) // a non-nil pointer to the zero value
		in := ptr.Interface()
		if isLeaf := primitiveLeaf(in, &leaf); isLeaf != (len(fields) > 0) {
			t.Errorf("primitiveLeaf(%s) = %t, but the RM fields holding it are %v; the list and the RM must agree", typ, isLeaf, fields)
		}
		bound := rmbind.PrimitiveValue(in)
		wantBound := len(fields) > 0 && typ.Elem().Kind() != reflect.String
		if isValue := reflect.TypeOf(bound) == typ.Elem(); isValue != wantBound {
			t.Errorf("rmbind.PrimitiveValue(%s) = %T, read as its value: %t, but the RM fields holding it are %v and a %s binds as its value: %t",
				typ, bound, isValue, fields, typ, wantBound)
		}
		if !wantBound && bound != in {
			t.Errorf("rmbind.PrimitiveValue(%s) = %#v, want the pointer back", typ, bound)
		}
	}
	if len(held) == 0 {
		t.Fatal("precondition: the census finds no pointer-held primitive field in the RM classes")
	}
}
