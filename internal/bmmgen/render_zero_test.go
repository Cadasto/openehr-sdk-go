package bmmgen

import (
	"context"
	"go/ast"
	"go/types"
	"maps"
	"slices"
	"strings"
	"testing"
)

// TestEmptyMembers_REQ052 pins the members the canonical encoders leave out
// when empty (wire.md § REQ-052 An absent mandatory member): a mandatory
// member held by value as a struct, directly or through the class's own type
// parameter, and nothing else. The two struct members a fixed reading decides
// are written whatever they hold, and a member held through an interface, a
// number and a string are never left out.
func TestEmptyMembers_REQ052(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, testResolver())
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	g, err := newRMAccessGen(plan)
	if err != nil {
		t.Fatal(err)
	}
	members := func(class string) map[string]emptyMember {
		t.Helper()
		pc, ok := plan.Classes[class]
		if !ok {
			t.Fatalf("%s not in the RM plan", class)
		}
		m, err := g.emptyMembers(pc)
		if err != nil {
			t.Fatalf("emptyMembers(%s): %v", class, err)
		}
		return m
	}
	for _, tc := range []struct {
		class, member, test string
	}{
		{"ROLE", "performer", "isZeroPartyRef"},
		{"PARTY_RELATIONSHIP", "source", "isZeroPartyRef"},
		{"COMPOSITION", "category", "isZeroDVCodedText"},
		{"COMPOSITION", "language", "isZeroCodePhrase"},
		{"COMPOSITION", "territory", "isZeroCodePhrase"},
		{"OBSERVATION", "data", "isZeroHistoryItemStructure"},
		{"CODE_PHRASE", "terminology_id", "isZeroTerminologyID"},
	} {
		if got, ok := members(tc.class)[tc.member]; !ok || got.param || got.test != tc.test {
			t.Errorf("%s.%s = %+v, %v; want a struct member read by %s", tc.class, tc.member, got, ok, tc.test)
		}
	}
	for _, class := range []string{"POINT_EVENT", "INTERVAL_EVENT"} {
		got, ok := members(class)["data"]
		var held []string
		for _, h := range got.held {
			held = append(held, h.name)
		}
		if want := []string{"ItemList", "ItemSingle", "ItemTable", "ItemTree"}; !ok || !got.param || !slices.Equal(held, want) {
			t.Errorf("%s.data = %+v, %v (held %q); want a type-parameter member holding %q", class, got, ok, held, want)
		}
	}
	for _, tc := range []struct{ class, member, why string }{
		{"EHR_STATUS", "subject", "a fixed reading: always present"},
		{"ARCHETYPED", "archetype_id", "a fixed reading: always present"},
		{"ELEMENT", "name", "held through an interface"},
		{"ELEMENT", "archetype_node_id", "a string"},
		{"DV_QUANTITY", "magnitude", "a number"},
		{"COMPOSITION", "composer", "held through an interface"},
		{"PARTY_IDENTITY", "details", "held through an interface"},
	} {
		if got, ok := members(tc.class)[tc.member]; ok {
			t.Errorf("%s.%s = %+v; want it written whatever it holds (%s)", tc.class, tc.member, got, tc.why)
		}
	}

	aom, err := BuildPlanForTarget(context.Background(), TargetAOM14, testResolver())
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	ag, err := newRMAccessGen(aom)
	if err != nil {
		t.Fatal(err)
	}
	for _, pc := range aom.ConcreteClasses {
		if pc.External {
			continue
		}
		if m, err := ag.emptyMembers(pc); err != nil || len(m) > 0 {
			t.Errorf("AOM 1.4 %s: emptyMembers = %v, %v; want none, as no floor reads an AOM class", pc.BMMName, slices.Sorted(maps.Keys(m)), err)
		}
	}
}

// TestZeroFileRegistersTheAccessLookups pins the seam between the zero file
// and the access (ADR 0002 D12, REQ-171 Structural access): the zero file's
// init registers exactly the zero tests the access looks up, and declares
// each one. A test the access names and the zero file does not register would
// fail when internal/rmaccess is initialised.
func TestZeroFileRegistersTheAccessLookups(t *testing.T) {
	_, _, access := rmAccessOutput(t)
	zero := rmZeroOutput(t)

	var lookups []string
	for _, d := range access.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Values) != 1 {
				continue
			}
			if strings.HasPrefix(types.ExprString(vs.Values[0]), "registeredZeroTest[") {
				lookups = append(lookups, vs.Names[0].Name)
			}
		}
	}
	slices.Sort(lookups)

	declared := map[string]bool{}
	var registered []string
	for _, d := range zero.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		declared[fd.Name.Name] = true
		if fd.Name.Name != "init" {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || types.ExprString(call.Fun) != "rmzero.Register" {
				return true
			}
			for _, a := range call.Args {
				registered = append(registered, types.ExprString(a))
			}
			return false
		})
	}
	slices.Sort(registered)

	if len(lookups) < 10 {
		t.Fatalf("the access looks up %d zero tests, want one per class it reads by value", len(lookups))
	}
	if !slices.Equal(registered, lookups) {
		t.Errorf("the zero file registers %q, want the tests the access looks up, %q", registered, lookups)
	}
	for _, name := range registered {
		if !declared[name] {
			t.Errorf("the zero file registers %s and does not declare it", name)
		}
	}
}
