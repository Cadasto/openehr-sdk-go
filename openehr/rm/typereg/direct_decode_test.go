package typereg

// REQ-052: the helpers a generated UnmarshalJSONFrom calls where it reads its
// class's members itself, and the gate that decides when it may.

import (
	"bytes"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestREQ052DirectDecodeGate pins which decoders let a generated
// UnmarshalJSONFrom read its members itself: the next value must be an
// object within the depth bound, on a decoder carrying the SDK's hooks under
// either identity and no option under which json/v2's struct decode reads a
// member otherwise. Each refused row is the case one condition exists for,
// with every other condition met, so dropping a condition from DirectDecode
// turns its row red; the accepted rows hold the options that change nothing a
// member reads.
func TestREQ052DirectDecodeGate(t *testing.T) {
	t.Parallel()
	set := unmarshalerAgg.Load()
	if set == nil {
		t.Fatal("unmarshalerAgg.Load() = nil: this test needs the rm package's interface hooks, which the typereg_test files import")
	}
	sdk := json.WithUnmarshalers(set.all)
	caller := json.WithUnmarshalers(json.JoinUnmarshalers(set.all, json.UnmarshalFromFunc(func(dec *jsontext.Decoder, _ *fakeBox) error { return dec.SkipValue() })))
	const object = `{"a":"x"}`
	nested := func(depth int) string { return strings.Repeat("[", depth) + object + strings.Repeat("]", depth) }
	for _, tc := range []struct {
		name  string
		doc   string
		depth int // tokens to read before the value
		opts  []json.Options
		want  bool
	}{
		{name: "an object, the SDK's hooks", doc: object, opts: []json.Options{sdk}, want: true},
		{name: "an object, the SDK's hooks under their buffered identity", doc: object, opts: []json.Options{set.bufferedOpt}, want: true},
		{name: "an object after a name", doc: `{"k":` + object + `}`, depth: 2, opts: []json.Options{sdk}, want: true},
		{name: "an object after a comma", doc: `[1,` + object + `]`, depth: 2, opts: []json.Options{sdk}, want: true},
		{name: "duplicate names and invalid UTF-8 allowed", doc: object, opts: []json.Options{sdk, jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true)}, want: true},
		{name: "the options turned off", doc: object, opts: []json.Options{sdk, json.MatchCaseInsensitiveNames(false), json.RejectUnknownMembers(false), json.StringifyNumbers(false), jsonv1.ReportErrorsWithLegacySemantics(false), jsonv1.MergeWithLegacySemantics(false)}, want: true},
		{name: "an object at the depth bound", doc: nested(maxDecodeDepth - 1), depth: maxDecodeDepth - 1, opts: []json.Options{sdk}, want: true},

		{name: "an object past the depth bound", doc: nested(maxDecodeDepth), depth: maxDecodeDepth, opts: []json.Options{sdk}},
		{name: "null", doc: `null`, opts: []json.Options{sdk}},
		{name: "an array", doc: `[]`, opts: []json.Options{sdk}},
		{name: "a string", doc: `"x"`, opts: []json.Options{sdk}},
		{name: "a number", doc: `1`, opts: []json.Options{sdk}},
		{name: "no input", doc: ``, opts: []json.Options{sdk}},
		{name: "malformed input", doc: `}`, opts: []json.Options{sdk}},
		{name: "no hooks", doc: object},
		{name: "no hooks, set to nil", doc: object, opts: []json.Options{json.WithUnmarshalers(nil)}},
		{name: "a caller's hooks", doc: object, opts: []json.Options{caller}},
		{name: "MatchCaseInsensitiveNames", doc: object, opts: []json.Options{sdk, json.MatchCaseInsensitiveNames(true)}},
		{name: "RejectUnknownMembers", doc: object, opts: []json.Options{sdk, json.RejectUnknownMembers(true)}},
		{name: "StringifyNumbers", doc: object, opts: []json.Options{sdk, json.StringifyNumbers(true)}},
		{name: "v1 error reporting", doc: object, opts: []json.Options{sdk, jsonv1.ReportErrorsWithLegacySemantics(true)}},
		{name: "v1 merge semantics", doc: object, opts: []json.Options{sdk, jsonv1.MergeWithLegacySemantics(true)}},
		{name: "encoding/json defaults", doc: object, opts: []json.Options{jsonv1.DefaultOptionsV1(), sdk}},
	} {
		dec := jsontext.NewDecoder(strings.NewReader(tc.doc), tc.opts...)
		for range tc.depth {
			if _, err := dec.ReadToken(); err != nil {
				t.Fatalf("%s: reading up to the value: %v", tc.name, err)
			}
		}
		if got := DirectDecode(dec); got != tc.want {
			t.Errorf("DirectDecode(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
	if DirectDecode(nil) {
		t.Error("DirectDecode(nil) = true, want false")
	}
}

// directProbe is a class with the generated pair of decodes: it reads its
// members itself where DirectDecode allows, through the helpers a generated
// readJSONMembers calls, and decodes a wrapper struct through DecodeInto
// otherwise.
type directProbe struct {
	A string   `json:"a"`
	B []string `json:"b"`
}

// rawDirectProbe is the method-free alias the wrapper decodes into.
type rawDirectProbe directProbe

// directProbeType is the `_type` of a [directProbe].
const directProbeType = "PROBE"

func (p *directProbe) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if !DirectDecode(dec) {
		w := struct {
			Type string `json:"_type"`
			*rawDirectProbe
		}{rawDirectProbe: (*rawDirectProbe)(p)}
		return DecodeInto(dec, directProbeType, &w, &w.Type)
	}
	got, err := p.readJSONMembers(dec)
	return DecodeDone(directProbeType, got, err)
}

func (p *directProbe) readJSONMembers(dec *jsontext.Decoder) (string, error) {
	if _, err := dec.ReadToken(); err != nil {
		return "", err
	}
	var got string
	for dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			return "", MemberNameFailure(err)
		}
		switch name.String() {
		case "_type":
			got, err = DecodeTypeMember(dec, directProbeType)
		case "a":
			err = json.UnmarshalDecode(dec, &p.A)
		case "b":
			err = json.UnmarshalDecode(dec, &p.B)
		default:
			err = dec.SkipValue()
		}
		if err != nil {
			return "", err
		}
	}
	_, err := dec.ReadToken()
	return got, err
}

// probeFailure describes err as a caller reads it: the text, the position and
// Go type json/v2 gives it, the path of a DecodeError in it, and the
// sentinels it wraps.
func probeFailure(err error) string {
	if err == nil {
		return "<nil>"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q", err.Error())
	if se, ok := errors.AsType[*json.SemanticError](err); ok && se != nil {
		fmt.Fprintf(&b, " semantic offset=%d pointer=%q gotype=%v", se.ByteOffset, se.JSONPointer, se.GoType)
	}
	if syn, ok := errors.AsType[*jsontext.SyntacticError](err); ok && syn != nil {
		fmt.Fprintf(&b, " syntactic offset=%d pointer=%q", syn.ByteOffset, syn.JSONPointer)
	}
	if de, ok := errors.AsType[*DecodeError](err); ok && de != nil {
		fmt.Fprintf(&b, " path=%q", de.Path)
	}
	for _, s := range []error{ErrInvalidShape, ErrTypeMismatch, jsontext.ErrDuplicateName} {
		fmt.Fprintf(&b, " %v=%v", s, errors.Is(err, s))
	}
	return b.String()
}

// TestREQ052DirectDecodeSettlesAsTheWrapper holds a class that reads its
// members itself to the failure the wrapper decode reports for the same
// input: a duplicate name, which the tokenizer refuses as it reads the name
// where json/v2's struct decode refuses it after (MemberNameFailure); a
// `_type` naming another class, refused after a member's failure wherever the
// two stand (DecodeTypeMember, DecodeDone); and a value of another kind. Each
// input decodes at the root, as a list element and as a struct field. Drop
// the settling MemberNameFailure does and the duplicate rows point at the
// value before the name instead.
func TestREQ052DirectDecodeSettlesAsTheWrapper(t *testing.T) {
	t.Parallel()
	sdk := Unmarshalers()
	wrapper := json.StringifyNumbers(true)
	for _, doc := range []string{
		`{"a":"x","a":"y"}`,
		`{"_type":"PROBE","b":[],"_type":"PROBE"}`,
		`{"zz":1,"a":"x","zz":2}`,
		`{"a":"x","b":["1"],"b":["2"]}`,
		`{"_type":"OTHER","a":"x"}`,
		`{"a":"x","_type":"OTHER"}`,
		`{"_type":"OTHER","a":5}`,
		`{"a":5,"_type":"OTHER"}`,
		`{"_type":5,"a":"x"}`,
		`{"_type":null,"a":"x"}`,
		`{"_type":"","a":"x"}`,
		`{"_type":"PROBE","a":"x"}`,
		`{"a":"x","b":"y"}`,
		`{"a":"x",`,
		`{"a":"x" "b"}`,
	} {
		for _, form := range []struct {
			name string
			wrap func(string) string
			newV func() any
		}{
			{"at the root", func(s string) string { return s }, func() any { return new(directProbe) }},
			{"as a list element", func(s string) string { return `[{"a":"first"},` + s + `]` }, func() any { return new([]directProbe) }},
			{"as a struct field", func(s string) string { return `{"n":"1","p":` + s + `}` }, func() any {
				return new(struct {
					N string      `json:"n"`
					P directProbe `json:"p"`
				})
			}},
		} {
			data := []byte(form.wrap(doc))
			direct, wrapped := form.newV(), form.newV()
			directErr := json.Unmarshal(data, direct, sdk)
			wrappedErr := json.Unmarshal(data, wrapped, sdk, wrapper)
			if d, w := probeFailure(directErr), probeFailure(wrappedErr); d != w {
				t.Errorf("%s %s:\ndirect:  %s\nwrapper: %s", doc, form.name, d, w)
			}
			if !reflect.DeepEqual(direct, wrapped) {
				t.Errorf("%s %s: direct read left %+v, wrapper %+v", doc, form.name, direct, wrapped)
			}
		}
	}
}

// TestREQ052DecodeTypeMember pins what a `_type` member reads as: the class
// it names when that is another class, and "" for the class itself, an empty
// string, an escaped spelling of the class and null; a value of another kind
// fails as the wrapper's string field fails, naming the Go string type.
func TestREQ052DecodeTypeMember(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value, want string
		fails       bool
	}{
		{value: `"PROBE"`},
		{value: `"PRO\u0042E"`},
		{value: `""`},
		{value: `null`},
		{value: `"OTHER"`, want: "OTHER"},
		{value: `"O\u0054HER"`, want: "OTHER"},
		{value: `5`, fails: true},
		{value: `{}`, fails: true},
		{value: `"x`, fails: true},
	} {
		dec := jsontext.NewDecoder(strings.NewReader(`{"_type":` + tc.value + `}`))
		if _, err := dec.ReadToken(); err != nil {
			t.Fatal(err)
		}
		if _, err := dec.ReadToken(); err != nil {
			t.Fatal(err)
		}
		got, err := DecodeTypeMember(dec, directProbeType)
		if got != tc.want || (err != nil) != tc.fails {
			t.Errorf("DecodeTypeMember(%s) = %q, %v; want %q, failing %v", tc.value, got, err, tc.want, tc.fails)
		}
	}
}

// TestREQ052DecodeDone pins how a direct read is settled: a failure is
// classified as DecodeInto classifies one and wins over a `_type` naming
// another class, which is refused as DecodeInto refuses it, on "/_type" with
// ErrTypeMismatch and no shape classification; with neither, nil.
func TestREQ052DecodeDone(t *testing.T) {
	t.Parallel()
	if err := DecodeDone(directProbeType, "", nil); err != nil {
		t.Errorf("DecodeDone(no failure, no other class) = %v, want nil", err)
	}
	mismatch := DecodeDone(directProbeType, "OTHER", nil)
	de, ok := errors.AsType[*DecodeError](mismatch)
	if !ok || de.Path != "/_type" || !errors.Is(mismatch, ErrTypeMismatch) || errors.Is(mismatch, ErrInvalidShape) {
		t.Errorf("DecodeDone(another class) = %v, want a DecodeError on /_type wrapping ErrTypeMismatch alone", mismatch)
	}
	var wire struct {
		Type string `json:"_type"`
	}
	wrapped := DecodeInto(jsontext.NewDecoder(strings.NewReader(`{"_type":"OTHER"}`)), directProbeType, &wire, &wire.Type)
	if mismatch == nil || wrapped == nil || mismatch.Error() != wrapped.Error() {
		t.Errorf("DecodeDone refuses another class as %v, DecodeInto as %v; want the same words", mismatch, wrapped)
	}
	failure := &json.SemanticError{Err: errors.New("refused")}
	if err := DecodeDone(directProbeType, "OTHER", failure); !errors.Is(err, ErrInvalidShape) || errors.Is(err, ErrTypeMismatch) || !strings.HasPrefix(err.Error(), "canjson: PROBE: ") {
		t.Errorf("DecodeDone(a member's failure, another class) = %v, want the failure classified as a shape failure of PROBE, ahead of the other class", err)
	}
	syntactic := &jsontext.SyntacticError{Err: errors.New("bad")}
	if err := DecodeDone(directProbeType, "", syntactic); err != syntactic { //nolint:errorlint // identity: malformed input passes through unwrapped
		t.Errorf("DecodeDone(malformed input) = %v, want it unwrapped", err)
	}
}

// TestREQ052MemberNameFailurePassesOthersThrough pins that MemberNameFailure
// settles a duplicate name only: any other failure to read a name, such as
// malformed or truncated input, comes back as the tokenizer returned it.
func TestREQ052MemberNameFailurePassesOthersThrough(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{`{"a":1,`, `{5:1}`, `{"a":1 "b":2}`} {
		dec := jsontext.NewDecoder(bytes.NewReader([]byte(doc)))
		var err error
		for err == nil {
			_, err = dec.ReadToken()
		}
		if got := MemberNameFailure(err); got != err { //nolint:errorlint // identity: the same error value comes back
			t.Errorf("MemberNameFailure(%v) = %v, want it unchanged", err, got)
		}
	}
	dec := jsontext.NewDecoder(strings.NewReader(`{"a":1,"a":2}`))
	var err error
	for err == nil {
		_, err = dec.ReadToken()
	}
	settled := MemberNameFailure(err)
	se, ok := errors.AsType[*json.SemanticError](settled)
	syn, _ := errors.AsType[*jsontext.SyntacticError](err)
	if !ok || syn == nil || se.ByteOffset != syn.ByteOffset || se.JSONPointer != syn.JSONPointer ||
		!errors.Is(settled, ErrInvalidShape) || !errors.Is(settled, jsontext.ErrDuplicateName) {
		t.Errorf("MemberNameFailure(%v) = %v, want a *json.SemanticError at the name, wrapping ErrInvalidShape and the duplicate", err, settled)
	}
	if got := MemberNameFailure(nil); got != nil {
		t.Errorf("MemberNameFailure(nil) = %v, want nil", got)
	}
}
