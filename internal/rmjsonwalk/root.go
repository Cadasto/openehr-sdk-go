package rmjsonwalk

import (
	"reflect"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk/polyreg"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// bmmNamer is a Go type that names the BMM class it implements.
type bmmNamer interface{ BMMName() string }

// rootOf is what [Model.RootOf] found for one target type.
type rootOf struct {
	root Root
	ok   bool
}

// RootOf reads the RM shape a decode target gives a document's root. target
// is what the decode writes into: a pointer to an RM class, such as a
// *rm.Composition or a *rm.DVInterval[rm.DVQuantity], to a polymorphic RM
// interface such as rm.DataValue, or to slices, arrays and maps of these, at
// any depth. A generic class takes the class its type argument names as the
// binding of its type parameter.
//
// The second result is false when the target's own shape holds no RM value:
// under its pointers, slices, arrays and maps it is neither a Go type that
// names a class of the model nor a polymorphic RM interface. Such a target,
// an any, a map[string]any, an AOM 1.4 type or a type of the caller's own,
// even one with RM fields, gives a walk no RM value to start from.
//
// The Go type tells the declared type a "_type" can override, and nothing
// else: RootOf reads no JSON. Results are kept per target type.
func (m *Model) RootOf(target any) (Root, bool) {
	t := reflect.TypeOf(target)
	if t == nil {
		return Root{}, false
	}
	if r, ok := m.roots.Load(t); ok {
		found := r.(rootOf)
		return found.root, found.ok
	}
	root, ok := m.readRoot(t)
	m.roots.Store(t, rootOf{root: root, ok: ok})
	return root, ok
}

// readRoot reads the Root of target type t: the containers around its RM
// values, then the values' declared type and binding.
func (m *Model) readRoot(t reflect.Type) (Root, bool) {
	var nest strings.Builder
	for {
		switch t.Kind() {
		case reflect.Pointer:
			t = t.Elem()
			continue
		case reflect.Slice, reflect.Array:
			nest.WriteByte('[')
			t = t.Elem()
			continue
		case reflect.Map:
			nest.WriteByte('{')
			t = t.Elem()
			continue
		}
		break
	}
	declared, ok := m.declaredOf(t)
	if !ok {
		return Root{}, false
	}
	return Root{Nest: nest.String(), Type: declared, Bound: m.boundOf(t, declared)}, true
}

// declaredOf returns the RM class a Go type is declared as, under any
// pointers: the class it names, or for a polymorphic RM interface the class
// the interface stands for, such as DATA_VALUE for rm.DataValue or DV_TEXT for
// rm.DVTextLike. The second result is false when t is neither.
//
// Both answers come from a new value of t: BMMName on the class's Go type, and
// for a pointer to an interface the table openehr/rm registers in polyreg.
func (m *Model) declaredOf(t reflect.Type) (string, bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	p := reflect.New(t).Interface()
	name := ""
	if class, ok := polyreg.Class(p); ok {
		name = class
	} else if n, ok := p.(bmmNamer); ok {
		name = n.BMMName()
	}
	if !m.IsClass(name) {
		return "", false
	}
	return name, true
}

// boundOf returns the class a generic RM type t binds the type parameter of
// class to: the class its type argument is declared as, such as DV_QUANTITY
// for an rm.DVInterval[rm.DVQuantity] or DATA_VALUE for an
// rm.OriginalVersion[rm.DataValue]. The type argument is read by field
// mapping: for each attribute rminfo declares with the type parameter, the Go
// field whose JSON tag is the attribute's name, its type peeled of pointers.
//
// It returns the empty string for a class with no such attribute, for a type
// argument that is no RM type, such as Go's any, which leaves a generic
// payload as plain JSON, and when the fields disagree: two attributes of one
// parameter held as different classes, or attributes of two parameters. With
// no binding, the walk reads such values by their own "_type", or not at all.
func (m *Model) boundOf(t reflect.Type, class string) string {
	lister, ok := m.lookup.(rminfo.AttributeLister)
	if !ok || t.Kind() != reflect.Struct {
		return ""
	}
	bound, param := "", ""
	for _, attr := range lister.AttributeNames(class) {
		typ, _ := m.lookup.AttributeRMType(class, attr)
		if m.kindOf(typ) != KindParameter {
			continue
		}
		for _, f := range reflect.VisibleFields(t) {
			if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name != attr || !f.IsExported() {
				continue
			}
			b, _ := m.declaredOf(f.Type)
			if param != "" && (typ != param || b != bound) {
				return ""
			}
			bound, param = b, typ
			break
		}
	}
	return bound
}
