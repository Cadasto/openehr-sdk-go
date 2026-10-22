package rmwrite

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// optionalSlot is an optional attribute of one scalar type T, which an RM
// class holds as a pointer, written through one parent and attribute, and
// how it reads back.
type optionalSlot[T comparable] struct {
	name   string
	parent func() any
	attr   string
	// got reads the attribute back from the parent EnsureSingle wrote.
	got func(parent any) *T
}

// optionalChild is one Go shape a caller can hand EnsureSingle for an
// optional attribute.
type optionalChild struct {
	name  string
	child any
}

// checkOptionalSlots checks that every slot takes each of the accepted
// shapes and holds want afterwards, and refuses each of the refused
// children with ErrTypeMismatch, a nil pointer among them.
func checkOptionalSlots[T comparable](t *testing.T, slots []optionalSlot[T], want T, accepted []optionalChild, refused []any) {
	t.Helper()
	for _, slot := range slots {
		for _, tc := range accepted {
			parent := slot.parent()
			if err := EnsureSingle(parent, slot.attr, tc.child); err != nil {
				t.Errorf("%s: EnsureSingle(%T, %q, %s) = %v, want nil", slot.name, parent, slot.attr, tc.name, err)
				continue
			}
			if got := slot.got(parent); got == nil || *got != want {
				t.Errorf("%s: after writing a %s, the attribute = %v, want %v", slot.name, tc.name, got, want)
			}
		}
		for _, child := range refused {
			parent := slot.parent()
			if err := EnsureSingle(parent, slot.attr, child); !errors.Is(err, ErrTypeMismatch) {
				t.Errorf("%s: EnsureSingle(%T, %q, %#v) = %v, want ErrTypeMismatch", slot.name, parent, slot.attr, child, err)
			}
			if got := slot.got(parent); got != nil {
				t.Errorf("%s: after the refused write of %#v, the attribute = %v, want it left unset", slot.name, child, *got)
			}
		}
	}
}

// TestREQ101_EnsureSingleWritesAnOptionalStringAsAValueOrAPointer checks
// that every optional String attribute takes a string as a value or as a
// pointer, as EVENT_CONTEXT location does and every other optional
// attribute of a pointer-held scalar does for its own type, so what
// composition.Builder.Set accepts does not depend on the attribute. A nil
// pointer and a child of another type are refused with ErrTypeMismatch, and
// the attribute then stays unset. The attribute holds a copy: a later
// change to the caller's string does not reach it.
func TestREQ101_EnsureSingleWritesAnOptionalStringAsAValueOrAPointer(t *testing.T) {
	t.Parallel()
	slots := []optionalSlot[string]{
		{"DV_IDENTIFIER issuer", func() any { return &rm.DVIdentifier{} }, "issuer", func(p any) *string { return p.(*rm.DVIdentifier).Issuer }},
		{"DV_IDENTIFIER assigner", func() any { return &rm.DVIdentifier{} }, "assigner", func(p any) *string { return p.(*rm.DVIdentifier).Assigner }},
		{"DV_IDENTIFIER type", func() any { return &rm.DVIdentifier{} }, "type", func(p any) *string { return p.(*rm.DVIdentifier).Type }},
		{"DV_COUNT magnitude_status", func() any { return &rm.DVCount{} }, "magnitude_status", func(p any) *string { return p.(*rm.DVCount).MagnitudeStatus }},
		{"DV_QUANTITY magnitude_status", func() any { return &rm.DVQuantity{} }, "magnitude_status", func(p any) *string { return p.(*rm.DVQuantity).MagnitudeStatus }},
		{"DV_QUANTITY units_system", func() any { return &rm.DVQuantity{} }, "units_system", func(p any) *string { return p.(*rm.DVQuantity).UnitsSystem }},
		{"DV_QUANTITY units_display_name", func() any { return &rm.DVQuantity{} }, "units_display_name", func(p any) *string { return p.(*rm.DVQuantity).UnitsDisplayName }},
		{"DV_PROPORTION magnitude_status", func() any { return &rm.DVProportion{} }, "magnitude_status", func(p any) *string { return p.(*rm.DVProportion).MagnitudeStatus }},
		{"DV_DURATION magnitude_status", func() any { return &rm.DVDuration{} }, "magnitude_status", func(p any) *string { return p.(*rm.DVDuration).MagnitudeStatus }},
		{"DV_DATE magnitude_status", func() any { return &rm.DVDate{} }, "magnitude_status", func(p any) *string { return p.(*rm.DVDate).MagnitudeStatus }},
		{"DV_TEXT formatting", func() any { return &rm.DVText{} }, "formatting", func(p any) *string { return p.(*rm.DVText).Formatting }},
		{"PARTY_IDENTIFIED name", func() any { return &rm.PartyIdentified{} }, "name", func(p any) *string { return p.(*rm.PartyIdentified).Name }},
		{"LOCATABLE_REF path", func() any { return &rm.LocatableRef{} }, "path", func(p any) *string { return p.(*rm.LocatableRef).Path }},
		{"EVENT_CONTEXT location", func() any { return &rm.EventContext{} }, "location", func(p any) *string { return p.(*rm.EventContext).Location }},
	}
	const want = "ward A3"
	checkOptionalSlots(t, slots, want,
		[]optionalChild{{"string", want}, {"*string", new(want)}},
		[]any{(*string)(nil), 42, []byte(want), new(42)})

	for _, slot := range slots {
		s := want
		parent := slot.parent()
		if err := EnsureSingle(parent, slot.attr, &s); err != nil {
			t.Errorf("%s: EnsureSingle(*string) = %v, want nil", slot.name, err)
			continue
		}
		s = "changed after the write"
		if got := slot.got(parent); got == nil || *got != want {
			t.Errorf("%s: after the caller changed its string, the attribute = %v, want the copy %q", slot.name, got, want)
		}
	}
}

// TestREQ101_EnsureSingleWritesAnOptionalRealAsAValueOrAPointer checks that
// every optional Real attribute takes a number of any Go numeric shape, and
// an rm.Real held by pointer, as an optional Integer attribute takes an
// rm.Integer held by pointer, and refuses a nil pointer and a child that is
// no number with ErrTypeMismatch, leaving the attribute unset. The
// attribute holds a copy.
func TestREQ101_EnsureSingleWritesAnOptionalRealAsAValueOrAPointer(t *testing.T) {
	t.Parallel()
	slots := []optionalSlot[rm.Real]{
		{"DV_COUNT accuracy", func() any { return &rm.DVCount{} }, "accuracy", func(p any) *rm.Real { return p.(*rm.DVCount).Accuracy }},
		{"DV_QUANTITY accuracy", func() any { return &rm.DVQuantity{} }, "accuracy", func(p any) *rm.Real { return p.(*rm.DVQuantity).Accuracy }},
		{"DV_PROPORTION accuracy", func() any { return &rm.DVProportion{} }, "accuracy", func(p any) *rm.Real { return p.(*rm.DVProportion).Accuracy }},
		{"DV_DURATION accuracy", func() any { return &rm.DVDuration{} }, "accuracy", func(p any) *rm.Real { return p.(*rm.DVDuration).Accuracy }},
	}
	const want rm.Real = 2
	checkOptionalSlots(t, slots, want,
		[]optionalChild{
			{"float64", float64(2)},
			{"int", 2},
			{"rm.Integer", rm.Integer(2)},
			{"rm.Real", want},
			{"*rm.Real", new(want)},
		},
		[]any{(*rm.Real)(nil), "2", true, new(float64(2))})

	for _, slot := range slots {
		r := want
		parent := slot.parent()
		if err := EnsureSingle(parent, slot.attr, &r); err != nil {
			t.Errorf("%s: EnsureSingle(*rm.Real) = %v, want nil", slot.name, err)
			continue
		}
		r = 99
		if got := slot.got(parent); got == nil || *got != want {
			t.Errorf("%s: after the caller changed its Real, the attribute = %v, want the copy %v", slot.name, got, want)
		}
	}
}

// TestREQ101_EnsureSingleWritesAnOptionalBooleanAsAValueOrAPointer checks
// that every optional Boolean attribute takes a bool as a value or as a
// pointer, and refuses a nil pointer and a child of another type with
// ErrTypeMismatch, leaving the attribute unset. The attribute holds a copy.
func TestREQ101_EnsureSingleWritesAnOptionalBooleanAsAValueOrAPointer(t *testing.T) {
	t.Parallel()
	slots := []optionalSlot[bool]{
		{"DV_COUNT accuracy_is_percent", func() any { return &rm.DVCount{} }, "accuracy_is_percent", func(p any) *bool { return p.(*rm.DVCount).AccuracyIsPercent }},
		{"DV_QUANTITY accuracy_is_percent", func() any { return &rm.DVQuantity{} }, "accuracy_is_percent", func(p any) *bool { return p.(*rm.DVQuantity).AccuracyIsPercent }},
		{"DV_PROPORTION accuracy_is_percent", func() any { return &rm.DVProportion{} }, "accuracy_is_percent", func(p any) *bool { return p.(*rm.DVProportion).AccuracyIsPercent }},
		{"DV_DURATION accuracy_is_percent", func() any { return &rm.DVDuration{} }, "accuracy_is_percent", func(p any) *bool { return p.(*rm.DVDuration).AccuracyIsPercent }},
	}
	checkOptionalSlots(t, slots, true,
		[]optionalChild{{"bool", true}, {"*bool", new(true)}},
		[]any{(*bool)(nil), "true", 1, new("true")})

	for _, slot := range slots {
		b := true
		parent := slot.parent()
		if err := EnsureSingle(parent, slot.attr, &b); err != nil {
			t.Errorf("%s: EnsureSingle(*bool) = %v, want nil", slot.name, err)
			continue
		}
		b = false
		if got := slot.got(parent); got == nil || !*got {
			t.Errorf("%s: after the caller changed its bool, the attribute = %v, want the copy true", slot.name, got)
		}
	}
}
