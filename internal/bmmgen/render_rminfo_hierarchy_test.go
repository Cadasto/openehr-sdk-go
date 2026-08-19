package bmmgen

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// rmInfoTestPlan builds the real RM plan for a test that needs the pinned
// model rather than a synthetic one.
func rmInfoTestPlan(t *testing.T) *Plan {
	t.Helper()
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	return plan
}

// synthClass builds a *bmm.SimpleClass with single properties in the given
// declaration order. Built field-by-field rather than from JSON because the
// property variants decode through the schema loader's discriminator, which
// plain json.Unmarshal does not reach.
func synthClass(name string, ancestors []string, propNames, propTypes []string) *bmm.SimpleClass {
	sc := &bmm.SimpleClass{}
	sc.Name = name
	sc.Ancestors_ = ancestors
	sc.Properties = make(map[string]bmm.Property, len(propNames))
	for i, pn := range propNames {
		p := &bmm.SingleProperty{TypeName: propTypes[i]}
		p.Name = pn
		sc.Properties[pn] = p
	}
	sc.PropertyOrder = propNames
	return sc
}

// TestEffectivePropertiesRecordsDeclaringClass asserts that the fold which
// flattens ancestor attributes into every descendant also records WHICH class
// declared each attribute (REQ-048 — the declaration site the fold erases).
//
// The site must come from the fold itself: the declaring class recorded for an
// attribute is the same class whose declaration supplied that attribute's
// type. A second, independent walk could disagree.
func TestEffectivePropertiesRecordsDeclaringClass(t *testing.T) {
	plan := &Plan{
		Target: TargetRM,
		Classes: map[string]*PlannedClass{
			"BASE": {BMMName: "BASE", GoName: "Base", Class: synthClass(
				"BASE", nil, []string{"name"}, []string{"DV_TEXT"})},
			"MID": {BMMName: "MID", GoName: "Mid", Class: synthClass(
				"MID", []string{"BASE"}, []string{"mid_own"}, []string{"STRING"})},
			"LEAF": {BMMName: "LEAF", GoName: "Leaf", Class: synthClass(
				"LEAF", []string{"MID"}, []string{"name"}, []string{"DV_CODED_TEXT"})},
		},
		AbstractDescendants: map[string][]string{},
		ConcreteSubtypes:    map[string][]string{},
		CyclicSingleProps:   map[string]map[string]bool{},
	}

	cases := []struct {
		class, attr    string
		wantDeclaredIn string
		wantType       string
	}{
		// Inherited two levels up: the site is the declaring ancestor,
		// not the class the caller asked about.
		{"MID", "name", "BASE", "DV_TEXT"},
		// Own attribute: the site is the class itself.
		{"MID", "mid_own", "MID", "STRING"},
		// Redefined on the descendant: the most-derived declaration wins
		// on shape, so it must also win on site — the two cannot disagree.
		{"LEAF", "name", "LEAF", "DV_CODED_TEXT"},
		{"LEAF", "mid_own", "MID", "STRING"},
	}
	for _, tc := range cases {
		attrs, _ := effectiveProperties(plan, tc.class)
		got, ok := attrs[tc.attr]
		if !ok {
			t.Errorf("%s.%s: absent from effective properties", tc.class, tc.attr)
			continue
		}
		if got.declaredIn != tc.wantDeclaredIn {
			t.Errorf("%s.%s declaredIn = %q, want %q", tc.class, tc.attr, got.declaredIn, tc.wantDeclaredIn)
		}
		if got.typeName != tc.wantType {
			t.Errorf("%s.%s typeName = %q, want %q — the site must track the shape",
				tc.class, tc.attr, got.typeName, tc.wantType)
		}
	}
}

// TestRMInfoUniverseIncludesAttributeLessClasses pins the class universe
// REQ-048 defines: every class the RM target emits a Go type for, including
// the abstract and attribute-less ones. Before REQ-048 the table carried only
// attribute-BEARING classes, so DATA_VALUE and PATHABLE — the abstract roots a
// descendant expansion starts from — were unaskable.
//
// The set below is exactly the delta REQ-048 adds to KnownRMTypes(). It is
// pinned so that widening it again is a deliberate edit, not a side effect.
func TestRMInfoUniverseIncludesAttributeLessClasses(t *testing.T) {
	plan := rmInfoTestPlan(t)
	universe := rmInfoUniverse(plan)

	var attributeLess []string
	for _, name := range universe {
		if attrs, _ := effectiveProperties(plan, name); len(attrs) == 0 {
			attributeLess = append(attributeLess, name)
		}
	}
	want := []string{
		"ACCESS_CONTROL_SETTINGS",
		"BASIC_DEFINITIONS",
		"CODE_SET_ACCESS",
		"DATA_VALUE",
		"EXTERNAL_ENVIRONMENT_ACCESS",
		"Iso8601_timezone",
		"MEASUREMENT_SERVICE",
		"OPENEHR_CODE_SET_IDENTIFIERS",
		"OPENEHR_DEFINITIONS",
		"OPENEHR_TERMINOLOGY_GROUP_IDENTIFIERS",
		"PATHABLE",
		"TERMINOLOGY_ACCESS",
		"TERMINOLOGY_SERVICE",
		"Time_Definitions",
	}
	if !slices.Equal(attributeLess, want) {
		t.Errorf("attribute-less classes in the universe:\n got %v\nwant %v", attributeLess, want)
	}

	// The wholesale generator exclusions stay excluded: ehr_extract
	// (REQ-042), enumerations, primitives, foundation typing constraints.
	for _, excluded := range []string{
		"EXTRACT", "SYNC_EXTRACT", "MESSAGE", // ehr_extract
		"PROPORTION_KIND",                                      // enumeration
		"String", "Any", "Ordered", "Interval", "Iso8601_date", // primitives / typing constraints
	} {
		if slices.Contains(universe, excluded) {
			t.Errorf("universe contains %q, which the generator skips wholesale", excluded)
		}
	}
	for _, want := range []string{"COMPOSITION", "LOCATABLE", "DV_QUANTITY", "ENTRY"} {
		if !slices.Contains(universe, want) {
			t.Errorf("universe missing %q", want)
		}
	}
	if !slices.IsSorted(universe) {
		t.Error("universe is not sorted — emission must be deterministic")
	}
}

// TestRMInfoParentsAreClosedOverTheUniverse asserts REQ-048's closure rule: an
// emitted parent edge always names a class in the universe. A BMM ancestor
// outside it — the foundation typing constraints (Any, Ordered, Interval, the
// Iso8601_* types) and the PROPORTION_KIND enumeration — is dropped, so the
// emitted graph is the RM class graph rather than the RM class graph plus
// unaskable names.
//
// Dropping those edges must not cost a single RM edge, which is why each case
// below pairs a dropped ancestor with the RM ancestor beside it.
func TestRMInfoParentsAreClosedOverTheUniverse(t *testing.T) {
	plan := rmInfoTestPlan(t)
	universe := rmInfoUniverse(plan)
	inUniverse := make(map[string]bool, len(universe))
	for _, n := range universe {
		inUniverse[n] = true
	}

	for _, name := range universe {
		for _, parent := range rmInfoParents(plan.Classes[name], inUniverse) {
			if !inUniverse[parent] {
				t.Errorf("%s has parent %q, which is not in the class universe", name, parent)
			}
		}
	}

	cases := map[string][]string{
		// Plain single inheritance inside the universe, unchanged.
		"LOCATABLE":  {"PATHABLE"},
		"CARE_ENTRY": {"ENTRY"},
		"ENTRY":      {"CONTENT_ITEM"},
		// PATHABLE's only BMM ancestor is `Any`, so it is a root.
		"PATHABLE": nil,
		// These declare an RM ancestor AND a foundation typing
		// constraint. The RM edge survives; the constraint does not.
		"DV_ORDERED":    {"DATA_VALUE"},  // ancestors: DATA_VALUE, Ordered
		"DV_INTERVAL":   {"DATA_VALUE"},  // ancestors: DATA_VALUE, Interval
		"DV_DATE":       {"DV_TEMPORAL"}, // ancestors: DV_TEMPORAL, Iso8601_date
		"DV_PROPORTION": {"DV_AMOUNT"},   // ancestors: PROPORTION_KIND, DV_AMOUNT
		// Genuine multiple inheritance inside the universe keeps both
		// edges, in BMM declaration order.
		"TERMINOLOGY_SERVICE": {"OPENEHR_TERMINOLOGY_GROUP_IDENTIFIERS", "OPENEHR_CODE_SET_IDENTIFIERS"},
	}
	for class, want := range cases {
		got := rmInfoParents(plan.Classes[class], inUniverse)
		if !slices.Equal(got, want) {
			t.Errorf("%s parents = %v, want %v", class, got, want)
		}
	}
}

// TestRenderRMInfoEmitsHierarchyFields asserts the rendered table carries the
// new generated fields — the data REQ-048 forbids re-deriving at run time and
// REQ-042 drift-checks.
func TestRenderRMInfoEmitsHierarchyFields(t *testing.T) {
	plan := rmInfoTestPlan(t)
	out, err := RenderRMInfoFile(plan)
	if err != nil {
		t.Fatalf("RenderRMInfoFile: %v", err)
	}
	src := string(out)

	// Patterns, not literals: gofmt aligns keyed struct fields, so the
	// run of spaces after a key depends on its neighbours in the block.
	for _, want := range []string{
		`"DATA_VALUE": \{(?s:.*?)Abstract: +true,`,
		`"PATHABLE": \{(?s:.*?)Abstract: +true,`,
		`"LOCATABLE": \{(?s:.*?)Parents: +\[\]string\{"PATHABLE"\},`,
		`"name": +\{TypeName: "DV_TEXT", Required: true, Container: false, DeclaredIn: "LOCATABLE"\},`,
	} {
		if !regexp.MustCompile(want).MatchString(src) {
			t.Errorf("rendered rminfo table does not match %q", want)
		}
	}
	// A concrete class carries no Abstract key at all (the zero value is
	// omitted), so the flag can never be silently inverted by a
	// mis-emitted literal.
	if regexp.MustCompile(`"DV_QUANTITY": \{[^}]*Abstract`).MatchString(src) {
		t.Error("concrete class DV_QUANTITY emitted an Abstract key")
	}
}

// TestDriftDetectionCoversHierarchyFields — REQ-042 discipline applied to the
// REQ-048 fields. TestDriftDetection already proves the mechanism, but it
// mutates a marker comment in a datatype file; REQ-048's acceptance criteria
// claim specifically that hand-editing a generated HIERARCHY field is caught,
// and nothing asserted that until now.
//
// The mutation is semantic rather than cosmetic on purpose: flipping
// `Abstract: true` to `false` is exactly the hand-edit that would make an
// abstract class look instantiable, so it is the one the drift check has to
// catch.
func TestDriftDetectionCoversHierarchyFields(t *testing.T) {
	dir := t.TempDir()
	opts := Options{
		ResourcesDir: testResources,
		OutDir:       dir,
		RootID:       "openehr_rm_1.2.0",
	}
	if _, err := Run(opts); err != nil {
		t.Fatalf("initial Run: %v", err)
	}
	target := filepath.Join(dir, "openehr", "rm", "rminfo", "lookup_gen.go")
	orig, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}

	// One case per generated hierarchy field, each restored before the next.
	// Each replacement is the SAME byte length as what it overwrites, so the
	// mutation is a pure substitution rather than a truncation the drift check
	// could catch for the wrong reason. The spellings carry gofmt's column
	// padding, which is why they are matched literally rather than by name.
	cases := []struct {
		name, from, to string
	}{
		{"Abstract", "Abstract:   true,", "Abstract:   false"},
		{"Parents", `Parents:    []string{"OPENEHR_DEFINITIONS"}`, `Parents:    []string{"openehr_definitions"}`},
		{"DeclaredIn", `DeclaredIn: "LOCATABLE"`, `DeclaredIn: "COMPOSITIO"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := bytes.Index(orig, []byte(tc.from))
			if idx < 0 {
				t.Fatalf("generated table has no %q to mutate", tc.from)
			}
			mutated := append([]byte{}, orig...)
			copy(mutated[idx:], tc.to)
			if err := os.WriteFile(target, mutated, 0o644); err != nil {
				t.Fatalf("write mutated: %v", err)
			}
			defer func() {
				if err := os.WriteFile(target, orig, 0o644); err != nil {
					t.Fatalf("restore: %v", err)
				}
			}()

			verify := opts
			verify.Verify = true
			result, err := Run(verify)
			if err != nil {
				t.Fatalf("verify Run: %v", err)
			}
			if !slices.ContainsFunc(result.Drifts, func(d DriftRecord) bool { return d.Path == target }) {
				t.Errorf("hand-editing %s went undetected; drifts = %v", tc.name, result.Drifts)
			}
		})
	}

	// Control: with the file restored, the tree is clean again — otherwise
	// the assertions above could be passing on unrelated drift.
	verify := opts
	verify.Verify = true
	result, err := Run(verify)
	if err != nil {
		t.Fatalf("final verify Run: %v", err)
	}
	if len(result.Drifts) != 0 {
		t.Errorf("tree not clean after restore: %v", result.Drifts)
	}
}
