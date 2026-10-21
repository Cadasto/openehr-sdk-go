package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// The OPTs below are small templates for the REQ-107 rules on object
// references and the identifiers they hold: a reference the generator
// writes carries an id, a namespace and a type; the id is a HIER_OBJECT_ID
// whose value is drawn like a uid where the OPT gives none (RM defaults,
// Identity); and a C_STRING the OPT pins on any part is kept.

// generateEvery runs Generate on opt under every policy, both value fills
// and both compile modes, with a counting UIDSource, so the ids the
// generator draws are uid-0001, uid-0002 and so on in the order it draws
// them. It checks each root against both validators and hands it to check.
func generateEvery(t *testing.T, opt string, check func(t *testing.T, call string, out any)) {
	t.Helper()
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range guardOptions() {
			opts.UIDSource = counterUID()
			call := fmt.Sprintf("Generate(%v, %v, implicit attributes %v)", opts.Policy, opts.ValueFill, implicit)
			out, err := instance.Generate(t.Context(), c, opts)
			if err != nil {
				t.Fatalf("%s: %v, want a root", call, err)
			}
			check(t, call, out)
			checkBothValidators(t, call, out, c)
		}
	}
}

// refString renders an object reference as the tests compare it: its
// class, namespace, type and id. A nil reference renders as "none".
func refString(ref any) string {
	r, ok := ref.(rm.ObjectRefLike)
	if !ok || rm.IsTypedNil(r) {
		return "none"
	}
	return fmt.Sprintf("%s %s::%s id %s", bmmName(ref), r.GetNamespace(), r.GetType(), idString(r.GetID()))
}

// idString renders an identifier as its class and value, and a GENERIC_ID's
// scheme. A nil id renders as "none".
func idString(id rm.ObjectID) string {
	switch v := id.(type) {
	case nil:
		return "none"
	case *rm.HierObjectID:
		return "HIER_OBJECT_ID " + v.Value
	case *rm.GenericID:
		return "GENERIC_ID " + v.Scheme + " " + v.Value
	case *rm.ObjectVersionID:
		return "OBJECT_VERSION_ID " + v.Value
	case *rm.ArchetypeID:
		return "ARCHETYPE_ID " + v.Value
	case *rm.TemplateID:
		return "TEMPLATE_ID " + v.Value
	case *rm.TerminologyID:
		return "TERMINOLOGY_ID " + v.Value
	}
	return fmt.Sprintf("%T", id)
}

func bmmName(v any) string {
	if n, ok := v.(interface{ BMMName() string }); ok {
		return n.BMMName()
	}
	return fmt.Sprintf("%T", v)
}

// requiredRef is a single-valued attribute called name the template
// requires, holding a child of rmType with attrs.
func requiredRef(name, rmType string, attrs ...string) string {
	return guardSingle(name, guardExistence11, guardChild("C_COMPLEX_OBJECT", rmType, "", guardOccurrences11, "", attrs...))
}

// pinnedString is a required single-valued attribute called name whose
// C_STRING admits only value.
func pinnedString(name, value string) string {
	return guardSingle(name, guardExistence11, guardString(value))
}

// TestREQ107_EntryReferencesTakeTheirParts is the REQ-107 check that an
// ENTRY's workflow_id and a CARE_ENTRY's guideline_id the template names
// come out whole and pass both validators, at every policy, either value
// fill and either compile mode. A workflow_id whose OBJECT_REF the template
// leaves open takes an id drawn from UIDSource, after the OBSERVATION's own
// uid, the namespace local and the type ANY. A guideline_id whose namespace,
// type and GENERIC_ID the template pins keeps every pin.
func TestREQ107_EntryReferencesTakeTheirParts(t *testing.T) {
	opt := guardRootOPT("OBSERVATION", guardObservationID,
		requiredRef("workflow_id", "OBJECT_REF"),
		requiredRef("guideline_id", "OBJECT_REF",
			pinnedString("namespace", "guidelines"),
			pinnedString("type", "GUIDELINE"),
			requiredRef("id", "GENERIC_ID", pinnedString("value", "NICE-NG136"), pinnedString("scheme", "NICE"))))
	generateEvery(t, opt, func(t *testing.T, call string, out any) {
		obs, ok := out.(*rm.Observation)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Observation", call, out)
		}
		if got, want := refString(obs.WorkflowID), "OBJECT_REF local::ANY id HIER_OBJECT_ID uid-0002"; got != want {
			t.Errorf("%s: workflow_id = %s, want %s", call, got, want)
		}
		if got, want := refString(obs.GuidelineID), "OBJECT_REF guidelines::GUIDELINE id GENERIC_ID NICE NICE-NG136"; got != want {
			t.Errorf("%s: guideline_id = %s, want %s", call, got, want)
		}
	})
}

// TestREQ107_InstructionDetailsTakeTheirParts is the REQ-107 check that an
// ACTION's instruction_details the template names comes out whole and
// passes both validators, at every policy, either value fill and either
// compile mode. Its instruction_id is a LOCATABLE_REF whose abstract
// UID_BASED_ID the generator builds as a HIER_OBJECT_ID with a value drawn
// from UIDSource, after the ACTION's own uid, with the namespace local and
// the type INSTRUCTION; the path, the activity_id and the wf_details the
// template constrains are kept.
func TestREQ107_InstructionDetailsTakeTheirParts(t *testing.T) {
	opt := guardRootOPT("ACTION", "openEHR-EHR-ACTION.example.v1",
		requiredRef("instruction_details", "INSTRUCTION_DETAILS",
			requiredRef("instruction_id", "LOCATABLE_REF",
				pinnedString("path", "/activities[at0001]"),
				requiredRef("id", "UID_BASED_ID")),
			pinnedString("activity_id", "activities[at0001]"),
			guardSingle("wf_details", guardExistence11, guardChild("C_COMPLEX_OBJECT", "ITEM_TREE", "at0010", guardOccurrences11, ""))))
	generateEvery(t, opt, func(t *testing.T, call string, out any) {
		action, ok := out.(*rm.Action)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Action", call, out)
		}
		d := action.InstructionDetails
		if d == nil {
			t.Fatalf("%s: instruction_details is unset, want the one the template names", call)
		}
		if got, want := refString(&d.InstructionID), "LOCATABLE_REF local::INSTRUCTION id HIER_OBJECT_ID uid-0002"; got != want {
			t.Errorf("%s: instruction_id = %s, want %s", call, got, want)
		}
		if d.InstructionID.Path == nil || *d.InstructionID.Path != "/activities[at0001]" {
			t.Errorf("%s: instruction_id path = %v, want /activities[at0001]", call, d.InstructionID.Path)
		}
		if d.ActivityID != "activities[at0001]" {
			t.Errorf("%s: activity_id = %q, want activities[at0001]", call, d.ActivityID)
		}
		if tree, ok := d.WfDetails.(*rm.ItemTree); !ok || tree.ArchetypeNodeID != "at0010" {
			t.Errorf("%s: wf_details = %#v, want the ITEM_TREE at0010", call, d.WfDetails)
		}
	})
}

// TestREQ107_PinnedObjectIDValueIsKept is the REQ-107 check that a C_STRING
// the template pins on the value of the identifier an OBJECT_REF holds is
// kept, whichever OBJECT_ID class the template names, at every policy,
// either value fill and either compile mode, and the output passes both
// validators. The id is never replaced by a drawn one.
func TestREQ107_PinnedObjectIDValueIsKept(t *testing.T) {
	cases := []struct {
		class, value string
	}{
		{class: "HIER_OBJECT_ID", value: "6e4b1c2a-0000-4000-8000-00000000abcd"},
		{class: "OBJECT_VERSION_ID", value: "6e4b1c2a-0000-4000-8000-00000000abcd::example.org::1"},
		{class: "ARCHETYPE_ID", value: "openEHR-EHR-OBSERVATION.example.v1"},
		{class: "TEMPLATE_ID", value: "example.template.v1"},
		{class: "TERMINOLOGY_ID", value: "SNOMED-CT"},
	}
	for _, tc := range cases {
		t.Run(tc.class, func(t *testing.T) {
			opt := guardRootOPT("OBSERVATION", guardObservationID,
				requiredRef("workflow_id", "OBJECT_REF", requiredRef("id", tc.class, pinnedString("value", tc.value))))
			generateEvery(t, opt, func(t *testing.T, call string, out any) {
				obs, ok := out.(*rm.Observation)
				if !ok {
					t.Fatalf("%s returned %T, want *rm.Observation", call, out)
				}
				if got, want := refString(obs.WorkflowID), "OBJECT_REF local::ANY id "+tc.class+" "+tc.value; got != want {
					t.Errorf("%s: workflow_id = %s, want %s", call, got, want)
				}
			})
		})
	}
}

// TestREQ107_PerformerIDTakesAValueFromUIDSource is the REQ-107 check that
// a ROLE performer whose template names its id as a HIER_OBJECT_ID, with
// no constraint on the value, comes out with a value drawn from UIDSource,
// after the ROLE's own uid, at every policy, either value fill and either
// compile mode, and passes both validators. It replaces the known-gap row
// in which rmwrite could not attach that id.
func TestREQ107_PerformerIDTakesAValueFromUIDSource(t *testing.T) {
	opt := rolePerformerOPT(requiredRef("id", "HIER_OBJECT_ID"))
	generateEvery(t, opt, func(t *testing.T, call string, out any) {
		role, ok := out.(*rm.Role)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Role", call, out)
		}
		if got, want := refString(&role.Performer), "PARTY_REF local::PERSON id HIER_OBJECT_ID uid-0002"; got != want {
			t.Errorf("%s: performer = %s, want %s", call, got, want)
		}
	})
}

// TestREQ107_PerformerWithoutIDTakesOneFromUIDSource is the REQ-107 check
// that the id a ROLE performer takes where the template names none is
// drawn from UIDSource, like a PARTY_RELATIONSHIP's source and target,
// rather than a fixed one.
func TestREQ107_PerformerWithoutIDTakesOneFromUIDSource(t *testing.T) {
	generateEvery(t, rolePerformerOPT(), func(t *testing.T, call string, out any) {
		role, ok := out.(*rm.Role)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Role", call, out)
		}
		if got, want := refString(&role.Performer), "PARTY_REF local::PERSON id HIER_OBJECT_ID uid-0002"; got != want {
			t.Errorf("%s: performer = %s, want %s", call, got, want)
		}
	})
}
