package canjson_test

import (
	v1json "encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestMarshalMapValueIncludesType pins REQ-052 / ADR 0022: an RM value held as
// a map value re-encodes with its `_type` on every entry under
// encoding/json/v2, where under v1 the same map value was unaddressable and
// lost its discriminator. Ruling R19's value-receiver MarshalJSONTo is what
// also carries it from the v1 entry point, which the v1 leg pins.
//
// Can-fail control: drop TranslationDetails' MarshalJSONTo method entirely and
// both legs re-encode the map value as its bare struct fields with no `_type`.
// Switching the method to a pointer receiver reddens only the v1 leg: v2 still
// calls a pointer-receiver method on a map value, v1 does not.
func TestMarshalMapValueIncludesType(t *testing.T) {
	translations := map[string]rm.TranslationDetails{
		"en": {
			Author: map[string]string{"name": "translator"},
			Language: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
				CodeString:    "en",
			},
		},
		"nl": {
			Author: map[string]string{"name": "vertaler"},
			Language: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
				CodeString:    "nl",
			},
		},
	}
	a := aom14.Archetype{Translations: &translations}
	legs := []struct {
		name    string
		marshal func(any) ([]byte, error)
	}{
		{"canjson", canjson.Marshal},
		{"encoding/json v1", v1json.Marshal},
	}
	for _, leg := range legs {
		t.Run(leg.name, func(t *testing.T) {
			got, err := leg.marshal(&a)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if n := strings.Count(string(got), `"_type":"TRANSLATION_DETAILS"`); n != len(translations) {
				t.Fatalf("each encoded map value must carry _type: got %d of %d; output %s", n, len(translations), got)
			}
		})
	}
}
