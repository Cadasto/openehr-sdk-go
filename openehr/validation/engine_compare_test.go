package validation_test

// engine_compare_test.go: REQ-171 § One walk, § Acceptance — the comparison
// harness. The one walk runs beside the floor until the comparison is done:
// over the vendored corpus, a root of every class the registry knows and the
// constraint-fixture matrix, a walk with no template reports the issues
// ValidateRM reports, finding by finding on path, code, layer and rule.
// Decoded documents, zero values and the generator's Example instances hold
// no shared pointer, so the one walk's only difference from the floor, a
// shared value validated at each place, cannot arise, and the two must agree
// exactly.

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// findingKey is what makes two issues the same finding for the comparison:
// the path, the code, the layer and the rule.
type findingKey struct {
	path, code string
	layer      validation.Layer
	rule       string
}

// String writes the key as the harness reports it.
func (k findingKey) String() string {
	return fmt.Sprintf("%s %s layer=%q rule=%q", k.code, k.path, k.layer, k.rule)
}

// findingKeys returns the key of each of issues, in result order.
func findingKeys(issues []validation.Issue) []findingKey {
	keys := make([]findingKey, len(issues))
	for i, iss := range issues {
		keys[i] = findingKey{path: iss.Path, code: iss.Code, layer: iss.Layer, rule: iss.Rule}
	}
	return keys
}

// multisetDiff returns, sorted, one line for each finding the two lists do
// not hold the same number of times: how often the floor holds it and how
// often the engine does.
func multisetDiff(floor, engine []findingKey) []string {
	counts := map[findingKey][2]int{}
	for _, k := range floor {
		c := counts[k]
		c[0]++
		counts[k] = c
	}
	for _, k := range engine {
		c := counts[k]
		c[1]++
		counts[k] = c
	}
	var out []string
	for k, c := range counts {
		if c[0] != c[1] {
			out = append(out, fmt.Sprintf("  floor %d, engine %d: %s", c[0], c[1], k))
		}
	}
	slices.Sort(out)
	return out
}

// comparedRoot is one root the harness validates both ways.
type comparedRoot struct {
	name string
	root any
}

// comparisonRoots returns every root the harness compares, by the group it
// comes from: the vendored corpus documents, the Example instance the
// generator builds for each corpus template, the corpus EHR_STATUS samples,
// the zero value of every RM class the registry knows, the instances of the
// constraint-fixture matrix, and the roots the guards refuse. It fails the
// test when a group is empty.
func comparisonRoots(t *testing.T) map[string][]comparedRoot {
	t.Helper()
	groups := map[string][]comparedRoot{}

	for _, doc := range corpusDocuments(t) {
		groups["corpus"] = append(groups["corpus"], comparedRoot{name: doc.rel.Rel, root: doc.root})
		if doc.c == nil {
			continue
		}
		if root, err := generateExample(t, doc.c); err == nil {
			groups["generated"] = append(groups["generated"], comparedRoot{name: "Example of " + doc.rel.Template, root: root})
		}
	}

	statuses, err := filepath.Glob(filepath.Join(fixtures.CorpusRoot(), "rm", "ehr_status_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range statuses {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var status rm.EHRStatus
		if err := canjson.Unmarshal(body, &status); err != nil {
			continue // a decode failure is the codec's tests' concern
		}
		groups["ehr_status"] = append(groups["ehr_status"], comparedRoot{name: filepath.Base(path), root: &status})
	}

	for _, name := range typereg.Default.Names() {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			t.Fatalf("typereg.Default lists %s but has no constructor for it", name)
		}
		root := ctor()
		typ := reflect.TypeOf(root)
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ.PkgPath() != rmPackage {
			continue
		}
		groups["registry"] = append(groups["registry"], comparedRoot{name: "zero " + name, root: root})
	}

	ids, err := fixtures.ConstraintTemplateIDs()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		raw, err := os.ReadFile(fixtures.CompositionJSON(id))
		if err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		var comp rm.Composition
		if err := canjson.Unmarshal(raw, &comp); err != nil {
			t.Fatalf("decode %s: %v", id, err)
		}
		groups["constraint_fixtures"] = append(groups["constraint_fixtures"], comparedRoot{name: id, root: &comp})
	}

	groups["guards"] = []comparedRoot{
		{name: "nil"},
		{name: "a typed-nil COMPOSITION", root: (*rm.Composition)(nil)},
		{name: "a value outside the closed RM set", root: struct{ Name string }{Name: "not RM"}},
	}

	for _, group := range []string{"corpus", "generated", "ehr_status", "registry", "constraint_fixtures"} {
		if len(groups[group]) == 0 {
			t.Fatalf("the harness found no %s root; its listing has gone blind", group)
		}
	}
	if n := len(groups["registry"]); n != registeredRMClasses {
		t.Fatalf("the harness built %d registry roots, want %d: a class was skipped", n, registeredRMClasses)
	}
	return groups
}

// REQ-171 § One walk — with no template the one walk is the floor: over every
// root the harness loads, it reports the issues ValidateRM reports, as
// multisets keyed by path, code, layer and rule, and in the same order, with
// the same coverage. None of these roots holds a shared pointer, so no
// difference is allowed; the test lists each one it finds.
func TestREQ171_EngineMatchesTheFloorWithNoTemplate(t *testing.T) {
	groups := comparisonRoots(t)
	for _, group := range slices.Sorted(maps.Keys(groups)) {
		roots, issues := 0, 0
		for _, cr := range groups[group] {
			floor := validation.ValidateRM(cr.root)
			engine := validation.RunEngine(cr.root, nil, validation.Limits{})
			roots++
			issues += len(floor.Issues)
			floorKeys, engineKeys := findingKeys(floor.Issues), findingKeys(engine.Issues)
			if diff := multisetDiff(floorKeys, engineKeys); len(diff) > 0 {
				t.Errorf("%s %s: the engine's findings differ from ValidateRM's (%d against %d):\n%s", group, cr.name, len(engineKeys), len(floorKeys), strings.Join(diff, "\n"))
				continue
			}
			if !slices.Equal(floorKeys, engineKeys) {
				t.Errorf("%s %s: the engine reports ValidateRM's findings in another order:\n engine %v\n floor  %v", group, cr.name, engineKeys, floorKeys)
				continue
			}
			if !reflect.DeepEqual(engine, floor) {
				t.Errorf("%s %s: the engine's result differs from ValidateRM's beyond the finding keys:\n engine %+v\n floor  %+v", group, cr.name, engine, floor)
			}
		}
		t.Logf("%s: compared %d roots, %d issues", group, roots, issues)
	}
}

// REQ-171 § One walk — the harness can fail: a list that drops a finding,
// adds one, or repeats one is reported as a difference, and a list that holds
// the same findings in another order is not.
func TestREQ171_EngineComparisonFailsOnEachDifference(t *testing.T) {
	t.Parallel()
	a := findingKey{path: "/a", code: "required", layer: validation.LayerRM, rule: "mandatory"}
	b := findingKey{path: "/b", code: "rm_invariant", layer: validation.LayerRM, rule: "ELEMENT.Inv_null_flavour_indicated"}
	bOtherRule := findingKey{path: "/b", code: "rm_invariant", layer: validation.LayerRM, rule: "DV_TEXT.Mappings_valid"}
	tests := []struct {
		name          string
		floor, engine []findingKey
		differs       bool
	}{
		{name: "the same findings", floor: []findingKey{a, b}, engine: []findingKey{a, b}},
		{name: "the same findings in another order", floor: []findingKey{a, b}, engine: []findingKey{b, a}},
		{name: "a finding dropped", floor: []findingKey{a, b}, engine: []findingKey{a}, differs: true},
		{name: "a finding added", floor: []findingKey{a}, engine: []findingKey{a, b}, differs: true},
		{name: "a finding repeated", floor: []findingKey{a}, engine: []findingKey{a, a}, differs: true},
		{name: "another rule at one path", floor: []findingKey{b}, engine: []findingKey{bOtherRule}, differs: true},
	}
	for _, tc := range tests {
		if got := multisetDiff(tc.floor, tc.engine); (len(got) > 0) != tc.differs {
			t.Errorf("multisetDiff(%s) = %q, want a difference: %t", tc.name, got, tc.differs)
		}
	}
}
