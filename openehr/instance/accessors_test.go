package instance_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestAccessors_ErrTypeMismatch pins the closed-root-set accessors
// (REQ-107 §accessors). Each As* surfaces ErrTypeMismatch when the
// caller passes a value whose concrete RM type differs from the
// accessor's target — the failure mode that consumers handle when
// Generate's `any` return is asserted against the wrong root type.
func TestAccessors_ErrTypeMismatch(t *testing.T) {
	// Cross-product table: input value vs accessor function. Every
	// row except the diagonal must surface ErrTypeMismatch.
	comp := &rm.Composition{}
	obs := &rm.Observation{}
	cases := []struct {
		name string
		in   any
		call func(any) error
	}{
		{"AsObservation(Composition)", comp, func(v any) error { _, err := instance.AsObservation(v); return err }},
		{"AsEvaluation(Composition)", comp, func(v any) error { _, err := instance.AsEvaluation(v); return err }},
		{"AsInstruction(Composition)", comp, func(v any) error { _, err := instance.AsInstruction(v); return err }},
		{"AsAction(Composition)", comp, func(v any) error { _, err := instance.AsAction(v); return err }},
		{"AsAdminEntry(Composition)", comp, func(v any) error { _, err := instance.AsAdminEntry(v); return err }},
		{"AsCluster(Composition)", comp, func(v any) error { _, err := instance.AsCluster(v); return err }},
		{"AsSection(Composition)", comp, func(v any) error { _, err := instance.AsSection(v); return err }},
		{"AsGenericEntry(Composition)", comp, func(v any) error { _, err := instance.AsGenericEntry(v); return err }},
		{"AsElement(Composition)", comp, func(v any) error { _, err := instance.AsElement(v); return err }},
		{"AsComposition(Observation)", obs, func(v any) error { _, err := instance.AsComposition(v); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(tc.in)
			if !errors.Is(err, instance.ErrTypeMismatch) {
				t.Errorf("err = %v; want errors.Is(_, ErrTypeMismatch)", err)
			}
		})
	}
}

// TestAccessors_Diagonal confirms each accessor returns the typed
// pointer (no error) when the input matches its target type.
func TestAccessors_Diagonal(t *testing.T) {
	comp := &rm.Composition{}
	got, err := instance.AsComposition(comp)
	if err != nil {
		t.Fatalf("AsComposition(*rm.Composition): %v", err)
	}
	if got != comp {
		t.Error("AsComposition returned a different pointer")
	}
	obs := &rm.Observation{}
	got2, err := instance.AsObservation(obs)
	if err != nil {
		t.Fatalf("AsObservation(*rm.Observation): %v", err)
	}
	if got2 != obs {
		t.Error("AsObservation returned a different pointer")
	}
}
