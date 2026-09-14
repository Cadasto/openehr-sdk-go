package serializeprobes

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestProbe030GuardCatchesDroppedFieldOnReEncode is the can-fail control for
// PROBE-030's typed deep comparison of A and B. The re-encode double drops
// `units` on the second encode only, so A still carries it and B does not. The
// mutation that turns this red is weakening the A-versus-B check in
// probe030RoundTrip (for example replacing reflect.DeepEqual with an
// always-equal comparison): then a lost field would slip through with
// Status == "pass".
func TestProbe030GuardCatchesDroppedFieldOnReEncode(t *testing.T) {
	body := []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`)
	r, err := probe030RoundTrip(body, func() any { return new(rm.DVQuantity) }, dropMemberReEncoder("units"))
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("status = %q, want fail: dropping units on the re-encode must make A and B differ", r.Status)
	}
	if !strings.Contains(r.Detail, "A and B differ") {
		t.Fatalf("detail = %q; want the typed deep comparison to fire first, so the A-versus-B guard is what caught it", r.Detail)
	}
}

// TestProbe030GuardCatchesNarrowedPolymorphicSlot is the can-fail control for a
// polymorphic slot narrowed on the re-encode path. The double deletes `_type`
// from ELEMENT.value, narrowing the DATA_VALUE slot toward its interface zero.
// The round trip must not report pass: either the third decode refuses the
// discriminator-less slot, or B decodes to a different concrete than A and the
// typed deep comparison flags it.
func TestProbe030GuardCatchesNarrowedPolymorphicSlot(t *testing.T) {
	body := []byte(`{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"n"},"value":{"_type":"DV_QUANTITY","magnitude":120,"units":"mm[Hg]"}}`)
	r, err := probe030RoundTrip(body, func() any { return new(rm.Element) }, stripSlotTypeReEncoder("value"))
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("status = %q (detail: %s), want fail: narrowing value._type must be caught", r.Status, r.Detail)
	}
}

// dropMemberReEncoder is a lossy re-encode double: it canjson-encodes the value
// and then removes one top-level member, standing in for a codec that drops a
// field on the re-encode path.
func dropMemberReEncoder(member string) func(any) ([]byte, error) {
	return func(v any) ([]byte, error) {
		b, err := canjson.Marshal(v)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		delete(m, member)
		return json.Marshal(m)
	}
}

// stripSlotTypeReEncoder is a lossy re-encode double: it canjson-encodes the
// value and then deletes `_type` from the named object-valued slot, standing in
// for a codec that narrows a polymorphic slot by dropping its discriminator.
func stripSlotTypeReEncoder(slot string) func(any) ([]byte, error) {
	return func(v any) ([]byte, error) {
		b, err := canjson.Marshal(v)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		if child, ok := m[slot].(map[string]any); ok {
			delete(child, "_type")
		}
		return json.Marshal(m)
	}
}
