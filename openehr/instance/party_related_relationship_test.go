package instance_test

import (
	"fmt"
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
// the OPT constrains with a child is not built here: the template-instance
// writer cannot write it, so TestREQ107_RelationshipDefaultYieldsToTheOPT
// checks that case on the default alone.
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
