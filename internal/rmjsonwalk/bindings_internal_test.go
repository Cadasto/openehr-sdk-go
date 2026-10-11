package rmjsonwalk

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	// The rm package registers the RM types in typereg.Default when it is
	// initialised; the reverse checks below walk that registry.
	_ "github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// registeredRMStructs returns the struct type the registry builds for every
// registered class rminfo knows, keyed by class name. A class outside rminfo
// (an AOM 1.4 class) is left out.
func registeredRMStructs(t *testing.T) map[string]reflect.Type {
	t.Helper()
	m, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	out := map[string]reflect.Type{}
	for _, name := range typereg.Default.Names() {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok || !m.IsClass(name) {
			continue
		}
		st := reflect.TypeOf(ctor())
		for st.Kind() == reflect.Pointer {
			st = st.Elem()
		}
		if st.Kind() == reflect.Struct {
			out[name] = st
		}
	}
	if len(out) < 100 {
		t.Fatalf("found %d registered RM struct types; want the full RM inventory, or the checks below assert nothing", len(out))
	}
	return out
}

// jsonField returns the field of st, promoted ones included, whose JSON name
// is name.
func jsonField(st reflect.Type, name string) (reflect.StructField, bool) {
	for _, f := range reflect.VisibleFields(st) {
		if tag, _, _ := strings.Cut(f.Tag.Get("json"), ","); tag == name && f.IsExported() {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

// TestBoundBindingsMatchTheRM checks boundBindings against the RM it is
// written from, in both directions, so the table cannot drift when the RM
// changes. Each entry must name a DV_INTERVAL slot declared on the keyed class
// and bind a concrete class. And every DV_INTERVAL field the generated rm
// types hold with a concrete bound type must be an entry binding that type:
// those are the slots the codec decodes an elided bound "_type" in.
func TestBoundBindingsMatchTheRM(t *testing.T) {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default does not implement rminfo.Hierarchy")
	}
	for key, bound := range boundBindings {
		class, attr, ok := strings.Cut(key, ".")
		if !ok {
			t.Errorf("key %q is not CLASS.attribute", key)
			continue
		}
		if typ, ok := rminfo.Default.AttributeRMType(class, attr); !ok || typ != "DV_INTERVAL" {
			t.Errorf("%s: AttributeRMType = %q, %v, want DV_INTERVAL, true", key, typ, ok)
		}
		if site, ok := h.DeclaredOn(class, attr); !ok || site != class {
			t.Errorf("%s: DeclaredOn = %q, %v, want %s, true", key, site, ok, class)
		}
		if abstract, known := h.IsAbstract(bound); !known || abstract {
			t.Errorf("%s binds %s: IsAbstract = %v, known %v, want a concrete RM class", key, bound, abstract, known)
		}
	}

	found := map[string]string{}
	for _, name := range typereg.Default.Names() {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			continue
		}
		st := reflect.TypeOf(ctor())
		for st.Kind() == reflect.Pointer {
			st = st.Elem()
		}
		if st.Kind() != reflect.Struct {
			continue
		}
		for f := range st.Fields() {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() != reflect.Struct || !strings.HasPrefix(ft.Name(), "DVInterval[") {
				continue
			}
			lower, ok := ft.FieldByName("Lower")
			if !ok || lower.Type.Kind() == reflect.Interface {
				continue
			}
			attr, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			site, ok := h.DeclaredOn(name, attr)
			if !ok {
				t.Errorf("%s.%s: the rm field has no rminfo attribute", name, attr)
				continue
			}
			boundName, ok := reflect.TypeAssert[interface{ BMMName() string }](reflect.New(lower.Type))
			if !ok {
				t.Errorf("%s.%s: bound type %s does not name its BMM class", name, attr, lower.Type)
				continue
			}
			key, bound := site+"."+attr, boundName.BMMName()
			if prev, seen := found[key]; seen && prev != bound {
				t.Errorf("%s: the rm types bind it to both %s and %s", key, prev, bound)
			}
			found[key] = bound
		}
	}
	if len(found) == 0 {
		t.Fatal("no DV_INTERVAL field with a concrete bound found in the rm types; the reverse check asserts nothing")
	}
	for _, key := range slices.Sorted(maps.Keys(found)) {
		if got, want := boundBindings[key], found[key]; got != want {
			t.Errorf("boundBindings[%q] = %q, want %q (the rm type holds the bound by that concrete type)", key, got, want)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(boundBindings)) {
		if _, ok := found[key]; !ok {
			t.Errorf("boundBindings[%q] names no rm field with a concrete bound type", key)
		}
	}
}

// TestPayloadsMatchTheRM checks payloads against rminfo and the registered rm
// types, in both directions (REQ-170: a value of a class outside the RM is not
// checked). For every registered class, every attribute rminfo types as a type
// parameter is a payload exactly when the registered Go type holds it as Go's
// any (or a pointer to one), which encoding/json/v2 fills with plain JSON
// values. An attribute held
// as an RM interface (DV_INTERVAL's bounds, EVENT's data) is decoded by its
// "_type" and is no payload.
func TestPayloadsMatchTheRM(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not implement rminfo.AttributeLister")
	}
	anyType := reflect.TypeFor[any]()
	found := map[string]bool{}
	parameters := 0
	for name, st := range registeredRMStructs(t) {
		for _, attr := range lister.AttributeNames(name) {
			typ, _ := rminfo.Default.AttributeRMType(name, attr)
			if m.kindOf(typ) != KindParameter {
				continue
			}
			parameters++
			f, ok := jsonField(st, attr)
			if !ok {
				t.Errorf("%s.%s: the registered type %s has no field for the type-parameter attribute", name, attr, st)
				continue
			}
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft == anyType {
				found[name+"."+attr] = true
			}
		}
	}
	if parameters == 0 {
		t.Fatal("no type-parameter attribute found on a registered rm type; the check asserts nothing")
	}
	for _, key := range slices.Sorted(maps.Keys(found)) {
		if !payloads[key] {
			t.Errorf("%s: the registered rm type holds it as any, want payloads to list it", key)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(payloads)) {
		if !found[key] {
			t.Errorf("payloads lists %s, which no registered rm type holds as any", key)
		}
	}
}
