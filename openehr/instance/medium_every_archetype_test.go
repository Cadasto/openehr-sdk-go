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
// identity the first carries; Large also builds that second use. Required
// builds no optional child, and the content is optional. Every root passes
// the RM floor and the template validator, under every policy, both value
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
		instance.Required: "",
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
