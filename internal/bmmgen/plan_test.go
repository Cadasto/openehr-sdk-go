package bmmgen

import (
	"context"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

const testResources = "../../" + bmm.DefaultResourcesDir

// TestPlanFileAssignments asserts that key classes land in the
// expected `<base>_gen.go` file. The set is deliberately small and
// load-bearing: if a refactor accidentally re-buckets DV_QUANTITY,
// the test fails.
//
// REQ-043: § Mapping rules, Schema → Go package set. The RM target emits one
// Go file per BMM package, so each class lands in the file of the package
// that declares it.
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

// TestPlanAbstractDescendants asserts the marker-method closure: the
// concrete descendants the plan lists for DATA_VALUE and for DV_ORDERED
// are exactly the owned concrete classes whose ancestor chain reaches
// that class. The expectation walks the ancestors upward, so it does not
// share the plan's downward walk.
//
// REQ-043: § Mapping rules, Class → Go type. An abstract class becomes a Go
// interface whose marker method every concrete descendant carries; this pins
// the descendant set the renderer emits those marker methods from.
func TestPlanAbstractDescendants(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	for _, tc := range []struct {
		root    string
		has     string // a descendant several levels down
		hasNot  string // a descendant of the other root only
		minSize int
	}{
		{root: "DATA_VALUE", has: "DV_QUANTITY", hasNot: "", minSize: 5},
		{root: "DV_ORDERED", has: "DV_QUANTITY", hasNot: "DV_TEXT", minSize: 5},
	} {
		want := concreteDescendantsByAncestry(plan, tc.root)
		got := plan.AbstractDescendants[tc.root]
		if !slices.Equal(got, want) {
			t.Errorf("AbstractDescendants[%s] = %v, want %v", tc.root, got, want)
		}
		// Vacuity guards: an empty or truncated walk on both sides would
		// otherwise compare equal.
		if len(want) < tc.minSize {
			t.Errorf("%s: %d concrete descendants by ancestry, want at least %d", tc.root, len(want), tc.minSize)
		}
		if !slices.Contains(want, tc.has) {
			t.Errorf("%s: concrete descendants by ancestry %v miss %s", tc.root, want, tc.has)
		}
		if tc.hasNot != "" && slices.Contains(want, tc.hasNot) {
			t.Errorf("%s: concrete descendants by ancestry %v include %s", tc.root, want, tc.hasNot)
		}
	}
}

// concreteDescendantsByAncestry returns, sorted, every owned concrete
// SimpleClass in the plan whose ancestor chain reaches root.
func concreteDescendantsByAncestry(plan *Plan, root string) []string {
	var out []string
	for name, pc := range plan.Classes {
		if pc.External || pc.Class.IsAbstract() {
			continue
		}
		if _, isSimple := pc.Class.(*bmm.SimpleClass); !isSimple {
			continue
		}
		seen := map[string]bool{}
		stack := slices.Clone(pc.Class.Ancestors())
		for len(stack) > 0 {
			anc := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[anc] {
				continue
			}
			seen[anc] = true
			if anc == root {
				out = append(out, name)
				break
			}
			if apc, ok := plan.Classes[anc]; ok {
				stack = append(stack, apc.Class.Ancestors()...)
			}
		}
	}
	slices.Sort(out)
	return out
}
