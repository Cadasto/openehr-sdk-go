package bmmtype

import (
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/bmmgen"
	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// loadPinnedBMM loads the RM schema rminfo is generated from, with the
// schemas it includes.
func loadPinnedBMM(t *testing.T) *bmm.Schema {
	t.Helper()
	id := bmmgen.TargetRM.RootID
	schema, err := bmm.LoadAll(id, bmm.FSResolver{Root: "../../resources/bmm"})
	if err != nil {
		t.Fatalf("LoadAll(%s): %v", id, err)
	}
	return schema
}

// REQ-100 — the compiler substitutes formal parameters from this table, so
// it must name exactly the generic classes of the rminfo class universe, each
// with the parameter names the vendored BMM declares. A schema bump that adds
// a generic class, or renames a parameter, fails here.
func TestFormalParametersMatchBMM(t *testing.T) {
	schema := loadPinnedBMM(t)
	want := map[string][]string{}
	for _, name := range rminfo.Default.KnownRMTypes() {
		cls, ok := schema.ClassDefinitions[name]
		if !ok {
			cls, ok = schema.PrimitiveTypes[name]
		}
		if !ok {
			t.Errorf("rminfo class %q is not in the BMM", name)
			continue
		}
		var defs map[string]*bmm.GenericParameterDef
		switch c := cls.(type) {
		case *bmm.SimpleClass:
			defs = c.GenericParameterDefs
		case *bmm.Interface:
			defs = c.GenericParameterDefs
		}
		if len(defs) > 0 {
			want[name] = slices.Sorted(maps.Keys(defs))
		}
	}
	if len(want) == 0 {
		t.Fatal("the BMM declares no generic class rminfo knows; the schema did not load as expected")
	}

	for _, name := range slices.Sorted(maps.Keys(want)) {
		params := want[name]
		if len(params) > 1 {
			// The decoded BMM keeps the parameters in a map, so their order
			// is lost; Substitute depends on it.
			t.Errorf("%s declares %d generic parameters %v: check the order of formalParameters[%q] against the schema file and extend this test", name, len(params), params, name)
			continue
		}
		if got := formalParameters[name]; !slices.Equal(got, params) {
			t.Errorf("formalParameters[%q] = %q, want %q (from the BMM)", name, got, params)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(formalParameters)) {
		if _, ok := want[name]; !ok {
			t.Errorf("formalParameters has %q, which is not a generic class rminfo knows", name)
		}
	}
}

// REQ-100 — the other direction: every attribute type rminfo records that
// names no class or primitive type of the BMM is a formal parameter of the
// attribute's class, and the table lists it for that class. Otherwise
// Substitute would leave it in place on a parameterised owner. Today these are
// the "T" of DV_INTERVAL's and the BASE intervals' bounds, of the EVENT
// family's data, and of ORIGINAL_VERSION's data.
func TestFormalParametersCoverRMInfoTypes(t *testing.T) {
	schema := loadPinnedBMM(t)
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not list attributes")
	}
	found := 0
	for _, class := range rminfo.Default.KnownRMTypes() {
		for _, attr := range lister.AttributeNames(class) {
			typ, _ := rminfo.Default.AttributeRMType(class, attr)
			if _, isClass := schema.ClassDefinitions[typ]; isClass {
				continue
			}
			if _, isPrimitive := schema.PrimitiveTypes[typ]; isPrimitive {
				continue
			}
			found++
			if !slices.Contains(formalParameters[class], typ) {
				t.Errorf("rminfo types %s.%s as %q, which names no BMM class; formalParameters[%q] = %q does not list it", class, attr, typ, class, formalParameters[class])
			}
		}
	}
	if found == 0 {
		t.Fatal("rminfo records no attribute typed by a formal parameter; the tables did not load as expected")
	}
}
