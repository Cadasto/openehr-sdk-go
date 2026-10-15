package validationprobes

// inputs.go: the roots the validation probes validate, built in Go, with the
// templates they are validated against. The probe tests and the validator's
// own comparison tests read the same values from here.

import (
	"fmt"
	"os"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Input is one root a validation probe validates, with the operational
// template it is validated against.
type Input struct {
	// Probe names the probe whose case this is.
	Probe string
	// Name is the probe case's name.
	Name string
	// OPT is the operational-template XML body the root is validated
	// against.
	OPT []byte
	// Root is the in-memory RM root under test.
	Root any
}

// Inputs returns every root the validation probes validate, each with the
// template its probe case compiles, in the order the probe tests run them.
// It reads vital_signs.opt and the Address.v2 template and document from the
// vendored corpus, and returns an error when one cannot be read or decoded.
func Inputs() ([]Input, error) {
	vitalSigns, err := os.ReadFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		return nil, fmt.Errorf("read vital_signs.opt: %w", err)
	}
	addrOPT, err := os.ReadFile(fixtures.TemplateOpt("Address.v2"))
	if err != nil {
		return nil, fmt.Errorf("read the Address.v2 template: %w", err)
	}
	addr, err := AddressFixture()
	if err != nil {
		return nil, err
	}
	const p025, p026, p074 = "PROBE-025", "PROBE-026", "PROBE-074"
	return []Input{
		{Probe: p025, Name: "valid_blood_pressure", OPT: vitalSigns, Root: ValidBloodPressureComposition()},
		{Probe: p025, Name: "systolic_2000_mmHg", OPT: vitalSigns, Root: SystolicOutOfRange()},
		{Probe: p026, Name: "missing_systolic_items", OPT: vitalSigns, Root: SystolicItemsAbsent()},
		{Probe: p026, Name: "unknown_archetype_id_under_content", OPT: vitalSigns, Root: UnknownArchetypeUnderContent()},
		{Probe: p026, Name: "systolic_units_psi", OPT: vitalSigns, Root: SystolicUnitsPSI()},
		{Probe: p026, Name: "category_code_999", OPT: vitalSigns, Root: CategoryCode999()},
		{Probe: p026, Name: "empty_events", OPT: vitalSigns, Root: EventsEmpty()},
		{Probe: p026, Name: "absent_events", OPT: vitalSigns, Root: EventsAbsent()},
		{Probe: p026, Name: "duplicate_systolic", OPT: vitalSigns, Root: SystolicTwice()},
		{Probe: p026, Name: "composer_party_identified", OPT: []byte(AlternativeMismatchOPT), Root: ComposerPartyIdentified()},
		{Probe: p026, Name: "composer_party_related_single_alt", OPT: []byte(RMTypeMismatchOPT), Root: ComposerPartyRelated()},
		{Probe: p074, Name: "address_fixture_clean", OPT: addrOPT, Root: addr},
		{Probe: p074, Name: "person_under_organisation_opt", OPT: []byte(OrganisationOPT), Root: PersonUnderOrganisationTemplate()},
		{Probe: p074, Name: "person_empty_identities", OPT: []byte(PersonIdentitiesOPT), Root: PersonEmptyIdentities()},
		{Probe: p074, Name: "person_absent_identities", OPT: []byte(PersonIdentitiesOPT), Root: PersonAbsentIdentities()},
		{Probe: p074, Name: "folder_archetype_id_mismatch", OPT: []byte(FolderOPT), Root: FolderOtherArchetype()},
		{Probe: p074, Name: "ehr_status_clean", OPT: []byte(EHRStatusOPT), Root: EHRStatusClean()},
	}, nil
}

// AddressFixture returns the vendored Address.v2 document, decoded.
func AddressFixture() (*rm.Address, error) {
	raw, err := os.ReadFile(fixtures.CompositionJSON("Address.v2"))
	if err != nil {
		return nil, fmt.Errorf("read the Address.v2 document: %w", err)
	}
	var addr rm.Address
	if err := canjson.Unmarshal(raw, &addr); err != nil {
		return nil, fmt.Errorf("decode the Address.v2 document: %w", err)
	}
	return &addr, nil
}

// archetyped returns the archetype_details the RM requires on an archetype
// root of archetypeID.
func archetyped(archetypeID string) *rm.Archetyped {
	return &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: archetypeID}, RMVersion: "1.1.0"}
}

// ValidBloodPressureComposition returns a composition that is valid against
// vital_signs.opt and against the RM. Each call builds a new one.
func ValidBloodPressureComposition() *rm.Composition {
	return &rm.Composition{
		ArchetypeNodeID:  "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:             rm.DVText{Value: "Encounter"},
		ArchetypeDetails: archetyped("openEHR-EHR-COMPOSITION.encounter.v1"),
		Category: rm.DVCodedText{
			DVText: rm.DVText{Value: "event"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "433",
			},
		},
		Composer: rm.PartySelf{},
		Language: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
			CodeString:    "en",
		},
		Territory: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
			CodeString:    "NL",
		},
		Content: []rm.ContentItem{
			&rm.Observation{
				ArchetypeNodeID:  "openEHR-EHR-OBSERVATION.blood_pressure.v1",
				Name:             rm.DVText{Value: "Blood pressure"},
				ArchetypeDetails: archetyped("openEHR-EHR-OBSERVATION.blood_pressure.v1"),
				Language: rm.CodePhrase{
					TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
					CodeString:    "en",
				},
				Encoding: rm.CodePhrase{
					TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"},
					CodeString:    "UTF-8",
				},
				Subject: rm.PartySelf{},
				Data: rm.History[rm.ItemStructure]{
					ArchetypeNodeID: "at0001",
					Name:            rm.DVText{Value: "history"},
					Origin:          rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
					Events: []rm.Event{
						&rm.PointEvent[rm.ItemStructure]{
							ArchetypeNodeID: "at0006",
							Name:            rm.DVText{Value: "any event"},
							Time:            rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
							Data: &rm.ItemList{
								ArchetypeNodeID: "at0003",
								Name:            rm.DVText{Value: "blood pressure"},
								Items: []rm.Element{{
									ArchetypeNodeID: "at0004",
									Name:            rm.DVText{Value: "Systolic"},
									Value: &rm.DVQuantity{
										Magnitude: rm.Real(120),
										Units:     "mm[Hg]",
									},
								}},
							},
							State: &rm.ItemList{
								ArchetypeNodeID: "at0007",
								Name:            rm.DVText{Value: "state"},
								Items: []rm.Element{{
									ArchetypeNodeID: "at0008",
									Name:            rm.DVText{Value: "Position"},
									Value:           &rm.DVText{Value: "Sitting"},
								}},
							},
						},
					},
				},
				Protocol: &rm.ItemTree{
					ArchetypeNodeID: "at0011",
					Name:            rm.DVText{Value: "protocol"},
					Items: []rm.Item{
						&rm.Cluster{
							ArchetypeNodeID: "openEHR-EHR-CLUSTER.device.v1",
							Name:            rm.DVText{Value: "Device"},
							Items: []rm.Item{&rm.Element{
								ArchetypeNodeID: "at0001",
								Name:            rm.DVText{Value: "Device name"},
								Value:           &rm.DVText{Value: "Cuff"},
							}},
						},
					},
				},
			},
		},
	}
}

// bloodPressureList returns the ITEM_LIST of comp's blood-pressure event, as
// [ValidBloodPressureComposition] builds it.
func bloodPressureList(comp *rm.Composition) *rm.ItemList {
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	return pe.Data.(*rm.ItemList)
}

// SystolicOutOfRange returns the valid blood-pressure composition with a
// systolic magnitude of 2000 mm[Hg], outside the template's range.
func SystolicOutOfRange() *rm.Composition {
	comp := ValidBloodPressureComposition()
	bloodPressureList(comp).Items[0].Value = &rm.DVQuantity{
		Magnitude: rm.Real(2000), // the template's range is [0,1000)
		Units:     "mm[Hg]",
	}
	return comp
}

// SystolicItemsAbsent returns the valid blood-pressure composition whose
// ITEM_LIST holds no items list at all.
func SystolicItemsAbsent() *rm.Composition {
	comp := ValidBloodPressureComposition()
	bloodPressureList(comp).Items = nil
	return comp
}

// UnknownArchetypeUnderContent returns the valid blood-pressure composition
// whose OBSERVATION carries an archetype id the template knows nowhere, so
// it fits no slot. The OBSERVATION is RM-valid apart from its id.
func UnknownArchetypeUnderContent() *rm.Composition {
	comp := ValidBloodPressureComposition()
	obs := comp.Content[0].(*rm.Observation)
	obs.ArchetypeNodeID = "openEHR-EHR-OBSERVATION.no_such_archetype.v1"
	obs.ArchetypeDetails = archetyped(obs.ArchetypeNodeID)
	return comp
}

// SystolicUnitsPSI returns the valid blood-pressure composition with a
// systolic pressure in psi, a unit the template does not list.
func SystolicUnitsPSI() *rm.Composition {
	comp := ValidBloodPressureComposition()
	bloodPressureList(comp).Items[0].Value = &rm.DVQuantity{
		Magnitude: rm.Real(120),
		Units:     "psi", // not in {mm[Hg]}
	}
	return comp
}

// CategoryCode999 returns the valid blood-pressure composition whose
// category is coded 999, outside the template's code list and outside the
// composition categories of the openEHR terminology.
func CategoryCode999() *rm.Composition {
	comp := ValidBloodPressureComposition()
	comp.Category.DefiningCode = rm.CodePhrase{
		TerminologyID: rm.TerminologyID{Value: "openehr"},
		CodeString:    "999",
	}
	return comp
}

// EventsEmpty returns the valid blood-pressure composition whose
// observation history holds a present and empty events list.
func EventsEmpty() *rm.Composition {
	comp := ValidBloodPressureComposition()
	comp.Content[0].(*rm.Observation).Data.Events = []rm.Event{}
	return comp
}

// EventsAbsent returns the valid blood-pressure composition whose
// observation history holds no events list at all.
func EventsAbsent() *rm.Composition {
	comp := ValidBloodPressureComposition()
	comp.Content[0].(*rm.Observation).Data.Events = nil
	return comp
}

// SystolicTwice returns the valid blood-pressure composition holding the
// systolic ELEMENT twice, above the template's occurrences of one.
func SystolicTwice() *rm.Composition {
	comp := ValidBloodPressureComposition()
	list := bloodPressureList(comp)
	systolic := list.Items[0]
	list.Items = []rm.Element{systolic, systolic}
	return comp
}

// AlternativeMismatchOPT is a COMPOSITION template whose composer is a
// PARTY_SELF or a PARTY_RELATED.
const AlternativeMismatchOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>alt-probe</value></template_id>
  <concept>alt-probe</concept>
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>composer</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_SELF</rm_type_name>
        <node_id />
      </children>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_RELATED</rm_type_name>
        <node_id />
      </children>
    </attributes>
  </definition>
</template>`

// RMTypeMismatchOPT is a COMPOSITION template whose composer is a
// PARTY_SELF.
const RMTypeMismatchOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>type-probe</value></template_id>
  <concept>type-probe</concept>
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>composer</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_SELF</rm_type_name>
        <node_id />
      </children>
    </attributes>
  </definition>
</template>`

// probeComposition returns a composition, valid against the RM, for the
// composer templates, with composer as its composer.
func probeComposition(composer rm.PartyProxy) *rm.Composition {
	return &rm.Composition{
		ArchetypeNodeID:  "at0000",
		Name:             rm.DVText{Value: "probe"},
		ArchetypeDetails: archetyped("openEHR-EHR-COMPOSITION.probe.v1"),
		Category: rm.DVCodedText{
			DVText: rm.DVText{Value: "event"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "433",
			},
		},
		Language: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
			CodeString:    "en",
		},
		Territory: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
			CodeString:    "NL",
		},
		Composer: composer,
	}
}

// ComposerPartyIdentified returns a composition whose composer is a
// PARTY_IDENTIFIED, which conforms to neither alternative of
// [AlternativeMismatchOPT].
func ComposerPartyIdentified() *rm.Composition {
	return probeComposition(&rm.PartyIdentified{Name: new("Dr. Jones")})
}

// ComposerPartyRelated returns a composition whose composer is an RM-valid
// PARTY_RELATED, which [RMTypeMismatchOPT] does not admit.
func ComposerPartyRelated() *rm.Composition {
	return probeComposition(&rm.PartyRelated{
		Name: new("Mother"),
		Relationship: rm.DVCodedText{
			Value:        "mother",
			DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "10"},
		},
	})
}

// OrganisationOPT is a minimal template rooted at ORGANISATION.
const OrganisationOPT = `<?xml version="1.0"?>
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

// PersonIdentitiesOPT is a template rooted at PERSON that requires at least
// one identity.
const PersonIdentitiesOPT = `<?xml version="1.0"?>
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

// FolderOPT is a minimal template rooted at FOLDER.
const FolderOPT = `<?xml version="1.0"?>
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

// EHRStatusOPT is a minimal template rooted at EHR_STATUS.
const EHRStatusOPT = `<?xml version="1.0"?>
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

// PersonUnderOrganisationTemplate returns a PERSON carrying the archetype
// id of [OrganisationOPT], which roots at ORGANISATION.
func PersonUnderOrganisationTemplate() *rm.Person {
	return &rm.Person{
		ArchetypeNodeID:  "openEHR-DEMOGRAPHIC-ORGANISATION.organisation.v1",
		Name:             rm.DVText{Value: "P"},
		ArchetypeDetails: archetyped("openEHR-DEMOGRAPHIC-ORGANISATION.organisation.v1"),
		Identities: []rm.PartyIdentity{{
			ArchetypeNodeID: "at0001",
			Name:            rm.DVText{Value: "Identity"},
			Details:         &rm.ItemTree{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "Details"}},
		}},
	}
}

// PersonEmptyIdentities returns a PERSON whose identities list is present
// and empty.
func PersonEmptyIdentities() *rm.Person {
	return &rm.Person{
		ArchetypeNodeID:  "openEHR-DEMOGRAPHIC-PERSON.person.v1",
		Name:             rm.DVText{Value: "P"},
		ArchetypeDetails: archetyped("openEHR-DEMOGRAPHIC-PERSON.person.v1"),
		Identities:       []rm.PartyIdentity{},
	}
}

// PersonAbsentIdentities returns a PERSON with no identities list at all.
func PersonAbsentIdentities() *rm.Person {
	return &rm.Person{
		ArchetypeNodeID:  "openEHR-DEMOGRAPHIC-PERSON.person.v1",
		Name:             rm.DVText{Value: "P"},
		ArchetypeDetails: archetyped("openEHR-DEMOGRAPHIC-PERSON.person.v1"),
	}
}

// FolderOtherArchetype returns a FOLDER whose archetype id differs from the
// one [FolderOPT] pins.
func FolderOtherArchetype() *rm.Folder {
	return &rm.Folder{
		ArchetypeNodeID: "openEHR-EHR-FOLDER.other.v1",
		Name:            rm.DVText{Value: "root"},
	}
}

// EHRStatusClean returns a minimal EHR_STATUS with every RM-mandatory
// attribute present, which [EHRStatusOPT] admits.
func EHRStatusClean() *rm.EHRStatus {
	return &rm.EHRStatus{
		ArchetypeNodeID:  "openEHR-EHR-EHR_STATUS.generic.v1",
		Name:             rm.DVText{Value: "EHR Status"},
		ArchetypeDetails: archetyped("openEHR-EHR-EHR_STATUS.generic.v1"),
		Subject:          rm.PartySelf{},
		IsModifiable:     true,
		IsQueryable:      true,
	}
}
