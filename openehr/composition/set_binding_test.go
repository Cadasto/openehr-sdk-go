package composition_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// Value children for compileAlternatives that the binding reads apart from
// the class hierarchy: the AOM 1.4 primitive short name DURATION, which the
// generator writes as its canonical DV wrapper DV_DURATION; a child with no
// RM type; and typed intervals, one over the abstract DV_ORDERED and one
// over the concrete DV_QUANTITY.
const (
	durationValueChild = `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>DURATION</rm_type_name><node_id></node_id>` +
		`<item xsi:type="C_DURATION"></item></children>`
	untypedValueChild    = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name></rm_type_name><node_id></node_id></children>`
	orderedIntervalType  = "DV_INTERVAL&lt;DV_ORDERED&gt;"
	quantityIntervalType = "DV_INTERVAL&lt;DV_QUANTITY&gt;"
)

// countInterval returns a DV_INTERVAL<DV_COUNT> from 1 to 5.
func countInterval() *rm.DVInterval[rm.DVCount] {
	return &rm.DVInterval[rm.DVCount]{
		Lower: rm.DVCount{Magnitude: 1}, LowerIncluded: true,
		Upper: rm.DVCount{Magnitude: 5}, UpperIncluded: true,
	}
}

// TestREQ101_SetAcceptsWhatTheValidatorBinds checks, for REQ-101 Type
// enforcement, that Set accepts a value the validator's alternatives rule
// (REQ-102) binds at the path beyond the class hierarchy, that Build writes
// it there, and that the validator reports no finding the composition did
// not have before: a DV_DURATION at a node typed with the AOM 1.4 short name
// DURATION, the value NewSkeleton itself writes there; a DV_QUANTITY at an
// untyped alternative after a DV_CODED_TEXT; and a DV_INTERVAL<DV_COUNT> at
// a DV_INTERVAL<DV_ORDERED> node, whose bounds conform to DV_ORDERED.
//
// An untyped node that NodeAt returns, the first or only child at the path,
// is out of reach: NewSkeleton cannot build a value for it, so no Builder
// exists to Set it on.
func TestREQ101_SetAcceptsWhatTheValidatorBinds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		c    *templatecompile.Compiled
		// nodeAt is the RM type of the node Compiled.NodeAt returns.
		nodeAt string
		// skeleton is the RM type of the value NewSkeleton writes at the path.
		skeleton string
		v        any
	}{
		{
			name: "DV_DURATION at DURATION", c: compileAlternatives(t, durationValueChild),
			nodeAt: "DURATION", skeleton: "DV_DURATION", v: &rm.DVDuration{Value: "PT5M"},
		},
		{
			name:   "DV_QUANTITY at an untyped alternative after DV_CODED_TEXT",
			c:      compileAlternatives(t, valueAlternative("DV_CODED_TEXT", false), untypedValueChild),
			nodeAt: "DV_CODED_TEXT", skeleton: "DV_CODED_TEXT", v: &rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"},
		},
		{
			name: "DV_INTERVAL<DV_COUNT> at DV_INTERVAL<DV_ORDERED>", c: compileAlternatives(t, valueAlternative(orderedIntervalType, false)),
			nodeAt: "DV_INTERVAL<DV_ORDERED>", skeleton: "DV_INTERVAL", v: countInterval(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			node, err := tc.c.NodeAt(abstractValuePath)
			if err != nil {
				t.Fatalf("NodeAt(%s): %v", abstractValuePath, err)
			}
			if got := node.RMTypeName(); got != tc.nodeAt {
				t.Fatalf("NodeAt(%s) has RM type %q, want %q (fixture drift)", abstractValuePath, got, tc.nodeAt)
			}
			b, err := composition.NewBuilder(t.Context(), tc.c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			skeleton, err := b.Build()
			if err != nil {
				t.Fatalf("Build of the bare skeleton: %v", err)
			}
			written, err := rmpath.ItemAtPath(skeleton, abstractValuePath)
			if err != nil {
				t.Fatalf("ItemAtPath(%s) on the skeleton: %v", abstractValuePath, err)
			}
			if name, _ := rm.RMTypeName(written); name != tc.skeleton {
				t.Fatalf("NewSkeleton writes %T (RM %q) at %s, want %s (generator drift)", written, name, abstractValuePath, tc.skeleton)
			}
			before := validation.ValidateComposition(skeleton, tc.c)

			if err := b.Set(abstractValuePath, tc.v); err != nil {
				t.Fatalf("Set(%s, %T) = %v, want nil", abstractValuePath, tc.v, err)
			}
			comp, err := b.Build()
			if err != nil {
				t.Fatalf("Build after Set(%s, %T) = %v, want nil", abstractValuePath, tc.v, err)
			}
			got, err := rmpath.ItemAtPath(comp, abstractValuePath)
			if err != nil {
				t.Fatalf("ItemAtPath(%s) on the built composition: %v", abstractValuePath, err)
			}
			if got != tc.v {
				t.Errorf("built composition holds %T %v at %s, want the value Set was given, %T %v", got, got, abstractValuePath, tc.v, tc.v)
			}
			after := validation.ValidateComposition(comp, tc.c)
			if added := newIssues(before, after); len(added) > 0 {
				t.Errorf("ValidateComposition after Set(%s, %T) reports new findings %q, want none", abstractValuePath, tc.v, added)
			}
		})
	}
}

// TestREQ101_SetRefusesWhatTheValidatorDoesNotBind checks, for REQ-101 Type
// enforcement, that Set returns ErrTypeMismatch, and Build carries it, for a
// value the alternatives rule (REQ-102) binds to no node at the path, at the
// nodes the short-name and interval tiers read: a DV_TEXT at DURATION; a
// DV_DATE, the wrapper of another short name, at DURATION; a DV_QUANTITY, an
// ordered value but no interval, at DV_INTERVAL<DV_ORDERED>; and a
// DV_INTERVAL<DV_COUNT> at DV_INTERVAL<DV_QUANTITY>, since a DV_COUNT bound
// does not conform to DV_QUANTITY. A Go string at DURATION is refused
// although the validator binds a string at a DURATION leaf: it is no RM
// value. A typed-nil *rm.DVDuration at DURATION is refused too.
func TestREQ101_SetRefusesWhatTheValidatorDoesNotBind(t *testing.T) {
	t.Parallel()
	duration := compileAlternatives(t, durationValueChild)
	orderedInterval := compileAlternatives(t, valueAlternative(orderedIntervalType, false))
	quantityInterval := compileAlternatives(t, valueAlternative(quantityIntervalType, false))

	cases := []struct {
		name string
		c    *templatecompile.Compiled
		v    any
	}{
		{name: "DV_TEXT at DURATION", c: duration, v: &rm.DVText{Value: "x"}},
		{name: "DV_DATE at DURATION", c: duration, v: &rm.DVDate{Value: "2026-10-15"}},
		{name: "Go string at DURATION", c: duration, v: "PT5M"},
		{name: "typed-nil DV_DURATION at DURATION", c: duration, v: (*rm.DVDuration)(nil)},
		{name: "DV_QUANTITY at DV_INTERVAL<DV_ORDERED>", c: orderedInterval, v: &rm.DVQuantity{Magnitude: 1, Units: "mm"}},
		{name: "DV_INTERVAL<DV_COUNT> at DV_INTERVAL<DV_QUANTITY>", c: quantityInterval, v: countInterval()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := composition.NewBuilder(t.Context(), tc.c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			if err := b.Set(abstractValuePath, tc.v); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Set(%s, %T) = %v, want ErrTypeMismatch", abstractValuePath, tc.v, err)
			}
			if _, err := b.Build(); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Build after Set(%s, %T) = %v, want ErrTypeMismatch", abstractValuePath, tc.v, err)
			}
		})
	}
}
