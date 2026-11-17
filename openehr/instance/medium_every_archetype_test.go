package instance_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// archetypeShape renders the archetype node ids of a COMPOSITION's content,
// in order, each SECTION followed by its own items in parentheses.
func archetypeShape(v any) string {
	comp, ok := v.(*rm.Composition)
	if !ok {
		return fmt.Sprintf("%T", v)
	}
	return contentShape(comp.Content)
}

func contentShape[T any](items []T) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = nodeID(item)
		if s, ok := any(item).(*rm.Section); ok {
			parts[i] += "(" + contentShape(s.Items) + ")"
		}
	}
	return strings.Join(parts, ",")
}

// TestREQ107_MediumBuildsEveryOptionalArchetype is the REQ-107 check that
// Medium builds one member of every optional archetype root under every
// attribute it visits. The content lists two optional OBSERVATION roots of
// different archetypes, a second use of the first one, and an optional
// SECTION whose items list two more. Medium builds one member of each
// archetype, the nested ones included, and none of the second use, whose
// identity the first carries and which holds no other archetype; Large
// also builds that second use. Required builds the entries and the SECTION
// Medium builds, each entry to its mandatory parts. Every root passes the
// RM floor and the template validator, under every policy, both value
// fills and both compile modes.
func TestREQ107_MediumBuildsEveryOptionalArchetype(t *testing.T) {
	const (
		first   = "openEHR-EHR-OBSERVATION.first.v1"
		second  = "openEHR-EHR-OBSERVATION.second.v1"
		nestedA = "openEHR-EHR-OBSERVATION.nested_a.v1"
		nestedB = "openEHR-EHR-OBSERVATION.nested_b.v1"
		section = "openEHR-EHR-SECTION.adhoc.v1"
	)
	opt := guardOPT(guardCompositionID, guardMultiple("content", guardExistence01,
		undescribedObservation(first, guardOccurrences01, describedHistory),
		undescribedObservation(second, guardOccurrences01, describedHistory),
		undescribedObservation(first, guardOccurrences01, describedHistory),
		guardChild("C_ARCHETYPE_ROOT", "SECTION", "at0000", guardOccurrences01, section,
			guardMultiple("items", guardExistence01,
				undescribedObservation(nestedA, guardOccurrences01, describedHistory),
				undescribedObservation(nestedB, guardOccurrences01, describedHistory)))))
	nested := section + "(" + nestedA + "," + nestedB + ")"
	want := map[instance.Policy]string{
		instance.Large:    strings.Join([]string{first, second, first, nested}, ","),
		instance.Medium:   strings.Join([]string{first, second, nested}, ","),
		instance.Required: strings.Join([]string{first, second, nested}, ","),
	}
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range guardOptions() {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				if got := archetypeShape(out); got != want[opts.Policy] {
					t.Errorf("Generate built %q, want %q", got, want[opts.Policy])
				}
				noFloorErrors(t, out)
				for _, iss := range templateErrors(out, c) {
					t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
				}
			})
		}
	}
}

// TestREQ107_MediumSkippedSiblingCarriesNoIdentity is the REQ-107 check of
// Sibling identity that an optional sibling that gets no member of its
// own, because it needs undescribed content, carries no identity for the
// siblings after it. The content lists two optional uses of one
// OBSERVATION archetype, the first with a protocol that admits only a
// slot, so under Medium and Required, as under Large, the second gets its
// member, which has no protocol. Every root passes the RM floor and the
// template validator, under every policy, both value fills and both
// compile modes.
func TestREQ107_MediumSkippedSiblingCarriesNoIdentity(t *testing.T) {
	slottedProtocol := optSingle("protocol", optSlot("ITEM_TREE", "at0002"))
	opt := guardOPT(guardCompositionID, guardMultiple("content", guardExistence01,
		undescribedObservation(usedObservationID, guardOccurrences01, describedHistory, slottedProtocol),
		undescribedObservation(usedObservationID, guardOccurrences01, describedHistory)))
	want := map[instance.Policy]string{
		instance.Large:    usedObservationID,
		instance.Medium:   usedObservationID,
		instance.Required: usedObservationID,
	}
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range guardOptions() {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				if got := archetypeShape(out); got != want[opts.Policy] {
					t.Fatalf("Generate built %q, want %q", got, want[opts.Policy])
				}
				if comp := out.(*rm.Composition); len(comp.Content) > 0 {
					if obs, ok := comp.Content[0].(*rm.Observation); !ok || obs.Protocol != nil && !rm.IsTypedNil(obs.Protocol) {
						t.Errorf("content member is %#v, want an OBSERVATION with no protocol, the second use's", comp.Content[0])
					}
				}
				noFloorErrors(t, out)
				for _, iss := range templateErrors(out, c) {
					t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
				}
			})
		}
	}
}

// TestREQ107_MediumBuildsALaterUseThatAddsAnArchetype is the REQ-107 check
// of Sibling identity that, under Medium, an optional child whose identity
// a sibling with a member of its own carries still gets its member where
// it holds an archetype root that no such sibling holds: a required
// sibling, before or after it, or an earlier optional one that gets a
// member. A root that needs undescribed content, which Medium does not
// build, is not held. A later use that adds no archetype gets none, so
// Large still builds more. The cases are uses of one SECTION archetype
// under a composition's content, which Required builds as Medium does,
// since each use it builds leads to an entry, and CLUSTERs of one node id
// under a CLUSTER's items, of which Required builds only the required one.
// Every root passes the RM floor and the template validator, under every
// policy, both value fills and both compile modes.
func TestREQ107_MediumBuildsALaterUseThatAddsAnArchetype(t *testing.T) {
	const (
		section = "openEHR-EHR-SECTION.adhoc.v1"
		obsA    = "openEHR-EHR-OBSERVATION.a.v1"
		obsB    = "openEHR-EHR-OBSERVATION.b.v1"
		obsC    = "openEHR-EHR-OBSERVATION.c.v1"
		obsD    = "openEHR-EHR-OBSERVATION.d.v1"
		inner   = "openEHR-EHR-CLUSTER.inner.v1"
	)
	slottedProtocol := optSingle("protocol", optSlot("ITEM_TREE", "at0002"))
	use := func(entries ...string) string {
		return guardChild("C_ARCHETYPE_ROOT", "SECTION", "at0000", guardOccurrences01, section,
			guardMultiple("items", guardExistence01, entries...))
	}
	optional := func(id string, attrs ...string) string {
		return undescribedObservation(id, guardOccurrences01, attrs...)
	}
	cluster := func(items ...string) string {
		return guardChild("C_COMPLEX_OBJECT", "CLUSTER", "at0001", guardOccurrences01, "", optMultiple("items", items...))
	}
	cases := []struct {
		name  string
		opt   string
		shape func(any) string
		// want is the shape per policy.
		want map[instance.Policy]string
	}{
		{
			// The second use adds b, the third adds nothing, the fourth
			// holds only c, which needs undescribed content, and the fifth
			// holds d, which the required root after it holds.
			name: "uses of one section archetype",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence01,
				use(optional(obsA, describedHistory)),
				use(optional(obsB, describedHistory)),
				use(optional(obsA, describedHistory), optional(obsB, describedHistory)),
				use(optional(obsC, describedHistory, slottedProtocol)),
				use(optional(obsD, describedHistory)),
				undescribedObservation(obsD, guardOccurrences11, describedHistory))),
			shape: archetypeShape,
			want: map[instance.Policy]string{
				instance.Large: strings.Join([]string{
					section + "(" + obsA + ")", section + "(" + obsB + ")", section + "(" + obsA + "," + obsB + ")",
					section + "()", section + "(" + obsD + ")", obsD,
				}, ","),
				instance.Medium:   strings.Join([]string{section + "(" + obsA + ")", section + "(" + obsB + ")", obsD}, ","),
				instance.Required: strings.Join([]string{section + "(" + obsA + ")", section + "(" + obsB + ")", obsD}, ","),
			},
		},
		{
			// The second CLUSTER adds the inner root; the third adds none.
			name: "clusters of one node id",
			opt: optTemplate("CLUSTER", optMultiple("items",
				cluster(optNode("ELEMENT", "at0002")),
				cluster(optArchetypeRoot("CLUSTER", inner, optMultiple("items", optNode("ELEMENT", "at0003")))),
				cluster(optNode("ELEMENT", "at0004")))),
			shape: undescribedShape,
			want: map[instance.Policy]string{
				instance.Large:    "at0001(at0002),at0001(" + inner + "(at0003)),at0001(at0004)",
				instance.Medium:   "at0001(at0002),at0001(" + inner + "(at0003))",
				instance.Required: "at0001(at0002)",
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := tc.shape(out); got != tc.want[opts.Policy] {
						t.Errorf("Generate built %q, want %q", got, tc.want[opts.Policy])
					}
					noFloorErrors(t, out)
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}
