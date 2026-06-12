package bmmgen

import (
	"encoding/json"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// mustSimpleClass builds a *bmm.SimpleClass from a compact JSON fragment.
// Panics on decode failure — tests only.
func mustSimpleClass(t *testing.T, raw string) *bmm.SimpleClass {
	t.Helper()
	sc := &bmm.SimpleClass{}
	if err := json.Unmarshal([]byte(raw), sc); err != nil {
		t.Fatalf("mustSimpleClass: %v", err)
	}
	return sc
}

// TestEffectivePropertiesCyclicAncestors verifies that effectiveProperties
// terminates when two classes list each other as ancestors (a cycle that
// would cause infinite recursion without the visitedClass guard).
//
// Without the guard the visit closure recurses A→B→A→B… until the
// goroutine stack overflows. With the guard it returns immediately on
// the second visit to a class.
func TestEffectivePropertiesCyclicAncestors(t *testing.T) {
	// Build a minimal *Plan whose Classes map has:
	//   A — ancestors: ["B"]
	//   B — ancestors: ["A"]
	// Both have no properties (we only care that the function returns,
	// not what it returns).
	classA := mustSimpleClass(t, `{"name":"A","ancestors":["B"]}`)
	classB := mustSimpleClass(t, `{"name":"B","ancestors":["A"]}`)

	plan := &Plan{
		Target: TargetRM,
		Classes: map[string]*PlannedClass{
			"A": {BMMName: "A", GoName: "A", Class: classA},
			"B": {BMMName: "B", GoName: "B", Class: classB},
		},
		AbstractDescendants: map[string][]string{},
		ConcreteSubtypes:    map[string][]string{},
		CyclicSingleProps:   map[string]map[string]bool{},
	}

	// If the cycle guard is missing, this call will stack-overflow-panic.
	// The test completing (without panic or timeout) is the assertion.
	attrs, order := effectiveProperties(plan, "A")
	_ = attrs
	_ = order
}
