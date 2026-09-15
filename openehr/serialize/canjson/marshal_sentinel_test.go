package canjson_test

import (
	json "encoding/json/v2"
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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
// *json.SemanticError underneath is still reachable.
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
	if _, ok := errors.AsType[*json.SemanticError](err); !ok {
		t.Errorf("underlying *json.SemanticError must stay reachable; got %v", err)
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

// TestMarshalRefusesInvalidUTF8ThroughPublicEntry pins ADR 0022: invalid UTF-8
// in a Go string reached on encode is refused through canjson.Marshal with
// ErrInvalidValue, not only when Character.MarshalJSON is called directly
// (character_test.go). Can-fail control: drop the ErrInvalidValue wrap in
// Marshal and this test still fails, but only on the direct Character path.
func TestMarshalRefusesInvalidUTF8ThroughPublicEntry(t *testing.T) {
	tm := rm.TermMapping{Match: rm.Character(string([]byte{0xff}))}
	got, err := canjson.Marshal(&tm)
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

// TestMarshalIndentRefusesNonWhitespaceIndent pins R23: encoding/json/v2's
// indent options accept only spaces and tabs and panic on anything else, where
// the v1 codec accepted any character. MarshalIndent refuses a bad prefix or
// indent with ErrInvalidValue before building any option, so the call returns
// an error rather than letting a panic cross the package boundary (REQ-025).
//
// Can-fail control: drop the strings.Trim guard in MarshalIndent and the
// newline-prefix and letter-indent cases panic (recovered here as a fatal test
// failure) instead of returning ErrInvalidValue.
func TestMarshalIndentRefusesNonWhitespaceIndent(t *testing.T) {
	cases := []struct {
		name           string
		prefix, indent string
		wantErr        bool
	}{
		{"newline prefix", "\n", "  ", true},
		{"letter indent", "", "x", true},
		{"spaces and tab accepted", " \t", " \t", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				out []byte
				err error
			)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("MarshalIndent panicked on prefix=%q indent=%q: %v; a public entry point must return an error, not panic (REQ-025)", tc.prefix, tc.indent, r)
					}
				}()
				out, err = canjson.MarshalIndent(map[string]int{"a": 1}, tc.prefix, tc.indent)
			}()
			if tc.wantErr {
				if !errors.Is(err, canjson.ErrInvalidValue) {
					t.Errorf("MarshalIndent(prefix=%q, indent=%q) err = %v; want errors.Is(_, canjson.ErrInvalidValue)", tc.prefix, tc.indent, err)
				}
				if out != nil {
					t.Errorf("no bytes on refusal; got %q", out)
				}
				return
			}
			if err != nil {
				t.Errorf("MarshalIndent(prefix=%q, indent=%q) err = %v; want nil (spaces and tabs are allowed)", tc.prefix, tc.indent, err)
			}
		})
	}
}
