package instance_test

import (
	mrand "math/rand/v2"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// minimalOptions are the Minimal settings a corpus composition case runs
// under, one per value fill.
func minimalOptions() []instance.Options {
	var out []instance.Options
	for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
		out = append(out, instance.Options{
			Policy:      instance.Minimal,
			ValueFill:   fill,
			ValueSource: mrand.NewPCG(1, 2),
			Territory:   "NL",
			Composer:    testComposer(),
			Now:         defaultsNow,
		})
	}
	return out
}

// generateComposition runs Generate on c under opts and returns the
// COMPOSITION it builds.
func generateComposition(t *testing.T, c *templatecompile.Compiled, opts instance.Options) *rm.Composition {
	t.Helper()
	out, err := instance.Generate(t.Context(), c, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	return comp
}

// contentIDs returns the archetype_node_id of each member of content, in
// order.
func contentIDs[T any](content []T) []string {
	ids := make([]string, 0, len(content))
	for _, item := range content {
		ids = append(ids, nodeID(item))
	}
	return ids
}

// noValidationErrors reports every error the template validator finds in
// comp.
func noValidationErrors(t *testing.T, comp *rm.Composition, c *templatecompile.Compiled) {
	t.Helper()
	for _, iss := range validation.ValidateComposition(comp, c).Issues {
		if iss.Severity == validation.Error {
			t.Errorf("ValidateComposition: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
		}
	}
}

// TestREQ107_MinimalKeepsEachOptionalContentArchetype is the REQ-107 check
// that, under Minimal, optional archetype roots of different archetypes
// under one attribute each get their member: vital_signs leaves its four
// content OBSERVATIONs optional, and every archetype root carries the node
// id at0000, but the identity the skip compares for an archetype root is
// its archetype id. The members come in OPT order and the composition
// validates. It holds under both value fills.
func TestREQ107_MinimalKeepsEachOptionalContentArchetype(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	want := []string{
		"openEHR-EHR-OBSERVATION.blood_pressure.v1",
		"openEHR-EHR-OBSERVATION.body_temperature.v1",
		"openEHR-EHR-OBSERVATION.heart_rate.v1",
		"openEHR-EHR-OBSERVATION.respiration.v1",
	}
	for _, opts := range minimalOptions() {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			if got := contentIDs(comp.Content); !slices.Equal(got, want) {
				t.Errorf("Generate(vital_signs, Minimal) content = %v, want %v", got, want)
			}
			noValidationErrors(t, comp, c)
		})
	}
}

// TestREQ107_MinimalSkipsARepeatedOptionalArchetype is the REQ-107 check
// that, under Minimal, an optional archetype root whose archetype id an
// earlier optional sibling shares still gets no member. Corona_Anamnese
// puts four optional SECTION.adhoc.v1 roots, told apart only by name,
// under content after an optional OBSERVATION.story.v1, and the first of
// those sections ('Symptome') holds eight optional symptom_sign_screening
// roots around one optional body_temperature root. So content holds the
// story and the first section alone, and that section holds the first
// screening and the temperature. The composition validates. It holds under
// both value fills.
func TestREQ107_MinimalSkipsARepeatedOptionalArchetype(t *testing.T) {
	opt, err := template.ParseFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
	if err != nil {
		t.Fatalf("ParseFile Corona_Anamnese: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile Corona_Anamnese: %v", err)
	}
	wantContent := []string{"openEHR-EHR-OBSERVATION.story.v1", "openEHR-EHR-SECTION.adhoc.v1"}
	wantItems := []string{"openEHR-EHR-OBSERVATION.symptom_sign_screening.v0", "openEHR-EHR-OBSERVATION.body_temperature.v2"}
	for _, opts := range minimalOptions() {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			if got := contentIDs(comp.Content); !slices.Equal(got, wantContent) {
				t.Fatalf("Generate(Corona_Anamnese, Minimal) content = %v, want %v", got, wantContent)
			}
			section, ok := comp.Content[1].(*rm.Section)
			if !ok {
				t.Fatalf("content[1] is %T, want *rm.Section", comp.Content[1])
			}
			if got := section.Name.GetValue(); got != "Symptome" {
				t.Errorf("content[1] name = %q, want the first adhoc section, %q", got, "Symptome")
			}
			if got := contentIDs(section.Items); !slices.Equal(got, wantItems) {
				t.Errorf("section 'Symptome' items = %v, want %v", got, wantItems)
			}
			noValidationErrors(t, comp, c)
		})
	}
}
