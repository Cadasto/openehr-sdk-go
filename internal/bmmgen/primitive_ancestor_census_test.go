package bmmgen

// STRAND-13 evidence: which class_definitions classes inherit a property from
// a primitive_types ancestor the generator maps to a Go primitive — and so
// never plans as a class, dropping the property from both the emitted struct
// and the rminfo tables. The strand's "evidence needed" is exactly this census,
// run across every pinned schema root rather than the RM reduction PROBE-094
// surfaces. The set is PINNED, not tolerated: a new entry must be added here by
// name (and the strand updated) before it can pass, and an entry that stops
// occurring is reported as stale. Folding anything in is forbidden ahead of the
// strand (REQ-048 § The attribute tables are complete against the BMM).

import (
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// pinnedSchemaRoots are the six roots vendored under resources/bmm, in the
// order the census reports them.
var pinnedSchemaRoots = []string{
	"openehr_base_1.3.0", "openehr_rm_1.2.0", "openehr_am_1.4.0",
	"openehr_am_2.4.0", "openehr_lang_1.1.0", "openehr_term_3.1.0",
}

// primitiveAncestorDrops is the census result as of 2026-09-05:
// "<Class>.<property> via <primitive-mapped ancestor>" -> the roots it appears
// in (same order as pinnedSchemaRoots). Exactly one: Iso8601_timezone declares
// no properties of its own and reaches `value` only through Iso8601_type. It
// shows up in every root that includes base; openehr_term_3.1.0 includes no
// base schema and so has no ISO 8601 classes at all. The four DV temporal
// types (DV_DATE, DV_TIME, DV_DATE_TIME, DV_DURATION) also descend from
// Iso8601_type but redeclare `value` themselves, so they are shipped and are
// deliberately NOT here.
var primitiveAncestorDrops = map[string][]string{
	"Iso8601_timezone.value via Iso8601_type": {
		"openehr_base_1.3.0", "openehr_rm_1.2.0", "openehr_am_1.4.0",
		"openehr_am_2.4.0", "openehr_lang_1.1.0",
	},
}

func TestPrimitiveMappedAncestorPropertyCensus(t *testing.T) { // STRAND-13
	got := map[string][]string{}
	for _, root := range pinnedSchemaRoots {
		schema, err := bmm.LoadAll(root, bmm.FSResolver{Root: testResources})
		if err != nil {
			t.Fatalf("LoadAll(%s): %v", root, err)
		}
		lookup := func(name string) (bmm.Class, bool) {
			if c, ok := schema.ClassDefinitions[name]; ok {
				return c, true
			}
			c, ok := schema.PrimitiveTypes[name]
			return c, ok
		}
		for _, name := range slices.Sorted(maps.Keys(schema.ClassDefinitions)) {
			// A property the class itself, or any non-primitive ancestor,
			// declares is planned and shipped (DV_DATE redeclares `value`
			// beside inheriting it from Iso8601_type, and the generator
			// emits it) — only a property reachable solely through a
			// primitive-mapped ancestor is dropped.
			declared := map[string]bool{}
			if c, ok := lookup(name); ok {
				props, _ := classProperties(c)
				for p := range props {
					declared[p] = true
				}
			}
			ancestors := transitiveAncestors(lookup, name)
			for _, anc := range ancestors {
				if isPrimitive(anc) {
					continue
				}
				if ac, ok := lookup(anc); ok {
					props, _ := classProperties(ac)
					for p := range props {
						declared[p] = true
					}
				}
			}
			for _, anc := range ancestors {
				if !isPrimitive(anc) {
					continue
				}
				ac, ok := lookup(anc)
				if !ok {
					continue
				}
				props, _ := classProperties(ac)
				for _, p := range slices.Sorted(maps.Keys(props)) {
					if declared[p] {
						continue
					}
					key := name + "." + p + " via " + anc
					got[key] = append(got[key], root)
				}
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(got)) {
		roots := got[key]
		want, pinned := primitiveAncestorDrops[key]
		if !pinned {
			t.Errorf("unpinned drop %s (in %v): a class_definitions property is inherited from a primitive-mapped ancestor and silently dropped — pin it here and record it under STRAND-13", key, roots)
			continue
		}
		if !slices.Equal(roots, want) {
			t.Errorf("%s: seen in %v, pinned for %v — update the pin and STRAND-13", key, roots, want)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(primitiveAncestorDrops)) {
		if _, ok := got[key]; !ok {
			t.Errorf("stale pin %s: no pinned schema exhibits it any more — drop the pin and close the STRAND-13 evidence", key)
		}
	}
}

// transitiveAncestors walks ancestors depth-first through both class and
// primitive definitions, visiting each name once, in discovery order.
func transitiveAncestors(lookup func(string) (bmm.Class, bool), name string) []string {
	seen := map[string]bool{}
	var out []string
	var rec func(string)
	rec = func(n string) {
		c, ok := lookup(n)
		if !ok {
			return
		}
		for _, anc := range c.Ancestors() {
			if seen[anc] {
				continue
			}
			seen[anc] = true
			out = append(out, anc)
			rec(anc)
		}
	}
	rec(name)
	return out
}
