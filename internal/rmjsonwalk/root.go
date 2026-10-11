package rmjsonwalk

import (
	"reflect"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// rmPackage is the import path of the package that declares the RM's Go
// types. An interface declared there is a polymorphic RM slot.
const rmPackage = "github.com/cadasto/openehr-sdk-go/openehr/rm"

// bmmNamer is a Go type that names the BMM class it implements.
type bmmNamer interface{ BMMName() string }

// rootOf is what [Model.RootOf] found for one target type.
type rootOf struct {
	root Root
	ok   bool
}

// RootOf reads the RM shape a decode target gives a document's root. target
// is what the decode writes into: a pointer to an RM type, such as a
// *rm.Composition or a *rm.DVInterval[rm.DVQuantity], to an RM interface such
// as rm.DataValue, or to slices, arrays and maps of these, at any depth.
//
// The second result is false when the target holds no RM value: under its
// pointers, slices, arrays and maps it is neither a Go type that names a
// class of the model nor an interface the RM package declares. Such a target,
// an any, a map[string]any, an AOM 1.4 type or a caller's own struct, decodes
// as no RM type, so a walk has nothing to check in it.
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

// declaredOf returns the RM type a Go type is declared as: the class it names,
// or for an RM interface the class its elided "_type" falls back to (see
// interfaceClass). The second result is false when t is no RM type.
func (m *Model) declaredOf(t reflect.Type) (string, bool) {
	if t.Kind() == reflect.Interface {
		if t.PkgPath() != rmPackage {
			return "", false
		}
		return m.interfaceClass(t), true
	}
	n, ok := reflect.TypeAssert[bmmNamer](reflect.New(t))
	if !ok {
		return "", false
	}
	name := n.BMMName()
	return name, m.IsClass(name)
}

// interfaceClass returns the class an RM interface t falls back to when a
// value elides its "_type": the one registered RM class every registered class
// implementing t conforms to, such as DV_TEXT for rm.DVTextLike, which DV_TEXT
// and DV_CODED_TEXT implement. It returns the empty string when there is none,
// as for rm.DataValue, whose classes share no concrete ancestor; a value there
// must carry its own "_type".
func (m *Model) interfaceClass(t reflect.Type) string {
	var classes []string
	for _, name := range typereg.Default.Names() {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok || !m.IsClass(name) {
			continue
		}
		if reflect.TypeOf(ctor()).Implements(t) {
			classes = append(classes, name)
		}
	}
	for _, c := range classes {
		all := true
		for _, other := range classes {
			if conforms, _ := m.hier.ConformsTo(other, c); !conforms {
				all = false
				break
			}
		}
		if all {
			return c
		}
	}
	return ""
}

// boundOf returns the class Go type t binds the type parameter of class to:
// the declared type of the Go field behind the class's first type-parameter
// attribute, as for the Lower field of an rm.DVInterval[rm.DVQuantity]. It
// returns the empty string when class has no such attribute, or the field's
// type names no class of its own (Go's any, or an RM interface with no
// fallback class).
func (m *Model) boundOf(t reflect.Type, class string) string {
	lister, ok := m.lookup.(rminfo.AttributeLister)
	if !ok || t.Kind() != reflect.Struct {
		return ""
	}
	for _, attr := range lister.AttributeNames(class) {
		typ, _ := m.lookup.AttributeRMType(class, attr)
		if m.kindOf(typ) != KindParameter {
			continue
		}
		for _, f := range reflect.VisibleFields(t) {
			if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name != attr || !f.IsExported() {
				continue
			}
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			bound, _ := m.declaredOf(ft)
			return bound
		}
		return ""
	}
	return ""
}
