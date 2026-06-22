package validation_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-110 — the template-driven walker validates archetypeable RM roots
// beyond COMPOSITION: the demographic PARTY hierarchy and the EHR-IM
// roots FOLDER / EHR_STATUS, through the same compiled-OPT machinery.

// mustDecodeJSON decodes a vendored JSON fixture into the given RM root
// via the generated RM unmarshaller (stdlib json.Unmarshal == canjson
// for these types). The validator takes an in-memory root, so decode is
// the test's concern, not the validator's.
func mustDecodeJSON(t *testing.T, path string, dst any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func mustCompileOPT(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOpt(name))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", name, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", name, err)
	}
	return c
}

func mustCompileInline(t *testing.T, body string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

// REQ-110 — a real demographic OPT (PERSON root, 174 compiled nodes) +
// its instance walk end-to-end through ValidateDemographic. The walk
// descends the full PARTY hierarchy (identities / contacts → addresses /
// details → cluster trees); residual issues are confined to two known
// categories, proving the demographic structure itself validates clean:
//   - DV_INTERVAL<DV_DATE> rm_type_mismatch — SDK-GAP-11: DV_INTERVAL
//     over DV_ORDERED is not yet type-matched by the walker (a DataValue
//     limitation, not demographic-specific);
//   - a genuine `relationships` cardinality finding — the instance has
//     no relationships while the template pins >= 1 (the validator
//     correctly catching a real violation).
func TestValidateDemographic_PersonFixture(t *testing.T) {
	c := mustCompileOPT(t, "TestPerson.v2")
	var person rm.Person
	mustDecodeJSON(t, fixtures.CompositionJSON("TestPerson.v2"), &person)

	r := validation.ValidateDemographic(&person, c)

	sawContacts, sawDetailsCluster, sawCardinality := false, false, false
	for _, iss := range r.Issues {
		switch iss.Code {
		case "rm_type_mismatch":
			if !strings.Contains(iss.Detail, "DV_INTERVAL") {
				t.Errorf("unexpected rm_type_mismatch at %s — %s", iss.Path, iss.Detail)
			}
		case "cardinality":
			sawCardinality = true // genuine instance non-conformance (relationships >= 1)
		default:
			t.Errorf("unexpected %s issue at %s — %s", iss.Code, iss.Path, iss.Detail)
		}
		if strings.Contains(iss.Path, "/contacts[") {
			sawContacts = true
		}
		if strings.Contains(iss.Path, "/details/items[") {
			sawDetailsCluster = true
		}
	}
	// The deep paths in the residual issues prove the walker descended
	// PERSON → contacts → addresses and PERSON → details → cluster trees.
	if !sawContacts {
		t.Error("walk did not descend into PERSON.contacts → addresses")
	}
	if !sawDetailsCluster {
		t.Error("walk did not descend into PERSON.details cluster trees")
	}
	// Enforce the "genuine relationships finding" the comment claims, so a
	// regression that drops it does not pass silently.
	if !sawCardinality {
		t.Error("expected the genuine /relationships cardinality finding")
	}
}

// REQ-110 — a real ADDRESS OPT + instance validate clean through the
// generic Validate (ADDRESS is not a PARTY, so no typed wrapper).
func TestValidate_AddressFixture(t *testing.T) {
	c := mustCompileOPT(t, "Address.v2")
	var addr rm.Address
	mustDecodeJSON(t, fixtures.CompositionJSON("Address.v2"), &addr)

	r := validation.Validate(&addr, c)
	for _, iss := range r.Issues {
		t.Logf("issue: %s %s — %s", iss.Path, iss.Code, iss.Detail)
	}
	if !r.OK {
		t.Errorf("Validate(Address.v2) not OK; %d issue(s)", len(r.Issues))
	}
}

// REQ-110 — a PERSON validated against an ORGANISATION-rooted OPT
// surfaces rm_type_mismatch at the root, not a silent pass.
func TestValidateDemographic_RootTypeMismatch(t *testing.T) {
	const body = `<?xml version="1.0"?>
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
	c := mustCompileInline(t, body)
	person := &rm.Person{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PERSON.person.v1", Name: rm.DVText{Value: "P"}}
	r := validation.ValidateDemographic(person, c)
	if r.OK {
		t.Fatal("expected rm_type_mismatch for PERSON under ORGANISATION OPT, got OK")
	}
	if !containsCode(r.Issues, "rm_type_mismatch") {
		t.Errorf("expected rm_type_mismatch, got %+v", r.Issues)
	}
}

// REQ-110 — an OPT pinning PERSON.identities existence >= 1 flags a
// person with no identities as `required`.
func TestValidateDemographic_RequiredIdentities(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>person-ident</value></template_id>
  <concept>person-ident</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>PERSON</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>identities</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique>
        <interval><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality>
    </attributes>
  </definition>
</template>`
	c := mustCompileInline(t, body)
	person := &rm.Person{ArchetypeNodeID: "at0000", Name: rm.DVText{Value: "P"}}
	r := validation.ValidateDemographic(person, c)
	if !containsIssue(r.Issues, "/identities", "required") {
		t.Errorf("expected required at /identities, got %+v", r.Issues)
	}
}

// REQ-110 — FOLDER validates against an OPT (synthetic root) and the
// walker descends sub-folders.
func TestValidateFolder_Synthetic(t *testing.T) {
	const body = `<?xml version="1.0"?>
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
	c := mustCompileInline(t, body)
	folder := &rm.Folder{
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
		Name:            rm.DVText{Value: "root"},
	}
	r := validation.ValidateFolder(folder, c)
	if !r.OK {
		t.Errorf("ValidateFolder(synthetic) not OK: %+v", r.Issues)
	}
	// Wrong archetype id at the root surfaces archetype_id_mismatch.
	bad := &rm.Folder{ArchetypeNodeID: "openEHR-EHR-FOLDER.other.v1", Name: rm.DVText{Value: "root"}}
	if rb := validation.ValidateFolder(bad, c); !containsCode(rb.Issues, "archetype_id_mismatch") {
		t.Errorf("expected archetype_id_mismatch for wrong FOLDER archetype id, got %+v", rb.Issues)
	}
}

// REQ-110 — EHR_STATUS validates against an OPT root.
func TestValidateEHRStatus_Synthetic(t *testing.T) {
	const body = `<?xml version="1.0"?>
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
	c := mustCompileInline(t, body)
	status := &rm.EHRStatus{
		ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		Name:            rm.DVText{Value: "EHR Status"},
		Subject:         rm.PartySelf{},
		IsModifiable:    true,
		IsQueryable:     true,
	}
	if r := validation.ValidateEHRStatus(status, c); !r.OK {
		t.Errorf("ValidateEHRStatus(synthetic) not OK: %+v", r.Issues)
	}
}

// REQ-110 — nil-root guards across the typed wrappers and generic entry.
func TestValidate_NilGuards(t *testing.T) {
	c := mustCompileInline(t, `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>g</value></template_id><concept>g</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition><rm_type_name>PERSON</rm_type_name><node_id>at0000</node_id></definition>
</template>`)

	if r := validation.ValidateDemographic(nil, c); r.OK || !containsCode(r.Issues, "nil_party") {
		t.Errorf("ValidateDemographic(nil) want nil_party, got %+v", r.Issues)
	}
	if r := validation.ValidateFolder(nil, c); r.OK || !containsCode(r.Issues, "nil_folder") {
		t.Errorf("ValidateFolder(nil) want nil_folder, got %+v", r.Issues)
	}
	if r := validation.ValidateEHRStatus(nil, c); r.OK || !containsCode(r.Issues, "nil_ehr_status") {
		t.Errorf("ValidateEHRStatus(nil) want nil_ehr_status, got %+v", r.Issues)
	}
	if r := validation.Validate(nil, c); r.OK || !containsCode(r.Issues, "nil_root") {
		t.Errorf("Validate(nil) want nil_root, got %+v", r.Issues)
	}
	// Typed-nil PARTY behind the interface must not panic and must honour
	// the wrapper's advertised nil_party contract (not the generic nil_root).
	var typedNilPerson *rm.Person
	if r := validation.ValidateDemographic(typedNilPerson, c); r.OK || !containsCode(r.Issues, "nil_party") {
		t.Errorf("ValidateDemographic(typed-nil *Person) want nil_party, got %+v", r.Issues)
	}
	// Typed-nil concrete behind the generic Validate(any) must not panic.
	var typedNilFolder *rm.Folder
	if r := validation.Validate(typedNilFolder, c); r.OK || !containsCode(r.Issues, "nil_root") {
		t.Errorf("Validate(typed-nil *Folder) want nil_root, got %+v", r.Issues)
	}
}

// REQ-110 — an ORGANISATION instance validates clean against its own
// ORGANISATION-rooted OPT (the positive counterpart to the
// PERSON-under-ORGANISATION rm_type_mismatch), proving the ACTOR
// subtype routing accepts a matching root.
func TestValidateDemographic_OrganisationRootClean(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>org-clean</value></template_id>
  <concept>org-clean</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>ORGANISATION</rm_type_name>
    <node_id>at0000</node_id>
    <archetype_id><value>openEHR-DEMOGRAPHIC-ORGANISATION.organisation.v1</value></archetype_id>
  </definition>
</template>`
	c := mustCompileInline(t, body)
	org := &rm.Organisation{
		ArchetypeNodeID: "openEHR-DEMOGRAPHIC-ORGANISATION.organisation.v1",
		Name:            rm.DVText{Value: "Org"},
		Identities:      []rm.PartyIdentity{{ArchetypeNodeID: "at0001"}},
	}
	if r := validation.ValidateDemographic(org, c); !r.OK {
		t.Errorf("ValidateDemographic(ORGANISATION) not OK: %+v", r.Issues)
	}
}
