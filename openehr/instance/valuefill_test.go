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

// TestGeneratedSettingSatisfiesSettingValid — REQ-107. EVENT_CONTEXT.setting
// carries the RM invariant Setting_valid: the defining code must be a member of
// the openEHR terminology's `setting` group. The generic example synthesiser
// will happily invent an archetype-local code for a template that does not
// constrain setting, which reads as "populated" and is still RM-invalid — and
// the FLAT ctx/setting form (code + implied openehr terminology) then cannot
// represent it, so MarshalFlat refused the generator's own output (PR #88
// review). Every generated composition must be openehr-coded.
func TestGeneratedSettingSatisfiesSettingValid(t *testing.T) {
	name := "Test Composer"
	for _, fixture := range []string{"body_weight", "vital_signs", "minimal_observation.en.v1"} {
		t.Run(fixture, func(t *testing.T) {
			out, err := instance.Generate(context.Background(), compileFixture(t, fixture), instance.Options{
				Policy:    instance.Example,
				Territory: "NL",
				Composer:  &rm.PartyIdentified{Name: &name},
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			comp, err := instance.AsComposition(out)
			if err != nil {
				t.Fatalf("AsComposition: %v", err)
			}
			code := comp.Context.Setting.DefiningCode
			if code.CodeString == "" {
				t.Error("generated setting has no defining code (EVENT_CONTEXT.setting is RM-mandatory)")
			}
			if code.TerminologyID.Value != "openehr" {
				t.Errorf("generated setting is coded %q/%q; Setting_valid requires a code from the openehr setting group",
					code.CodeString, code.TerminologyID.Value)
			}
		})
	}
}

// TestGeneratedSubjectSatisfiesBasicValidity — REQ-107. ENTRY.subject is an
// RM-mandatory PARTY_PROXY, and the generator fills an unconstrained abstract
// attribute with a bare instance of its concrete stand-in. PARTY_IDENTIFIED is
// not a valid stand-in there: its invariant `Basic_validity` requires at least
// one of `name`, `identifiers` or `external_ref`, so the empty one carried an
// RM violation from the moment it was generated — and on the FLAT wire it is
// also indistinguishable from PARTY_SELF, which the simplified codec spells by
// the absence of every party key (PR #89 review). PARTY_SELF is the only
// PARTY_PROXY subtype that is valid with no attributes at all.
func TestGeneratedSubjectSatisfiesBasicValidity(t *testing.T) {
	name := "Test Composer"
	for _, fixture := range []string{"body_weight", "vital_signs", "minimal_observation.en.v1"} {
		t.Run(fixture, func(t *testing.T) {
			out, err := instance.Generate(context.Background(), compileFixture(t, fixture), instance.Options{
				Policy:    instance.Example,
				Territory: "NL",
				Composer:  &rm.PartyIdentified{Name: &name},
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			comp, err := instance.AsComposition(out)
			if err != nil {
				t.Fatalf("AsComposition: %v", err)
			}
			for _, item := range comp.Content {
				entry, isEntry := item.(*rm.Observation)
				if !isEntry {
					continue
				}
				switch subject := entry.Subject.(type) {
				case *rm.PartySelf, rm.PartySelf:
				case *rm.PartyIdentified:
					if subject.Name == nil && subject.ExternalRef == nil && len(subject.Identifiers) == 0 {
						t.Error("generated ENTRY.subject is an empty PARTY_IDENTIFIED, which violates Basic_validity")
					}
				case nil:
					t.Error("generated ENTRY.subject is absent (ENTRY.subject is RM-mandatory)")
				}
			}
		})
	}
}
