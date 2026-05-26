package compositionprobes_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	compositionprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/composition"
)

func optPath(t *testing.T, name string) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "openehr", "template", "testdata")
	return filepath.Join(root, name)
}

func compileFixture(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(optPath(t, name))
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

// TestProbe023VitalSignsPasses verifies the canonical authoring
// flow: NewBuilder over vital_signs.opt → SetQuantity at systolic
// and diastolic → Build → canjson.Marshal → fragments present.
func TestProbe023VitalSignsPasses(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	systolic := "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"
	diastolic := "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0005]/value"
	r, err := compositionprobes.Probe023BuilderRoundTrip(
		context.Background(),
		c,
		[]composition.Option{
			composition.WithTerritory("NL"),
			composition.WithComposer(testComposer()),
		},
		[]compositionprobes.Assignment{
			{
				Path: systolic,
				Apply: func(b *composition.Builder) error {
					return b.SetQuantity(systolic, 120, "mm[Hg]")
				},
				WireFragments: [][]byte{
					[]byte(`"magnitude":120`),
					[]byte(`"units":"mm[Hg]"`),
				},
			},
			{
				Path: diastolic,
				Apply: func(b *composition.Builder) error {
					return b.SetQuantity(diastolic, 80, "mm[Hg]")
				},
				WireFragments: [][]byte{
					[]byte(`"magnitude":80`),
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Probe023: %v", err)
	}
	if r.Status != "pass" {
		t.Errorf("Probe023 status=%q detail=%q", r.Status, r.Detail)
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
	c := compileFixture(t, "vital_signs.opt")
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
