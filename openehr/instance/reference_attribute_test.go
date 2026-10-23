package instance_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
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
	generateEveryCompiled(t, opt, func(t *testing.T, call string, out any, _ *templatecompile.Compiled) {
		t.Helper()
		check(t, call, out)
	})
}

// generateEveryCompiled is generateEvery, and also hands check the compiled
// template the root was generated from.
func generateEveryCompiled(t *testing.T, opt string, check func(t *testing.T, call string, out any, c *templatecompile.Compiled)) {
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
			check(t, call, out, c)
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
// validators. The id is never replaced by a drawn one. Where the template
// names the abstract OBJECT_ID or UID_BASED_ID, the id is a HIER_OBJECT_ID
// (RM defaults).
func TestREQ107_PinnedObjectIDValueIsKept(t *testing.T) {
	cases := []struct {
		// class is the id class the template names, built the class the
		// generator builds for it.
		class, built, value string
	}{
		{class: "HIER_OBJECT_ID", built: "HIER_OBJECT_ID", value: "6e4b1c2a-0000-4000-8000-00000000abcd"},
		{class: "OBJECT_VERSION_ID", built: "OBJECT_VERSION_ID", value: "6e4b1c2a-0000-4000-8000-00000000abcd::example.org::1"},
		{class: "ARCHETYPE_ID", built: "ARCHETYPE_ID", value: "openEHR-EHR-OBSERVATION.example.v1"},
		{class: "TEMPLATE_ID", built: "TEMPLATE_ID", value: "example.template.v1"},
		{class: "TERMINOLOGY_ID", built: "TERMINOLOGY_ID", value: "SNOMED-CT"},
		{class: "OBJECT_ID", built: "HIER_OBJECT_ID", value: "6e4b1c2a-0000-4000-8000-00000000abcd"},
		{class: "UID_BASED_ID", built: "HIER_OBJECT_ID", value: "6e4b1c2a-0000-4000-8000-00000000abcd"},
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
				if got, want := refString(obs.WorkflowID), "OBJECT_REF local::ANY id "+tc.built+" "+tc.value; got != want {
					t.Errorf("%s: workflow_id = %s, want %s", call, got, want)
				}
			})
		})
	}
}

// TestREQ107_AccessGroupTypeIsAccessGroup is the REQ-107 check that an
// ACCESS_GROUP_REF the generator writes, here as an ENTRY's workflow_id,
// carries the type ACCESS_GROUP, the one ACCESS_GROUP_REF Type_validity
// admits, at every policy, either value fill and either compile mode, and
// passes both validators: where the template leaves the type open, and
// where a C_STRING on it admits that type among others (Precedence 3). A
// C_STRING that rejects ACCESS_GROUP makes Generate return an error
// wrapping ErrConstraintUnsatisfiable, and no root.
func TestREQ107_AccessGroupTypeIsAccessGroup(t *testing.T) {
	cases := []struct {
		name          string
		attrs         []string
		unsatisfiable bool
	}{
		{name: "type left open"},
		{name: "pattern .*", attrs: []string{guardSingle("type", guardExistence11, optPrimitive("STRING", "C_STRING", "<pattern>.*</pattern>"))}},
		{name: "list with another class first", attrs: []string{guardSingle("type", guardExistence11,
			optPrimitive("STRING", "C_STRING", "<list>GROUP</list><list>ACCESS_GROUP</list>"))}},
		{name: "list that rejects ACCESS_GROUP", attrs: []string{pinnedString("type", "GROUP")}, unsatisfiable: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt := guardRootOPT("OBSERVATION", guardObservationID, requiredRef("workflow_id", "ACCESS_GROUP_REF", tc.attrs...))
			if tc.unsatisfiable {
				c := compileOPTText(t, opt, true)
				for _, opts := range guardOptions() {
					call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
					out, err := instance.Generate(t.Context(), c, opts)
					if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
						t.Errorf("%s error = %v, want one wrapping ErrConstraintUnsatisfiable", call, err)
					}
					if out != nil {
						t.Errorf("%s returned %T, want no root", call, out)
					}
				}
				return
			}
			generateEvery(t, opt, func(t *testing.T, call string, out any) {
				obs, ok := out.(*rm.Observation)
				if !ok {
					t.Fatalf("%s returned %T, want *rm.Observation", call, out)
				}
				if got, want := refString(obs.WorkflowID), "ACCESS_GROUP_REF local::ACCESS_GROUP id HIER_OBJECT_ID uid-0002"; got != want {
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

// TestREQ107_UIDSourceReturningNilDoesNotPanic is the REQ-107 check that a
// UIDSource that returns nil, where the generator draws the value of an id
// a reference holds, makes Generate return rather than panic, at every
// policy and either value fill. The id then keeps no value.
func TestREQ107_UIDSourceReturningNilDoesNotPanic(t *testing.T) {
	c := compileOPTText(t, guardRootOPT("OBSERVATION", guardObservationID,
		requiredRef("workflow_id", "OBJECT_REF", requiredRef("id", "HIER_OBJECT_ID"))), true)
	for _, opts := range guardOptions() {
		opts.UIDSource = func() *rm.HierObjectID { return nil }
		call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
		out, err := instance.Generate(t.Context(), c, opts)
		if err != nil {
			t.Fatalf("%s: %v, want a root", call, err)
		}
		obs, ok := out.(*rm.Observation)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Observation", call, out)
		}
		if got, want := refString(obs.WorkflowID), "OBJECT_REF local::ANY id HIER_OBJECT_ID "; got != want {
			t.Errorf("%s: workflow_id = %q, want %q", call, got, want)
		}
	}
}

// TestREQ107_RequiredReferenceTheTemplateNamesNoChildForIsBuilt is the
// REQ-107 check that a reference the generator builds from the RM alone,
// for a multi-valued attribute whose cardinality asks for a member and that
// the template names no child for, comes out whole and passes both
// validators, at every policy, either value fill and either compile mode.
// An existence of 1..1 alone would ask for no member (What a visit writes). A FOLDER's items hold an OBJECT_REF,
// so the member takes the namespace local, the type ANY and an id drawn
// from UIDSource; a PERSON's roles hold a PARTY_REF, so the member takes
// the type ROLE, the one a role's reference has, and an id drawn after the
// PERSON's own uid.
func TestREQ107_RequiredReferenceTheTemplateNamesNoChildForIsBuilt(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// member returns the reference the root holds in the attribute.
		member func(t *testing.T, call string, out any) string
		want   string
	}{
		{
			name: "FOLDER items",
			opt:  guardRootOPT("FOLDER", "openEHR-EHR-FOLDER.generic.v1", guardCountedMultiple("items", guardExistence11)),
			member: func(t *testing.T, call string, out any) string {
				t.Helper()
				folder, ok := out.(*rm.Folder)
				if !ok {
					t.Fatalf("%s returned %T, want *rm.Folder", call, out)
				}
				if len(folder.Items) != 1 {
					t.Fatalf("%s: items has %d members, want 1", call, len(folder.Items))
				}
				return refString(folder.Items[0])
			},
			want: "OBJECT_REF local::ANY id HIER_OBJECT_ID uid-0001",
		},
		{
			name: "PERSON roles",
			opt:  guardRootOPT("PERSON", "openEHR-DEMOGRAPHIC-PERSON.example.v1", guardCountedMultiple("roles", guardExistence11)),
			member: func(t *testing.T, call string, out any) string {
				t.Helper()
				person, ok := out.(*rm.Person)
				if !ok {
					t.Fatalf("%s returned %T, want *rm.Person", call, out)
				}
				if len(person.Roles) != 1 {
					t.Fatalf("%s: roles has %d members, want 1", call, len(person.Roles))
				}
				return refString(&person.Roles[0])
			},
			want: "PARTY_REF local::ROLE id HIER_OBJECT_ID uid-0002",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			generateEvery(t, tc.opt, func(t *testing.T, call string, out any) {
				if got := tc.member(t, call, out); got != tc.want {
					t.Errorf("%s: member = %s, want %s", call, got, tc.want)
				}
			})
		})
	}
}

// idForm is one identifier class a reference's id takes, and the value the
// generator gives it where the template names the class with no value.
type idForm struct {
	class string
	// uidBased is set for the classes a UID_BASED_ID holds: a
	// LOCATABLE_REF's id is no other class.
	uidBased bool
	// attrs are pins the template gives the id beside its value, which it
	// leaves open: a GENERIC_ID's scheme.
	attrs []string
	// shown renders the id as idString renders it, given the uid drawn.
	shown func(uid string) string
	// parse is the class's parser in rm, nil for a class that has none.
	parse func(value string) error
}

// idForms are the identifier classes an OBJECT_REF's id takes that the
// generator gives a value: the value of a drawn uid for a HIER_OBJECT_ID, a
// GENERIC_ID, a TEMPLATE_ID and a TERMINOLOGY_ID; that value followed by
// ::example::1 for an OBJECT_VERSION_ID; and a fixed archetype id for an
// ARCHETYPE_ID, for which no uid is drawn.
var idForms = []idForm{
	{
		class:    "HIER_OBJECT_ID",
		uidBased: true,
		shown:    func(uid string) string { return "HIER_OBJECT_ID " + uid },
	},
	{
		class: "GENERIC_ID",
		attrs: []string{pinnedString("scheme", "local")},
		shown: func(uid string) string { return "GENERIC_ID local " + uid },
	},
	{
		class: "TEMPLATE_ID",
		shown: func(uid string) string { return "TEMPLATE_ID " + uid },
	},
	{
		class: "TERMINOLOGY_ID",
		shown: func(uid string) string { return "TERMINOLOGY_ID " + uid },
		parse: func(value string) error { _, err := rm.ParseTerminologyID(value); return err },
	},
	{
		class:    "OBJECT_VERSION_ID",
		uidBased: true,
		shown:    func(uid string) string { return "OBJECT_VERSION_ID " + uid + "::example::1" },
		parse:    func(value string) error { _, err := rm.ParseObjectVersionID(value); return err },
	},
	{
		class: "ARCHETYPE_ID",
		shown: func(string) string { return "ARCHETYPE_ID openEHR-EHR-CLUSTER.example.v1" },
		parse: func(value string) error { _, err := rm.ParseArchetypeID(value); return err },
	},
}

// idHolder is a reference the generator writes whose id the template
// names, with the OPT that holds it and the reference as the root holds it.
type idHolder struct {
	name string
	// uidBasedOnly is set where the reference's id is a UID_BASED_ID, which
	// no other identifier class fits.
	uidBasedOnly bool
	// opt returns the OPT whose reference holds id, the OPT of its id node.
	opt func(id string) string
	// ref returns the reference, and what refString renders of it before
	// the id.
	ref func(t *testing.T, call string, out any) (ref any, shown string)
}

// idHolders are the three kinds of reference the cases below put an id on:
// an OBJECT_REF, a PARTY_REF and a LOCATABLE_REF, whose id is a UID_BASED_ID.
var idHolders = []idHolder{
	{
		name: "OBSERVATION workflow_id",
		opt: func(id string) string {
			return guardRootOPT("OBSERVATION", guardObservationID, requiredRef("workflow_id", "OBJECT_REF", id))
		},
		ref: func(t *testing.T, call string, out any) (any, string) {
			t.Helper()
			obs, ok := out.(*rm.Observation)
			if !ok {
				t.Fatalf("%s returned %T, want *rm.Observation", call, out)
			}
			return obs.WorkflowID, "OBJECT_REF local::ANY id "
		},
	},
	{
		name: "ROLE performer",
		opt:  func(id string) string { return rolePerformerOPT(id) },
		ref: func(t *testing.T, call string, out any) (any, string) {
			t.Helper()
			role, ok := out.(*rm.Role)
			if !ok {
				t.Fatalf("%s returned %T, want *rm.Role", call, out)
			}
			return &role.Performer, "PARTY_REF local::PERSON id "
		},
	},
	{
		name:         "ACTION instruction_id",
		uidBasedOnly: true,
		opt: func(id string) string {
			return guardRootOPT("ACTION", "openEHR-EHR-ACTION.example.v1",
				requiredRef("instruction_details", "INSTRUCTION_DETAILS",
					requiredRef("instruction_id", "LOCATABLE_REF", id),
					pinnedString("activity_id", "activities[at0001]"),
					guardSingle("wf_details", guardExistence11, guardChild("C_COMPLEX_OBJECT", "ITEM_TREE", "at0010", guardOccurrences11, ""))))
		},
		ref: func(t *testing.T, call string, out any) (any, string) {
			t.Helper()
			action, ok := out.(*rm.Action)
			if !ok {
				t.Fatalf("%s returned %T, want *rm.Action", call, out)
			}
			if action.InstructionDetails == nil {
				t.Fatalf("%s: instruction_details is unset, want the one the template names", call)
			}
			return &action.InstructionDetails.InstructionID, "LOCATABLE_REF local::INSTRUCTION id "
		},
	},
}

// eachIDCase runs fn as a subtest for each reference an id of each class
// fits: every class on an OBJECT_REF and a PARTY_REF, the UID_BASED_ID
// classes alone on a LOCATABLE_REF.
func eachIDCase(t *testing.T, fn func(t *testing.T, holder idHolder, form idForm)) {
	t.Helper()
	for _, holder := range idHolders {
		for _, form := range idForms {
			if holder.uidBasedOnly && !form.uidBased {
				continue
			}
			t.Run(holder.name+"/"+form.class, func(t *testing.T) { fn(t, holder, form) })
		}
	}
}

// TestREQ107_EmptyReferenceIDTakesItsClassForm is the REQ-107 Identity
// check that an id the template names with no value on a reference the
// generator writes takes a value in its class's form, whichever reference
// holds it, at every policy, either value fill and either compile mode, and
// the output passes both validators. A HIER_OBJECT_ID, a GENERIC_ID, a
// TEMPLATE_ID and a TERMINOLOGY_ID take the value of the uid drawn from
// UIDSource, after the root's own uid; an OBJECT_VERSION_ID takes that value
// followed by ::example::1; and an ARCHETYPE_ID takes
// openEHR-EHR-CLUSTER.example.v1, with no uid drawn. A class the rm package
// parses accepts what the generator wrote.
func TestREQ107_EmptyReferenceIDTakesItsClassForm(t *testing.T) {
	eachIDCase(t, func(t *testing.T, holder idHolder, form idForm) {
		opt := holder.opt(requiredRef("id", form.class, form.attrs...))
		generateEvery(t, opt, func(t *testing.T, call string, out any) {
			ref, prefix := holder.ref(t, call, out)
			if got, want := refString(ref), prefix+form.shown("uid-0002"); got != want {
				t.Errorf("%s: reference = %s, want %s", call, got, want)
			}
			checkIDParses(t, call, ref, form)
		})
	})
}

// TestREQ107_EmptyReferenceIDParsesWithTheRandomUIDFallback is the REQ-107
// Identity check that an id the template names with no value takes a value
// in its class's form where Options.UIDSource is nil, so the uid is the
// random fallback's: the rm package's parser accepts what the generator
// wrote for each class that has one, and every other class gets a value. It
// holds at every policy, either value fill and either compile mode, and the
// output passes both validators.
func TestREQ107_EmptyReferenceIDParsesWithTheRandomUIDFallback(t *testing.T) {
	eachIDCase(t, func(t *testing.T, holder idHolder, form idForm) {
		opt := holder.opt(requiredRef("id", form.class, form.attrs...))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, opt, implicit)
			for _, opts := range guardOptions() {
				call := fmt.Sprintf("Generate(%v, %v, implicit attributes %v)", opts.Policy, opts.ValueFill, implicit)
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%s: %v, want a root", call, err)
				}
				ref, _ := holder.ref(t, call, out)
				checkIDParses(t, call, ref, form)
				checkBothValidators(t, call, out, c)
			}
		}
	})
}

// checkIDParses fails t unless the id ref holds has a value and, where
// form's class has a parser in rm, the parser accepts it.
func checkIDParses(t *testing.T, call string, ref any, form idForm) {
	t.Helper()
	r, ok := ref.(rm.ObjectRefLike)
	if !ok || rm.IsTypedNil(r) {
		t.Fatalf("%s: %T is no reference", call, ref)
	}
	value := idValue(r.GetID())
	if value == "" {
		t.Errorf("%s: %s id has no value, want one", call, form.class)
		return
	}
	if form.parse == nil {
		return
	}
	if err := form.parse(value); err != nil {
		t.Errorf("%s: %s id value %q: %v, want one its class's parser accepts", call, form.class, value, err)
	}
}

// idValue returns the value of an identifier, or "" for none.
func idValue(id rm.ObjectID) string {
	switch v := id.(type) {
	case *rm.HierObjectID:
		return v.Value
	case *rm.GenericID:
		return v.Value
	case *rm.ObjectVersionID:
		return v.Value
	case *rm.ArchetypeID:
		return v.Value
	case *rm.TemplateID:
		return v.Value
	case *rm.TerminologyID:
		return v.Value
	}
	return ""
}

// TestREQ107_UIDSourceReturningNilLeavesAUIDBasedValueEmpty is the REQ-107
// Identity and Exceptions check that a UIDSource that returns nil, where the
// generator draws the value of an id a reference holds, leaves the value of
// each class that takes the uid's value empty, an OBJECT_VERSION_ID's
// included, which no ::example::1 stands alone for, while an ARCHETYPE_ID,
// for which no uid is drawn, takes its class's form all the same. It holds
// at every policy and either value fill.
func TestREQ107_UIDSourceReturningNilLeavesAUIDBasedValueEmpty(t *testing.T) {
	holder := idHolders[0]
	for _, form := range idForms {
		t.Run(form.class, func(t *testing.T) {
			c := compileOPTText(t, holder.opt(requiredRef("id", form.class, form.attrs...)), true)
			for _, opts := range guardOptions() {
				opts.UIDSource = func() *rm.HierObjectID { return nil }
				call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%s: %v, want a root", call, err)
				}
				ref, _ := holder.ref(t, call, out)
				want := ""
				if form.class == "ARCHETYPE_ID" {
					want = "openEHR-EHR-CLUSTER.example.v1"
				}
				if got := idValue(ref.(rm.ObjectRefLike).GetID()); got != want {
					t.Errorf("%s: %s id value = %q, want %q", call, form.class, got, want)
				}
			}
		})
	}
}
