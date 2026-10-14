package validation_test

// rmfloor_presence_readings_test.go: REQ-171 Structural access and REQ-112 —
// the presence readings of the generated attribute access, as the walks meet
// them. A DV_TEXT at an optional attribute is entered whatever it holds, so
// what it lacks is reported below it; an object reference held by value with
// a part missing is entered, so the reference entry reports the part at its
// own path; a value held by value whose only content is a nil pointer is
// absent; the payload of a version built in Go is walked; and the template
// walk binds a CODE_PHRASE's terminology_id to the TERMINOLOGY_ID node that
// constrains it.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestValidateRM_REQ171_OptionalTextEntered pins REQ-171 Structural access on
// the floor: at an optional attribute (ELEMENT null_reason, AUDIT_DETAILS
// description) a DV_TEXT whose value is empty is present, so the walk enters
// it and reports its absent value, an empty mappings and a mapping's bad
// match below it, by the REQ-112 DV_TEXT and TERM_MAPPING entries. At a
// mandatory attribute (ELEMENT name) the same DV_TEXT is absent, reported
// once as required at the attribute.
func TestValidateRM_REQ171_OptionalTextEntered(t *testing.T) {
	noInformation := rm.DVCodedText{Value: "no information", DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "271"}}
	element := func(name, nullReason rm.DVTextLike) *rm.Element {
		return &rm.Element{ArchetypeNodeID: "at0001", Name: name, NullFlavour: &noInformation, NullReason: nullReason}
	}
	badMatch := []rm.TermMapping{{Match: "x", Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "1"}}}
	audit := &rm.AuditDetails{
		SystemID:      "system",
		TimeCommitted: rm.DVDateTime{Value: "2026-10-14T10:00:00Z"},
		ChangeType:    rm.DVCodedText{Value: "creation", DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "249"}},
		Committer:     &rm.PartySelf{},
		Description:   &rm.DVText{},
	}
	for _, tc := range []struct {
		name string
		root any
		want []string // code path layer rule
	}{
		{
			name: "ELEMENT null_reason with an empty value and mappings",
			root: element(&rm.DVText{Value: "e"}, &rm.DVText{Mappings: []rm.TermMapping{}}),
			want: []string{"mappings_valid /null_reason/mappings rm DV_TEXT.Mappings_valid", "required /null_reason/value rm mandatory"},
		},
		{
			name: "ELEMENT null_reason with an empty value and a bad match",
			root: element(&rm.DVText{Value: "e"}, &rm.DVText{Mappings: badMatch}),
			want: []string{"required /null_reason/value rm mandatory", "term_mapping_match /null_reason/mappings[1]/match rm TERM_MAPPING.Match_valid"},
		},
		{
			name: "AUDIT_DETAILS description with an empty value",
			root: audit,
			want: []string{"required /description/value rm mandatory"},
		},
		{
			name: "ELEMENT name, mandatory, with an empty value and a bad match",
			root: element(&rm.DVText{Mappings: badMatch}, nil),
			want: []string{"required /name rm mandatory"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := identityKeys(validation.ValidateRM(tc.root).Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) issues (code path layer rule) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestValidateRM_REQ112_ReferenceHeldByValueReportsItsMissingPart pins the
// OBJECT_REF entry of REQ-112 on a reference held by value (ROLE performer,
// PARTY_RELATIONSHIP source): a reference with one part missing is present
// (REQ-171 Structural access), so the walk enters it and the part is reported
// once, as rm_invariant, rule mandatory, at its own path, and nothing is
// reported at the attribute; the zero reference is absent and reported once
// as required at the attribute.
func TestValidateRM_REQ112_ReferenceHeldByValueReportsItsMissingPart(t *testing.T) {
	id := &rm.HierObjectID{Value: "9fcf1d0e-0000-4000-8000-000000000001"}
	full := rm.PartyRef{ID: id, Namespace: "local", Type: "PERSON"}
	refs := []struct {
		missing string
		ref     rm.PartyRef
	}{
		{missing: "id", ref: rm.PartyRef{Namespace: "local", Type: "PERSON"}},
		{missing: "namespace", ref: rm.PartyRef{ID: id, Type: "PERSON"}},
		{missing: "type", ref: rm.PartyRef{ID: id, Namespace: "local"}},
	}
	holders := []struct {
		name    string
		at      string // the reference's path
		holding func(rm.PartyRef) any
	}{
		{name: "ROLE performer", at: "/performer", holding: func(ref rm.PartyRef) any {
			return &rm.Role{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-ROLE.role.v1", Name: rm.DVText{Value: "GP"}, Performer: ref}
		}},
		{name: "PARTY_RELATIONSHIP source", at: "/source", holding: func(ref rm.PartyRef) any {
			return &rm.PartyRelationship{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PARTY_RELATIONSHIP.relation.v1", Name: rm.DVText{Value: "relation"}, Source: ref, Target: full}
		}},
	}
	// at returns the issues at or below path, as identityKeys writes them.
	at := func(issues []validation.Issue, path string) []string {
		var out []validation.Issue
		for _, i := range issues {
			if i.Path == path || strings.HasPrefix(i.Path, path+"/") {
				out = append(out, i)
			}
		}
		return identityKeys(out)
	}
	for _, h := range holders {
		for _, r := range refs {
			t.Run(h.name+" without its "+r.missing, func(t *testing.T) {
				want := []string{"rm_invariant " + h.at + "/" + r.missing + " rm mandatory"}
				if got := at(validation.ValidateRM(h.holding(r.ref)).Issues, h.at); !slices.Equal(got, want) {
					t.Errorf("ValidateRM(%s without its %s) issues at or below %s = %q, want %q", h.name, r.missing, h.at, got, want)
				}
			})
		}
		t.Run(h.name+" zero", func(t *testing.T) {
			want := []string{"required " + h.at + " rm mandatory"}
			if got := at(validation.ValidateRM(h.holding(rm.PartyRef{})).Issues, h.at); !slices.Equal(got, want) {
				t.Errorf("ValidateRM(%s zero) issues at or below %s = %q, want %q", h.name, h.at, got, want)
			}
		})
	}
}

// TestValidateRM_REQ171_NilPointerOnlyValueIsAbsent pins REQ-171 Structural
// access on the floor: a value held by value whose only content is a nil
// pointer in an interface field is its zero value, so it is absent. A
// REFERENCE_RANGE range whose bound is a nil pointer, and an ITEM_SINGLE item
// whose value is a nil pointer, are each reported once as required at the
// attribute, and nothing is reported below it.
func TestValidateRM_REQ171_NilPointerOnlyValueIsAbsent(t *testing.T) {
	for _, tc := range []struct {
		name string
		root any
		want []string // code path layer rule
	}{
		{
			name: "DV_COUNT whose reference range has a nil-pointer bound",
			root: &rm.DVCount{Magnitude: 3, OtherReferenceRanges: []rm.ReferenceRange[rm.DVCount]{{
				Meaning: &rm.DVText{Value: "normal"},
				Range:   rm.DVInterval[rm.DVOrdered]{Lower: (*rm.DVCount)(nil)},
			}}},
			want: []string{"required /other_reference_ranges[1]/range rm mandatory"},
		},
		{
			name: "ITEM_SINGLE whose item holds only a nil-pointer value",
			root: &rm.ItemSingle{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "single"}, Item: rm.Element{Value: (*rm.DVQuantity)(nil)}},
			want: []string{"required /item rm mandatory"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := identityKeys(validation.ValidateRM(tc.root).Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) issues (code path layer rule) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestValidateRM_REQ171_VersionPayloadWalked pins REQ-171 Structural access on
// the floor: the walk goes below ORIGINAL_VERSION data, the payload a version
// built in Go holds behind a pointer to an interface. Every issue the payload
// draws alone is reported under /data, with its code, layer and rule. A
// version whose data points to a nil interface draws nothing under /data.
func TestValidateRM_REQ171_VersionPayloadWalked(t *testing.T) {
	alone := validation.ValidateRM(&rm.Composition{}).Issues
	if len(alone) == 0 {
		t.Fatal("precondition: ValidateRM(empty COMPOSITION) reports no issue")
	}
	var payload any = &rm.Composition{}
	r := validation.ValidateRM(&rm.OriginalVersion[any]{Data: &payload})
	var under []validation.Issue
	for _, i := range r.Issues {
		if strings.HasPrefix(i.Path, "/data/") {
			under = append(under, i)
		}
	}
	want := make([]validation.Issue, 0, len(alone))
	for _, i := range alone {
		i.Path = "/data" + i.Path
		want = append(want, i)
	}
	if got, w := identityKeys(under), identityKeys(want); !slices.Equal(got, w) {
		t.Errorf("ValidateRM(ORIGINAL_VERSION holding an empty COMPOSITION) issues under /data = %q, want those of the COMPOSITION alone %q", got, w)
	}

	var nothing any
	for _, i := range validation.ValidateRM(&rm.OriginalVersion[any]{Data: &nothing}).Issues {
		if i.Path == "/data" || strings.HasPrefix(i.Path, "/data/") {
			t.Errorf("ValidateRM(ORIGINAL_VERSION whose data points to a nil interface) reports %s at %s, want nothing at /data", i.Code, i.Path)
		}
	}
}

// terminologyIDOPT constrains a DV_MULTIMEDIA's media_type, a CODE_PHRASE,
// through a TERMINOLOGY_ID node whose value is one of a list.
const terminologyIDOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>media</value></template_id><concept>media</concept>
<definition><rm_type_name>DV_MULTIMEDIA</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>media_type</rm_attribute_name>
<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>CODE_PHRASE</rm_type_name><node_id></node_id>
<occurrences><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></occurrences>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>terminology_id</rm_attribute_name>
<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>TERMINOLOGY_ID</rm_type_name><node_id></node_id>
<occurrences><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></occurrences>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>
<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>
<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id></node_id>
<occurrences><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></occurrences>
<item xsi:type="C_STRING"><list>IANA_media-types</list></item>
</children>
</attributes>
</children>
</attributes>
</children>
</attributes>
</definition>
</template>`

// TestValidate_REQ171_TerminologyIDBoundToItsNode pins REQ-171 Structural
// access in the template walk: CODE_PHRASE terminology_id is read as the
// TERMINOLOGY_ID it holds, so it binds to the TERMINOLOGY_ID node that
// constrains it and the walk goes below it. A terminology the node's value
// list admits draws no template issue, and one it does not is reported at
// the TERMINOLOGY_ID's value.
func TestValidate_REQ171_TerminologyIDBoundToItsNode(t *testing.T) {
	c := mustCompileInline(t, terminologyIDOPT)
	for _, tc := range []struct {
		terminology string
		want        []string // code path layer rule
	}{
		{terminology: "IANA_media-types", want: nil},
		{terminology: "openEHR", want: []string{"primitive_not_in_list /media_type/terminology_id/value template not_in_list"}},
	} {
		t.Run(tc.terminology, func(t *testing.T) {
			media := &rm.DVMultimedia{MediaType: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: tc.terminology}, CodeString: "text/plain"}, Size: 1}
			if got := templateIssues(validation.Validate(media, c)); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(DV_MULTIMEDIA, media_type under terminology %s) template issues = %q, want %q", tc.terminology, got, tc.want)
			}
		})
	}
}
