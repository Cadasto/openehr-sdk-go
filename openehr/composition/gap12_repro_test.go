package composition_test

import (
	"context"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func parseGAP12Fixture(t *testing.T, name string) *template.OperationalTemplate {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("ReadFile %s: %v", name, err)
	}
	opt, err := fixtures.ParseOPTBytes(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return opt
}

func testComposerGap12() *rm.PartyIdentified {
	name := "Test Composer"
	return &rm.PartyIdentified{Name: &name}
}

// TestNewSkeleton_GAP12_passes asserts SDK-GAP-12 corpus OPTs compile,
// synthesise via NewSkeleton, and validate clean. See
// docs/plans/2026-06-19-sdk-gap-12-newskeleton.md.
func TestNewSkeleton_GAP12_passes(t *testing.T) {
	for _, name := range []string{"Referral Request.v1", "Demonstration.v1", "social"} {
		t.Run(name, func(t *testing.T) {
			opt := parseGAP12Fixture(t, name)
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			comp, err := composition.NewSkeleton(context.Background(), c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposerGap12()),
			)
			if err != nil {
				t.Fatalf("NewSkeleton: %v", err)
			}
			res := validation.ValidateComposition(comp, c)
			if !res.OK {
				t.Fatalf("ValidateComposition: %d issues (first: %+v)", len(res.Issues), res.Issues[0])
			}
		})
	}
}
