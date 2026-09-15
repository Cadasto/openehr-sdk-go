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
	r, err := probe030RoundTrip(body, func() any { return new(rm.DVQuantity) }, dropMemberReEncoder("units"), false)
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
// polymorphic slot rewritten to a different concrete on the re-encode path. The
// double replaces ELEMENT.value (a DV_QUANTITY) with a decodable DV_TEXT, so B
// decodes cleanly but holds a different dynamic type in the slot than A. B is
// still RM-valid, so only the typed deep comparison of A and B can catch it,
// which is what this pins. Replacing that comparison with an always-true one
// turns this test red: the wire-equivalence secondary then fires with a
// different detail, so the "A and B differ" assertion below fails.
func TestProbe030GuardCatchesNarrowedPolymorphicSlot(t *testing.T) {
	body := []byte(`{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"n"},"value":{"_type":"DV_QUANTITY","magnitude":120,"units":"mm[Hg]"}}`)
	double := retypeSlotReEncoder("value", map[string]any{"_type": "DV_TEXT", "value": "120 mm[Hg]"})
	r, err := probe030RoundTrip(body, func() any { return new(rm.Element) }, double, false)
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("status = %q (detail: %s), want fail: retyping value to a different concrete must be caught", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "A and B differ") {
		t.Fatalf("detail = %q; want the typed deep comparison to fire, so the A-versus-B guard is what caught the changed dynamic type", r.Detail)
	}
}

// TestProbe030SkipFloorKeysAllMatchAnInput pins that every probe030SkipFloor key
// names a cassette that is actually in Probe030Inputs. The lookup at
// loadCassetteInputs is a plain map index that never reports a miss, so a dead
// key (a typo, or a cassette removed from the corpus) would silently hold
// nothing out. The count of inputs carrying SkipFloor must equal
// len(probe030SkipFloor); adding a key that matches no input turns this red.
func TestProbe030SkipFloorKeysAllMatchAnInput(t *testing.T) {
	var held int
	for _, in := range Probe030Inputs {
		if in.SkipFloor {
			held++
		}
	}
	if held != len(probe030SkipFloor) {
		t.Errorf("inputs with SkipFloor = %d, want %d (len(probe030SkipFloor)); a key that matches no cassette is dead and holds nothing out", held, len(probe030SkipFloor))
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

// retypeSlotReEncoder is a lossy re-encode double: it canjson-encodes the value
// and then replaces the named object-valued slot with a different decodable
// concrete, standing in for a codec that narrows a polymorphic slot to the
// wrong dynamic type. B then decodes to a different concrete than A.
func retypeSlotReEncoder(slot string, replacement map[string]any) func(any) ([]byte, error) {
	return func(v any) ([]byte, error) {
		b, err := canjson.Marshal(v)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		m[slot] = replacement
		return json.Marshal(m)
	}
}
