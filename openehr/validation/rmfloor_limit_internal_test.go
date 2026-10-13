package validation

// rmfloor_limit_internal_test.go: REQ-112 § Driver, REQ-171 § Surface — the
// floor's default visit limit sits far above what a decoded document needs,
// so it stops only a value built in Go that holds itself by value.

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
		rmType, _, ok := rmTypeInfo(root)
		if !ok {
			continue
		}
		w := newFloorWalker(nil, Limits{})
		w.walk(root, rmType, "/", roleNone, 0)
		walked++
		if w.stopped || w.depthStopped || w.visits*10 > w.maxNodes {
			t.Errorf("%s: the floor visits %d values (stopped: %t, depth limit reached: %t), want under a tenth of the default limit %d", rel.Rel, w.visits, w.stopped, w.depthStopped, w.maxNodes)
		}
	}
	if walked == 0 {
		t.Fatal("no corpus document was walked; the fixture listing found nothing")
	}
}

// REQ-171 § Surface — the floor walker visits exactly the number of values
// its node limit allows, then stops and reports max_nodes once.
func TestFloorWalker_REQ171_VisitsExactlyTheNodeLimit(t *testing.T) {
	root := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "bottom"}}
	for range 100 {
		root = &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "level"}, Items: []rm.ContentItem{root}}
	}
	for _, n := range []int{1, 7, 100} {
		w := newFloorWalker(nil, Limits{Nodes: n})
		w.walk(root, "SECTION", "/", roleNone, 0)
		if !w.stopped || w.visits != n {
			t.Errorf("under Nodes %d the walk visits %d values (stopped: %t), want %d and stopped", n, w.visits, w.stopped, n)
		}
		if len(w.stoppedAt) != 1 || len(w.issues) == 0 || w.issues[len(w.issues)-1].Code != "max_nodes" {
			t.Errorf("under Nodes %d the walk stopped at %q with issues %v, want one stop, reported last as max_nodes", n, w.stoppedAt, w.issues)
		}
	}
}
