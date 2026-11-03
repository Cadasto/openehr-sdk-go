package aom14_test

// REQ-176, REQ-052: canonical JSON writes the operational template model's
// classes under their registered type names and a `Hash` as a JSON object.

import (
	json "encoding/json/v2"
	"os"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ176_REQ052_OperationalTemplateCanonicalJSON pins canonical JSON for
// the operational template model: social.opt, read from canonical XML, writes
// its classes under the type names the registry holds, OPERATIONAL_TEMPLATE
// at the root and C_ARCHETYPE_ROOT for its definition, and a `Hash` as a JSON
// object, an ARCHETYPE_TERM's items by key and RESOURCE_DESCRIPTION's details
// by language; and the JSON decodes back to the same value.
func TestREQ176_REQ052_OperationalTemplateCanonicalJSON(t *testing.T) {
	raw, err := os.ReadFile("../../../testkit/corpus/templates/social.opt")
	if err != nil {
		t.Fatalf("read social.opt: %v", err)
	}
	var ot aom14.OperationalTemplate
	if err := canxml.Unmarshal(raw, &ot); err != nil {
		t.Fatalf("canxml.Unmarshal(social.opt) = %v", err)
	}
	b, err := canjson.Marshal(&ot)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var doc struct {
		Type       string `json:"_type"`
		Definition struct {
			Type            string `json:"_type"`
			TermDefinitions []struct {
				Items map[string]string `json:"items"`
			} `json:"term_definitions"`
		} `json:"definition"`
		Description struct {
			Details map[string]struct {
				Type string `json:"_type"`
			} `json:"details"`
		} `json:"description"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("reading the canonical JSON: %v", err)
	}
	if doc.Type != "OPERATIONAL_TEMPLATE" || doc.Definition.Type != "C_ARCHETYPE_ROOT" {
		t.Errorf("_type of the root and its definition = %q, %q; want OPERATIONAL_TEMPLATE, C_ARCHETYPE_ROOT", doc.Type, doc.Definition.Type)
	}
	if len(doc.Definition.TermDefinitions) == 0 || doc.Definition.TermDefinitions[0].Items["text"] == "" {
		t.Errorf("the first term's items are not a JSON object holding its text: %+v", doc.Definition.TermDefinitions)
	}
	if doc.Description.Details["nl"].Type != "RESOURCE_DESCRIPTION_ITEM" {
		t.Errorf("description.details = %+v; want a JSON object keyed by language holding a RESOURCE_DESCRIPTION_ITEM", doc.Description.Details)
	}
	var back aom14.OperationalTemplate
	if err := canjson.Unmarshal(b, &back, canjson.WithStrict()); err != nil {
		t.Fatalf("canjson.Unmarshal = %v", err)
	}
	if !reflect.DeepEqual(back, ot) {
		t.Error("social.opt's canonical JSON decodes to another value than the one written")
	}
}
