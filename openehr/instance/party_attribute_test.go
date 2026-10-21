package instance_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The OPTs below are small ROLE and PERSON templates for the REQ-107 rules
// on party attributes the OPT names: the output passes both validators, and
// a C_STRING the OPT pins is honoured.

// guardString is a C_PRIMITIVE_OBJECT whose C_STRING admits only value.
func guardString(value string) string {
	return optPrimitive("STRING", "C_STRING", "<list>"+value+"</list>")
}

// rolePerformerOPT is a ROLE template that names performer as a PARTY_REF.
// The PARTY_REF holds attrs, such as C_STRING pins on namespace and type.
func rolePerformerOPT(attrs ...string) string {
	return guardRootOPT("ROLE", "openEHR-DEMOGRAPHIC-ROLE.example.v1",
		guardSingle("performer", guardExistence11,
			guardChild("C_COMPLEX_OBJECT", "PARTY_REF", "", guardOccurrences11, "", attrs...)))
}

// checkBothValidators fails t unless out passes the RM floor and the
// template validator for c.
func checkBothValidators(t *testing.T, call string, out any, c *templatecompile.Compiled) {
	t.Helper()
	if r := validation.ValidateRM(out); !r.OK {
		t.Errorf("%s: ValidateRM issues %+v, want none", call, r.Issues)
	}
	if r := validation.Validate(out, c); !r.OK {
		t.Errorf("%s: Validate issues %+v, want none", call, r.Issues)
	}
}

// TestREQ107_RolePerformerPassesBothValidators is the REQ-107 check that a
// ROLE whose template names performer as a PARTY_REF generates output both
// validators accept, at every policy and either value fill. With no pins,
// the performer takes the generator's default reference, local::PERSON. A
// C_STRING the template pins on the reference's namespace or type is
// honoured, and only the parts the template leaves open take the default.
func TestREQ107_RolePerformerPassesBothValidators(t *testing.T) {
	cases := []struct {
		name          string
		opt           string
		namespace, rt string
	}{
		{name: "performer without pins", opt: rolePerformerOPT(), namespace: "local", rt: "PERSON"},
		{
			name: "performer with namespace and type pinned",
			opt: rolePerformerOPT(
				guardSingle("namespace", guardExistence11, guardString("demographic")),
				guardSingle("type", guardExistence11, guardString("ORGANISATION")),
			),
			namespace: "demographic", rt: "ORGANISATION",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, true)
			for _, opts := range guardOptions() {
				call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%s: %v, want a root", call, err)
				}
				role, ok := out.(*rm.Role)
				if !ok {
					t.Fatalf("%s returned %T, want *rm.Role", call, out)
				}
				p := role.Performer
				if p.ID == nil || p.Namespace != tc.namespace || p.Type != tc.rt {
					t.Errorf("%s: performer = id %v, namespace %q, type %q; want an id, namespace %q, type %q",
						call, p.ID, p.Namespace, p.Type, tc.namespace, tc.rt)
				}
				checkBothValidators(t, call, out, c)
			}
		})
	}
}

// guardDateInterval is a time_validity attribute the template requires,
// holding a DV_INTERVAL<DV_DATE>.
func guardDateInterval() string {
	return guardSingle("time_validity", guardExistence11,
		guardChild("C_COMPLEX_OBJECT", "DV_INTERVAL&lt;DV_DATE&gt;", "", guardOccurrences11, ""))
}

// TestREQ107_TimeValidityPassesBothValidators is the REQ-107 check that a
// time_validity the template requires, on a ROLE root or on a capability
// of one, comes out set and passes both validators at every policy and
// either value fill.
func TestREQ107_TimeValidityPassesBothValidators(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// validity returns the time_validity the case requires.
		validity func(t *testing.T, call string, role *rm.Role) *rm.DVInterval[rm.DVDate]
	}{
		{
			name: "on a ROLE root",
			opt:  guardRootOPT("ROLE", "openEHR-DEMOGRAPHIC-ROLE.example.v1", guardDateInterval()),
			validity: func(t *testing.T, call string, role *rm.Role) *rm.DVInterval[rm.DVDate] {
				t.Helper()
				return role.TimeValidity
			},
		},
		{
			name: "on a ROLE capability",
			opt: guardRootOPT("ROLE", "openEHR-DEMOGRAPHIC-ROLE.example.v1",
				guardMultiple("capabilities", guardExistence11,
					guardChild("C_COMPLEX_OBJECT", "CAPABILITY", "at0001", guardOccurrences11, "", guardDateInterval()))),
			validity: func(t *testing.T, call string, role *rm.Role) *rm.DVInterval[rm.DVDate] {
				t.Helper()
				if len(role.Capabilities) != 1 {
					t.Fatalf("%s: the ROLE has %d capabilities, want 1", call, len(role.Capabilities))
				}
				return role.Capabilities[0].TimeValidity
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, true)
			for _, opts := range guardOptions() {
				call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%s: %v, want a root", call, err)
				}
				role, ok := out.(*rm.Role)
				if !ok {
					t.Fatalf("%s returned %T, want *rm.Role", call, out)
				}
				if tc.validity(t, call, role) == nil {
					t.Errorf("%s: time_validity is unset, want the interval the template requires", call)
				}
				checkBothValidators(t, call, out, c)
			}
		})
	}
}

// TestREQ107_ActorLanguagesAndRolesTakeTheirMembers is the REQ-107 check
// that a PERSON whose template names languages holding a DV_TEXT and roles
// holding a PARTY_REF comes out with one member of each, at every policy,
// either value fill and either compile mode, and passes both validators,
// which bind such a member by its RM type. The language is the text the
// template pins. A role whose template leaves it open takes the namespace
// local, the type ROLE and an id drawn from UIDSource, after the PERSON's
// own uid; a pattern on its type that admits ROLE among other names gives
// ROLE too, and a list that names only another class keeps that one (a list
// that names ROLE and another class keeps the member the value fill picks:
// TestREQ107_RoleTypeListKeepsItsMemberOverTheDefault). These replace the
// known-gap rows in which rmwrite could not write either attribute.
func TestREQ107_ActorLanguagesAndRolesTakeTheirMembers(t *testing.T) {
	cases := []struct {
		name     string
		roleType []string // the attributes the template names on the role's PARTY_REF
		want     string
	}{
		{name: "role left open", want: "PARTY_REF local::ROLE id HIER_OBJECT_ID uid-0002"},
		{
			name:     "role type pattern .*",
			roleType: []string{guardSingle("type", guardExistence11, optPrimitive("STRING", "C_STRING", "<pattern>.*</pattern>"))},
			want:     "PARTY_REF local::ROLE id HIER_OBJECT_ID uid-0002",
		},
		{
			name:     "role type pinned PERSON",
			roleType: []string{guardSingle("type", guardExistence11, guardString("PERSON"))},
			want:     "PARTY_REF local::PERSON id HIER_OBJECT_ID uid-0002",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt := guardRootOPT("PERSON", "openEHR-DEMOGRAPHIC-PERSON.example.v1",
				guardMultiple("languages", guardExistence11,
					guardChild("C_COMPLEX_OBJECT", "DV_TEXT", "", guardOccurrences11, "",
						guardSingle("value", guardExistence11, guardString("Dutch")))),
				guardMultiple("roles", guardExistence11,
					guardChild("C_COMPLEX_OBJECT", "PARTY_REF", "", guardOccurrences11, "", tc.roleType...)))
			generateEvery(t, opt, func(t *testing.T, call string, out any) {
				person, ok := out.(*rm.Person)
				if !ok {
					t.Fatalf("%s returned %T, want *rm.Person", call, out)
				}
				if len(person.Languages) != 1 || person.Languages[0] == nil || person.Languages[0].GetValue() != "Dutch" {
					t.Errorf("%s: languages = %#v, want one text Dutch", call, person.Languages)
				}
				if len(person.Roles) != 1 {
					t.Fatalf("%s: roles has %d members, want 1", call, len(person.Roles))
				}
				if got := refString(&person.Roles[0]); got != tc.want {
					t.Errorf("%s: roles[0] = %s, want %s", call, got, tc.want)
				}
			})
		})
	}
}

// TestREQ107_RoleTypeListKeepsItsMemberOverTheDefault is the REQ-107 check
// that a list the template pins on the type of a PERSON's role keeps the
// member the value fill picks, where Type_validity admits it, rather than
// the type ROLE a role takes where the template leaves it open
// (Precedence 3), at every policy and either compile mode. ExampleFill
// takes the list's first member and RandomFill any member. Both validators
// accept the PERSON.
func TestREQ107_RoleTypeListKeepsItsMemberOverTheDefault(t *testing.T) {
	cases := []struct {
		name string
		list string // the C_STRING body
		// wantExample is the type ExampleFill must give, and wantRandom the
		// types RandomFill may give.
		wantExample string
		wantRandom  []string
	}{
		{
			name:        "ROLE after another class name",
			list:        "<list>ORGANISATION</list><list>ROLE</list>",
			wantExample: "ORGANISATION",
			wantRandom:  []string{"ORGANISATION", "ROLE"},
		},
		{
			name:        "ROLE only",
			list:        "<list>ROLE</list>",
			wantExample: "ROLE",
			wantRandom:  []string{"ROLE"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt := guardRootOPT("PERSON", "openEHR-DEMOGRAPHIC-PERSON.example.v1",
				guardMultiple("roles", guardExistence11,
					guardChild("C_COMPLEX_OBJECT", "PARTY_REF", "", guardOccurrences11, "",
						guardSingle("type", guardExistence11, optPrimitive("STRING", "C_STRING", tc.list)))))
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, opt, implicit)
				for _, opts := range guardOptions() {
					call := fmt.Sprintf("Generate(%v, %v, implicit attributes %v)", opts.Policy, opts.ValueFill, implicit)
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("%s: %v, want a root", call, err)
					}
					person, ok := out.(*rm.Person)
					if !ok {
						t.Fatalf("%s returned %T, want *rm.Person", call, out)
					}
					if len(person.Roles) != 1 {
						t.Fatalf("%s: roles has %d members, want 1", call, len(person.Roles))
					}
					got := person.Roles[0].Type
					if opts.ValueFill == instance.ExampleFill && got != tc.wantExample {
						t.Errorf("%s: role type = %q, want %q", call, got, tc.wantExample)
					}
					if opts.ValueFill == instance.RandomFill && !slices.Contains(tc.wantRandom, got) {
						t.Errorf("%s: role type = %q, want one of %v", call, got, tc.wantRandom)
					}
					checkBothValidators(t, call, out, c)
				}
			}
		})
	}
}

// TestREQ107_AttributeThePinnedRMDoesNotHaveIsRefused is the REQ-107 check
// that an attribute the pinned RM does not have cannot be written, so
// Generate returns an error wrapping rmwrite.ErrUnknownAttribute, and no
// root, where a template puts an object under one, at every policy and
// either value fill (§ Exceptions and known gaps, "An attribute the pinned
// RM does not have"). Its row is a made-up attribute on a DV_ORDINAL, which
// no writer can write. The rows it pinned as known gaps are written now: a
// PERSON's languages and roles and a ROLE performer's id
// (TestREQ107_ActorLanguagesAndRolesTakeTheirMembers,
// TestREQ107_PerformerIDTakesAValueFromUIDSource), and a DV_ORDINAL's
// symbol the template names as a node of its own
// (TestREQ107_OrdinalAndScaleTakeTheirSymbolAndValue).
func TestREQ107_AttributeThePinnedRMDoesNotHaveIsRefused(t *testing.T) {
	cases := []struct {
		name string
		opt  string
	}{
		{
			name: "DV_ORDINAL attribute the pinned RM does not have",
			opt: optTemplate("ELEMENT", optSingle("value", optNode("DV_ORDINAL", "",
				optSingle("no_such_attribute", optNode("DV_CODED_TEXT", ""))))),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, true)
			for _, opts := range guardOptions() {
				call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
				out, err := instance.Generate(t.Context(), c, opts)
				if !errors.Is(err, rmwrite.ErrUnknownAttribute) {
					t.Errorf("%s error = %v, want one wrapping rmwrite.ErrUnknownAttribute", call, err)
				}
				if out != nil {
					t.Errorf("%s returned %T, want no root", call, out)
				}
			}
		})
	}
}

// partyRefTypes are the class names BASE PARTY_REF Type_validity admits as
// a reference's type.
var partyRefTypes = []string{"PERSON", "ORGANISATION", "GROUP", "AGENT", "ROLE", "PARTY", "ACTOR"}

// TestREQ107_PinnedPerformerTypeIsAPartyClass is the REQ-107 check that a
// C_STRING the template pins on a ROLE performer's type yields a type BASE
// PARTY_REF Type_validity admits, at every policy and either value fill.
// Neither validator evaluates that invariant, so the test reads the value.
// An open C_STRING or a pattern that also admits other strings gives the
// default PERSON; a list pin keeps its member, and ExampleFill its first
// member when the list has several; a pin that rejects PERSON and whose own
// example is no class name gives the first class name it admits; and a pin
// that admits no class name makes Generate return an error wrapping
// ErrConstraintUnsatisfiable, and no root.
func TestREQ107_PinnedPerformerTypeIsAPartyClass(t *testing.T) {
	cases := []struct {
		name string
		pin  string // the C_STRING body
		// want is the type every setting must give, and wantExample the
		// type ExampleFill must give; "" accepts any class name
		// Type_validity admits.
		want, wantExample string
		unsatisfiable     bool
	}{
		{name: "open C_STRING", pin: "", want: "PERSON"},
		{name: "pattern .*", pin: "<pattern>.*</pattern>", want: "PERSON"},
		{name: "pattern [A-Z]+", pin: "<pattern>[A-Z]+</pattern>", want: "PERSON"},
		{name: "list pin", pin: "<list>ORGANISATION</list>", want: "ORGANISATION"},
		{name: "list pin with two class names", pin: "<list>ROLE</list><list>ORGANISATION</list>", wantExample: "ROLE"},
		{name: "list whose first member is no class name", pin: "<list>CLINICIAN</list><list>ORGANISATION</list>", want: "ORGANISATION"},
		{name: "pattern that admits a later class name", pin: "<pattern>ORG.*</pattern>", want: "ORGANISATION"},
		{name: "pattern that admits only GROUP", pin: "<pattern>GR.*</pattern>", want: "GROUP"},
		{name: "pattern that admits only PARTY", pin: "<pattern>PARTY</pattern>", want: "PARTY"},
		{name: "pattern that admits only ACTOR", pin: "<pattern>ACT.*</pattern>", want: "ACTOR"},
		{name: "list that admits no class name", pin: "<list>CLINICIAN</list>", unsatisfiable: true},
		{name: "pattern that admits no class name", pin: "<pattern>[a-z]+</pattern>", unsatisfiable: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, rolePerformerOPT(
				guardSingle("type", guardExistence11, optPrimitive("STRING", "C_STRING", tc.pin))), true)
			for _, opts := range guardOptions() {
				call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
				out, err := instance.Generate(t.Context(), c, opts)
				if tc.unsatisfiable {
					if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
						t.Errorf("%s error = %v, want one wrapping ErrConstraintUnsatisfiable", call, err)
					}
					if out != nil {
						t.Errorf("%s returned %T, want no root", call, out)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s: %v, want a root", call, err)
				}
				role, ok := out.(*rm.Role)
				if !ok {
					t.Fatalf("%s returned %T, want *rm.Role", call, out)
				}
				got := role.Performer.Type
				if !slices.Contains(partyRefTypes, got) {
					t.Errorf("%s: performer type = %q, want one of %v (PARTY_REF Type_validity)", call, got, partyRefTypes)
				}
				if tc.want != "" && got != tc.want {
					t.Errorf("%s: performer type = %q, want %q", call, got, tc.want)
				}
				if tc.wantExample != "" && opts.ValueFill == instance.ExampleFill && got != tc.wantExample {
					t.Errorf("%s: performer type = %q, want the list's first member %q", call, got, tc.wantExample)
				}
				checkBothValidators(t, call, out, c)
			}
		})
	}
}

// TestREQ102_ConstrainedPerformerIDIsRead is the REQ-102 check that the
// template validator reads a ROLE performer's id when the template
// constrains it: a HIER_OBJECT_ID whose value the template pins with a
// C_STRING pattern. The ROLE comes from the template without the id
// constraint, so its performer id can be set by hand to one the pattern
// accepts and to one it rejects. An id the pattern accepts passes both
// validators; one it rejects is reported at its value.
func TestREQ102_ConstrainedPerformerIDIsRead(t *testing.T) {
	withID := compileOPTText(t, rolePerformerOPT(
		guardSingle("id", guardExistence11,
			guardChild("C_COMPLEX_OBJECT", "HIER_OBJECT_ID", "", guardOccurrences11, "",
				guardSingle("value", guardExistence11, optPrimitive("STRING", "C_STRING", "<pattern>[0-9a-f-]+</pattern>"))))), true)
	plain := compileOPTText(t, rolePerformerOPT(), true)
	for _, opts := range guardOptions() {
		call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
		out, err := instance.Generate(t.Context(), plain, opts)
		if err != nil {
			t.Fatalf("%s: %v, want a root", call, err)
		}
		role, ok := out.(*rm.Role)
		if !ok {
			t.Fatalf("%s returned %T, want *rm.Role", call, out)
		}
		role.Performer.ID = &rm.HierObjectID{Value: "6e4b1c2a-0000-4000-8000-00000000abcd"}
		checkBothValidators(t, call+" with an id the pattern accepts", role, withID)

		role.Performer.ID = &rm.HierObjectID{Value: "NOT-HEX"}
		r := validation.Validate(role, withID)
		var atValue bool
		for _, issue := range r.Issues {
			if issue.Path == "/performer/id/value" && issue.Code != "required" {
				atValue = true
			}
		}
		if !atValue {
			t.Errorf("%s with an id the pattern rejects: Validate issues %+v, want one at /performer/id/value that is not required", call, r.Issues)
		}
	}
}
