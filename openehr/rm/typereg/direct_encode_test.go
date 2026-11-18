package typereg_test

import (
	"bytes"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestREQ052DirectEncodeGate pins which encoders let a generated MarshalJSONTo
// write its members itself: each option under which json/v2 writes a member,
// or words a failure, otherwise turns DirectEncode false, and an option that
// changes neither leaves it true. Dropping a check from DirectEncode turns its
// case red.
func TestREQ052DirectEncodeGate(t *testing.T) {
	t.Parallel()
	marshalers := json.WithMarshalers(json.MarshalToFunc(func(*jsontext.Encoder, struct{}) error { return nil }))
	for _, tc := range []struct {
		name string
		opts []json.Options
		want bool
	}{
		{"no option", nil, true},
		{"the canonical-JSON options", []json.Options{typereg.EncodeOptions()}, true},
		{"indentation and escaping", []json.Options{jsontext.WithIndent("  "), jsontext.EscapeForHTML(true), jsontext.AllowInvalidUTF8(true)}, true},
		{"no marshalers", []json.Options{json.WithMarshalers(nil)}, true},
		{"OmitZeroStructFields off", []json.Options{json.OmitZeroStructFields(false)}, true},
		{"v1 method calls, which change no class", []json.Options{jsonv1.CallMethodsWithLegacySemantics(true)}, true},
		{"a caller's marshalers", []json.Options{marshalers}, false},
		{"OmitZeroStructFields", []json.Options{json.OmitZeroStructFields(true)}, false},
		{"StringifyNumbers", []json.Options{json.StringifyNumbers(true)}, false},
		{"v1 omitempty", []json.Options{jsonv1.OmitEmptyWithLegacySemantics(true)}, false},
		{"v1 error reporting", []json.Options{jsonv1.ReportErrorsWithLegacySemantics(true)}, false},
		{"encoding/json defaults", []json.Options{jsonv1.DefaultOptionsV1()}, false},
	} {
		enc := jsontext.NewEncoder(io.Discard, tc.opts...)
		if got := typereg.DirectEncode(enc); got != tc.want {
			t.Errorf("DirectEncode(encoder with %s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestREQ052DirectEncodeGateDepth pins that a value nested more than 900
// levels deep takes the wrapper, so json/v2, which looks for a pointer cycle
// from 1000 levels down, sees every pointer below.
func TestREQ052DirectEncodeGateDepth(t *testing.T) {
	t.Parallel()
	enc := jsontext.NewEncoder(io.Discard)
	for depth := 1; depth <= 901; depth++ {
		if err := enc.WriteToken(jsontext.BeginArray); err != nil {
			t.Fatal(err)
		}
		if got, want := typereg.DirectEncode(enc), depth <= 900; got != want {
			t.Fatalf("DirectEncode at depth %d = %v, want %v", depth, got, want)
		}
	}
}

// failingClass is a MarshalJSONTo json/v2 calls, which writes prefix and then
// fails with err.
type failingClass struct {
	prefix []jsontext.Token
	err    error
}

func (f failingClass) MarshalJSONTo(enc *jsontext.Encoder) error {
	for _, tok := range f.prefix {
		if err := enc.WriteToken(tok); err != nil {
			return err
		}
	}
	return f.err
}

// errRefused is the refusal a failing class returns.
var errRefused = errors.New("refused")

// TestREQ052EncodeFailureSettlesAsJSONV2 holds the settled failure of a
// generated MarshalJSONTo to the error json/v2 makes of the same failure at a
// method boundary: the same Go type, the same JSON Pointer and the same
// cause. Each case marshals a value holding a [failingClass] through json/v2,
// then writes the same tokens on an encoder of its own, fails the same way and
// settles the failure with the helper the generated code calls. The cases put
// the failure in an object member, in a list element and at the root, before
// and after the class wrote anything.
func TestREQ052EncodeFailureSettlesAsJSONV2(t *testing.T) {
	t.Parallel()
	open := []jsontext.Token{jsontext.BeginObject, jsontext.String("k")}
	syntactic := &jsontext.SyntacticError{Err: errRefused}
	for _, tc := range []struct {
		name      string
		container func(failingClass) any
		before    []jsontext.Token // what json/v2 writes ahead of the class
		class     failingClass
		settle    func(enc *jsontext.Encoder, err error) error
	}{
		{
			name:      "refusal in a member",
			container: func(f failingClass) any { return map[string]any{"m": f} },
			before:    []jsontext.Token{jsontext.BeginObject, jsontext.String("m")},
			class:     failingClass{err: errRefused},
			settle:    typereg.EncodeRefusal[failingClass],
		},
		{
			name:      "refusal in a list element",
			container: func(f failingClass) any { return []any{1, f} },
			before:    []jsontext.Token{jsontext.BeginArray, jsontext.Int(1)},
			class:     failingClass{err: errRefused},
			settle:    typereg.EncodeRefusal[failingClass],
		},
		{
			name:      "refusal at the root",
			container: func(f failingClass) any { return f },
			class:     failingClass{err: errRefused},
			settle:    typereg.EncodeRefusal[failingClass],
		},
		{
			name:      "failure inside the class's object, in a list element",
			container: func(f failingClass) any { return []any{1, f} },
			before:    []jsontext.Token{jsontext.BeginArray, jsontext.Int(1)},
			class:     failingClass{prefix: open, err: syntactic},
			settle: func(enc *jsontext.Encoder, err error) error {
				return typereg.EncodeFailure[failingClass](enc, true, err)
			},
		},
		{
			name:      "failure inside the class's object, after a whole member",
			container: func(f failingClass) any { return map[string]any{"m": f} },
			before:    []jsontext.Token{jsontext.BeginObject, jsontext.String("m")},
			class:     failingClass{prefix: append(open, jsontext.Int(2)), err: syntactic},
			settle: func(enc *jsontext.Encoder, err error) error {
				return typereg.EncodeFailure[failingClass](enc, true, err)
			},
		},
		{
			name:      "failure before the class wrote anything, in a list element",
			container: func(f failingClass) any { return []any{1, f} },
			before:    []jsontext.Token{jsontext.BeginArray, jsontext.Int(1)},
			class:     failingClass{err: syntactic},
			settle: func(enc *jsontext.Encoder, err error) error {
				return typereg.EncodeFailure[failingClass](enc, false, err)
			},
		},
		{
			name:      "failure through the wrapper, after a whole member",
			container: func(f failingClass) any { return []any{1, f} },
			before:    []jsontext.Token{jsontext.BeginArray, jsontext.Int(1)},
			class:     failingClass{prefix: append(open, jsontext.Int(2)), err: syntactic},
			settle:    nil, // EncodeFailureSince, from a mark taken before the class
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := json.Marshal(tc.container(tc.class))
			want, ok := errors.AsType[*json.SemanticError](err)
			if !ok {
				t.Fatalf("json/v2 failed with %v, want a *json.SemanticError", err)
			}

			enc := jsontext.NewEncoder(&bytes.Buffer{})
			for _, tok := range tc.before {
				if err := enc.WriteToken(tok); err != nil {
					t.Fatal(err)
				}
			}
			mark := typereg.MarkEncode(enc)
			failure := tc.class.MarshalJSONTo(enc)
			var settled error
			if tc.settle == nil {
				settled = typereg.EncodeFailureSince[failingClass](enc, mark, failure)
			} else {
				settled = tc.settle(enc, failure)
			}
			got, ok := errors.AsType[*json.SemanticError](settled)
			if !ok {
				t.Fatalf("settled failure = %v, want a *json.SemanticError", settled)
			}
			if got.GoType != want.GoType || got.JSONPointer != want.JSONPointer || !errors.Is(got, errRefused) {
				t.Errorf("settled failure names %v at %q, json/v2's names %v at %q", got.GoType, got.JSONPointer, want.GoType, want.JSONPointer)
			}
		})
	}
}

// TestREQ052EncodeFailureKeepsWhatItFinds pins what settling leaves alone: a
// writer's failure passes through unchanged, and a *json.SemanticError keeps
// the Go type and the JSON Pointer it already carries, as a failure that
// settled in a nested class does on its way out.
func TestREQ052EncodeFailureKeepsWhatItFinds(t *testing.T) {
	t.Parallel()
	enc := jsontext.NewEncoder(io.Discard)
	if err := enc.WriteToken(jsontext.BeginArray); err != nil {
		t.Fatal(err)
	}
	writer := errors.New("write refused")
	if got := typereg.EncodeFailure[failingClass](enc, true, writer); got != writer { //nolint:errorlint // identity: the same error value comes back
		t.Errorf("EncodeFailure(a writer's failure) = %v, want it unchanged", got)
	}
	nested := &json.SemanticError{GoType: reflect.TypeFor[int](), JSONPointer: "/0/x", Err: errRefused}
	got := typereg.EncodeFailure[failingClass](enc, true, nested)
	if got != nested || nested.GoType != reflect.TypeFor[int]() || nested.JSONPointer != "/0/x" { //nolint:errorlint // identity: the same error value comes back
		t.Errorf("EncodeFailure(a settled failure) = %v (%v at %q), want it unchanged", got, nested.GoType, nested.JSONPointer)
	}
	unnamed := &json.SemanticError{Err: errRefused}
	_ = typereg.EncodeFailure[failingClass](enc, true, unnamed)
	if unnamed.GoType != reflect.TypeFor[failingClass]() {
		t.Errorf("EncodeFailure named %v, want the class", unnamed.GoType)
	}
	if typereg.EncodeFailureSince[failingClass](enc, typereg.MarkEncode(enc), nil) != nil {
		t.Error("EncodeFailureSince(nil) is not nil")
	}
}
