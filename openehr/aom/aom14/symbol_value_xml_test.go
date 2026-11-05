package aom14_test

// REQ-176 § Shapes, REQ-056: canonical XML writes the `value` of an ORDINAL or
// SCALE symbol in the DV_CODED_TEXT shape only when it is non-empty; REQ-175:
// the missing-member report holds that shape to the members its schema shape
// requires, so it does not report the `value`; a symbol of neither shape is
// refused with canxml.ErrInvalidShape.

import (
	"encoding/xml"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// symbolValues returns the text of each `value` element directly inside a
// `symbol` element of doc, and how many symbols hold none.
func symbolValues(t *testing.T, doc []byte) (values []string, without int) {
	t.Helper()
	var root struct {
		Symbols []struct {
			Values []string `xml:"value"`
		} `xml:"symbol"`
	}
	if err := xml.Unmarshal(doc, &root); err != nil {
		t.Fatalf("reading %s: %v", doc, err)
	}
	for _, s := range root.Symbols {
		if len(s.Values) == 0 {
			without++
		}
		values = append(values, s.Values...)
	}
	return values, without
}

// TestREQ176_SymbolValueIsWrittenOnlyWhenNonEmpty pins REQ-176's symbol rule:
// in the DV_CODED_TEXT shape, ORDINAL.symbol and SCALE.symbol are written with
// a `value` only when it is non-empty, held by pointer or by value, compact
// and indented, so a symbol read with no `value` or an empty one is written
// with none, and one with text keeps it, with the rest of the symbol as its
// own encoder writes it; each round-trips under strict. A `value` of XML
// whitespace only is written, and reads back empty: it carries no value in
// canonical XML (REQ-175, REQ-056).
func TestREQ176_SymbolValueIsWrittenOnlyWhenNonEmpty(t *testing.T) {
	code := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"}
	language := &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"}
	for _, tc := range []struct {
		name      string
		symbol    rm.DVCodedText
		wantValue []string
		wantBack  string // the symbol's value read back
	}{
		{"empty", rm.DVCodedText{DefiningCode: code}, nil, ""},
		{"empty, with the optional elements", rm.DVCodedText{Language: language, DefiningCode: code}, nil, ""},
		{"with text", rm.DVCodedText{Value: "Mild", DefiningCode: code}, []string{"Mild"}, "Mild"},
		{"white space", rm.DVCodedText{Value: " ", DefiningCode: code}, []string{" "}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sym := tc.symbol
			for name, v := range map[string]canxml.BMMNamer{
				"ORDINAL, by pointer": &aom14.Ordinal{Value: 1, Symbol: &sym},
				"ORDINAL, by value":   &aom14.Ordinal{Value: 1, Symbol: sym},
				"SCALE, by pointer":   &aom14.Scale{Value: 1, Symbol: &sym},
			} {
				for _, indent := range []string{"", "  "} {
					b, err := canxml.MarshalIndent(v, "", indent)
					if err != nil {
						t.Fatalf("canxml.MarshalIndent(%s): %v", name, err)
					}
					values, without := symbolValues(t, b)
					if !reflect.DeepEqual(values, tc.wantValue) || (tc.wantValue == nil) != (without == 1) {
						t.Errorf("%s writes the symbol's values %q (%d with none); want %q:\n%s", name, values, without, tc.wantValue, b)
					}
					if !strings.Contains(string(b), "<code_string>at0001</code_string>") || (tc.symbol.Language != nil) != strings.Contains(string(b), "<language>") {
						t.Errorf("%s loses the rest of the symbol:\n%s", name, b)
					}
					back := reflect.New(reflect.TypeOf(v).Elem()).Interface()
					if err := canxml.Unmarshal(b, back, canxml.WithStrict()); err != nil {
						t.Fatalf("round trip of %s = %v", b, err)
					}
					want := sym
					want.Value = tc.wantBack
					got := reflect.ValueOf(back).Elem().FieldByName("Symbol").Interface()
					if !reflect.DeepEqual(got, &want) {
						t.Errorf("round trip of %s: Symbol = %#v; want %#v", b, got, &want)
					}
				}
			}
		})
	}
}

// TestREQ176_REQ175_SymbolValueIsNotMissing pins that WithMissing holds a
// choice shape to the members its schema shape requires, not to those of the
// RM class that reads it: the DV_CODED_TEXT shape of ORDINAL_SYMBOL makes
// `value` optional, so a symbol with no `value`, or an empty one, reports
// nothing, while one that leaves out its `defining_code`, which the shape
// requires, reports it, as ORDINAL and as SCALE.
func TestREQ176_REQ175_SymbolValueIsNotMissing(t *testing.T) {
	const code = `<defining_code><terminology_id><value>local</value></terminology_id><code_string>at1</code_string></defining_code>`
	for _, class := range []string{"ordinal", "scale"} {
		for _, tc := range []struct {
			name, symbol string
			want         []string
		}{
			{"no value", `<symbol>` + code + `</symbol>`, nil},
			{"an empty value", `<symbol><value></value>` + code + `</symbol>`, nil},
			{"a value", `<symbol><value>Mild</value>` + code + `</symbol>`, nil},
			{"no defining_code", `<symbol><value>Mild</value></symbol>`, []string{"/symbol/defining_code"}},
		} {
			doc := `<` + class + ` xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><value>1</value>` +
				tc.symbol + `</` + class + `>`
			var missing []string
			report := canxml.WithMissing(func(p string) { missing = append(missing, p) })
			into := map[string]any{"ordinal": new(aom14.Ordinal), "scale": new(aom14.Scale)}[class]
			if err := canxml.Unmarshal([]byte(doc), into, report); err != nil {
				t.Fatalf("%s, %s: canxml.Unmarshal = %v", class, tc.name, err)
			}
			if !reflect.DeepEqual(missing, tc.want) {
				t.Errorf("%s, %s: WithMissing reported %q; want %q", class, tc.name, missing, tc.want)
			}
		}
	}
}

// TestREQ176_REQ056_SymbolOfAnotherTypeIsRefused pins that canonical XML
// refuses an ORDINAL or SCALE symbol that holds neither of its shapes, a Go
// value or a class value through a pointer or by value, with
// canxml.ErrInvalidShape, as it refuses an EXPR_LEAF item it cannot write: the
// refusal names the member and the Go type the symbol holds, never the value.
func TestREQ176_REQ056_SymbolOfAnotherTypeIsRefused(t *testing.T) {
	const secret = "Jane Doe"
	for _, symbol := range []struct {
		value any
		text  string // the value's text, which the refusal does not carry
	}{
		{secret, secret},
		{98765, "98765"},
		{rm.DVText{Value: secret}, secret},
		{&rm.DVText{Value: secret}, secret},
	} {
		for member, v := range map[string]canxml.BMMNamer{
			"ORDINAL.symbol": &aom14.Ordinal{Value: 1, Symbol: symbol.value},
			"SCALE.symbol":   &aom14.Scale{Value: 1, Symbol: symbol.value},
		} {
			out, err := canxml.Marshal(v)
			if !errors.Is(err, canxml.ErrInvalidShape) {
				t.Errorf("canxml.Marshal(%s holding %T) = %s, %v; want canxml.ErrInvalidShape", member, symbol.value, out, err)
				continue
			}
			got, typ := err.Error(), fmt.Sprintf("%T", symbol.value)
			if !strings.Contains(got, member+" holds "+typ) || strings.Contains(got, symbol.text) {
				t.Errorf("canxml.Marshal(%s holding %T) = %q; want it to name %s and %s, and not the value", member, symbol.value, got, member, typ)
			}
		}
	}
}
