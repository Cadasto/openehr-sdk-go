package canjson_test

// options_route_test.go: REQ-052 — a decode given WithUnmatched or WithStrict
// takes its own route: Decoder.Decode reads the whole value first and then
// decodes those bytes. The member check must change nothing else, so every
// failure that route reports carries the classification the plain route
// gives it, the duplicate-member refusal's ErrInvalidShape among them.

import (
	"encoding/json/jsontext"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// memberOptions are the options that send a decode down the member-checking
// route, each with a name for the subtest.
var memberOptions = []struct {
	name string
	opt  func() canjson.DecoderOption
}{
	{name: "WithStrict", opt: canjson.WithStrict},
	{name: "WithUnmatched", opt: func() canjson.DecoderOption { return canjson.WithUnmatched(func(string) {}) }},
}

// REQ-052 — a duplicate member name is refused with ErrInvalidShape on the
// member-checking route too, through Unmarshal and through Decoder.Decode,
// whose route reads the value whole before decoding it. jsontext refuses the
// duplicate while Decoder.Decode reads the value, so the classification of
// that read's error is what adds the sentinel. Can-fail control: return the
// read's error unclassified and both Decoder.Decode cases redden.
func TestDecodeWithMemberOptionsDuplicateMemberNameWrapsErrInvalidShape(t *testing.T) {
	const in = `{"_type":"DV_TEXT","value":"a","value":"b"}`
	for _, o := range memberOptions {
		for _, entry := range []struct {
			name   string
			decode func(v any) error
		}{
			{name: "Unmarshal", decode: func(v any) error { return canjson.Unmarshal([]byte(in), v, o.opt()) }},
			{name: "Decoder.Decode", decode: func(v any) error { return canjson.NewDecoder(strings.NewReader(in), o.opt()).Decode(v) }},
		} {
			t.Run(entry.name+"/"+o.name, func(t *testing.T) {
				var text rm.DVText
				err := entry.decode(&text)
				if err == nil {
					t.Fatalf("%s(%s, %s) = nil; want a duplicate-name refusal", entry.name, in, o.name)
				}
				if !errors.Is(err, canjson.ErrInvalidShape) {
					t.Errorf("%s(%s, %s) = %v; want errors.Is(_, canjson.ErrInvalidShape)", entry.name, in, o.name, err)
				}
				if !errors.Is(err, jsontext.ErrDuplicateName) {
					t.Errorf("%s(%s, %s) = %v; want errors.Is(_, jsontext.ErrDuplicateName)", entry.name, in, o.name, err)
				}
				assertShapeSentinelDistinct(t, err)
			})
		}
	}
}

// errClass is the classification a caller can read off a decode error.
type errClass struct {
	none, invalidShape, duplicateName, syntactic, unexpectedEOF, eof, decodeError, maxDepth, readFailure, unknownMember bool
}

// String names the classes c holds, or "none" for a nil error.
func (c errClass) String() string {
	var names []string
	for _, f := range []struct {
		set  bool
		name string
	}{
		{set: c.none, name: "none"},
		{set: c.invalidShape, name: "ErrInvalidShape"},
		{set: c.duplicateName, name: "jsontext.ErrDuplicateName"},
		{set: c.syntactic, name: "*jsontext.SyntacticError"},
		{set: c.unexpectedEOF, name: "io.ErrUnexpectedEOF"},
		{set: c.eof, name: "io.EOF"},
		{set: c.decodeError, name: "*DecodeError"},
		{set: c.maxDepth, name: "typereg.ErrMaxDepthExceeded"},
		{set: c.readFailure, name: "reader failure"},
		{set: c.unknownMember, name: "ErrUnknownMember"},
	} {
		if f.set {
			names = append(names, f.name)
		}
	}
	return "{" + strings.Join(names, ", ") + "}"
}

// classOf reads the classification of err.
func classOf(err error) errClass {
	_, syntactic := errors.AsType[*jsontext.SyntacticError](err)
	_, decodeError := errors.AsType[*canjson.DecodeError](err)
	return errClass{
		none:          err == nil,
		invalidShape:  errors.Is(err, canjson.ErrInvalidShape),
		duplicateName: errors.Is(err, jsontext.ErrDuplicateName),
		syntactic:     syntactic,
		unexpectedEOF: errors.Is(err, io.ErrUnexpectedEOF),
		eof:           errors.Is(err, io.EOF),
		decodeError:   decodeError,
		maxDepth:      errors.Is(err, typereg.ErrMaxDepthExceeded),
		readFailure:   errors.Is(err, errRead),
		unknownMember: errors.Is(err, canjson.ErrUnknownMember),
	}
}

// errRead is the failure failingReader returns once its bytes run out.
var errRead = errors.New("read failed")

// failingReader serves its bytes, then fails every read with errRead.
type failingReader struct{ r io.Reader }

func (f *failingReader) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errRead
	}
	return n, err
}

// nestedClusters returns a CLUSTER nested depth levels deep through items.
func nestedClusters(depth int) string {
	const open = `{"_type":"CLUSTER","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"c"},"items":[`
	return strings.Repeat(open, depth) + `{"_type":"ELEMENT","archetype_node_id":"at0002","name":{"_type":"DV_TEXT","value":"e"}}` + strings.Repeat("]}", depth)
}

// REQ-052 — every failure the plain route classifies is classified the same
// on the member-checking route, through Unmarshal and through each
// Decoder.Decode call on a stream. No input here carries a member the decode
// does not read, so the member check itself finds nothing.
func TestDecodeWithMemberOptionsClassifiesFailuresLikePlainRoute(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		reader func(in string) io.Reader // nil: a strings.Reader over in
		target func() any
	}{
		{name: "syntax error", in: `{"_type":"DV_TEXT","value":}`, target: newDVText},
		{name: "invalid UTF-8", in: "{\"_type\":\"DV_TEXT\",\"value\":\"\xff\"}", target: newDVText},
		{name: "lone surrogate escape", in: `{"_type":"DV_TEXT","value":"\ud800"}`, target: newDVText},
		{name: "truncated value", in: `{"_type":"DV_TEXT","value":"a"`, target: newDVText},
		{name: "empty input", in: ``, target: newDVText},
		{name: "whitespace only", in: " \n\t", target: newDVText},
		{name: "trailing data", in: `{"_type":"DV_TEXT","value":"a"}x`, target: newDVText},
		{name: "trailing second value", in: `{"_type":"DV_TEXT","value":"a"} {"_type":"DV_TEXT","value":"b"}`, target: newDVText},
		{name: "member of the wrong kind", in: `{"_type":"DV_TEXT","value":1}`, target: newDVText},
		{name: "array for an object", in: `[1]`, target: newDVText},
		{name: "string for an object", in: `"a"`, target: newDVText},
		{name: "number out of range", in: `{"_type":"DV_QUANTITY","magnitude":1e999,"units":"kg"}`, target: func() any { return &rm.DVQuantity{} }},
		{name: "_type of another class", in: `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"}`, target: newDVText},
		{name: "slot without _type", in: `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"e"},"value":{"value":"x"}}`, target: func() any { return &rm.Element{} }},
		{name: "slot with unknown _type", in: `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"e"},"value":{"_type":"NOPE","value":"x"}}`, target: func() any { return &rm.Element{} }},
		{name: "duplicate member name", in: `{"_type":"DV_TEXT","value":"a","value":"b"}`, target: newDVText},
		{name: "duplicate member name in a map", in: `{"a":1,"a":2}`, target: func() any { return &map[string]any{} }},
		{name: "nesting past the depth bound", in: nestedClusters(600), target: func() any { return &rm.Cluster{} }},
		{
			name:   "read failure mid-value",
			in:     `{"_type":"DV_TEXT","val`,
			reader: func(in string) io.Reader { return &failingReader{r: strings.NewReader(in)} },
			target: newDVText,
		},
		{
			name:   "read failure after a value",
			in:     `{"_type":"DV_TEXT","value":"a"}`,
			reader: func(in string) io.Reader { return &failingReader{r: strings.NewReader(in)} },
			target: newDVText,
		},
	}
	for _, tc := range cases {
		for _, o := range memberOptions {
			t.Run(tc.name+"/"+o.name, func(t *testing.T) {
				if tc.reader == nil {
					plain, opted := canjson.Unmarshal([]byte(tc.in), tc.target()), canjson.Unmarshal([]byte(tc.in), tc.target(), o.opt())
					if got, want := classOf(opted), classOf(plain); got != want {
						t.Errorf("Unmarshal(%.60q, %s) = %v, class %v; want the class of Unmarshal without options, %v (%v)", tc.in, o.name, opted, got, want, plain)
					}
				}
				open := func() io.Reader { return strings.NewReader(tc.in) }
				if tc.reader != nil {
					open = func() io.Reader { return tc.reader(tc.in) }
				}
				plain, opted := canjson.NewDecoder(open()), canjson.NewDecoder(open(), o.opt())
				// Decode until the plain route fails, at most three values.
				for call := 1; call <= 3; call++ {
					plainErr, optedErr := plain.Decode(tc.target()), opted.Decode(tc.target())
					if got, want := classOf(optedErr), classOf(plainErr); got != want {
						t.Errorf("Decode call %d over %.60q with %s = %v, class %v; want the class of Decode without options, %v (%v)", call, tc.in, o.name, optedErr, got, want, plainErr)
					}
					if plainErr != nil || optedErr != nil {
						break
					}
				}
			})
		}
	}
}

func newDVText() any { return &rm.DVText{} }
