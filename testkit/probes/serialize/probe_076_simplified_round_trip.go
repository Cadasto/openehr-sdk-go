package serializeprobes

// PROBE-076 — FLAT / STRUCTURED simplified-format composition round-trip
// (REQ-053). Given a vendored upstream OPT + canonical COMPOSITION, the probe
// builds the Web Template (REQ-106), decodes the canonical composition, and
// asserts the simplified codecs round-trip it without losing the data the
// formats carry:
//
//   - FLAT idempotence:      MarshalFlat -> UnmarshalFlat -> MarshalFlat is stable;
//   - STRUCTURED round-trip: MarshalStructured -> UnmarshalStructured re-encodes
//     to the same FLAT;
//   - interconversion:       FlatToStructured -> StructuredToFlat is the identity.
//
// Scope: round-trip idempotence PLUS an OPT-conformance check — when the source
// composition is itself OPT-valid, a WithTemplate decode (names + RM-mandatory
// completion) must also validate against the OPT. That conformance leg catches
// dropped or mistyped leaves that pure FLAT idempotence (a symmetric omission on
// both encode and decode) would miss. It does not compare the emitted
// FLAT/STRUCTURED byte-for-byte against vendored upstream simplified output — a
// documented follow-up needing those fixtures (see
// openehr/serialize/simplified/deviations.md § Conformance).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Probe076SimplifiedRoundTrip runs the round-trip conformance checks for one
// (OPT, canonical composition) pair. A template the Web Template builder or the
// OPT compiler cannot yet handle yields Status "skip" (not "fail") so the probe
// distinguishes an un-modelled template from a codec defect. Framework misuse
// (nil inputs) returns a non-nil error.
func Probe076SimplifiedRoundTrip(optBody, compBody []byte) (Result, error) {
	r := Result{Probe: "PROBE-076"}
	if optBody == nil || compBody == nil {
		return r, errors.New("PROBE-076: nil opt/composition body")
	}
	opt, err := fixtures.ParseOPTBytes(optBody)
	if err != nil {
		r.Status, r.Detail = "fail", "parse opt: "+err.Error()
		return r, nil
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		r.Status, r.Detail = "skip", "compile: "+err.Error()
		return r, nil
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		r.Status, r.Detail = "skip", "web template: "+err.Error()
		return r, nil
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(compBody, &comp); err != nil {
		r.Status, r.Detail = "fail", "canjson decode: "+err.Error()
		return r, nil
	}

	f1, err := simplified.MarshalFlat(&comp, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat: "+err.Error()
		return r, nil
	}
	// FLAT idempotence.
	comp2, err := simplified.UnmarshalFlat(f1, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "UnmarshalFlat: "+err.Error()
		return r, nil
	}
	f2, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat (re-encode): "+err.Error()
		return r, nil
	}
	if !flatMapsEqual(f1, f2) {
		r.Status, r.Detail = "fail", "FLAT round-trip not idempotent"
		return r, nil
	}
	// STRUCTURED round-trip via the OPT.
	s, err := simplified.MarshalStructured(&comp, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalStructured: "+err.Error()
		return r, nil
	}
	comp3, err := simplified.UnmarshalStructured(s, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "UnmarshalStructured: "+err.Error()
		return r, nil
	}
	f3, err := simplified.MarshalFlat(comp3, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat (from structured): "+err.Error()
		return r, nil
	}
	if !flatMapsEqual(f1, f3) {
		r.Status, r.Detail = "fail", "STRUCTURED round-trip diverges from FLAT"
		return r, nil
	}
	// OPT-free interconversion preserves the data. STRUCTURED is arrays-always
	// (spec), so the back-conversion cannot know a single-cardinality leaf had no
	// :index and emits a redundant :0 — valid-but-verbose FLAT, not data loss.
	// Assert semantic (canonical) equivalence: the interconverted FLAT decodes
	// and re-encodes to the same canonical FLAT. See deviations.md.
	sBytes, err := simplified.FlatToStructured(f1)
	if err != nil {
		r.Status, r.Detail = "fail", "FlatToStructured: "+err.Error()
		return r, nil
	}
	back, err := simplified.StructuredToFlat(sBytes)
	if err != nil {
		r.Status, r.Detail = "fail", "StructuredToFlat: "+err.Error()
		return r, nil
	}
	compIC, err := simplified.UnmarshalFlat(back, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "UnmarshalFlat (interconverted): "+err.Error()
		return r, nil
	}
	fIC, err := simplified.MarshalFlat(compIC, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat (interconverted): "+err.Error()
		return r, nil
	}
	if !flatMapsEqual(f1, fIC) {
		r.Status, r.Detail = "fail", "FLAT<->STRUCTURED interconversion loses data"
		return r, nil
	}
	// Conformance: when the source composition is itself OPT-valid, a WithTemplate
	// decode (names + RM-mandatory completion) must also validate — this catches
	// dropped/mistyped leaves that FLAT idempotence alone (a symmetric omission)
	// would miss.
	if validation.Validate(&comp, compiled).OK {
		named, err := simplified.UnmarshalFlat(f1, wt, simplified.WithTemplate(compiled))
		if err != nil {
			r.Status, r.Detail = "fail", "UnmarshalFlat (WithTemplate): "+err.Error()
			return r, nil
		}
		if vr := validation.Validate(named, compiled); !vr.OK {
			r.Status, r.Detail = "fail", "WithTemplate decode of a valid composition does not validate: "+firstIssue(vr)
			return r, nil
		}
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("%d FLAT keys round-tripped", flatKeyCount(f1))
	return r, nil
}

func firstIssue(r validation.Result) string {
	if len(r.Issues) == 0 {
		return ""
	}
	return r.Issues[0].Code + " " + r.Issues[0].Path
}

// flatMapsEqual compares two FLAT payloads for exact semantic equality. Both
// sides are decoded with json.Number — comparing through float64 would round
// integers above 2^53 on both sides and mask a regression of exactly the
// precision guarantee the codec documents (json.Number values compare as
// strings under DeepEqual, i.e. exactly).
func flatMapsEqual(a, b []byte) bool {
	ma, err := decodeNumberMap(a)
	if err != nil {
		return false
	}
	mb, err := decodeNumberMap(b)
	if err != nil {
		return false
	}
	return reflect.DeepEqual(ma, mb)
}

func decodeNumberMap(b []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func flatKeyCount(b []byte) int {
	m, err := decodeNumberMap(b)
	if err != nil {
		return 0
	}
	return len(m)
}
