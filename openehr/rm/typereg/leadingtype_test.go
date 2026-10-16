package typereg

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"strings"
	"testing"
)

// sdkSlotOptions returns the options a polymorphic slot's decoder carries on
// the SDK's own decode path: the aggregate interface hooks and v2 error
// semantics, as hookOptions builds them.
func sdkSlotOptions() json.Options {
	unmarshalers, legacySemantics := hookOptions(aggregateUnmarshalers())
	return json.JoinOptions(unmarshalers, legacySemantics)
}

// lowercaseTypeHook is a caller's string hook that rewrites every string it
// decodes, _type included, to lower case: code a caller may run on _type
// through peekType, which leadingType cannot run.
func lowercaseTypeHook() json.Options {
	rewrite := json.UnmarshalFromFunc(func(dec *jsontext.Decoder, s *string) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return err
		}
		*s = strings.ToLower(tok.String())
		return nil
	})
	return json.WithUnmarshalers(json.JoinUnmarshalers(aggregateUnmarshalers(), rewrite))
}

// leadingTypeAgrees reports a disagreement between leadingType and peekType on
// raw under opts, or nil when they agree. They agree when leadingType declines
// (the decode then falls back to peekType) or when peekType succeeds and reads
// the same name leadingType returned.
func leadingTypeAgrees(raw jsontext.Value, opts json.Options) error {
	name, ok := leadingType(raw, opts)
	if !ok {
		return nil
	}
	peeked, err := peekType(raw, opts)
	switch {
	case err != nil:
		return fmt.Errorf("leadingType(%q) = %q, true, but peekType failed: %w", raw, name, err)
	case peeked != name:
		return fmt.Errorf("leadingType(%q) = %q, true, but peekType = %q", raw, name, peeked)
	}
	return nil
}

// TestLeadingType pins the shapes leadingType reads without a decode and every
// shape and option set it leaves to peekType. Whenever it reads a name, the
// row also checks that peekType reads the same one.
func TestLeadingType(t *testing.T) {
	sdk := sdkSlotOptions()
	cases := []struct {
		name   string
		raw    string
		opts   json.Options // nil: sdk
		want   string
		wantOK bool
	}{
		{name: "_type first", raw: `{"_type":"DV_TEXT","value":"x"}`, want: "DV_TEXT", wantOK: true},
		{name: "_type only", raw: `{"_type":"ELEMENT"}`, want: "ELEMENT", wantOK: true},
		{name: "whitespace around name, colon and value", raw: " {\t\"_type\" \n:\r\n \"DV_CODED_TEXT\" , \"value\":\"x\"}", want: "DV_CODED_TEXT", wantOK: true},
		{name: "generic type name", raw: `{"_type":"DV_INTERVAL<DV_QUANTITY>"}`, want: "DV_INTERVAL<DV_QUANTITY>", wantOK: true},
		{name: "empty name", raw: `{"_type":""}`, want: "", wantOK: true},
		{name: "duplicate names explicitly refused", raw: `{"_type":"DV_TEXT"}`, opts: json.JoinOptions(sdk, jsontext.AllowDuplicateNames(false)), want: "DV_TEXT", wantOK: true},
		{name: "case-sensitive names explicitly", raw: `{"_type":"DV_TEXT"}`, opts: json.JoinOptions(sdk, json.MatchCaseInsensitiveNames(false)), want: "DV_TEXT", wantOK: true},

		{name: "_type not first", raw: `{"value":"x","_type":"DV_TEXT"}`},
		{name: "escaped member name", raw: `{"\u005ftype":"DV_TEXT"}`},
		{name: "longer member name", raw: `{"_types":"DV_TEXT"}`},
		{name: "case variant member name", raw: `{"_TYPE":"DV_TEXT"}`},
		{name: "escaped value", raw: `{"_type":"DV\u005fTEXT"}`},
		{name: "escaped quote in value", raw: `{"_type":"DV\"TEXT"}`},
		{name: "non-ASCII value", raw: `{"_type":"DV_TÉXT"}`},
		{name: "number _type", raw: `{"_type":1}`},
		{name: "null _type", raw: `{"_type":null}`},
		{name: "object _type", raw: `{"_type":{}}`},
		{name: "empty object", raw: `{}`},
		{name: "array", raw: `["_type","DV_TEXT"]`},
		{name: "string", raw: `"_type"`},
		{name: "null", raw: `null`},
		{name: "empty input", raw: ``},
		{name: "unterminated value", raw: `{"_type":"DV_TEXT`},

		{name: "duplicate names allowed", raw: `{"_type":"DV_TEXT"}`, opts: json.JoinOptions(sdk, jsontext.AllowDuplicateNames(true))},
		{name: "case-insensitive names", raw: `{"_type":"DV_TEXT"}`, opts: json.JoinOptions(sdk, json.MatchCaseInsensitiveNames(true))},
		{name: "caller hooks", raw: `{"_type":"DV_TEXT"}`, opts: json.JoinOptions(sdk, lowercaseTypeHook())},
		{name: "no hook set", raw: `{"_type":"DV_TEXT"}`, opts: jsonv1.ReportErrorsWithLegacySemantics(false)},
		{name: "v1 defaults", raw: `{"_type":"DV_TEXT"}`, opts: json.JoinOptions(jsonv1.DefaultOptionsV1(), sdk)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			if opts == nil {
				opts = sdk
			}
			got, ok := leadingType(jsontext.Value(tc.raw), opts)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("leadingType(%q) = %q, %v; want %q, %v", tc.raw, got, ok, tc.want, tc.wantOK)
			}
			if err := leadingTypeAgrees(jsontext.Value(tc.raw), opts); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestLeadingTypeGuardsAgreeWithPeekType pins that each guard in leadingType is
// load-bearing. In each row the full decode does not read the leading member's
// raw bytes as _type: with duplicate names allowed a later _type wins, with
// case-insensitive matching a later _TYPE also matches (and is refused as a
// duplicate), a caller's string hook rewrites the value, an escape decodes to
// another byte, and with invalid UTF-8 allowed an invalid byte decodes to
// U+FFFD. The control asserts that, so each row still discriminates if
// encoding/json/v2 changes. With the matching guard removed, leadingType
// returns the leading bytes and the agreement check fails.
func TestLeadingTypeGuardsAgreeWithPeekType(t *testing.T) {
	sdk := sdkSlotOptions()
	cases := []struct {
		name    string
		raw     string
		opts    json.Options
		leading string // the leading _type value's raw bytes
	}{
		{name: "duplicate names allowed", raw: `{"_type":"DV_TEXT","_type":"DV_CODED_TEXT"}`, opts: json.JoinOptions(sdk, jsontext.AllowDuplicateNames(true)), leading: "DV_TEXT"},
		{name: "case-insensitive names", raw: `{"_type":"DV_TEXT","_TYPE":"DV_CODED_TEXT"}`, opts: json.JoinOptions(sdk, json.MatchCaseInsensitiveNames(true)), leading: "DV_TEXT"},
		{name: "caller string hook", raw: `{"_type":"DV_TEXT"}`, opts: json.JoinOptions(sdk, lowercaseTypeHook()), leading: "DV_TEXT"},
		{name: "escaped value", raw: `{"_type":"DV\u005fTEXT"}`, opts: sdk, leading: `DV\u005fTEXT`},
		{name: "invalid UTF-8 allowed", raw: "{\"_type\":\"DV\xffTEXT\"}", opts: json.JoinOptions(sdk, jsontext.AllowInvalidUTF8(true)), leading: "DV\xffTEXT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := jsontext.Value(tc.raw)
			if peeked, err := peekType(raw, tc.opts); err == nil && peeked == tc.leading {
				t.Fatalf("control: peekType(%q) = %q, nil under %s; want another name or an error, or this row no longer tests the guard", tc.raw, peeked, tc.name)
			}
			if err := leadingTypeAgrees(raw, tc.opts); err != nil {
				t.Errorf("%s: %v", tc.name, err)
			}
		})
	}
}

// FuzzLeadingTypeAgreesWithPeekType checks that, under the SDK's own slot
// options, leadingType never reads a _type that peekType would read
// differently or refuse: for any syntactically valid JSON value, either
// leadingType declines or peekType succeeds with the same name.
func FuzzLeadingTypeAgreesWithPeekType(f *testing.F) {
	for _, seed := range []string{
		`{"_type":"DV_TEXT","value":"systolic"}`,
		`{"_type":"DV_CODED_TEXT","value":"present","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"},"code_string":"at0001"}}`,
		`{"_type":"ELEMENT","archetype_node_id":"at0004","name":{"_type":"DV_TEXT","value":"systolic"},"value":{"_type":"DV_QUANTITY","magnitude":120,"units":"mm[Hg]"}}`,
		`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`,
		"{ \"_type\" : \"DV_TEXT\" , \"value\" : \"x\" }",
		"{\n\t\"_type\":\r\n\"DV_TEXT\"}",
		`{"_type":"DV_TEXT","_type":"DV_CODED_TEXT"}`,
		`{"_type":"DV_TEXT","_TYPE":"DV_CODED_TEXT"}`,
		`{"value":"x","_type":"DV_TEXT"}`,
		`{"\u005ftype":"DV_TEXT"}`,
		`{"_type":"DV\u005fTEXT"}`,
		`{"_type":"DV_TÉXT"}`,
		`{"_type":""}`,
		`{"_type":null}`,
		`{"_type":1}`,
		`{}`,
		`[]`,
		`"DV_TEXT"`,
	} {
		f.Add([]byte(seed))
	}
	opts := sdkSlotOptions()
	f.Fuzz(func(t *testing.T, b []byte) {
		raw := jsontext.Value(b)
		if !raw.IsValid() {
			return
		}
		if err := leadingTypeAgrees(raw, opts); err != nil {
			t.Fatal(err)
		}
	})
}
