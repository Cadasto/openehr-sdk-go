package datamap

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

func loadTestkitOPT(t *testing.T, name string) *template.OperationalTemplate {
	t.Helper()
	b, err := os.ReadFile("../../testkit/cassettes/templates/" + name + ".opt")
	if err != nil {
		t.Fatal(err)
	}
	opt, err := template.ParseOPT(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("ParseOPT(%s): %v", name, err)
	}
	return opt
}

func loadTestkitPartyJSON(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile("../../testkit/cassettes/compositions/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var party map[string]any
	if err := json.Unmarshal(b, &party); err != nil {
		t.Fatal(err)
	}
	return party
}

func TestIsPartyTemplate(t *testing.T) {
	person := loadTestkitOPT(t, "TestPerson.v2")
	if !IsPartyTemplate(person) {
		t.Fatal("TestPerson.v2 should be a party template")
	}
	comp := loadOPT(t, "development-1")
	if IsPartyTemplate(comp) {
		t.Fatal("development-1 should be a composition template")
	}
}

func TestPartySchema(t *testing.T) {
	opt := loadTestkitOPT(t, "TestPerson.v2")
	schema := Schema(opt)
	if schema["title"] != "TestPerson.v2 DMv2 party datamap" {
		t.Fatalf("schema title = %v", schema["title"])
	}
	props, _ := schema["properties"].(map[string]any)
	for _, key := range []string{"identities", "details", "contacts"} {
		if props[key] == nil {
			t.Errorf("party schema missing %q", key)
		}
	}
}

func TestFromPartyToPartyRoundTrip(t *testing.T) {
	opt := loadTestkitOPT(t, "TestPerson.v2")
	party := loadTestkitPartyJSON(t, "TestPerson.v2")

	dm, err := FromParty(opt, party)
	if err != nil {
		t.Fatalf("FromParty: %v", err)
	}
	if dm["name"] != "Persoon" {
		t.Errorf("name = %v, want Persoon", dm["name"])
	}
	idents, ok := dm["identities"].(map[string]any)
	if !ok || len(idents) == 0 {
		t.Fatal("expected identities in datamap")
	}

	round, err := ToParty(opt, dm)
	if err != nil {
		t.Fatalf("ToParty: %v", err)
	}
	if round["_type"] != "PERSON" {
		t.Errorf("_type = %v, want PERSON", round["_type"])
	}
	roundIDs, ok := round["identities"].([]any)
	if !ok || len(roundIDs) == 0 {
		t.Fatal("ToParty: expected identities")
	}
	origIDs := party["identities"].([]any)
	if len(roundIDs) != len(origIDs) {
		t.Fatalf("identities count = %d, want %d", len(roundIDs), len(origIDs))
	}

	// Spot-check a decoded identity field survives the round-trip.
	id0 := roundIDs[0].(map[string]any)
	details := id0["details"].(map[string]any)
	items := details["items"].([]any)
	found := false
	for _, it := range items {
		el, ok := it.(map[string]any)
		if !ok || el["archetype_node_id"] != "at0002" {
			continue
		}
		val := el["value"].(map[string]any)
		if val["value"] == "waijvbts yieja dlnaekb crdsl wer" {
			found = true
		}
	}
	if !found {
		t.Error("Voornaam value lost in round-trip")
	}
}

func TestToCompositionRejectsPartyTemplate(t *testing.T) {
	opt := loadTestkitOPT(t, "TestPerson.v2")
	_, err := ToComposition(opt, map[string]any{})
	if err == nil {
		t.Fatal("expected error for party template on ToComposition")
	}
}
