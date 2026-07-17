package rmwrite

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestEnsureSingleNumericAndInterval pins the REQ-107 writers that
// the OPT corpus reaches only transitively (so they were otherwise
// 0%): the DV_COUNT/DV_QUANTITY scalar writers and the generic
// DV_INTERVAL<T> bound writer across two distinct T's (DVCount value
// → coerceValueOrPtr `case T`; *DVDate → coerceValueOrPtr `case *T`).
func TestEnsureSingleNumericAndInterval(t *testing.T) {
	cases := []struct {
		name   string
		parent any
		attr   string
		child  any
		check  func(t *testing.T, parent any)
	}{
		{
			name:   "DVCount.magnitude",
			parent: &rm.DVCount{},
			attr:   "magnitude",
			child:  5,
			check: func(t *testing.T, p any) {
				if got := p.(*rm.DVCount).Magnitude; got != 5 {
					t.Errorf("Magnitude = %d, want 5", got)
				}
			},
		},
		{
			name:   "DVQuantity.magnitude",
			parent: &rm.DVQuantity{},
			attr:   "magnitude",
			child:  98.6,
			check: func(t *testing.T, p any) {
				if got := p.(*rm.DVQuantity).Magnitude; got != rm.Real(98.6) {
					t.Errorf("Magnitude = %v, want 98.6", got)
				}
			},
		},
		{
			name:   "DVQuantity.units",
			parent: &rm.DVQuantity{},
			attr:   "units",
			child:  "mm[Hg]",
			check: func(t *testing.T, p any) {
				if got := p.(*rm.DVQuantity).Units; got != "mm[Hg]" {
					t.Errorf("Units = %q, want mm[Hg]", got)
				}
			},
		},
		{
			name:   "DVInterval[DVCount].lower (value bound)",
			parent: &rm.DVInterval[rm.DVCount]{},
			attr:   "lower",
			child:  rm.DVCount{Magnitude: 10},
			check: func(t *testing.T, p any) {
				if got := p.(*rm.DVInterval[rm.DVCount]).Lower.Magnitude; got != 10 {
					t.Errorf("Lower.Magnitude = %d, want 10", got)
				}
			},
		},
		{
			name:   "DVInterval[DVDate].upper (pointer bound)",
			parent: &rm.DVInterval[rm.DVDate]{},
			attr:   "upper",
			child:  &rm.DVDate{Value: "2020-01-01"},
			check: func(t *testing.T, p any) {
				if got := p.(*rm.DVInterval[rm.DVDate]).Upper.Value; got != "2020-01-01" {
					t.Errorf("Upper.Value = %q, want 2020-01-01", got)
				}
			},
		},
		{
			name:   "DVInterval[DVQuantity].lower_included (bool)",
			parent: &rm.DVInterval[rm.DVQuantity]{},
			attr:   "lower_included",
			child:  true,
			check: func(t *testing.T, p any) {
				if !p.(*rm.DVInterval[rm.DVQuantity]).LowerIncluded {
					t.Error("LowerIncluded = false, want true")
				}
			},
		},
		{
			name:   "Cluster.name (DV_TEXT)",
			parent: &rm.Cluster{},
			attr:   "name",
			child:  &rm.DVText{Value: "panel"},
			check: func(t *testing.T, p any) {
				name, ok := p.(*rm.Cluster).Name.(rm.DVText)
				if !ok {
					t.Fatalf("Name type = %T, want rm.DVText", p.(*rm.Cluster).Name)
				}
				if name.Value != "panel" {
					t.Errorf("Name.Value = %q, want panel", name.Value)
				}
			},
		},
		{
			// REQ-107 fix #2: an un-terminologised media_type code
			// defaults to the IANA media-types code set, not openehr.
			name:   "DVMultimedia.media_type (IANA default)",
			parent: &rm.DVMultimedia{},
			attr:   "media_type",
			child:  &rm.CodePhrase{CodeString: "application/pdf"},
			check: func(t *testing.T, p any) {
				mt := p.(*rm.DVMultimedia).MediaType
				if mt.TerminologyID.Value != "IANA_media-types" {
					t.Errorf("media_type terminology = %q, want IANA_media-types", mt.TerminologyID.Value)
				}
				if mt.CodeString != "application/pdf" {
					t.Errorf("media_type code = %q, want application/pdf", mt.CodeString)
				}
			},
		},
		{
			name:   "DVMultimedia.size",
			parent: &rm.DVMultimedia{},
			attr:   "size",
			child:  2048,
			check: func(t *testing.T, p any) {
				if got := p.(*rm.DVMultimedia).Size; got != 2048 {
					t.Errorf("Size = %d, want 2048", got)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := EnsureSingle(tc.parent, "", tc.attr, tc.child); err != nil {
				t.Fatalf("EnsureSingle: %v", err)
			}
			tc.check(t, tc.parent)
		})
	}
}

// TestEnsureSingleNumericIntervalMismatch pins the type-guard on the
// new writers: a wrong-typed bound / magnitude returns ErrTypeMismatch
// rather than silently coercing or panicking.
func TestEnsureSingleNumericIntervalMismatch(t *testing.T) {
	cases := []struct {
		name   string
		parent any
		attr   string
		child  any
	}{
		{"DVInterval[DVCount].lower wrong type", &rm.DVInterval[rm.DVCount]{}, "lower", "not a count"},
		{"DVQuantity.magnitude wrong type", &rm.DVQuantity{}, "magnitude", "not a real"},
		{"DVCount.magnitude wrong type", &rm.DVCount{}, "magnitude", "not an int"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := EnsureSingle(tc.parent, "", tc.attr, tc.child)
			if !errors.Is(err, ErrTypeMismatch) {
				t.Fatalf("want ErrTypeMismatch, got %v", err)
			}
		})
	}
}
