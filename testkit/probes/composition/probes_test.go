package compositionprobes_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	compositionprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/composition"
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

const (
	systolicPath  = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"
	diastolicPath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0005]/value"
)

// vitalSignsOptions returns the builder options every vital_signs run in
// this file uses.
func vitalSignsOptions() []composition.Option {
	return []composition.Option{
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	}
}

// vitalSignsAssignments sets systolic and diastolic on vital_signs.opt and
// names the fragments each must leave in the marshalled output.
func vitalSignsAssignments() []compositionprobes.Assignment {
	return []compositionprobes.Assignment{
		{
			Path: systolicPath,
			Apply: func(b *composition.Builder) error {
				return b.SetQuantity(systolicPath, 120, "mm[Hg]")
			},
			WireFragments: [][]byte{
				[]byte(`"magnitude":120`),
				[]byte(`"units":"mm[Hg]"`),
			},
		},
		{
			Path: diastolicPath,
			Apply: func(b *composition.Builder) error {
				return b.SetQuantity(diastolicPath, 80, "mm[Hg]")
			},
			WireFragments: [][]byte{
				[]byte(`"magnitude":80`),
			},
		},
	}
}

// TestProbe023VitalSignsPasses verifies the canonical authoring
// flow: NewBuilder over vital_signs.opt → SetQuantity at systolic
// and diastolic → Build → canjson.Marshal → fragments present.
//
// REQ-101: PROBE-023 builds with NewBuilder and Set, then checks the values
// survive a canjson round trip.
func TestProbe023VitalSignsPasses(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	r, err := compositionprobes.Probe023BuilderRoundTrip(context.Background(), c, vitalSignsOptions(), vitalSignsAssignments())
	if err != nil {
		t.Fatalf("Probe023: %v", err)
	}
	if r.Status != "pass" {
		t.Errorf("Probe023 status=%q detail=%q", r.Status, r.Detail)
	}
}

// TestProbe023RoundTripLossFails pins the second leg of PROBE-023 (REQ-101):
// a decode and re-marshal that loses one fragment fails the probe and names
// that fragment, although the first marshal still holds it.
func TestProbe023RoundTripLossFails(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	lost := []byte(`"magnitude":80`)
	dropOne := func(payload []byte) ([]byte, error) {
		out, err := compositionprobes.CanjsonRoundTrip(payload)
		if err != nil {
			return nil, err
		}
		if !bytes.Contains(out, lost) {
			t.Errorf("the round trip lost %q before the test dropped it", lost)
		}
		return bytes.ReplaceAll(out, lost, []byte(`"magnitude":81`)), nil
	}
	r, err := compositionprobes.Probe023WithRoundTrip(context.Background(), c, vitalSignsOptions(), vitalSignsAssignments(), dropOne)
	if err != nil {
		t.Fatalf("Probe023: %v", err)
	}
	want := fmt.Sprintf("Set@%s: round-trip output missing fragment %q", diastolicPath, lost)
	if r.Status != "fail" || r.Detail != want {
		t.Errorf("Probe023 with %q dropped on the round trip: status=%q detail=%q, want status=\"fail\" detail=%q",
			lost, r.Status, r.Detail, want)
	}
}

func TestProbe023NilCompiledFails(t *testing.T) {
	_, err := compositionprobes.Probe023BuilderRoundTrip(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil compiled template")
	}
}

// TestProbe023MissingComposerFails confirms the underlying
// NewBuilder enforcement is propagated through the probe.
func TestProbe023MissingComposerFails(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	r, err := compositionprobes.Probe023BuilderRoundTrip(
		context.Background(),
		c,
		[]composition.Option{composition.WithTerritory("NL")},
		nil,
	)
	if err != nil {
		t.Fatalf("Probe023: %v", err)
	}
	if r.Status != "fail" {
		t.Errorf("missing Composer should fail, got status=%q detail=%q", r.Status, r.Detail)
	}
}
