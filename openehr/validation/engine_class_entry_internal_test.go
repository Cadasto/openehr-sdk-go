package validation

// engine_class_entry_internal_test.go: REQ-171 § One walk — what the walk
// reads of the class of a node, its RM rules and the table of its
// attributes, is found in one lookup per node, and is what a lookup of each
// in its own table finds.

import (
	"maps"
	"slices"
	"testing"
)

// REQ-171 § One walk — for every class that has RM rules or an attribute
// table, for a generic instantiation and for a type no class has, the one
// lookup the walk makes per node finds the rules [floorRules] holds for the
// bare class and the table [classAttributes] holds for it, each nil where its
// own table holds none.
func TestREQ171_ClassEntryHoldsTheRulesAndTheTableOfTheClass(t *testing.T) {
	t.Parallel()
	if r, tb := floorRules.byClass["LOCATABLE"], classAttributes["LOCATABLE"]; r != nil || tb == nil {
		t.Fatalf("precondition: LOCATABLE has rules %v and table %v, want a table and no rules", r, tb)
	}
	types := slices.Concat(slices.Sorted(maps.Keys(floorRules.byClass)), slices.Sorted(maps.Keys(classAttributes)))
	types = append(types, "DV_INTERVAL<DV_QUANTITY>", "NO_SUCH_CLASS")
	for _, rmType := range types {
		entry, class := classOf(rmType)
		want := rmType
		if rmType == "DV_INTERVAL<DV_QUANTITY>" {
			want = "DV_INTERVAL"
		}
		if class != want || entry.rules != floorRules.byClass[want] || entry.attrs != classAttributes[want] {
			t.Errorf("classOf(%s) = rules %p, table %p, class %s; want rules %p, table %p, class %s",
				rmType, entry.rules, entry.attrs, class, floorRules.byClass[want], classAttributes[want], want)
		}
	}
}

// REQ-171 § One walk — a class that has RM rules and no attribute table
// keeps its rules, and one that has a table and no rules keeps its table:
// joining the two tables loses neither side.
func TestREQ171_JoinClassesKeepsAClassOfOneTableOnly(t *testing.T) {
	t.Parallel()
	rules := &classRules{index: 7}
	table := &classAttributeTable{}
	got := joinClasses(map[string]*classRules{"RULES_ONLY": rules}, map[string]*classAttributeTable{"TABLE_ONLY": table})
	want := map[string]classEntry{"RULES_ONLY": {rules: rules}, "TABLE_ONLY": {attrs: table}}
	if !maps.Equal(got, want) {
		t.Errorf("joinClasses(rules of RULES_ONLY, table of TABLE_ONLY) = %v, want %v", got, want)
	}
}
