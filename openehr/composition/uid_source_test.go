package composition_test

import (
	"bytes"
	"fmt"
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

// entryTypes are the ENTRY concretes, every one of which the generator
// gives a uid.
var entryTypes = []string{"OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION", "ADMIN_ENTRY"}

// treeUID is one uid of a decoded tree: the _type of the object that
// carries it, and its value, or "" for an ENTRY that carries none.
type treeUID struct {
	rmType string
	value  string
}

// treeUIDs returns every uid in a decoded tree, and an entry with an
// empty value for each ENTRY that carries none.
func treeUIDs(v any) []treeUID {
	var out []treeUID
	switch node := v.(type) {
	case map[string]any:
		rmType, _ := node["_type"].(string)
		uid, hasUID := node["uid"].(map[string]any)
		value, _ := uid["value"].(string)
		if hasUID || slices.Contains(entryTypes, rmType) {
			out = append(out, treeUID{rmType: rmType, value: value})
		}
		for _, child := range node {
			out = append(out, treeUIDs(child)...)
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
// and NewBuilder alike: on vital_signs under Large, every ENTRY carries a
// uid, and every uid in the skeleton is one the source issued.
func TestREQ101_WithUIDSourceGivesEveryUID(t *testing.T) {
	t.Parallel()
	for _, entry := range skeletonEntryPoints {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			uids := &issuedUIDs{}
			comp := entry.build(t, compileFixture(t, "vital_signs"),
				policyOptions(composition.WithPolicy(instance.Large), composition.WithUIDSource(uids.next))...)
			entries := 0
			for _, uid := range treeUIDs(decodedJSON(t, comp)) {
				if slices.Contains(entryTypes, uid.rmType) {
					entries++
				}
				if !slices.Contains(uids.issued, uid.value) {
					t.Errorf("%s uid = %q, want one WithUIDSource's source issued (issued %v)", uid.rmType, uid.value, uids.issued)
				}
			}
			if entries == 0 {
				t.Fatal("the skeleton holds no ENTRY, so nothing was checked")
			}
		})
	}
}

// seededSkeletonOptions are the options of the reproduction in issue 296:
// vital_signs under Large and RandomFill, with a fixed clock and value
// and uid sources each on a new PCG in a fixed state.
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
// state, give equal canonical JSON, uids included.
func TestREQ101_REQ107_SeededSkeletonIsReproducible(t *testing.T) {
	t.Parallel()
	for _, entry := range skeletonEntryPoints {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			c := compileFixture(t, "vital_signs")
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
			if !slices.ContainsFunc(treeUIDs(decodedJSON(t, comp)), func(u treeUID) bool { return u.value != "" }) {
				t.Fatal("the skeleton carries no uid, so the uids were not checked")
			}
			if !bytes.Equal(first, second) {
				t.Errorf("two seeded %s calls gave different skeletons, first at\n  %s\nand\n  %s", entry.name, aroundFirstDifference(first, second), aroundFirstDifference(second, first))
			}
		})
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
// skeleton built with it and one built without hold the same tree once
// the uids are masked, and their uids are version-4 UUIDs that differ
// between the two.
func TestREQ101_WithUIDSourceNilIsNoOption(t *testing.T) {
	t.Parallel()
	for _, entry := range skeletonEntryPoints {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			c := compileFixture(t, "vital_signs")
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
