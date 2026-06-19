package rmpath_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
)

// REQ-121 — locatable path read access.

// vitalSigns builds a blood-pressure-shaped composition:
//
//	COMPOSITION[openEHR-EHR-COMPOSITION.encounter.v1] "Vital Signs"
//	  content → OBSERVATION[openEHR-EHR-OBSERVATION.blood_pressure.v1] "Blood pressure"
//	    data (HISTORY[at0001])
//	      events → POINT_EVENT[at0006] "Any event"
//	        data (ITEM_TREE[at0003])
//	          items → ELEMENT[at0004] "Systolic" = 120 mm[Hg]
//	          items → ELEMENT[at0005] "Diastolic" = 80 mm[Hg]
func vitalSigns() *rm.Composition {
	tree := &rm.ItemTree{
		ArchetypeNodeID: "at0003",
		Items: []rm.Item{
			&rm.Element{
				ArchetypeNodeID: "at0004",
				Name:            rm.DVText{Value: "Systolic"},
				Value:           rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"},
			},
			&rm.Element{
				ArchetypeNodeID: "at0005",
				Name:            rm.DVText{Value: "Diastolic"},
				Value:           rm.DVQuantity{Magnitude: 80, Units: "mm[Hg]"},
			},
		},
	}
	obs := &rm.Observation{
		ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1",
		Name:            rm.DVText{Value: "Blood pressure"},
		Data: rm.History[rm.ItemStructure]{
			ArchetypeNodeID: "at0001",
			Events: []rm.Event{
				&rm.PointEvent[rm.ItemStructure]{
					ArchetypeNodeID: "at0006",
					Name:            rm.DVText{Value: "Any event"},
					Data:            tree,
				},
			},
		},
	}
	return &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            rm.DVText{Value: "Vital Signs"},
		Content:         []rm.ContentItem{obs},
	}
}

const (
	bpEvent   = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]"
	systolic  = bpEvent + "/data/items[at0004]/value"
	itemsPath = bpEvent + "/data/items"
)

func TestItemAtPathUnique(t *testing.T) {
	comp := vitalSigns()
	got, err := rmpath.ItemAtPath(comp, systolic)
	if err != nil {
		t.Fatalf("ItemAtPath(systolic) = %v", err)
	}
	q, ok := got.(rm.DVQuantity)
	if !ok {
		t.Fatalf("ItemAtPath(systolic) = %T, want rm.DVQuantity", got)
	}
	if q.Magnitude != 120 || q.Units != "mm[Hg]" {
		t.Errorf("systolic = %v %s, want 120 mm[Hg]", q.Magnitude, q.Units)
	}
}

func TestItemAtPathNamePredicate(t *testing.T) {
	comp := vitalSigns()
	got, err := rmpath.ItemAtPath(comp, bpEvent+"/data/items[at0004,'Systolic']/value")
	if err != nil {
		t.Fatalf("ItemAtPath(name predicate) = %v", err)
	}
	if q, ok := got.(rm.DVQuantity); !ok || q.Magnitude != 120 {
		t.Errorf("name-predicate match = %v (%T)", got, got)
	}
}

func TestItemsAtPathNonUnique(t *testing.T) {
	comp := vitalSigns()
	items, err := rmpath.ItemsAtPath(comp, itemsPath)
	if err != nil {
		t.Fatalf("ItemsAtPath(items) = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ItemsAtPath(items) returned %d, want 2", len(items))
	}
}

func TestPathExistsAbsent(t *testing.T) {
	comp := vitalSigns()
	if rmpath.PathExists(comp, bpEvent+"/data/items[at9999]/value") {
		t.Error("PathExists(absent) = true, want false")
	}
	if !rmpath.PathExists(comp, systolic) {
		t.Error("PathExists(systolic) = false, want true")
	}
}

func TestPathUniqueAndAmbiguous(t *testing.T) {
	comp := vitalSigns()
	if !rmpath.PathUnique(comp, systolic) {
		t.Error("PathUnique(systolic) = false, want true")
	}
	if rmpath.PathUnique(comp, itemsPath) {
		t.Error("PathUnique(items) = true, want false (2 items)")
	}
	// A multi-match path errors with ErrPathAmbiguous.
	if _, err := rmpath.ItemAtPath(comp, itemsPath); !errors.Is(err, rmpath.ErrPathAmbiguous) {
		t.Errorf("ItemAtPath(items) err = %v, want ErrPathAmbiguous", err)
	}
}

func TestItemAtPathNotFound(t *testing.T) {
	comp := vitalSigns()
	if _, err := rmpath.ItemAtPath(comp, bpEvent+"/data/items[at9999]/value"); !errors.Is(err, rmpath.ErrPathNotFound) {
		t.Errorf("err = %v, want ErrPathNotFound", err)
	}
}

func TestPathSyntaxError(t *testing.T) {
	comp := vitalSigns()
	for _, bad := range []string{"/content[at0001", "/content]bad", "//double"} {
		if _, err := rmpath.ItemsAtPath(comp, bad); !errors.Is(err, rmpath.ErrPathSyntax) {
			t.Errorf("ItemsAtPath(%q) err = %v, want ErrPathSyntax", bad, err)
		}
	}
}

// reportComposition exercises spine types beyond the vital-signs chain:
//
//	COMPOSITION → SECTION → EVALUATION → ITEM_LIST → ELEMENT
func reportComposition() *rm.Composition {
	list := &rm.ItemList{
		ArchetypeNodeID: "at0010",
		Items: []rm.Element{
			{ArchetypeNodeID: "at0011", Name: rm.DVText{Value: "Field A"}, Value: rm.DVText{Value: "alpha"}},
			{ArchetypeNodeID: "at0012", Name: rm.DVText{Value: "Field B"}, Value: rm.DVText{Value: "beta"}},
		},
	}
	eval := &rm.Evaluation{
		ArchetypeNodeID: "openEHR-EHR-EVALUATION.problem.v1",
		Name:            rm.DVText{Value: "Problem"},
		Data:            list,
	}
	section := &rm.Section{
		ArchetypeNodeID: "openEHR-EHR-SECTION.adhoc.v1",
		Name:            rm.DVText{Value: "Findings"},
		Items:           []rm.ContentItem{eval},
	}
	return &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1",
		Name:            rm.DVText{Value: "Report"},
		Content:         []rm.ContentItem{section},
	}
}

func TestSectionEvaluationItemListPath(t *testing.T) {
	comp := reportComposition()
	const base = "/content[openEHR-EHR-SECTION.adhoc.v1]/items[openEHR-EHR-EVALUATION.problem.v1]/data/items"
	got, err := rmpath.ItemAtPath(comp, base+"[at0011]/value")
	if err != nil {
		t.Fatalf("ItemAtPath = %v", err)
	}
	if dv, ok := got.(rm.DVText); !ok || dv.Value != "alpha" {
		t.Errorf("at0011 value = %v (%T), want DVText alpha", got, got)
	}
}

func TestPredicateForms(t *testing.T) {
	comp := reportComposition()
	const base = "/content[openEHR-EHR-SECTION.adhoc.v1]/items[openEHR-EHR-EVALUATION.problem.v1]/data/items"
	cases := map[string]string{
		"node id":             base + "[at0012]/value",
		"name only quoted":    base + "['Field B']/value",
		"aql name/value":      base + "[name/value='Field B']/value",
		"node and name/value": base + "[at0012 and name/value='Field B']/value",
		"node, name comma":    base + "[at0012,'Field B']/value",
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := rmpath.ItemAtPath(comp, p)
			if err != nil {
				t.Fatalf("ItemAtPath(%q) = %v", p, err)
			}
			if dv, ok := got.(rm.DVText); !ok || dv.Value != "beta" {
				t.Errorf("= %v (%T), want DVText beta", got, got)
			}
		})
	}
}

// TestWalkerTypedNilNoPanic guards the no-panic contract: a typed-nil
// pointer or a genuine nil inside a container must not crash the walker.
func TestWalkerTypedNilNoPanic(t *testing.T) {
	comp := &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.x.v1",
		Content:         []rm.ContentItem{(*rm.Observation)(nil), nil},
	}
	if rmpath.PathExists(comp, "/content/data") {
		t.Error("expected no resolution through nil content entries")
	}
	if _, err := rmpath.ItemsAtPath(comp, "/content[at0001]/data/items/value"); err != nil {
		t.Errorf("ItemsAtPath over nil content = %v, want nil error", err)
	}
}

func TestEmptyPathIsRoot(t *testing.T) {
	comp := vitalSigns()
	got, err := rmpath.ItemAtPath(comp, "/")
	if err != nil {
		t.Fatalf("ItemAtPath(/) = %v", err)
	}
	if got != rm.Locatable(comp) {
		t.Errorf("ItemAtPath(/) did not return the root")
	}
}

// spineComposition exercises the spine types not covered by the
// vital-signs / report fixtures: INSTRUCTION→ACTIVITY, ACTION, ADMIN_ENTRY
// →ITEM_TABLE→CLUSTER, GENERIC_ENTRY→ITEM_SINGLE, and INTERVAL_EVENT.
func spineComposition() *rm.Composition {
	leaf := func(node, name, val string) rm.Element {
		return rm.Element{ArchetypeNodeID: node, Name: rm.DVText{Value: name}, Value: rm.DVText{Value: val}}
	}
	instr := &rm.Instruction{
		ArchetypeNodeID: "openEHR-EHR-INSTRUCTION.order.v1",
		Name:            rm.DVText{Value: "Order"},
		Activities: []rm.Activity{{
			ArchetypeNodeID: "at0100",
			Name:            rm.DVText{Value: "Act"},
			// ITEM_SINGLE (an ITEM_STRUCTURE) under the activity.
			Description: &rm.ItemSingle{ArchetypeNodeID: "at0101", Item: leaf("at0102", "Dose", "5mg")},
		}},
	}
	action := &rm.Action{
		ArchetypeNodeID: "openEHR-EHR-ACTION.proc.v1",
		Name:            rm.DVText{Value: "Action"},
		Description:     &rm.ItemTree{ArchetypeNodeID: "at0200", Items: []rm.Item{leaf("at0201", "Step", "done")}},
	}
	admin := &rm.AdminEntry{
		ArchetypeNodeID: "openEHR-EHR-ADMIN_ENTRY.appt.v1",
		Name:            rm.DVText{Value: "Admin"},
		Data: &rm.ItemTable{ArchetypeNodeID: "at0300", Rows: []rm.Cluster{{
			ArchetypeNodeID: "at0301", Items: []rm.Item{leaf("at0302", "Cell", "x")},
		}}},
	}
	generic := &rm.GenericEntry{
		ArchetypeNodeID: "openEHR-EHR-GENERIC_ENTRY.g.v1",
		Name:            rm.DVText{Value: "Generic"},
		// GENERIC_ENTRY.data is an ITEM (here an ELEMENT), not an ITEM_STRUCTURE.
		Data: leaf("at0401", "Only", "v"),
	}
	obs := &rm.Observation{
		ArchetypeNodeID: "openEHR-EHR-OBSERVATION.iv.v1",
		Name:            rm.DVText{Value: "IV obs"},
		Data: rm.History[rm.ItemStructure]{
			ArchetypeNodeID: "at0500",
			Events: []rm.Event{&rm.IntervalEvent[rm.ItemStructure]{
				ArchetypeNodeID: "at0501",
				Name:            rm.DVText{Value: "Interval"},
				Data:            &rm.ItemTree{ArchetypeNodeID: "at0502", Items: []rm.Item{leaf("at0503", "IVval", "iv")}},
			}},
		},
	}
	return &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.spine.v1",
		Name:            rm.DVText{Value: "Spine"},
		Content:         []rm.ContentItem{instr, action, admin, generic, obs},
	}
}

func TestSpineTypeCoverage(t *testing.T) {
	comp := spineComposition()
	cases := map[string]struct{ path, want string }{
		"instruction/activity/itemsingle": {"/content[openEHR-EHR-INSTRUCTION.order.v1]/activities[at0100]/description/item/value", "5mg"},
		"action/itemtree":                 {"/content[openEHR-EHR-ACTION.proc.v1]/description/items[at0201]/value", "done"},
		"adminentry/itemtable/cluster":    {"/content[openEHR-EHR-ADMIN_ENTRY.appt.v1]/data/rows[at0301]/items[at0302]/value", "x"},
		"genericentry/element":            {"/content[openEHR-EHR-GENERIC_ENTRY.g.v1]/data/value", "v"},
		"observation/intervalevent":       {"/content[openEHR-EHR-OBSERVATION.iv.v1]/data/events[at0501]/data/items[at0503]/value", "iv"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := rmpath.ItemAtPath(comp, c.path)
			if err != nil {
				t.Fatalf("ItemAtPath(%q) = %v", c.path, err)
			}
			if dv, ok := got.(rm.DVText); !ok || dv.Value != c.want {
				t.Errorf("= %v (%T), want DVText %q", got, got, c.want)
			}
		})
	}
}

func TestMalformedPathBooleansAndItemAt(t *testing.T) {
	comp := vitalSigns()
	const bad = "/content[at0001" // unterminated predicate
	if rmpath.PathExists(comp, bad) {
		t.Error("PathExists(malformed) = true, want false")
	}
	if rmpath.PathUnique(comp, bad) {
		t.Error("PathUnique(malformed) = true, want false")
	}
	if _, err := rmpath.ItemAtPath(comp, bad); !errors.Is(err, rmpath.ErrPathSyntax) {
		t.Errorf("ItemAtPath(malformed) err = %v, want ErrPathSyntax", err)
	}
}

func TestTypedNilRootNoPanic(t *testing.T) {
	var c *rm.Composition // typed-nil root
	if _, err := rmpath.ItemAtPath(c, "/content"); !errors.Is(err, rmpath.ErrPathNotFound) {
		t.Errorf("ItemAtPath(typed-nil root) err = %v, want ErrPathNotFound", err)
	}
	if rmpath.PathExists(c, "/content/data") {
		t.Error("PathExists(typed-nil root) = true, want false")
	}
}
