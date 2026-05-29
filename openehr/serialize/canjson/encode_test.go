package canjson_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestMarshalDVQuantityEmitsTypeFirst asserts the `_type`-first rule
// from REQ-052 by reading the literal first key of the encoded
// output.
func TestMarshalDVQuantityEmitsTypeFirst(t *testing.T) {
	q := &rm.DVQuantity{Magnitude: 80.5, Units: "kg"}
	got, err := canjson.Marshal(q)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.HasPrefix(string(got), `{"_type":"DV_QUANTITY"`) {
		t.Errorf("output must start with _type=DV_QUANTITY first: %s", got)
	}
}

// TestMarshalDVQuantityEmitsNilOptionalAsAbsent asserts that
// nil-pointer optional fields encode as ABSENT (no key), not `null`.
func TestMarshalDVQuantityEmitsNilOptionalAsAbsent(t *testing.T) {
	q := &rm.DVQuantity{Magnitude: 80.5, Units: "kg"}
	got, err := canjson.Marshal(q)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, banned := range []string{"accuracy", "magnitude_status", "normal_range", "normal_status", "other_reference_ranges", "precision", "units_display_name", "units_system"} {
		if strings.Contains(string(got), `"`+banned+`"`) {
			t.Errorf("nil-pointer optional %q must be absent: %s", banned, got)
		}
	}
}

// TestMarshalDVQuantityRoundsTripStructurally re-decodes the encoded
// output into a generic map and checks the field values survive.
func TestMarshalDVQuantityRoundsTripStructurally(t *testing.T) {
	q := &rm.DVQuantity{Magnitude: 80.5, Units: "kg"}
	got, err := canjson.Marshal(q)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(got, &generic); err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if generic["_type"] != "DV_QUANTITY" {
		t.Errorf("_type = %v; want DV_QUANTITY", generic["_type"])
	}
	if generic["magnitude"].(float64) != 80.5 {
		t.Errorf("magnitude = %v; want 80.5", generic["magnitude"])
	}
	if generic["units"].(string) != "kg" {
		t.Errorf("units = %v; want kg", generic["units"])
	}
}

// TestMarshalCompositionEmitsContentTypePerItem ensures each polymorphic
// content item carries its own `_type`. Items inside a `Composition.Content`
// slice are stored under the abstract `ContentItem` interface; the
// generated MarshalJSON on the concrete type (here a single
// `*rm.Observation`) carries the discriminator.
func TestMarshalCompositionEmitsContentTypePerItem(t *testing.T) {
	c := &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            &rm.DVText{Value: "body_weight"},
		Language:        rm.CodePhrase{CodeString: "en"},
		Territory:       rm.CodePhrase{CodeString: "GB"},
		Category:        rm.DVCodedText{DVText: rm.DVText{Value: "event"}},
		Composer:        &rm.PartySelf{},
		Content: []rm.ContentItem{
			&rm.Observation{
				ArchetypeNodeID: "openEHR-EHR-OBSERVATION.body_weight.v2",
				Name:            &rm.DVText{Value: "Body weight"},
				Language:        rm.CodePhrase{CodeString: "en"},
				Encoding:        rm.CodePhrase{CodeString: "UTF-8"},
				Subject:         &rm.PartySelf{},
			},
		},
	}
	got, err := canjson.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(got)
	if !strings.HasPrefix(s, `{"_type":"COMPOSITION"`) {
		t.Errorf("outer _type missing: %s", s)
	}
	if !strings.Contains(s, `{"_type":"OBSERVATION"`) {
		t.Errorf("inner OBSERVATION _type missing: %s", s)
	}
	if !strings.Contains(s, `{"_type":"PARTY_SELF"`) {
		t.Errorf("composer PARTY_SELF _type missing: %s", s)
	}
}

// TestMarshalEmptyContainerAbsent — REQ-052: empty containers with
// cardinality.lower == 0 (i.e. fields tagged `omitempty`) encode as
// ABSENT, not as `[]`.
func TestMarshalEmptyContainerAbsent(t *testing.T) {
	c := &rm.Composition{
		ArchetypeNodeID: "x",
		Name:            &rm.DVText{Value: "x"},
		Language:        rm.CodePhrase{CodeString: "en"},
		Territory:       rm.CodePhrase{CodeString: "GB"},
		Category:        rm.DVCodedText{DVText: rm.DVText{Value: "event"}},
		Composer:        &rm.PartySelf{},
		Content:         nil, // empty; must be absent on the wire
	}
	got, err := canjson.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(got), `"content"`) {
		t.Errorf("empty content must be absent: %s", got)
	}
}
