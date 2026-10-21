package instance_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// The OPTs below are small templates for the REQ-107 rules on the party
// proxies an ENTRY holds, on a PARTY_RELATIONSHIP and on a CONTACT: what
// the template names there comes out, a reference among it takes the parts
// RM defaults and Identity give, and every pin is kept.

// identifierChild is a DV_IDENTIFIER child whose id the template pins.
func identifierChild(id string) string {
	return guardChild("C_COMPLEX_OBJECT", "DV_IDENTIFIER", "", guardOccurrences11, "", pinnedString("id", id))
}

// identifierIDs returns the id of each identifier, in order.
func identifierIDs(identifiers []rm.DVIdentifier) []string {
	ids := make([]string, 0, len(identifiers))
	for _, id := range identifiers {
		ids = append(ids, id.ID)
	}
	return ids
}

// TestREQ107_PartyIdentifiedProviderTakesItsParts is the REQ-107 check
// that an ENTRY's provider the template names as a PARTY_IDENTIFIED with
// identifiers and an external_ref comes out with both, at every policy,
// either value fill and either compile mode, and passes both validators.
// The external_ref keeps the namespace the template pins and takes the
// type PERSON and an id drawn from UIDSource, after the OBSERVATION's own
// uid. With an identifier and an external_ref the party meets
// Basic_validity, so it takes no name.
func TestREQ107_PartyIdentifiedProviderTakesItsParts(t *testing.T) {
	opt := guardRootOPT("OBSERVATION", guardObservationID,
		requiredRef("provider", "PARTY_IDENTIFIED",
			guardMultiple("identifiers", guardExistence11, identifierChild("123456")),
			requiredRef("external_ref", "PARTY_REF", pinnedString("namespace", "demographic"))))
	generateEvery(t, opt, func(t *testing.T, call string, out any) {
		obs, ok := out.(*rm.Observation)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Observation", call, out)
		}
		p, ok := obs.Provider.(*rm.PartyIdentified)
		if !ok {
			t.Fatalf("%s: provider is %T, want *rm.PartyIdentified", call, obs.Provider)
		}
		if got := identifierIDs(p.Identifiers); !slices.Equal(got, []string{"123456"}) {
			t.Errorf("%s: provider identifiers = %v, want [123456]", call, got)
		}
		if got, want := refString(p.ExternalRef), "PARTY_REF demographic::PERSON id HIER_OBJECT_ID uid-0002"; got != want {
			t.Errorf("%s: provider external_ref = %s, want %s", call, got, want)
		}
		if p.Name != nil {
			t.Errorf("%s: provider name = %q, want none", call, *p.Name)
		}
	})
}

// TestREQ107_PartyRelatedSubjectTakesItsParts is the REQ-107 check that an
// ENTRY's subject the template names as a PARTY_RELATED with a name,
// identifiers and an external_ref comes out with all three, at every
// policy, either value fill and either compile mode, and passes both
// validators: the name the template pins, the identifier, and an
// external_ref with the namespace local, the type PERSON and an id drawn
// from UIDSource, after the OBSERVATION's own uid. Its relationship takes
// the RM default openehr 253|unknown|.
func TestREQ107_PartyRelatedSubjectTakesItsParts(t *testing.T) {
	opt := guardRootOPT("OBSERVATION", guardObservationID,
		requiredRef("subject", "PARTY_RELATED",
			pinnedString("name", "Jane"),
			guardMultiple("identifiers", guardExistence11, identifierChild("998877")),
			requiredRef("external_ref", "PARTY_REF")))
	unknown := rm.CodePhrase{CodeString: "253", TerminologyID: rm.TerminologyID{Value: "openehr"}}
	generateEvery(t, opt, func(t *testing.T, call string, out any) {
		obs, ok := out.(*rm.Observation)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Observation", call, out)
		}
		p, ok := obs.Subject.(*rm.PartyRelated)
		if !ok {
			t.Fatalf("%s: subject is %T, want *rm.PartyRelated", call, obs.Subject)
		}
		if p.Name == nil || *p.Name != "Jane" {
			t.Errorf("%s: subject name = %v, want Jane", call, p.Name)
		}
		if got := identifierIDs(p.Identifiers); !slices.Equal(got, []string{"998877"}) {
			t.Errorf("%s: subject identifiers = %v, want [998877]", call, got)
		}
		if got, want := refString(p.ExternalRef), "PARTY_REF local::PERSON id HIER_OBJECT_ID uid-0002"; got != want {
			t.Errorf("%s: subject external_ref = %s, want %s", call, got, want)
		}
		if p.Relationship.DefiningCode != unknown || p.Relationship.Value != "unknown" {
			t.Errorf("%s: subject relationship = %q %+v, want unknown %+v", call, p.Relationship.Value, p.Relationship.DefiningCode, unknown)
		}
	})
}

// TestREQ107_PartySelfSubjectTakesItsExternalRef is the REQ-107 check that
// an ENTRY's subject the template names as a PARTY_SELF with an
// external_ref comes out with it, at every policy, either value fill and
// either compile mode, and passes both validators: the type the template
// pins, the namespace local and an id drawn from UIDSource, after the
// OBSERVATION's own uid.
func TestREQ107_PartySelfSubjectTakesItsExternalRef(t *testing.T) {
	opt := guardRootOPT("OBSERVATION", guardObservationID,
		requiredRef("subject", "PARTY_SELF", requiredRef("external_ref", "PARTY_REF", pinnedString("type", "GROUP"))))
	generateEvery(t, opt, func(t *testing.T, call string, out any) {
		obs, ok := out.(*rm.Observation)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Observation", call, out)
		}
		p, ok := obs.Subject.(*rm.PartySelf)
		if !ok {
			t.Fatalf("%s: subject is %T, want *rm.PartySelf", call, obs.Subject)
		}
		if got, want := refString(p.ExternalRef), "PARTY_REF local::GROUP id HIER_OBJECT_ID uid-0002"; got != want {
			t.Errorf("%s: subject external_ref = %s, want %s", call, got, want)
		}
	})
}

// TestREQ107_PartyRelationshipKeepsItsPins is the REQ-107 check that a
// PARTY_RELATIONSHIP root whose template names its details and
// time_validity, and pins parts of its source and target, comes out with
// every part, at every policy, either value fill and either compile mode,
// and passes both validators. Only the parts the template leaves empty take
// a default: the source keeps its pinned namespace and type and takes an id
// drawn from UIDSource; the target keeps its pinned id and takes the
// namespace local and the type PERSON. The relationship's own uid is drawn
// after the source id.
func TestREQ107_PartyRelationshipKeepsItsPins(t *testing.T) {
	const targetID = "6e4b1c2a-0000-4000-8000-00000000abcd"
	opt := guardRootOPT("PARTY_RELATIONSHIP", "openEHR-DEMOGRAPHIC-PARTY_RELATIONSHIP.example.v1",
		guardSingle("details", guardExistence11, guardChild("C_COMPLEX_OBJECT", "ITEM_TREE", "at0001", guardOccurrences11, "")),
		guardDateInterval(),
		requiredRef("source", "PARTY_REF", pinnedString("namespace", "demographic"), pinnedString("type", "ORGANISATION")),
		requiredRef("target", "PARTY_REF", requiredRef("id", "HIER_OBJECT_ID", pinnedString("value", targetID))))
	generateEvery(t, opt, func(t *testing.T, call string, out any) {
		rel, ok := out.(*rm.PartyRelationship)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.PartyRelationship", call, out)
		}
		if got, want := refString(&rel.Source), "PARTY_REF demographic::ORGANISATION id HIER_OBJECT_ID uid-0001"; got != want {
			t.Errorf("%s: source = %s, want %s", call, got, want)
		}
		if got, want := refString(&rel.Target), "PARTY_REF local::PERSON id HIER_OBJECT_ID "+targetID; got != want {
			t.Errorf("%s: target = %s, want %s", call, got, want)
		}
		if uid := hierIDText(rel.GetUID()); uid != "uid-0002" {
			t.Errorf("%s: uid = %q, want uid-0002", call, uid)
		}
		if tree, ok := rel.Details.(*rm.ItemTree); !ok || tree.ArchetypeNodeID != "at0001" {
			t.Errorf("%s: details = %#v, want the ITEM_TREE at0001", call, rel.Details)
		}
		if rel.TimeValidity == nil {
			t.Errorf("%s: time_validity is unset, want the interval the template requires", call)
		}
	})
}

// TestREQ107_ContactTakesItsNameAndTimeValidity is the REQ-107 check that a
// CONTACT whose template names its name and time_validity comes out with
// both, at every policy, either value fill and either compile mode, and
// passes both validators: the name is the text the template pins.
func TestREQ107_ContactTakesItsNameAndTimeValidity(t *testing.T) {
	opt := guardRootOPT("PERSON", "openEHR-DEMOGRAPHIC-PERSON.example.v1",
		guardMultiple("contacts", guardExistence11, guardChild("C_COMPLEX_OBJECT", "CONTACT", "at0001", guardOccurrences11, "",
			requiredRef("name", "DV_TEXT", pinnedString("value", "home")),
			guardDateInterval())))
	generateEvery(t, opt, func(t *testing.T, call string, out any) {
		person, ok := out.(*rm.Person)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Person", call, out)
		}
		if len(person.Contacts) != 1 {
			t.Fatalf("%s: the PERSON has %d contacts, want 1", call, len(person.Contacts))
		}
		contact := person.Contacts[0]
		if contact.Name == nil || contact.Name.GetValue() != "home" {
			t.Errorf("%s: contact name = %#v, want the text home", call, contact.Name)
		}
		if contact.TimeValidity == nil {
			t.Errorf("%s: contact time_validity is unset, want the interval the template requires", call)
		}
	})
}
