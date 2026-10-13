package validation

// rmfloor_limit_internal_test.go: REQ-112 § Driver — the floor's visit limit
// sits far above what a decoded document needs, so it stops only a value
// built in Go that holds itself by value.

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-112 — every corpus document the fixtures list is walked in under a
// tenth of maxWalkNodes visits, and none reports max_nodes. A fixture that
// comes within a tenth of the limit fails here, before a real document
// could reach it.
func TestMaxWalkNodes_REQ112_FarAboveTheCorpus(t *testing.T) {
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
		w := &rmFloorWalker{info: rminfo.Default}
		w.walk(root, rmType, "/", roleNone, 0)
		walked++
		if w.stopped || w.visits*10 > maxWalkNodes {
			t.Errorf("%s: the floor visits %d values (stopped: %t), want under a tenth of the limit %d", rel.Rel, w.visits, w.stopped, maxWalkNodes)
		}
	}
	if walked == 0 {
		t.Fatal("no corpus document was walked; the fixture listing found nothing")
	}
}
