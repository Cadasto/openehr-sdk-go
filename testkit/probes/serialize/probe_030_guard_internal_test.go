package serializeprobes

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
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
// RM-floor findings in vendored content; the fidelity legs still pass, but the
// RM floor fails. The plant runs through both public entry points with the
// floor on, so it fails when the ValidateRM block in probe030RoundTrip is
// removed, when Probe030CanjsonRoundTrip stops passing skipFloor=false, and
// when Probe030CanjsonRoundTripInput stops honouring an input's
// SkipFloor=false: each of those reports Status == "pass".
func TestProbe030GuardCatchesMissingValidateRM(t *testing.T) {
	body, err := os.ReadFile(fixtures.CompositionJSON("clinical_notes.v0"))
	if err != nil {
		t.Fatalf("read clinical_notes.v0: %v", err)
	}
	factory := func() any { return new(rm.Composition) }
	entries := []struct {
		name string
		run  func() (Result, error)
	}{
		{"Probe030CanjsonRoundTrip", func() (Result, error) { return Probe030CanjsonRoundTrip(body, factory) }},
		{"Probe030CanjsonRoundTripInput", func() (Result, error) {
			return Probe030CanjsonRoundTripInput(Probe030Input{Name: "plant", Body: body, Factory: factory})
		}},
	}
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			r, err := e.run()
			if err != nil {
				t.Fatalf("probe framework error: %v", err)
			}
			if r.Status != "fail" {
				t.Fatalf("status = %q, want fail: clinical_notes.v0 with the floor on must fail the RM floor leg", r.Status)
			}
			if !strings.Contains(r.Detail, "RM floor") {
				t.Fatalf("detail = %q; want the ValidateRM leg to fire, not a fidelity leg", r.Detail)
			}
		})
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
// not just that its entries behave. The RM-floor leg (REQ-112) is a MUST for
// every cassette; only a cassette whose vendored content carries a finding
// invariant to the round trip may be held out, and today that is exactly the
// five named below, each justified at its probe030SkipFloor entry. Because probe030RoundTrip skips the floor for any key in
// this map, an entry added here silently drops the floor MUST for that cassette
// while TestProbe030 and the ValidateRM plant both stay green. This guard fails
// when the set changes, so a new skip has to be justified in the probe.
//
// Can-fail control: add any cassette to probe030SkipFloor and this test reddens.
func TestProbe030SkipFloorSetIsLocked(t *testing.T) {
	want := map[string]bool{
		"compositions/clinical_notes.v0.json":                               true,
		"compositions/Demonstration.v1.json":                                true,
		"compositions/TestPerson.v2.json":                                   true,
		"compositions/Test_dv_interval_dv_count_open_constraint.v0.json":    true,
		"compositions/Test_dv_interval_dv_quantity_open_constraint.v0.json": true,
	}
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

// TestProbe030InputsSkipFloorFollowsTheLockedSet pins the wiring between
// probe030SkipFloor and Probe030Inputs (REQ-112): an input skips the RM floor
// if and only if it is a cassette named in the locked set. The lock above pins
// the set's membership; this pins that nothing else sets SkipFloor.
//
// Can-fail control: set SkipFloor to true for every cassette in
// loadCassetteInputs (or on a leaf entry) and this test reddens, while the
// set lock stays green.
func TestProbe030InputsSkipFloorFollowsTheLockedSet(t *testing.T) {
	skipped := 0
	for _, in := range Probe030Inputs {
		rel, isCassette := strings.CutPrefix(in.Name, "cassette:")
		want := isCassette && probe030SkipFloor[rel]
		if in.SkipFloor != want {
			t.Errorf("%s: SkipFloor = %v, want %v (only cassettes in probe030SkipFloor may skip the RM floor)", in.Name, in.SkipFloor, want)
		}
		if in.SkipFloor {
			skipped++
		}
	}
	if skipped != len(probe030SkipFloor) {
		t.Errorf("%d inputs skip the RM floor, want %d (one per probe030SkipFloor entry; a missing cassette leaves a stale skip)", skipped, len(probe030SkipFloor))
	}
}

// TestProbe030SkipFloorFindingsAreInvariantToTheRoundTrip pins the premise that
// justifies each probe030SkipFloor holdout (REQ-112): the RM-floor findings on
// the input decode and on the round-tripped value B are the same set, so the
// round trip adds none. Without it the skip would hide any new floor violation
// the round trip introduced for that cassette.
//
// Can-fail control: a round-trip regression that drops a required attribute
// on the encode path adds a finding to B only, and this test reddens.
func TestProbe030SkipFloorFindingsAreInvariantToTheRoundTrip(t *testing.T) {
	for _, in := range Probe030Inputs {
		rel, isCassette := strings.CutPrefix(in.Name, "cassette:")
		if !isCassette || !probe030SkipFloor[rel] {
			continue
		}
		t.Run(rel, func(t *testing.T) {
			first := in.Factory()
			if err := canjson.Unmarshal(in.Body, first); err != nil {
				t.Fatalf("first decode: %v", err)
			}
			b := first
			for i := range 2 {
				wire, err := canjson.Marshal(b)
				if err != nil {
					t.Fatalf("encode %d: %v", i+1, err)
				}
				b = in.Factory()
				if err := canjson.Unmarshal(wire, b); err != nil {
					t.Fatalf("decode %d: %v", i+2, err)
				}
			}
			before, after := floorFindings(validation.ValidateRM(first)), floorFindings(validation.ValidateRM(b))
			if len(before) == 0 {
				t.Fatalf("no RM-floor findings on the input decode: the holdout is stale, remove it from probe030SkipFloor")
			}
			if !slices.Equal(before, after) {
				t.Fatalf("the round trip changed the RM-floor findings:\ninput decode: %v\nround-tripped: %v", before, after)
			}
		})
	}
}

// floorFindings returns each issue's code and path, the identity of a finding
// independent of its human-readable detail.
func floorFindings(r validation.Result) []string {
	out := make([]string, 0, len(r.Issues))
	for _, is := range r.Issues {
		out = append(out, is.Code+" "+is.Path)
	}
	return out
}
