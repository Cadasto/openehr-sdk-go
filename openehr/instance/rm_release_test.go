package instance_test

// REQ-107 — every ARCHETYPED the instance generator writes carries rm.Release
// as its rm_version.

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// archetypedRMVersions returns the rm_version of every ARCHETYPED in the
// canonical JSON of v, keyed by the path of the node that carries it.
func archetypedRMVersions(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := canjson.Marshal(v)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var tree any
	if err := json.Unmarshal(b, &tree); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	out := map[string]any{}
	var walk func(path string, n any)
	walk = func(path string, n any) {
		switch x := n.(type) {
		case map[string]any:
			if ad, ok := x["archetype_details"].(map[string]any); ok {
				out[path] = ad["rm_version"]
			}
			for k, c := range x {
				walk(path+"/"+k, c)
			}
		case []any:
			for i, c := range x {
				walk(path+"["+strconv.Itoa(i)+"]", c)
			}
		}
	}
	walk("", tree)
	return out
}

// TestREQ107_GeneratedArchetypedCarriesRMRelease — every ARCHETYPED the
// instance generator writes, on the template root and on each archetype root
// beneath it, carries the RM release the SDK is generated from.
func TestREQ107_GeneratedArchetypedCarriesRMRelease(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	out, err := instance.Generate(context.Background(), c, instance.Options{
		Policy:    instance.Example,
		Territory: "NL",
		Composer:  testComposer(),
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := archetypedRMVersions(t, out)
	if len(got) < 2 {
		t.Fatalf("generated composition carries %d archetype_details, want the root and at least one entry", len(got))
	}
	for path, v := range got {
		if v != rm.Release {
			t.Errorf("archetype_details at %q has rm_version %v, want rm.Release %q", path, v, rm.Release)
		}
	}
}

// slotNoIncludesOPT is the required-slot fixture with its include assertion
// removed, so the generator fills the slot with the RM-type-prefix fallback id.
var slotNoIncludesOPT = regexp.MustCompile(`(?s)\s*<includes>.*?</includes>`).
	ReplaceAllString(requiredSlotOnlyUnsynthesisableOPT, "")

// TestREQ107_SlotFillArchetypedCarriesRMRelease — the ARCHETYPED stamped on a
// generated slot fill carries rm.Release too.
func TestREQ107_SlotFillArchetypedCarriesRMRelease(t *testing.T) {
	c := compileSyntheticOPT(t, slotNoIncludesOPT)
	out, err := instance.Generate(context.Background(), c, instance.Options{
		Policy:    instance.Minimal,
		Territory: "NL",
		Composer:  testComposer(),
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	if len(comp.Content) != 1 {
		t.Fatalf("content has %d items, want the one slot fill", len(comp.Content))
	}
	obs, ok := comp.Content[0].(*rm.Observation)
	if !ok {
		t.Fatalf("content[0] is %T, want *rm.Observation", comp.Content[0])
	}
	if obs.ArchetypeDetails == nil {
		t.Fatal("slot fill carries no archetype_details")
	}
	if obs.ArchetypeDetails.RMVersion != rm.Release {
		t.Errorf("slot fill rm_version = %q, want rm.Release %q", obs.ArchetypeDetails.RMVersion, rm.Release)
	}
}
