package bmmgen

// REQ-049 — the generated absence table: every name the RM generation
// target's pinned schemas declare that the class universe omits, with the
// reason it is out, under the fixed precedence
// (primitive → enumeration → excluded class → excluded package).

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// mustEnumeration builds a named *bmm.Enumeration. The BMM kind is what the
// enumeration rule reads; the item list is irrelevant here.
func mustEnumeration(name string) *bmm.Enumeration {
	e := &bmm.Enumeration{}
	e.Name = name
	return e
}

// absencePlan builds a synthetic RM-target plan whose declarations cover every
// absence rule plus two belt-and-braces overlaps. It goes through the real
// planner, so what lands out of the universe here lands out of it in
// production for the same reasons.
func absencePlan(t *testing.T) *Plan {
	t.Helper()

	pkg := func(dotted string, classes ...string) *bmm.Package {
		return &bmm.Package{Name: dotted, Classes: classes}
	}
	schema := &bmm.Schema{
		RMPublisher: "openehr",
		SchemaName:  "rm",
		RMRelease:   "1.2.0",
		Packages: map[string]*bmm.Package{
			"rm_test":    pkg("org.openehr.rm.test", "KEEPER", "MY_ENUM", "TUPLE", "Comparable"),
			"functional": pkg("org.openehr.base.foundation_types.functional", "FUNCTION"),
			"extract":    pkg("org.openehr.rm.ehr_extract.common", "EXTRACT_THING"),
			"primitives": pkg("org.openehr.base.foundation_types.primitive_types", "String", "Ordered", "Interval"),
		},
		ClassDefinitions: map[string]bmm.Class{
			"KEEPER":        mustSimpleClass(t, `{"name":"KEEPER"}`),
			"MY_ENUM":       mustEnumeration("MY_ENUM"),
			"TUPLE":         mustSimpleClass(t, `{"name":"TUPLE"}`),
			"Comparable":    mustEnumeration("Comparable"),
			"FUNCTION":      mustSimpleClass(t, `{"name":"FUNCTION"}`),
			"EXTRACT_THING": mustSimpleClass(t, `{"name":"EXTRACT_THING"}`),
		},
		PrimitiveTypes: map[string]bmm.Class{
			"String":   mustSimpleClass(t, `{"name":"String"}`),
			"Ordered":  mustSimpleClass(t, `{"name":"Ordered"}`),
			"Interval": mustSimpleClass(t, `{"name":"Interval"}`),
		},
	}
	plan, err := PlanFromSchemaForTarget(schema, TargetRM)
	if err != nil {
		t.Fatalf("PlanFromSchemaForTarget: %v", err)
	}
	return plan
}

// TestRMInfoAbsenceReasonsUnderTheFixedPrecedence pins one name per rule and,
// for the two names two rules match, pins that the EARLIER rule wins: what a
// name IS (primitive, enumeration) ranks ahead of why it was SKIPPED (REQ-049).
func TestRMInfoAbsenceReasonsUnderTheFixedPrecedence(t *testing.T) {
	entries, err := rmInfoAbsence(absencePlan(t))
	if err != nil {
		t.Fatalf("rmInfoAbsence: %v", err)
	}
	got := make(map[string]absenceReason, len(entries))
	for _, e := range entries {
		got[e.className] = e.reason
	}

	want := map[string]absenceReason{
		// Declared in primitive_types and mapped to a Go primitive.
		"String": absencePrimitive,
		// Belt and braces: a primitive_types entry the named-class
		// exclusion set restates. Primitive outranks excluded class.
		"Ordered": absencePrimitive,
		// A primitive_types entry the target DOES emit a Go type for —
		// still a value the RM uses, not a class this surface models.
		"Interval": absencePrimitive,
		// The BMM enumeration kind.
		"MY_ENUM": absenceEnumeration,
		// Belt and braces the other way: an enumeration the named-class
		// exclusion set also lists. Enumeration outranks excluded class.
		"Comparable": absenceEnumeration,
		// Named-class exclusion, in a package no prefix skips — the rule
		// decides this one on its own.
		"TUPLE": absenceExcludedClass,
		// Named-class exclusion AND a skipped package. Class outranks
		// package.
		"FUNCTION": absenceExcludedClass,
		// Only the wholesale package skip accounts for this one.
		"EXTRACT_THING": absenceExcludedPackage,
	}
	for name, wantReason := range want {
		if got[name] != wantReason {
			t.Errorf("%s: absence reason %q, want %q", name, got[name], wantReason)
		}
	}
	// KEEPER is the one class in the universe: a universe member must never
	// appear in the table (REQ-049 § Generated, accounted, computed).
	if reason, ok := got["KEEPER"]; ok {
		t.Errorf("KEEPER is in the class universe but the table reports %q", reason)
	}
	if len(entries) != len(want) {
		t.Errorf("table has %d entries, want %d: %v", len(entries), len(want), got)
	}
}

// TestRMInfoAbsenceIsSortedByClassName — the rendered table's order is the
// class name's, so two runs on one input produce one byte sequence.
func TestRMInfoAbsenceIsSortedByClassName(t *testing.T) {
	entries, err := rmInfoAbsence(absencePlan(t))
	if err != nil {
		t.Fatalf("rmInfoAbsence: %v", err)
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].className >= entries[i].className {
			t.Errorf("entries not sorted: %q before %q", entries[i-1].className, entries[i].className)
		}
	}
}

// TestRMInfoAbsenceUnaccountedNameFailsGeneration — a declared name the
// universe omits that no rule accounts for is a generation ERROR naming the
// class, not a silent *undeclared* (REQ-049: that failure is what keeps a
// future exclusion change from reclassifying a real openEHR class as a name no
// schema declares).
//
// Today's pinned BMM cannot produce the shape — every declared-but-omitted
// name matches a rule — so the test constructs it directly: a class the
// schema declares and the plan's universe does not carry.
func TestRMInfoAbsenceUnaccountedNameFailsGeneration(t *testing.T) {
	plan := &Plan{
		Target: TargetRM,
		Schema: &bmm.Schema{
			Packages: map[string]*bmm.Package{
				"rm_test": {Name: "org.openehr.rm.test", Classes: []string{"MYSTERY"}},
			},
			ClassDefinitions: map[string]bmm.Class{
				"MYSTERY": mustSimpleClass(t, `{"name":"MYSTERY"}`),
			},
		},
		ClassPackages: map[string]string{"MYSTERY": "org.openehr.rm.test"},
		Classes:       map[string]*PlannedClass{},
	}

	entries, err := rmInfoAbsence(plan)
	if err == nil {
		t.Fatalf("rmInfoAbsence accepted an unaccounted class, entries = %v", entries)
	}
	if !strings.Contains(err.Error(), "MYSTERY") {
		t.Errorf("error does not name the class: %v", err)
	}

	// The same failure must stop the render rather than emit a partial table.
	if body, err := RenderRMInfoAbsenceFile(plan); err == nil {
		t.Errorf("RenderRMInfoAbsenceFile emitted %d bytes for an unaccounted class", len(body))
	}
}

// TestRMInfoAbsenceNeedsTheSchema — the declarations ARE this table's input,
// so a plan without a schema is an error rather than an empty table that
// claims the pinned BMM declares nothing.
func TestRMInfoAbsenceNeedsTheSchema(t *testing.T) {
	_, err := rmInfoAbsence(&Plan{Target: TargetRM, Classes: map[string]*PlannedClass{}})
	if err == nil {
		t.Error("rmInfoAbsence accepted a plan with no schema")
	}
}

// TestRMInfoAbsenceAccountsForThePinnedBMM — on the real pinned schemas the
// table and the universe partition the declared names: no name in both, no
// declared name in neither (REQ-049 § Generated, accounted, computed). The
// independent BMM reduction is PROBE-098's job; this asserts the generator's
// own two halves add up.
func TestRMInfoAbsenceAccountsForThePinnedBMM(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	entries, err := rmInfoAbsence(plan)
	if err != nil {
		t.Fatalf("rmInfoAbsence: %v", err)
	}

	universe := rmInfoUniverse(plan)
	inUniverse := make(map[string]bool, len(universe))
	for _, name := range universe {
		inUniverse[name] = true
	}
	absent := make(map[string]absenceReason, len(entries))
	for _, e := range entries {
		if inUniverse[e.className] {
			t.Errorf("%s is in the class universe AND in the absence table", e.className)
		}
		if _, dup := absent[e.className]; dup {
			t.Errorf("%s appears twice in the absence table", e.className)
		}
		absent[e.className] = e.reason
	}

	declared := map[string]bool{}
	for name := range plan.Schema.ClassDefinitions {
		declared[name] = true
	}
	for name := range plan.Schema.PrimitiveTypes {
		declared[name] = true
	}
	for name := range declared {
		if !inUniverse[name] && absent[name] == "" {
			t.Errorf("declared class %s is neither in the universe nor in the absence table", name)
		}
	}
	if len(universe)+len(entries) != len(declared) {
		t.Errorf("universe %d + table %d != declared %d", len(universe), len(entries), len(declared))
	}

	// One representative name per reason, pinned so a silent
	// reclassification fails here rather than in a consumer.
	for name, want := range map[string]absenceReason{
		"EXTRACT":         absenceExcludedPackage, // org.openehr.rm.ehr_extract.common
		"TUPLE":           absenceExcludedClass,   // named exclusion + skipped package
		"PROPORTION_KIND": absenceEnumeration,
		"Interval":        absencePrimitive, // emitted as a Go type, still not a class
		"Ordered":         absencePrimitive, // belt and braces vs the exclusion set
	} {
		if absent[name] != want {
			t.Errorf("%s: absence reason %q, want %q", name, absent[name], want)
		}
	}
}

// TestRenderRMInfoAbsenceFile checks the emitted file: the generated header,
// the map declaration, determinism across runs, and that no computed reason is
// ever stored (REQ-049 — *undeclared* is computed, *none* means in-universe).
func TestRenderRMInfoAbsenceFile(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	body, err := RenderRMInfoAbsenceFile(plan)
	if err != nil {
		t.Fatalf("RenderRMInfoAbsenceFile: %v", err)
	}
	src := string(body)

	for _, want := range []string{
		"// Code generated by bmmgen; DO NOT EDIT.\n",
		"// Source: " + TargetRM.SourceLabel + "\n",
		"package rminfo\n",
		"var absenceData = map[string]AbsenceReason{",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("emitted file does not contain %q", want)
		}
	}
	// One row, matched loosely on the gofmt alignment padding.
	if !regexp.MustCompile(`(?m)^\t"EXTRACT": +AbsenceExcludedPackage,$`).MatchString(src) {
		t.Error(`emitted file has no "EXTRACT": AbsenceExcludedPackage row`)
	}
	for _, never := range []string{"AbsenceNone", "AbsenceUndeclared"} {
		if strings.Contains(src, never) {
			t.Errorf("emitted file stores %s — that answer is computed, never stored", never)
		}
	}

	again, err := RenderRMInfoAbsenceFile(plan)
	if err != nil {
		t.Fatalf("RenderRMInfoAbsenceFile (second run): %v", err)
	}
	if !bytes.Equal(body, again) {
		t.Error("two renders of one plan differ — output is not deterministic")
	}
}

// TestRenderRMInfoAbsenceFileSkipsNonRMTargets — rminfo is generated alongside
// the RM target only, exactly as lookup_gen.go is.
func TestRenderRMInfoAbsenceFileSkipsNonRMTargets(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	body, err := RenderRMInfoAbsenceFile(plan)
	if err != nil {
		t.Fatalf("RenderRMInfoAbsenceFile(AOM14): %v", err)
	}
	if body != nil {
		t.Errorf("AOM 1.4 target emitted %d bytes of rminfo absence data", len(body))
	}
}
