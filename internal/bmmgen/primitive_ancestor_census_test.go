package bmmgen

// STRAND-13 evidence: which class_definitions classes inherit a property from
// an ancestor the generator drops — one mapped to a Go primitive, or one on the
// skipped-primitive list — and so never plans as a class, dropping the property
// from both the emitted struct and the rminfo tables. The census asks the
// question with the generator's own predicate ([generatorDropsAncestor]), so
// both halves of the condition the generator actually evaluates are covered.
// The strand's "evidence needed" is exactly this census, run across every
// pinned schema root rather than the RM reduction PROBE-094 surfaces. The set
// is PINNED, not tolerated: a new entry must be added here by name (and the
// strand updated) before it can pass, and an entry that stops occurring is
// reported as stale. Folding anything in is forbidden ahead of the
// strand (REQ-048 § The attribute tables are complete against the BMM).
//
// The census result is a single positive case, and one positive case proves
// little on its own, so these guards keep the evidence honest:
//
//   - TestPinnedSchemaRootsMatchVendoredSchemas checks the root list against
//     what is actually vendored under resources/bmm, so adding a seventh schema
//     cannot slip past the census unwalked;
//   - the census logic runs on hand-built schemas that exercise transitive,
//     shadowed, secondary and cyclic ancestry — see
//     primitive_ancestor_census_synthetic_test.go — so a regression in
//     transitiveAncestors or in the declared-property filter fails loudly
//     instead of quietly emptying or inflating the census;
//   - assertDVTemporalRedeclareValue pins that the four DV temporal types are
//     absent from the census BECAUSE they redeclare `value`, not because they
//     are missing from the schema.

import (
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// pinnedSchemaRoots are the roots vendored under resources/bmm, in the
// order the census reports them. Kept honest against the directory by
// TestPinnedSchemaRootsMatchVendoredSchemas.
var pinnedSchemaRoots = []string{
	"openehr_base_1.3.0", "openehr_rm_1.2.0", "openehr_am_1.4.0",
	"openehr_am_2.4.0", "openehr_lang_1.1.0", "openehr_term_3.1.0",
}

// primitiveAncestorDrops is the census result as of 2026-09-05:
// "<Class>.<property> via <dropped ancestor>" -> the roots it appears
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

// schemaFileSuffix is the filename suffix of a vendored BMM schema. It alone
// defines what counts as a schema root on disk — bmm.FSResolver resolves a
// root as <Root>/<id>.bmm.json, so any file carrying the suffix is a root the
// census must walk, whatever its prefix.
const schemaFileSuffix = ".bmm.json"

// dvTemporalRoot is the schema root that carries the DV temporal types, and
// dvTemporalRedeclarers the four types themselves. They descend from the
// primitive-mapped Iso8601_type yet stay out of the census: each redeclares
// `value`. That has to be asserted, or "not in the census" could equally mean
// "not in the schema".
const dvTemporalRoot = "openehr_rm_1.2.0"

var dvTemporalRedeclarers = []string{"DV_DATE", "DV_TIME", "DV_DATE_TIME", "DV_DURATION"}

func TestPrimitiveMappedAncestorPropertyCensus(t *testing.T) { // STRAND-13
	got := map[string][]string{}
	dvChecked := false
	for _, root := range pinnedSchemaRoots {
		schema, err := bmm.LoadAll(root, bmm.FSResolver{Root: testResources})
		if err != nil {
			t.Fatalf("LoadAll(%s): %v", root, err)
		}
		lookup := schemaClassLookup(schema)
		classes := slices.Sorted(maps.Keys(schema.ClassDefinitions))
		for _, key := range censusPrimitiveAncestorDrops(classes, lookup, generatorDropsAncestor) {
			got[key] = append(got[key], root)
		}
		if root == dvTemporalRoot {
			assertDVTemporalRedeclareValue(t, root, lookup)
			dvChecked = true
		}
	}
	if !dvChecked {
		t.Errorf("assertDVTemporalRedeclareValue never ran: dvTemporalRoot = %q is not in pinnedSchemaRoots %v", dvTemporalRoot, pinnedSchemaRoots)
	}
	for _, key := range slices.Sorted(maps.Keys(got)) {
		roots := got[key]
		want, pinned := primitiveAncestorDrops[key]
		if !pinned {
			t.Errorf("unpinned drop %s (in %v): a class_definitions property is inherited from an ancestor the generator drops (primitive-mapped or skipped-primitive) and is silently lost — pin it here and record it under STRAND-13", key, roots)
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

// TestPinnedSchemaRootsMatchVendoredSchemas keeps pinnedSchemaRoots — a hand
// written list — honest against resources/bmm. Without it, vendoring a seventh
// schema would leave the census silently narrower than the tree it claims to
// cover, and the census would still pass.
func TestPinnedSchemaRootsMatchVendoredSchemas(t *testing.T) { // STRAND-13
	onDisk, err := vendoredSchemaRoots(testResources)
	if err != nil {
		t.Fatalf("vendoredSchemaRoots(%s): %v", testResources, err)
	}
	pinned := slices.Sorted(slices.Values(pinnedSchemaRoots))
	if slices.Equal(onDisk, pinned) {
		return
	}
	for _, root := range onDisk {
		if !slices.Contains(pinned, root) {
			t.Errorf("%s is vendored under %s but the census never walks it: add it to pinnedSchemaRoots and re-run the census; record the result under STRAND-13", root, testResources)
		}
	}
	for _, root := range pinned {
		if !slices.Contains(onDisk, root) {
			t.Errorf("pinnedSchemaRoots names %s but no %s%s exists under %s: remove it from pinnedSchemaRoots and re-run the census; record the result under STRAND-13", root, root, schemaFileSuffix, testResources)
		}
	}
	t.Errorf("pinnedSchemaRoots = %v, vendored = %v", pinned, onDisk)
}

// vendoredSchemaRoots lists the schema roots present in dir — every
// "*.bmm.json" file with the suffix stripped — sorted.
func vendoredSchemaRoots(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var roots []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if root, ok := strings.CutSuffix(entry.Name(), schemaFileSuffix); ok {
			roots = append(roots, root)
		}
	}
	slices.Sort(roots)
	return roots, nil
}

// generatorDropsAncestor is the predicate the generator itself applies when it
// decides an ancestor contributes nothing to the emitted struct: render.go
// (renderConcreteClass) and render_jsonmar.go both skip an ancestor on
// `isPrimitive(anc) || isSkippedPrimitive(anc)`, and plan.go skips the same
// names when it plans primitive_types. A property reachable only through such
// an ancestor is therefore absent from the emitted struct and from the rminfo
// tables alike. The census must ask the question this way: passing isPrimitive
// alone would leave the skipped-primitive half of the generator's condition
// uncensused — the skipped foundation types that actually occur (Ordered,
// Numeric, Ordered_Numeric, ROUTINE, TUPLE) declare no properties, so the
// answer is the same today, but nothing would catch a future schema giving one
// of them a property.
func generatorDropsAncestor(name string) bool {
	return isPrimitive(name) || isSkippedPrimitive(name)
}

// schemaClassLookup resolves a name against a loaded schema, class definitions
// first and primitive types second — the same two maps the generator consults.
func schemaClassLookup(schema *bmm.Schema) func(string) (bmm.Class, bool) {
	return func(name string) (bmm.Class, bool) {
		if c, ok := schema.ClassDefinitions[name]; ok {
			return c, true
		}
		c, ok := schema.PrimitiveTypes[name]
		return c, ok
	}
}

// censusPrimitiveAncestorDrops is the census core, kept free of schema loading
// so the synthetic cases can drive it directly. For each named class it emits
// one "<Class>.<property> via <dropped ancestor>" key per property reachable
// ONLY through an ancestor the generator drops, sorted.
//
// lookup resolves a class or primitive name; dropsAncestor reports whether the
// generator drops an ancestor of that name — [generatorDropsAncestor] over a
// real schema.
func censusPrimitiveAncestorDrops(
	classes []string,
	lookup func(string) (bmm.Class, bool),
	dropsAncestor func(string) bool,
) []string {
	var out []string
	for _, name := range classes {
		// A property the class itself, or any ancestor the generator
		// keeps, declares is planned and shipped (DV_DATE redeclares
		// `value` beside inheriting it from Iso8601_type, and the
		// generator emits it) — only a property reachable solely
		// through a dropped ancestor is lost.
		declared := map[string]bool{}
		if c, ok := lookup(name); ok {
			props, _ := classProperties(c)
			for p := range props {
				declared[p] = true
			}
		}
		ancestors := transitiveAncestors(lookup, name)
		for _, anc := range ancestors {
			if dropsAncestor(anc) {
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
			if !dropsAncestor(anc) {
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
				out = append(out, name+"."+p+" via "+anc)
			}
		}
	}
	slices.Sort(out)
	return out
}

// assertDVTemporalRedeclareValue pins WHY the four DV temporal types stay out
// of the census: each one reaches the primitive-mapped Iso8601_type and each
// one redeclares `value` itself, so the generator plans and emits the property.
// Absence from the census on its own would be satisfied just as well by the
// types having disappeared from the schema, which is not the fact being
// recorded under STRAND-13.
func assertDVTemporalRedeclareValue(t *testing.T, root string, lookup func(string) (bmm.Class, bool)) {
	t.Helper()
	for _, name := range dvTemporalRedeclarers {
		c, ok := lookup(name)
		if !ok {
			t.Errorf("%s: %s is absent from the schema — the census cannot show it is excluded by redeclaration", root, name)
			continue
		}
		ancestors := transitiveAncestors(lookup, name)
		if !slices.Contains(ancestors, "Iso8601_type") {
			t.Errorf("%s: %s no longer reaches Iso8601_type (ancestors %v) — it is excluded from the census by absent ancestry, not by redeclaring `value`", root, name, ancestors)
		}
		props, _ := classProperties(c)
		if _, ok := props["value"]; !ok {
			t.Errorf("%s: %s declares %v, none of them `value` — it inherits `value` only through the primitive-mapped Iso8601_type and belongs in primitiveAncestorDrops", root, name, slices.Sorted(maps.Keys(props)))
		}
	}
}

// transitiveAncestors walks ancestors depth-first through both class and
// primitive definitions, visiting each name once, in discovery order. The
// visited set is what makes a cyclic schema terminate; the synthetic cycle case
// fails if it is lost.
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
