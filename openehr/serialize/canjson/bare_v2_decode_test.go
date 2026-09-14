package canjson_test

import (
	jsonv2 "encoding/json/v2"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"

	// Register the RM types (and their decode hooks) into the shared typereg
	// aggregate. The blank import is what a bare v2 caller would rely on for
	// the polymorphic slot to resolve.
	_ "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestBareV2UnmarshalResolvesPolymorphicSlot is ruling R6's pin: the
// per-interface decode hooks are threaded by the top-level type's
// UnmarshalJSONFrom through typereg.DecodeInto, so a polymorphic slot resolves
// from ANY entry point — including a bare encoding/json/v2 Unmarshal with no
// options and no canjson involvement. That is what makes the streaming pair
// green in this task, before canjson moves to v2 (Task 5): decoding a
// COMPOSITION cassette straight through v2 finds the concrete content[0] type.
//
// Can-fail control: the hooks reach the slot only because DecodeInto joins
// typereg.Unmarshalers() into every nested decode. Drop that join (or the
// hook registration) and content[0] comes back a nil interface, so the type
// assertion below fails.
func TestBareV2UnmarshalResolvesPolymorphicSlot(t *testing.T) {
	raw, err := os.ReadFile(fixtures.ResolveCompositionJSON(findCassette(t, "BMI.json")))
	if err != nil {
		t.Fatalf("read BMI cassette: %v", err)
	}

	var comp rm.Composition
	if err := jsonv2.Unmarshal(raw, &comp); err != nil {
		t.Fatalf("bare v2 Unmarshal(COMPOSITION) error: %v", err)
	}
	if len(comp.Content) == 0 {
		t.Fatal("decoded COMPOSITION has no content to resolve")
	}
	obs, ok := comp.Content[0].(*rm.Observation)
	if !ok {
		t.Fatalf("content[0] = %T, want *rm.Observation — the ContentItem hook did not fire through a bare v2 Unmarshal with no options", comp.Content[0])
	}
	if obs == nil {
		t.Fatal("content[0] is a typed-nil *rm.Observation")
	}
}

// findCassette returns the composition cassette whose relative path ends in
// name, failing the test if it is not among the vendored fixtures.
func findCassette(t *testing.T, name string) fixtures.CompositionJSONRel {
	t.Helper()
	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		t.Fatalf("list cassettes: %v", err)
	}
	for _, rel := range rels {
		if strings.HasSuffix(rel.Rel, "/"+name) || rel.Rel == name {
			return rel
		}
	}
	t.Fatalf("cassette %q not found among %d fixtures", name, len(rels))
	return fixtures.CompositionJSONRel{}
}
