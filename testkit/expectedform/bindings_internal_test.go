package expectedform

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	// The rm package registers the RM types in typereg.Default when it is
	// initialised; the reverse check below walks that registry.
	_ "github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

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
			found[site+"."+attr] = boundName.BMMName()
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
