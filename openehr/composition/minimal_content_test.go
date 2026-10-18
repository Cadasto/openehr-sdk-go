package composition_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// heartRatePath is the Rate DV_QUANTITY of the vital_signs template's
// heart-rate OBSERVATION, the third of its four optional content
// archetypes.
const heartRatePath = "/content[openEHR-EHR-OBSERVATION.heart_rate.v1]/data/events[at0003]/data/items[at0004]/value"

// TestREQ101_REQ107_SetReachesEachOptionalContentArchetype checks that the
// skeleton a Builder starts from holds a member of each optional content
// archetype the template names, not only the first: vital_signs leaves its
// four content OBSERVATIONs optional, and every archetype root carries the
// node id at0000. So a value set at a path into the heart-rate observation
// lands there, Build returns no error, and the validator finds nothing the
// bare skeleton did not have.
func TestREQ101_REQ107_SetReachesEachOptionalContentArchetype(t *testing.T) {
	t.Parallel()
	c := compileFixture(t, "vital_signs")
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	skeleton, err := b.Build()
	if err != nil {
		t.Fatalf("Build of the bare skeleton: %v", err)
	}
	before := validation.ValidateComposition(skeleton, c)

	rate := &rm.DVQuantity{Magnitude: 80, Units: "/min"}
	if err := b.Set(heartRatePath, rate); err != nil {
		t.Fatalf("Set(%s) = %v, want nil", heartRatePath, err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build after Set(%s) = %v, want nil", heartRatePath, err)
	}

	got, err := rmpath.ItemAtPath(comp, heartRatePath)
	if err != nil {
		t.Fatalf("ItemAtPath(%s) on the built composition: %v", heartRatePath, err)
	}
	if got != rate {
		t.Errorf("built composition holds %#v at %s, want the value Set was given, %#v", got, heartRatePath, rate)
	}
	after := validation.ValidateComposition(comp, c)
	if added := newIssues(before, after); len(added) > 0 {
		t.Errorf("ValidateComposition after Set(%s) reports new findings %q, want none", heartRatePath, added)
	}
}
