package validation_test

// engine_walk_test.go: REQ-171 § One walk — the rules of the one walk that
// the floor does not share. A value held from two places is validated at
// each, so a defect in it is reported at each path; a value that holds itself
// is a cycle, detected on the active ancestry alone, reported where it recurs
// and not entered; a value reached again outside its own ancestry is no
// cycle. The depth and visit limits each stop the walk and are reported once,
// with the path in Coverage.Stopped, and a zero or negative limit field reads
// as its default.

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// walkEngine runs the one walk with no template within l, failing the test
// when it has not returned within deadline.
func walkEngine(t *testing.T, deadline time.Duration, root any, l validation.Limits) validation.Result {
	t.Helper()
	return validateWithin(t, "the engine", deadline, func() validation.Result { return validation.RunEngine(root, nil, l) })
}

// sharedClusterGraph returns a CLUSTER that holds the next level twice, levels
// deep, around a leaf CLUSTER whose items are present and empty. Every level
// holds one pointer from two places, so the leaf is reached by 2^levels
// paths, none of them inside the leaf.
func sharedClusterGraph(levels int) *rm.Cluster {
	root := &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "leaf"}, Items: []rm.Item{}}
	for range levels {
		root = &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "level"}, Items: []rm.Item{root, root}}
	}
	return root
}

// REQ-171 § One walk, § Acceptance — a Go value with one defect, held from
// two places, draws two issues, one at each path; the floor, which walks a
// shared value once, draws one. The places are two items of one list, two
// single-valued attributes, and two depths.
func TestREQ171_EngineValidatesASharedValueAtEachPlace(t *testing.T) {
	t.Parallel()
	emptyItems := &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "leaf"}, Items: []rm.Item{}}
	noValue := &rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "e"}}
	ref := &rm.PartyRef{ID: &rm.HierObjectID{Value: "a3e8b1c2-0d4f-4e5a-9b6c-7d8e9f0a1b2c"}, Type: "PERSON"} // namespace missing
	tests := []struct {
		name string
		root any
		code string
		want []string // the paths of the issues with code
	}{
		{
			name: "a CLUSTER whose items are empty, held twice in one list",
			root: &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "root"}, Items: []rm.Item{emptyItems, emptyItems}},
			code: "cardinality",
			want: []string{"/items[1]/items", "/items[2]/items"},
		},
		{
			name: "an ELEMENT with neither value nor null_flavour, held at two depths",
			root: &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "root"}, Items: []rm.Item{
				noValue,
				&rm.Cluster{ArchetypeNodeID: "at0003", Name: rm.DVText{Value: "inner"}, Items: []rm.Item{noValue}},
			}},
			code: "rm_invariant",
			want: []string{"/items[at0002]", "/items[at0003]/items[at0002]"},
		},
		{
			name: "a PARTY_REF with no namespace, held by two performers",
			root: &rm.EventContext{
				StartTime: rm.DVDateTime{Value: "2026-10-12T09:00:00Z"},
				Setting: rm.DVCodedText{
					DVText:       rm.DVText{Value: "home"},
					DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "225"},
				},
				Participations: []rm.Participation{
					{Function: &rm.DVText{Value: "first"}, Performer: &rm.PartySelf{ExternalRef: ref}},
					{Function: &rm.DVText{Value: "second"}, Performer: &rm.PartySelf{ExternalRef: ref}},
				},
			},
			code: "rm_invariant",
			want: []string{"/participations[1]/performer/external_ref/namespace", "/participations[2]/performer/external_ref/namespace"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pathsOf(validation.ValidateRM(tc.root).Issues, tc.code); len(got) != 1 {
				t.Fatalf("precondition: ValidateRM(%s) reports %s at %q, want once: the floor walks a shared value once", tc.name, tc.code, got)
			}
			r := walkEngine(t, walkDeadline, tc.root, validation.Limits{})
			if got := issueKeys(r.Issues); len(got) != len(tc.want) {
				t.Errorf("engine(%s) issues = %q, want %d, the one defect at each place", tc.name, got, len(tc.want))
			}
			if got := pathsOf(r.Issues, tc.code); !slices.Equal(got, tc.want) {
				t.Errorf("engine(%s) reports %s at %q, want at %q: one issue at each place", tc.name, tc.code, got, tc.want)
			}
		})
	}
}

// REQ-171 § One walk — a value that contains itself is a cycle: the walk
// reports cyclic_value at each path where it recurs, does not enter it there,
// so nothing below the recurrence is reported, and ends.
func TestREQ171_EngineReportsACycleWhereItRecurs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		root func() any
		want []string
	}{
		{
			name: "SECTION holding itself twice",
			root: func() any {
				s := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "s"}}
				s.Items = []rm.ContentItem{s, s}
				return s
			},
			want: []string{"cyclic_value /items[1]", "cyclic_value /items[2]"},
		},
		{
			name: "two CLUSTERs holding each other",
			root: func() any {
				a := &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "a"}}
				b := &rm.Cluster{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "b"}, Items: []rm.Item{a}}
				a.Items = []rm.Item{b}
				return a
			},
			want: []string{"cyclic_value /items[at0002]/items[at0001]"},
		},
		{
			name: "DV_CODED_TEXT reached again through a single-valued attribute",
			root: func() any {
				// openehr::671 is in the term mapping purpose group, so the
				// text is a valid purpose for the mapping it carries.
				text := &rm.DVCodedText{
					Value:        "research study",
					DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "671"},
				}
				target := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "271649006"}
				text.Mappings = []rm.TermMapping{{Match: rm.Character("="), Target: target, Purpose: text}}
				return text
			},
			want: []string{"cyclic_value /mappings[1]/purpose"},
		},
		{
			name: "SECTION with no name holding itself",
			root: func() any {
				s := &rm.Section{ArchetypeNodeID: "at0001"}
				s.Items = []rm.ContentItem{s}
				return s
			},
			// The missing name is reported once, at the root: the
			// recurrence is not entered, so not at /items[at0001]/name.
			want: []string{"cyclic_value /items[at0001]", "required /name"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := walkEngine(t, walkDeadline, tc.root(), validation.Limits{})
			if got := issueKeys(r.Issues); strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("engine(%s) issues = %q, want %q", tc.name, got, tc.want)
			}
			if len(r.Coverage.Stopped) != 0 {
				t.Errorf("engine(%s) Coverage.Stopped = %q, want none: the cycle ends the walk there, not a limit", tc.name, r.Coverage.Stopped)
			}
		})
	}
}

// REQ-171 § One walk — a value reached again outside its own ancestry is no
// cycle: the walk checks a value against the values it is inside, never
// against those it has finished. A valid CLUSTER held three times, once below
// a sibling, draws no issue at all.
func TestREQ171_EngineSharedValueIsNoCycle(t *testing.T) {
	t.Parallel()
	shared := &rm.Cluster{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "shared"}, Items: []rm.Item{
		&rm.Element{ArchetypeNodeID: "at0003", Name: rm.DVText{Value: "e"}, Value: &rm.DVBoolean{Value: true}},
	}}
	root := &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "root"}, Items: []rm.Item{
		shared,
		&rm.Cluster{ArchetypeNodeID: "at0004", Name: rm.DVText{Value: "sibling"}, Items: []rm.Item{shared}},
		shared,
	}}
	r := walkEngine(t, walkDeadline, root, validation.Limits{})
	if len(r.Issues) != 0 {
		t.Errorf("engine(CLUSTER held three times) issues = %q, want none: a value reached outside its own ancestry is no cycle", issueKeys(r.Issues))
	}
}

// REQ-171 § One walk — at the depth limit the walk stops descending and
// reports max_depth once per walk, at the first path where it stopped, with
// that path alone in Coverage.Stopped: on one chain at several depths, and on
// twenty chains, each of which reaches the limit, under a depth of 3.
func TestREQ171_EngineDepthLimitStopsTheWalk(t *testing.T) {
	t.Parallel()
	root := nestedSections(40)
	for _, depth := range []int{1, 5, 30} {
		r := walkEngine(t, walkDeadline, root, validation.Limits{Depth: depth})
		assertStoppedOnce(t, fmt.Sprintf("engine(40 nested SECTIONs) under Depth %d", depth), r, "max_depth", firstDepthStop("/items[at0001]", depth))
		if n := len(pathsOf(r.Issues, "max_nodes")); n != 0 {
			t.Errorf("engine(40 nested SECTIONs) under Depth %d reports max_nodes %d times, want none", depth, n)
		}
	}

	chains := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "chains"}}
	for range 20 {
		chains.Items = append(chains.Items, brokenChain(5))
	}
	r := walkEngine(t, walkDeadline, chains, validation.Limits{Depth: 3})
	const first = "/items[1]/items[at0001]/name"
	assertStoppedOnce(t, "engine(20 chains, each 5 deep) under Depth 3", r, "max_depth", first)
	if len(r.Issues) != 1 {
		t.Errorf("engine(20 chains, each 5 deep) under Depth 3 issues = %q, want the max_depth alone: nothing below the limit is walked, on any branch", issueKeys(r.Issues))
	}
}

// REQ-171 § One walk — at the visit limit the walk stops and reports
// max_nodes once, as its last issue, at the path where it stopped, which
// Coverage.Stopped lists. Over a tree that shares no value the engine stops
// where the floor stops. A graph that shares one value at each of 40 levels
// reaches its leaf by 2^40 paths; the engine validates it at each, so only
// the visit limit ends the walk.
func TestREQ171_EngineNodeLimitStopsTheWalk(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		root any
		l    validation.Limits
	}{
		{name: "40 nested SECTIONs under Nodes 1", root: nestedSections(40), l: validation.Limits{Nodes: 1}},
		{name: "40 nested SECTIONs under Nodes 7", root: nestedSections(40), l: validation.Limits{Nodes: 7}},
		{name: "40 nested SECTIONs under Nodes 100", root: nestedSections(40), l: validation.Limits{Nodes: 100}},
		{name: "20,000 ELEMENTs under the default", root: wideCluster(20_000)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := walkEngine(t, visitLimitDeadline, tc.root, tc.l)
			floor := limited(tc.l).Validate(tc.root, nil)
			want := pathsOf(floor.Issues, "max_nodes")
			if len(want) != 1 {
				t.Fatalf("precondition: Validate(%s, nil) reports max_nodes at %q, want once", tc.name, want)
			}
			assertStoppedOnce(t, "engine("+tc.name+")", r, "max_nodes", want[0])
			assertStoppedAtMaxNodes(t, "engine("+tc.name+")", r)
		})
	}

	t.Run("a value shared at each of 40 levels", func(t *testing.T) {
		r := walkEngine(t, visitLimitDeadline, sharedClusterGraph(40), validation.Limits{})
		assertStoppedAtMaxNodes(t, "engine(a value shared at each of 40 levels)", r)
		if got := pathsOf(r.Issues, "max_nodes"); !slices.Equal(r.Coverage.Stopped, got) {
			t.Errorf("engine(a value shared at each of 40 levels) Coverage.Stopped = %q, want the max_nodes path %q", r.Coverage.Stopped, got)
		}
		if containsCode(r.Issues, "cyclic_value") {
			t.Errorf("engine(a value shared at each of 40 levels) reports cyclic_value, want none: no value holds itself; issues %q", issueKeys(r.Issues))
		}
		if n := countCode(r.Issues, "cardinality"); n < 2 {
			t.Errorf("engine(a value shared at each of 40 levels) reports the leaf's cardinality %d times before the limit, want it at each place reached", n)
		}
	})
}

// REQ-171 § Surface — a zero or negative limit field reads as its default:
// the depth limit stops 300 nested SECTIONs where the default depth does, and
// the visit limit stops 20,000 ELEMENTs where the default count does, never
// lets a walk run unbounded.
func TestREQ171_EngineZeroOrNegativeLimitIsTheDefault(t *testing.T) {
	t.Parallel()
	deep := nestedSections(300)
	for _, l := range []validation.Limits{{}, {Depth: 0, Nodes: 1_000_000}, {Depth: -1}, {Depth: math.MinInt}} {
		r := walkEngine(t, walkDeadline, deep, l)
		assertStoppedOnce(t, fmt.Sprintf("engine(300 nested SECTIONs) under %+v", l), r, "max_depth", firstDepthStop("/items[at0001]", defaultDepth))
	}

	wide := wideCluster(20_000)
	def := walkEngine(t, visitLimitDeadline, wide, validation.Limits{Nodes: defaultNodes})
	if n := len(pathsOf(def.Issues, "max_nodes")); n != 1 {
		t.Fatalf("precondition: engine(20,000 ELEMENTs) under Nodes %d reports max_nodes %d times, want once: the cluster needs more than the default", defaultNodes, n)
	}
	for _, l := range []validation.Limits{{}, {Nodes: 0, Depth: 1_000}, {Nodes: -1}, {Nodes: math.MinInt}} {
		r := walkEngine(t, visitLimitDeadline, wide, l)
		if !reflect.DeepEqual(r, def) {
			t.Errorf("engine(20,000 ELEMENTs) under %+v: max_nodes at %q, Stopped %q; want the default's, at %q", l, pathsOf(r.Issues, "max_nodes"), r.Coverage.Stopped, def.Coverage.Stopped)
		}
	}
}
