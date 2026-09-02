package canjson_test

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestUnmarshalLeafConcreteType — a non-polymorphic leaf type
// (DV_QUANTITY) decodes cleanly with default encoding/json: no
// generated UnmarshalJSON required.
func TestUnmarshalLeafConcreteType(t *testing.T) {
	in := []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`)
	var q rm.DVQuantity
	if err := canjson.Unmarshal(in, &q); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if q.Magnitude != 80.5 || q.Units != "kg" {
		t.Errorf("got Magnitude=%v Units=%v; want 80.5 kg", q.Magnitude, q.Units)
	}
}

// TestUnmarshalCompositionDispatchesContent — Composition.content is a
// []ContentItem; the generated UnmarshalJSON MUST consult typereg
// per-item and produce the right concrete types.
func TestUnmarshalCompositionDispatchesContent(t *testing.T) {
	in := []byte(`{
        "_type": "COMPOSITION",
        "archetype_node_id": "x",
        "name": {"_type": "DV_TEXT", "value": "x"},
        "language": {"_type": "CODE_PHRASE", "code_string": "en"},
        "territory": {"_type": "CODE_PHRASE", "code_string": "GB"},
        "category": {"_type": "DV_CODED_TEXT", "value": "event"},
        "composer": {"_type": "PARTY_SELF"},
        "content": [
            {"_type": "OBSERVATION", "archetype_node_id": "obs1", "name": {"_type":"DV_TEXT","value":"obs1"}, "language":{"_type":"CODE_PHRASE","code_string":"en"}, "encoding":{"_type":"CODE_PHRASE","code_string":"UTF-8"}, "subject":{"_type":"PARTY_SELF"}}
        ]
    }`)
	var c rm.Composition
	if err := canjson.Unmarshal(in, &c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(c.Content) != 1 {
		t.Fatalf("content len = %d; want 1", len(c.Content))
	}
	obs, ok := c.Content[0].(*rm.Observation)
	if !ok {
		t.Errorf("content[0] is %T; want *rm.Observation", c.Content[0])
	} else if obs.ArchetypeNodeID != "obs1" {
		t.Errorf("obs.ArchetypeNodeID = %q; want obs1", obs.ArchetypeNodeID)
	}
	if _, ok := c.Composer.(*rm.PartySelf); !ok {
		t.Errorf("composer is %T; want *rm.PartySelf", c.Composer)
	}
}

// TestUnmarshalUnknownTypeWrapsTypereg — an unrecognised `_type` at a
// polymorphic site MUST return an error that errors.Is against
// typereg.ErrUnknownType (PROBE-031).
func TestUnmarshalUnknownTypeWrapsTypereg(t *testing.T) {
	in := []byte(`{
        "_type": "COMPOSITION",
        "archetype_node_id": "x",
        "name": {"_type": "DV_TEXT", "value": "x"},
        "language": {"_type": "CODE_PHRASE", "code_string": "en"},
        "territory": {"_type": "CODE_PHRASE", "code_string": "GB"},
        "category": {"_type": "DV_CODED_TEXT", "value": "event"},
        "composer": {"_type": "NEVER_REGISTERED_TYPE"}
    }`)
	var c rm.Composition
	err := canjson.Unmarshal(in, &c)
	if err == nil {
		t.Fatal("expected error for unknown _type")
	}
	if !errors.Is(err, typereg.ErrUnknownType) {
		t.Errorf("err = %v; want errors.Is(_, typereg.ErrUnknownType)", err)
	}
}

// TestUnmarshalMissingTypeStrictDefault — strict default: a missing
// `_type` at a polymorphic site is an error wrapping
// typereg.ErrMissingType.
func TestUnmarshalMissingTypeStrictDefault(t *testing.T) {
	in := []byte(`{
        "_type": "COMPOSITION",
        "archetype_node_id": "x",
        "name": {"_type": "DV_TEXT", "value": "x"},
        "language": {"_type": "CODE_PHRASE", "code_string": "en"},
        "territory": {"_type": "CODE_PHRASE", "code_string": "GB"},
        "category": {"_type": "DV_CODED_TEXT", "value": "event"},
        "composer": {"name": {"value": "Dr. X"}}
    }`)
	var c rm.Composition
	err := canjson.Unmarshal(in, &c)
	if err == nil {
		t.Fatal("expected error for missing _type at polymorphic site")
	}
	if !errors.Is(err, typereg.ErrMissingType) {
		t.Errorf("err = %v; want errors.Is(_, typereg.ErrMissingType)", err)
	}
}

// TestDecodeErrorCarriesPath — the DecodeError envelope MUST carry
// a JSON-pointer-ish path so callers can locate the bad node.
func TestDecodeErrorCarriesPath(t *testing.T) {
	in := []byte(`{
        "_type": "COMPOSITION",
        "archetype_node_id": "x",
        "name": {"_type": "DV_TEXT", "value": "x"},
        "language": {"_type": "CODE_PHRASE", "code_string": "en"},
        "territory": {"_type": "CODE_PHRASE", "code_string": "GB"},
        "category": {"_type": "DV_CODED_TEXT", "value": "event"},
        "composer": {"_type": "PARTY_SELF"},
        "content": [
            {"_type": "BOGUS_ITEM"}
        ]
    }`)
	var c rm.Composition
	err := canjson.Unmarshal(in, &c)
	if err == nil {
		t.Fatal("expected error for bogus _type inside content[0]")
	}
	de, ok := errors.AsType[*canjson.DecodeError](err)
	if !ok {
		t.Fatalf("err = %v; want *canjson.DecodeError", err)
	}
	if !strings.Contains(de.Path, "content") {
		t.Errorf("DecodeError.Path = %q; want path to mention content", de.Path)
	}
}

// shapeErrorInputs are the three JSON-level shape-error classes
// REQ-052 (wire.md) discusses under canjson.ErrInvalidShape: a syntax
// error, a type mismatch on a non-polymorphic field, and a numeric
// magnitude out of float64 range. Only the last two are raised inside
// a generated UnmarshalJSON and so carry the sentinel; encoding/json
// rejects the syntax error before any UnmarshalJSON method runs, so
// that one reaches the caller unclassified.
//
// The want fields pin WHICH arm of the documented classification
// produced each failure, so a later change cannot quietly move the
// arm. They differ for the syntax row on purpose: Unmarshal sees
// the whole input and reports *json.SyntaxError, while Decoder.Decode
// runs out of stream and reports io.ErrUnexpectedEOF. The other
// stream-level divergences Decode's godoc names — an empty stream and
// content after the first value — are pinned by
// TestDecoderDecodeStreamDivergesFromUnmarshal.
var shapeErrorInputs = []struct {
	name string
	in   string
	// wantUnmarshalErr / wantDecodeErr are substrings of the error text.
	wantUnmarshalErr string
	wantDecodeErr    string
	// wantDecodeSentinel, when set, is the sentinel Decode's error must
	// match under errors.Is — the one Decode's godoc names.
	wantDecodeSentinel error
	// wantShapeSentinel is whether the failure carries
	// canjson.ErrInvalidShape: true for the in-type failures, false for
	// the syntax error encoding/json reports on its own.
	wantShapeSentinel bool
	// assertCause, when set, checks that the typed cause the sentinel is
	// forbidden to displace is still reachable — the "classification
	// costs no diagnostic" half of the clause. The two in-type rows fail
	// on different causes because rm.Real accepts quoted decimals
	// (ADR 0004): a quoted non-number fails in strconv, an out-of-range
	// JSON number fails in encoding/json.
	assertCause func(t *testing.T, err error)
}{
	{
		name:               "syntax error: object truncated after the opening brace",
		in:                 `{`,
		wantUnmarshalErr:   "unexpected end of JSON input",
		wantDecodeErr:      "unexpected EOF",
		wantDecodeSentinel: io.ErrUnexpectedEOF,
		wantShapeSentinel:  false,
	},
	{
		name:              "type mismatch: magnitude is not a number",
		in:                `{"_type":"DV_QUANTITY","magnitude":"not-a-number","units":"kg"}`,
		wantUnmarshalErr:  "canjson: DV_QUANTITY:",
		wantDecodeErr:     "canjson: DV_QUANTITY:",
		wantShapeSentinel: true,
		assertCause: func(t *testing.T, err error) {
			t.Helper()
			if _, ok := errors.AsType[*strconv.NumError](err); !ok {
				t.Errorf("err = %v; want errors.As to still reach *strconv.NumError under the sentinel", err)
			}
		},
	},
	{
		name:              "numeric overflow: magnitude out of float64 range",
		in:                `{"_type":"DV_QUANTITY","magnitude":1e400,"units":"kg"}`,
		wantUnmarshalErr:  "canjson: DV_QUANTITY:",
		wantDecodeErr:     "canjson: DV_QUANTITY:",
		wantShapeSentinel: true,
		assertCause: func(t *testing.T, err error) {
			t.Helper()
			if _, ok := errors.AsType[*json.UnmarshalTypeError](err); !ok {
				t.Errorf("err = %v; want errors.As to still reach *json.UnmarshalTypeError under the sentinel", err)
			}
		},
	},
}

// assertShapeSentinelDistinct pins the Global-Constraint half of
// REQ-052 on a decode failure: whatever else it carries, it MUST NOT
// match the encode-only canjson.ErrInvalidValue nor the transport-level
// transport.ErrInvalidShape — three distinct sentinel values. The
// same-named canxml.ErrInvalidShape is checked beside them: it is a
// fourth, unrelated value (the XML codec's own, used in both
// directions), and only a test keeps two same-named sentinels from
// being quietly conflated.
func assertShapeSentinelDistinct(t *testing.T, err error) {
	t.Helper()
	if errors.Is(err, canjson.ErrInvalidValue) {
		t.Errorf("decode failure must not match the encode-only canjson.ErrInvalidValue; got %v", err)
	}
	if errors.Is(err, transport.ErrInvalidShape) {
		t.Errorf("decode failure must not match transport.ErrInvalidShape; got %v", err)
	}
	if errors.Is(err, canxml.ErrInvalidShape) {
		t.Errorf("a JSON decode failure must not match the XML codec's canxml.ErrInvalidShape; got %v", err)
	}
}

// TestUnmarshalWrapsErrInvalidShape pins REQ-052's decode-side shape
// sentinel: a shape failure raised inside a generated UnmarshalJSON —
// the `canjson: <RM_TYPE>:` family — matches errors.Is against
// canjson.ErrInvalidShape, while malformed JSON, which never reaches a
// generated UnmarshalJSON, does not. The classification costs nothing:
// the error text is unchanged and the encoding/json cause stays
// reachable with errors.As.
func TestUnmarshalWrapsErrInvalidShape(t *testing.T) {
	for _, tt := range shapeErrorInputs {
		t.Run(tt.name, func(t *testing.T) {
			var q rm.DVQuantity
			err := canjson.Unmarshal([]byte(tt.in), &q)
			if err == nil {
				t.Fatalf("Unmarshal(%s) = nil; want a JSON shape error", tt.in)
			}
			if !strings.Contains(err.Error(), tt.wantUnmarshalErr) {
				t.Errorf("Unmarshal(%s) err = %v; want the text to contain %q", tt.in, err, tt.wantUnmarshalErr)
			}
			if got := errors.Is(err, canjson.ErrInvalidShape); got != tt.wantShapeSentinel {
				t.Errorf("Unmarshal(%s) err = %v: errors.Is(_, canjson.ErrInvalidShape) = %t; want %t", tt.in, err, got, tt.wantShapeSentinel)
			}
			if tt.assertCause != nil {
				tt.assertCause(t, err)
			}
			assertShapeSentinelDistinct(t, err)
		})
	}
}

// TestDecoderDecodeWrapsErrInvalidShape is the streaming twin of
// the test above — the only coverage of Decoder.Decode in the package.
// It also pins the sentinel Decode's godoc names for a truncated
// value, io.ErrUnexpectedEOF, where Unmarshal reports a
// *json.SyntaxError.
func TestDecoderDecodeWrapsErrInvalidShape(t *testing.T) {
	for _, tt := range shapeErrorInputs {
		t.Run(tt.name, func(t *testing.T) {
			var q rm.DVQuantity
			err := canjson.NewDecoder(strings.NewReader(tt.in)).Decode(&q)
			if err == nil {
				t.Fatalf("Decode(%s) = nil; want a JSON shape error", tt.in)
			}
			if !strings.Contains(err.Error(), tt.wantDecodeErr) {
				t.Errorf("Decode(%s) err = %v; want the text to contain %q", tt.in, err, tt.wantDecodeErr)
			}
			if tt.wantDecodeSentinel != nil && !errors.Is(err, tt.wantDecodeSentinel) {
				t.Errorf("Decode(%s) err = %v; want errors.Is(_, %v)", tt.in, err, tt.wantDecodeSentinel)
			}
			if got := errors.Is(err, canjson.ErrInvalidShape); got != tt.wantShapeSentinel {
				t.Errorf("Decode(%s) err = %v: errors.Is(_, canjson.ErrInvalidShape) = %t; want %t", tt.in, err, got, tt.wantShapeSentinel)
			}
			if tt.assertCause != nil {
				tt.assertCause(t, err)
			}
			assertShapeSentinelDistinct(t, err)
		})
	}
}

// TestUnmarshalNestedDecodeErrorIsNotShapeTagged draws the sentinel's
// far boundary (REQ-052). A polymorphic failure inside a nested value
// travels out through the enclosing type's `canjson: <RM_TYPE>:`
// funnel — here DV_QUANTITY's, because normal_range is a plain field
// of the wire struct — and MUST keep its *DecodeError classification
// without picking up ErrInvalidShape on the way. The sentinel means
// "JSON-level shape", not "any decode failure".
func TestUnmarshalNestedDecodeErrorIsNotShapeTagged(t *testing.T) {
	const in = `{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg",` +
		`"normal_range":{"lower":{"_type":"NEVER_REGISTERED_TYPE"}}}`
	var q rm.DVQuantity
	err := canjson.Unmarshal([]byte(in), &q)
	if err == nil {
		t.Fatal("Unmarshal(nested unknown _type) = nil; want a polymorphic decode error")
	}
	if !strings.Contains(err.Error(), "canjson: DV_QUANTITY:") {
		t.Fatalf("err = %v; want the text to show it passed through DV_QUANTITY's funnel — otherwise this test no longer covers the nesting case", err)
	}
	if _, ok := errors.AsType[*canjson.DecodeError](err); !ok {
		t.Errorf("err = %v (%T); want errors.As to reach *canjson.DecodeError", err, err)
	}
	if !errors.Is(err, typereg.ErrUnknownType) {
		t.Errorf("err = %v; want errors.Is(_, typereg.ErrUnknownType)", err)
	}
	if errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; a nested polymorphic failure must not be re-classified as a JSON shape error", err)
	}
	assertShapeSentinelDistinct(t, err)
}

// TestUnmarshalWholeValueTypeMismatchIsNotShapeTagged draws the
// sentinel's other exclusion boundary (REQ-052). A generated
// UnmarshalJSON refuses a `_type` that names a different class than the
// target with a *DecodeError on "/_type" — the same exclusion the
// polymorphic-slot case gets, raised on the whole value rather than at
// a slot. It must not acquire ErrInvalidShape either: the bytes are a
// perfectly well-shaped DV_TEXT, they are simply not the requested
// class.
func TestUnmarshalWholeValueTypeMismatchIsNotShapeTagged(t *testing.T) {
	const in = `{"_type":"DV_TEXT","value":"hello"}`
	var q rm.DVQuantity
	err := canjson.Unmarshal([]byte(in), &q)
	if err == nil {
		t.Fatal("Unmarshal(DV_TEXT into rm.DVQuantity) = nil; want a _type mismatch error")
	}
	de, ok := errors.AsType[*canjson.DecodeError](err)
	if !ok {
		t.Fatalf("err = %v (%T); want errors.As to reach *canjson.DecodeError", err, err)
	}
	if de.Path != "/_type" {
		t.Errorf("DecodeError.Path = %q; want %q — this test covers the whole-value arm, not a slot", de.Path, "/_type")
	}
	if !errors.Is(err, typereg.ErrTypeMismatch) {
		t.Errorf("err = %v; want errors.Is(_, typereg.ErrTypeMismatch)", err)
	}
	if errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; a whole-value _type mismatch is a DecodeError, not a JSON shape error", err)
	}
	assertShapeSentinelDistinct(t, err)
}

// TestDecoderDecodeStreamDivergesFromUnmarshal pins the two divergences
// Decode's godoc names beyond the truncated-value one: reading a stream
// rather than a whole input changes the answer for an empty input and
// for content after the first value. Both are measured behaviour of
// encoding/json's Decoder, not a canjson policy — this test is the
// tripwire if a future codec swap changes either.
func TestDecoderDecodeStreamDivergesFromUnmarshal(t *testing.T) {
	t.Run("empty stream is io.EOF, not a syntax error", func(t *testing.T) {
		var q rm.DVQuantity
		err := canjson.NewDecoder(strings.NewReader("")).Decode(&q)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Decode(\"\") err = %v; want errors.Is(_, io.EOF)", err)
		}
		var uq rm.DVQuantity
		if uerr := canjson.Unmarshal([]byte(""), &uq); !errors.As(uerr, new(*json.SyntaxError)) {
			t.Errorf("Unmarshal(\"\") err = %v; want *json.SyntaxError (the divergence this test pins)", uerr)
		}
	})

	t.Run("content after the first value is the next value, not an error", func(t *testing.T) {
		const in = `{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}{"_type":"DV_QUANTITY","magnitude":1.5,"units":"kg"}`
		dec := canjson.NewDecoder(strings.NewReader(in))
		var first rm.DVQuantity
		if err := dec.Decode(&first); err != nil {
			t.Fatalf("Decode(first value) = %v; want nil", err)
		}
		if first.Magnitude != 80.5 {
			t.Errorf("first.Magnitude = %v; want 80.5", first.Magnitude)
		}
		var second rm.DVQuantity
		if err := dec.Decode(&second); err != nil {
			t.Fatalf("Decode(second value) = %v; want nil — trailing content is the next stream value", err)
		}
		if second.Magnitude != 1.5 {
			t.Errorf("second.Magnitude = %v; want 1.5", second.Magnitude)
		}
		var uq rm.DVQuantity
		if uerr := canjson.Unmarshal([]byte(in), &uq); !errors.As(uerr, new(*json.SyntaxError)) {
			t.Errorf("Unmarshal(two values) err = %v; want *json.SyntaxError (the divergence this test pins)", uerr)
		}
	})
}

// TestUnmarshalOverflowIsATypedError pins the half of REQ-052's
// floating-point clause that IS met: an out-of-range magnitude fails
// with a typed error a caller can reach by errors.As, even though the
// generated UnmarshalJSON wraps it behind a `canjson: DV_QUANTITY:`
// prefix. "Typed error" here is *json.UnmarshalTypeError, not the
// ErrInvalidShape sentinel — wrapping the sentinel alone would not
// discharge the clause.
func TestUnmarshalOverflowIsATypedError(t *testing.T) {
	var q rm.DVQuantity
	err := canjson.Unmarshal([]byte(`{"_type":"DV_QUANTITY","magnitude":1e400,"units":"kg"}`), &q)
	if err == nil {
		t.Fatal("Unmarshal(magnitude 1e400) = nil; want a typed range error")
	}
	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); !ok {
		t.Errorf("err = %v (%T); want errors.As to reach *json.UnmarshalTypeError", err, err)
	}
}

// TestUnmarshalMantissaPrecisionLossIsSilent pins the half of REQ-052's
// floating-point clause that is still OPEN: a magnitude carrying more
// significant digits than float64 holds is rounded silently, where the
// clause requires a typed error "rather than silently rounding". This
// test documents today's behaviour, not the target; the producer that
// closes the gap (docs/plans/archive/2026-08-30-read-path-decode-taxonomy.md)
// must invert it.
func TestUnmarshalMantissaPrecisionLossIsSilent(t *testing.T) {
	const in = `{"_type":"DV_QUANTITY","magnitude":0.1234567890123456789,"units":"kg"}`
	var q rm.DVQuantity
	if err := canjson.Unmarshal([]byte(in), &q); err != nil {
		t.Fatalf("Unmarshal(%s) = %v; today precision loss is silent, so want nil", in, err)
	}
	const want = 0.12345678901234568 // the float64 nearest the wire value
	if q.Magnitude != want {
		t.Errorf("Magnitude = %.17g; want %.17g (the rounded value, gap not yet closed)", q.Magnitude, want)
	}
}
