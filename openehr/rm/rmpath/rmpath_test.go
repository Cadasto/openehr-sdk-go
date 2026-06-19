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
