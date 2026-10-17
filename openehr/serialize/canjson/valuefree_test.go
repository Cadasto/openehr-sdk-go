package canjson_test

// valuefree_test.go: REQ-052 "Value-free decode errors". Every failure the
// input causes comes back from the two canjson entry points as a
// *canjson.DecodeError at the top of the chain, whose text names the class
// and the pointer but no value from the document, whose Value holds the
// literal the decoder could not take, and beneath which every cause the
// decode reached stays reachable.

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// entryPoint is one way of decoding a whole input through canjson.
type entryPoint struct {
	name   string
	decode func(in string, v any, opts ...canjson.DecoderOption) error
}

// entryPoints are the two entry points the value-free rule binds.
var entryPoints = []entryPoint{
	{name: "Unmarshal", decode: func(in string, v any, opts ...canjson.DecoderOption) error {
		return canjson.Unmarshal([]byte(in), v, opts...)
	}},
	{name: "Decoder.Decode", decode: func(in string, v any, opts ...canjson.DecoderOption) error {
		return canjson.NewDecoder(strings.NewReader(in), opts...).Decode(v)
	}},
}

// topDecodeError returns err as a *canjson.DecodeError when it is one at the
// top of the chain, without unwrapping.
func topDecodeError(err error) (*canjson.DecodeError, bool) {
	de, ok := err.(*canjson.DecodeError) //nolint:errorlint // REQ-052: the envelope is the error itself, found without unwrapping
	return de, ok && de != nil
}

// target is a decode target for a case, with a name for the subtest.
type target struct {
	name string
	new  func() any
}

// concreteAndSlot gives the concrete RM type and the polymorphic DataValue
// slot as targets, so the pointer is checked across a slot's own decoder.
func concreteAndSlot(concrete func() any) []target {
	return []target{
		{name: "concrete", new: concrete},
		{name: "DataValue slot", new: func() any { return new(rm.DataValue) }},
	}
}

// TestUnmarshal_REQ052_ValueFreeDecodeErrors is REQ-052's acceptance table:
// each input, decoded with canjson.Unmarshal and with a canjson.Decoder in
// every decode mode, returns a *canjson.DecodeError at the top of the chain
// whose Path and text name the failing value's JSON Pointer and whose text
// names the class and carries none of the document's values. Value holds the
// number as written for the four numeric inputs and nothing for the two
// malformed ones.
//
// Can-fail control: render the cause of a DecodeError with its own Error()
// and the literals reappear in the text; return the codec's error without the
// envelope and the top-level assertion fails.
func TestUnmarshal_REQ052_ValueFreeDecodeErrors(t *testing.T) {
	const quantity = `{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg","precision":`
	cases := []struct {
		name    string
		in      string
		targets []target
		pointer string
		class   string   // a phrase naming the failure's class
		absent  []string // values from the document the text must not carry
		value   any      // want Value.Reveal()
	}{
		{
			name:    "DV_COUNT magnitude 3.5",
			in:      `{"_type":"DV_COUNT","magnitude":3.5}`,
			targets: concreteAndSlot(func() any { return new(rm.DVCount) }),
			pointer: "/magnitude",
			class:   "unmarshal JSON number into Go int64",
			absent:  []string{"3.5"},
			value:   "3.5",
		},
		{
			name:    "DV_COUNT magnitude 98765.4321",
			in:      `{"_type":"DV_COUNT","magnitude":98765.4321}`,
			targets: concreteAndSlot(func() any { return new(rm.DVCount) }),
			pointer: "/magnitude",
			class:   "unmarshal JSON number into Go int64",
			absent:  []string{"98765.4321", "98765"},
			value:   "98765.4321",
		},
		{
			name:    "DV_QUANTITY precision 1.5",
			in:      quantity + `1.5}`,
			targets: concreteAndSlot(func() any { return new(rm.DVQuantity) }),
			pointer: "/precision",
			class:   "unmarshal JSON number into Go int32",
			absent:  []string{"1.5", "80.5"},
			value:   "1.5",
		},
		{
			name:    "DV_QUANTITY precision 2147483648",
			in:      quantity + `2147483648}`,
			targets: concreteAndSlot(func() any { return new(rm.DVQuantity) }),
			pointer: "/precision",
			class:   "unmarshal JSON number into Go int32",
			absent:  []string{"2147483648", "80.5"},
			value:   "2147483648",
		},
		{
			name:    "DV_TEXT value with an invalid escape",
			in:      `{"_type":"DV_TEXT","value":"Jane \q Doe"}`,
			targets: concreteAndSlot(func() any { return new(rm.DVText) }),
			pointer: "/value",
			class:   "invalid escape sequence",
			absent:  []string{`\q`, "Jane", "Doe"},
		},
		{
			name:    "DV_TEXT value with a refused character",
			in:      `{"_type":"DV_TEXT","value":Jane}`,
			targets: concreteAndSlot(func() any { return new(rm.DVText) }),
			pointer: "/value",
			class:   "invalid character",
			absent:  []string{"'J'", "Jane"},
		},
	}
	for _, tc := range cases {
		for _, tg := range tc.targets {
			for _, ep := range entryPoints {
				for mode, opts := range jsonDecodeModes() {
					t.Run(tc.name+"/"+tg.name+"/"+ep.name+"/"+mode, func(t *testing.T) {
						err := ep.decode(tc.in, tg.new(), opts...)
						de, ok := topDecodeError(err)
						if !ok {
							t.Fatalf("%s(%s) = %T %v; want a *canjson.DecodeError at the top of the chain", ep.name, tc.in, err, err)
						}
						if de.Path != tc.pointer {
							t.Errorf("%s(%s): DecodeError.Path = %q; want %q", ep.name, tc.in, de.Path, tc.pointer)
						}
						msg := err.Error()
						if !strings.Contains(msg, tc.pointer) {
							t.Errorf("%s(%s) = %q; want the text to name the pointer %s", ep.name, tc.in, msg, tc.pointer)
						}
						if !strings.Contains(msg, tc.class) {
							t.Errorf("%s(%s) = %q; want the text to name the class %q", ep.name, tc.in, msg, tc.class)
						}
						for _, v := range tc.absent {
							if strings.Contains(msg, v) {
								t.Errorf("%s(%s) = %q; the text carries %q from the document", ep.name, tc.in, msg, v)
							}
						}
						if got := de.Value.Reveal(); got != tc.value {
							t.Errorf("%s(%s): Value.Reveal() = %#v; want %#v", ep.name, tc.in, got, tc.value)
						}
					})
				}
			}
		}
	}
}

// TestUnmarshal_REQ052_PrimitiveRefusalsKeepTheLiteralInValue is REQ-052's
// acceptance line for the SDK's own primitive codecs: decoded with
// canjson.Unmarshal, and with a canjson.Decoder in every decode mode,
// Value.Reveal() returns the refused literal, as the document wrote it, for an
// rm.Real number past its digit budget and for a two-character value
// rm.Character refuses, and nothing for a value of the wrong JSON kind. In
// every case Error() names the pointer and carries none of the document's
// values.
//
// Can-fail control: return rm.Real's digit-budget refusal or rm.Character's
// one-character refusal without the literal and the matching rows' Value
// assertions fail.
func TestUnmarshal_REQ052_PrimitiveRefusalsKeepTheLiteralInValue(t *testing.T) {
	const (
		mappings = `{"_type":"DV_TEXT","value":"a","mappings":[{"_type":"TERM_MAPPING","match":`
		mapped   = `,"target":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"SNOMED-CT"},"code_string":"73211009"}}]}`
	)
	cases := []struct {
		name    string
		in      string
		targets []target
		pointer string
		absent  []string // values from the document the text must not carry
		value   any      // want Value.Reveal()
	}{
		{
			name:    "Real past its digit budget, a number",
			in:      `{"_type":"DV_QUANTITY","magnitude":0.1234567890123456789,"units":"kg"}`,
			targets: concreteAndSlot(func() any { return new(rm.DVQuantity) }),
			pointer: "/magnitude",
			absent:  []string{"1234567890123456789"},
			value:   "0.1234567890123456789",
		},
		{
			name:    "Real past its digit budget, a quoted number",
			in:      `{"_type":"DV_QUANTITY","magnitude":"18446744073709551616","units":"kg"}`,
			targets: concreteAndSlot(func() any { return new(rm.DVQuantity) }),
			pointer: "/magnitude",
			absent:  []string{"18446744073709551616", "1844674407"},
			value:   `"18446744073709551616"`,
		},
		{
			name:    "Character of two characters",
			in:      mappings + `"\u03a9§"` + mapped,
			targets: concreteAndSlot(func() any { return new(rm.DVText) }),
			pointer: "/mappings/0/match",
			absent:  []string{`\u03a9`, "Ω", "§"},
			value:   `"\u03a9§"`,
		},
		{
			name:    "Character of no character",
			in:      mappings + `""` + mapped,
			targets: concreteAndSlot(func() any { return new(rm.DVText) }),
			pointer: "/mappings/0/match",
			value:   `""`,
		},
		{
			name:    "a DV_BOOLEAN value of the wrong JSON kind",
			in:      `{"_type":"DV_BOOLEAN","value":"yes"}`,
			targets: concreteAndSlot(func() any { return new(rm.DVBoolean) }),
			pointer: "/value",
			absent:  []string{"yes"},
		},
		{
			name:    "a Character of the wrong JSON kind",
			in:      mappings + `true` + mapped,
			targets: concreteAndSlot(func() any { return new(rm.DVText) }),
			pointer: "/mappings/0/match",
		},
	}
	for _, tc := range cases {
		for _, tg := range tc.targets {
			for _, ep := range entryPoints {
				for mode, opts := range jsonDecodeModes() {
					t.Run(tc.name+"/"+tg.name+"/"+ep.name+"/"+mode, func(t *testing.T) {
						err := ep.decode(tc.in, tg.new(), opts...)
						de, ok := topDecodeError(err)
						if !ok {
							t.Fatalf("%s(%s) = %T %v; want a *canjson.DecodeError at the top of the chain", ep.name, tc.in, err, err)
						}
						if de.Path != tc.pointer {
							t.Errorf("%s(%s): DecodeError.Path = %q; want %q", ep.name, tc.in, de.Path, tc.pointer)
						}
						msg := err.Error()
						if !strings.Contains(msg, tc.pointer) {
							t.Errorf("%s(%s) = %q; want the text to name the pointer %s", ep.name, tc.in, msg, tc.pointer)
						}
						for _, v := range tc.absent {
							if strings.Contains(msg, v) {
								t.Errorf("%s(%s) = %q; the text carries %q from the document", ep.name, tc.in, msg, v)
							}
						}
						if got := de.Value.Reveal(); got != tc.value {
							t.Errorf("%s(%s): Value.Reveal() = %#v; want %#v", ep.name, tc.in, got, tc.value)
						}
						if !errors.Is(err, canjson.ErrInvalidShape) {
							t.Errorf("%s(%s) = %v; want errors.Is(_, canjson.ErrInvalidShape), the classification the refusal carried before", ep.name, tc.in, err)
						}
					})
				}
			}
		}
	}
}

// TestUnmarshal_REQ052_StructureStaysInDecodeErrorText pins the line REQ-052
// draws between values and structure: a member name and a `_type` value are
// structure, so a duplicate member name and an unknown `_type` keep their
// names in the text, even names the RM does not define.
//
// Can-fail control: withhold the duplicate refusal's member name or the
// slot's `_type` from the text and the name assertions fail.
func TestUnmarshal_REQ052_StructureStaysInDecodeErrorText(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		target   func() any
		sentinel error
		path     string
	}{
		{
			name:     "duplicate member name",
			in:       `{"_type":"DV_TEXT","value":"a","Jane Doe":1,"Jane Doe":2}`,
			target:   func() any { return new(rm.DVText) },
			sentinel: jsontext.ErrDuplicateName,
			path:     "/Jane Doe",
		},
		{
			name:     "unknown _type",
			in:       `{"_type":"COMPOSITION","content":[{"_type":"Jane Doe"}]}`,
			target:   func() any { return new(rm.Composition) },
			sentinel: typereg.ErrUnknownType,
			path:     "/content/0",
		},
	}
	for _, tc := range cases {
		for _, ep := range entryPoints {
			t.Run(tc.name+"/"+ep.name, func(t *testing.T) {
				err := ep.decode(tc.in, tc.target())
				de, ok := topDecodeError(err)
				if !ok {
					t.Fatalf("%s(%s) = %T %v; want a *canjson.DecodeError at the top of the chain", ep.name, tc.in, err, err)
				}
				if !errors.Is(err, tc.sentinel) {
					t.Errorf("%s(%s) = %v; want errors.Is(_, %v)", ep.name, tc.in, err, tc.sentinel)
				}
				if de.Path != tc.path {
					t.Errorf("%s(%s): DecodeError.Path = %q; want %q", ep.name, tc.in, de.Path, tc.path)
				}
				if !strings.Contains(err.Error(), "Jane Doe") {
					t.Errorf("%s(%s) = %q; want the text to keep the name Jane Doe, which is structure", ep.name, tc.in, err)
				}
				if got := de.Value.Reveal(); got != nil {
					t.Errorf("%s(%s): Value.Reveal() = %#v; want nothing for a failure that concerns no value", ep.name, tc.in, got)
				}
			})
		}
	}
}

// TestUnmarshal_REQ052_DecodeErrorKeepsEveryCause pins the reachability half
// of REQ-052: under the envelope every entry point now returns, each sentinel
// and typed cause the decode reached before stays reachable with errors.Is
// and errors.As, no failure gains a classification it did not have, and Value
// holds a literal only where the failure concerns one.
//
// Can-fail control: have the entry point return an envelope whose Inner is
// anything but the codec's own error, a rendered copy included, and the
// reachability assertions fail for every row.
func TestUnmarshal_REQ052_DecodeErrorKeepsEveryCause(t *testing.T) {
	const element = `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"e"},"value":`
	isSemantic := func(err error) bool { _, ok := errors.AsType[*json.SemanticError](err); return ok }
	isSyntactic := func(err error) bool { _, ok := errors.AsType[*jsontext.SyntacticError](err); return ok }
	isNumError := func(err error) bool { _, ok := errors.AsType[*strconv.NumError](err); return ok }
	cases := []struct {
		name   string
		in     string
		target func() any
		opts   []canjson.DecoderOption
		is     []error                // must match errors.Is
		isNot  []error                // must not match errors.Is
		as     []func(err error) bool // typed causes reachable with errors.As
		path   string
		value  any
	}{
		{
			name: "non-integral number", in: `{"_type":"DV_COUNT","magnitude":3.5}`, target: func() any { return new(rm.DVCount) },
			is: []error{canjson.ErrInvalidShape, strconv.ErrSyntax}, as: []func(error) bool{isSemantic},
			path: "/magnitude", value: "3.5",
		},
		{
			name: "number out of range", in: `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg","precision":2147483648}`, target: func() any { return new(rm.DVQuantity) },
			is: []error{canjson.ErrInvalidShape, strconv.ErrRange}, as: []func(error) bool{isSemantic},
			path: "/precision", value: "2147483648",
		},
		{
			name: "quoted Real literal that does not parse", in: `{"_type":"DV_QUANTITY","magnitude":"12.3.4","units":"kg"}`, target: func() any { return new(rm.DVQuantity) },
			is: []error{canjson.ErrInvalidShape, strconv.ErrSyntax}, as: []func(error) bool{isSemantic, isNumError},
			path: "/magnitude", value: `"12.3.4"`,
		},
		{
			name: "quoted Integer literal out of range", in: `{"_type":"DV_PROPORTION","numerator":1,"denominator":2,"type":"99999999999"}`, target: func() any { return new(rm.DVProportion) },
			is: []error{canjson.ErrInvalidShape, strconv.ErrRange}, as: []func(error) bool{isSemantic, isNumError},
			path: "/type", value: `"99999999999"`,
		},
		{
			name: "Real past its digit budget", in: `{"_type":"DV_QUANTITY","magnitude":0.1234567890123456789,"units":"kg"}`, target: func() any { return new(rm.DVQuantity) },
			is: []error{canjson.ErrInvalidShape}, as: []func(error) bool{isSemantic},
			path: "/magnitude", value: "0.1234567890123456789",
		},
		{
			name: "Character of two characters", in: `{"_type":"TERM_MAPPING","match":"ab","target":{"terminology_id":{"value":"x"},"code_string":"c"}}`, target: func() any { return new(rm.TermMapping) },
			is: []error{canjson.ErrInvalidShape}, as: []func(error) bool{isSemantic},
			path: "/match", value: `"ab"`,
		},
		{
			name: "value of the wrong JSON kind", in: `{"_type":"DV_BOOLEAN","value":"yes"}`, target: func() any { return new(rm.DVBoolean) },
			is: []error{canjson.ErrInvalidShape}, as: []func(error) bool{isSemantic},
			path: "/value",
		},
		{
			name: "malformed JSON", in: `{"_type":"DV_TEXT","value":"Jane \q Doe"}`, target: func() any { return new(rm.DVText) },
			isNot: []error{canjson.ErrInvalidShape}, as: []func(error) bool{isSyntactic},
			path: "/value",
		},
		{
			name: "truncated input", in: `{"_type":"DV_TEXT","value":"a"`, target: func() any { return new(rm.DVText) },
			is: []error{io.ErrUnexpectedEOF}, isNot: []error{canjson.ErrInvalidShape}, as: []func(error) bool{isSyntactic},
		},
		{
			name: "duplicate member name", in: `{"_type":"DV_TEXT","value":"a","value":"b"}`, target: func() any { return new(rm.DVText) },
			is: []error{canjson.ErrInvalidShape, jsontext.ErrDuplicateName}, as: []func(error) bool{isSyntactic},
			path: "/value",
		},
		{
			name: "slot without _type", in: element + `{"value":"x"}}`, target: func() any { return new(rm.Element) },
			is: []error{typereg.ErrMissingType}, isNot: []error{canjson.ErrInvalidShape},
			path: "/value",
		},
		{
			name: "slot with unknown _type", in: element + `{"_type":"NOPE","value":"x"}}`, target: func() any { return new(rm.Element) },
			is: []error{typereg.ErrUnknownType}, isNot: []error{canjson.ErrInvalidShape},
			path: "/value",
		},
		{
			name: "_type of another class", in: `{"_type":"DV_TEXT","value":"a"}`, target: func() any { return new(rm.DVCount) },
			is: []error{typereg.ErrTypeMismatch}, isNot: []error{canjson.ErrInvalidShape},
			path: "/_type",
		},
		{
			name: "nesting past the depth bound", in: nestedClusters(600), target: func() any { return new(rm.Cluster) },
			is: []error{typereg.ErrMaxDepthExceeded}, isNot: []error{canjson.ErrInvalidShape},
			path: "/items/0", // the slot whose nesting passes the bound
		},
		{
			name: "unknown member under WithStrict", in: `{"_type":"DV_TEXT","value":"a","x-note":"` + plantedValue + `"}`, target: func() any { return new(rm.DVText) },
			opts: []canjson.DecoderOption{canjson.WithStrict()},
			is:   []error{canjson.ErrUnknownMember}, isNot: []error{canjson.ErrInvalidShape},
			path: "/x-note",
		},
	}
	for _, tc := range cases {
		for _, ep := range entryPoints {
			t.Run(tc.name+"/"+ep.name, func(t *testing.T) {
				err := ep.decode(tc.in, tc.target(), tc.opts...)
				de, ok := topDecodeError(err)
				if !ok {
					t.Fatalf("%s(%.80s) = %T %v; want a *canjson.DecodeError at the top of the chain", ep.name, tc.in, err, err)
				}
				for _, s := range tc.is {
					if !errors.Is(err, s) {
						t.Errorf("%s(%.80s) = %v; want errors.Is(_, %v)", ep.name, tc.in, err, s)
					}
				}
				for _, s := range tc.isNot {
					if errors.Is(err, s) {
						t.Errorf("%s(%.80s) = %v; want no errors.Is(_, %v)", ep.name, tc.in, err, s)
					}
				}
				for i, as := range tc.as {
					if !as(err) {
						t.Errorf("%s(%.80s) = %v; typed cause %d is not reachable with errors.As", ep.name, tc.in, err, i)
					}
				}
				if de.Path != tc.path {
					t.Errorf("%s(%.80s): DecodeError.Path = %q; want %q", ep.name, tc.in, de.Path, tc.path)
				}
				if got := de.Value.Reveal(); got != tc.value {
					t.Errorf("%s(%.80s): Value.Reveal() = %#v; want %#v", ep.name, tc.in, got, tc.value)
				}
				if lit, ok := tc.value.(string); ok && strings.Contains(err.Error(), strings.Trim(lit, `"`)) {
					t.Errorf("%s(%.80s) = %q; the text carries the literal %s", ep.name, tc.in, err, lit)
				}
				assertShapeSentinelDistinct(t, err)
			})
		}
	}
}

// TestUnmarshal_REQ052_SDKMessagesStayInDecodeErrorText pins the value-free
// text the SDK's own codecs write: rendering a DecodeError without values
// keeps it, so a refusal still says what was wrong.
//
// Can-fail control: name every cause the renderer does not build itself by
// its class alone, the SDK's own included, and these phrases disappear.
func TestUnmarshal_REQ052_SDKMessagesStayInDecodeErrorText(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		target func() any
		opts   []canjson.DecoderOption
		want   []string
	}{
		{
			name:   "Real past its digit budget",
			in:     `{"_type":"DV_QUANTITY","magnitude":0.1234567890123456789,"units":"kg"}`,
			target: func() any { return new(rm.DVQuantity) },
			want:   []string{"canjson: DV_QUANTITY: ", "rm.Real: literal carries more than 17 significant decimal digits"},
		},
		{
			name:   "quoted Real literal",
			in:     `{"_type":"DV_QUANTITY","magnitude":"12.3.4","units":"kg"}`,
			target: func() any { return new(rm.DVQuantity) },
			want:   []string{"canjson: DV_QUANTITY: ", "rm.Real: parse quoted literal: strconv.ParseFloat: invalid syntax"},
		},
		{
			name:   "quoted Integer literal",
			in:     `{"_type":"DV_PROPORTION","numerator":1,"denominator":2,"type":"1.5"}`,
			target: func() any { return new(rm.DVProportion) },
			want:   []string{"canjson: DV_PROPORTION: ", "rm.Integer: parse quoted literal: strconv.ParseInt: invalid syntax"},
		},
		{
			name:   "Character of two characters",
			in:     `{"_type":"TERM_MAPPING","match":"ab","target":{"terminology_id":{"value":"x"},"code_string":"c"}}`,
			target: func() any { return new(rm.TermMapping) },
			want:   []string{"canjson: TERM_MAPPING: ", "rm.Character: must be exactly one character, got 2"},
		},
		{
			name:   "_type of another class",
			in:     `{"_type":"DV_TEXT","value":"a"}`,
			target: func() any { return new(rm.DVCount) },
			want:   []string{`canjson: expected "DV_COUNT", got "DV_TEXT": typereg: decoded type does not satisfy target`},
		},
		{
			name:   "unknown member under WithStrict",
			in:     `{"_type":"DV_TEXT","value":"a","x-note":"` + plantedValue + `"}`,
			target: func() any { return new(rm.DVText) },
			opts:   []canjson.DecoderOption{canjson.WithStrict()},
			want:   []string{`canjson: unknown member "/x-note"`},
		},
	}
	for _, tc := range cases {
		for _, ep := range entryPoints {
			t.Run(tc.name+"/"+ep.name, func(t *testing.T) {
				err := ep.decode(tc.in, tc.target(), tc.opts...)
				if _, ok := topDecodeError(err); !ok {
					t.Fatalf("%s(%s) = %T %v; want a *canjson.DecodeError at the top of the chain", ep.name, tc.in, err, err)
				}
				for _, w := range tc.want {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("%s(%s) = %q; want the text to keep %q", ep.name, tc.in, err, w)
					}
				}
				if strings.Contains(err.Error(), plantedValue) {
					t.Errorf("%s(%s) = %q; the text carries a member's value", ep.name, tc.in, err)
				}
			})
		}
	}
}

// TestDecoder_REQ052_ReaderFailureAndEndOfStreamComeBackUnwrapped pins the two
// errors REQ-052 leaves as they are, because the input does not cause them: a
// failing reader's own error, and io.EOF from a Decoder whose stream holds no
// further value, which a caller compares with ==. The reader fails inside a
// member name, inside a polymorphic slot's value and inside a number, in every
// decode mode: jsontext reports a failure partway through a value as a
// *jsontext.SyntacticError around its I/O error, which is still not a failure
// of the input.
//
// Can-fail control: wrap every error Decode returns, or take every
// *jsontext.SyntacticError as caused by the input, and the reader rows fail.
func TestDecoder_REQ052_ReaderFailureAndEndOfStreamComeBackUnwrapped(t *testing.T) {
	const element = `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"e"},"value":`
	readerCases := []struct {
		name   string
		in     string // the bytes the reader serves before it fails
		target func() any
	}{
		{name: "inside a member name", in: `{"_type":"DV_TEXT","val`, target: func() any { return new(rm.DVText) }},
		{name: "inside a slot's value", in: element + `{"_type":"DV_TEXT","value":"Ωq`, target: func() any { return new(rm.Element) }},
		{name: "inside a number", in: `{"_type":"DV_QUANTITY","units":"kg","magnitude":12.3`, target: func() any { return new(rm.DVQuantity) }},
	}
	sentinels := []error{
		canjson.ErrInvalidShape, canjson.ErrUnknownMember,
		typereg.ErrMissingType, typereg.ErrUnknownType, typereg.ErrTypeMismatch, typereg.ErrMaxDepthExceeded,
	}
	for mode, opts := range jsonDecodeModes() {
		for _, rc := range readerCases {
			t.Run("reader failure/"+rc.name+"/"+mode, func(t *testing.T) {
				dec := canjson.NewDecoder(&failingReader{r: strings.NewReader(rc.in)}, opts...)
				err := dec.Decode(rc.target())
				if !errors.Is(err, errRead) {
					t.Fatalf("Decode over a reader failing %s = %v; want errors.Is(_, the reader's error)", rc.name, err)
				}
				if de, ok := errors.AsType[*canjson.DecodeError](err); ok {
					t.Errorf("Decode over a reader failing %s = %v; want the reader's failure unwrapped, got a *canjson.DecodeError (Path %q)", rc.name, err, de.Path)
				}
				for _, s := range sentinels {
					if errors.Is(err, s) {
						t.Errorf("Decode over a reader failing %s = %v; want no SDK sentinel, got errors.Is(_, %v)", rc.name, err, s)
					}
				}
			})
		}
		for _, in := range []string{``, " \n", `{"_type":"DV_TEXT","value":"a"}`} {
			t.Run("end of stream/"+mode+"/"+strconv.Quote(in), func(t *testing.T) {
				dec := canjson.NewDecoder(strings.NewReader(in), opts...)
				var err error
				for range 3 {
					if err = dec.Decode(new(rm.DVText)); err != nil {
						break
					}
				}
				if err != io.EOF { //nolint:errorlint // REQ-052: io.EOF comes back as itself, so a caller's == holds
					t.Errorf("Decode at the end of %q = %T %v; want io.EOF itself", in, err, err)
				}
			})
		}
	}
}

// corpusLiteral is the number planted in place of each numeric leaf: out of
// float64's range, so every numeric RM field refuses it, and distinctive
// enough that its digits cannot appear in an error's text by chance.
const corpusLiteral = "7.7777e777"

// TestUnmarshal_REQ052_CorpusLiteralsStayOutOfDecodeErrorText is a property
// net over the vendored compositions: each numeric leaf, replaced in turn by
// a literal every numeric field refuses, gives a *canjson.DecodeError whose
// text does not carry the literal and whose Value holds it as written. Each
// string leaf, replaced by one with an invalid escape, gives a text that
// carries neither the escape nor the string around it. A new site that quoted
// a value into a decode error's text would fail it on whatever field reached
// that site.
//
// Can-fail control: render a SemanticError's cause with its own Error() and
// every numeric row fails.
func TestUnmarshal_REQ052_CorpusLiteralsStayOutOfDecodeErrorText(t *testing.T) {
	const perDocument = 6 // leaves of each kind tried per document
	paths, err := filepath.Glob(filepath.Join(fixtures.CorpusRoot(), "compositions", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	numbers, strs := 0, 0
	for _, path := range paths {
		doc, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if canjson.Unmarshal(doc, new(rm.Composition)) != nil {
			continue // not a composition this codec decodes whole; nothing to perturb
		}
		numLeaves, strLeaves := leafSpans(t, doc)
		for _, span := range spread(numLeaves, perDocument) {
			in := string(doc[:span[0]]) + corpusLiteral + string(doc[span[1]:])
			for _, ep := range entryPoints {
				err := ep.decode(in, new(rm.Composition))
				if err == nil {
					continue // a field that holds any number takes this one too
				}
				numbers++
				de, ok := topDecodeError(err)
				if !ok {
					t.Errorf("%s: %s with %s at byte %d = %T %v; want a *canjson.DecodeError", filepath.Base(path), ep.name, corpusLiteral, span[0], err, err)
					continue
				}
				if strings.Contains(err.Error(), "7777") {
					t.Errorf("%s: %s with %s at byte %d = %q; the text carries the literal", filepath.Base(path), ep.name, corpusLiteral, span[0], err)
				}
				if got := de.Value.Reveal(); got != corpusLiteral {
					t.Errorf("%s: %s with %s at byte %d: Value.Reveal() = %#v; want the literal as written", filepath.Base(path), ep.name, corpusLiteral, span[0], got)
				}
			}
		}
		for _, span := range spread(strLeaves, perDocument) {
			const planted = `"Jane \q 7777 Doe"`
			in := string(doc[:span[0]]) + planted + string(doc[span[1]:])
			for _, ep := range entryPoints {
				err := ep.decode(in, new(rm.Composition))
				strs++
				de, ok := topDecodeError(err)
				if !ok {
					t.Errorf("%s: %s with %s at byte %d = %T %v; want a *canjson.DecodeError", filepath.Base(path), ep.name, planted, span[0], err, err)
					continue
				}
				for _, v := range []string{"Jane", `\q`, "7777"} {
					if strings.Contains(err.Error(), v) {
						t.Errorf("%s: %s with %s at byte %d = %q; the text carries %q", filepath.Base(path), ep.name, planted, span[0], err, v)
					}
				}
				if got := de.Value.Reveal(); got != nil {
					t.Errorf("%s: %s with %s at byte %d: Value.Reveal() = %#v; want nothing for malformed JSON", filepath.Base(path), ep.name, planted, span[0], got)
				}
			}
		}
	}
	// The net catches nothing if the corpus stops reaching it: a floor on
	// the refusals it saw keeps it from passing on an empty sweep.
	t.Logf("the sweep reached %d numeric and %d string refusals over %d documents", numbers, strs, len(paths))
	if numbers < 100 || strs < 100 {
		t.Errorf("the sweep reached %d numeric and %d string refusals; want at least 100 of each", numbers, strs)
	}
}

// leafSpans returns the byte spans of the number and string values in doc,
// member names excluded.
func leafSpans(t *testing.T, doc []byte) (numbers, strs [][2]int) {
	t.Helper()
	dec := jsontext.NewDecoder(bytes.NewReader(doc))
	for {
		before := dec.InputOffset()
		kind := dec.PeekKind()
		if kind == 0 {
			break
		}
		// Inside an object, an even count of tokens read so far means the
		// next string is a member name.
		container, read := dec.StackIndex(dec.StackDepth())
		isName := container == '{' && read%2 == 0
		if _, err := dec.ReadToken(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("read the corpus document: %v", err)
		}
		end := dec.InputOffset()
		switch {
		case kind == '0':
			start := before + int64(bytes.IndexAny(doc[before:end], "-0123456789"))
			numbers = append(numbers, [2]int{int(start), int(end)})
		case kind == '"' && !isName:
			start := before + int64(bytes.IndexByte(doc[before:end], '"'))
			strs = append(strs, [2]int{int(start), int(end)})
		}
	}
	return numbers, strs
}

// spread picks up to n spans spread evenly over spans, first and last
// included, so a document's leaves are sampled across the whole tree.
func spread(spans [][2]int, n int) [][2]int {
	if len(spans) <= n {
		return spans
	}
	out := make([][2]int, 0, n)
	for i := range n {
		out = append(out, spans[i*(len(spans)-1)/(n-1)])
	}
	return slices.Compact(out)
}
