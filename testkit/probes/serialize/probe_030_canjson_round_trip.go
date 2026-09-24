// Package serializeprobes hosts the openEHR conformance probes
// for the openEHR serialization codecs. Each probe corresponds to a
// PROBE-NNN entry in docs/specifications/conformance.md and is implemented in both
// any openEHR-conformant implementation against shared cassettes (REQ-080).
//
// Probes are plain Go functions returning (Result, error) and are
// designed to be invocable from:
//
//   - the SDK's own test suite (via TestProbeNNN);
//   - the conformance harness in `make conformance`;
//   - third-party consumers checking their integration.
//
// The probes deliberately avoid `testing.T` so they can run outside
// `go test`.
package serializeprobes

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// Result is the shared probe outcome (REQ-082).
type Result = probe.Result

// Probe030CanjsonRoundTrip implements PROBE-030 (REQ-052): a
// canonical-JSON RM value survives the SDK round trip with its meaning
// intact. It decodes `body`, encodes it (`b1`), decodes that (`A`),
// encodes again (`b2`), then decodes that (`B`).
//
// A and B MUST be equal by typed deep comparison (reflect.DeepEqual over
// the decoded RM values, which compares an interface-typed field by its
// dynamic type and value and so covers every substitutable slot and
// every DV_INTERVAL[T] bound). B MUST satisfy validation.ValidateRM
// (REQ-112) with no issues. As a secondary check, b1 and b2 MUST be
// wire-equivalent (testkit/wireequiv): equal once each is parsed into a
// generic JSON value, with member order ignored and array order kept.
//
// The comparison straddles the second encode, not the first. The first
// encode may legitimately collapse a container (DV_TEXT.mappings under
// omitempty, wire.md REQ-052), so A and B are read either side of the
// re-encode instead. The input is never compared byte-wise: JSON member
// order carries no meaning (RFC 8259 section 4) and the encoder makes no
// byte-level promise.
//
// `body` MUST be canonical-JSON bytes for a known concrete RM type.
// `factory` returns a fresh pointer to the target Go type, called three
// times so the probe owns each value's lifecycle.
//
// Errors returned by canjson during the round trip surface as
// Result{Status: probe.StatusFail}; mechanical failures (e.g. nil factory)
// return a non-nil error so the harness can distinguish probe
// failure from probe-framework failure.
func Probe030CanjsonRoundTrip(body []byte, factory func() any) (Result, error) {
	return probe030RoundTrip(body, factory, canjson.Marshal, false)
}

// Probe030CanjsonRoundTripInput runs PROBE-030 for one input from
// [Probe030Inputs], honoring its SkipFloor flag: an input whose vendored
// content carries an RM-floor finding independent of the round trip runs the
// fidelity legs (typed deep comparison, wire equivalence) but skips the
// validation.ValidateRM leg. Use this when iterating the corpus;
// Probe030CanjsonRoundTrip is the body/factory form with the floor leg always
// on.
func Probe030CanjsonRoundTripInput(in Probe030Input) (Result, error) {
	if in.loadErr != nil {
		return Result{Probe: "PROBE-030", Status: "fail", Detail: "cassette discovery: " + in.loadErr.Error()}, nil
	}
	return probe030RoundTrip(in.Body, in.Factory, canjson.Marshal, in.SkipFloor)
}

// probe030RoundTrip runs the PROBE-030 pipeline with reEncode as the
// second encode step. Probe030CanjsonRoundTrip passes the real
// canjson.Marshal. A can-fail test passes a lossy double to prove the
// typed deep comparison of A and B catches a field dropped, or a
// polymorphic slot narrowed, on the re-encode path: each mutation
// changes B without touching A, so reflect.DeepEqual and the
// wire-equivalence secondary both flag it (probe_030_guard_internal_test.go).
//
// skipFloor drops only the validation.ValidateRM leg, for an input whose
// vendored content carries an RM-floor finding independent of the round trip
// (see [Probe030Input.SkipFloor]); the fidelity legs always run.
func probe030RoundTrip(body []byte, factory func() any, reEncode func(any) ([]byte, error), skipFloor bool) (Result, error) {
	r := Result{Probe: "PROBE-030"}
	if factory == nil {
		return r, errors.New("PROBE-030: factory is nil")
	}
	if body == nil {
		r.Status = "fail"
		r.Detail = "input body is nil, likely a cassette discovery failure"
		return r, nil
	}
	v := factory()
	if err := canjson.Unmarshal(body, v); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("first decode: %v", err)
		return r, nil
	}
	b1, err := canjson.Marshal(v)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("first encode (b1): %v", err)
		return r, nil
	}
	valueA := factory()
	if err := canjson.Unmarshal(b1, valueA); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("second decode (A): %v", err)
		return r, nil
	}
	b2, err := reEncode(valueA)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("re-encode (b2): %v", err)
		return r, nil
	}
	valueB := factory()
	if err := canjson.Unmarshal(b2, valueB); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("third decode (B): %v", err)
		return r, nil
	}
	if !reflect.DeepEqual(valueA, valueB) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("A and B differ across the re-encode (a field or polymorphic slot was lost)\nb1=%s\nb2=%s", b1, b2)
		return r, nil
	}
	if !skipFloor {
		if vr := validation.ValidateRM(valueB); !vr.OK {
			r.Status = "fail"
			r.Detail = "round-tripped value does not satisfy the RM floor (REQ-112): " + firstIssue(vr)
			return r, nil
		}
	}
	if ok, diff := wireequiv.Equivalent(b1, b2); !ok {
		r.Status = "fail"
		r.Detail = "the two SDK encodes are not wire-equivalent: " + diff
		return r, nil
	}
	r.Status = "pass"
	return r, nil
}

// Probe030Inputs is the set of inputs exercised by PROBE-030 in sandbox
// mode. Each input survives the round trip with its meaning intact
// (typed deep comparison of A and B, plus wire equivalence, and the RM
// floor unless the input sets SkipFloor) when fed the vendored cassettes
// (REQ-080, REQ-112). The set spans leaf RM values and full composition
// cassettes vendored under `testkit/cassettes/compositions/` and
// `testkit/cassettes/rm/`. The Event/History polymorphism that initially
// blocked composition round-trip is resolved in ADR 0003
// (docs/adr/0003-rm-event-polymorphism.md).
//
// Every discovered cassette stays in the set so the fidelity legs run on
// all of them; an input carrying an RM-floor finding independent of the
// round trip sets SkipFloor, which drops only the ValidateRM leg.
//
// Populated at package init: the leaf entries are inline; cassette
// entries are discovered from disk so adding a fixture file does not
// require editing this source.
var Probe030Inputs = func() []Probe030Input {
	out := []Probe030Input{
		{
			Name:    "DV_QUANTITY",
			Body:    []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`),
			Factory: func() any { return new(rm.DVQuantity) },
		},
		{
			Name:    "DV_CODED_TEXT",
			Body:    []byte(`{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}`),
			Factory: func() any { return new(rm.DVCodedText) },
		},
	}
	cassettes, err := loadCassetteInputs()
	if err != nil {
		// Surfacing at probe-invocation time rather than crashing at
		// init keeps the conformance harness's failure mode observable:
		// callers see a missing-cassette entry, not an opaque panic.
		out = append(out, Probe030Input{
			Name:    "_cassette_discovery_error",
			Body:    nil,
			Factory: func() any { return new(rm.Composition) },
			loadErr: err,
		})
		return out
	}
	return append(out, cassettes...)
}()

// Probe030Input is one input entry for PROBE-030.
type Probe030Input struct {
	Name    string
	Body    []byte
	Factory func() any
	// SkipFloor drops only the validation.ValidateRM leg for this input,
	// for a cassette whose vendored content carries an RM-floor finding
	// independent of the round trip. The fidelity legs (typed deep
	// comparison, wire equivalence) still run. See probe030SkipFloor.
	SkipFloor bool
	// loadErr is set when the cassette discovery step failed at init for
	// this entry; Probe030CanjsonRoundTripInput surfaces it as Status=fail.
	loadErr error
}

// probe030SkipFloor names cassettes whose vendored content carries RM-floor
// findings that are present before any round trip, so the ValidateRM leg is
// skipped for them while the fidelity legs still run. Vendored content is not
// edited.
//
// Each entry lists the exact findings (Issue.Code, a space, Issue.Path) that
// validation.ValidateRM reports for the cassette. A guard test asserts that the
// input decode and the round-tripped value both report exactly this list, so a
// regression that adds a floor finding to a held-out cassette still fails, and
// the list cannot drift from the content it names
// (TestProbe030SkipFloorFindingsArePinned).
var probe030SkipFloor = map[string][]string{
	// clinical_notes.v0 leaves required RM attributes absent or empty, for
	// example the empty string at action_archetype_id.
	"compositions/clinical_notes.v0.json": {
		"required /content[0]/data/origin",
		"required /content[0]/data/events[0]/time",
		"required /content[2]/narrative",
		"required /content[2]/activities[0]/timing/value",
		"required /content[2]/activities[0]/timing/formalism",
		"required /content[2]/activities[0]/action_archetype_id",
		"required /content[2]/activities[0]/description/items[2]/items[0]/value/value",
		"required /content[2]/activities[0]/description/items[2]/items[1]/value/value",
		"required /content[2]/activities[0]/description/items[2]/items[2]/value/value",
	},
	// Demonstration.v1: seven DV_INTERVAL values (DV_QUANTITY and DV_COUNT
	// bounds) have lower greater than upper, for example 30 over 12.25 cm.
	"compositions/Demonstration.v1.json": {
		"rm_invariant /content[0]/data/events[0]/data/items[1]/items[4]/value",
		"rm_invariant /content[0]/data/events[0]/data/items[1]/items[6]/value",
		"rm_invariant /content[0]/data/events[1]/data/items[1]/items[4]/value",
		"rm_invariant /content[0]/data/events[1]/data/items[1]/items[6]/value",
		"rm_invariant /content[0]/data/events[2]/data/items[1]/items[6]/value",
		"rm_invariant /content[0]/data/events[3]/data/items[1]/items[4]/value",
		"rm_invariant /content[0]/data/events[3]/data/items[2]/items[4]/value",
	},
	// TestPerson.v2: DV_MULTIMEDIA.media_type is a CODE_PHRASE whose
	// code_string is null, which the floor reports both as an empty
	// CODE_PHRASE and as an absent mandatory attribute.
	"compositions/TestPerson.v2.json": {
		"rm_invariant /details/items[13]/items[5]/value/media_type",
		"required /details/items[13]/items[5]/value/media_type/code_string",
	},
	// Test_dv_interval_dv_count_open_constraint.v0: a DV_INTERVAL<DV_COUNT>
	// with lower 200 over upper 100.
	"compositions/Test_dv_interval_dv_count_open_constraint.v0.json": {
		"rm_invariant /content[0]/data/events[0]/data/items[0]/value",
	},
	// Test_dv_interval_dv_quantity_open_constraint.v0: a
	// DV_INTERVAL<DV_QUANTITY> with lower 200 mm over upper 100 mm.
	"compositions/Test_dv_interval_dv_quantity_open_constraint.v0.json": {
		"rm_invariant /content[0]/data/events[0]/data/items[0]/value",
	},
}

// loadCassetteInputs discovers vendored cassettes relative to this
// source file and returns one Probe030Input per `*.json` cassette.
// Path resolution uses runtime.Caller so the helper works regardless
// of the caller's working directory — the conformance harness invokes
// probes outside of `go test`.
//
// Discovery walks one level deep so vendored upstream sets (e.g.
// `rm/` ehrbase samples) are exercised alongside the SDK's own
// fixtures. Each input's target RM type is picked via
// [factoryForCassette] using filename hints — `ehr_status` → EHR_STATUS,
// `folder` → FOLDER, otherwise COMPOSITION. Without per-cassette
// dispatch the EHR_STATUS / FOLDER ehrbase cassettes would fail with
// `typereg: decoded type does not satisfy target` on first decode.
func loadCassetteInputs() ([]Probe030Input, error) {
	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		return nil, fmt.Errorf("PROBE-030: list cassettes: %w", err)
	}
	out := make([]Probe030Input, 0, len(rels))
	for _, rel := range rels {
		body, err := os.ReadFile(fixtures.ResolveCompositionJSON(rel))
		if err != nil {
			return nil, fmt.Errorf("PROBE-030: read cassette %q: %w", rel.Rel, err)
		}
		factory, ok := fixtures.FactoryForJSONRel(rel)
		if !ok {
			continue
		}
		out = append(out, Probe030Input{
			Name:      "cassette:" + rel.Rel,
			Body:      body,
			Factory:   factory,
			SkipFloor: len(probe030SkipFloor[rel.Rel]) > 0,
		})
	}
	return out, nil
}
