package serializeprobes

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
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

// TestProbe030GuardCatchesMissingValidateRM is the can-fail control for
// PROBE-030's validation.ValidateRM leg (REQ-112). clinical_notes.v0 carries
// an empty action_archetype_id in vendored content; the fidelity legs still
// pass, but the RM floor fails. The mutation that turns this red is removing
// the ValidateRM block in probe030RoundTrip: then this cassette would report
// Status == "pass" with skipFloor false.
func TestProbe030GuardCatchesMissingValidateRM(t *testing.T) {
	body, err := os.ReadFile(fixtures.CompositionJSON("clinical_notes.v0"))
	if err != nil {
		t.Fatalf("read clinical_notes.v0: %v", err)
	}
	r, err := probe030RoundTrip(body, func() any { return new(rm.Composition) }, canjson.Marshal, false)
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("status = %q, want fail: clinical_notes.v0 must fail the RM floor leg", r.Status)
	}
	if !strings.Contains(r.Detail, "RM floor") {
		t.Fatalf("detail = %q; want the ValidateRM leg to fire, not a fidelity leg", r.Detail)
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

// TestProbe030SkipFloorSetIsLocked pins the membership of probe030SkipFloor,
// not just that its one entry behaves. The RM-floor leg (REQ-112) is a MUST for
// every cassette; only a cassette whose vendored content carries a finding
// invariant to the round trip may be held out, and today that is exactly
// clinical_notes.v0. Because probe030RoundTrip skips the floor for any key in
// this map, an entry added here silently drops the floor MUST for that cassette
// while TestProbe030 and the ValidateRM plant both stay green. This guard fails
// when the set changes, so a new skip has to be justified in the probe.
//
// Can-fail control: add any cassette to probe030SkipFloor and this test reddens.
func TestProbe030SkipFloorSetIsLocked(t *testing.T) {
	want := map[string]bool{"compositions/clinical_notes.v0.json": true}
	if len(probe030SkipFloor) != len(want) {
		t.Fatalf("probe030SkipFloor has %d entries, want %d: %v", len(probe030SkipFloor), len(want), probe030SkipFloor)
	}
	for k := range want {
		if !probe030SkipFloor[k] {
			t.Errorf("probe030SkipFloor is missing the expected skip %q", k)
		}
	}
	for k := range probe030SkipFloor {
		if !want[k] {
			t.Errorf("probe030SkipFloor holds an unexpected skip %q — a floor-MUST holdout must be justified in the probe, not added silently", k)
		}
	}
}
