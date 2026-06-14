package serializeprobes

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Probe033CanxmlRoundTrip implements PROBE-033: decoding a
// canonical-XML RM value and re-encoding produces byte-identical
// compact output. Mirrors PROBE-030 (canjson) for the XML wire.
//
// The probe asserts byte-stability of the SDK's XML round-trip
// pipeline (Decode → Encode → Decode → Encode), not byte equality
// against an arbitrary upstream serializer. Stability is the
// load-bearing guarantee for hashing/signing/diff tooling against the
// XML wire.
//
// `body` MUST be canonical-XML bytes for a known concrete RM type.
// `factory` returns a fresh pointer to the target Go type so the
// probe owns the value lifecycle.
func Probe033CanxmlRoundTrip(body []byte, factory func() any) (Result, error) {
	r := Result{Probe: "PROBE-033"}
	if factory == nil {
		return r, errors.New("PROBE-033: factory is nil")
	}
	if body == nil {
		r.Status = "fail"
		r.Detail = "input body is nil — likely a cassette discovery failure"
		return r, nil
	}
	v1 := factory()
	if err := canxml.Unmarshal(body, v1); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("first decode: %v", err)
		return r, nil
	}
	b1, err := canxml.Marshal(v1)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("first encode: %v", err)
		return r, nil
	}
	v2 := factory()
	if err := canxml.Unmarshal(b1, v2); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("second decode: %v", err)
		return r, nil
	}
	b2, err := canxml.Marshal(v2)
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

// Probe033Inputs is the canonical set of inputs exercised by
// PROBE-033 in sandbox mode. Leaf entries are bootstrap-encoded; cassette
// entries are discovered from `testkit/cassettes/compositions/*.xml` and
// `testkit/cassettes/rm/*.xml` via [fixtures.ListRMXML] (mirrors PROBE-030).
var Probe033Inputs = func() []Probe033Input {
	must := func(v any) []byte {
		b, err := canxml.Marshal(v)
		if err != nil {
			panic(fmt.Sprintf("PROBE-033 bootstrap encode: %v", err))
		}
		return b
	}
	out := []Probe033Input{
		{
			Name:    "DV_QUANTITY",
			Body:    must(&rm.DVQuantity{Magnitude: 80.5, Units: "kg"}),
			Factory: func() any { return new(rm.DVQuantity) },
		},
		{
			Name:    "DV_TEXT",
			Body:    must(&rm.DVText{Value: "hello"}),
			Factory: func() any { return new(rm.DVText) },
		},
		{
			Name: "Composition-with-polymorphic-composer",
			Body: must(&rm.Composition{
				ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
				Name:            rm.DVText{Value: "x"},
				Language:        rm.CodePhrase{CodeString: "en"},
				Territory:       rm.CodePhrase{CodeString: "GB"},
				Category:        rm.DVCodedText{DVText: rm.DVText{Value: "event"}},
				Composer:        &rm.PartySelf{},
			}),
			Factory: func() any { return new(rm.Composition) },
		},
	}
	cassettes, err := loadXMLCassetteInputs()
	if err != nil {
		out = append(out, Probe033Input{
			Name:    "_cassette_discovery_error",
			Body:    nil,
			Factory: func() any { return new(rm.Composition) },
			loadErr: err,
		})
		return out
	}
	return append(out, cassettes...)
}()

// Probe033Input is one input entry for PROBE-033.
type Probe033Input struct {
	Name    string
	Body    []byte
	Factory func() any
	// loadErr is set when cassette discovery failed at init.
	loadErr error
}

func loadXMLCassetteInputs() ([]Probe033Input, error) {
	rels, err := fixtures.ListRMXML()
	if err != nil {
		return nil, fmt.Errorf("PROBE-033: list cassettes: %w", err)
	}
	root := fixtures.CassettesRoot()
	out := make([]Probe033Input, 0, len(rels))
	for _, rel := range rels {
		path := filepath.Join(root, filepath.FromSlash(rel))
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("PROBE-033: read cassette %q: %w", rel, err)
		}
		factory, ok := fixtures.FactoryForXMLBody(body)
		if !ok {
			continue
		}
		out = append(out, Probe033Input{
			Name:    "cassette:" + rel,
			Body:    body,
			Factory: factory,
		})
	}
	return out, nil
}
