package instance_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The OPTs below are small EHR_STATUS and FOLDER templates for the REQ-107
// rule that the generator builds any RM class an OPT roots, here the
// EHR-wide roots REQ-110 validates: what the template names comes out, and
// the root passes both validators and the typed REQ-110 entry for its
// class.

// optTrueOnly is a required single-valued attribute called name whose
// C_BOOLEAN admits true only.
func optTrueOnly(name string) string {
	return guardSingle(name, guardExistence11, optPrimitive("BOOLEAN", "C_BOOLEAN",
		"<true_valid>true</true_valid><false_valid>false</false_valid>"))
}

// TestREQ107_EHRStatusRootTakesItsParts is the REQ-107 check that an
// EHR_STATUS root whose template names its subject, both flags and
// other_details comes out with each, at every policy, either value fill and
// either compile mode, and passes both validators and
// validation.ValidateEHRStatus (REQ-110). The subject is a PARTY_SELF whose
// external_ref keeps the namespace the template pins and takes the type
// PERSON and an id drawn from UIDSource; is_queryable and is_modifiable
// take the one value their C_BOOLEAN admits.
func TestREQ107_EHRStatusRootTakesItsParts(t *testing.T) {
	opt := guardRootOPT("EHR_STATUS", "openEHR-EHR-EHR_STATUS.generic.v1",
		requiredRef("subject", "PARTY_SELF", requiredRef("external_ref", "PARTY_REF", pinnedString("namespace", "DEMOGRAPHIC"))),
		optTrueOnly("is_queryable"),
		optTrueOnly("is_modifiable"),
		guardSingle("other_details", guardExistence11, guardChild("C_COMPLEX_OBJECT", "ITEM_TREE", "at0001", guardOccurrences11, "")))
	generateEveryCompiled(t, opt, func(t *testing.T, call string, out any, c *templatecompile.Compiled) {
		status, ok := out.(*rm.EHRStatus)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.EHRStatus", call, out)
		}
		if got, want := refString(status.Subject.ExternalRef), "PARTY_REF DEMOGRAPHIC::PERSON id HIER_OBJECT_ID uid-0001"; got != want {
			t.Errorf("%s: subject external_ref = %s, want %s", call, got, want)
		}
		if !status.IsQueryable || !status.IsModifiable {
			t.Errorf("%s: is_queryable %v, is_modifiable %v, want both true", call, status.IsQueryable, status.IsModifiable)
		}
		if tree, ok := status.OtherDetails.(*rm.ItemTree); !ok || tree.ArchetypeNodeID != "at0001" {
			t.Errorf("%s: other_details = %#v, want the ITEM_TREE at0001", call, status.OtherDetails)
		}
		if r := validation.ValidateEHRStatus(status, c); !r.OK {
			t.Errorf("%s: ValidateEHRStatus issues %+v, want none", call, r.Issues)
		}
	})
}

// TestREQ107_FolderRootTakesItsParts is the REQ-107 check that a FOLDER root
// whose template names its details, a sub-folder and two items comes out
// with each, at every policy, either value fill and either compile mode,
// and passes both validators and validation.ValidateFolder (REQ-110). The
// items are an OBJECT_REF whose type the template pins and an
// ACCESS_GROUP_REF it leaves open, which takes the type ACCESS_GROUP; each
// takes the namespace local and an id drawn from UIDSource, in OPT order.
func TestREQ107_FolderRootTakesItsParts(t *testing.T) {
	opt := guardRootOPT("FOLDER", "openEHR-EHR-FOLDER.generic.v1",
		guardSingle("details", guardExistence11, guardChild("C_COMPLEX_OBJECT", "ITEM_TREE", "at0001", guardOccurrences11, "")),
		guardMultiple("folders", guardExistence11, guardChild("C_COMPLEX_OBJECT", "FOLDER", "at0002", guardOccurrences11, "")),
		guardMultiple("items", guardExistence11,
			guardChild("C_COMPLEX_OBJECT", "OBJECT_REF", "", guardOccurrences11, "", pinnedString("type", "VERSIONED_COMPOSITION")),
			guardChild("C_COMPLEX_OBJECT", "ACCESS_GROUP_REF", "", guardOccurrences11, "")))
	generateEveryCompiled(t, opt, func(t *testing.T, call string, out any, c *templatecompile.Compiled) {
		folder, ok := out.(*rm.Folder)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Folder", call, out)
		}
		if tree, ok := folder.Details.(*rm.ItemTree); !ok || tree.ArchetypeNodeID != "at0001" {
			t.Errorf("%s: details = %#v, want the ITEM_TREE at0001", call, folder.Details)
		}
		if len(folder.Folders) != 1 || folder.Folders[0].ArchetypeNodeID != "at0002" {
			t.Errorf("%s: folders = %#v, want one FOLDER at0002", call, folder.Folders)
		}
		var items []string
		for _, item := range folder.Items {
			items = append(items, refString(item))
		}
		want := []string{
			"OBJECT_REF local::VERSIONED_COMPOSITION id HIER_OBJECT_ID uid-0001",
			"ACCESS_GROUP_REF local::ACCESS_GROUP id HIER_OBJECT_ID uid-0002",
		}
		if len(items) != len(want) || items[0] != want[0] || items[1] != want[1] {
			t.Errorf("%s: items = %q, want %q", call, items, want)
		}
		if r := validation.ValidateFolder(folder, c); !r.OK {
			t.Errorf("%s: ValidateFolder issues %+v, want none", call, r.Issues)
		}
	})
}
