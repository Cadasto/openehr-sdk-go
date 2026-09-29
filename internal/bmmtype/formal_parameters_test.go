package bmmtype

import (
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// formalParameters must name exactly the generic classes of the rminfo class
// universe, each with the parameter names the vendored BMM declares. A schema
// bump that adds a generic class, or renames a parameter, fails here.
func TestFormalParametersMatchBMM(t *testing.T) {
	schema, err := bmm.LoadAll("openehr_rm_1.2.0", bmm.FSResolver{Root: "../../resources/bmm"})
	if err != nil {
		t.Fatalf("LoadAll(openehr_rm_1.2.0): %v", err)
	}
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
