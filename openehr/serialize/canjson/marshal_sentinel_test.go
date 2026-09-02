package canjson_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// REQ-052 — encode-side refusal sentinel. A Marshal / MarshalIndent
// failure wraps the encode-only canjson.ErrInvalidValue, which stays
// errors.Is-distinguishable from the decode-side canjson.ErrInvalidShape
// and from the transport-level transport.ErrInvalidShape, while the
// underlying encoder error stays reachable through unwrapping.

// assertEncodeRefusal pins the whole clause on one returned error:
// the encode sentinel matches, neither decode sentinel does, and the
// *json.UnsupportedTypeError underneath is still reachable.
func assertEncodeRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error for a value the encoder cannot represent")
	}
	if !errors.Is(err, canjson.ErrInvalidValue) {
		t.Errorf("must wrap canjson.ErrInvalidValue; got %v", err)
	}
	if errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("encode failure must not match the decode-side canjson.ErrInvalidShape; got %v", err)
	}
	if errors.Is(err, transport.ErrInvalidShape) {
		t.Errorf("encode failure must not match transport.ErrInvalidShape; got %v", err)
	}
	if _, ok := errors.AsType[*json.UnsupportedTypeError](err); !ok {
		t.Errorf("underlying *json.UnsupportedTypeError must stay reachable; got %v", err)
	}
}

// REQ-052
func TestMarshalRefusalWrapsErrInvalidValue(t *testing.T) {
	got, err := canjson.Marshal(make(chan int))
	if got != nil {
		t.Errorf("no bytes on refusal; got %q", got)
	}
	assertEncodeRefusal(t, err)
}

// REQ-052
func TestMarshalIndentRefusalWrapsErrInvalidValue(t *testing.T) {
	got, err := canjson.MarshalIndent(make(chan int), "", "  ")
	if got != nil {
		t.Errorf("no bytes on refusal; got %q", got)
	}
	assertEncodeRefusal(t, err)
}

// TestMarshalSuccessIsUnaffectedBySentinel pins that the wrap is
// failure-only: a marshalable value still yields its exact canonical
// bytes and a nil error (REQ-052).
func TestMarshalSuccessIsUnaffectedBySentinel(t *testing.T) {
	got, err := canjson.Marshal(map[string]int{"b": 2, "a": 1})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if want := `{"a":1,"b":2}`; string(got) != want {
		t.Errorf("bytes must pass through unchanged: got %s, want %s", got, want)
	}

	indented, err := canjson.MarshalIndent(map[string]int{"a": 1}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if want := "{\n  \"a\": 1\n}"; string(indented) != want {
		t.Errorf("bytes must pass through unchanged: got %q, want %q", indented, want)
	}
}

// TestDecodeFailureDoesNotCarryErrInvalidValue guards the other half
// of the distinctness rule: the encode-only sentinel MUST NOT appear
// on any decode path (REQ-052). The input here is malformed JSON,
// which encoding/json rejects before any generated UnmarshalJSON runs,
// so it carries no decode sentinel either; where canjson.ErrInvalidShape
// does and does not attach is pinned in decode_test.go.
func TestDecodeFailureDoesNotCarryErrInvalidValue(t *testing.T) {
	var into map[string]any
	err := canjson.Unmarshal([]byte(`{"a":`), &into)
	if err == nil {
		t.Fatal("expected a decode error for truncated JSON")
	}
	if errors.Is(err, canjson.ErrInvalidValue) {
		t.Errorf("encode-only sentinel must not appear on a decode path; got %v", err)
	}
}
