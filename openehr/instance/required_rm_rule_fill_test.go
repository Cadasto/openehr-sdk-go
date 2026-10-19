package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestREQ107_RMRuleFillFollowsTheOPTChild is the REQ-107 check of
// Precedence 2 under every policy: an attribute an RM rule needs is written
// as a visit writes it, from its first allowed OPT child, where the policy
// would not visit it. Required visits none of the optional attributes
// below, so the RM rule alone has each one written, and it must take the
// OPT's constraints, not a default of its BMM type: an ELEMENT whose
// null_flavour the OPT prohibits takes the DV_QUANTITY its optional value
// allows, an ELEMENT with no value takes the null flavour its optional
// null_flavour lists, and a PARTY_IDENTIFIED or PARTY_RELATED takes the
// name its optional C_STRING lists. Each case holds in both compile modes,
// and the output passes the RM floor and the template validator.
func TestREQ107_RMRuleFillFollowsTheOPTChild(t *testing.T) {
	// prohibits marks a case whose OPT prohibits the attribute the RM rule
	// writes: the template validator reports that prohibition (REQ-107
	// Exceptions and known gaps), so only the RM floor is checked.
	// nameOnly marks a PARTY_RELATED case: the generator gives its
	// relationship no default, so the RM floor reports Relationship_valid
	// whatever the name, and only the name is checked.
	cases := []struct {
		name      string
		opt       string
		prohibits bool
		nameOnly  bool
		check     func(t *testing.T, out any)
	}{
		{
			name: "ELEMENT value 0..1 over DV_QUANTITY, null_flavour prohibited",
			opt: optTemplate("ELEMENT",
				optOptionalSingleOver("value", optNode("DV_QUANTITY", "")),
				optProhibitedSingle("null_flavour")),
			check: func(t *testing.T, out any) {
				t.Helper()
				el := generatedElement(t, out)
				if _, ok := el.Value.(*rm.DVQuantity); !ok {
					t.Errorf("ELEMENT.value = %#v, want the DV_QUANTITY the OPT allows", el.Value)
				}
				if el.NullFlavour != nil {
					t.Errorf("ELEMENT.null_flavour = %+v, want none", el.NullFlavour)
				}
			},
		},
		{
			name: "ELEMENT null_flavour 0..1 over openehr 253, no value",
			opt: optTemplate("ELEMENT",
				optOptionalSingleOver("null_flavour", optCodedText(terminology.ID, "253"))),
			check: func(t *testing.T, out any) {
				t.Helper()
				el := generatedElement(t, out)
				if el.Value != nil && !rm.IsTypedNil(el.Value) {
					t.Errorf("ELEMENT.value = %#v, want none", el.Value)
				}
				if el.NullFlavour == nil {
					t.Fatal("ELEMENT.null_flavour absent, want the openehr 253 the OPT lists")
				}
				checkOpenEHRCode(t, "ELEMENT.null_flavour", *el.NullFlavour, terminology.NullFlavours, "253")
			},
		},
		{
			name: "PARTY_IDENTIFIED name 0..1 over a C_STRING list",
			opt: optTemplate("OBSERVATION", optSingle("subject", optNode("PARTY_IDENTIFIED", "",
				optOptionalSingleOver("name", optPrimitive("STRING", "C_STRING", "<list>Dr X</list>"))))),
			check: subjectNamed("Dr X"),
		},
		{
			// The OPT prohibits name (existence 0..0) yet lists a child under
			// it; Basic_validity still needs a name, which is written from
			// that child under every policy, not as the text "example".
			name: "PARTY_IDENTIFIED name 0..0 over a C_STRING list",
			opt: optTemplate("OBSERVATION", optSingle("subject", optNode("PARTY_IDENTIFIED", "",
				optProhibitedSingle("name", optPrimitive("STRING", "C_STRING", "<list>Dr X</list>"))))),
			prohibits: true,
			check:     subjectNamed("Dr X"),
		},
		{
			name: "PARTY_RELATED name 0..1 over a C_STRING list",
			opt: optTemplate("OBSERVATION", optSingle("subject", optNode("PARTY_RELATED", "",
				optOptionalSingleOver("name", optPrimitive("STRING", "C_STRING", "<list>Dr X</list>"))))),
			nameOnly: true,
			check:    subjectNamed("Dr X"),
		},
		{
			name: "PARTY_RELATED name 0..0 over a C_STRING list",
			opt: optTemplate("OBSERVATION", optSingle("subject", optNode("PARTY_RELATED", "",
				optProhibitedSingle("name", optPrimitive("STRING", "C_STRING", "<list>Dr X</list>"))))),
			nameOnly: true,
			check:    subjectNamed("Dr X"),
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, policy := range everyPolicy {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", tc.name, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					tc.check(t, out)
					if tc.nameOnly {
						return
					}
					if tc.prohibits {
						checkFloor(t, out)
						return
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}

// generatedElement returns out as the ELEMENT it must be.
func generatedElement(t *testing.T, out any) *rm.Element {
	t.Helper()
	el, ok := out.(*rm.Element)
	if !ok {
		t.Fatalf("generated root is %T, want *rm.Element", out)
	}
	return el
}

// subjectNamed checks that out is an OBSERVATION whose subject is a
// PARTY_IDENTIFIED or PARTY_RELATED named want.
func subjectNamed(want string) func(t *testing.T, out any) {
	return func(t *testing.T, out any) {
		t.Helper()
		obs, ok := out.(*rm.Observation)
		if !ok {
			t.Fatalf("generated root is %T, want *rm.Observation", out)
		}
		var name *string
		switch party := obs.Subject.(type) {
		case *rm.PartyIdentified:
			name = party.Name
		case *rm.PartyRelated:
			name = party.Name
		default:
			t.Fatalf("OBSERVATION.subject is %T, want *rm.PartyIdentified or *rm.PartyRelated", obs.Subject)
		}
		if name == nil {
			t.Fatalf("%T.name absent, want the %q the OPT lists", obs.Subject, want)
		}
		if *name != want {
			t.Errorf("%T.name = %q, want the %q the OPT lists", obs.Subject, *name, want)
		}
	}
}

// checkFloor reports every error the RM floor finds in out.
func checkFloor(t *testing.T, out any) {
	t.Helper()
	for _, iss := range validation.ValidateRM(out).Issues {
		if iss.Severity == validation.Error {
			t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
		}
	}
}
