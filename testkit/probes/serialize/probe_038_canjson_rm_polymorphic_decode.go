package serializeprobes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// Probe038CanjsonRMPolymorphicDecode implements PROBE-038: canjson
// MUST decode every BMM-admissible `_type` discriminator at every
// substitutable slot — covering (a) substitutable subtypes in
// concrete-typed slots (e.g. LOCATABLE.name DV_TEXT carrying
// DV_CODED_TEXT, per openEHR RM Liskov substitution) and (b) generic
// types parameterised over an abstract bound (e.g.
// DV_INTERVAL[T: DV_ORDERED]).
//
// Pins REQ-052. For the given `body` and target `factory`, the
// probe asserts:
//
//  1. canjson.Unmarshal succeeds.
//  2. canjson.Marshal of the recovered value succeeds.
//  3. Every `_type` discriminator the input carried also appears at
//     least once in the re-marshalled output. Substitution must be
//     lossless across decode then re-marshal; a silent narrowing
//     (e.g. DV_CODED_TEXT decoded into a parent DVText struct that
//     loses defining_code and re-emits as DV_TEXT) is the regression
//     this assertion guards against.
//  4. Re-marshalling is a wire-equivalence fixpoint: decoding the
//     re-marshalled bytes (b1) and encoding again (b2) produces a
//     document wire-equivalent to b1 (testkit/wireequiv). The catalog
//     states this leg as "re-marshalling produces a document
//     wire-equivalent to the same logical content" (conformance.md
//     PROBE-038 Wire assertion); it catches a subtype or bound field
//     that survives the first re-marshal but not the second decode,
//     which the discriminator multiset alone can miss.
//
// `body` MUST be canonical-JSON bytes for a known concrete RM type.
// `factory` returns a fresh pointer to the target Go type; it is
// called twice, so the probe owns each decoded value's lifecycle.
func Probe038CanjsonRMPolymorphicDecode(body []byte, factory func() any) (Result, error) {
	return probe038PolymorphicDecode(body, factory, canjson.Marshal)
}

// probe038PolymorphicDecode runs PROBE-038. Probe038CanjsonRMPolymorphicDecode
// passes the real canjson.Marshal for the b2 step; can-fail tests pass a lossy
// double that drops a field on the b2 encode, proving the wire-equivalence
// fixpoint leg catches a field lost across the second decode-and-encode
// (probe_038_guard_internal_test.go).
func probe038PolymorphicDecode(body []byte, factory func() any, secondMarshal func(any) ([]byte, error)) (Result, error) {
	r := Result{Probe: "PROBE-038"}
	if factory == nil {
		return r, errors.New("PROBE-038: factory is nil")
	}
	if body == nil {
		r.Status = "fail"
		r.Detail = "input body is nil — likely a cassette discovery failure"
		return r, nil
	}
	v := factory()
	if err := canjson.Unmarshal(body, v); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("decode: %v", err)
		return r, nil
	}
	out, err := canjson.Marshal(v)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("re-marshal: %v", err)
		return r, nil
	}
	wantTypes := collectDiscriminators(body)
	gotTypes := collectDiscriminators(out)
	missing := wantTypes.Diff(gotTypes)
	if len(missing) > 0 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("re-marshal lost %d discriminator(s): %v (substitution narrowed; subtype-only fields dropped)", len(missing), missing)
		return r, nil
	}
	// Wire-equivalence leg: the re-marshalled document (b1) must be a
	// fixpoint under a further decode-and-encode. Decode b1, encode b2,
	// and compare the two SDK encodes with wire equivalence (member order
	// ignored, array order kept). A subtype or bound field that b1 still
	// carried but the second decode drops surfaces here.
	b1 := out
	v2 := factory()
	if err := canjson.Unmarshal(b1, v2); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("second decode of the re-marshalled document: %v", err)
		return r, nil
	}
	b2, err := secondMarshal(v2)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("re-marshal (b2): %v", err)
		return r, nil
	}
	if ok, diff := wireequiv.Equivalent(b1, b2); !ok {
		r.Status = "fail"
		r.Detail = "the two SDK encodes are not wire-equivalent across the second decode: " + diff
		return r, nil
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("decoded + re-marshalled (wire-equivalent fixpoint); %d discriminators preserved", len(wantTypes))
	return r, nil
}

// typeSet is a counted multiset of `_type` discriminator values that
// appeared in a canonical-JSON document. Counts let the probe
// distinguish "lost a DV_CODED_TEXT entirely" (the regression) from
// "elided a duplicate" (acceptable when canonical-ordering merges).
type typeSet map[string]int

// Diff returns discriminators that appear in `want` but are missing
// (or under-represented) in `got`. Sorted for stable Detail output.
func (want typeSet) Diff(got typeSet) []string {
	var out []string
	for k, n := range want {
		if got[k] < n {
			out = append(out, fmt.Sprintf("%s (want %d, got %d)", k, n, got[k]))
		}
	}
	slices.Sort(out)
	return out
}

// collectDiscriminators returns a multiset of every `_type` string
// value reachable via JSON-tree walk. Robust against ordering changes
// (canonical-JSON re-orders keys) — counts only.
func collectDiscriminators(b []byte) typeSet {
	out := typeSet{}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if tn, ok := t["_type"].(string); ok {
				out[tn]++
			}
			// Sort keys for determinism — the SDK's canonical JSON
			// already does this on emit, but inputs might not.
			for _, k := range slices.Sorted(maps.Keys(t)) {
				walk(t[k])
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	var top any
	if err := dec.Decode(&top); err != nil {
		return out
	}
	walk(top)
	return out
}

// Probe038Input is one input entry for PROBE-038.
type Probe038Input struct {
	Name    string
	Body    []byte
	Factory func() any
	loadErr error
}

// Probe038Inputs is the canonical set of inputs exercised by
// PROBE-038 in sandbox mode. Each entry isolates one substitution
// failure pattern (Issue A: concrete-typed slot receives subtype;
// Issue B: generic-over-abstract-bound) plus a representative
// composition that exercises both within one decode.
var Probe038Inputs = func() []Probe038Input {
	names := []string{
		"polymorphic/name_dv_coded_text",
		"polymorphic/dv_interval_quantity",
		"polymorphic/representative_full",
	}
	out := make([]Probe038Input, 0, len(names))
	for _, n := range names {
		body, err := os.ReadFile(fixtures.RMJSON(n))
		if err != nil {
			out = append(out, Probe038Input{
				Name:    "cassette:" + n,
				loadErr: fmt.Errorf("PROBE-038: read %q: %w", n, err),
				Factory: func() any { return new(rm.Composition) },
			})
			continue
		}
		out = append(out, Probe038Input{
			Name:    "cassette:" + n,
			Body:    body,
			Factory: func() any { return new(rm.Composition) },
		})
	}
	return out
}()
