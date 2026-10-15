package simplified

// REQ-053 — the ctx object in OPT-free FLAT ↔ STRUCTURED interconversion. A
// ctx/ field holds a direct value, and a field carrying |suffixes nests them
// as members of the field's object, its own value then riding the `"|"`
// member, as at a clinical leaf.

import (
	"encoding/json"
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
