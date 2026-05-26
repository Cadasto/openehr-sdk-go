package serializeprobes_test

import (
	"testing"

	serializeprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/serialize"
)

// TestProbe030 runs PROBE-030 across the canonical input set and
// asserts every input produces Status == "pass". The set spans leaf
// RM values and full composition cassettes vendored under
// testkit/cassettes/compositions/ and testkit/cassettes/rm/. The conformance harness in
// `make conformance` invokes the same probe function against shared
// cross-SDK cassettes (REQ-080).
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
