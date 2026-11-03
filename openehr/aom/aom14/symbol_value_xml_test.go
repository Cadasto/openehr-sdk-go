package aom14_test

// REQ-176 § Shapes, REQ-056: canonical XML writes the `value` of an ORDINAL or
// SCALE symbol in the DV_CODED_TEXT shape only when it is non-empty.

import (
	"encoding/xml"
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
// own encoder writes it; each round-trips under strict.
func TestREQ176_SymbolValueIsWrittenOnlyWhenNonEmpty(t *testing.T) {
	code := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"}
	language := &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"}
	for _, tc := range []struct {
		name      string
		symbol    rm.DVCodedText
		wantValue []string
	}{
		{"empty", rm.DVCodedText{DefiningCode: code}, nil},
		{"empty, with the optional elements", rm.DVCodedText{Language: language, DefiningCode: code}, nil},
		{"with text", rm.DVCodedText{Value: "Mild", DefiningCode: code}, []string{"Mild"}},
		{"white space", rm.DVCodedText{Value: " ", DefiningCode: code}, []string{" "}},
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
					got := reflect.ValueOf(back).Elem().FieldByName("Symbol").Interface()
					if !reflect.DeepEqual(got, &sym) {
						t.Errorf("round trip of %s: Symbol = %#v; want %#v", b, got, &sym)
					}
				}
			}
		})
	}
}
