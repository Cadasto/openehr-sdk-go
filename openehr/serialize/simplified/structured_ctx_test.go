package simplified

// REQ-053 — the ctx object in OPT-free FLAT ↔ STRUCTURED interconversion. A
// ctx/ field holds a direct value, and a field carrying |suffixes nests them
// as members of the field's object, its own value then riding the `"|"`
// member, as at a clinical leaf.

import (
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

// TestFlatToStructuredRefusesAnObjectCtxValue — REQ-053: a ctx/ field holds a
// value, and its suffixes ride keys of their own. A direct value that is a
// JSON object has the shape STRUCTURED gives those suffixes, so FLAT →
// STRUCTURED refuses it naming the key, rather than merging it with the
// field's |suffix keys, where one would silently overwrite the other, or
// handing it to STRUCTURED → FLAT to read back as suffixes. A scalar direct
// value beside the field's suffixes stays refused as before.
func TestFlatToStructuredRefusesAnObjectCtxValue(t *testing.T) {
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

// TestStructuredLocationsMirrorTheConversion — REQ-170: the STRUCTURED
// locator records each FLAT key [structuredToFlat] makes of a body under the
// key the conversion writes, so a report or a strict refusal of that key can
// name its member. A ctx field's `"|"` member is its own key, as a leaf's is.
func TestStructuredLocationsMirrorTheConversion(t *testing.T) {
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
