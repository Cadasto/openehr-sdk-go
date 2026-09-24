package canjson_test

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
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
	// REQ-052: a polymorphic dispatch failure stays outside the shape
	// sentinel — the same exclusion the unknown-`_type` and
	// whole-value-mismatch arms assert.
	if errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; a missing `_type` is a dispatch failure, not a JSON shape error", err)
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
// a generated UnmarshalJSON and so carry the sentinel; the tokenizer
// refuses the syntax error during tokenisation, so that one reaches the
// caller unclassified.
//
// The want fields pin WHICH arm of the documented classification
// produced each failure, so a later change cannot quietly move the
// arm. Under encoding/json/v2 both Unmarshal and Decoder.Decode report
// a *jsontext.SyntacticError wrapping io.ErrUnexpectedEOF on this
// truncated input, so the syntax row's want strings both match the same
// "unexpected EOF" text. The stream-level divergences Decode's godoc
// names, an empty stream and content after the first value, are pinned
// by TestDecoderDecodeStreamDivergesFromUnmarshal.
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
	// JSON number fails in encoding/json/v2 (a *json.SemanticError).
	assertCause func(t *testing.T, err error)
}{
	{
		name:               "syntax error: object truncated after the opening brace",
		in:                 `{`,
		wantUnmarshalErr:   "unexpected EOF",
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
			if _, ok := errors.AsType[*jsonv2.SemanticError](err); !ok {
				t.Errorf("err = %v; want errors.As to still reach *encoding/json/v2.SemanticError under the sentinel", err)
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
// canjson.ErrInvalidShape, while malformed JSON, which the tokenizer
// refuses during tokenisation, does not. The classification costs nothing:
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
// far boundary (REQ-052). A polymorphic dispatch failure inside a nested value
// travels out through the enclosing type's `canjson: <RM_TYPE>:` funnel and
// MUST keep its *DecodeError classification without picking up ErrInvalidShape
// on the way. The sentinel means "JSON-level shape", not "any decode failure".
//
// normal_range is a DVInterval[DVQuantity], a CONCRETE bound under the
// streaming codec (ADR 0022), not a registry-dispatched slot as it was under
// the per-field decoder. So an unregistered `_type` on /lower is a mismatch
// against the bound's own type (expected DV_QUANTITY), which is still a
// dispatch failure carrying ErrTypeMismatch and staying outside the shape
// sentinel, the boundary the test guards.
func TestUnmarshalNestedDecodeErrorIsNotShapeTagged(t *testing.T) {
	const in = `{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg",` +
		`"normal_range":{"lower":{"_type":"NEVER_REGISTERED_TYPE"}}}`
	var q rm.DVQuantity
	err := canjson.Unmarshal([]byte(in), &q)
	if err == nil {
		t.Fatal("Unmarshal(nested wrong _type) = nil; want a polymorphic decode error")
	}
	if !strings.Contains(err.Error(), "canjson: DV_QUANTITY:") {
		t.Fatalf("err = %v; want the text to show it passed through DV_QUANTITY's funnel — otherwise this test no longer covers the nesting case", err)
	}
	if _, ok := errors.AsType[*canjson.DecodeError](err); !ok {
		t.Errorf("err = %v (%T); want errors.As to reach *canjson.DecodeError", err, err)
	}
	if !errors.Is(err, typereg.ErrTypeMismatch) {
		t.Errorf("err = %v; want errors.Is(_, typereg.ErrTypeMismatch)", err)
	}
	if errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; a nested dispatch failure must not be re-classified as a JSON shape error", err)
	}
	assertShapeSentinelDistinct(t, err)
}

// TestUnmarshalSlotNestedShapeFailureCarriesBothClassifications pins
// the near boundary of REQ-052's shape sentinel, the mirror of the test
// above. When the concrete type selected at a polymorphic slot fails on
// *shape* — here a DV_QUANTITY whose `units` is a number where the wire
// contract wants a string — the failure raised beneath the slot is a
// JSON-level shape failure, and the enclosing *DecodeError MUST NOT
// strip that classification on the way out. So both hold at once: the
// DecodeError names the slot on its Path, and errors.Is reaches
// canjson.ErrInvalidShape. Only the polymorphic *dispatch* failure
// (missing / unknown / mismatched `_type`) stays outside the sentinel.
func TestUnmarshalSlotNestedShapeFailureCarriesBothClassifications(t *testing.T) {
	// ELEMENT.value is declared DATA_VALUE, a genuine polymorphic slot the
	// streaming codec resolves through the registered DataValue hook. The wire
	// selects DV_QUANTITY there and gives its `units` a number where the
	// contract wants a string; a quoted *magnitude* would be tolerated instead
	// (ADR 0004 numeric wire tolerance), so it cannot drive this case. The
	// hook's DecodeError names the slot on its Path and does not strip the
	// ErrInvalidShape the concrete DV_QUANTITY funnel raised beneath it.
	const in = `{"_type":"ELEMENT","archetype_node_id":"at0","name":{"_type":"DV_TEXT","value":"n"},` +
		`"value":{"_type":"DV_QUANTITY","magnitude":80,"units":5}}`
	var e rm.Element
	err := canjson.Unmarshal([]byte(in), &e)
	if err == nil {
		t.Fatal("Unmarshal(slot-nested wrong-typed units) = nil; want a decode error")
	}
	if !strings.Contains(err.Error(), "canjson: ELEMENT:") {
		t.Fatalf("err = %v; want the text to show it passed through ELEMENT's funnel, otherwise this test no longer covers the nesting case", err)
	}
	de, ok := errors.AsType[*canjson.DecodeError](err)
	if !ok {
		t.Fatalf("err = %v (%T); want errors.As to reach *canjson.DecodeError", err, err)
	}
	if de.Path != "/value" {
		t.Errorf("DecodeError.Path = %q, want %q: the slot it failed at, so a consumer keeps the path alongside the kind", de.Path, "/value")
	}
	if !errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; a shape failure raised beneath a polymorphic slot stays a shape failure — a DecodeError must not strip the classification", err)
	}
	// The encoding/json/v2 cause stays reachable through the SDK classification:
	// the underlying SemanticError (which carries the number-into-string detail)
	// is not severed. The spec names only SDK sentinels, so the classification
	// above is what a consumer branches on; this pins that the cause survives.
	if _, ok := errors.AsType[*jsonv2.SemanticError](err); !ok {
		t.Errorf("err = %v; want the encoding/json/v2 cause to stay reachable with errors.AsType", err)
	}
	for _, sentinel := range []error{typereg.ErrUnknownType, typereg.ErrMissingType, typereg.ErrTypeMismatch} {
		if errors.Is(err, sentinel) {
			t.Errorf("err = %v; must not match %v — the `_type` dispatched fine, the shape did not", err, sentinel)
		}
	}
	assertShapeSentinelDistinct(t, err)
}

// TestUnmarshalNarrowSlotMissingTypeFallbackKeepsBothClassifications
// covers the one route into a slot-nested shape failure the test above
// does not: the missing-`_type` fallback on a *narrow* slot. `ELEMENT.name`
// is declared `DV_TEXT` and admits `DV_CODED_TEXT`, so the generator
// lifts it to DVTextLike and, when the wire omits `_type`, retries the
// bytes against the declared parent (the missing-`_type` tolerance in
// wire.md § REQ-052). That retry is a plain json.Unmarshal into DVText,
// so the reviewer's question was whether it can hand back a raw
// encoding/json error with no classification at all.
//
// It cannot: the retry goes through DVText's own generated
// UnmarshalJSON, whose funnel is typereg.WrapShapeError, so the failure
// arrives already carrying ErrInvalidShape and the generated code stores
// it as *DecodeError{Path: "/name"}. Both classifications hold, exactly
// as for the `_type`-present slot. ErrMissingType is *not* in the chain:
// the fallback consumed that condition and the dispatch error was
// discarded — what remains is the shape failure the retry produced.
func TestUnmarshalNarrowSlotMissingTypeFallbackKeepsBothClassifications(t *testing.T) {
	// `name` carries no `_type`, so dispatch falls back to DV_TEXT; its
	// `value` must be a string, and 5 is not one.
	const in = `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"value":5}}`
	var e rm.Element
	err := canjson.Unmarshal([]byte(in), &e)
	if err == nil {
		t.Fatal("Unmarshal(narrow slot, no _type, wrong-typed value) = nil; want a decode error")
	}
	de, ok := errors.AsType[*canjson.DecodeError](err)
	if !ok {
		t.Fatalf("err = %v (%T); want errors.As to reach *canjson.DecodeError", err, err)
	}
	if de.Path != "/name" {
		t.Errorf("DecodeError.Path = %q, want %q — the narrow slot the fallback failed at", de.Path, "/name")
	}
	if !errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; the fallback decodes through DV_TEXT's own funnel, so its shape failure must keep the sentinel", err)
	}
	if _, ok := errors.AsType[*jsonv2.SemanticError](err); !ok {
		t.Errorf("err = %v; want the encoding/json/v2 cause to stay reachable with errors.AsType", err)
	}
	if errors.Is(err, typereg.ErrMissingType) {
		t.Errorf("err = %v; the fallback consumed the missing-`_type` condition — what is reported is the retry's shape failure", err)
	}
	assertShapeSentinelDistinct(t, err)
}

// TestUnmarshalNarrowSlotMissingTypeDefaultsToParent is the positive
// twin of the test above: the same narrow slot with `_type` omitted and
// a well-shaped body decodes as the declared parent type. This is the
// missing-`_type` tolerance wire.md § REQ-052 grants permissive
// producers, and it is what makes the failing case above a *shape*
// failure rather than a dispatch one.
func TestUnmarshalNarrowSlotMissingTypeDefaultsToParent(t *testing.T) {
	const in = `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"value":"Systolic"}}`
	var e rm.Element
	if err := canjson.Unmarshal([]byte(in), &e); err != nil {
		t.Fatalf("Unmarshal(narrow slot, no _type, well-shaped body) = %v; want the declared parent to be assumed", err)
	}
	name, ok := e.Name.(*rm.DVText)
	if !ok {
		t.Fatalf("Element.Name is %T; want *rm.DVText — the declared parent of the DVTextLike slot", e.Name)
	}
	if name.Value != "Systolic" {
		t.Errorf("Element.Name.Value = %q, want %q", name.Value, "Systolic")
	}
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

// TestUnmarshalConcreteTypeMismatchPrecedence pins the precedence of the
// single-pass concrete decode (REQ-052, ADR 0022, ruling F8). The helper
// decodes the whole value first and reads the declared `_type` field only
// afterwards, so when a body is BOTH mislabelled (its `_type` names another
// class) AND fails the target's shape, the shape failure is reported, not the
// `_type` mismatch. This is the one observable behaviour change from the old
// buffer-and-peek, which checked `_type` before it decoded the body and so
// reported the mismatch first. Reverting the helper to peek `_type` before
// decoding turns the "mislabelled and failing the target's shape" case red (it
// would report typereg.ErrTypeMismatch on /_type instead of the shape failure).
func TestUnmarshalConcreteTypeMismatchPrecedence(t *testing.T) {
	t.Run("mislabelled and failing the target's shape reports the shape failure, not the mismatch", func(t *testing.T) {
		// _type names DV_TEXT (wrong for DVQuantity) and magnitude is a
		// non-number (which fails the target's shape). The single decode fails
		// on magnitude before the discriminator guard runs.
		const in = `{"_type":"DV_TEXT","magnitude":"not-a-number","units":"kg"}`
		var q rm.DVQuantity
		err := canjson.Unmarshal([]byte(in), &q)
		if err == nil {
			t.Fatalf("Unmarshal(%s) = nil; want the shape failure to win", in)
		}
		if !errors.Is(err, canjson.ErrInvalidShape) {
			t.Errorf("err = %v; a body that fails the target's shape reports the shape failure (ErrInvalidShape), the discriminator is read only after a clean decode", err)
		}
		if errors.Is(err, typereg.ErrTypeMismatch) {
			t.Errorf("err = %v; the shape failure precedes the _type check, so ErrTypeMismatch must not win here (the precedence flip)", err)
		}
		// The message shows it passed through DV_QUANTITY's own shape funnel.
		if !strings.Contains(err.Error(), "canjson: DV_QUANTITY:") {
			t.Errorf("err = %v; want the DV_QUANTITY shape funnel prefix", err)
		}
	})

	t.Run("mislabelled but well-formed reports the mismatch on /_type", func(t *testing.T) {
		// _type names DV_TEXT (wrong) but the body is a clean DV_TEXT; the decode
		// succeeds and the discriminator guard then refuses the mismatch.
		const in = `{"_type":"DV_TEXT","value":"hello"}`
		var q rm.DVQuantity
		err := canjson.Unmarshal([]byte(in), &q)
		if err == nil {
			t.Fatalf("Unmarshal(%s) = nil; want a _type mismatch", in)
		}
		de, ok := errors.AsType[*canjson.DecodeError](err)
		if !ok {
			t.Fatalf("err = %v (%T); want errors.As to reach *canjson.DecodeError", err, err)
		}
		if de.Path != "/_type" {
			t.Errorf("DecodeError.Path = %q; want %q", de.Path, "/_type")
		}
		if !errors.Is(err, typereg.ErrTypeMismatch) {
			t.Errorf("err = %v; want errors.Is(_, typereg.ErrTypeMismatch)", err)
		}
		if errors.Is(err, canjson.ErrInvalidShape) {
			t.Errorf("err = %v; a well-formed mislabelled body is a mismatch, not a shape failure", err)
		}
	})

	t.Run("no _type on a concrete target decodes as that target", func(t *testing.T) {
		// A concrete target admits an absent _type (REQ-052): the discriminator
		// stays empty and the guard passes.
		const in = `{"magnitude":80.5,"units":"kg"}`
		var q rm.DVQuantity
		if err := canjson.Unmarshal([]byte(in), &q); err != nil {
			t.Fatalf("Unmarshal(%s) = %v; want a concrete target to accept an absent _type", in, err)
		}
		if q.Magnitude != 80.5 || q.Units != "kg" {
			t.Errorf("got Magnitude=%v Units=%q; want 80.5 kg", q.Magnitude, q.Units)
		}
	})
}

// TestUnmarshalFirstFailureInDocumentOrderWins pins the precedence between a
// shape failure and malformed bytes on the single-pass decode (REQ-052, ADR
// 0022): the first failure in document order wins. The tokenizer refuses
// malformed bytes as the value they malform is decoded, and a shape failure
// earlier in the value stops the decode before the later bytes are read. So a
// member that fails the target's shape ahead of the malformed bytes reports the
// shape failure, and malformed bytes ahead of that member report the syntax
// error. Both entry points are driven, since they reach the bytes differently
// (a whole input and a stream).
//
// Can-fail (checked with go test -overlay on a patched copy of streaming.go):
// making DecodeInto read the whole value with dec.ReadValue before decoding
// it, the old buffer-first shape, turns both "shape failure before malformed
// bytes" cases red on both entry points, because the tokenizer then refuses
// the later bytes before the shape failure is reached.
func TestUnmarshalFirstFailureInDocumentOrderWins(t *testing.T) {
	entries := []struct {
		name   string
		decode func([]byte, any) error
	}{
		{"Unmarshal", canjson.Unmarshal},
		{"Decoder.Decode", func(b []byte, v any) error {
			return canjson.NewDecoder(strings.NewReader(string(b))).Decode(v)
		}},
	}
	shapeFirst := []struct{ name, in string }{
		// magnitude fails the target's shape before the stray `x` is read.
		{"stray byte", `{"_type":"DV_QUANTITY","magnitude":"abc","units":"kg" x}`},
		// magnitude fails the target's shape before the missing closing brace
		// is reached.
		{"truncated", `{"_type":"DV_QUANTITY","magnitude":true,"units":"kg"`},
	}
	const malformedFirst = `{"_type":"DV_QUANTITY","units":"kg" x,"magnitude":"abc"}`

	for _, e := range entries {
		for _, tc := range shapeFirst {
			in := tc.in
			t.Run(e.name+"/shape failure before malformed bytes/"+tc.name, func(t *testing.T) {
				var q rm.DVQuantity
				err := e.decode([]byte(in), &q)
				if !errors.Is(err, canjson.ErrInvalidShape) {
					t.Fatalf("%s(%s) err = %v; want errors.Is(_, canjson.ErrInvalidShape): the member that fails the target's shape comes first", e.name, in, err)
				}
				if se, ok := errors.AsType[*jsontext.SyntacticError](err); ok && se != nil {
					t.Errorf("%s(%s) err = %v; want no *jsontext.SyntacticError in the chain, the malformed bytes are never read", e.name, in, err)
				}
				if !strings.Contains(err.Error(), "canjson: DV_QUANTITY:") {
					t.Errorf("%s(%s) err = %v; want the DV_QUANTITY shape funnel prefix %q", e.name, in, err, "canjson: DV_QUANTITY:")
				}
			})
		}
		t.Run(e.name+"/malformed bytes before shape failure", func(t *testing.T) {
			var q rm.DVQuantity
			err := e.decode([]byte(malformedFirst), &q)
			if se, ok := errors.AsType[*jsontext.SyntacticError](err); !ok || se == nil {
				t.Fatalf("%s(%s) err = %v (%T); want a *jsontext.SyntacticError: the malformed bytes come first", e.name, malformedFirst, err, err)
			}
			if errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("%s(%s) err = %v; malformed input must not carry canjson.ErrInvalidShape", e.name, malformedFirst, err)
			}
		})
	}

	// The slot exception REQ-052 states: a polymorphic slot is read whole
	// (DecodePolymorphic's ReadValue) before it is decoded, so malformed bytes
	// anywhere inside the slot value are reported ahead of a shape failure
	// earlier in that same value. The DV_QUANTITY below is the shape-first body
	// that reports ErrInvalidShape at top level; inside ELEMENT.value it
	// reports the syntax error instead.
	t.Run("Unmarshal/polymorphic slot is read whole before it is decoded", func(t *testing.T) {
		const in = `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"n"},` +
			`"value":{"_type":"DV_QUANTITY","magnitude":"abc","units":"kg" x}}`
		var el rm.Element
		err := canjson.Unmarshal([]byte(in), &el)
		if se, ok := errors.AsType[*jsontext.SyntacticError](err); !ok || se == nil {
			t.Fatalf("Unmarshal(%s) err = %v (%T); want a *jsontext.SyntacticError: the slot value is read whole, so its malformed bytes win", in, err, err)
		}
		if errors.Is(err, canjson.ErrInvalidShape) {
			t.Errorf("Unmarshal(%s) err = %v; malformed bytes inside a slot must not carry canjson.ErrInvalidShape", in, err)
		}
	})

	// typereg.Default.Decode buffers its input and peeks `_type` over the
	// whole of it before the concrete decode, so the stray byte is refused
	// first even though magnitude fails DV_QUANTITY's shape earlier in the
	// document.
	t.Run("Registry.Decode/buffers and peeks first", func(t *testing.T) {
		in := shapeFirst[0].in
		_, err := typereg.Default.Decode([]byte(in))
		if se, ok := errors.AsType[*jsontext.SyntacticError](err); !ok || se == nil {
			t.Fatalf("typereg.Default.Decode(%s) err = %v (%T); want a *jsontext.SyntacticError: the registry reads the whole input before it decodes", in, err, err)
		}
		if errors.Is(err, canjson.ErrInvalidShape) {
			t.Errorf("typereg.Default.Decode(%s) err = %v; malformed input must not carry canjson.ErrInvalidShape", in, err)
		}
	})
}

// TestUnmarshalTypeMismatchLeavesReceiverAsDocumented pins what a whole-value
// `_type` mismatch leaves in the receiver on each of the two generated wire
// shapes, as ADR 0022 documents (REQ-052). The codec promises nothing about
// the receiver after an error; this test records the shape difference so a
// change to it is a conscious one. The alias shape decodes in place, so the
// receiver already holds the decoded members when the guard refuses the
// discriminator. The flat shape decodes into a separate wire struct and
// returns before its field copies, so the receiver is untouched.
func TestUnmarshalTypeMismatchLeavesReceiverAsDocumented(t *testing.T) {
	t.Run("alias shape decodes in place", func(t *testing.T) {
		const in = `{"_type":"DV_CODED_TEXT","value":"foreign","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"},"code_string":"at0001"}}`
		v := rm.DVText{Value: "orig"}
		err := canjson.Unmarshal([]byte(in), &v)
		if !errors.Is(err, typereg.ErrTypeMismatch) {
			t.Fatalf("Unmarshal(%s) into rm.DVText err = %v; want errors.Is(_, typereg.ErrTypeMismatch)", in, err)
		}
		if v.Value != "foreign" {
			t.Errorf("after the mismatch DVText.Value = %q; want %q (the alias shape decodes in place; if intentional, update ADR 0022)", v.Value, "foreign")
		}
	})
	t.Run("flat shape returns before its field copies", func(t *testing.T) {
		const in = `{"_type":"DV_TEXT","value":"foreign"}`
		v := rm.DVCodedText{Value: "orig"}
		err := canjson.Unmarshal([]byte(in), &v)
		if !errors.Is(err, typereg.ErrTypeMismatch) {
			t.Fatalf("Unmarshal(%s) into rm.DVCodedText err = %v; want errors.Is(_, typereg.ErrTypeMismatch)", in, err)
		}
		if v.Value != "orig" {
			t.Errorf("after the mismatch DVCodedText.Value = %q; want %q (the flat shape returns before its field copies; if intentional, update ADR 0022)", v.Value, "orig")
		}
	})
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
		// io.EOF is named by REQ-052 as a malformed-input failure that
		// MUST NOT acquire the shape sentinel: nothing reached a
		// generated UnmarshalJSON.
		if errors.Is(err, canjson.ErrInvalidShape) {
			t.Errorf("Decode(\"\") err = %v; an empty stream is io.EOF, not a JSON shape failure", err)
		}
		var uq rm.DVQuantity
		uerr := canjson.Unmarshal([]byte(""), &uq)
		if _, ok := errors.AsType[*jsontext.SyntacticError](uerr); !ok {
			t.Errorf("Unmarshal(\"\") err = %v; want *jsontext.SyntacticError (the divergence this test pins)", uerr)
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
		uerr := canjson.Unmarshal([]byte(in), &uq)
		if _, ok := errors.AsType[*jsontext.SyntacticError](uerr); !ok {
			t.Errorf("Unmarshal(two values) err = %v; want *jsontext.SyntacticError (the divergence this test pins)", uerr)
		}
	})
}

// TestUnmarshalOverflowIsATypedError pins the half of REQ-052's
// floating-point clause that IS met: an out-of-range magnitude fails
// with a typed error a caller can reach by errors.As, even though the
// generated UnmarshalJSON wraps it behind a `canjson: DV_QUANTITY:`
// prefix. "Typed error" here is *encoding/json/v2.SemanticError, not the
// ErrInvalidShape sentinel: wrapping the sentinel alone would not
// discharge the clause.
func TestUnmarshalOverflowIsATypedError(t *testing.T) {
	var q rm.DVQuantity
	err := canjson.Unmarshal([]byte(`{"_type":"DV_QUANTITY","magnitude":1e400,"units":"kg"}`), &q)
	if err == nil {
		t.Fatal("Unmarshal(magnitude 1e400) = nil; want a typed range error")
	}
	if _, ok := errors.AsType[*jsonv2.SemanticError](err); !ok {
		t.Errorf("err = %v (%T); want errors.AsType to reach *encoding/json/v2.SemanticError", err, err)
	}
}

// TestUnmarshalMantissaPrecisionLossIsATypedError pins the half of
// REQ-052's floating-point clause that used to be open: a magnitude
// carrying more significant digits than float64 holds (here 19) now
// fails rather than rounding silently, wrapping canjson.ErrInvalidShape
// so a caller can classify it with errors.Is alone — closed by
// docs/plans/archive/2026-09-01-rm-canonical-json-fidelity.md, which
// replaces the retired TestUnmarshalMantissaPrecisionLossIsSilent this
// pinned before the gap closed. rm.Real's own significant-digit trigger
// is unit-tested directly in openehr/rm/real_test.go; this only proves
// DV_QUANTITY.magnitude inherits it through the ordinary struct-field
// decode path — no per-field path wrapping exists for a scalar Real
// field today (only polymorphic slots and whole-value `_type` mismatches
// attach a *typereg.DecodeError), so this test does not assert one.
func TestUnmarshalMantissaPrecisionLossIsATypedError(t *testing.T) {
	const in = `{"_type":"DV_QUANTITY","magnitude":0.1234567890123456789,"units":"kg"}`
	var q rm.DVQuantity
	err := canjson.Unmarshal([]byte(in), &q)
	if err == nil {
		t.Fatalf("Unmarshal(%s) = nil; want a precision error (gap closed)", in)
	}
	if !errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; want errors.Is(err, canjson.ErrInvalidShape)", err)
	}
}

// TestUnmarshalMantissaPrecisionLossInheritedByDVProportion is the same
// assertion over DV_PROPORTION.numerator, proving the check is inherited
// by every Real-typed field, not special-cased to DV_QUANTITY.
func TestUnmarshalMantissaPrecisionLossInheritedByDVProportion(t *testing.T) {
	const in = `{"_type":"DV_PROPORTION","numerator":0.1234567890123456789,"denominator":1,"type":0}`
	var p rm.DVProportion
	err := canjson.Unmarshal([]byte(in), &p)
	if err == nil {
		t.Fatalf("Unmarshal(%s) = nil; want a precision error", in)
	}
	if !errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; want errors.Is(err, canjson.ErrInvalidShape)", err)
	}
}

// TestUnmarshalDuplicateMemberNameWrapsErrInvalidShape pins REQ-052's
// duplicate-member-name clause: an object carrying the same member name twice
// is refused, and the refusal wraps canjson.ErrInvalidShape. RFC 8259 § 4 says
// names SHOULD be unique, and an object with duplicates has no single defined
// value. jsontext raises jsontext.ErrDuplicateName during tokenisation, before
// any generated decode runs, so the entry point is where canjson attaches the
// classification; the operation-specific cause stays reachable. The map target
// is the case where that entry-point attachment is the only thing adding the
// sentinel, because no generated type's funnel sits on the path.
//
// Can-fail control: delete the typereg.ClassifyShape wrap in canjson's
// classifyDecode and the error still occurs but loses the sentinel, so the
// ErrInvalidShape assertion goes red while the jsontext.ErrDuplicateName one
// stays green. Threading jsontext.AllowDuplicateNames(true) into the
// entry-point options turns both cases red (checked by overlay): the concrete
// decode and the slot peek both run under the caller's options, so nothing
// re-validates the value with v2 defaults.
func TestUnmarshalDuplicateMemberNameWrapsErrInvalidShape(t *testing.T) {
	// A duplicate "units": the tokens are valid, the object is not.
	const in = `{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg","units":"g"}`
	cases := []struct {
		name   string
		target func() any
	}{
		{"generated RM type", func() any { return &rm.DVQuantity{} }},
		{"map[string]any", func() any { return &map[string]any{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := canjson.Unmarshal([]byte(in), tc.target())
			if err == nil {
				t.Fatalf("Unmarshal(%s) = nil; want a duplicate-name refusal", in)
			}
			// The shared sentinel, so a caller classifies with errors.Is alone.
			if !errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("err = %v; want errors.Is(_, canjson.ErrInvalidShape)", err)
			}
			// The operation-specific facet: the duplicate-name cause itself, not
			// merely the shared sentinel a different shape failure would also carry.
			if !errors.Is(err, jsontext.ErrDuplicateName) {
				t.Errorf("err = %v; want errors.Is(_, jsontext.ErrDuplicateName)", err)
			}
			// Message control for the single-pass concrete decode (REQ-052): a
			// duplicate-name refusal is a *jsontext.SyntacticError the typereg
			// helper passes through unwrapped, so it never picks up a generated
			// type's `canjson: <RM_TYPE>:` funnel prefix. Can-fail: route that
			// syntactic error through typereg.WrapShapeError in typereg's
			// classifyDecode and the generated-RM-type case gains
			// `canjson: DV_QUANTITY:`, turning this red (the map case has no
			// funnel on its path, so it cannot gain a prefix and stays green).
			if strings.Contains(err.Error(), "canjson: DV_QUANTITY:") {
				t.Errorf("err = %v; a duplicate-name refusal must not gain the DV_QUANTITY funnel prefix", err)
			}
			assertShapeSentinelDistinct(t, err)
		})
	}
}

// TestDecoderDecodeDuplicateMemberNameWrapsErrInvalidShape is the streaming
// twin: Decoder.Decode classifies a duplicate member name the same way
// Unmarshal does, so the guarantee does not depend on which entry point a
// caller reaches.
func TestDecoderDecodeDuplicateMemberNameWrapsErrInvalidShape(t *testing.T) {
	const in = `{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg","units":"g"}`
	var q rm.DVQuantity
	err := canjson.NewDecoder(strings.NewReader(in)).Decode(&q)
	if err == nil {
		t.Fatalf("Decode(%s) = nil; want a duplicate-name refusal", in)
	}
	if !errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; want errors.Is(_, canjson.ErrInvalidShape)", err)
	}
	if !errors.Is(err, jsontext.ErrDuplicateName) {
		t.Errorf("err = %v; want errors.Is(_, jsontext.ErrDuplicateName)", err)
	}
	// Message control for the single-pass concrete decode (REQ-052): the
	// duplicate-name refusal is a *jsontext.SyntacticError the typereg helper
	// passes through unwrapped, so DV_QUANTITY's funnel prefix stays off it.
	// Can-fail: route that syntactic error through typereg.WrapShapeError in
	// typereg's classifyDecode and the message gains `canjson: DV_QUANTITY:`.
	if strings.Contains(err.Error(), "canjson: DV_QUANTITY:") {
		t.Errorf("err = %v; a duplicate-name refusal must not gain the DV_QUANTITY funnel prefix", err)
	}
	assertShapeSentinelDistinct(t, err)
}

// TestUnmarshalMatchesMemberNamesExactly pins REQ-052's exact-case member
// matching: canonical-JSON member names match case-sensitively on this codec
// path (the case-insensitive Extras rule binds only the Definition, System and
// AQL surfaces, which stay on v1). encoding/json/v2 matches names exactly by
// default, and canjson sets no case-insensitive option, so a wrongly-cased
// member does not populate its field. This is a behaviour change from the v1
// codec, which matched a struct field case-insensitively.
//
// Can-fail control: thread jsonv2.MatchCaseInsensitiveNames(true) into the
// entry-point options and "Magnitude" would populate the field, so the
// zero-magnitude assertion goes red.
func TestUnmarshalMatchesMemberNamesExactly(t *testing.T) {
	// "Magnitude" is mis-cased; "units" is exact. The mis-cased member is an
	// unknown key the codec ignores (canjson does not reject unknown members),
	// so it must not reach the lowercase magnitude field.
	in := []byte(`{"_type":"DV_QUANTITY","Magnitude":1,"units":"kg"}`)
	var q rm.DVQuantity
	if err := canjson.Unmarshal(in, &q); err != nil {
		t.Fatalf("Unmarshal(%s) = %v; want nil (a mis-cased member is ignored, not an error)", in, err)
	}
	// The operation-specific facet: the exactly-cased member populated, the
	// mis-cased one did not.
	if q.Magnitude != 0 {
		t.Errorf("q.Magnitude = %v; want 0: a mis-cased \"Magnitude\" must not match the field", q.Magnitude)
	}
	if q.Units != "kg" {
		t.Errorf("q.Units = %q; want \"kg\": the exactly-cased member must populate", q.Units)
	}
}

// errBoom is a distinct reader failure the failing-reader control below finds
// with errors.Is, proving the reader's own error survives the classification.
var errBoom = errors.New("boom")

// prefixThenErrReader serves prefix, then returns err on the next Read, so a
// reader failure lands partway through a valid value on the streaming entry.
type prefixThenErrReader struct {
	prefix []byte
	off    int
	err    error
}

func (r *prefixThenErrReader) Read(p []byte) (int, error) {
	if r.off < len(r.prefix) {
		n := copy(p, r.prefix[r.off:])
		r.off += n
		return n, nil
	}
	return 0, r.err
}

// TestDecoderDecodeFailingReaderIsNotShapeTagged pins that a reader failing
// partway through a valid body is malformed input, not a JSON shape failure
// (REQ-052). jsontext returns the reader's error as its own IO error type, which
// is neither a *jsontext.SyntacticError nor errors.Is-equal to io.EOF, so an
// EOF-only pass-through would wrap it as canjson.ErrInvalidShape. typereg's
// classifyDecode passes through any error whose chain carries neither a
// *json.SemanticError nor a *DecodeError, so the reader failure keeps no
// sentinel and its own error stays reachable.
//
// Can-fail control: narrow the pass-through in typereg.classifyDecode back to
// the EOF-only form (return err only for a *jsontext.SyntacticError or
// io.EOF/io.ErrUnexpectedEOF) and this reader error acquires
// canjson.ErrInvalidShape, turning the sentinel assertion red.
func TestDecoderDecodeFailingReaderIsNotShapeTagged(t *testing.T) {
	// A prefix of a valid DV_QUANTITY body, cut mid-token so the decode is still
	// in progress when the reader fails.
	r := &prefixThenErrReader{prefix: []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"un`), err: errBoom}
	var q rm.DVQuantity
	err := canjson.NewDecoder(r).Decode(&q)
	if err == nil {
		t.Fatal("Decode(failing reader) = nil; want the reader's error")
	}
	if errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("err = %v; a failing reader is malformed input, not a JSON shape failure, so it must not carry ErrInvalidShape", err)
	}
	if !errors.Is(err, errBoom) {
		t.Errorf("err = %v; want the reader's own error to stay reachable with errors.Is", err)
	}
	assertShapeSentinelDistinct(t, err)
}

// TestBareV1CallerOptionsGovernPolymorphicSlots pins ruling P17 (REQ-052): the
// caller's decoder options govern the whole decode, polymorphic slots included.
// DecodePolymorphic reads a slot with the caller's decoder and then peeks its
// `_type` under the same options, so a bare v1 encoding/json caller inherits
// v1's leniencies inside ELEMENT.name exactly as it does at top level, while
// canjson (encoding/json/v2 defaults) refuses the same bytes, each refusal
// classified as REQ-052 requires.
//
// Can-fail (checked with go test -overlay on a copy of streaming.go whose
// DecodePolymorphic calls peekType(raw) with no options): the v1 "invalid
// UTF-8" and "duplicate name" cases turn red, because the v2-default peek
// refuses what the v1 tokenizer accepted and the refusal gains ErrInvalidShape
// through WrapShapeError. The v1 "_TYPE" case turns red as well: the
// exact-matching peek does not see `_TYPE`, so the narrow slot falls back to
// DV_TEXT while v1's concrete decode would have matched it.
func TestBareV1CallerOptionsGovernPolymorphicSlots(t *testing.T) {
	const head = `{"_type":"ELEMENT","archetype_node_id":"at0001","name":`
	cases := []struct {
		name string
		in   string
	}{
		// 0xff is never valid UTF-8.
		{"invalid UTF-8 in name.value", head + "{\"_type\":\"DV_TEXT\",\"value\":\"a\xffb\"}}"},
		{"duplicate value in name", head + `{"_type":"DV_TEXT","value":"first","value":"last"}}`},
		{"_TYPE in capitals in name", head + `{"_TYPE":"DV_CODED_TEXT","value":"n","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"},"code_string":"at0001"}}}`},
	}

	t.Run("bare v1", func(t *testing.T) {
		t.Run(cases[0].name, func(t *testing.T) {
			var e rm.Element
			if err := jsonv1.Unmarshal([]byte(cases[0].in), &e); err != nil {
				t.Fatalf("jsonv1.Unmarshal(%q) = %v; want nil: a v1 caller accepts invalid UTF-8 inside a slot as at top level", cases[0].in, err)
			}
			name, ok := e.Name.(*rm.DVText)
			if !ok {
				t.Fatalf("Element.Name is %T; want *rm.DVText", e.Name)
			}
			if !strings.ContainsRune(name.Value, '�') {
				t.Errorf("Element.Name.Value = %q; want it to contain U+FFFD (v1 substitutes invalid UTF-8)", name.Value)
			}
		})
		t.Run(cases[1].name, func(t *testing.T) {
			var e rm.Element
			if err := jsonv1.Unmarshal([]byte(cases[1].in), &e); err != nil {
				t.Fatalf("jsonv1.Unmarshal(%s) = %v; want nil: a v1 caller accepts a duplicate name inside a slot as at top level", cases[1].in, err)
			}
			name, ok := e.Name.(*rm.DVText)
			if !ok {
				t.Fatalf("Element.Name is %T; want *rm.DVText", e.Name)
			}
			if name.Value != "last" {
				t.Errorf("Element.Name.Value = %q; want %q (v1 keeps the last duplicate)", name.Value, "last")
			}
		})
		t.Run(cases[2].name, func(t *testing.T) {
			var e rm.Element
			if err := jsonv1.Unmarshal([]byte(cases[2].in), &e); err != nil {
				t.Fatalf("jsonv1.Unmarshal(%s) = %v; want nil", cases[2].in, err)
			}
			if _, ok := e.Name.(*rm.DVCodedText); !ok {
				t.Errorf("Element.Name is %T; want *rm.DVCodedText: v1 matches `_TYPE` case-insensitively, so the slot dispatches on it", e.Name)
			}
		})
		// The fourth leniency the canjson doc bullet lists: v1 validates the
		// whole input before it decodes, so a stray byte after a member that
		// fails the target's shape is reported as a syntax error first.
		t.Run("malformed bytes after a shape failure", func(t *testing.T) {
			const in = `{"_type":"DV_QUANTITY","magnitude":"abc","units":"kg" x}`
			var q rm.DVQuantity
			err := jsonv1.Unmarshal([]byte(in), &q)
			if se, ok := errors.AsType[*jsonv1.SyntaxError](err); !ok || se == nil {
				t.Fatalf("jsonv1.Unmarshal(%s) err = %v (%T); want a *json.SyntaxError: v1 reports malformed bytes ahead of a shape failure", in, err, err)
			}
			if errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("jsonv1.Unmarshal(%s) err = %v; malformed input must not carry canjson.ErrInvalidShape", in, err)
			}
		})
	})

	t.Run("canjson", func(t *testing.T) {
		t.Run(cases[0].name, func(t *testing.T) {
			var e rm.Element
			err := canjson.Unmarshal([]byte(cases[0].in), &e)
			if se, ok := errors.AsType[*jsontext.SyntacticError](err); !ok || se == nil {
				t.Fatalf("canjson.Unmarshal(%q) err = %v (%T); want a *jsontext.SyntacticError for invalid UTF-8", cases[0].in, err, err)
			}
			if errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("canjson.Unmarshal(%q) err = %v; invalid UTF-8 is malformed input and must not carry canjson.ErrInvalidShape", cases[0].in, err)
			}
		})
		t.Run(cases[1].name, func(t *testing.T) {
			var e rm.Element
			err := canjson.Unmarshal([]byte(cases[1].in), &e)
			if !errors.Is(err, canjson.ErrInvalidShape) || !errors.Is(err, jsontext.ErrDuplicateName) {
				t.Errorf("canjson.Unmarshal(%s) err = %v; want errors.Is for both canjson.ErrInvalidShape and jsontext.ErrDuplicateName", cases[1].in, err)
			}
		})
		t.Run(cases[2].name, func(t *testing.T) {
			// ELEMENT.name is a narrow slot (DV_TEXT is its declared parent), so a
			// `_TYPE` that exact matching does not see is a missing `_type`, and
			// the slot falls back to DV_TEXT rather than refusing.
			var e rm.Element
			if err := canjson.Unmarshal([]byte(cases[2].in), &e); err != nil {
				t.Fatalf("canjson.Unmarshal(%s) = %v; want the narrow-slot fallback to DV_TEXT", cases[2].in, err)
			}
			if _, ok := e.Name.(*rm.DVText); !ok {
				t.Errorf("Element.Name is %T; want *rm.DVText: canjson matches names exactly, so `_TYPE` is not the discriminator and the narrow slot falls back to its parent", e.Name)
			}
		})
	})

	// A caller's RejectUnknownMembers binds the concrete decode, not the
	// `_type` peek, whose head struct declares only `_type`. Can-fail: drop the
	// RejectUnknownMembers(false) override in typereg.peekType and the peek
	// refuses `value` inside name, turning this red.
	t.Run("bare v2 RejectUnknownMembers does not reach the peek", func(t *testing.T) {
		in := head + `{"_type":"DV_TEXT","value":"n"}}`
		var e rm.Element
		if err := jsonv2.Unmarshal([]byte(in), &e, typereg.Unmarshalers(), jsonv2.RejectUnknownMembers(true)); err != nil {
			t.Fatalf("jsonv2.Unmarshal(%s, RejectUnknownMembers(true)) = %v; want nil: every member is declared", in, err)
		}
	})
}
