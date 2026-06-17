package datamap

import "testing"

// TestEncodeIdentifierDVIdentifier covers the DV_IDENTIFIER value encoder:
// a bare id string and an object form, with empty optional fields omitted
// (Cadasto rejects empty issuer/assigner/type) and an empty id dropped.
func TestEncodeIdentifierDVIdentifier(t *testing.T) {
	if got := encodeIdentifier("BSN123"); got == nil || got["_type"] != "DV_IDENTIFIER" || got["id"] != "BSN123" {
		t.Fatalf("string id: got %v", got)
	}
	if got := encodeIdentifier(""); got != nil {
		t.Fatalf("empty id should drop: got %v", got)
	}
	obj := encodeIdentifier(map[string]any{"id": "AGB42", "issuer": "VEKTIS", "type": ""})
	if obj["id"] != "AGB42" || obj["issuer"] != "VEKTIS" {
		t.Fatalf("object id: got %v", obj)
	}
	if _, has := obj["type"]; has {
		t.Fatalf("empty type must be omitted: got %v", obj)
	}
}

// TestToPartyCodedIdentityName proves the PARTY_IDENTITY name is emitted as a
// constrained DV_CODED_TEXT when the OPT closes the name to a code list — even
// without an explicit _code in the payload (Cadasto rejects a DV_TEXT there).
func TestToPartyCodedIdentityName(t *testing.T) {
	opt := loadTestkitOPT(t, "TestPerson.v2")
	payload := map[string]any{
		"identities": map[string]any{
			"openEHR-DEMOGRAPHIC-PARTY_IDENTITY.person_name.v2|Naamgegevens": map[string]any{
				"at0003|Achternaam": []any{"Persoon"},
			},
		},
	}
	out, err := ToParty(opt, payload)
	if err != nil {
		t.Fatalf("ToParty: %v", err)
	}
	ids, _ := out["identities"].([]any)
	if len(ids) == 0 {
		t.Fatal("no identities encoded")
	}
	id0, _ := ids[0].(map[string]any)
	name, _ := id0["name"].(map[string]any)
	if name["_type"] != "DV_CODED_TEXT" {
		t.Fatalf("identity name _type = %v, want DV_CODED_TEXT", name["_type"])
	}
	if _, ok := name["defining_code"]; !ok {
		t.Fatalf("DV_CODED_TEXT name missing defining_code: %v", name)
	}
}
