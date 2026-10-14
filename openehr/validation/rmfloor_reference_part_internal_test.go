package validation

// rmfloor_reference_part_internal_test.go: REQ-112 OBJECT_REF family. The
// floor reports a missing part of a reference (id, namespace, type) once:
// checkObjectRef reports it on every class the rminfo hierarchy places at or
// below OBJECT_REF, and the required-set check leaves the three parts out on
// exactly those classes, so no class gets both.

import (
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// referencePartNames are the RM-mandatory parts of a reference that
// checkObjectRef reports when absent.
var referencePartNames = []string{"id", "namespace", "type"}

// REQ-112 — over every class the registry decodes, the required-set check
// leaves the parts of a reference out exactly where the hierarchy places the
// class at or below OBJECT_REF, which is also where checkObjectRef runs.
func TestFloor_REQ112_ReferencePartsFollowTheHierarchy(t *testing.T) {
	t.Parallel()
	h := pinnedSource.hierarchy
	if h == nil {
		t.Fatal("rminfo.Default answers no class hierarchy; the check has gone blind")
	}
	references := 0
	for _, class := range slices.Sorted(maps.Keys(floorRules.byClass)) {
		want := conformsTo(h, class, "OBJECT_REF")
		if want {
			references++
		}
		r := floorRules.byClass[class]
		for _, part := range referencePartNames {
			if got := r.leavesOut(part); got != want {
				t.Errorf("rules of %s leave out %s: %t, want %t, whether the hierarchy places it at or below OBJECT_REF", class, part, got, want)
			}
		}
	}
	if references < 4 {
		t.Errorf("found %d registered classes at or below OBJECT_REF, want at least OBJECT_REF, PARTY_REF, LOCATABLE_REF and ACCESS_GROUP_REF", references)
	}
}

// REQ-112 — the parts left out follow the hierarchy the dispatch is built
// from, not a list of class names: a class a doctored hierarchy places below
// OBJECT_REF runs checkObjectRef and has the three parts left out, and a class
// beside it has neither.
func TestFloor_REQ112_ReferencePartsOfADoctoredSubtype(t *testing.T) {
	t.Parallel()
	src := inventorySource{
		classes:    map[string][]string{"OBJECT_REF": nil, "X_REF": nil, "OTHER": nil},
		inventory:  map[string]inventoryEntry{},
		checks:     map[string]invariantCheck{},
		attributes: attributeChecks,
		hierarchy: hierarchyOf(rminfo.New(map[string]rminfo.ClassMeta{
			"OBJECT_REF": {},
			"X_REF":      {Parents: []string{"OBJECT_REF"}},
			"OTHER":      {},
		})),
	}
	if src.hierarchy == nil {
		t.Fatal("rminfo.New answers no class hierarchy; the check has gone blind")
	}
	rules := src.build()
	for _, tc := range []struct {
		class     string
		reference bool
	}{
		{class: "OBJECT_REF", reference: true},
		{class: "X_REF", reference: true},
		{class: "OTHER", reference: false},
	} {
		r := rules.byClass[tc.class]
		for _, part := range referencePartNames {
			if got := r.leavesOut(part); got != tc.reference {
				t.Errorf("build() rules of %s leave out %s: %t, want %t", tc.class, part, got, tc.reference)
			}
		}
		if got := len(r.checks) == 1; got != tc.reference {
			t.Errorf("build() runs %d checks on %s, want checkObjectRef alone: %t", len(r.checks), tc.class, tc.reference)
		}
	}
}
