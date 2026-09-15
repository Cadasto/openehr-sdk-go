package canjson_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestMarshalMapValueIncludesType pins ADR 0022: an RM value held as a map
// value is addressable under encoding/json/v2, so the value-receiver
// MarshalJSONTo emits `_type` on every entry. Under v1 the same map value was
// unaddressable and lost its discriminator.
//
// Can-fail control: switch TranslationDetails back to a pointer-receiver-only
// MarshalJSONTo (or drop the value receiver) and this test fails while ordinary
// struct-field encodes still pass.
func TestMarshalMapValueIncludesType(t *testing.T) {
	translations := map[string]rm.TranslationDetails{
		"en": {
			Author: map[string]string{"name": "translator"},
			Language: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
				CodeString:    "en",
			},
		},
	}
	a := aom14.Archetype{Translations: &translations}
	got, err := canjson.Marshal(&a)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(got), `"_type":"TRANSLATION_DETAILS"`) {
		t.Fatalf("encoded map value must carry _type; got %s", got)
	}
}
