package instance_test

import (
	"fmt"
	"testing"

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
// validators accept, at either policy and either value fill. With no pins,
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
