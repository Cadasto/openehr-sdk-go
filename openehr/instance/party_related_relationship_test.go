package instance_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// partyRelatedSubject is an OBSERVATION whose subject the OPT pins to a
// PARTY_RELATED node with attrs.
func partyRelatedSubject(attrs ...string) string {
	return optTemplate("OBSERVATION", optSingle("subject", optNode("PARTY_RELATED", "", attrs...)))
}

// subjectRelationship returns the relationship of out's subject, which
// must be a PARTY_RELATED under an OBSERVATION.
func subjectRelationship(t *testing.T, out any) rm.DVCodedText {
	t.Helper()
	obs, ok := out.(*rm.Observation)
	if !ok {
		t.Fatalf("generated root is %T, want *rm.Observation", out)
	}
	party, ok := obs.Subject.(*rm.PartyRelated)
	if !ok {
		t.Fatalf("OBSERVATION.subject is %T, want *rm.PartyRelated", obs.Subject)
	}
	return party.Relationship
}

// TestREQ107_PartyRelatedGetsARelationship is the REQ-107 check of the
// RM default for a PARTY_RELATED's relationship: where the OPT gives it
// no value, because it leaves the attribute silent or names it with no
// child, the generator writes openehr 253 with its rubric "unknown" from
// the subject relationship group, so RM Relationship_valid holds. The
// output passes the RM floor and the template validator. It holds under
// every policy, both value fills and both compile modes. A relationship
// the OPT constrains with a child is TestREQ107_RelationshipKeepsTheOPTCode's.
func TestREQ107_PartyRelatedGetsARelationship(t *testing.T) {
	cases := []struct {
		name string
		opt  string
	}{
		{name: "relationship silent", opt: partyRelatedSubject()},
		{name: "relationship named with no child", opt: partyRelatedSubject(optSingle("relationship"))},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					rel := subjectRelationship(t, out)
					checkOpenEHRCode(t, "PARTY_RELATED.relationship", rel, terminology.SubjectRelationship, "253")
					if rel.Value != "unknown" {
						t.Errorf("PARTY_RELATED.relationship.value = %q, want %q", rel.Value, "unknown")
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}

// TestREQ107_RelationshipKeepsTheOPTCode is the REQ-107 check of
// Precedence 1 on a PARTY_RELATED's relationship the OPT constrains with
// a DV_CODED_TEXT child: the generator writes the code the walk builds
// under that child, with that code's rubric in the subject relationship
// group as its text, not the walk's placeholder, and the RM default
// openehr 253 "unknown" is not written over it. With one code listed the
// relationship is openehr 10 "mother". With two listed, 10 and 253, it is
// a listed code with its rubric, and under ExampleFill, whose choice is
// fixed, the walk's 10 "mother", so a 253 written over it fails. The
// output passes the RM floor and the template validator, under every
// policy, both value fills and both compile modes.
func TestREQ107_RelationshipKeepsTheOPTCode(t *testing.T) {
	cases := []struct {
		name  string
		codes []string
		// example is the code the walk writes under ExampleFill.
		example string
	}{
		{name: "one code", codes: []string{"10"}, example: "10"},
		{name: "two codes", codes: []string{"10", "253"}, example: "10"},
	}
	if rubric, _ := terminology.SubjectRelationship.Rubric("10"); rubric != "mother" {
		t.Fatalf("subject relationship rubric of 10 = %q, want %q", rubric, "mother")
	}
	for _, tc := range cases {
		opt := partyRelatedSubject(optSingle("relationship", optCodedText(terminology.ID, tc.codes...)))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					rel := subjectRelationship(t, out)
					code := rel.DefiningCode
					if code.TerminologyID.Value != terminology.ID || !slices.Contains(tc.codes, code.CodeString) {
						t.Errorf("PARTY_RELATED.relationship.defining_code = %+v, want a code of %s in %v", code, terminology.ID, tc.codes)
					}
					if opts.ValueFill == instance.ExampleFill && code.CodeString != tc.example {
						t.Errorf("PARTY_RELATED.relationship code under ExampleFill = %q, want the walk's %q", code.CodeString, tc.example)
					}
					rubric, ok := terminology.SubjectRelationship.Rubric(code.CodeString)
					if !ok {
						t.Fatalf("code %q has no rubric in the subject relationship group", code.CodeString)
					}
					if rel.Value != rubric {
						t.Errorf("PARTY_RELATED.relationship.value = %q, want the group rubric %q of code %s", rel.Value, rubric, code.CodeString)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}
