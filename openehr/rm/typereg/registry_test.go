package typereg

import (
	"errors"
	"strings"
	"testing"
)

type fakeBox struct {
	Type    string `json:"_type"`
	Payload string `json:"payload"`
}

func TestRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register("FAKE_BOX", func() any { return &fakeBox{} })
	ctor, ok := r.Lookup("FAKE_BOX")
	if !ok {
		t.Fatal("Lookup miss after Register")
	}
	v := ctor()
	if _, ok := v.(*fakeBox); !ok {
		t.Errorf("ctor returned %T, want *fakeBox", v)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	r := NewRegistry()
	r.Register("X", func() any { return &fakeBox{} })
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic on duplicate Register")
		}
	}()
	r.Register("X", func() any { return &fakeBox{} })
}

func TestDecode(t *testing.T) {
	r := NewRegistry()
	r.Register("FAKE_BOX", func() any { return &fakeBox{} })
	v, err := r.Decode([]byte(`{"_type":"FAKE_BOX","payload":"hi"}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	box, ok := v.(*fakeBox)
	if !ok {
		t.Fatalf("Decode returned %T", v)
	}
	if box.Payload != "hi" {
		t.Errorf("payload = %q, want %q", box.Payload, "hi")
	}
}

func TestDecodeUnknownType(t *testing.T) {
	r := NewRegistry()
	_, err := r.Decode([]byte(`{"_type":"UNKNOWN"}`))
	if err == nil {
		t.Fatal("expected error for unknown _type")
	}
	if !strings.Contains(err.Error(), "UNKNOWN") {
		t.Errorf("error %q should mention UNKNOWN", err)
	}
}

func TestDecodeMissingType(t *testing.T) {
	r := NewRegistry()
	_, err := r.Decode([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error for missing _type")
	}
}

// Sentinels: callers MUST be able to distinguish missing _type from
// unknown _type via errors.Is so that the canjson codec can wrap them
// in a DecodeError without dropping the kind. PROBE-031 asserts
// ErrUnknownType specifically.
func TestDecodeMissingTypeIsErrMissingType(t *testing.T) {
	r := NewRegistry()
	_, err := r.Decode([]byte(`{"foo":"bar"}`))
	if err == nil {
		t.Fatal("expected error for missing _type")
	}
	if !errors.Is(err, ErrMissingType) {
		t.Errorf("err = %v; want errors.Is(_, ErrMissingType)", err)
	}
	// REQ-052: a dispatch failure stays outside the shape sentinel.
	if errors.Is(err, ErrInvalidShape) {
		t.Errorf("err = %v; a missing `_type` is a dispatch failure, not a JSON shape error", err)
	}
}

func TestDecodeUnknownTypeIsErrUnknownType(t *testing.T) {
	r := NewRegistry()
	_, err := r.Decode([]byte(`{"_type":"UNKNOWN"}`))
	if err == nil {
		t.Fatal("expected error for unknown _type")
	}
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("err = %v; want errors.Is(_, ErrUnknownType)", err)
	}
}

func TestDecodeAsConcreteValueTypeParameter(t *testing.T) {
	const typeName = "FAKE_BOX_VALUE_T"
	if _, ok := Default.Lookup(typeName); !ok {
		Default.Register(typeName, func() any { return &fakeBox{} })
	}
	got, err := DecodeAs[fakeBox]([]byte(`{"_type":"FAKE_BOX_VALUE_T","payload":"ok"}`))
	if err != nil {
		t.Fatalf("DecodeAs[fakeBox]: %v", err)
	}
	if got.Payload != "ok" {
		t.Errorf("Payload = %q, want %q", got.Payload, "ok")
	}
}

func TestDecodeAsTypeMismatchIsErrTypeMismatch(t *testing.T) {
	r := NewRegistry()
	r.Register("FAKE_BOX", func() any { return &fakeBox{} })
	// DecodeAs operates on Default — register there too for this test.
	if _, ok := Default.Lookup("FAKE_BOX_ASGUARD"); !ok {
		Default.Register("FAKE_BOX_ASGUARD", func() any { return &fakeBox{} })
	}
	type unrelated interface{ unrelated() }
	_, err := DecodeAs[unrelated]([]byte(`{"_type":"FAKE_BOX_ASGUARD"}`))
	if err == nil {
		t.Fatal("expected type-mismatch error")
	}
	if !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("err = %v; want errors.Is(_, ErrTypeMismatch)", err)
	}
}

// TestDecode_maxDepthExceeded verifies that Decode rejects input whose
// JSON nesting exceeds maxDecodeDepth before dispatch (so the test does
// NOT need a registered type — the depth check runs first).
func TestDecode_maxDepthExceeded(t *testing.T) {
	// Build a document nested ~2000 levels deep: far above maxDecodeDepth (512).
	const depth = 2000
	inner := `{"_type":"X"}`
	for range depth {
		inner = `{"_type":"X","x":` + inner + `}`
	}
	r := NewRegistry()
	_, err := r.Decode([]byte(inner))
	if err == nil {
		t.Fatal("expected error for deeply-nested document, got nil")
	}
	if !errors.Is(err, ErrMaxDepthExceeded) {
		t.Errorf("err = %v; want errors.Is(_, ErrMaxDepthExceeded)", err)
	}
}

// TestDecode_shallowOK verifies that a shallow (non-deep) document with
// an unknown _type still reaches dispatch and returns ErrUnknownType —
// i.e. the depth guard does not introduce false positives.
func TestDecode_shallowOK(t *testing.T) {
	r := NewRegistry()
	_, err := r.Decode([]byte(`{"_type":"NOPE"}`))
	if err == nil {
		t.Fatal("expected error for unknown _type, got nil")
	}
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("err = %v; want errors.Is(_, ErrUnknownType)", err)
	}
}

// TestJSONNestingDepth exercises the depth scanner with hand-crafted
// literals, including braces inside strings (which must not be counted).
func TestJSONNestingDepth(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{`{}`, 1},
		{`{"a":{"b":[1,2]}}`, 3},
		// Braces inside a string value must not be counted.
		{`{"a":"}}}"} `, 1},
		// Array of objects.
		{`[{"a":1},{"b":2}]`, 2},
		// Escape before closing quote — the \" inside the string should
		// not end the string early.
		{`{"a":"x\"}"} `, 1},
	}
	for _, tc := range cases {
		got := jsonNestingDepth([]byte(tc.input))
		if got != tc.want {
			t.Errorf("jsonNestingDepth(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// TestJSONNestingDepth_boundary pins the exact fence at maxDecodeDepth:
// a value nested exactly to the cap is accepted (depth == cap), while
// cap+1 is reported over (the early-exit returns a value > cap). Guards
// against an off-by-one slipping the > maxDecodeDepth check to >=.
func TestJSONNestingDepth_boundary(t *testing.T) {
	nested := func(n int) []byte {
		return []byte(strings.Repeat("[", n) + strings.Repeat("]", n))
	}
	if got := jsonNestingDepth(nested(maxDecodeDepth)); got != maxDecodeDepth {
		t.Errorf("depth at cap: got %d, want %d (must be accepted)", got, maxDecodeDepth)
	}
	if got := jsonNestingDepth(nested(maxDecodeDepth + 1)); got <= maxDecodeDepth {
		t.Errorf("depth cap+1: got %d, want > %d (must be rejected)", got, maxDecodeDepth)
	}
}

// TestWrapShapeErrorKeepsTextAndAddsSentinel pins the two halves of the
// REQ-052 wrap that the canjson end-to-end tests can only see through a
// substring: the message is EXACTLY the historical
// `canjson: <RM_TYPE>: <cause>` text, and the cause stays reachable
// beside the added ErrInvalidShape classification.
func TestWrapShapeErrorKeepsTextAndAddsSentinel(t *testing.T) {
	cause := errors.New("json: cannot unmarshal string into Go value of type float64")
	err := WrapShapeError("DV_QUANTITY", cause)

	if want := "canjson: DV_QUANTITY: " + cause.Error(); err.Error() != want {
		t.Errorf("Error() = %q; want %q — the wrap must not change the text", err.Error(), want)
	}
	if !errors.Is(err, ErrInvalidShape) {
		t.Errorf("err = %v; want errors.Is(_, ErrInvalidShape)", err)
	}
	if !errors.Is(err, cause) {
		t.Errorf("err = %v; the classification must not cost the cause", err)
	}
	// Single-step unwrapping must land on the cause, not on nil: a
	// multi-error Unwrap would satisfy errors.Is/As but break every
	// caller that walks the chain one hop at a time. The comparison is
	// identity on purpose — errors.Is would also pass if Unwrap returned
	// some other wrapper around the cause, which is the failure this
	// assertion exists to catch.
	if got := errors.Unwrap(err); got != cause { //nolint:errorlint // identity by design — see above
		t.Errorf("errors.Unwrap(err) = %v; want the cause %v — one unwrap step must reach it", got, cause)
	}
}

// TestWrapShapeErrorLeavesDecodeErrorUnclassified pins the boundary: a
// nested polymorphic failure travelling out through an enclosing type's
// UnmarshalJSON keeps its DecodeError classification and does NOT become
// a shape error — ErrInvalidShape means "JSON-level shape", not "any
// decode failure" (REQ-052).
func TestWrapShapeErrorLeavesDecodeErrorUnclassified(t *testing.T) {
	inner := &DecodeError{Path: "/lower", Inner: ErrUnknownType}
	err := WrapShapeError("DV_QUANTITY", inner)

	if want := "canjson: DV_QUANTITY: " + inner.Error(); err.Error() != want {
		t.Errorf("Error() = %q; want %q", err.Error(), want)
	}
	if errors.Is(err, ErrInvalidShape) {
		t.Errorf("err = %v; a nested DecodeError must not be re-classified as a shape error", err)
	}
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("err = %v; want the polymorphic classification to survive the wrap", err)
	}
	if _, ok := errors.AsType[*DecodeError](err); !ok {
		t.Errorf("err = %v; want errors.As to still reach *DecodeError", err)
	}
}

// TestWrapShapeErrorNilIsNil guards the success path: a nil cause must
// not be turned into an error by the wrap.
func TestWrapShapeErrorNilIsNil(t *testing.T) {
	if err := WrapShapeError("DV_QUANTITY", nil); err != nil {
		t.Errorf("WrapShapeError(_, nil) = %v; want nil", err)
	}
}
