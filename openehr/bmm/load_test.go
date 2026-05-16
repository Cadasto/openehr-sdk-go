package bmm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// resourcesDir is the relative path from openehr/bmm/ to the pinned
// BMM JSON files.
const resourcesDir = "../../" + DefaultResourcesDir

func mustOpen(t *testing.T, name string) io.ReadCloser {
	t.Helper()
	f, err := os.Open(filepath.Join(resourcesDir, name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	return f
}

// loadFixture is a helper that reads and parses one BMM file from
// resources/.
func loadFixture(t *testing.T, name string) *Schema {
	t.Helper()
	rc := mustOpen(t, name)
	defer func() { _ = rc.Close() }()
	s, err := Load(rc)
	if err != nil {
		t.Fatalf("Load(%s): %v", name, err)
	}
	return s
}

func TestLoad_eachBMM(t *testing.T) {
	// Sanity counts confirmed via Python probe on the live BMM files:
	//   base 1.3.0 → 43 classes, 29 primitives
	//   rm   1.2.0 → 146 classes
	//   am   1.4.0 → 39 classes
	//   am   2.4.0 → 75 classes
	//   lang 1.1.0 → 172 classes
	//   term 3.1.0 → 6 classes
	cases := []struct {
		file       string
		classes    int
		primitives int
		// expected schema_name + rm_release combined id
		id              string
		hasIncludes     []string
		topLevelPkgKeys []string
	}{
		{
			file:            "openehr_base_1.3.0.bmm.json",
			classes:         43,
			primitives:      29,
			id:              "openehr_base_1.3.0",
			topLevelPkgKeys: []string{"org.openehr.base.base_types", "org.openehr.base.foundation_types", "org.openehr.base.resource"},
		},
		{
			file:            "openehr_rm_1.2.0.bmm.json",
			classes:         146,
			primitives:      0,
			id:              "openehr_rm_1.2.0",
			hasIncludes:     []string{"openehr_base_1.3.0"},
			topLevelPkgKeys: []string{"org.openehr.rm.data_types", "org.openehr.rm.composition"},
		},
		{
			file:        "openehr_am_1.4.0.bmm.json",
			classes:     39,
			primitives:  0,
			id:          "openehr_am_1.4.0",
			hasIncludes: []string{"openehr_base_1.3.0"},
		},
		{
			file:        "openehr_am_2.4.0.bmm.json",
			classes:     75,
			primitives:  0,
			id:          "openehr_am_2.4.0",
			hasIncludes: []string{"openehr_base_1.3.0", "openehr_lang_1.1.0"},
		},
		{
			file:       "openehr_lang_1.1.0.bmm.json",
			classes:    172,
			primitives: 0,
			id:         "openehr_lang_1.1.0",
		},
		{
			file:       "openehr_term_3.1.0.bmm.json",
			classes:    6,
			primitives: 0,
			id:         "openehr_term_3.1.0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			s := loadFixture(t, tc.file)
			if got, want := len(s.ClassDefinitions), tc.classes; got != want {
				t.Errorf("ClassDefinitions count: got %d, want %d", got, want)
			}
			if got, want := len(s.PrimitiveTypes), tc.primitives; got != want {
				t.Errorf("PrimitiveTypes count: got %d, want %d", got, want)
			}
			if got := s.SchemaID(); got != tc.id {
				t.Errorf("SchemaID: got %q, want %q", got, tc.id)
			}
			for _, inc := range tc.hasIncludes {
				if _, ok := s.Includes[inc]; !ok {
					t.Errorf("expected includes[%q] entry, got map=%v", inc, s.Includes)
				}
			}
			for _, pk := range tc.topLevelPkgKeys {
				if _, ok := s.Packages[pk]; !ok {
					t.Errorf("expected packages[%q] entry, got keys=%v", pk, mapKeys(s.Packages))
				}
			}
		})
	}
}

func TestLoad_basePrimitivesIncludeIso8601(t *testing.T) {
	s := loadFixture(t, "openehr_base_1.3.0.bmm.json")
	// Iso8601_type is declared in primitive_types; it has _type absent
	// (default class), is_abstract=true, ancestors=[Temporal,Time_Definitions].
	pc, ok := s.PrimitiveTypes["Iso8601_type"]
	if !ok {
		t.Fatalf("Iso8601_type not in primitive_types")
	}
	sc, ok := pc.(*SimpleClass)
	if !ok {
		t.Fatalf("Iso8601_type: want *SimpleClass, got %T", pc)
	}
	if !sc.IsAbstract() {
		t.Errorf("Iso8601_type: expected abstract")
	}
	if !contains(sc.Ancestors(), "Temporal") {
		t.Errorf("Iso8601_type.ancestors does not include Temporal: %v", sc.Ancestors())
	}
}

func TestLoad_baseInterfacesAndGenerics(t *testing.T) {
	s := loadFixture(t, "openehr_base_1.3.0.bmm.json")
	// Env is P_BMM_INTERFACE.
	env, ok := s.ClassDefinitions["Env"]
	if !ok {
		t.Fatalf("Env not found")
	}
	if _, ok := env.(*Interface); !ok {
		t.Fatalf("Env: want *Interface, got %T", env)
	}
	// VALIDITY_KIND is P_BMM_ENUMERATION_STRING (item_values absent).
	vk, ok := s.ClassDefinitions["VALIDITY_KIND"]
	if !ok {
		t.Fatalf("VALIDITY_KIND not found")
	}
	enum, ok := vk.(*Enumeration)
	if !ok {
		t.Fatalf("VALIDITY_KIND: want *Enumeration, got %T", vk)
	}
	if !enum.IsStringEnum() {
		t.Errorf("VALIDITY_KIND: expected string enumeration, got kind=%q", enum.EnumKind)
	}
	if got, want := len(enum.ItemNames), 4; got != want {
		t.Errorf("VALIDITY_KIND.ItemNames: got %d, want %d", got, want)
	}
	if !reflect.DeepEqual(enum.ItemValuesString, enum.ItemNames) {
		t.Errorf("VALIDITY_KIND.ItemValuesString should default to ItemNames")
	}

	// Hash (in primitive_types) is generic with K and V.
	hash, ok := s.PrimitiveTypes["Hash"]
	if !ok {
		t.Fatalf("Hash not in primitive_types")
	}
	hsc, ok := hash.(*SimpleClass)
	if !ok {
		t.Fatalf("Hash: want *SimpleClass, got %T", hash)
	}
	if !hsc.IsGeneric() {
		t.Errorf("Hash: expected IsGeneric()=true")
	}
	if _, ok := hsc.GenericParameterDefs["K"]; !ok {
		t.Errorf("Hash: missing K generic parameter")
	}
}

func TestLoad_rmEnumerationInteger(t *testing.T) {
	s := loadFixture(t, "openehr_rm_1.2.0.bmm.json")
	pk, ok := s.ClassDefinitions["PROPORTION_KIND"]
	if !ok {
		t.Fatalf("PROPORTION_KIND not found")
	}
	enum, ok := pk.(*Enumeration)
	if !ok {
		t.Fatalf("PROPORTION_KIND: want *Enumeration, got %T", pk)
	}
	if !enum.IsIntegerEnum() {
		t.Errorf("PROPORTION_KIND: expected integer enum")
	}
	if got, want := enum.ItemValuesInt, []int64{0, 1, 2, 3, 4}; !reflect.DeepEqual(got, want) {
		t.Errorf("PROPORTION_KIND.ItemValuesInt: got %v, want %v", got, want)
	}
}

func TestLoad_baseContainerPropertyShape(t *testing.T) {
	s := loadFixture(t, "openehr_base_1.3.0.bmm.json")
	rd, ok := s.ClassDefinitions["RESOURCE_DESCRIPTION"]
	if !ok {
		t.Fatalf("RESOURCE_DESCRIPTION not found")
	}
	sc := rd.(*SimpleClass)
	oc, ok := sc.Properties["other_contributors"]
	if !ok {
		t.Fatalf("RESOURCE_DESCRIPTION.other_contributors missing")
	}
	cp, ok := oc.(*ContainerProperty)
	if !ok {
		t.Fatalf("other_contributors: want *ContainerProperty, got %T", oc)
	}
	if cp.TypeDef.ContainerType != "List" {
		t.Errorf("other_contributors: container_type %q != List", cp.TypeDef.ContainerType)
	}
	// inner type is the short form "type":"String" → normalised to *SimpleType.
	st, ok := cp.TypeDef.TypeDef.(*SimpleType)
	if !ok {
		t.Fatalf("other_contributors inner: want *SimpleType, got %T", cp.TypeDef.TypeDef)
	}
	if st.TypeName != "String" {
		t.Errorf("other_contributors inner type: %q != String", st.TypeName)
	}
	if cp.Cardinality == nil || !cp.Cardinality.UpperUnbounded {
		t.Errorf("other_contributors cardinality should be upper_unbounded; got %#v", cp.Cardinality)
	}
}

func TestLoad_baseGenericPropertyNestedDefs(t *testing.T) {
	s := loadFixture(t, "openehr_base_1.3.0.bmm.json")
	ra, ok := s.ClassDefinitions["RESOURCE_ANNOTATIONS"]
	if !ok {
		t.Fatalf("RESOURCE_ANNOTATIONS not found")
	}
	sc := ra.(*SimpleClass)
	doc, ok := sc.Properties["documentation"]
	if !ok {
		t.Fatalf("RESOURCE_ANNOTATIONS.documentation missing")
	}
	gp, ok := doc.(*GenericProperty)
	if !ok {
		t.Fatalf("documentation: want *GenericProperty, got %T", doc)
	}
	if gp.TypeDef.RootType != "Hash" {
		t.Errorf("documentation root_type %q != Hash", gp.TypeDef.RootType)
	}
	if len(gp.TypeDef.GenericParameterDefs) != 2 {
		t.Errorf("documentation: expected 2 generic_parameter_defs, got %d", len(gp.TypeDef.GenericParameterDefs))
	}
	// V should itself be a nested GenericType{Hash, …}
	v := gp.TypeDef.GenericParameterDefs["V"]
	gtV, ok := v.(*GenericType)
	if !ok {
		t.Fatalf("documentation V: want *GenericType, got %T", v)
	}
	if gtV.RootType != "Hash" {
		t.Errorf("V.root_type %q != Hash", gtV.RootType)
	}
}

func TestLoadAll_rmIncludesBaseMergesPrimitives(t *testing.T) {
	r := FSResolver{Root: resourcesDir}
	s, err := LoadAll("openehr_rm_1.2.0", r)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	// Should now carry base primitives (29 of them).
	if got := len(s.PrimitiveTypes); got != 29 {
		t.Errorf("merged primitives: got %d, want 29", got)
	}
	// Should have base classes + rm classes minus the 5 names that are
	// re-declared (shadowed) in rm: AUTHORED_RESOURCE, CODE_PHRASE,
	// RESOURCE_DESCRIPTION, RESOURCE_DESCRIPTION_ITEM, TRANSLATION_DETAILS.
	const overlap = 5
	if got, want := len(s.ClassDefinitions), 43+146-overlap; got != want {
		t.Errorf("merged classes: got %d, want %d", got, want)
	}
	// Sanity: a known base class should now be reachable via the
	// descendant schema.
	if _, ok := s.ClassDefinitions["OBJECT_REF"]; !ok {
		t.Errorf("merged schema is missing OBJECT_REF from base")
	}
	// And a known rm class:
	if _, ok := s.ClassDefinitions["COMPOSITION"]; !ok {
		t.Errorf("merged schema is missing COMPOSITION from rm")
	}
}

func TestLoadAll_am2IncludesBaseAndLang(t *testing.T) {
	r := FSResolver{Root: resourcesDir}
	s, err := LoadAll("openehr_am_2.4.0", r)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if got := len(s.PrimitiveTypes); got != 29 {
		t.Errorf("am2 merged primitives: got %d, want 29", got)
	}
	// am2 (75) + lang (172) + base (43) = 290, less any shared names
	// across the trio. We don't pin a precise count here because the
	// shape of the overlap is incidental; instead we assert lower- and
	// upper-bound sanity plus presence of marker classes.
	got := len(s.ClassDefinitions)
	if got < 250 || got > 43+172+75 {
		t.Errorf("am2 merged classes: got %d, want roughly %d", got, 43+172+75)
	}
	if _, ok := s.ClassDefinitions["OBJECT_REF"]; !ok {
		t.Errorf("am2 merged: missing OBJECT_REF (base)")
	}
}

func TestLoadAll_descendantShadowsAncestor(t *testing.T) {
	// Descendant declares class X, ancestor also declares X: the
	// descendant's value MUST win and no error MUST be returned.
	parent := `{
		"schema_name": "p",
		"rm_release": "1.0.0",
		"class_definitions": {
			"X": { "name": "X", "documentation": "from parent" }
		}
	}`
	child := `{
		"schema_name": "c",
		"rm_release": "1.0.0",
		"includes": { "p_1.0.0": { "id": "p_1.0.0" } },
		"class_definitions": {
			"X": { "name": "X", "documentation": "from child" }
		}
	}`
	r := MapResolver{
		"c_1.0.0": []byte(child),
		"p_1.0.0": []byte(parent),
	}
	s, err := LoadAll("c_1.0.0", r)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	x, ok := s.ClassDefinitions["X"]
	if !ok {
		t.Fatalf("X not present after merge")
	}
	if got := x.Documentation(); got != "from child" {
		t.Errorf("descendant should shadow ancestor: got doc %q", got)
	}
}

func TestLoadAll_siblingAncestorsConflict(t *testing.T) {
	// Two siblings (parent1 and parent2) both define class X; the
	// descendant does not. mergeAncestor MUST surface ErrSchemaConflict.
	parent1 := `{
		"schema_name": "p1",
		"rm_release": "1.0.0",
		"class_definitions": {
			"X": { "name": "X" }
		}
	}`
	parent2 := `{
		"schema_name": "p2",
		"rm_release": "1.0.0",
		"class_definitions": {
			"X": { "name": "X" }
		}
	}`
	child := `{
		"schema_name": "c",
		"rm_release": "1.0.0",
		"includes": {
			"p1_1.0.0": { "id": "p1_1.0.0" },
			"p2_1.0.0": { "id": "p2_1.0.0" }
		}
	}`
	r := MapResolver{
		"c_1.0.0":  []byte(child),
		"p1_1.0.0": []byte(parent1),
		"p2_1.0.0": []byte(parent2),
	}
	_, err := LoadAll("c_1.0.0", r)
	if err == nil {
		t.Fatalf("expected ErrSchemaConflict")
	}
	if !errors.Is(err, ErrSchemaConflict) {
		t.Errorf("got %v, want errors.Is(err, ErrSchemaConflict)", err)
	}
}

func TestLoadAll_circularIncludes(t *testing.T) {
	a := `{"schema_name":"a","rm_release":"1.0.0","includes":{"b_1.0.0":{"id":"b_1.0.0"}}}`
	b := `{"schema_name":"b","rm_release":"1.0.0","includes":{"a_1.0.0":{"id":"a_1.0.0"}}}`
	r := MapResolver{
		"a_1.0.0": []byte(a),
		"b_1.0.0": []byte(b),
	}
	_, err := LoadAll("a_1.0.0", r)
	if err == nil {
		t.Fatalf("expected ErrCircularIncludes")
	}
	if !errors.Is(err, ErrCircularIncludes) {
		t.Errorf("got %v, want errors.Is(err, ErrCircularIncludes)", err)
	}
}

func TestLoadAll_resolverMiss(t *testing.T) {
	r := MapResolver{}
	_, err := LoadAll("does_not_exist", r)
	if err == nil {
		t.Fatalf("expected ErrSchemaNotFound")
	}
	if !errors.Is(err, ErrSchemaNotFound) {
		t.Errorf("got %v, want errors.Is(err, ErrSchemaNotFound)", err)
	}
}

func TestLoad_malformedJSON(t *testing.T) {
	_, err := Load(bytes.NewReader([]byte("{not json")))
	if err == nil {
		t.Fatalf("expected error on malformed JSON")
	}
}

func TestLoad_emptyInput(t *testing.T) {
	_, err := Load(bytes.NewReader(nil))
	if err == nil {
		t.Fatalf("expected error on empty input")
	}
}

func TestLoad_nilReader(t *testing.T) {
	_, err := Load(nil)
	if err == nil {
		t.Fatalf("expected error on nil reader")
	}
}

func TestLoad_unknownType(t *testing.T) {
	doc := `{
		"schema_name": "x",
		"rm_release": "1.0.0",
		"class_definitions": {
			"Y": {
				"name": "Y",
				"_type": "P_BMM_NOT_A_REAL_TYPE"
			}
		}
	}`
	_, err := Load(bytes.NewReader([]byte(doc)))
	if err == nil {
		t.Fatalf("expected unknown _type error")
	}
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("got %v, want errors.Is(err, ErrUnknownType)", err)
	}
}

func TestLoad_unknownPropertyType(t *testing.T) {
	doc := `{
		"schema_name": "x",
		"rm_release": "1.0.0",
		"class_definitions": {
			"Y": {
				"name": "Y",
				"properties": { "p": {"_type": "P_BMM_NONSENSE_PROPERTY", "name": "p"} }
			}
		}
	}`
	_, err := Load(bytes.NewReader([]byte(doc)))
	if err == nil {
		t.Fatalf("expected unknown _type error on property")
	}
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("got %v, want errors.Is(err, ErrUnknownType)", err)
	}
}

func TestLoad_missingClassName(t *testing.T) {
	doc := `{
		"schema_name": "x",
		"rm_release": "1.0.0",
		"class_definitions": { "X": {} }
	}`
	_, err := Load(bytes.NewReader([]byte(doc)))
	if err == nil {
		t.Fatalf("expected missing-field error")
	}
	if !errors.Is(err, ErrMissingField) {
		t.Errorf("got %v, want errors.Is(err, ErrMissingField)", err)
	}
}

func TestLoad_missingSchemaName(t *testing.T) {
	doc := `{"class_definitions":{}}`
	_, err := Load(bytes.NewReader([]byte(doc)))
	if err == nil {
		t.Fatalf("expected missing schema_name")
	}
	if !errors.Is(err, ErrMissingField) {
		t.Errorf("got %v, want errors.Is(err, ErrMissingField)", err)
	}
}

func TestFSResolver_missing(t *testing.T) {
	r := FSResolver{Root: t.TempDir()}
	_, err := r.Resolve(context.Background(), "missing")
	if err == nil {
		t.Fatalf("expected ErrSchemaNotFound")
	}
	if !errors.Is(err, ErrSchemaNotFound) {
		t.Errorf("got %v, want ErrSchemaNotFound", err)
	}
}

func TestMapResolver_missing(t *testing.T) {
	_, err := MapResolver{}.Resolve(context.Background(), "x")
	if err == nil {
		t.Fatalf("expected ErrSchemaNotFound")
	}
	if !errors.Is(err, ErrSchemaNotFound) {
		t.Errorf("got %v, want ErrSchemaNotFound", err)
	}
}

func mapKeys(m map[string]*Package) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
