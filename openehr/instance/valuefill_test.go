package instance_test

import (
	"bytes"
	"context"
	"fmt"
	mrand "math/rand/v2"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// counterUID yields deterministic uids so a reproducibility check
// isolates the value-fill seam: with the clock and uids pinned, the only
// thing that can vary between two runs is the sampled leaf values.
func counterUID() func() *rm.HierObjectID {
	n := 0
	return func() *rm.HierObjectID {
		n++
		return &rm.HierObjectID{Value: fmt.Sprintf("uid-%04d", n)}
	}
}

// TestGenerateRandomFillReproducibleAndValid covers REQ-107 end to
// end: RandomFill output (a) validates clean, (b) is byte-reproducible
// under a fixed ValueSource, and (c) varies from the fixed ExampleFill.
func TestGenerateRandomFillReproducibleAndValid(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	name := "Test Composer"

	gen := func(vf instance.ValueFill, src mrand.Source) []byte {
		t.Helper()
		out, err := instance.Generate(context.Background(), c, instance.Options{
			Policy:      instance.Example,
			Territory:   "NL",
			Composer:    &rm.PartyIdentified{Name: &name},
			Now:         now,
			UIDSource:   counterUID(),
			ValueFill:   vf,
			ValueSource: src,
		})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		comp, err := instance.AsComposition(out)
		if err != nil {
			t.Fatalf("AsComposition: %v", err)
		}
		if r := validation.ValidateComposition(comp, c); !r.OK {
			for _, iss := range r.Issues {
				t.Logf("%s @ %s — %s", iss.Code, iss.Path, iss.Detail)
			}
			t.Fatalf("ValueFill=%s output failed validation: %d issues", vf, len(r.Issues))
		}
		b, err := canjson.Marshal(comp)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		return b
	}

	// (b) Same ValueSource seed → byte-reproducible.
	a := gen(instance.RandomFill, mrand.NewPCG(7, 7))
	b := gen(instance.RandomFill, mrand.NewPCG(7, 7))
	if !bytes.Equal(a, b) {
		t.Error("same ValueSource seed should produce byte-identical output")
	}

	// (c) RandomFill varies from the fixed ExampleFill baseline. The
	// vital_signs OPT carries DV_QUANTITY leaves with magnitude ranges, so
	// a seeded run must differ from the deterministic example fill.
	ex := gen(instance.ExampleFill, nil)
	if bytes.Equal(a, ex) {
		t.Error("RandomFill produced no leaf variation vs ExampleFill")
	}
}
