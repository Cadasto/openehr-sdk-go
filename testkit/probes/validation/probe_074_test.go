package validationprobes_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/validation"
)

// minimal synthetic OPTs rooted at non-COMPOSITION RM types.

const probe074OrganisationOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>org-root</value></template_id>
  <concept>org-root</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>ORGANISATION</rm_type_name>
    <node_id>at0000</node_id>
    <archetype_id><value>openEHR-DEMOGRAPHIC-ORGANISATION.organisation.v1</value></archetype_id>
  </definition>
</template>`

const probe074PersonIdentitiesOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>person-ident</value></template_id>
  <concept>person-ident</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>PERSON</rm_type_name>
    <node_id>at0000</node_id>
    <archetype_id><value>openEHR-DEMOGRAPHIC-PERSON.person.v1</value></archetype_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>identities</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique>
        <interval><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality>
    </attributes>
  </definition>
</template>`

const probe074FolderOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>folder-root</value></template_id>
  <concept>folder-root</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>FOLDER</rm_type_name>
    <node_id>at0000</node_id>
    <archetype_id><value>openEHR-EHR-FOLDER.generic.v1</value></archetype_id>
  </definition>
</template>`

const probe074EHRStatusOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>status-root</value></template_id>
  <concept>status-root</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>EHR_STATUS</rm_type_name>
    <node_id>at0000</node_id>
    <archetype_id><value>openEHR-EHR-EHR_STATUS.generic.v1</value></archetype_id>
  </definition>
</template>`

// PROBE-074 — template-driven validation extends beyond COMPOSITION to
// the demographic PARTY hierarchy and the EHR-IM roots. The issue-code
// multiset per (OPT, root) shape is stable across conformant
// implementations (REQ-110).
func TestProbe074NonCompositionValidate(t *testing.T) {
	var addr rm.Address
	raw, err := os.ReadFile(fixtures.CompositionJSON("Address.v2"))
	if err != nil {
		t.Fatal(err)
	}
	if err := canjson.Unmarshal(raw, &addr); err != nil {
		t.Fatalf("decode Address.v2: %v", err)
	}
	addrOPT, err := os.ReadFile(fixtures.TemplateOpt("Address.v2"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []probes.RootCase{
		{
			// Real ADDRESS OPT + conformant instance validates clean.
			Name:      "address_fixture_clean",
			OPT:       addrOPT,
			Root:      &addr,
			WantCodes: nil,
		},
		{
			// PERSON root under an ORGANISATION-rooted OPT → rm_type_mismatch.
			Name: "person_under_organisation_opt",
			OPT:  []byte(probe074OrganisationOPT),
			Root: &rm.Person{
				ArchetypeNodeID: "openEHR-DEMOGRAPHIC-ORGANISATION.organisation.v1",
				Name:            rm.DVText{Value: "P"},
				Identities:      []rm.PartyIdentity{{ArchetypeNodeID: "at0001"}},
			},
			WantCodes: []string{"rm_type_mismatch"},
		},
		{
			// PERSON with no identities under an OPT pinning identities >= 1.
			Name: "person_missing_identities",
			OPT:  []byte(probe074PersonIdentitiesOPT),
			Root: &rm.Person{
				ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PERSON.person.v1",
				Name:            rm.DVText{Value: "P"},
			},
			// existence lower >= 1 → required; cardinality interval
			// lower >= 1 with zero children → cardinality (both clauses
			// independent, mirroring PROBE-026 empty-events).
			WantCodes: []string{"required", "cardinality"},
		},
		{
			// FOLDER whose archetype id differs from the OPT pin.
			Name: "folder_archetype_id_mismatch",
			OPT:  []byte(probe074FolderOPT),
			Root: &rm.Folder{
				ArchetypeNodeID: "openEHR-EHR-FOLDER.other.v1",
				Name:            rm.DVText{Value: "root"},
			},
			WantCodes: []string{"archetype_id_mismatch"},
		},
		{
			// Minimal EHR_STATUS with all BMM-mandatory channels present.
			Name: "ehr_status_clean",
			OPT:  []byte(probe074EHRStatusOPT),
			Root: &rm.EHRStatus{
				ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
				Name:            rm.DVText{Value: "EHR Status"},
				Subject:         rm.PartySelf{},
				IsModifiable:    true,
				IsQueryable:     true,
			},
			WantCodes: nil,
		},
	}

	r, err := probes.Probe074NonCompositionValidate(cases)
	if err != nil {
		t.Fatalf("Probe074: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe074 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-074" {
		t.Errorf("Probe id = %q, want PROBE-074", r.Probe)
	}
}
