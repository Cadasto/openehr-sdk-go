package canjson_test

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestBareV2UnmarshalResolvesPolymorphicSlot is ruling R6's pin: the
// per-interface decode hooks are threaded by the top-level type's
// UnmarshalJSONFrom through typereg.DecodeInto, so a polymorphic slot resolves
// from ANY entry point, including a bare encoding/json/v2 Unmarshal with no
// options and no canjson involvement. That independence is what lets a slot
// resolve the same way whether canjson (now itself on v2) or a bare v2 caller
// drives the decode: decoding a COMPOSITION cassette straight through v2 finds
// the concrete content[0] type.
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
		t.Fatalf("content[0] = %T, want *rm.Observation: the ContentItem hook did not fire through a bare v2 Unmarshal with no options", comp.Content[0])
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
// outermost. A caller hook for *rm.DVText must run both at ELEMENT.name (a
// direct DVText field) and inside the polymorphic ELEMENT.value slot.
//
// Before the fix, decodeOptions joined only the SDK aggregate, and because
// WithUnmarshalers is single-valued that join replaced the caller's hooks
// wholesale at every nested decode, so the caller's hook never reached the
// nested value.
//
// The hook tags each value it decodes with a unique marker, and the test
// asserts the marker on BOTH decoded values. That is what makes this a real
// control rather than a tautology: the SDK's own DVText.UnmarshalJSONFrom
// produces "hello" (no marker), so a bare `dv.Value == "hello"` check would
// pass even if the caller hook never reached the nested slot. Requiring the
// marker on ELEMENT.value proves the caller hook, not the SDK's, decoded it;
// requiring it on ELEMENT.name proves the hook is not merely firing once at the
// top. Can-fail control: drop the caller-join in decodeOptions and the nested
// value loses its marker.
func TestCallerUnmarshalersReachNestedSlot(t *testing.T) {
	const marker = "::caller-hook::"
	mine := jsonv2.UnmarshalFromFunc(func(dec *jsontext.Decoder, out *rm.DVText) error {
		// Decode the value into a shadow struct (no _type, no RM methods) so the
		// hook does not re-invoke itself; then populate the receiver, tagging the
		// value so the assertions can tell a caller-hook decode from an SDK one.
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
		out.Value = shadow.Value + marker
		return nil
	})

	const in = `{"_type":"ELEMENT","archetype_node_id":"at0",` +
		`"name":{"_type":"DV_TEXT","value":"n"},` +
		`"value":{"_type":"DV_TEXT","value":"hello"}}`
	var elem rm.Element
	if err := jsonv2.Unmarshal([]byte(in), &elem, jsonv2.WithUnmarshalers(mine)); err != nil {
		t.Fatalf("bare v2 Unmarshal(ELEMENT) with caller hook: %v", err)
	}
	if elem.Name == nil || elem.Name.GetValue() != "n"+marker {
		t.Errorf("elem.Name = %v, want a DVText with value %q — the caller hook did not decode the ELEMENT.name field", elem.Name, "n"+marker)
	}
	dv, ok := elem.Value.(*rm.DVText)
	if !ok {
		t.Fatalf("elem.Value = %T, want *rm.DVText decoded through the caller's hook", elem.Value)
	}
	if dv.Value != "hello"+marker {
		t.Errorf("elem.Value.Value = %q, want %q — the caller hook did not decode the nested ELEMENT.value slot (decodeOptions dropped its WithUnmarshalers there)", dv.Value, "hello"+marker)
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

// TestBareV2DecodeDuplicateMemberWrapsErrInvalidShape pins that a caller
// driving a generated UnmarshalJSONFrom through bare encoding/json/v2 — with no
// canjson entry point and no options — gets the same duplicate-member
// classification a canjson caller does (REQ-052): the refusal wraps
// ErrInvalidShape and stays reachable as jsontext.ErrDuplicateName. The decode
// is a single pass: the tokenizer refuses the duplicate as the value holding it
// is decoded, and the generated body decoding that value attaches the
// sentinel. For a concrete value (the top-level DV_TEXT, or the CODE_PHRASE
// nested in a DV_CODED_TEXT) that is typereg.classifyDecode at that level; for
// a value in a polymorphic slot (the ELEMENT's name) it is
// typereg.DecodePolymorphic's ReadValue, which applies the same gate.
//
// Each case isolates a gate, or deliberately does not. The CODE_PHRASE case
// isolates the concrete gate (classifyDecode): the duplicate sits in a concrete
// value no slot buffers. The top-level interface case (a bare rm.DataValue
// decoded with typereg.Unmarshalers()) isolates the slot gate: the hook's
// DecodePolymorphic reads the value with ReadValue, which refuses the duplicate
// before any concrete decode runs. The ELEMENT case is guarded by either gate:
// the slot's ReadValue refuses the duplicate inside name, and the concrete
// classifyDecode at the enclosing ELEMENT would classify it too, so it
// survives either single mutation.
//
// Can-fail controls (checked with go test -overlay on patched copies of
// streaming.go): revert classifyDecode's `return ClassifyDuplicate(err)` to
// `return err` and the ErrInvalidShape assertions go red for the top-level
// DV_TEXT and the nested CODE_PHRASE while the ErrDuplicateName ones stay green
// (the tokenizer still refuses the duplicate, only the SDK sentinel is lost).
// Revert DecodePolymorphic's `return ClassifyDuplicate(err)` at its ReadValue
// to `return err` and exactly the top-level interface case goes red.
func TestBareV2DecodeDuplicateMemberWrapsErrInvalidShape(t *testing.T) {
	cases := []struct {
		name string
		data string
		into func() any
		opts []jsonv2.Options
	}{
		{"top-level DV_TEXT", `{"_type":"DV_TEXT","value":"a","value":"b"}`, func() any { return new(rm.DVText) }, nil},
		{"nested concrete CODE_PHRASE in DV_CODED_TEXT", `{"_type":"DV_CODED_TEXT","value":"a","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"},"code_string":"at1","code_string":"at2"}}`, func() any { return new(rm.DVCodedText) }, nil},
		{"nested in ELEMENT", `{"_type":"ELEMENT","archetype_node_id":"at0","name":{"_type":"DV_TEXT","value":"a","value":"b"}}`, func() any { return new(rm.Element) }, nil},
		{"top-level interface rm.DataValue", `{"_type":"DV_TEXT","value":"a","value":"b"}`, func() any { return new(rm.DataValue) }, []jsonv2.Options{typereg.Unmarshalers()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := jsonv2.Unmarshal([]byte(tc.data), tc.into(), tc.opts...)
			if !errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("bare v2 Unmarshal(duplicate) err = %v; want errors.Is(_, canjson.ErrInvalidShape)", err)
			}
			if !errors.Is(err, jsontext.ErrDuplicateName) {
				t.Errorf("bare v2 Unmarshal(duplicate) err = %v; want errors.Is(_, jsontext.ErrDuplicateName)", err)
			}
		})
	}
}
