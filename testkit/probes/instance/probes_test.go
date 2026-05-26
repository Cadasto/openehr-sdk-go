package instanceprobes_test

import (
	"context"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	instanceprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/instance"
)

func compileFixture(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("ParseFile %s: %v", name, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile %s: %v", name, err)
	}
	return c
}

func testComposer() *rm.PartyIdentified {
	name := "Test Composer"
	return &rm.PartyIdentified{Name: &name}
}

func TestProbe027VitalSignsPasses(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	r, err := instanceprobes.Probe027GeneratedValidates(context.Background(), c, instance.Options{
		Territory: "NL",
		Composer:  testComposer(),
	})
	if err != nil {
		t.Fatalf("Probe027: %v", err)
	}
	if r.Status != "pass" {
		t.Errorf("Probe027 status=%q detail=%q", r.Status, r.Detail)
	}
}

// TestProbe027ClinicalNotePasses pins the constraint-driven path on
// the second vendored fixture: clinical_note.opt uses the AOM 1.4
// primitive-short-name shape (DV_DURATION → value → C_PRIMITIVE_OBJECT
// → DURATION → C_DURATION). The C_PRIMITIVE_OBJECT inner-`<item>`
// extraction lands the CDuration constraint on the compiled tree;
// the synthesiser routes via applyPrimitiveExample(child, parentDV)
// so the DV wrapper's primary value channel ("P0D" for CDuration's
// ExampleValue) lands BEFORE validation runs. PROBE-027 round-trips
// clean on both vendored OPTs.
func TestProbe027ClinicalNotePasses(t *testing.T) {
	c := compileFixture(t, "clinical_note")
	r, err := instanceprobes.Probe027GeneratedValidates(context.Background(), c, instance.Options{
		Territory: "NL",
		Composer:  testComposer(),
	})
	if err != nil {
		t.Fatalf("Probe027: %v", err)
	}
	if r.Status != "pass" {
		t.Errorf("Probe027 clinical_note status=%q detail=%q", r.Status, r.Detail)
	}
}

func TestProbe027NilCompiledFails(t *testing.T) {
	_, err := instanceprobes.Probe027GeneratedValidates(context.Background(), nil, instance.Options{})
	if err == nil {
		t.Fatal("expected error for nil compiled template, got nil")
	}
}

func TestProbe027MissingTerritoryFails(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	r, err := instanceprobes.Probe027GeneratedValidates(context.Background(), c, instance.Options{
		Composer: testComposer(),
	})
	if err != nil {
		t.Fatalf("Probe027: %v", err)
	}
	if r.Status != "fail" {
		t.Errorf("missing Territory should fail, got status=%q detail=%q", r.Status, r.Detail)
	}
}
