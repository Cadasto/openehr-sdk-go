package bmmgen

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// markerOnlyInterface reports whether src declares `type goName interface`
// whose body is exactly the one unexported marker method is<goName>().
func markerOnlyInterface(src, goName string) bool {
	re := regexp.MustCompile(`(?m)^type ` + goName + ` interface \{\s*is` + goName + `\(\)\s*\}`)
	return re.MatchString(src)
}

// TestInterfaceClassesMarkerOnlyRM covers the REQ-043 § Mapping rules,
// Class → Go type rule for P_BMM_INTERFACE (ADR 0002 D4) on the pinned RM.
// CODE_SET_ACCESS and TERMINOLOGY_ACCESS render as marker-only interfaces,
// and no method stub naming either class appears anywhere in the RM output.
//
// The BMM declares four functions on CODE_SET_ACCESS and six on
// TERMINOLOGY_ACCESS; the generator emits none of them today.
func TestInterfaceClassesMarkerOnlyRM(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	wantFunctions := map[string]int{"CODE_SET_ACCESS": 4, "TERMINOLOGY_ACCESS": 6}
	goNames := map[string]string{"CODE_SET_ACCESS": "CodeSetAccess", "TERMINOLOGY_ACCESS": "TerminologyAccess"}
	// Vacuity guard: the classes must be P_BMM_INTERFACE and must carry
	// functions, otherwise "no stub" proves nothing.
	for bmmName, n := range wantFunctions {
		pc, ok := plan.Classes[bmmName]
		if !ok {
			t.Fatalf("%s not in the RM plan", bmmName)
		}
		ifc, ok := pc.Class.(*bmm.Interface)
		if !ok {
			t.Fatalf("%s is %T, want *bmm.Interface", bmmName, pc.Class)
		}
		if got := len(ifc.Functions); got != n {
			t.Fatalf("%s declares %d functions in the pinned BMM, want %d", bmmName, got, n)
		}
		if pc.GoName != goNames[bmmName] {
			t.Fatalf("%s Go name = %q, want %q", bmmName, pc.GoName, goNames[bmmName])
		}
	}

	var all strings.Builder
	rendered := map[string]bool{}
	for _, f := range plan.Files {
		out, err := RenderFile(plan, f)
		if err != nil {
			t.Fatalf("RenderFile(%s): %v", f.FileBase, err)
		}
		src := string(out)
		all.WriteString(src)
		for bmmName, goName := range goNames {
			if !strings.Contains(src, "type "+goName+" interface") {
				continue
			}
			rendered[bmmName] = true
			if !markerOnlyInterface(src, goName) {
				t.Errorf("%s: %s is not exactly `interface { is%s() }`", f.FileBase, goName, goName)
			}
		}
	}
	for bmmName := range wantFunctions {
		if !rendered[bmmName] {
			t.Errorf("no rendered file declares %s", goNames[bmmName])
		}
		if strings.Contains(all.String(), bmmName+".") {
			t.Errorf("RM output names %s.<function>, want no method stub for an interface class", bmmName)
		}
	}
}

// TestInterfaceWithoutDescendantMarkerOnlySynthetic pins the same rule on a
// hand-built schema (REQ-043 § Mapping rules, Class → Go type): a
// P_BMM_INTERFACE that declares a function and has no concrete descendant
// renders as a marker-only interface and emits no method stub.
func TestInterfaceWithoutDescendantMarkerOnlySynthetic(t *testing.T) {
	ifc := &bmm.Interface{}
	ifc.Name = "SVC_ACCESS"
	ifc.Functions = map[string]*bmm.Function{
		"ping": {Name: "ping", Result: &bmm.SimpleType{TypeName: "Boolean"}},
	}
	other := &bmm.SimpleClass{}
	other.Name = "UNRELATED"
	plan := &Plan{
		Target: TargetRM,
		Classes: map[string]*PlannedClass{
			"SVC_ACCESS": {BMMName: "SVC_ACCESS", GoName: "SvcAccess", Class: ifc},
			"UNRELATED":  {BMMName: "UNRELATED", GoName: "Unrelated", Class: other},
		},
		AbstractDescendants: map[string][]string{},
		ConcreteSubtypes:    map[string][]string{},
		CyclicSingleProps:   map[string]map[string]bool{},
	}
	computeAbstractDescendants(plan)
	file := &PlannedFile{
		FileBase: "synthetic",
		Classes:  []*PlannedClass{plan.Classes["SVC_ACCESS"], plan.Classes["UNRELATED"]},
	}

	out, err := RenderFile(plan, file)
	if err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	src := string(out)
	if !markerOnlyInterface(src, "SvcAccess") {
		t.Errorf("SvcAccess is not exactly `interface { isSvcAccess() }`:\n%s", src)
	}
	if strings.Contains(src, "SVC_ACCESS.ping") || strings.Contains(src, "Ping(") {
		t.Errorf("a method stub for SVC_ACCESS.ping was emitted:\n%s", src)
	}
	if plan.MethodStubsEmitted != 0 {
		t.Errorf("MethodStubsEmitted = %d, want 0 for an interface without descendants", plan.MethodStubsEmitted)
	}
}

// TestInterfaceDescendantGetsNoMarkerSynthetic pins the rest of ADR 0002 D4
// for a P_BMM_INTERFACE (REQ-043 § Mapping rules, Class → Go type): a
// concrete descendant of an interface receives neither the is<X>() marker
// nor a stub for the interface's functions, because no pinned BMM interface
// carries is_abstract (STRAND-12). Resolving STRAND-12 changes this test
// together with D4.
func TestInterfaceDescendantGetsNoMarkerSynthetic(t *testing.T) {
	ifc := &bmm.Interface{}
	ifc.Name = "SVC_ACCESS"
	ifc.Functions = map[string]*bmm.Function{
		"ping": {Name: "ping", Result: &bmm.SimpleType{TypeName: "Boolean"}},
	}
	impl := &bmm.SimpleClass{}
	impl.Name = "SVC_IMPL"
	impl.Ancestors_ = []string{"SVC_ACCESS"}
	plan := &Plan{
		Target: TargetRM,
		Classes: map[string]*PlannedClass{
			"SVC_ACCESS": {BMMName: "SVC_ACCESS", GoName: "SvcAccess", Class: ifc},
			"SVC_IMPL":   {BMMName: "SVC_IMPL", GoName: "SvcImpl", Class: impl},
		},
		AbstractDescendants: map[string][]string{},
		ConcreteSubtypes:    map[string][]string{},
		CyclicSingleProps:   map[string]map[string]bool{},
	}
	computeAbstractDescendants(plan)
	file := &PlannedFile{
		FileBase: "synthetic",
		Classes:  []*PlannedClass{plan.Classes["SVC_ACCESS"], plan.Classes["SVC_IMPL"]},
	}

	out, err := RenderFile(plan, file)
	if err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	src := string(out)
	if !markerOnlyInterface(src, "SvcAccess") {
		t.Errorf("SvcAccess is not exactly `interface { isSvcAccess() }`:\n%s", src)
	}
	if strings.Contains(src, ") isSvcAccess()") {
		t.Errorf("a descendant received the isSvcAccess() marker:\n%s", src)
	}
	if strings.Contains(src, "SVC_ACCESS.ping") || strings.Contains(src, "Ping(") {
		t.Errorf("a method stub for SVC_ACCESS.ping was emitted:\n%s", src)
	}
	if plan.MethodStubsEmitted != 0 {
		t.Errorf("MethodStubsEmitted = %d, want 0 for an interface's descendant", plan.MethodStubsEmitted)
	}
}
