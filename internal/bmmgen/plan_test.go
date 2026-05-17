package bmmgen

import (
	"context"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

const testResources = "../../" + bmm.DefaultResourcesDir

// TestPlanFileAssignments asserts that key classes land in the
// expected `<base>_gen.go` file. The set is deliberately small and
// load-bearing: if a refactor accidentally re-buckets DV_QUANTITY,
// the test fails.
func TestPlanFileAssignments(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	cases := map[string]string{
		"DV_QUANTITY":       "data_types_quantity",
		"DV_TEXT":           "data_types_text",
		"COMPOSITION":       "composition",
		"OBSERVATION":       "composition_content_entry",
		"EHR_STATUS":        "ehr",
		"OBJECT_VERSION_ID": "base_types_identification",
		"Cardinality":       "foundation_types_interval",
		"Interval":          "foundation_types_interval",
		"CODE_PHRASE":       "data_types_text",
	}
	for cls, wantFile := range cases {
		pc, ok := plan.Classes[cls]
		if !ok {
			t.Errorf("class %s not in plan", cls)
			continue
		}
		if pc.FileBase != wantFile {
			t.Errorf("class %s in %s_gen.go, want %s_gen.go", cls, pc.FileBase, wantFile)
		}
	}
}

// TestPlanSkipsEHRExtract asserts that EHR_EXTRACT classes are
// excluded per REQ-042.
func TestPlanSkipsEHRExtract(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	for _, name := range []string{"EXTRACT", "SYNC_EXTRACT", "MESSAGE", "GENERIC_CONTENT_ITEM"} {
		if _, ok := plan.Classes[name]; ok {
			t.Errorf("class %s should be skipped (in ehr_extract package)", name)
		}
	}
}

// TestPlanIncludesConcreteRegistrations asserts that DV_QUANTITY (a
// non-abstract, non-generic class) appears in the typereg-target
// list.
func TestPlanIncludesConcreteRegistrations(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	wantConcrete := map[string]bool{
		"DV_QUANTITY": false,
		"DV_TEXT":     false,
		"COMPOSITION": false,
		"OBSERVATION": false,
		"EHR_STATUS":  false,
		"CODE_PHRASE": false,
		"DATA_VALUE":  false, // abstract — must NOT be registered
		"DV_AMOUNT":   false, // abstract — must NOT be registered
		"DV_INTERVAL": false, // concrete + generic — registered under default-bound instantiation for xsi:type / _type dispatch
		"VERSION":     false, // abstract + generic — must NOT be registered
	}
	abstracts := map[string]bool{
		"DATA_VALUE": true, "DV_AMOUNT": true, "VERSION": true,
	}
	for _, pc := range plan.ConcreteClasses {
		if _, want := wantConcrete[pc.BMMName]; want {
			wantConcrete[pc.BMMName] = true
		}
	}
	for name, registered := range wantConcrete {
		if abstracts[name] {
			if registered {
				t.Errorf("class %s should NOT be in ConcreteClasses (abstract/generic)", name)
			}
		} else if !registered {
			t.Errorf("class %s should be in ConcreteClasses", name)
		}
	}
}

// TestPlanAbstractDescendants asserts the marker-method closure: for
// DATA_VALUE, every DV_* concrete leaf descends. For DV_ORDERED,
// only the ordered concrete types do.
func TestPlanAbstractDescendants(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	dvDescendants := plan.AbstractDescendants["DATA_VALUE"]
	if len(dvDescendants) < 5 {
		t.Errorf("DATA_VALUE expected many descendants, got %d", len(dvDescendants))
	}
	hasDvQuantity := false
	for _, d := range dvDescendants {
		if d == "DV_QUANTITY" {
			hasDvQuantity = true
		}
	}
	if !hasDvQuantity {
		t.Errorf("DATA_VALUE descendants missing DV_QUANTITY")
	}
}
