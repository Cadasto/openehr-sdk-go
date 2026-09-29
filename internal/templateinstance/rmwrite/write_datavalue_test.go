package rmwrite

import (
	"errors"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
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

// openInterval returns a DV_INTERVAL<T> whose two sides are both open,
// the state the instance generator seeds before it writes any bound.
func openInterval[T rm.DVOrdered]() *rm.DVInterval[T] {
	iv := &rm.DVInterval[T]{}
	iv.LowerUnbounded = true
	iv.UpperUnbounded = true
	return iv
}

// intervalOpenFlags reads the two *_unbounded flags off one of the
// DV_INTERVAL instantiations the table below builds.
func intervalOpenFlags(t *testing.T, parent any) (lowerOpen, upperOpen bool) {
	t.Helper()
	switch iv := parent.(type) {
	case *rm.DVInterval[rm.DVCount]:
		return iv.LowerUnbounded, iv.UpperUnbounded
	case *rm.DVInterval[rm.DVDate]:
		return iv.LowerUnbounded, iv.UpperUnbounded
	case *rm.DVInterval[rm.DVOrdered]:
		return iv.LowerUnbounded, iv.UpperUnbounded
	}
	t.Fatalf("intervalOpenFlags: unsupported parent %T", parent)
	return false, false
}

// TestEnsureSingleIntervalBoundClosesItsSide pins the DV_INTERVAL<T>
// bound writer that REQ-101's composition builder and the REQ-107
// generator share: BASE Interval says an open side carries no bound,
// so writing a real bound closes that side and leaves the other one
// alone. A Void bound (a nil, or a typed-nil behind the
// DV_INTERVAL<DV_ORDERED> interface) keeps the side open, and a flag
// written after the bound is kept as written (last write wins). Each
// case starts from an interval open on both sides.
func TestEnsureSingleIntervalBoundClosesItsSide(t *testing.T) {
	type write struct {
		attr    string
		child   any
		wantErr error // nil: the write succeeds
	}
	var (
		nilCount    *rm.DVCount
		nilQuantity *rm.DVQuantity
	)
	cases := []struct {
		name          string
		parent        func() any
		writes        []write
		wantLowerOpen bool
		wantUpperOpen bool
	}{
		{
			name:          "DV_COUNT lower closes the lower side only",
			parent:        func() any { return openInterval[rm.DVCount]() },
			writes:        []write{{attr: "lower", child: rm.DVCount{Magnitude: 10}}},
			wantLowerOpen: false,
			wantUpperOpen: true,
		},
		{
			name:          "DV_COUNT upper (pointer) closes the upper side only",
			parent:        func() any { return openInterval[rm.DVCount]() },
			writes:        []write{{attr: "upper", child: &rm.DVCount{Magnitude: 90}}},
			wantLowerOpen: true,
			wantUpperOpen: false,
		},
		{
			// A concrete bound cannot be Void: a count of 0 is a count.
			name:          "DV_COUNT lower of magnitude 0 is a real bound",
			parent:        func() any { return openInterval[rm.DVCount]() },
			writes:        []write{{attr: "lower", child: rm.DVCount{}}},
			wantLowerOpen: false,
			wantUpperOpen: true,
		},
		{
			name:          "DV_DATE upper (pointer) closes the upper side only",
			parent:        func() any { return openInterval[rm.DVDate]() },
			writes:        []write{{attr: "upper", child: &rm.DVDate{Value: "2020-01-01"}}},
			wantLowerOpen: true,
			wantUpperOpen: false,
		},
		{
			name:          "DV_COUNT nil pointer lower is refused and the side stays open",
			parent:        func() any { return openInterval[rm.DVCount]() },
			writes:        []write{{attr: "lower", child: nilCount, wantErr: ErrTypeMismatch}},
			wantLowerOpen: true,
			wantUpperOpen: true,
		},
		{
			name:          "DV_ORDERED lower closes the lower side only",
			parent:        func() any { return openInterval[rm.DVOrdered]() },
			writes:        []write{{attr: "lower", child: rm.DVQuantity{Magnitude: 1, Units: "kg"}}},
			wantLowerOpen: false,
			wantUpperOpen: true,
		},
		{
			// An interface bound spells Void as nil, so any value is a bound,
			// even one whose fields are all zero.
			name:          "DV_ORDERED upper with all-zero fields is a real bound",
			parent:        func() any { return openInterval[rm.DVOrdered]() },
			writes:        []write{{attr: "upper", child: rm.DVCount{}}},
			wantLowerOpen: true,
			wantUpperOpen: false,
		},
		{
			name:          "DV_ORDERED typed-nil lower keeps the lower side open",
			parent:        func() any { return openInterval[rm.DVOrdered]() },
			writes:        []write{{attr: "lower", child: nilQuantity}},
			wantLowerOpen: true,
			wantUpperOpen: true,
		},
		{
			name:          "DV_ORDERED typed-nil upper keeps the upper side open",
			parent:        func() any { return openInterval[rm.DVOrdered]() },
			writes:        []write{{attr: "upper", child: nilCount}},
			wantLowerOpen: true,
			wantUpperOpen: true,
		},
		{
			name:          "DV_ORDERED nil lower is refused and the side stays open",
			parent:        func() any { return openInterval[rm.DVOrdered]() },
			writes:        []write{{attr: "lower", child: nil, wantErr: ErrTypeMismatch}},
			wantLowerOpen: true,
			wantUpperOpen: true,
		},
		{
			name:   "lower_unbounded written after the lower bound is kept",
			parent: func() any { return openInterval[rm.DVCount]() },
			writes: []write{
				{attr: "lower", child: rm.DVCount{Magnitude: 10}},
				{attr: "lower_unbounded", child: true},
			},
			wantLowerOpen: true,
			wantUpperOpen: true,
		},
		{
			name:   "upper_unbounded written after the upper bound is kept",
			parent: func() any { return openInterval[rm.DVOrdered]() },
			writes: []write{
				{attr: "upper", child: rm.DVCount{Magnitude: 90}},
				{attr: "upper_unbounded", child: true},
			},
			wantLowerOpen: true,
			wantUpperOpen: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := tc.parent()
			for _, w := range tc.writes {
				err := EnsureSingle(parent, "", w.attr, w.child)
				if w.wantErr == nil && err != nil {
					t.Fatalf("EnsureSingle(%T, %q, %#v) = %v, want nil", parent, w.attr, w.child, err)
				}
				if w.wantErr != nil && !errors.Is(err, w.wantErr) {
					t.Fatalf("EnsureSingle(%T, %q, %#v) = %v, want %v", parent, w.attr, w.child, err, w.wantErr)
				}
			}
			lowerOpen, upperOpen := intervalOpenFlags(t, parent)
			if lowerOpen != tc.wantLowerOpen || upperOpen != tc.wantUpperOpen {
				t.Errorf("after writes, (lower_unbounded, upper_unbounded) = (%v, %v), want (%v, %v)",
					lowerOpen, upperOpen, tc.wantLowerOpen, tc.wantUpperOpen)
			}
		})
	}
}

// TestIsVoidBound pins the Void test behind the bound writer directly.
// EnsureSingle refuses a nil child before the bound reaches it, so the
// nil row is only reachable here.
func TestIsVoidBound(t *testing.T) {
	var nilQuantity *rm.DVQuantity
	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "DV_ORDERED nil", got: isVoidBound[rm.DVOrdered](nil), want: true},
		{name: "DV_ORDERED typed-nil *DV_QUANTITY", got: isVoidBound[rm.DVOrdered](nilQuantity), want: true},
		{name: "DV_ORDERED all-zero DV_COUNT", got: isVoidBound[rm.DVOrdered](rm.DVCount{}), want: false},
		{name: "DV_ORDERED DV_QUANTITY", got: isVoidBound[rm.DVOrdered](rm.DVQuantity{Units: "kg"}), want: false},
		{name: "concrete all-zero DV_COUNT", got: isVoidBound(rm.DVCount{}), want: false},
		{name: "concrete DV_DATE", got: isVoidBound(rm.DVDate{Value: "2020-01-01"}), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("isVoidBound(%s) = %v, want %v", tc.name, tc.got, tc.want)
			}
		})
	}
}

// intervalWriterCase is one DV_INTERVAL<T> instantiation the writer must
// dispatch: a fresh interval, and a bound of its parameter type.
type intervalWriterCase struct {
	boundRM string
	parent  func() any
	bound   any
}

func typedIntervalWriterCase[T rm.DVOrdered]() intervalWriterCase {
	var bound T
	name, _ := rm.RMTypeName(bound)
	return intervalWriterCase{
		boundRM: name,
		parent:  func() any { return &rm.DVInterval[T]{} },
		bound:   bound,
	}
}

// TestIntervalWriterParity checks that EnsureSingle writes every
// DV_INTERVAL instantiation the reader covers. REQ-101's builder and the
// REQ-107 generator both attach interval attributes through it, so a
// missing instantiation fails them. rmwrite sits outside openehr/ and so
// cannot import openehr/internal/rmnames; the canonical set is derived the
// way TestTypedIntervalReaderParity (openehr/validation/rmread) derives
// it, one DV_INTERVAL<X> per registered type X that implements
// rm.DVOrdered. A new DV_ORDERED descendant fails here until the case list
// and the writer both gain it, and a dropped writer arm fails the writes.
// The bare DV_INTERVAL<DV_ORDERED> is reached through its own
// registration.
func TestIntervalWriterParity(t *testing.T) {
	var fromRegistry []string
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.DVOrdered); ok {
			fromRegistry = append(fromRegistry, name)
		}
	}
	if len(fromRegistry) == 0 {
		t.Fatal("registry yields no DVOrdered implementers; registrations missing?")
	}
	slices.Sort(fromRegistry)

	cases := []intervalWriterCase{
		typedIntervalWriterCase[rm.DVCount](),
		typedIntervalWriterCase[rm.DVDate](),
		typedIntervalWriterCase[rm.DVDateTime](),
		typedIntervalWriterCase[rm.DVDuration](),
		typedIntervalWriterCase[rm.DVOrdinal](),
		typedIntervalWriterCase[rm.DVProportion](),
		typedIntervalWriterCase[rm.DVQuantity](),
		typedIntervalWriterCase[rm.DVScale](),
		typedIntervalWriterCase[rm.DVTime](),
	}
	listed := make([]string, 0, len(cases))
	for _, c := range cases {
		listed = append(listed, c.boundRM)
	}
	slices.Sort(listed)
	if !slices.Equal(fromRegistry, listed) {
		t.Errorf("typed DV_INTERVAL drift:\n  registry-derived bounds: %v\n  writer cases:            %v", fromRegistry, listed)
	}

	bare, ok := typereg.Default.Lookup("DV_INTERVAL")
	if !ok {
		t.Fatal(`typereg has no "DV_INTERVAL" registration`)
	}
	cases = append(cases, intervalWriterCase{boundRM: "DV_ORDERED", parent: bare, bound: rm.DVCount{Magnitude: 1}})

	for _, c := range cases {
		t.Run("DV_INTERVAL<"+c.boundRM+">", func(t *testing.T) {
			parent := c.parent()
			for _, w := range []struct {
				attr  string
				child any
			}{
				{attr: "lower", child: c.bound},
				{attr: "upper", child: c.bound},
				{attr: "lower_unbounded", child: false},
				{attr: "upper_unbounded", child: false},
				{attr: "lower_included", child: true},
				{attr: "upper_included", child: true},
			} {
				if err := EnsureSingle(parent, "DV_INTERVAL", w.attr, w.child); err != nil {
					t.Errorf("EnsureSingle(%T, %q, %T) = %v, want nil", parent, w.attr, w.child, err)
				}
			}
		})
	}
}
