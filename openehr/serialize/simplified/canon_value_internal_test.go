package simplified

import (
	jsonv2 "encoding/json/v2"
	"reflect"
	"testing"
)

// canonValueSeeds are JSON objects whose members [canonValue] must read as a
// decode into map[string]any does: strings carrying the bytes the scanner
// steps over (quotes, brackets, braces, commas, colons, backslashes), escaped
// member names, nested arrays and objects, every literal kind, and whitespace
// between tokens.
var canonValueSeeds = []string{
	`{}`,
	`{"_type":"DV_TEXT","value":"x"}`,
	`{"value":"a \"quoted\" } ] { [ , : \\ value"}`,
	`{"a\"b":1,"c\\d":true,"\u0065":null}`,
	`{"n":-1.5e+3,"t":true,"f":false,"z":null,"arr":[1,{"x":"]"},[[]],"}"],"obj":{"k":{"k":"v"}}}`,
	`{ "spaced" : "yes" , "next" : [ 1 , 2 ] , "last" : { } }`,
	`{"_type":"DV_CODED_TEXT","value":"v","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"},"code_string":"at0001"}}`,
	`{"value":"\u00e9t\u00e9 \ud83d\ude00"}`,
}

// TestCanonValueReadsAsDecodedObject holds the in-place readers the capture
// checks use to what decoding the same canonical bytes into map[string]any
// gives: the same member names, each value decoding to the same thing, the
// same member count, and a string read as the decoded string.
func TestCanonValueReadsAsDecodedObject(t *testing.T) {
	for _, in := range canonValueSeeds {
		t.Run(in, func(t *testing.T) {
			if msg := canonValueMismatch([]byte(in)); msg != "" {
				t.Error(msg)
			}
		})
	}
}

// FuzzCanonValueReadsAsDecodedObject extends [TestCanonValueReadsAsDecodedObject]
// to any object encoding/json/v2 decodes, and checks the readers stop on any
// other input without panicking.
func FuzzCanonValueReadsAsDecodedObject(f *testing.F) {
	for _, in := range canonValueSeeds {
		f.Add([]byte(in))
	}
	for _, in := range []string{`{"a`, `{"a":`, `{"a":"b`, `{"a\"`, `{"a" "b"}`, `{"a":[1,"]`, `{`, `"x"`, `null`, ``} {
		f.Add([]byte(in))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		if msg := canonValueMismatch(in); msg != "" {
			t.Error(msg)
		}
	})
}

// canonValueMismatch reads in through [canonValue] and returns how the result
// differs from decoding in into map[string]any, or "" when it does not. Input
// that is no object, or that the decoder refuses, is only read, to show the
// readers do not panic on it.
func canonValueMismatch(in []byte) string {
	v := canonValue(in)
	n := 0
	for range v.members {
		n++
	}
	var want map[string]any
	if !v.isObject() || jsonv2.Unmarshal(in, &want) != nil {
		return ""
	}
	if got := v.memberCount(); got != len(want) || got != n {
		return "memberCount(" + string(in) + ") disagrees with the decoded member count"
	}
	for name, raw := range v.members {
		w, ok := want[string(name)]
		if !ok {
			return "members(" + string(in) + ") yields " + string(name) + ", which the decode does not have"
		}
		var got any
		if err := jsonv2.Unmarshal(raw, &got); err != nil {
			return "members(" + string(in) + ") yields for " + string(name) + " the value " + string(raw) + ", which does not decode: " + err.Error()
		}
		if !reflect.DeepEqual(got, w) {
			return "members(" + string(in) + ") yields for " + string(name) + " the value " + string(raw) + ", which decodes unlike the whole object's member"
		}
		if m := v.member(string(name)); string(m) != string(raw) {
			return "member(" + string(name) + ") of " + string(in) + " = " + string(m) + ", want " + string(raw)
		}
		s, isString := w.(string)
		if raw.isString() != isString || !raw.textIs(s) {
			return "textIs on " + string(name) + " of " + string(in) + " does not read the decoded string"
		}
		if isString && s != "" && raw.textIs("") {
			return "textIs(\"\") on " + string(name) + " of " + string(in) + " reads a non-empty string as empty"
		}
		if raw.isNil() != (w == nil) {
			return "isNil on " + string(name) + " of " + string(in) + " disagrees with the decoded value"
		}
	}
	if v.member("\x00absent") != nil {
		return "member of a name " + string(in) + " lacks is not nil"
	}
	return ""
}

// TestCanonValueTextIsReadsAbsentAndNonStringAsEmpty pins the read the capture
// checks rely on for code_string and terminology_id: a member that is absent,
// null or not a string reads as the empty string, and only a string reads as
// itself.
func TestCanonValueTextIsReadsAbsentAndNonStringAsEmpty(t *testing.T) {
	for _, tc := range []struct {
		v    canonValue
		s    string
		want bool
	}{
		{v: nil, s: "", want: true},
		{v: canonValue(`null`), s: "", want: true},
		{v: canonValue(`1`), s: "", want: true},
		{v: canonValue(`1`), s: "1", want: false},
		{v: canonValue(`{"value":"x"}`), s: "", want: true},
		{v: canonValue(`""`), s: "", want: true},
		{v: canonValue(`"x"`), s: "x", want: true},
		{v: canonValue(`"x"`), s: "", want: false},
		{v: canonValue(`"a\"b"`), s: `a"b`, want: true},
		{v: canonValue(`"a\"b"`), s: `a\"b`, want: false},
		{v: canonValue(`"\u00e9"`), s: "é", want: true},
	} {
		if got := tc.v.textIs(tc.s); got != tc.want {
			t.Errorf("canonValue(%s).textIs(%q) = %v, want %v", tc.v, tc.s, got, tc.want)
		}
	}
}

// TestCanonicalObjectNonObjectForms pins the two forms canonical JSON can take
// that are no object: null reads as an empty object, as it decodes into a nil
// map, and any other form is refused with the error decoding it into an object
// gives.
func TestCanonicalObjectNonObjectForms(t *testing.T) {
	got, err := canonicalObject(nil)
	if err != nil || string(got) != "{}" {
		t.Errorf("canonicalObject(nil) = %s, %v; want {}, nil", got, err)
	}
	var m map[string]any
	wantErr := jsonv2.Unmarshal([]byte(`"abc"`), &m)
	if wantErr == nil {
		t.Fatal(`decoding "abc" into map[string]any succeeded, want an error to compare against`)
	}
	if _, err := canonicalObject("abc"); err == nil || err.Error() != wantErr.Error() {
		t.Errorf("canonicalObject(%q) error = %v, want %v", "abc", err, wantErr)
	}
}
