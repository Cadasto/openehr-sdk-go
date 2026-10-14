package rminfo_test

import (
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
