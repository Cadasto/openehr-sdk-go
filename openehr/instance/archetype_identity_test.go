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

// corpusOptions are the settings a corpus composition case runs under
// policy, one per value fill.
func corpusOptions(policy instance.Policy) []instance.Options {
	var out []instance.Options
	for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
		out = append(out, instance.Options{
			Policy:      policy,
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

// TestREQ107_LargeKeepsEachOptionalContentArchetype is the REQ-107 check
// that, under Large, optional archetype roots of different archetypes
// under one attribute each get their member: vital_signs leaves its four
// content OBSERVATIONs optional, and every archetype root carries the node
// id at0000, but the identity the skip compares for an archetype root is
// its archetype id. The members come in OPT order and the composition
// validates. It holds under both value fills.
func TestREQ107_LargeKeepsEachOptionalContentArchetype(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	want := []string{
		"openEHR-EHR-OBSERVATION.blood_pressure.v1",
		"openEHR-EHR-OBSERVATION.body_temperature.v1",
		"openEHR-EHR-OBSERVATION.heart_rate.v1",
		"openEHR-EHR-OBSERVATION.respiration.v1",
	}
	for _, opts := range corpusOptions(instance.Large) {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			if got := contentIDs(comp.Content); !slices.Equal(got, want) {
				t.Errorf("Generate(vital_signs, Large) content = %v, want %v", got, want)
			}
			noValidationErrors(t, comp, c)
		})
	}
}

// TestREQ107_MediumKeepsOneOptionalContentArchetype is the REQ-107 check
// that, under Medium, an optional archetype root after the first optional
// archetype root of an attribute gets no member of its own, whatever its
// archetype: of vital_signs' four optional content OBSERVATIONs, content
// holds the first alone. The composition validates. It holds under both
// value fills.
func TestREQ107_MediumKeepsOneOptionalContentArchetype(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	want := []string{"openEHR-EHR-OBSERVATION.blood_pressure.v1"}
	for _, opts := range corpusOptions(instance.Medium) {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			if got := contentIDs(comp.Content); !slices.Equal(got, want) {
				t.Errorf("Generate(vital_signs, Medium) content = %v, want %v", got, want)
			}
			noValidationErrors(t, comp, c)
		})
	}
}

// compileCoronaAnamnese compiles the Corona_Anamnese web-template OPT.
func compileCoronaAnamnese(t *testing.T) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
	if err != nil {
		t.Fatalf("ParseFile Corona_Anamnese: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile Corona_Anamnese: %v", err)
	}
	return c
}

// TestREQ107_LargeSkipsARepeatedOptionalArchetype is the REQ-107 check
// that, under Large, an optional archetype root whose archetype id an
// earlier optional sibling shares still gets no member. Corona_Anamnese
// puts four optional SECTION.adhoc.v1 roots, told apart only by name,
// under content after an optional OBSERVATION.story.v1, and the first of
// those sections ('Symptome') holds eight optional symptom_sign_screening
// roots around one optional body_temperature root. So content holds the
// story and the first section alone, and that section holds the first
// screening and the temperature. The composition validates. It holds under
// both value fills.
func TestREQ107_LargeSkipsARepeatedOptionalArchetype(t *testing.T) {
	c := compileCoronaAnamnese(t)
	wantContent := []string{"openEHR-EHR-OBSERVATION.story.v1", "openEHR-EHR-SECTION.adhoc.v1"}
	wantItems := []string{"openEHR-EHR-OBSERVATION.symptom_sign_screening.v0", "openEHR-EHR-OBSERVATION.body_temperature.v2"}
	for _, opts := range corpusOptions(instance.Large) {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			if got := contentIDs(comp.Content); !slices.Equal(got, wantContent) {
				t.Fatalf("Generate(Corona_Anamnese, Large) content = %v, want %v", got, wantContent)
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

// TestREQ107_MediumSkipsEveryLaterOptionalArchetype is the REQ-107 check
// that, under Medium, the optional archetype roots after the first one of
// an attribute get no member of their own, of another archetype or of the
// same: Corona_Anamnese's content holds the optional OBSERVATION.story.v1
// alone, and none of the four optional SECTION.adhoc.v1 roots after it.
// The composition validates. It holds under both value fills.
func TestREQ107_MediumSkipsEveryLaterOptionalArchetype(t *testing.T) {
	c := compileCoronaAnamnese(t)
	want := []string{"openEHR-EHR-OBSERVATION.story.v1"}
	for _, opts := range corpusOptions(instance.Medium) {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			if got := contentIDs(comp.Content); !slices.Equal(got, want) {
				t.Errorf("Generate(Corona_Anamnese, Medium) content = %v, want %v", got, want)
			}
			noValidationErrors(t, comp, c)
		})
	}
}
