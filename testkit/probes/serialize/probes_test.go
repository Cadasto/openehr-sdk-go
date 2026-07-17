package serializeprobes_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	serializeprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/serialize"
)

// TestProbe030 runs PROBE-030 across the canonical input set and
// asserts every input produces Status == "pass". The set spans leaf
// RM values and full composition cassettes vendored under
// testkit/cassettes/compositions/ and testkit/cassettes/rm/. The conformance harness in
// `make conformance` invokes the same probe function against shared
// openEHR conformance cassettes (REQ-080).
func TestProbe030(t *testing.T) {
	// Sanity-check the input set: at least one leaf entry AND at
	// least one cassette entry — guards against a silent regression
	// in cassette discovery.
	var leafSeen, cassetteSeen bool
	for _, in := range serializeprobes.Probe030Inputs {
		if len(in.Name) > len("cassette:") && in.Name[:len("cassette:")] == "cassette:" {
			cassetteSeen = true
		} else {
			leafSeen = true
		}
	}
	if !leafSeen {
		t.Error("Probe030Inputs missing leaf-type entries")
	}
	if !cassetteSeen {
		t.Error("Probe030Inputs missing cassette entries — check testkit/cassettes discovery via testkit/fixtures")
	}

	for _, in := range serializeprobes.Probe030Inputs {
		t.Run(in.Name, func(t *testing.T) {
			r, err := serializeprobes.Probe030CanjsonRoundTrip(in.Body, in.Factory)
			if err != nil {
				t.Fatalf("probe framework error: %v", err)
			}
			if r.Status != "pass" {
				t.Errorf("status = %q (detail: %s); want pass", r.Status, r.Detail)
			}
		})
	}
}

// TestProbe038 runs PROBE-038 across the polymorphic-decode fixture
// set vendored under testkit/cassettes/rm/polymorphic/ and asserts
// every input decodes + re-marshals with the original `_type`
// discriminators preserved (the REQ-052 substitutability guarantee).
func TestProbe038(t *testing.T) {
	if len(serializeprobes.Probe038Inputs) == 0 {
		t.Fatal("Probe038Inputs is empty — polymorphic fixture set missing")
	}
	for _, in := range serializeprobes.Probe038Inputs {
		t.Run(in.Name, func(t *testing.T) {
			r, err := serializeprobes.Probe038CanjsonRMPolymorphicDecode(in.Body, in.Factory)
			if err != nil {
				t.Fatalf("probe framework error: %v", err)
			}
			if r.Status != "pass" {
				t.Errorf("status = %q (detail: %s); want pass", r.Status, r.Detail)
			}
		})
	}
}

// TestProbe031 runs PROBE-031 and asserts the unknown-_type input
// surfaces as typereg.ErrUnknownType via errors.Is.
func TestProbe031(t *testing.T) {
	r, err := serializeprobes.Probe031TyperegUnknownType()
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "pass" {
		t.Errorf("status = %q (detail: %s); want pass", r.Status, r.Detail)
	}
}

// TestProbe033 runs PROBE-033 across the canonical XML input set and
// asserts every input round-trips byte-stable through canxml. Mirror
// of TestProbe030 for the XML wire.
func TestProbe033(t *testing.T) {
	if len(serializeprobes.Probe033Inputs) == 0 {
		t.Fatal("Probe033Inputs is empty — bootstrap encoder failed at init")
	}
	var leafSeen, cassetteSeen bool
	for _, in := range serializeprobes.Probe033Inputs {
		if len(in.Name) > len("cassette:") && in.Name[:len("cassette:")] == "cassette:" {
			cassetteSeen = true
		} else {
			leafSeen = true
		}
	}
	if !leafSeen {
		t.Error("Probe033Inputs missing leaf-type entries")
	}
	if !cassetteSeen {
		t.Error("Probe033Inputs missing cassette entries — check testkit/cassettes discovery via testkit/fixtures")
	}
	for _, in := range serializeprobes.Probe033Inputs {
		t.Run(in.Name, func(t *testing.T) {
			r, err := serializeprobes.Probe033CanxmlRoundTrip(in.Body, in.Factory)
			if err != nil {
				t.Fatalf("probe framework error: %v", err)
			}
			if r.Status != "pass" {
				t.Errorf("status = %q (detail: %s); want pass", r.Status, r.Detail)
			}
		})
	}
}

// TestProbe034 runs PROBE-034 and asserts the unknown-xsi:type input
// surfaces as typereg.ErrUnknownType via errors.Is.
func TestProbe034(t *testing.T) {
	r, err := serializeprobes.Probe034TyperegXSIUnknown()
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "pass" {
		t.Errorf("status = %q (detail: %s); want pass", r.Status, r.Detail)
	}
}

// TestProbe076 runs PROBE-076 across the vendored (OPT + canonical composition)
// pairs — the EHRbase Test_dv_* datatype corpus and the other constraint
// templates. Every template that the Web Template builder can model MUST
// round-trip (Status "pass"); a template it cannot yet model is "skip" (never
// "fail"). A pass floor guards against the corpus silently emptying.
func TestProbe076(t *testing.T) {
	ids, err := fixtures.ConstraintTemplateIDs()
	if err != nil {
		t.Fatalf("ConstraintTemplateIDs: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("no constraint template ids discovered")
	}
	var passes int
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			optBody, err := os.ReadFile(fixtures.TemplateOpt(id))
			if err != nil {
				t.Skipf("no OPT: %v", err)
			}
			compBody, err := os.ReadFile(fixtures.CompositionJSON(id))
			if err != nil {
				t.Skipf("no composition: %v", err)
			}
			r, err := serializeprobes.Probe076SimplifiedRoundTrip(optBody, compBody)
			if err != nil {
				t.Fatalf("probe framework error: %v", err)
			}
			switch r.Status {
			case "pass":
				passes++
			case "skip":
				t.Skipf("skip: %s", r.Detail)
			default:
				t.Errorf("status = %q (detail: %s); want pass", r.Status, r.Detail)
			}
		})
	}
	if passes == 0 {
		t.Error("PROBE-076 produced no passes — check cassette discovery / codec regressions")
	}
}
