package rminfo_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

func TestIsNonStorableAttr(t *testing.T) {
	cases := []struct {
		parentRM, attr string
		want           bool
	}{
		{"POINT_EVENT", "offset", true},
		{"INTERVAL_EVENT", "offset", true},
		{"DV_QUANTITY", "is_integral", true},
		{"DV_PROPORTION", "is_integral", true},
		{"RESOURCE_DESCRIPTION", "parent_resource", true},
		{"POINT_EVENT", "time", false},
		{"DV_QUANTITY", "magnitude", false},
		{"OBSERVATION", "data", false},
		{"RESOURCE_DESCRIPTION", "details", false},
	}
	for _, tc := range cases {
		t.Run(tc.parentRM+"."+tc.attr, func(t *testing.T) {
			if got := rminfo.IsNonStorableAttr(tc.parentRM, tc.attr); got != tc.want {
				t.Fatalf("IsNonStorableAttr(%q, %q) = %v, want %v", tc.parentRM, tc.attr, got, tc.want)
			}
		})
	}
}

// TestIsNonStorableAttr_REQ112_EveryBMMFunction pins § REQ-112 Driver: the
// set IsNonStorableAttr names is every BMM function of the class or of an
// ancestor that is a property neither of the class nor of any class
// descending from it. A function the class declares, one an ancestor
// declares, and a name that is a function on one class and a property on
// another are each answered for the class asked; a function of an abstract
// class that a descendant stores as a property is not named.
func TestIsNonStorableAttr_REQ112_EveryBMMFunction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		class, attr string
		want        bool
	}{
		{class: "EVENT", attr: "offset", want: true},                       // declared on the class, which is abstract
		{class: "INTERVAL_EVENT", attr: "interval_start_time", want: true}, // declared on the class
		{class: "ELEMENT", attr: "is_null", want: true},
		{class: "COMPOSITION", attr: "is_persistent", want: true},
		{class: "OBSERVATION", attr: "is_archetype_root", want: true}, // declared on the ancestor LOCATABLE
		{class: "DV_DURATION", attr: "magnitude", want: true},         // declared on the ancestor DV_QUANTIFIED
		{class: "DV_DURATION", attr: "years", want: true},             // declared on Iso8601_duration, a BASE primitive type
		{class: "DV_QUANTITY", attr: "magnitude", want: false},        // a function of DV_QUANTIFIED, a property here
		{class: "DV_COUNT", attr: "magnitude", want: false},
		{class: "DV_AMOUNT", attr: "magnitude", want: false},     // a property of the descendant DV_QUANTITY
		{class: "DV_QUANTIFIED", attr: "magnitude", want: false}, // a property of the descendants DV_QUANTITY and DV_COUNT
		{class: "DV_PROPORTION", attr: "magnitude", want: true},  // a function here, and DV_PROPORTION has no descendant
		{class: "VERSION", attr: "data", want: false},            // a property of the descendant ORIGINAL_VERSION
		{class: "VERSION", attr: "is_branch", want: true},
		{class: "ORIGINAL_VERSION", attr: "data", want: false}, // a function of VERSION, a property here
		{class: "EVENT", attr: "width", want: false},           // a property of a descendant, not a function
		{class: "ISM_TRANSITION", attr: "name", want: false},
		{class: "NOT_AN_RM_CLASS", attr: "offset", want: false},
	}
	for _, tc := range tests {
		if got := rminfo.IsNonStorableAttr(tc.class, tc.attr); got != tc.want {
			t.Errorf("IsNonStorableAttr(%q, %q) = %v, want %v", tc.class, tc.attr, got, tc.want)
		}
	}
}

// TestIsNonStorableAttr_REQ112_NoPropertyIsAFunction pins the other half of §
// REQ-112 Driver's set: a property of the class is never one of its
// functions, whatever an ancestor declares under the name, so the only
// property the set names is RESOURCE_DESCRIPTION.parent_resource. The census
// covers every attribute of every class Default knows.
func TestIsNonStorableAttr_REQ112_NoPropertyIsAFunction(t *testing.T) {
	t.Parallel()
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not implement rminfo.AttributeLister")
	}
	var named []string
	for _, class := range rminfo.Default.KnownRMTypes() {
		for _, attr := range lister.AttributeNames(class) {
			if rminfo.IsNonStorableAttr(class, attr) {
				named = append(named, class+"."+attr)
			}
		}
	}
	if want := []string{"RESOURCE_DESCRIPTION.parent_resource"}; !slices.Equal(named, want) {
		t.Errorf("properties IsNonStorableAttr names = %q, want %q", named, want)
	}
}

// TestIsNonStorableAttr_REQ112_NoDescendantPropertyIsAFunction pins § REQ-112
// Driver's descendant rule: a name IsNonStorableAttr names on a class is a
// property of no class descending from it, so no instance the class denotes
// stores it. A generator walk or a validator that asks with an abstract class,
// as an OPT node typed DV_AMOUNT does, then never skips what the concrete
// DV_QUANTITY it builds or reads carries. The census covers every class
// Default knows against every class that conforms to it.
func TestIsNonStorableAttr_REQ112_NoDescendantPropertyIsAFunction(t *testing.T) {
	t.Parallel()
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not implement rminfo.AttributeLister")
	}
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default does not implement rminfo.Hierarchy")
	}
	classes := rminfo.Default.KnownRMTypes()
	var named []string
	for _, class := range classes {
		for _, sub := range classes {
			if sub == class {
				continue
			}
			if conforms, _ := h.ConformsTo(sub, class); !conforms {
				continue
			}
			for _, attr := range lister.AttributeNames(sub) {
				if rminfo.IsNonStorableAttr(class, attr) {
					named = append(named, class+"."+attr+" (a property of "+sub+")")
				}
			}
		}
	}
	if len(named) > 0 {
		t.Errorf("IsNonStorableAttr names a property of a descendant: %q", named)
	}
}

// TestIsNonStorableAttr_REQ112_RequiredAttributesNoValueCanHold pins REQ-112
// Driver: the floor leaves out the attributes IsNonStorableAttr names, and an
// RM-mandatory attribute typed by a class with no concrete descendant must be
// among them. No value can be held there and no serialization carries one, so
// a floor that required it would report it on every instance of its class.
// The census covers every concrete class Default knows, so a BMM bump that
// adds such an attribute fails here until the predicate names it.
func TestIsNonStorableAttr_REQ112_RequiredAttributesNoValueCanHold(t *testing.T) {
	t.Parallel()
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default does not implement rminfo.Hierarchy")
	}
	var found []string
	for _, class := range rminfo.Default.KnownRMTypes() {
		if abstract, _ := h.IsAbstract(class); abstract {
			continue
		}
		for _, attr := range rminfo.Default.RequiredAttributes(class) {
			declared, ok := rminfo.Default.AttributeRMType(class, attr)
			if !ok {
				t.Fatalf("AttributeRMType(%q, %q) not found for an attribute RequiredAttributes lists", class, attr)
			}
			// A primitive type or a formal parameter is not a known class,
			// and a known class with a concrete descendant can hold a value.
			if concrete, known := h.ConcreteDescendants(bmmtype.Class(declared)); !known || len(concrete) > 0 {
				continue
			}
			found = append(found, class+"."+attr)
			if !rminfo.IsNonStorableAttr(class, attr) {
				t.Errorf("IsNonStorableAttr(%q, %q) = false, want true: %s.%s is RM-mandatory and typed %s, a class with no concrete descendant, so the floor would report it absent on every %s",
					class, attr, class, attr, declared, class)
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("the census found no RM-mandatory attribute typed by a class with no concrete descendant, so it checks nothing; the pinned RM has RESOURCE_DESCRIPTION.parent_resource")
	}
	t.Logf("RM-mandatory attributes typed by a class with no concrete descendant: %v", found)
}
