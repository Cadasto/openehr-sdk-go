package canjson_test

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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

// TestCallerUnmarshalersReachNestedSlot pins Important 2: a caller-supplied
// json.WithUnmarshalers must reach every value the SDK decodes, not just the
// outermost. A caller hook for *rm.DVText fires when a DVText decodes inside the
// polymorphic ELEMENT.value slot.
//
// Before the fix, decodeOptions joined only the SDK aggregate, and because
// WithUnmarshalers is single-valued that join replaced the caller's hooks
// wholesale at every nested decode — so this test was red: the marker never
// fired and the value decoded through DVText's own UnmarshalJSONFrom.
func TestCallerUnmarshalersReachNestedSlot(t *testing.T) {
	var fired bool
	mine := jsonv2.UnmarshalFromFunc(func(dec *jsontext.Decoder, out *rm.DVText) error {
		fired = true
		// Decode the value into a shadow struct (no _type, no RM methods) so the
		// hook does not re-invoke itself; then populate the receiver.
		var shadow struct {
			Value string `json:"value"`
		}
		raw, err := dec.ReadValue()
		if err != nil {
			return err
		}
		if err := jsonv2.Unmarshal(raw, &shadow); err != nil {
			return err
		}
		out.Value = shadow.Value
		return nil
	})

	const in = `{"_type":"ELEMENT","archetype_node_id":"at0",` +
		`"name":{"_type":"DV_TEXT","value":"n"},` +
		`"value":{"_type":"DV_TEXT","value":"hello"}}`
	var elem rm.Element
	if err := jsonv2.Unmarshal([]byte(in), &elem, jsonv2.WithUnmarshalers(mine)); err != nil {
		t.Fatalf("bare v2 Unmarshal(ELEMENT) with caller hook: %v", err)
	}
	if !fired {
		t.Fatal("caller hook did not fire — decodeOptions dropped the caller's WithUnmarshalers at the nested DATA_VALUE slot")
	}
	dv, ok := elem.Value.(*rm.DVText)
	if !ok {
		t.Fatalf("elem.Value = %T, want *rm.DVText decoded through the caller's hook", elem.Value)
	}
	if dv.Value != "hello" {
		t.Errorf("elem.Value.Value = %q, want %q", dv.Value, "hello")
	}
}

// TestMalformedPolymorphicSlotErrorIsClean pins Minor 4: a non-object at a
// polymorphic slot is a shape failure whose message must name the interface,
// not leak a bare `: :` (empty RM type) or an anonymous Go struct spelling from
// the discriminator peek.
func TestMalformedPolymorphicSlotErrorIsClean(t *testing.T) {
	const in = `{"_type":"ELEMENT","archetype_node_id":"at0",` +
		`"name":{"_type":"DV_TEXT","value":"n"},"value":[1,2]}`
	err := canjson.Unmarshal([]byte(in), &rm.Element{})
	if err == nil {
		t.Fatal("Unmarshal(array at DATA_VALUE slot) = nil; want a shape error")
	}
	msg := err.Error()
	if strings.Contains(msg, ": :") {
		t.Errorf("error message leaks a bare colon (empty RM type): %s", msg)
	}
	if strings.Contains(msg, "struct {") {
		t.Errorf("error message leaks an anonymous Go struct spelling: %s", msg)
	}
	if !errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; want errors.Is(_, canjson.ErrInvalidShape)", err)
	}
}
