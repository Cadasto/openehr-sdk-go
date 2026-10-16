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

// REQ-171 § One walk — the attributes the table of each class walks, built
// once per class, are the attributes it lists that a serialization carries
// and the metadata types, in the class's order, and find returns each by its
// name and no other. Read on a node in each role, each is the attribute the
// walk built per node before: its name, declared type and shape as the
// metadata gives them; mandatory when the metadata marks it mandatory for
// the class, no rule of the class reports its absence itself and, for a
// single-valued one, the role does not let it be blank; with no template
// declaration. Each records whether its declared type has RM rules.
func TestREQ171_ClassTableWalksEachAttributeAsTheWalkReadsIt(t *testing.T) {
	t.Parallel()
	code := classAttributes["CODE_PHRASE"].find("code_string")
	if code == nil || !code.inRole(roleNone).mandatory || code.inRole(roleScaleSymbolCode).mandatory || !code.noRules {
		t.Fatalf("precondition: CODE_PHRASE code_string reads as %+v; want a mandatory String, not mandatory in a DV_SCALE symbol's code", code)
	}
	if lang := classAttributes["COMPOSITION"].find("language"); lang == nil || lang.noRules {
		t.Fatalf("precondition: COMPOSITION language reads as %+v; want a CODE_PHRASE, which has RM rules", lang)
	}
	roles := []floorRole{roleNone, roleScaleSymbol, roleScaleSymbolCode, roleArchetypedID}
	for _, class := range slices.Sorted(maps.Keys(classAttributes)) {
		table, rules := classAttributes[class], floorRules.byClass[class]
		var walked []attributeFacts
		for _, f := range table.listed {
			if f.NonStorable || !f.Typed {
				if got := table.find(f.Name); got != nil {
					t.Errorf("%s table finds %s, which it does not walk, as %+v; want none", class, f.Name, got)
				}
				continue
			}
			walked = append(walked, f)
		}
		if len(table.walked) != len(walked) {
			t.Errorf("%s table walks %d attributes, want %d", class, len(table.walked), len(walked))
			continue
		}
		for i, f := range walked {
			if got := table.find(f.Name); got != &table.walked[i] {
				t.Errorf("%s table finds %s as %p, want the attribute at position %d, %p", class, f.Name, got, i, &table.walked[i])
			}
			shape := shapeFrom(f.Container, f.PrimitiveItems, f.Hash)
			declared, _ := rulesFor(f.RMType)
			for _, role := range roles {
				want := engineAttr{
					name:      f.Name,
					rmType:    f.RMType,
					shape:     shape,
					mandatory: f.Mandatory && !rules.leavesOut(f.Name) && (shape != shapeSingle || !mayBeBlank(role, f.Name)),
					noRules:   declared == nil,
				}
				if got := table.walked[i].inRole(role); got != want {
					t.Errorf("%s %s in role %d reads as %+v, want %+v", class, f.Name, role, got, want)
				}
			}
		}
	}
	if got := (*classAttributeTable)(nil).find("value"); got != nil {
		t.Errorf("a nil table finds value as %+v, want none", got)
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
