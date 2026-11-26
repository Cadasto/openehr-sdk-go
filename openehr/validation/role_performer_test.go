package validation_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// genericRoleJSON is a ROLE with no performer: a generic role, such as one
// that every nurse on a ward plays.
const genericRoleJSON = `{"_type":"ROLE","archetype_node_id":"openEHR-DEMOGRAPHIC-ROLE.generic.v1",` +
	`"name":{"_type":"DV_TEXT","value":"Registered nurse, cardiology ward"},` +
	`"identities":[{"_type":"PARTY_IDENTITY","archetype_node_id":"at0001",` +
	`"name":{"_type":"DV_TEXT","value":"role name"},` +
	`"details":{"_type":"ITEM_TREE","archetype_node_id":"at0002","name":{"_type":"DV_TEXT","value":"details"}}}]}`

// performerIssues returns the issues of r at or below /performer, as
// identityKeys writes them.
func performerIssues(r validation.Result) []string {
	var out []validation.Issue
	for _, i := range r.Issues {
		if i.Path == "/performer" || strings.HasPrefix(i.Path, "/performer/") {
			out = append(out, i)
		}
	}
	return identityKeys(out)
}

// TestRole_REQ041_REQ112_PerformerIsOptional pins REQ-041 § SDK-authored root
// schemas, Observable, on ROLE: the RM root declares performer optional, so a
// ROLE without one, a generic role, decodes from canonical JSON and XML with
// no member missing, has no issue at /performer on the REQ-112 floor, and
// encodes with no performer, while a ROLE with a performer keeps it through
// both codecs. On the pinned RM schema, where performer is mandatory, each
// decode reports /performer missing and the floor reports it required, so
// this test fails if the root's ROLE goes.
func TestRole_REQ041_REQ112_PerformerIsOptional(t *testing.T) {
	t.Parallel()
	var generic rm.Role
	var missing []string
	if err := canjson.Unmarshal([]byte(genericRoleJSON), &generic,
		canjson.WithMissing(func(p canjson.Pointer) { missing = append(missing, p.String()) })); err != nil {
		t.Fatalf("canjson.Unmarshal(generic ROLE): %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("canjson.Unmarshal(generic ROLE) missing members = %q, want none", missing)
	}
	if generic.Performer != nil {
		t.Errorf("canjson.Unmarshal(generic ROLE) performer = %+v, want nil", generic.Performer)
	}
	if got := performerIssues(validation.ValidateRMDemographic(&generic)); len(got) != 0 {
		t.Errorf("ValidateRMDemographic(generic ROLE) issues at /performer = %q, want none", got)
	}

	js, err := canjson.Marshal(&generic)
	if err != nil {
		t.Fatalf("canjson.Marshal(generic ROLE): %v", err)
	}
	if bytes.Contains(js, []byte(`"performer"`)) {
		t.Errorf("canjson.Marshal(generic ROLE) = %s, want no performer member", js)
	}
	doc, err := canxml.Marshal(&generic)
	if err != nil {
		t.Fatalf("canxml.Marshal(generic ROLE): %v", err)
	}
	if bytes.Contains(doc, []byte("<performer")) {
		t.Errorf("canxml.Marshal(generic ROLE) = %s, want no performer element", doc)
	}
	var fromXML rm.Role
	missing = nil
	if err := canxml.Unmarshal(doc, &fromXML,
		canxml.WithMissing(func(p canxml.Pointer) { missing = append(missing, p.String()) })); err != nil {
		t.Fatalf("canxml.Unmarshal(generic ROLE): %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("canxml.Unmarshal(generic ROLE) missing members = %q, want none", missing)
	}
	if fromXML.Performer != nil {
		t.Errorf("canxml.Unmarshal(generic ROLE) performer = %+v, want nil", fromXML.Performer)
	}

	personal := generic
	personal.Performer = &rm.PartyRef{
		ID: &rm.HierObjectID{Value: "9fcf1d0e-0000-4000-8000-000000000001"}, Namespace: "local", Type: "PERSON",
	}
	if got := performerIssues(validation.ValidateRMDemographic(&personal)); len(got) != 0 {
		t.Errorf("ValidateRMDemographic(personal ROLE) issues at /performer = %q, want none", got)
	}
	js, err = canjson.Marshal(&personal)
	if err != nil {
		t.Fatalf("canjson.Marshal(personal ROLE): %v", err)
	}
	var back rm.Role
	if err := canjson.Unmarshal(js, &back); err != nil {
		t.Fatalf("canjson.Unmarshal(%s): %v", js, err)
	}
	if back.Performer == nil || back.Performer.Type != "PERSON" || back.Performer.Namespace != "local" {
		t.Errorf("canjson round trip of a personal ROLE: performer = %+v, want local::PERSON", back.Performer)
	}
	doc, err = canxml.Marshal(&personal)
	if err != nil {
		t.Fatalf("canxml.Marshal(personal ROLE): %v", err)
	}
	back = rm.Role{}
	if err := canxml.Unmarshal(doc, &back); err != nil {
		t.Fatalf("canxml.Unmarshal(%s): %v", doc, err)
	}
	if back.Performer == nil || back.Performer.Type != "PERSON" || back.Performer.Namespace != "local" {
		t.Errorf("canxml round trip of a personal ROLE: performer = %+v, want local::PERSON", back.Performer)
	}
}
