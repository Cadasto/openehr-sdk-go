package bmmgen

import (
	"encoding/json"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// mustSimpleClass builds a *bmm.SimpleClass from a compact JSON fragment.
// Panics on decode failure — tests only.
func mustSimpleClass(t *testing.T, raw string) *bmm.SimpleClass {
	t.Helper()
	sc := &bmm.SimpleClass{}
	if err := json.Unmarshal([]byte(raw), sc); err != nil {
		t.Fatalf("mustSimpleClass: %v", err)
	}
	return sc
}

// TestEffectivePropertiesCyclicAncestors verifies that effectiveProperties
// terminates when two classes list each other as ancestors (a cycle that
// would cause infinite recursion without the visitedClass guard).
//
// Without the guard the visit closure recurses A→B→A→B… until the
// goroutine stack overflows. With the guard it returns immediately on
// the second visit to a class.
func TestEffectivePropertiesCyclicAncestors(t *testing.T) {
	// Build a minimal *Plan whose Classes map has:
	//   A — ancestors: ["B"]
	//   B — ancestors: ["A"]
	// Both have no properties (we only care that the function returns,
	// not what it returns).
	classA := mustSimpleClass(t, `{"name":"A","ancestors":["B"]}`)
	classB := mustSimpleClass(t, `{"name":"B","ancestors":["A"]}`)

	plan := &Plan{
		Target: TargetRM,
		Classes: map[string]*PlannedClass{
			"A": {BMMName: "A", GoName: "A", Class: classA},
			"B": {BMMName: "B", GoName: "B", Class: classB},
		},
		AbstractDescendants: map[string][]string{},
		ConcreteSubtypes:    map[string][]string{},
		CyclicSingleProps:   map[string]map[string]bool{},
	}

	// If the cycle guard is missing, this call will stack-overflow-panic.
	// The test completing (without panic or timeout) is the assertion.
	attrs, order := effectiveProperties(plan, "A")
	_ = attrs
	_ = order
}

// TestPropToAttrGenericPropertyWithoutTypeDef pins the fallback
// rootTypeName documents: a generic property with no type definition
// reports "Any". The loader refuses such a property, but a hand-built plan
// can carry one, and the nil definition reaches rootTypeName as a
// non-nil interface holding a nil pointer.
func TestPropToAttrGenericPropertyWithoutTypeDef(t *testing.T) {
	p := &bmm.GenericProperty{}
	p.Name = "untyped"
	attr, ok := propToAttr(p, nil)
	if !ok {
		t.Fatal("propToAttr(GenericProperty with nil TypeDef): ok = false, want true")
	}
	if attr.typeName != "Any" {
		t.Errorf("propToAttr(GenericProperty with nil TypeDef).typeName = %q, want %q", attr.typeName, "Any")
	}
	if attr.boundTypeName != "Any" {
		t.Errorf("propToAttr(GenericProperty with nil TypeDef).boundTypeName = %q, want %q", attr.boundTypeName, "Any")
	}
}

// TestBoundTypeName_REQ048 pins how a BMM type reference is spelled with the
// generic parameters it binds (REQ-048): BMM's type_name form (root, then the
// parameters in angle brackets, comma-separated, no space), the one ITS-JSON
// uses for DV_INTERVAL<DV_DATE>. A type that leaves any parameter open, at
// any depth, keeps its bare root: "open" means a formal parameter of the
// class declaring the property, passed as formal.
func TestBoundTypeName_REQ048(t *testing.T) {
	simple := func(name string) bmm.Type { return &bmm.SimpleType{TypeName: name} }
	generic := func(root string, params ...bmm.Type) bmm.Type {
		return &bmm.GenericType{RootType: root, GenericParameters: params}
	}
	formalT := map[string]*bmm.GenericParameterDef{"T": {Name: "T"}}

	cases := []struct {
		name   string
		typ    bmm.Type
		formal map[string]*bmm.GenericParameterDef
		want   string
	}{
		{name: "simple type", typ: simple("DV_TEXT"), want: "DV_TEXT"},
		{name: "open simple type", typ: simple("T"), formal: formalT, want: "T"},
		{name: "one concrete parameter", typ: generic("DV_INTERVAL", simple("DV_DATE")), want: "DV_INTERVAL<DV_DATE>"},
		{
			name: "several parameters keep declaration order",
			typ:  generic("Hash", simple("String"), simple("TRANSLATION_DETAILS")),
			want: "Hash<String,TRANSLATION_DETAILS>",
		},
		{
			name: "nested generic parameter",
			typ:  generic("Hash", simple("String"), generic("Hash", simple("String"), simple("String"))),
			want: "Hash<String,Hash<String,String>>",
		},
		{
			name: "keyed parameters are ordered by name",
			typ: &bmm.GenericType{RootType: "Hash", GenericParameterDefs: map[string]bmm.Type{
				"V": generic("List", simple("DV_TEXT")),
				"K": simple("String"),
			}},
			want: "Hash<String,List<DV_TEXT>>",
		},
		{
			name: "container parameter",
			typ:  generic("TABLE", &bmm.ContainerType{ContainerType: "List", TypeDef: simple("THING")}, simple("String")),
			want: "TABLE<List<THING>,String>",
		},
		{name: "open parameter", typ: generic("EVENT", simple("T")), formal: formalT, want: "EVENT"},
		{
			name:   "mix of open and concrete parameters",
			typ:    generic("Hash", simple("String"), simple("T")),
			formal: formalT,
			want:   "Hash",
		},
		{
			name:   "open parameter nested under a concrete one",
			typ:    generic("Hash", simple("String"), &bmm.ContainerType{ContainerType: "List", TypeDef: simple("T")}),
			formal: formalT,
			want:   "Hash",
		},
		{
			name: "a name the declaring class does not declare as formal is spelled as given",
			typ:  generic("EVENT", simple("T")),
			want: "EVENT<T>",
		},
		{name: "generic type with no parameters", typ: generic("ROOT"), want: "ROOT"},
		{name: "nil type", typ: nil, want: "Any"},
		{name: "nil generic type", typ: (*bmm.GenericType)(nil), want: "Any"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := boundTypeName(tc.typ, tc.formal); got != tc.want {
				t.Errorf("boundTypeName(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestEffectivePropertiesBindsGenericParameters_REQ048 reads the bound type
// (REQ-048) off the pinned RM: every row the BMM declares with a concrete parameter
// keeps it, the bare type name beside it is unchanged, and a row whose
// parameter is open on its declaring class stays bare.
func TestEffectivePropertiesBindsGenericParameters_REQ048(t *testing.T) {
	plan := rmInfoTestPlan(t)
	cases := []struct {
		class, attr string
		wantType    string
		wantBound   string
	}{
		{"PARTY_RELATIONSHIP", "time_validity", "DV_INTERVAL", "DV_INTERVAL<DV_DATE>"},
		{"CONTACT", "time_validity", "DV_INTERVAL", "DV_INTERVAL<DV_DATE>"},
		{"CAPABILITY", "time_validity", "DV_INTERVAL", "DV_INTERVAL<DV_DATE>"},
		{"ROLE", "time_validity", "DV_INTERVAL", "DV_INTERVAL<DV_DATE>"},
		{"PARTICIPATION", "time", "DV_INTERVAL", "DV_INTERVAL<DV_DATE_TIME>"},
		{"DV_QUANTITY", "normal_range", "DV_INTERVAL", "DV_INTERVAL<DV_QUANTITY>"},
		{"DV_COUNT", "normal_range", "DV_INTERVAL", "DV_INTERVAL<DV_COUNT>"},
		{"DV_PROPORTION", "normal_range", "DV_INTERVAL", "DV_INTERVAL<DV_PROPORTION>"},
		// A container reports its element type, bound like any other.
		{"DV_QUANTITY", "other_reference_ranges", "REFERENCE_RANGE", "REFERENCE_RANGE<DV_QUANTITY>"},
		{"DV_COUNT", "other_reference_ranges", "REFERENCE_RANGE", "REFERENCE_RANGE<DV_COUNT>"},
		{"DV_PROPORTION", "other_reference_ranges", "REFERENCE_RANGE", "REFERENCE_RANGE<DV_PROPORTION>"},
		{"DV_ORDERED", "other_reference_ranges", "REFERENCE_RANGE", "REFERENCE_RANGE<DV_ORDERED>"},
		// Inherited from DV_ORDERED's declaration, as the fold takes it.
		{"DV_DURATION", "other_reference_ranges", "REFERENCE_RANGE", "REFERENCE_RANGE<DV_ORDERED>"},
		{"OBSERVATION", "data", "HISTORY", "HISTORY<ITEM_STRUCTURE>"},
		{"OBSERVATION", "state", "HISTORY", "HISTORY<ITEM_STRUCTURE>"},
		// Several parameters, and the keyed form nested twice.
		{"AUTHORED_RESOURCE", "translations", "Hash", "Hash<String,TRANSLATION_DETAILS>"},
		{"RESOURCE_ANNOTATIONS", "documentation", "Hash", "Hash<String,Hash<String,Hash<String,String>>>"},
		// Open on the declaring class: bare.
		{"HISTORY", "events", "EVENT", "EVENT"},
		{"EVENT", "data", "T", "T"},
		{"DV_INTERVAL", "lower", "T", "T"},
		// The BMM declares these without parameters at all.
		{"REFERENCE_RANGE", "range", "DV_INTERVAL", "DV_INTERVAL"},
		{"DV_DURATION", "normal_range", "DV_INTERVAL", "DV_INTERVAL"},
		{"COMPOSITION", "category", "DV_CODED_TEXT", "DV_CODED_TEXT"},
	}
	for _, tc := range cases {
		attrs, _ := effectiveProperties(plan, tc.class)
		got, ok := attrs[tc.attr]
		if !ok {
			t.Errorf("%s.%s: absent from effective properties", tc.class, tc.attr)
			continue
		}
		if got.typeName != tc.wantType {
			t.Errorf("%s.%s typeName = %q, want %q", tc.class, tc.attr, got.typeName, tc.wantType)
		}
		if got.boundTypeName != tc.wantBound {
			t.Errorf("%s.%s boundTypeName = %q, want %q", tc.class, tc.attr, got.boundTypeName, tc.wantBound)
		}
	}
}
