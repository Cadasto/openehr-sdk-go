package bmmgen

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// withFunctions returns sc with a function of each of names.
func withFunctions(sc *bmm.SimpleClass, names ...string) *bmm.SimpleClass {
	sc.Functions = make(map[string]*bmm.Function, len(names))
	for _, n := range names {
		sc.Functions[n] = &bmm.Function{Name: n}
	}
	return sc
}

// TestEffectiveFunctionsFoldsAncestorsLessProperties_REQ112 pins the set §
// REQ-112 Driver gives rminfo.IsNonStorableAttr: the functions a class
// declares or inherits, less every name that is a property of the class,
// declared there or inherited. A name an ancestor declares as a function and
// the class as a property is the class's property, and a property an
// ancestor declares hides a function of the same name further down.
func TestEffectiveFunctionsFoldsAncestorsLessProperties_REQ112(t *testing.T) {
	plan := &Plan{
		Target: TargetRM,
		Classes: map[string]*PlannedClass{
			"BASE": {BMMName: "BASE", Class: withFunctions(synthClass(
				"BASE", nil, []string{"base_prop"}, []string{"String"}), "shared", "base_fn")},
			"MID": {BMMName: "MID", Class: withFunctions(synthClass(
				"MID", []string{"BASE"}, []string{"shared"}, []string{"Real"}), "mid_fn")},
			"LEAF": {BMMName: "LEAF", Class: withFunctions(synthClass(
				"LEAF", []string{"MID"}, nil, nil), "leaf_fn", "base_prop")},
		},
	}
	tests := []struct {
		class string
		want  []string
	}{
		{class: "BASE", want: []string{"base_fn", "shared"}},
		{class: "MID", want: []string{"base_fn", "mid_fn"}},
		{class: "LEAF", want: []string{"base_fn", "leaf_fn", "mid_fn"}},
	}
	for _, tc := range tests {
		if got := effectiveFunctions(plan, tc.class); !slices.Equal(got, tc.want) {
			t.Errorf("effectiveFunctions(%s) = %q, want %q", tc.class, got, tc.want)
		}
	}
}

// functionEntry returns the names the rendered function table lists for
// class, and whether it lists the class at all.
func functionEntry(t *testing.T, src, class string) ([]string, bool) {
	t.Helper()
	start := strings.Index(src, "var functionData = map[string][]string{")
	if start < 0 {
		t.Fatal("rendered file has no functionData table")
	}
	m := regexp.MustCompile(`(?m)^\t"` + regexp.QuoteMeta(class) + `": +\{([^}]*)\},$`).FindStringSubmatch(src[start:])
	if m == nil {
		return nil, false
	}
	var names []string
	for f := range strings.SplitSeq(m[1], ",") {
		names = append(names, strings.Trim(strings.TrimSpace(f), `"`))
	}
	return names, true
}

// TestRenderRMInfoEmitsFunctions_REQ112 pins the rendered function table on
// the pinned RM: a function the class declares and one an ancestor declares
// are listed, one of an ancestor the class universe leaves out included, and
// a name an ancestor declares as a function and the class as a property is
// not.
func TestRenderRMInfoEmitsFunctions_REQ112(t *testing.T) {
	out, err := RenderRMInfoFile(rmInfoTestPlan(t))
	if err != nil {
		t.Fatalf("RenderRMInfoFile: %v", err)
	}
	src := string(out)
	tests := []struct {
		class, name string
		want        bool
	}{
		{class: "EVENT", name: "offset", want: true},
		{class: "POINT_EVENT", name: "offset", want: true},
		{class: "ELEMENT", name: "is_null", want: true},
		{class: "DV_DURATION", name: "magnitude", want: true},
		{class: "DV_DURATION", name: "years", want: true}, // declared on Iso8601_duration, outside the class universe
		{class: "DV_QUANTITY", name: "magnitude", want: false},
		{class: "DV_QUANTITY", name: "is_integral", want: true},
		{class: "VERSION", name: "data", want: true},
		{class: "ORIGINAL_VERSION", name: "data", want: false},
	}
	for _, tc := range tests {
		names, ok := functionEntry(t, src, tc.class)
		if !ok {
			t.Errorf("functionData has no entry for %s", tc.class)
			continue
		}
		if got := slices.Contains(names, tc.name); got != tc.want {
			t.Errorf("functionData[%s] lists %s = %t, want %t (entry %q)", tc.class, tc.name, got, tc.want, names)
		}
		if !slices.IsSorted(names) {
			t.Errorf("functionData[%s] = %q, want it sorted", tc.class, names)
		}
	}
}

// TestDriftDetectionCoversFunctionTable_REQ042 pins REQ-042 on the function
// table: a hand edit of a function name in the generated lookup_gen.go is a
// drift the verify run reports.
func TestDriftDetectionCoversFunctionTable_REQ042(t *testing.T) {
	dir := t.TempDir()
	opts := Options{
		ResourcesDir:  testResources,
		OutDir:        dir,
		RootID:        "openehr_rm_1.2.0",
		SkipInventory: true,
	}
	if _, err := Run(opts); err != nil {
		t.Fatalf("initial Run: %v", err)
	}
	target := filepath.Join(dir, "openehr", "rm", "rminfo", "lookup_gen.go")
	orig, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	table := bytes.Index(orig, []byte("var functionData"))
	if table < 0 {
		t.Fatal("generated lookup_gen.go has no functionData table")
	}
	idx := bytes.Index(orig[table:], []byte(`"offset"`))
	if idx < 0 {
		t.Fatal(`functionData lists no "offset" to mutate`)
	}
	// Same length and still valid Go, as a real hand edit would be.
	mutated := slices.Clone(orig)
	copy(mutated[table+idx:], `"offsex"`)
	if err := os.WriteFile(target, mutated, 0o644); err != nil {
		t.Fatalf("write mutated: %v", err)
	}
	verify := opts
	verify.Verify = true
	result, err := Run(verify)
	if err != nil {
		t.Fatalf("verify Run: %v", err)
	}
	if !slices.ContainsFunc(result.Drifts, func(d DriftRecord) bool { return d.Path == target }) {
		t.Errorf("hand-editing a function name in %s went undetected; drifts = %v", target, result.Drifts)
	}
}
