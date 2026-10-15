package simplified

// REQ-053 — the ctx object in OPT-free FLAT ↔ STRUCTURED interconversion. A
// ctx/ field holds a direct value, and a field carrying |suffixes nests them
// as members of the field's object, its own value then riding the `"|"`
// member, as at a clinical leaf.

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestREQ053CtxBareMemberIsTheFieldKey — REQ-053: STRUCTURED → FLAT reads a
// ctx field's `"|"` member back as the field's own key, whether it holds null
// or a value, as it does at a leaf. It is never spelled `ctx/<field>|`.
func TestREQ053CtxBareMemberIsTheFieldKey(t *testing.T) {
	for name, tc := range map[string]struct {
		structured string
		want       map[string]any
	}{
		"null":  {`{"ctx":{"setting":{"|":null,"|code":"238"}}}`, map[string]any{"ctx/setting": nil, "ctx/setting|code": "238"}},
		"value": {`{"ctx":{"setting":{"|":"x","|code":"238"}}}`, map[string]any{"ctx/setting": "x", "ctx/setting|code": "238"}},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := StructuredToFlat([]byte(tc.structured))
			if err != nil {
				t.Fatalf("StructuredToFlat(%s) = %v", tc.structured, err)
			}
			var got map[string]any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("StructuredToFlat(%s) = %s, want %s", tc.structured, out, mustJSON(t, tc.want))
			}
		})
	}
}

// TestREQ053FlatToStructuredRefusesAnObjectCtxValue — REQ-053: a ctx/ field holds a
// value, and its suffixes ride keys of their own. A direct value that is a
// JSON object has the shape STRUCTURED gives those suffixes, so FLAT →
// STRUCTURED refuses it naming the key, rather than merging it with the
// field's |suffix keys, where one would silently overwrite the other, or
// handing it to STRUCTURED → FLAT to read back as suffixes. A scalar direct
// value beside the field's suffixes stays refused as before.
func TestREQ053FlatToStructuredRefusesAnObjectCtxValue(t *testing.T) {
	for name, tc := range map[string]struct {
		flat  string
		want  error
		names string
	}{
		"an object beside the field's suffix": {`{"ctx/setting":{"|code":"999"},"ctx/setting|code":"238"}`, ErrUnsupportedDatatype, `"ctx/setting"`},
		"an object alone":                     {`{"ctx/setting":{"|code":"999"}}`, ErrUnsupportedDatatype, `"ctx/setting"`},
		"a scalar beside the field's suffix":  {`{"ctx/setting":"x","ctx/setting|code":"238"}`, ErrUnknownPath, `"ctx/setting|code"`},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := FlatToStructured([]byte(tc.flat))
			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), tc.names) {
				t.Errorf("FlatToStructured(%s) = %s, %v; want %v naming %s", tc.flat, out, err, tc.want, tc.names)
			}
		})
	}
}

// TestREQ170StructuredLocationsMirrorTheConversion — REQ-170: the STRUCTURED
// locator records each FLAT key [structuredToFlat] makes of a body under the
// key the conversion writes, so a report or a strict refusal of that key can
// name its member. A ctx field's `"|"` member is its own key, as a leaf's is.
func TestREQ170StructuredLocationsMirrorTheConversion(t *testing.T) {
	body := `{"ctx":{"language":"en","setting":{"|":null,"|code":"238","|value":"other care"}},` +
		`"root":{"leaf":[{"|":"t","|formatting":"f"}],"other":["x"]}}`
	var s map[string]any
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatal(err)
	}
	flat, err := structuredToFlat(s)
	if err != nil {
		t.Fatalf("structuredToFlat: %v", err)
	}
	loc, err := structuredLocations([]byte(body))
	if err != nil {
		t.Fatalf("structuredLocations: %v", err)
	}
	for _, key := range slices.Sorted(maps.Keys(flat)) {
		if _, ok := loc[key]; !ok {
			t.Errorf("structuredLocations has no %q, a key structuredToFlat writes", key)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(loc)) {
		if _, written := flat[key]; !written && !strings.HasPrefix(key, memberKeyPrefix) {
			t.Errorf("structuredLocations records %q, a key structuredToFlat does not write", key)
		}
	}
	if got := loc["ctx/setting"].path; got != "ctx/setting/|" {
		t.Errorf(`location of "ctx/setting" = %q, want the "|" member's path "ctx/setting/|"`, got)
	}
}

// TestREQ140CtxValueBesideSuffixesRoundTrips — REQ-140: "OPT-free FLAT ↔
// STRUCTURED interconversion **MUST** preserve it", for a ctx/ field holding a
// value beside its own |suffixes, both ways. FLAT → STRUCTURED gives the value
// the `"|"` member of the field's object, as it gives a null one and as a leaf
// slot gives a bare value beside its suffixes, and STRUCTURED → FLAT reads
// that member back as the field's own key. Before, FLAT → STRUCTURED refused
// the body STRUCTURED → FLAT had just written.
func TestREQ140CtxValueBesideSuffixesRoundTrips(t *testing.T) {
	for name, tc := range map[string]struct{ flat, structured string }{
		"a string": {
			`{"ctx/setting":"x","ctx/setting|code":"238"}`,
			`{"ctx":{"setting":{"|":"x","|code":"238"}}}`,
		},
		"a number beside two suffixes": {
			`{"ctx/setting":1,"ctx/setting|code":"238","ctx/setting|value":"other care"}`,
			`{"ctx":{"setting":{"|":1,"|code":"238","|value":"other care"}}}`,
		},
		"a Boolean": {
			`{"ctx/composer_self":true,"ctx/composer_self|x":"y"}`,
			`{"ctx":{"composer_self":{"|":true,"|x":"y"}}}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, err := FlatToStructured([]byte(tc.flat))
			if err != nil {
				t.Fatalf("FlatToStructured(%s) = %v", tc.flat, err)
			}
			assertSameJSON(t, "FlatToStructured("+tc.flat+")", s, tc.structured)
			back, err := StructuredToFlat(s)
			if err != nil {
				t.Fatalf("StructuredToFlat(%s) = %v", s, err)
			}
			assertSameJSON(t, "StructuredToFlat("+string(s)+")", back, tc.flat)

			f, err := StructuredToFlat([]byte(tc.structured))
			if err != nil {
				t.Fatalf("StructuredToFlat(%s) = %v", tc.structured, err)
			}
			assertSameJSON(t, "StructuredToFlat("+tc.structured+")", f, tc.flat)
			again, err := FlatToStructured(f)
			if err != nil {
				t.Fatalf("FlatToStructured(%s) = %v", f, err)
			}
			assertSameJSON(t, "FlatToStructured("+string(f)+")", again, tc.structured)
		})
	}
}

// TestREQ140CtxValueBesideSuffixesDecodesAlikeInBothForms — REQ-140 and
// REQ-170: a FLAT body carrying a ctx/ field's value beside its suffixes and
// the STRUCTURED form FLAT → STRUCTURED gives it decode alike. A
// `ctx/language` beside a `|code` the field does not read decodes in both,
// and a bare `ctx/setting` beside its pair is refused in both, naming the key.
func TestREQ140CtxValueBesideSuffixesDecodesAlikeInBothForms(t *testing.T) {
	wt, _ := conformanceWT(t)
	t.Run("decoded", func(t *testing.T) {
		body := rmattrBody(map[string]any{"ctx/language|code": "en"})
		flat, err := UnmarshalFlat(mustJSON(t, body), wt)
		if err != nil {
			t.Fatalf("UnmarshalFlat = %v", err)
		}
		structured, err := UnmarshalStructured(convertToStructured(t, body), wt)
		if err != nil {
			t.Fatalf("UnmarshalStructured = %v", err)
		}
		if got, want := canonical(t, structured), canonical(t, flat); !bytes.Equal(got, want) {
			t.Errorf("STRUCTURED decodes to\n%s\nwant what FLAT decodes to\n%s", got, want)
		}
	})
	t.Run("refused", func(t *testing.T) {
		body := rmattrBody(map[string]any{"ctx/setting": "other care", "ctx/setting|code": "238", "ctx/setting|value": "other care"})
		_, flatErr := UnmarshalFlat(mustJSON(t, body), wt)
		_, structuredErr := UnmarshalStructured(convertToStructured(t, body), wt)
		for form, err := range map[string]error{"FLAT": flatErr, "STRUCTURED": structuredErr} {
			if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), `"ctx/setting"`) {
				t.Errorf("%s: decode = %v, want ErrUnsupportedDatatype naming \"ctx/setting\"", form, err)
			}
		}
	})
}

// convertToStructured is the STRUCTURED form FLAT → STRUCTURED gives the FLAT
// body, failing the test on an error.
func convertToStructured(t *testing.T, body map[string]any) []byte {
	t.Helper()
	s, err := FlatToStructured(mustJSON(t, body))
	if err != nil {
		t.Fatalf("FlatToStructured = %v", err)
	}
	return s
}

// assertSameJSON fails unless got and want hold the same JSON value; call
// names the conversion that gave got.
func assertSameJSON(t *testing.T, call string, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("%s: %v", call, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("%s = %s, want %s", call, got, want)
	}
}
