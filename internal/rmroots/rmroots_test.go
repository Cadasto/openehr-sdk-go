package rmroots_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
)

// bmmArchetypeRoots reads the vendored RM BMM and returns every class it
// defines, the classes that declare the Is_archetype_root invariant, and the
// concrete classes that are archetype roots: a declaring class or any
// descendant of one, abstract classes left out. It walks the BMM the way the
// RM floor's own test does (openehr/validation/rmfloor_archetype_test.go).
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
// classes to the vendored BMM (REQ-112, ADR 0001). For every class the BMM
// defines, IsArchetypeRoot must answer true exactly when the class is
// concrete and declares Is_archetype_root or descends from a class that
// does. The abstract declarers (PARTY, ENTRY) and every other class answer
// false, as does any name the BMM does not define. A BMM bump that adds a
// root class fails here until the list gains it.
func TestREQ112_IsArchetypeRootMatchesBMM(t *testing.T) {
	classes, declarers, roots := bmmArchetypeRoots(t)
	if len(declarers) == 0 || len(roots) == 0 {
		t.Fatalf("BMM gives %d Is_archetype_root declarers and %d concrete roots; want some of each, or the check below is vacuous", len(declarers), len(roots))
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
}
