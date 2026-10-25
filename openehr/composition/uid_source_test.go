package composition_test

import (
	"bytes"
	"fmt"
	"maps"
	mrand "math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"uuid"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// issuedUIDs is a counting uid source that remembers every uid it issued.
type issuedUIDs struct {
	issued []string
}

func (r *issuedUIDs) next() *rm.HierObjectID {
	v := fmt.Sprintf("skeleton-uid-%04d", len(r.issued)+1)
	r.issued = append(r.issued, v)
	return &rm.HierObjectID{Value: v}
}

// interval is an AOM interval body over lower..upper, both ends included.
func interval(lower, upper string) string {
	return `<lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>` + lower + `</lower><upper>` + upper + `</upper>`
}

// requiredUID is a uid attribute the template requires, with an existence
// of 1..1 and no child.
const requiredUID = `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>uid</rm_attribute_name><existence>` +
	`<lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded>` +
	`<upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence></attributes>`

// requiredUIDsOPT is a COMPOSITION template whose content holds one
// OBSERVATION it requires, which requires its uid, as does the one ELEMENT
// of its one event, whose DV_COUNT takes a magnitude from 0 to 1000, so
// that RandomFill draws it. The generator gives a uid only where the
// template requires one (REQ-107 Identity), so the skeleton carries two.
var requiredUIDsOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>required_uids</value></template_id><concept>required_uids</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>content</rm_attribute_name>
<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>OBSERVATION</rm_type_name><occurrences>` + interval("1", "1") + `</occurrences><node_id>at0000</node_id>` + requiredUID + `
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>data</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>HISTORY</rm_type_name><node_id>at0001</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>events</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>POINT_EVENT</rm_type_name><occurrences>` + interval("1", "1") + `</occurrences><node_id>at0002</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>data</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0003</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name><occurrences>` + interval("1", "1") + `</occurrences><node_id>at0004</node_id>` + requiredUID + `
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_COUNT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>magnitude</rm_attribute_name>
<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>INTEGER</rm_type_name><node_id></node_id><item xsi:type="C_INTEGER"><range>` + interval("0", "1000") + `</range></item></children>
</attributes></children></attributes></children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes></children></attributes></children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes></children></attributes>
<archetype_id><value>openEHR-EHR-OBSERVATION.required_uids.v1</value></archetype_id></children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.required_uids.v1</value></archetype_id></definition>
</template>`

// requiredUIDTypes are the _type of each object requiredUIDsOPT requires a
// uid of, in the order treeUIDs meets them.
var requiredUIDTypes = []string{"OBSERVATION", "ELEMENT"}

// treeUID is one uid of a decoded tree: the _type of the object that
// carries it, and its value.
type treeUID struct {
	rmType string
	value  string
}

// treeUIDs returns every uid in a decoded tree, an object's own before
// those below it.
func treeUIDs(v any) []treeUID {
	var out []treeUID
	switch node := v.(type) {
	case map[string]any:
		rmType, _ := node["_type"].(string)
		if uid, hasUID := node["uid"].(map[string]any); hasUID {
			value, _ := uid["value"].(string)
			out = append(out, treeUID{rmType: rmType, value: value})
		}
		for _, k := range slices.Sorted(maps.Keys(node)) {
			out = append(out, treeUIDs(node[k])...)
		}
	case []any:
		for _, child := range node {
			out = append(out, treeUIDs(child)...)
		}
	}
	return out
}

// skeletonEntryPoint builds a skeleton from c through one of the two
// entry points.
type skeletonEntryPoint struct {
	name  string
	build func(t *testing.T, c *templatecompile.Compiled, opts ...composition.Option) *rm.Composition
}

// skeletonEntryPoints are NewSkeleton, and NewBuilder followed by Build
// with no Set.
var skeletonEntryPoints = []skeletonEntryPoint{
	{
		name: "NewSkeleton",
		build: func(t *testing.T, c *templatecompile.Compiled, opts ...composition.Option) *rm.Composition {
			t.Helper()
			comp, err := composition.NewSkeleton(t.Context(), c, opts...)
			if err != nil {
				t.Fatalf("NewSkeleton: %v", err)
			}
			return comp
		},
	},
	{
		name: "NewBuilder",
		build: func(t *testing.T, c *templatecompile.Compiled, opts ...composition.Option) *rm.Composition {
			t.Helper()
			b, err := composition.NewBuilder(t.Context(), c, opts...)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			comp, err := b.Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			return comp
		},
	},
}

// TestREQ101_WithUIDSourceGivesEveryUID is the REQ-101 Uid source check
// that WithUIDSource reaches instance.Options.UIDSource for NewSkeleton
// and NewBuilder alike: under Large, the OBSERVATION and the ELEMENT whose
// uid requiredUIDsOPT requires carry one, no other object does (REQ-107
// Identity), and every uid in the skeleton is one the source issued.
func TestREQ101_WithUIDSourceGivesEveryUID(t *testing.T) {
	t.Parallel()
	for _, entry := range skeletonEntryPoints {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			uids := &issuedUIDs{}
			comp := entry.build(t, compileInline(t, requiredUIDsOPT),
				policyOptions(composition.WithPolicy(instance.Large), composition.WithUIDSource(uids.next))...)
			var types []string
			for _, uid := range treeUIDs(decodedJSON(t, comp)) {
				types = append(types, uid.rmType)
				if !slices.Contains(uids.issued, uid.value) {
					t.Errorf("%s uid = %q, want one WithUIDSource's source issued (issued %v)", uid.rmType, uid.value, uids.issued)
				}
			}
			if !slices.Equal(types, requiredUIDTypes) {
				t.Errorf("the skeleton carries uids on %v, want on %v: the objects whose uid the template requires", types, requiredUIDTypes)
			}
		})
	}
}

// seededSkeletonOptions are the options of the reproduction in issue 296:
// Large and RandomFill, with a fixed clock and value and uid sources each
// on a new PCG in a fixed state.
func seededSkeletonOptions() []composition.Option {
	return []composition.Option{
		composition.WithComposer(testComposer()),
		composition.WithTerritory("NL"),
		composition.WithNow(policyNow),
		composition.WithPolicy(instance.Large),
		composition.WithValueFill(instance.RandomFill),
		composition.WithValueSource(mrand.NewPCG(42, 1)),
		composition.WithUIDSource(instance.UIDsFrom(mrand.NewPCG(42, 2))),
	}
}

// TestREQ101_REQ107_SeededSkeletonIsReproducible is the REQ-101 Uid
// source check that, with instance.UIDsFrom on a fixed source, WithNow
// and WithValueSource under instance.RandomFill, a skeleton is
// reproducible as REQ-107 Reproducible uids says: two NewSkeleton calls,
// and two NewBuilder and Build calls, each on fresh sources in the same
// state, give equal canonical JSON, uids included. It holds on
// requiredUIDsOPT, whose skeleton carries uids, and on vital_signs, the
// template of issue 296, whose skeleton carries none, as its OPT requires
// no uid (REQ-107 Identity).
func TestREQ101_REQ107_SeededSkeletonIsReproducible(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		compile func(t *testing.T) *templatecompile.Compiled
		uids    bool
	}{
		{name: "required uids", compile: func(t *testing.T) *templatecompile.Compiled { return compileInline(t, requiredUIDsOPT) }, uids: true},
		{name: "vital_signs", compile: func(t *testing.T) *templatecompile.Compiled { return compileFixture(t, "vital_signs") }},
	} {
		for _, entry := range skeletonEntryPoints {
			t.Run(tc.name+"/"+entry.name, func(t *testing.T) {
				t.Parallel()
				seededSkeletonsMatch(t, entry, tc.compile(t), tc.uids)
			})
		}
	}
}

// seededSkeletonsMatch builds two skeletons of c through entry under
// seededSkeletonOptions and reports where their canonical JSON differs,
// and, where uids is true, a skeleton that carries no uid.
func seededSkeletonsMatch(t *testing.T, entry skeletonEntryPoint, c *templatecompile.Compiled, uids bool) {
	t.Helper()
	build := func() (*rm.Composition, []byte) {
		comp := entry.build(t, c, seededSkeletonOptions()...)
		raw, err := canjson.Marshal(comp)
		if err != nil {
			t.Fatalf("canjson.Marshal: %v", err)
		}
		return comp, raw
	}
	comp, first := build()
	_, second := build()
	if carried := len(treeUIDs(decodedJSON(t, comp))); uids && carried == 0 {
		t.Fatal("the skeleton carries no uid, so the uids were not checked")
	} else if !uids && carried > 0 {
		t.Errorf("the skeleton carries %d uids, want none: its template requires no uid", carried)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("two seeded %s calls gave different skeletons, first at\n  %s\nand\n  %s", entry.name, aroundFirstDifference(first, second), aroundFirstDifference(second, first))
	}
}

// aroundFirstDifference returns the bytes of a around the first byte at
// which a and b differ, so a failure shows the member that differs
// without printing the whole skeleton.
func aroundFirstDifference(a, b []byte) string {
	i := 0
	for i < min(len(a), len(b)) && a[i] == b[i] {
		i++
	}
	return string(a[max(0, i-120):min(len(a), i+40)])
}

// TestREQ101_WithUIDSourceNilIsNoOption is the REQ-101 Uid source check
// that WithUIDSource(nil) leaves the generator's random fallback in force,
// as no WithUIDSource does: under value sources in the same state, a
// skeleton of requiredUIDsOPT built with it and one built without hold the
// same tree once the uids are masked, and their uids are version-4 UUIDs
// that differ between the two.
func TestREQ101_WithUIDSourceNilIsNoOption(t *testing.T) {
	t.Parallel()
	for _, entry := range skeletonEntryPoints {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			c := compileInline(t, requiredUIDsOPT)
			build := func(extra ...composition.Option) (tree any, uids []string) {
				opts := policyOptions(append([]composition.Option{
					composition.WithPolicy(instance.Large),
					composition.WithValueFill(instance.RandomFill),
					composition.WithValueSource(mrand.NewPCG(42, 1)),
				}, extra...)...)
				tree = decodedJSON(t, entry.build(t, c, opts...))
				for _, uid := range treeUIDs(tree) {
					uids = append(uids, uid.value)
				}
				maskTreeUIDs(tree)
				return tree, uids
			}
			withNil, nilUIDs := build(composition.WithUIDSource(nil))
			without, noOptionUIDs := build()
			if len(nilUIDs) == 0 {
				t.Fatal("the skeleton carries no uid, so nothing was checked")
			}
			for _, s := range nilUIDs {
				u, err := uuid.Parse(s)
				if err != nil || u[6]>>4 != 4 {
					t.Errorf("uid %q with WithUIDSource(nil), want a version-4 UUID from the random fallback", s)
				}
				if slices.Contains(noOptionUIDs, s) {
					t.Errorf("uid %q issued both with WithUIDSource(nil) and without WithUIDSource, want the random fallback's", s)
				}
			}
			if !reflect.DeepEqual(withNil, without) {
				t.Error("with uids masked, WithUIDSource(nil) and no WithUIDSource give different skeletons, want the same")
			}
		})
	}
}

// maskTreeUIDs replaces every uid member of a decoded tree, in place,
// with the same placeholder.
func maskTreeUIDs(v any) {
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			if k == "uid" {
				node[k] = "masked"
				continue
			}
			maskTreeUIDs(child)
		}
	case []any:
		for _, child := range node {
			maskTreeUIDs(child)
		}
	}
}
