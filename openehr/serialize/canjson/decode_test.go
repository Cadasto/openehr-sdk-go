package canjson_test

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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
// canjson.ErrInvalidShape is reserved for (REQ-052, wire.md): a
// syntax error, a type mismatch on a non-polymorphic field, and a
// numeric magnitude out of float64 range. All three fail today — none
// of them through the sentinel.
//
// The want fields pin WHICH arm of the documented classification
// produced each failure, so a future producer cannot quietly change
// the arm. They differ for the syntax row on purpose: Unmarshal sees
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
}{
	{
		name:               "syntax error: object truncated after the opening brace",
		in:                 `{`,
		wantUnmarshalErr:   "unexpected end of JSON input",
		wantDecodeErr:      "unexpected EOF",
		wantDecodeSentinel: io.ErrUnexpectedEOF,
	},
	{
		name:             "type mismatch: magnitude is not a number",
		in:               `{"_type":"DV_QUANTITY","magnitude":"not-a-number","units":"kg"}`,
		wantUnmarshalErr: "canjson: DV_QUANTITY:",
		wantDecodeErr:    "canjson: DV_QUANTITY:",
	},
	{
		name:             "numeric overflow: magnitude out of float64 range",
		in:               `{"_type":"DV_QUANTITY","magnitude":1e400,"units":"kg"}`,
		wantUnmarshalErr: "canjson: DV_QUANTITY:",
		wantDecodeErr:    "canjson: DV_QUANTITY:",
	},
}

// TestUnmarshalDoesNotWrapErrInvalidShape pins the deferred REQ-052
// producer: no decode path returns canjson.ErrInvalidShape, so an
// errors.Is against it never matches, whichever shape error the input
// carries. Landing the sentinel means updating this test — that is the
// follow-up which closes the gap.
func TestUnmarshalDoesNotWrapErrInvalidShape(t *testing.T) {
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
			if errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("Unmarshal(%s) err = %v wraps ErrInvalidShape; no decode path produces the sentinel yet (REQ-052 producer deferred)", tt.in, err)
			}
		})
	}
}

// TestDecoderDecodeDoesNotWrapErrInvalidShape is the streaming twin of
// the test above — the only coverage of Decoder.Decode in the package.
// It also pins the sentinel Decode's godoc names for a truncated
// value, io.ErrUnexpectedEOF, where Unmarshal reports a
// *json.SyntaxError.
func TestDecoderDecodeDoesNotWrapErrInvalidShape(t *testing.T) {
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
			if errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("Decode(%s) err = %v wraps ErrInvalidShape; no decode path produces the sentinel yet (REQ-052 producer deferred)", tt.in, err)
			}
		})
	}
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
// closes the gap (docs/plans/2026-08-30-read-path-decode-taxonomy.md)
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
