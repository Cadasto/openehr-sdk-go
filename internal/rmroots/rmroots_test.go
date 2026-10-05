package rmroots_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// bmmArchetypeRoots reads the vendored RM BMM and returns every class it
// defines, the classes that declare the Is_archetype_root invariant, and the
// concrete classes that are archetype roots: a declaring class or any
// descendant of one, abstract classes left out.
func bmmArchetypeRoots(t *testing.T) (classes, declarers, concrete []string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "resources", "bmm", "openehr_rm_1.2.0.bmm.json"))
	if err != nil {
		t.Fatalf("read vendored RM BMM: %v", err)
	}
	var schema struct {
		ClassDefinitions map[string]struct {
			Ancestors  []string          `json:"ancestors"`
			IsAbstract bool              `json:"is_abstract"`
			Invariants map[string]string `json:"invariants"`
		} `json:"class_definitions"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode vendored RM BMM: %v", err)
	}
	defs := schema.ClassDefinitions
	for name, c := range defs {
		classes = append(classes, name)
		expr, ok := c.Invariants["Is_archetype_root"]
		if !ok {
			continue
		}
		if expr != "is_archetype_root" {
			t.Errorf("BMM %s.Is_archetype_root = %q, want %q: the list reads the invariant as fixing is_archetype_root true", name, expr, "is_archetype_root")
		}
		declarers = append(declarers, name)
	}
	memo := map[string]bool{}
	var isRoot func(string) bool
	isRoot = func(name string) bool {
		if v, seen := memo[name]; seen {
			return v
		}
		memo[name] = false // guards a cyclic ancestry
		c, ok := defs[name]
		if !ok {
			return false
		}
		_, declares := c.Invariants["Is_archetype_root"]
		root := declares || slices.ContainsFunc(c.Ancestors, isRoot)
		memo[name] = root
		return root
	}
	for name, c := range defs {
		if !c.IsAbstract && isRoot(name) {
			concrete = append(concrete, name)
		}
	}
	slices.Sort(classes)
	slices.Sort(declarers)
	slices.Sort(concrete)
	return classes, declarers, concrete
}

// TestREQ112_IsArchetypeRootMatchesBMM pins the closed list of archetype-root
// classes to the vendored BMM (REQ-112, ADR 0001). It is the one test that
// reads the BMM for the list; the RM floor's own test and the generator's
// tests take their expectations from IsArchetypeRoot.
//
// The BMM must give the declaring classes and concrete roots the spec names.
// For every class the BMM defines, IsArchetypeRoot must answer true exactly
// when the class is concrete and declares Is_archetype_root or descends from a
// class that does; the abstract declarers (PARTY, ENTRY) and every other class
// answer false, as does any name the BMM does not define. Every root class
// must have a registered LOCATABLE Go type, or neither the floor nor the
// generator can reach it. A BMM bump that adds a root class fails here until
// the list and the spec gain it.
func TestREQ112_IsArchetypeRootMatchesBMM(t *testing.T) {
	classes, declarers, roots := bmmArchetypeRoots(t)
	if want := []string{"COMPOSITION", "EHR_ACCESS", "EHR_STATUS", "ENTRY", "PARTY"}; !slices.Equal(declarers, want) {
		t.Errorf("BMM Is_archetype_root declarers = %v, want %v: update the list and the spec together (ADR 0001)", declarers, want)
	}
	wantRoots := []string{
		"ACTION", "ADMIN_ENTRY", "AGENT", "COMPOSITION", "EHR_ACCESS", "EHR_STATUS", "EVALUATION",
		"GROUP", "INSTRUCTION", "OBSERVATION", "ORGANISATION", "PERSON", "ROLE",
	}
	if !slices.Equal(roots, wantRoots) {
		t.Errorf("BMM concrete archetype-root classes = %v, want %v: update the list and the spec together (ADR 0001)", roots, wantRoots)
	}
	for _, name := range classes {
		want := slices.Contains(roots, name)
		if got := rmroots.IsArchetypeRoot(name); got != want {
			t.Errorf("IsArchetypeRoot(%q) = %v, want %v from the vendored BMM (concrete roots %v): update the list and the spec together (ADR 0001)", name, got, want, roots)
		}
	}
	for _, name := range []string{"", "observation", " OBSERVATION", "OBSERVATION ", "DV_INTERVAL<OBSERVATION>", "Any", "X_UNKNOWN"} {
		if rmroots.IsArchetypeRoot(name) {
			t.Errorf("IsArchetypeRoot(%q) = true, want false: the BMM defines no class of that name", name)
		}
	}
	for _, name := range roots {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			t.Errorf("archetype root %s has no registered Go type, so neither the floor nor the generator can reach it", name)
			continue
		}
		if v := ctor(); !isLocatable(v) {
			t.Errorf("archetype root %s is registered as %T, which is not a LOCATABLE", name, v)
		}
	}
}

// isLocatable reports whether v is a LOCATABLE that can carry
// archetype_details.
func isLocatable(v any) bool {
	_, ok := v.(rm.MutableLocatable)
	return ok
}
