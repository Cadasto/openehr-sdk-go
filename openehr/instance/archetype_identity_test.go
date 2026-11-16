package instance_test

import (
	"maps"
	mrand "math/rand/v2"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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
// under one attribute each get their members: vital_signs leaves its four
// content OBSERVATIONs optional, and every archetype root carries the node
// id at0000, but the identity the skip compares for an archetype root is
// its archetype id. Each root may occur any number of times, so each gets
// two members (Members of a list). The members come in OPT order and the
// composition validates. It holds under both value fills.
func TestREQ107_LargeKeepsEachOptionalContentArchetype(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	want := []string{
		"openEHR-EHR-OBSERVATION.blood_pressure.v1",
		"openEHR-EHR-OBSERVATION.blood_pressure.v1",
		"openEHR-EHR-OBSERVATION.body_temperature.v1",
		"openEHR-EHR-OBSERVATION.body_temperature.v1",
		"openEHR-EHR-OBSERVATION.heart_rate.v1",
		"openEHR-EHR-OBSERVATION.heart_rate.v1",
		"openEHR-EHR-OBSERVATION.respiration.v1",
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

// TestREQ107_MediumKeepsEachOptionalContentArchetype is the REQ-107 check
// that, under Medium, optional archetype roots of different archetypes
// under one attribute each get their member: content holds all four of
// vital_signs' optional content OBSERVATIONs, in OPT order. The
// composition validates. It holds under both value fills.
func TestREQ107_MediumKeepsEachOptionalContentArchetype(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	want := []string{
		"openEHR-EHR-OBSERVATION.blood_pressure.v1",
		"openEHR-EHR-OBSERVATION.body_temperature.v1",
		"openEHR-EHR-OBSERVATION.heart_rate.v1",
		"openEHR-EHR-OBSERVATION.respiration.v1",
	}
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
	opt, err := readOPTFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
	if err != nil {
		t.Fatalf("readOPTFile Corona_Anamnese: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile Corona_Anamnese: %v", err)
	}
	return c
}

// TestREQ107_LargeWritesEachRepeatedOptionalArchetype is the REQ-107
// check that, under Large, an optional archetype root whose archetype id an
// earlier optional sibling shares gets its member too. Corona_Anamnese puts
// four optional SECTION.adhoc.v1 roots, told apart only by name, under
// content after an optional OBSERVATION.story.v1, and the first of those
// sections ('Symptome') holds eight optional symptom_sign_screening roots
// around one optional body_temperature root. So content holds the story,
// twice, since it may occur any number of times (Members of a list), and the
// four sections, each of which occurs once at most, in OPT order, and that
// section holds the nine roots. The composition validates. It holds under
// both value fills.
func TestREQ107_LargeWritesEachRepeatedOptionalArchetype(t *testing.T) {
	const (
		screening = "openEHR-EHR-OBSERVATION.symptom_sign_screening.v0"
		adhoc     = "openEHR-EHR-SECTION.adhoc.v1"
		story     = "openEHR-EHR-OBSERVATION.story.v1"
	)
	c := compileCoronaAnamnese(t)
	wantContent := []string{story, story, adhoc, adhoc, adhoc, adhoc}
	wantSections := []string{"Symptome", "Kontakt", "Risikogebiet", "Allgemeine Angaben"}
	wantItems := []string{screening, screening, screening, screening, "openEHR-EHR-OBSERVATION.body_temperature.v2", screening, screening, screening, screening}
	for _, opts := range corpusOptions(instance.Large) {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			if got := contentIDs(comp.Content); !slices.Equal(got, wantContent) {
				t.Fatalf("Generate(Corona_Anamnese, Large) content = %v, want %v", got, wantContent)
			}
			var names []string
			for _, item := range comp.Content[2:] {
				section, ok := item.(*rm.Section)
				if !ok {
					t.Fatalf("content item is %T, want *rm.Section", item)
				}
				names = append(names, section.Name.GetValue())
			}
			if !slices.Equal(names, wantSections) {
				t.Errorf("content section names = %q, want %q", names, wantSections)
			}
			if got := contentIDs(comp.Content[2].(*rm.Section).Items); !slices.Equal(got, wantItems) {
				t.Errorf("section 'Symptome' items = %v, want %v", got, wantItems)
			}
			noValidationErrors(t, comp, c)
		})
	}
}

// TestREQ107_MediumBuildsEveryArchetypeLargeBuilds is the REQ-107 check
// that, under Medium, a later use of an archetype gets its member where it
// holds an archetype that no sibling with a member of its own holds, and
// none where it adds no archetype. Corona_Anamnese's content uses
// SECTION.adhoc.v1 four times, each holding other entries, so content holds
// the story and the four sections, in OPT order, and Medium builds a member
// of every archetype Large builds. The first section ('Symptome') holds
// eight uses of symptom_sign_screening that hold the same archetypes, so it
// holds the first of them alone, with its body_temperature root, where
// Large builds all eight. The composition validates. It holds under both
// value fills.
func TestREQ107_MediumBuildsEveryArchetypeLargeBuilds(t *testing.T) {
	const adhoc = "openEHR-EHR-SECTION.adhoc.v1"
	c := compileCoronaAnamnese(t)
	large := make(map[string]int)
	archetypeIDCounts(decodedJSON(t, generateComposition(t, c, corpusPolicyOptions(instance.Large))), large)
	wantIDs := slices.Sorted(maps.Keys(large))
	wantContent := []string{"openEHR-EHR-OBSERVATION.story.v1", adhoc, adhoc, adhoc, adhoc}
	wantSections := []string{"Symptome", "Kontakt", "Risikogebiet", "Allgemeine Angaben"}
	wantItems := []string{"openEHR-EHR-OBSERVATION.symptom_sign_screening.v0", "openEHR-EHR-OBSERVATION.body_temperature.v2"}
	for _, opts := range corpusOptions(instance.Medium) {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			medium := make(map[string]int)
			archetypeIDCounts(decodedJSON(t, comp), medium)
			if got := slices.Sorted(maps.Keys(medium)); !slices.Equal(got, wantIDs) {
				t.Errorf("Generate(Corona_Anamnese, Medium) archetypes = %v, want Large's %v", got, wantIDs)
			}
			if got := contentIDs(comp.Content); !slices.Equal(got, wantContent) {
				t.Fatalf("Generate(Corona_Anamnese, Medium) content = %v, want %v", got, wantContent)
			}
			var names []string
			for _, item := range comp.Content[1:] {
				section, ok := item.(*rm.Section)
				if !ok {
					t.Fatalf("content item is %T, want *rm.Section", item)
				}
				names = append(names, section.Name.GetValue())
			}
			if !slices.Equal(names, wantSections) {
				t.Errorf("content section names = %q, want %q", names, wantSections)
			}
			if got := contentIDs(comp.Content[1].(*rm.Section).Items); !slices.Equal(got, wantItems) {
				t.Errorf("section 'Symptome' items = %v, want %v", got, wantItems)
			}
			noValidationErrors(t, comp, c)
		})
	}
}
