package validation

// rmfloor_limit_internal_test.go: REQ-112 § Driver, REQ-171 § Surface — the
// walk's default visit limit sits far above what a decoded document needs,
// so it stops only a value built in Go that holds itself by value. Under a
// node limit the walk visits exactly that many values; under a depth limit
// it visits nothing below it.

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-112 — every corpus document the fixtures list is walked in under a
// tenth of the default visit limit, and none reports max_nodes. A fixture
// that comes within a tenth of the limit fails here, before a real document
// could reach it.
func TestDefaultLimits_REQ112_REQ171_FarAboveTheCorpus(t *testing.T) {
	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		t.Fatalf("ListCompositionJSON: %v", err)
	}
	walked := 0
	for _, rel := range rels {
		factory, ok := fixtures.FactoryForJSONRel(rel)
		if !ok {
			continue
		}
		body, err := os.ReadFile(fixtures.ResolveCompositionJSON(rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel.Rel, err)
		}
		root := factory()
		if err := canjson.Unmarshal(body, root); err != nil {
			continue // a decode failure is the codec's tests' concern
		}
		_, e := engineWalk(root, nil, Limits{})
		if e == nil {
			continue
		}
		walked++
		if e.stopped || e.depthStopped || e.visits*10 > e.maxNodes {
			t.Errorf("%s: the walk visits %d values (stopped: %t, depth limit reached: %t), want under a tenth of the default limit %d", rel.Rel, e.visits, e.stopped, e.depthStopped, e.maxNodes)
		}
	}
	if walked == 0 {
		t.Fatal("no corpus document was walked; the fixture listing found nothing")
	}
}

// REQ-171 § Surface — the walk visits exactly the number of values its node
// limit allows, then stops and reports max_nodes once, as its last issue.
func TestEngine_REQ171_VisitsExactlyTheNodeLimit(t *testing.T) {
	root := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "bottom"}}
	for range 100 {
		root = &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "level"}, Items: []rm.ContentItem{root}}
	}
	for _, n := range []int{1, 7, 100} {
		_, e := engineWalk(root, nil, Limits{Nodes: n})
		if !e.stopped || e.visits != n {
			t.Errorf("under Nodes %d the walk visits %d values (stopped: %t), want %d and stopped", n, e.visits, e.stopped, n)
		}
		issues := e.found.issues
		if len(e.stoppedAt) != 1 || len(issues) == 0 || issues[len(issues)-1].Code != "max_nodes" {
			t.Errorf("under Nodes %d the walk stopped at %q with issues %v, want one stop, reported last as max_nodes", n, e.stoppedAt, issues)
		}
	}
}

// REQ-171 § Surface, REQ-112 § Driver — the walk visits nothing below the
// depth limit, on the first branch that reaches it or any later one: under a
// depth of 3, twenty chains five deep and twenty chains fifty deep take the
// same number of visits, and report one stop.
func TestEngine_REQ171_VisitsNothingBelowTheDepthLimit(t *testing.T) {
	chains := func(levels int) *rm.Section {
		root := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "chains"}}
		for range 20 {
			chain := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "bottom"}}
			for range levels {
				chain = &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "level"}, Items: []rm.ContentItem{chain}}
			}
			root.Items = append(root.Items, chain)
		}
		return root
	}
	visits := map[int]int{}
	for _, levels := range []int{5, 50} {
		_, e := engineWalk(chains(levels), nil, Limits{Depth: 3})
		if len(e.stoppedAt) != 1 || e.stopped {
			t.Fatalf("chains %d deep under Depth 3: stopped at %q (visit limit reached: %t), want one depth stop", levels, e.stoppedAt, e.stopped)
		}
		visits[levels] = e.visits
	}
	if visits[5] != visits[50] {
		t.Errorf("under Depth 3 the walk visits %d values for chains 5 deep and %d for chains 50 deep, want the same: nothing below the limit is visited", visits[5], visits[50])
	}
}
