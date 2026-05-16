package bmmgen

import "testing"

func TestPascalCase(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"DV_QUANTITY", "DVQuantity"},
		{"EHR_STATUS", "EHRStatus"},
		{"OBJECT_VERSION_ID", "ObjectVersionID"},
		{"HIER_OBJECT_ID", "HierObjectID"},
		{"Iso8601_date", "ISO8601Date"},
		{"Iso8601_date_time", "ISO8601DateTime"},
		{"CODE_PHRASE", "CodePhrase"},
		{"Multiplicity_interval", "MultiplicityInterval"},
		{"X_VERSIONED_PARTY", "XVersionedParty"},
		{"DV_TEXT", "DVText"},
		{"ISO_OID", "ISOOID"},
		{"UID_BASED_ID", "UIDBasedID"},
		{"ARCHETYPE_ID", "ArchetypeID"},
		{"HL7", "HL7"},
		{"Cardinality", "Cardinality"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := PascalCase(c.in)
			if got != c.want {
				t.Errorf("PascalCase(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestFileBase(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"org.openehr.rm.data_types.quantity", "data_types_quantity"},
		{"org.openehr.base.base_types.identification", "base_types_identification"},
		{"org.openehr.base.foundation_types.primitive_types", "foundation_types_primitive_types"},
		{"org.openehr.rm.composition.content.entry", "composition_content_entry"},
		{"org.openehr.rm.ehr", "ehr"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := FileBase(c.in)
			if got != c.want {
				t.Errorf("FileBase(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
