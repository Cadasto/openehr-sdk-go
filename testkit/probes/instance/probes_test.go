package instanceprobes_test

import (
	"context"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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
// validated under every policy the probe promises (Large, Medium and
// Required), each with both value fills.
func probe027PassDetail() string {
	var names []string
	for _, p := range []instance.Policy{instance.Large, instance.Medium, instance.Required} {
		for _, f := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			names = append(names, p.String()+"/"+f.String())
		}
	}
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

// TestProbe027FailsWhenAValidatorRefuses pins both checks PROBE-027 applies
// to each generated instance: when ValidateComposition or ValidateRM refuses
// it, the probe fails and names the policy/fill pair and the check, so
// dropping either check fails this test (REQ-107).
func TestProbe027FailsWhenAValidatorRefuses(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	pass := func(*rm.Composition, *templatecompile.Compiled) validation.Result { return validation.Result{OK: true} }
	passRM := func(*rm.Composition) validation.Result { return validation.Result{OK: true} }
	refuse := validation.Result{OK: false, Issues: []validation.Issue{{Code: "stub", Path: "/", Detail: "refused by the test"}}}
	cases := []struct {
		name        string
		composition func(*rm.Composition, *templatecompile.Compiled) validation.Result
		rmCheck     func(*rm.Composition) validation.Result
		want        string
	}{
		{
			name:        "ValidateComposition refuses",
			composition: func(*rm.Composition, *templatecompile.Compiled) validation.Result { return refuse },
			rmCheck:     passRM,
			want:        "[large/example] ValidateComposition produced 1 issue(s)",
		},
		{
			name:        "ValidateRM refuses",
			composition: pass,
			rmCheck:     func(*rm.Composition) validation.Result { return refuse },
			want:        "[large/example] ValidateRM produced 1 issue(s)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := instanceprobes.Probe027WithValidators(context.Background(), c, instance.Options{
				Territory: "NL",
				Composer:  testComposer(),
			}, tc.composition, tc.rmCheck)
			if err != nil {
				t.Fatalf("Probe027: %v", err)
			}
			if r.Status != "fail" || !strings.Contains(r.Detail, tc.want) {
				t.Errorf("Probe027 status=%q detail=%q, want status=\"fail\" and a detail naming %q", r.Status, r.Detail, tc.want)
			}
		})
	}
}

// countingSource is a math/rand/v2 Source that counts its draws.
type countingSource struct {
	src   rand.Source
	draws int
}

func (s *countingSource) Uint64() uint64 {
	s.draws++
	return s.src.Uint64()
}

// TestProbe027DrawsFromTheCallersValueSource pins PROBE-027's RandomFill
// runs to the caller's Options.ValueSource, which its doc promises they draw
// from (REQ-107).
func TestProbe027DrawsFromTheCallersValueSource(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	src := &countingSource{src: rand.NewPCG(1, 1)}
	r, err := instanceprobes.Probe027GeneratedValidates(context.Background(), c, instance.Options{
		Territory:   "NL",
		Composer:    testComposer(),
		ValueSource: src,
	})
	if err != nil {
		t.Fatalf("Probe027: %v", err)
	}
	if r.Status != "pass" || src.draws == 0 {
		t.Errorf("Probe027 with a caller's ValueSource: status=%q, %d draw(s) from it, want a pass that draws from it", r.Status, src.draws)
	}
}

// TestProbe027IsReproducibleWithoutAValueSource pins the fixed seed
// PROBE-027's RandomFill runs fall back to when the caller passes no
// ValueSource, which its doc promises makes the probe reproducible
// (REQ-107): two runs with the clock and the uids pinned check the same
// instances.
func TestProbe027IsReproducibleWithoutAValueSource(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	pass := func(*rm.Composition, *templatecompile.Compiled) validation.Result { return validation.Result{OK: true} }
	run := func() []string {
		var instances []string
		capture := func(comp *rm.Composition) validation.Result {
			out, err := canjson.Marshal(comp)
			if err != nil {
				t.Fatalf("canjson.Marshal of a generated instance: %v", err)
			}
			instances = append(instances, string(out))
			return validation.Result{OK: true}
		}
		r, err := instanceprobes.Probe027WithValidators(t.Context(), c, instance.Options{
			Territory: "NL",
			Composer:  testComposer(),
			Now:       time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			UIDSource: instance.UIDsFrom(rand.NewPCG(1, 1)),
		}, pass, capture)
		if err != nil || r.Status != "pass" {
			t.Fatalf("Probe027: status=%q detail=%q err=%v, want a pass", r.Status, r.Detail, err)
		}
		return instances
	}
	first, second := run(), run()
	if len(first) != len(second) {
		t.Fatalf("Probe027 checked %d instances, then %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("Probe027 instance %d differs between two runs with no ValueSource", i)
		}
	}
}
