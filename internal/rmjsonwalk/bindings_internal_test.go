package rmjsonwalk

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	// The rm and aom14 packages register their types in typereg.Default
	// when they are initialised; the reverse checks below walk that
	// registry.
	_ "github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
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

// TestHashValuesMatchTheRM checks hashValues against rminfo and the Go map
// fields of the registered types, in both directions (REQ-170: every object
// that decodes as an RM class is checked). Each entry must name a Hash
// attribute declared on the keyed class and give a concrete RM class. And
// every registered library type, RM or AOM 1.4, that holds a Hash attribute
// as a Go map whose values are a class of the model must be an entry giving
// that class: the decoder reads each of those values as that class. The AOM
// 1.4 ARCHETYPE is the only holder of AUTHORED_RESOURCE.translations, which no
// RM class inherits.
func TestHashValuesMatchTheRM(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	h := m.hier
	for key, class := range hashValues {
		owner, attr, ok := strings.Cut(key, ".")
		if !ok {
			t.Errorf("key %q is not CLASS.attribute", key)
			continue
		}
		if typ, ok := rminfo.Default.AttributeRMType(owner, attr); !ok || typ != "Hash" {
			t.Errorf("%s: AttributeRMType = %q, %v, want Hash, true", key, typ, ok)
		}
		if site, ok := h.DeclaredOn(owner, attr); !ok || site != owner {
			t.Errorf("%s: DeclaredOn = %q, %v, want %s, true", key, site, ok, owner)
		}
		if abstract, known := h.IsAbstract(class); !known || abstract {
			t.Errorf("%s gives %s: IsAbstract = %v, known %v, want a concrete RM class", key, class, abstract, known)
		}
	}

	// declaringSites returns every class.attr where the model declares attr
	// as a Hash, for a holder rminfo does not know (an AOM 1.4 class).
	declaringSites := func(attr string) []string {
		var sites []string
		for _, c := range rminfo.Default.KnownRMTypes() {
			typ, _ := rminfo.Default.AttributeRMType(c, attr)
			if site, _ := h.DeclaredOn(c, attr); typ == "Hash" && site == c {
				sites = append(sites, c+"."+attr)
			}
		}
		return sites
	}
	found := map[string]string{}
	holders := 0
	for _, name := range typereg.Default.Names() {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			continue
		}
		st := reflect.TypeOf(ctor())
		for st.Kind() == reflect.Pointer {
			st = st.Elem()
		}
		if st.Kind() != reflect.Struct || !strings.HasPrefix(st.PkgPath(), "github.com/cadasto/openehr-sdk-go/openehr/") {
			continue
		}
		for _, f := range reflect.VisibleFields(st) {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if !f.IsExported() || ft.Kind() != reflect.Map || ft.Key().Kind() != reflect.String {
				continue
			}
			vt := ft.Elem()
			for vt.Kind() == reflect.Pointer {
				vt = vt.Elem()
			}
			value, ok := reflect.TypeAssert[bmmNamer](reflect.New(vt))
			if !ok || !m.IsClass(value.BMMName()) {
				continue // a map of strings or of plain JSON
			}
			attr, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			var sites []string
			if site, ok := h.DeclaredOn(name, attr); ok {
				sites = []string{site + "." + attr}
			} else {
				sites = declaringSites(attr)
			}
			if len(sites) != 1 {
				t.Errorf("%s.%s: the model declares it as a Hash on %q, want exactly one class", name, attr, sites)
				continue
			}
			holders++
			if prev, seen := found[sites[0]]; seen && prev != value.BMMName() {
				t.Errorf("%s: the Go types hold its values as both %s and %s", sites[0], prev, value.BMMName())
			}
			found[sites[0]] = value.BMMName()
		}
	}
	if holders == 0 {
		t.Fatal("no Go map field holding RM values found; the reverse check asserts nothing")
	}
	for _, key := range slices.Sorted(maps.Keys(found)) {
		if got, want := hashValues[key], found[key]; got != want {
			t.Errorf("hashValues[%q] = %q, want %q (a Go type holds the Hash's values as that class)", key, got, want)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(hashValues)) {
		if _, ok := found[key]; !ok {
			t.Errorf("hashValues[%q] names no Go map field holding RM values", key)
		}
	}
}
