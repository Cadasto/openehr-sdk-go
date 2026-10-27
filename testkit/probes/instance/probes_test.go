package instanceprobes_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
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

// probe027PassDetail is the Detail of a PROBE-027 pass that generated and
// validated under every policy the probe promises: Large, Medium and
// Required.
func probe027PassDetail() string {
	names := []string{instance.Large.String(), instance.Medium.String(), instance.Required.String()}
	return "validated clean under " + strings.Join(names, ", ")
}

// TestProbe027VitalSignsPasses pins a PROBE-027 pass on vital_signs.opt under
// every policy the probe promises (REQ-107).
func TestProbe027VitalSignsPasses(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	r, err := instanceprobes.Probe027GeneratedValidates(context.Background(), c, instance.Options{
		Territory: "NL",
		Composer:  testComposer(),
	})
	if err != nil {
		t.Fatalf("Probe027: %v", err)
	}
	if want := probe027PassDetail(); r.Status != "pass" || r.Detail != want {
		t.Errorf("Probe027 status=%q detail=%q, want status=\"pass\" detail=%q", r.Status, r.Detail, want)
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
// clean on both vendored OPTs, under every policy it promises (REQ-107).
func TestProbe027ClinicalNotePasses(t *testing.T) {
	c := compileFixture(t, "clinical_note")
	r, err := instanceprobes.Probe027GeneratedValidates(context.Background(), c, instance.Options{
		Territory: "NL",
		Composer:  testComposer(),
	})
	if err != nil {
		t.Fatalf("Probe027: %v", err)
	}
	if want := probe027PassDetail(); r.Status != "pass" || r.Detail != want {
		t.Errorf("Probe027 clinical_note status=%q detail=%q, want status=\"pass\" detail=%q", r.Status, r.Detail, want)
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

// TestProbe027_RealWorldCorpus extends PROBE-027 to the real-world OPTs
// filed against REQ-107. The assertion below is the Large policy;
// TestREQ107_CorpusRatchet runs the other settings over the same OPTs.
func TestProbe027_RealWorldCorpus(t *testing.T) {
	opts := instance.Options{
		Policy:    instance.Large,
		Territory: "NL",
		Composer:  testComposer(),
	}
	for _, name := range []string{"Referral Request.v1", "Demonstration.v1", "social"} {
		t.Run(name, func(t *testing.T) {
			c := compileRealWorldFixture(t, name)
			out, err := instance.Generate(context.Background(), c, opts)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			comp, err := instance.AsComposition(out)
			if err != nil {
				t.Fatalf("AsComposition: %v", err)
			}
			result := validation.ValidateComposition(comp, c)
			if !result.OK {
				t.Fatalf("ValidateComposition: %d issues (first: %+v)", len(result.Issues), result.Issues[0])
			}
		})
	}
}

func compileRealWorldFixture(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("ReadFile %s: %v", name, err)
	}
	opt, err := fixtures.ParseOPTBytes(raw)
	if err != nil {
		t.Fatalf("ParseOPTBytes %s: %v", name, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile %s: %v", name, err)
	}
	return c
}
