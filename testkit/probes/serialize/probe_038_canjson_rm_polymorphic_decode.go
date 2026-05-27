package serializeprobes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Probe038CanjsonRMPolymorphicDecode implements PROBE-038: canjson
// MUST decode every BMM-admissible `_type` discriminator at every
// substitutable slot — covering (a) substitutable subtypes in
// concrete-typed slots (e.g. LOCATABLE.name DV_TEXT carrying
// DV_CODED_TEXT, per openEHR RM Liskov substitution) and (b) generic
// types parameterised over an abstract bound (e.g.
// DV_INTERVAL[T: DV_ORDERED]).
//
// Pins SDK-GAP-11. For the given `body` and target `factory`, the
// probe asserts:
//
//  1. canjson.Unmarshal succeeds.
//  2. canjson.Marshal of the recovered value succeeds.
//  3. Every `_type` discriminator the input carried also appears at
//     least once in the re-marshalled output. Substitution must be
//     lossless across decode → re-marshal; a silent narrowing
//     (e.g. DV_CODED_TEXT decoded into a parent DVText struct that
//     loses defining_code and re-emits as DV_TEXT) is the regression
//     this assertion guards against.
//
// `body` MUST be canonical-JSON bytes for a known concrete RM type.
// `factory` returns a fresh pointer to the target Go type.
func Probe038CanjsonRMPolymorphicDecode(body []byte, factory func() any) (Result, error) {
	r := Result{Probe: "PROBE-038"}
	if factory == nil {
		return r, fmt.Errorf("PROBE-038: factory is nil")
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
	r.Status = "pass"
	r.Detail = fmt.Sprintf("decoded + re-marshalled; %d discriminators preserved", len(wantTypes))
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
	sort.Strings(out)
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
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
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
