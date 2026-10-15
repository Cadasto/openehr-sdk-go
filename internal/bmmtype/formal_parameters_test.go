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

// REQ-101 — the composition builder checks a value at an attribute that its
// class types with a formal parameter, on an owner whose type name leaves
// the parameter open, against the parameter's constraint, which
// ParameterConstraint reads from parameterConstraints. This holds that table
// to the vendored BMM: every parameter formalParameters lists must have the
// conforms_to_type that the BMM's GenericParameterDef sets it, or, where the
// class sets none, that its generic ancestor sets the parameter of the same
// name. A schema bump that adds, drops or changes a constraint fails here.
func TestREQ101_ParameterConstraintsMatchBMM(t *testing.T) {
	schema := loadPinnedBMM(t)
	constrained := 0
	for _, class := range slices.Sorted(maps.Keys(formalParameters)) {
		for _, param := range formalParameters[class] {
			want := bmmParameterConstraint(t, schema, class, param)
			if want != "" {
				constrained++
			}
			if got, ok := ParameterConstraint(class, param); !ok || got != want {
				t.Errorf("ParameterConstraint(%q, %q) = (%q, %v), want (%q, true) (from the BMM)", class, param, got, ok, want)
			}
		}
	}
	if constrained == 0 {
		t.Fatal("the BMM constrains no formal parameter of a class rminfo knows; the schema did not load as expected")
	}
	for _, class := range slices.Sorted(maps.Keys(parameterConstraints)) {
		for _, param := range slices.Sorted(maps.Keys(parameterConstraints[class])) {
			if !slices.Contains(formalParameters[class], param) {
				t.Errorf("parameterConstraints has %s.%s, which formalParameters does not list", class, param)
			}
			if parameterConstraints[class][param] == "" {
				t.Errorf("parameterConstraints has %s.%s with an empty constraint; leave an unconstrained parameter out", class, param)
			}
		}
	}
	for _, tc := range []struct{ class, param string }{
		{class: "DV_INTERVAL", param: "U"},
		{class: "DV_TEXT", param: "T"},
	} {
		if got, ok := ParameterConstraint(tc.class, tc.param); ok || got != "" {
			t.Errorf("ParameterConstraint(%q, %q) = (%q, %v), want (\"\", false): %s has no formal parameter %s", tc.class, tc.param, got, ok, tc.class, tc.param)
		}
	}
}

// bmmParameterConstraint returns the conforms_to_type that the schema sets
// the formal parameter param of class. Where class sets none, it returns the
// one a generic ancestor sets the parameter of the same name: the RM schema
// writes a generic ancestor without actual parameters ("EVENT" for
// POINT_EVENT), so the parameter passes to it by name. It returns "" when no
// constraint is set.
func bmmParameterConstraint(t *testing.T, schema *bmm.Schema, class, param string) string {
	t.Helper()
	cls, ok := schemaClass(schema, class)
	if !ok {
		t.Fatalf("class %q is not in the BMM", class)
	}
	def := genericParameterDefs(cls)[param]
	if def == nil {
		t.Fatalf("the BMM declares no generic parameter %q on %s", param, class)
	}
	if def.ConformsToType != "" {
		return def.ConformsToType
	}
	for _, ancestor := range cls.Ancestors() {
		name, params, ok := Split(ancestor)
		if !ok {
			t.Fatalf("%s has an ancestor %q that is not a type name", class, ancestor)
		}
		parent, ok := schemaClass(schema, name)
		if !ok || genericParameterDefs(parent)[param] == nil {
			continue
		}
		if len(params) > 0 {
			t.Fatalf("%s names its ancestor as %q, with actual parameters: read the constraint on %s from them and extend this test", class, ancestor, param)
		}
		if c := bmmParameterConstraint(t, schema, name, param); c != "" {
			return c
		}
	}
	return ""
}

// schemaClass looks name up among the schema's classes and primitive types.
func schemaClass(schema *bmm.Schema, name string) (bmm.Class, bool) {
	if cls, ok := schema.ClassDefinitions[name]; ok {
		return cls, true
	}
	cls, ok := schema.PrimitiveTypes[name]
	return cls, ok
}

// genericParameterDefs returns the formal generic parameters cls declares,
// by name, or nil when it declares none.
func genericParameterDefs(cls bmm.Class) map[string]*bmm.GenericParameterDef {
	switch c := cls.(type) {
	case *bmm.SimpleClass:
		return c.GenericParameterDefs
	case *bmm.Interface:
		return c.GenericParameterDefs
	}
	return nil
}
