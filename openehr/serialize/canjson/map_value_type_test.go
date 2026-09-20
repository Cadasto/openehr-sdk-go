package canjson_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestMarshalMapValueIncludesType pins ADR 0022: an RM value held as a map
// value re-encodes with its `_type` on every entry under encoding/json/v2,
// where under v1 the same map value was unaddressable and lost its
// discriminator (ruling R19's value-receiver MarshalJSONTo is what also carries
// it from the v1 entry points).
//
// Can-fail control: drop TranslationDetails' MarshalJSONTo method entirely and
// the map value re-encodes as its bare struct fields with no `_type`, while
// ordinary struct-field encodes on other types still pass. (Switching the
// method to a pointer receiver is NOT a can-fail here: v2 still calls it on a
// map value, so `_type` would still be emitted.)
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
