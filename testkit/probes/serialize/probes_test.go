package serializeprobes_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	conformance "github.com/cadasto/openehr-sdk-go/testkit/conformance/webtemplate"
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

// TestProbe086 — upstream FLAT serialisation parity over the pinned EHRbase
// conformance corpus (REQ-080). Unlike PROBE-076 above, the input is FLAT
// this SDK did not write, so it can catch a path the SDK never emits or a
// leaf it silently drops.
//
// The corpus is enumerated once here and each case is handed to the probe:
// corpus I/O is the caller's job, as with every other probe in this package.
func TestProbe086(t *testing.T) {
	target, err := serializeprobes.NewProbe086Target()
	if err != nil {
		t.Fatalf("build corpus target: %v", err)
	}
	cases, err := conformance.Cases()
	if err != nil {
		t.Fatalf("enumerate corpus: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("PROBE-086 found no corpus fixtures — check the vendored corpus")
	}
	var passes int
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			r, err := serializeprobes.Probe086UpstreamFlatParity(target, c)
			if err != nil {
				t.Fatalf("probe framework error: %v", err)
			}
			if r.Status != "pass" {
				t.Errorf("status = %q (detail: %s); want pass", r.Status, r.Detail)
				return
			}
			passes++
		})
	}
	if passes != len(cases) {
		t.Errorf("PROBE-086 passed %d/%d fixtures", passes, len(cases))
	}
}

// TestProbe086CoverageFloor pins the coverage floor: a fixture the codec
// refuses in its entirety compares nothing, and Report.Clean() is vacuously
// true over an empty compared set — so the probe must report "fail", not the
// "pass" a consumer reading Status alone would otherwise be given.
//
// The synthetic body is a single key under a template root that does not
// exist, which the codec refuses as an unknown path. That drives Compared to
// 0 without needing any new API from the conformance package: Case is exported
// and carries only a name and a path to a FLAT body.
func TestProbe086CoverageFloor(t *testing.T) {
	target, err := serializeprobes.NewProbe086Target()
	if err != nil {
		t.Fatalf("build corpus target: %v", err)
	}
	path := filepath.Join(t.TempDir(), "synthetic_all_refused.flat.json")
	if err := os.WriteFile(path, []byte(`{"no_such_root/no_such_path|value":"x"}`), 0o600); err != nil {
		t.Fatalf("write synthetic fixture: %v", err)
	}
	r, err := serializeprobes.Probe086UpstreamFlatParity(target,
		conformance.Case{Name: "synthetic_all_refused", Flat: path})
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "fail" {
		t.Errorf("status = %q (detail: %s); want fail — a fixture comparing 0 keys must not pass", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "compared 0") {
		t.Errorf("detail = %q; want it to report the zero-coverage cause", r.Detail)
	}
}

// TestProbe086FrameworkMisuse — a nil target or a case with no FLAT body is
// framework misuse (a non-nil error), not a probe failure, so a harness can
// tell "the probe could not run" from "the codec is wrong".
func TestProbe086FrameworkMisuse(t *testing.T) {
	if _, err := serializeprobes.Probe086UpstreamFlatParity(nil, conformance.Case{Name: "x", Flat: "x.json"}); err == nil {
		t.Error("nil target: err = nil; want a framework error")
	}
	target, err := serializeprobes.NewProbe086Target()
	if err != nil {
		t.Fatalf("build corpus target: %v", err)
	}
	if _, err := serializeprobes.Probe086UpstreamFlatParity(target, conformance.Case{Name: "x"}); err == nil {
		t.Error("case with empty Flat: err = nil; want a framework error")
	}
}
