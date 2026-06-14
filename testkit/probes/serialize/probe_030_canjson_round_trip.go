// Package serializeprobes hosts the cross-SDK conformance probes
// for the openEHR serialization codecs. Each probe corresponds to a
// PROBE-NNN entry in docs/specifications/conformance.md and is implemented in both
// the Go and PHP SDKs against shared cassettes (REQ-080).
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
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary
// text for failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}

// Probe030CanjsonRoundTrip implements PROBE-030: decoding a
// canonical-JSON RM value and re-encoding produces byte-identical
// output after the SDK's canonical-ordering pass.
//
// The probe asserts byte-stability of the SDK's round-trip pipeline
// (Decode → Encode → Decode → Encode), not byte equality against an
// arbitrary upstream serializer. Stability is the load-bearing
// guarantee for hashing, signing, and diff tooling.
//
// `body` MUST be canonical-JSON bytes for a known concrete RM type.
// `factory` returns a fresh pointer to the target Go type — passed
// twice during the probe so the probe owns the value lifecycle.
//
// Errors returned by canjson during the round-trip surface as
// Result{Status:"fail"}; mechanical failures (e.g. nil factory)
// return a non-nil error so the harness can distinguish probe
// failure from probe-framework failure.
func Probe030CanjsonRoundTrip(body []byte, factory func() any) (Result, error) {
	r := Result{Probe: "PROBE-030"}
	if factory == nil {
		return r, errors.New("PROBE-030: factory is nil")
	}
	if body == nil {
		r.Status = "fail"
		r.Detail = "input body is nil — likely a cassette discovery failure"
		return r, nil
	}
	v1 := factory()
	if err := canjson.Unmarshal(body, v1); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("first decode: %v", err)
		return r, nil
	}
	b1, err := canjson.Marshal(v1)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("first encode: %v", err)
		return r, nil
	}
	v2 := factory()
	if err := canjson.Unmarshal(b1, v2); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("second decode: %v", err)
		return r, nil
	}
	b2, err := canjson.Marshal(v2)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("second encode: %v", err)
		return r, nil
	}
	if !bytes.Equal(b1, b2) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("round-trip not byte-stable\nb1=%s\nb2=%s", b1, b2)
		return r, nil
	}
	r.Status = "pass"
	return r, nil
}

// Probe030Inputs is the canonical set of inputs exercised by
// PROBE-030 in sandbox mode. The cross-SDK harness asserts that both
// Go and PHP produce byte-equal results across this set when fed the
// same shared cassettes (REQ-081). The set spans leaf RM values and
// full composition cassettes vendored under
// `testkit/cassettes/compositions/` and `testkit/cassettes/rm/`. The Event/History polymorphism
// that initially blocked composition round-trip is resolved in ADR
// 0003 (docs/adr/0003-rm-event-polymorphism.md).
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
	// loadErr is set when the cassette discovery step failed at init
	// for this entry; Probe030CanjsonRoundTrip surfaces it as Status=fail.
	loadErr error
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
			Name:    "cassette:" + rel.Rel,
			Body:    body,
			Factory: factory,
		})
	}
	return out, nil
}
