package validation

// engine_primitive_leaf_internal_test.go: REQ-171 § One walk and § Surface —
// a primitive leaf, the value of a Go primitive type that an attribute no
// template clause constrains holds, counts as one visit, as every other
// value does, and the visit limit reached at such a leaf is reported at the
// leaf's own path.

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// elementHolding returns an ELEMENT named "e" that holds value.
func elementHolding(value rm.DataValue) *rm.Element {
	return &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "e"}, Value: value}
}

// REQ-171 § Surface — the one walk visits the same number of values on each
// composition of the benchmarks, with its template and with the RM rules
// alone, as it did before primitive leaves were counted without being
// entered: more than half of those values are primitive leaves, so a leaf
// left uncounted, or counted twice, changes the number.
func TestEngine_REQ171_VisitCountOfTheBenchmarkCompositions(t *testing.T) {
	want := map[string]int{"BMI": 403, syntheticID: 1018, syntheticName: 9858}
	for _, bc := range benchCases(t) {
		for _, mode := range []struct {
			name string
			c    *templatecompile.Compiled
		}{{name: "rm"}, {name: "template", c: bc.c}} {
			t.Run(bc.name+"/"+mode.name, func(t *testing.T) {
				_, e := engineWalk(bc.comp, mode.c, Limits{})
				if e == nil {
					t.Fatalf("engineWalk(%s, %s) refused the call, want a walk", bc.name, mode.name)
				}
				if e.stopped || e.visits != want[bc.name] {
					t.Errorf("engineWalk(%s, %s) visits %d values (stopped: %t), want %d and not stopped", bc.name, mode.name, e.visits, e.stopped, want[bc.name])
				}
			})
		}
	}
}

// REQ-171 § Surface, REQ-102 — under a node limit of nodes, where the walk's
// next visit is a primitive leaf, the walk reports max_nodes at that leaf's
// path, naming the type its attribute declares and the limit, as its last
// and only stop, and lists that path alone in Coverage.Stopped. The leaves
// are of Go primitive types (String, Boolean, Integer64) and of named RM
// types (Real, Integer), at an attribute of the root and below, with no
// template and with one. Each case's limit and path were read off the walk
// before primitive leaves were counted without being entered.
func TestEngine_REQ171_NodeLimitAtAPrimitiveLeaf(t *testing.T) {
	bmi := benchCases(t)[0]
	precision := rm.Integer(1)
	quantity := elementHolding(&rm.DVQuantity{Magnitude: 1.5, Units: "kg", Precision: &precision})
	text := elementHolding(&rm.DVText{Value: "x"})
	const bmiValue = "/content[openEHR-EHR-OBSERVATION.body_mass_index.v1]/data/events[at0002]/data/items[at0004]/value"
	tests := []struct {
		name   string
		root   any
		c      *templatecompile.Compiled
		nodes  int
		path   string
		rmType string
	}{
		{name: "String at an attribute of the root", root: text, nodes: 3, path: "/archetype_node_id", rmType: "String"},
		{name: "String in a DV_TEXT name", root: text, nodes: 2, path: "/name/value", rmType: "String"},
		{name: "String in a DV_TEXT value", root: text, nodes: 5, path: "/value/value", rmType: "String"},
		{name: "Boolean in a DV_BOOLEAN", root: elementHolding(&rm.DVBoolean{Value: true}), nodes: 5, path: "/value/value", rmType: "Boolean"},
		{name: "Integer64 in a DV_COUNT", root: elementHolding(&rm.DVCount{Magnitude: 3}), nodes: 5, path: "/value/magnitude", rmType: "Integer64"},
		{name: "Real in a DV_QUANTITY", root: quantity, nodes: 5, path: "/value/magnitude", rmType: "Real"},
		{name: "Integer in a DV_QUANTITY", root: quantity, nodes: 6, path: "/value/precision", rmType: "Integer"},
		{name: "String in a DV_QUANTITY", root: quantity, nodes: 7, path: "/value/units", rmType: "String"},
		{name: "String in BMI, BMI template", root: bmi.comp, c: bmi.c, nodes: 4, path: "/category/defining_code/terminology_id/value", rmType: "String"},
		{name: "Real in BMI, BMI template", root: bmi.comp, c: bmi.c, nodes: 44, path: bmiValue + "/magnitude", rmType: "Real"},
		{name: "String units in BMI, BMI template", root: bmi.comp, c: bmi.c, nodes: 46, path: bmiValue + "/units", rmType: "String"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, e := engineWalk(tc.root, tc.c, Limits{Nodes: tc.nodes})
			if e == nil {
				t.Fatalf("engineWalk under Nodes %d refused the call, want a walk", tc.nodes)
			}
			if !e.stopped || e.visits != tc.nodes {
				t.Errorf("under Nodes %d the walk visits %d values (stopped: %t), want %d and stopped", tc.nodes, e.visits, e.stopped, tc.nodes)
			}
			if len(r.Issues) == 0 {
				t.Fatalf("under Nodes %d the walk reports no issue, want max_nodes at %q", tc.nodes, tc.path)
			}
			last := r.Issues[len(r.Issues)-1]
			detail := fmt.Sprintf("the walk stopped at %s after visiting %d values, its limit per call", tc.rmType, tc.nodes)
			if last.Code != "max_nodes" || last.Path != tc.path || last.Detail != detail {
				t.Errorf("under Nodes %d the last issue is %s at %q (%q), want max_nodes at %q (%q)", tc.nodes, last.Code, last.Path, last.Detail, tc.path, detail)
			}
			stops := 0
			for _, iss := range r.Issues {
				if iss.Code == "max_nodes" {
					stops++
				}
			}
			if stops != 1 {
				t.Errorf("under Nodes %d the walk reports max_nodes %d times, want once", tc.nodes, stops)
			}
			if !slices.Equal(r.Coverage.Stopped, []string{tc.path}) {
				t.Errorf("under Nodes %d Coverage.Stopped = %q, want [%q]", tc.nodes, r.Coverage.Stopped, tc.path)
			}
		})
	}
}

// REQ-171 § One walk — a value of a Go primitive type is counted without
// being entered only where the type its attribute declares has no RM rules.
// Read under a declared type that has them, it is visited as that type, whose
// class is then among the visited, so the invariants that class leaves
// unevaluated are in Coverage.Unevaluated. No attribute of the pinned RM
// declares such a type for a primitive value, so the attribute is built, as
// a class's table builds it ([walkedAttr]): the value of a DV_TEXT, declared
// a DV_TEXT.
func TestEngine_REQ171_PrimitiveValueUnderATypeWithRulesIsVisited(t *testing.T) {
	rules, _ := rulesFor("DV_TEXT")
	if rules == nil {
		t.Fatal("precondition: DV_TEXT has no RM rules")
	}
	want := unevaluatedIn(indexSet(nil).with(rules.index))
	if len(want) == 0 {
		t.Fatal("precondition: DV_TEXT leaves no invariant unevaluated, so Coverage cannot tell whether its rules ran")
	}
	e := newEngine(Limits{})
	e.visitAttribute(&engineNode{value: &rm.DVText{Value: "x"}, rmType: "DV_TEXT", path: "/"}, "DV_TEXT",
		new(walkedAttr(attributeFacts{Name: "value", RMType: "DV_TEXT", Typed: true}, nil)))
	r := e.result()
	if e.visits != 1 || len(r.Issues) != 0 {
		t.Errorf("visit of DV_TEXT value declared DV_TEXT: %d visits, issues %v; want 1 visit and no issue", e.visits, r.Issues)
	}
	if !slices.Equal(r.Coverage.Unevaluated, want) {
		t.Errorf("visit of DV_TEXT value declared DV_TEXT: Coverage.Unevaluated = %q, want %q: the declared type's rules run on the value", r.Coverage.Unevaluated, want)
	}
}

// REQ-171 § Surface — once the visit limit has ended the walk, a primitive
// leaf is neither counted nor reported, as no other value is: the walk
// reports max_nodes once.
func TestEngine_REQ171_PrimitiveLeafAfterTheWalkEnded(t *testing.T) {
	_, e := engineWalk(elementHolding(&rm.DVText{Value: "x"}), nil, Limits{Nodes: 1})
	if e == nil || !e.stopped {
		t.Fatal("precondition: the walk under Nodes 1 did not stop")
	}
	visits, issues := e.visits, len(e.found.issues)
	e.visitAttribute(&engineNode{value: &rm.DVText{Value: "y"}, rmType: "DV_TEXT", path: "/"}, "DV_TEXT",
		new(walkedAttr(attributeFacts{Name: "value", RMType: "String", Typed: true}, nil)))
	if e.visits != visits || len(e.found.issues) != issues {
		t.Errorf("visit of a DV_TEXT value after the walk ended: %d visits and issues %v, want %d visits and %d issues", e.visits, e.found.issues, visits, issues)
	}
}
