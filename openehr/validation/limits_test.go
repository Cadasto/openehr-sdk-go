package validation_test

// limits_test.go: REQ-171 § Surface, REQ-112 § Driver — the limits that bound
// the floor's walk. The depth limit stops descent and is reported once per
// walk, at the first path where it stopped; the visit limit ends the walk and
// is reported once. Both are the Limits of a Validator, a zero or negative
// field reads as its default, and each path where a limit stopped the walk is
// listed in Coverage.Stopped.

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The limits a Validator built with no options applies.
const (
	defaultDepth = 256
	defaultNodes = 100_000
)

// nestedSections returns levels SECTIONs nested one inside the next through
// items, each with the node id at0001, around a bottom SECTION with no items.
// It is deep but finite and holds no cycle, so a walk with no depth limit
// still ends.
func nestedSections(levels int) *rm.Section {
	root := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "bottom"}}
	for range levels {
		root = &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "level"}, Items: []rm.ContentItem{root}}
	}
	return root
}

// firstDepthStop is the first path where a depth limit of depth stops the
// walk over SECTIONs nested through items, each item written as step: the
// name of the SECTION depth-1 levels down. The floor walks a SECTION's name
// before its items, and that name sits at depth, so the walk stops there
// before it reaches the SECTION depth levels down.
func firstDepthStop(step string, depth int) string {
	return strings.Repeat(step, depth-1) + "/name"
}

// wideCluster returns a CLUSTER holding n ELEMENTs side by side. The floor
// visits six values for each ELEMENT, so a cluster of 20,000 needs about
// 120,000 visits: more than the default visit limit, but finite, so a walk
// with no visit limit still ends.
func wideCluster(n int) *rm.Cluster {
	c := &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "wide"}}
	c.Items = make([]rm.Item, n)
	for i := range c.Items {
		c.Items[i] = &rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "e"}, Value: &rm.DVBoolean{Value: true}}
	}
	return c
}

// pathsOf returns the paths of the issues that carry code, in result order.
func pathsOf(issues []validation.Issue, code string) []string {
	var out []string
	for _, i := range issues {
		if i.Code == code {
			out = append(out, i.Path)
		}
	}
	return out
}

// limited returns a Validator with limits l.
func limited(l validation.Limits) *validation.Validator {
	return validation.New(validation.WithLimits(l))
}

// assertStoppedOnce fails the test unless r reports code once, at want, lists
// that path alone in Coverage.Stopped, and is neither OK nor complete.
func assertStoppedOnce(t *testing.T, call string, r validation.Result, code, want string) {
	t.Helper()
	if got := pathsOf(r.Issues, code); len(got) != 1 || got[0] != want {
		t.Errorf("%s reports %s at %q, want once, at %q", call, code, got, want)
	}
	if !slices.Equal(r.Coverage.Stopped, []string{want}) {
		t.Errorf("%s Coverage.Stopped = %q, want [%q]", call, r.Coverage.Stopped, want)
	}
	if r.Coverage.Complete() {
		t.Errorf("%s Coverage.Complete() = true, want false: a limit stopped the walk", call)
	}
	if r.OK {
		t.Errorf("%s OK = true, want false: %s is an error", call, code)
	}
}

// REQ-171 Acceptance, REQ-112 § Driver — WithLimits with a depth below what a
// root needs stops the walk there: max_depth once, at the first path where
// the walk stopped, and that path in Coverage.Stopped. The same root under
// the default limit is walked to the bottom.
func TestValidator_REQ171_REQ112_DepthLimitStopsTheWalk(t *testing.T) {
	root := nestedSections(40)
	for _, depth := range []int{1, 5, 30} {
		r := limited(validation.Limits{Depth: depth}).Validate(root, nil)
		assertStoppedOnce(t, fmt.Sprintf("Validate(40 nested SECTIONs) under Depth %d", depth), r, "max_depth", firstDepthStop("/items[at0001]", depth))
		if n := len(pathsOf(r.Issues, "max_nodes")); n != 0 {
			t.Errorf("Validate(40 nested SECTIONs) under Depth %d reports max_nodes %d times, want none", depth, n)
		}
	}
	r := validation.New().Validate(root, nil)
	if !r.OK || len(r.Issues) != 0 || !r.Coverage.Complete() {
		t.Errorf("New().Validate(40 nested SECTIONs) OK = %t, issues %q, coverage %+v; want OK, none and complete", r.OK, issueKeys(r.Issues), r.Coverage)
	}
}

// REQ-171 § One walk — the depth limit bounds the walk with a template as
// without one: below the device CLUSTER of vital_signs.opt, which fills a
// slot, 300 nested CLUSTERs stop the walk at the default depth, reported
// once, as max_depth at the first path where it stopped, which
// Coverage.Stopped lists.
func TestValidator_REQ171_DepthLimitStopsTheWalkWithATemplate(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	tree, ok := bpObservation(t, comp).Protocol.(*rm.ItemTree)
	if !ok || len(tree.Items) != 1 {
		t.Fatalf("fixture protocol is %T, want an ITEM_TREE holding the device CLUSTER", bpObservation(t, comp).Protocol)
	}
	device, ok := tree.Items[0].(*rm.Cluster)
	if !ok {
		t.Fatalf("fixture protocol item is %T, want *rm.Cluster", tree.Items[0])
	}
	deep := &rm.Cluster{ArchetypeNodeID: "at0098", Name: rm.DVText{Value: "bottom"}}
	for range 300 {
		deep = &rm.Cluster{ArchetypeNodeID: "at0098", Name: rm.DVText{Value: "level"}, Items: []rm.Item{deep}}
	}
	device.Items = append(device.Items, deep)

	r := validation.Validate(comp, c)
	got := pathsOf(r.Issues, "max_depth")
	const nested = bloodPressurePath + "/protocol/items[openEHR-EHR-CLUSTER.device.v1]/items[at0098]"
	if len(got) != 1 || !strings.HasPrefix(got[0], nested+"/") {
		t.Fatalf("Validate(device CLUSTER holding 300 nested CLUSTERs) reports max_depth at %q, want once, below the device CLUSTER", got)
	}
	assertStoppedOnce(t, "Validate(device CLUSTER holding 300 nested CLUSTERs)", r, "max_depth", got[0])
}

// REQ-171 Acceptance, REQ-112 § Driver — WithLimits with a node count below
// what a root needs stops the walk there: max_nodes once, as the last issue,
// and its path in Coverage.Stopped. It bounds the floor inside a
// template-driven validation too. The same roots under the default limit
// are walked to the end.
func TestValidator_REQ171_REQ112_NodeLimitStopsTheWalk(t *testing.T) {
	bmi, _, _, _ := bmiSkeleton(t)
	c := mustCompile(t, "BMI")
	tests := []struct {
		name string
		root any
		c    *templatecompile.Compiled
	}{
		{name: "40 nested SECTIONs, no template", root: nestedSections(40)},
		{name: "BMI skeleton, no template", root: bmi},
		{name: "BMI skeleton, BMI template", root: bmi, c: c},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := limited(validation.Limits{Nodes: 10}).Validate(tc.root, tc.c)
			got := pathsOf(r.Issues, "max_nodes")
			if len(got) != 1 {
				t.Fatalf("Validate(%s) under Nodes 10 reports max_nodes %d times, want once; issues %q", tc.name, len(got), issueKeys(r.Issues))
			}
			assertStoppedOnce(t, "Validate("+tc.name+") under Nodes 10", r, "max_nodes", got[0])
			if last := r.Issues[len(r.Issues)-1]; last.Code != "max_nodes" {
				t.Errorf("Validate(%s) under Nodes 10 ends with %s at %q, want max_nodes: the limit ends the walk", tc.name, last.Code, last.Path)
			}
			full := validation.New().Validate(tc.root, tc.c)
			if !full.OK || !full.Coverage.Complete() {
				t.Errorf("New().Validate(%s) OK = %t, coverage %+v; want OK and complete", tc.name, full.OK, full.Coverage)
			}
		})
	}
}

// REQ-171 Acceptance — a Limits field that is zero or negative reads as its
// default, so no option disables a limit. A root deeper than the default
// depth stops at the default's first path, and one that needs more visits
// than the default stops where the default stops it. Were zero to disable the
// limit, neither would stop; were it read as a limit of zero, both would stop
// at the root.
func TestValidator_REQ171_ZeroOrNegativeLimitIsTheDefault(t *testing.T) {
	deep := nestedSections(300)
	for _, l := range []validation.Limits{{}, {Depth: 0, Nodes: 1_000_000}, {Depth: -1}, {Depth: math.MinInt}} {
		r := limited(l).Validate(deep, nil)
		assertStoppedOnce(t, fmt.Sprintf("Validate(300 nested SECTIONs) under %+v", l), r, "max_depth", firstDepthStop("/items[at0001]", defaultDepth))
	}

	wide := wideCluster(20_000)
	if r := limited(validation.Limits{Nodes: 1_000_000}).Validate(wide, nil); len(pathsOf(r.Issues, "max_nodes")) != 0 {
		t.Fatalf("precondition: Validate(20,000 ELEMENTs) under Nodes 1,000,000 reports max_nodes; want the cluster finite under a high limit")
	}
	def := limited(validation.Limits{Nodes: defaultNodes}).Validate(wide, nil)
	if len(pathsOf(def.Issues, "max_nodes")) != 1 {
		t.Fatalf("precondition: Validate(20,000 ELEMENTs) under Nodes %d reports max_nodes %d times, want once: the cluster needs more than the default", defaultNodes, len(pathsOf(def.Issues, "max_nodes")))
	}
	for _, l := range []validation.Limits{{}, {Nodes: 0, Depth: 1_000}, {Nodes: -1}, {Nodes: math.MinInt}} {
		r := limited(l).Validate(wide, nil)
		if !reflect.DeepEqual(r, def) {
			t.Errorf("Validate(20,000 ELEMENTs) under %+v: max_nodes at %q, Stopped %q; want the default's, at %q", l, pathsOf(r.Issues, "max_nodes"), r.Coverage.Stopped, def.Coverage.Stopped)
		}
	}
	var zero validation.Validator
	for name, r := range map[string]validation.Result{"New()": validation.New().Validate(wide, nil), "Validator{}": zero.Validate(wide, nil)} {
		if !reflect.DeepEqual(r, def) {
			t.Errorf("%s.Validate(20,000 ELEMENTs): max_nodes at %q, Stopped %q; want the default's, at %q", name, pathsOf(r.Issues, "max_nodes"), r.Coverage.Stopped, def.Coverage.Stopped)
		}
	}
}

// REQ-171 — New takes its options in order: a nil option is ignored, and a
// later WithLimits replaces an earlier one whole, its zero fields standing
// for their defaults.
func TestNew_REQ171_OptionsInOrder(t *testing.T) {
	root := nestedSections(40)
	tests := []struct {
		name string
		v    *validation.Validator
		want []string // the paths of max_depth
	}{
		{name: "a nil option", v: validation.New(nil)},
		{name: "a nil option among limits", v: validation.New(validation.WithLimits(validation.Limits{Depth: 5}), nil), want: []string{firstDepthStop("/items[at0001]", 5)}},
		{
			name: "the later depth wins",
			v:    validation.New(validation.WithLimits(validation.Limits{Depth: 5}), validation.WithLimits(validation.Limits{Depth: 10})),
			want: []string{firstDepthStop("/items[at0001]", 10)},
		},
		{
			name: "the later limits replace the earlier whole",
			v:    validation.New(validation.WithLimits(validation.Limits{Depth: 5}), validation.WithLimits(validation.Limits{Nodes: 1_000_000})),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.v.Validate(root, nil)
			if got := pathsOf(r.Issues, "max_depth"); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(40 nested SECTIONs) reports max_depth at %q, want %q", got, tc.want)
			}
		})
	}
}

// brokenChain returns nestedSections(levels) whose bottom SECTION holds an
// OBSERVATION that lacks every attribute the RM makes mandatory on it, so a
// walk that reaches the bottom reports several issues there.
func brokenChain(levels int) *rm.Section {
	root := nestedSections(levels)
	bottom := root
	for range levels {
		bottom = bottom.Items[0].(*rm.Section)
	}
	bottom.Items = []rm.ContentItem{&rm.Observation{ArchetypeNodeID: "at0099", Name: rm.DVText{Value: "broken"}}}
	return root
}

// REQ-112 § Driver, REQ-171 § One walk — past the depth limit the walk does
// not descend, and it reports max_depth once per walk, at the first path where
// it stopped, not once for each node at that depth. A SECTION held by value
// that holds itself through its own list reaches the limit on every branch:
// before max_nodes ended the walk it reported 33,006 max_depth issues. A
// fan-out of 50 SECTIONs just above the limit reaches it 100 times, and 20
// chains under a depth of 3 reach it on each chain. Every chain of those two
// ends below the limit in an OBSERVATION that breaks RM rules, so a walk that
// descends past the limit on any branch, the first or a later one, reports
// more than the one max_depth.
func TestValidateRM_REQ112_REQ171_MaxDepthOncePerWalk(t *testing.T) {
	if r := validation.ValidateRM(brokenChain(3)); len(r.Issues) < 2 {
		t.Fatalf("precondition: ValidateRM(chain ending in a broken OBSERVATION) issues = %q, want several", issueKeys(r.Issues))
	}
	fan := nestedSections(defaultDepth - 2)
	bottom := fan
	for range defaultDepth - 2 {
		bottom = bottom.Items[0].(*rm.Section)
	}
	for range 50 {
		bottom.Items = append(bottom.Items, brokenChain(3))
	}
	chains := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "chains"}}
	for range 20 {
		chains.Items = append(chains.Items, brokenChain(5))
	}
	tests := []struct {
		name     string
		validate func(root any) validation.Result
		root     any
		first    string // the path of the one max_depth
	}{
		{
			name:     "50 SECTIONs, each 3 deep, at depth 255",
			validate: validation.ValidateRM,
			root:     fan,
			// The 50 SECTIONs share at0001, so each is written by its
			// position; the first one's name is the first node at the limit.
			first: strings.Repeat("/items[at0001]", defaultDepth-2) + "/items[1]/name",
		},
		{
			name:     "20 chains, each 5 deep, under Depth 3",
			validate: func(root any) validation.Result { return limited(validation.Limits{Depth: 3}).Validate(root, nil) },
			root:     chains,
			first:    "/items[1]/items[at0001]/name",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.validate(tc.root)
			if got := pathsOf(r.Issues, "max_depth"); len(got) != 1 || got[0] != tc.first {
				t.Fatalf("Validate(%s) reports max_depth %d times, first at %q; want once, at %q", tc.name, len(got), firstOf(got), tc.first)
			}
			if len(r.Issues) != 1 {
				t.Errorf("Validate(%s) issues = %q, want the max_depth alone: nothing below the limit is walked, on any branch", tc.name, issueKeys(r.Issues))
			}
			if !slices.Equal(r.Coverage.Stopped, []string{tc.first}) {
				t.Errorf("Validate(%s) Coverage.Stopped = %q, want [%q]", tc.name, r.Coverage.Stopped, tc.first)
			}
		})
	}

	t.Run("SECTION holding itself by value", func(t *testing.T) {
		r := validateWithin(t, "ValidateRM", visitLimitDeadline, func() validation.Result { return validation.ValidateRM(selfSharingSection()) })
		// Both items are copies of the section, so each is written by its
		// position.
		first := firstDepthStop("/items[1]", defaultDepth)
		if got := pathsOf(r.Issues, "max_depth"); len(got) != 1 || got[0] != first {
			t.Fatalf("ValidateRM(SECTION holding itself by value) reports max_depth %d times, first at %q; want once, at %q", len(got), firstOf(got), first)
		}
		if !slices.Contains(r.Coverage.Stopped, first) {
			t.Errorf("ValidateRM(SECTION holding itself by value) Coverage.Stopped = %q, want it to hold %q", r.Coverage.Stopped, first)
		}
	})
}

// firstOf returns the first of paths, or "" when there is none.
func firstOf(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}
