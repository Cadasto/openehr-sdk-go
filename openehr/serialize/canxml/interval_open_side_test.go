package canxml_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"reflect"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ056OpenIntervalSideEmptyBound pins the REQ-056 reading of the
// REQ-052 rule for canonical XML: an open interval side's empty bound (nil,
// or the zero value of a concrete bound type) is emitted as ABSENT, so the
// `lower` / `upper` element of a side marked unbounded is left out. A
// non-empty bound beside its own open flag, and any bound on a bounded side,
// is emitted as it stands, and the other elements keep their order. REQ-112
// reads the same shape on the floor.
//
// Each case lists the child elements of the root, in order. The concrete
// cases also decode the output back and compare it with the input, so
// dropping an empty bound loses nothing (round-trip fidelity is semantic).
func TestREQ056OpenIntervalSideEmptyBound(t *testing.T) {
	q := rm.DVQuantity{Magnitude: 5, Units: "mmol/L"}
	count := rm.DVCount{Magnitude: 3}
	when := rm.DVDateTime{Value: "2026-10-01T08:00:00Z"}
	var nilQuantity *rm.DVQuantity

	// Every interval class shares this element order: the bounds first, then
	// the flags.
	all := []string{"lower", "upper", "lower_unbounded", "upper_unbounded", "lower_included", "upper_included"}

	cases := []struct {
		name  string
		value any
		want  []string
		// fresh returns an empty value of the same type to decode the output
		// into; nil skips the semantic round trip.
		fresh func() any
	}{
		{
			name:  "DV_QUANTITY, open upper side, empty bound omitted",
			value: &rm.DVInterval[rm.DVQuantity]{Lower: q, LowerIncluded: true, UpperUnbounded: true},
			want:  childrenWithout(all, "upper"),
			fresh: func() any { return &rm.DVInterval[rm.DVQuantity]{} },
		},
		{
			name:  "DV_QUANTITY, open lower side, empty bound omitted",
			value: &rm.DVInterval[rm.DVQuantity]{LowerUnbounded: true, Upper: q, UpperIncluded: true},
			want:  childrenWithout(all, "lower"),
			fresh: func() any { return &rm.DVInterval[rm.DVQuantity]{} },
		},
		{
			name:  "DV_QUANTITY, both sides open, both empty bounds omitted",
			value: &rm.DVInterval[rm.DVQuantity]{LowerUnbounded: true, UpperUnbounded: true},
			want:  childrenWithout(all, "lower", "upper"),
			fresh: func() any { return &rm.DVInterval[rm.DVQuantity]{} },
		},
		{
			name:  "DV_COUNT, open upper side, empty bound omitted",
			value: &rm.DVInterval[rm.DVCount]{Lower: count, LowerIncluded: true, UpperUnbounded: true},
			want:  childrenWithout(all, "upper"),
			fresh: func() any { return &rm.DVInterval[rm.DVCount]{} },
		},
		{
			name:  "DV_COUNT, open lower side, empty bound omitted",
			value: &rm.DVInterval[rm.DVCount]{LowerUnbounded: true, Upper: count},
			want:  childrenWithout(all, "lower"),
			fresh: func() any { return &rm.DVInterval[rm.DVCount]{} },
		},
		{
			name:  "DV_DATE_TIME, open upper side, empty bound omitted",
			value: &rm.DVInterval[rm.DVDateTime]{Lower: when, LowerIncluded: true, UpperUnbounded: true},
			want:  childrenWithout(all, "upper"),
			fresh: func() any { return &rm.DVInterval[rm.DVDateTime]{} },
		},
		{
			name:  "DV_DATE_TIME, open lower side, empty bound omitted",
			value: &rm.DVInterval[rm.DVDateTime]{LowerUnbounded: true, Upper: when},
			want:  childrenWithout(all, "lower"),
			fresh: func() any { return &rm.DVInterval[rm.DVDateTime]{} },
		},
		{
			name:  "non-empty bound beside its own open flag is kept",
			value: &rm.DVInterval[rm.DVQuantity]{Lower: q, LowerUnbounded: true, Upper: q, UpperUnbounded: true},
			want:  all,
			fresh: func() any { return &rm.DVInterval[rm.DVQuantity]{} },
		},
		{
			name:  "zero DV_COUNT on a bounded side is a real bound and is kept",
			value: &rm.DVInterval[rm.DVCount]{Lower: rm.DVCount{}, LowerIncluded: true, Upper: count},
			want:  all,
			fresh: func() any { return &rm.DVInterval[rm.DVCount]{} },
		},
		{
			name:  "Proper_interval of DV_COUNT, open upper side, empty bound omitted",
			value: &rm.ProperInterval[rm.DVCount]{Lower: count, UpperUnbounded: true},
			want:  childrenWithout(all, "upper"),
			fresh: func() any { return &rm.ProperInterval[rm.DVCount]{} },
		},
		{
			name:  "Proper_interval of DV_COUNT, open lower side, empty bound omitted",
			value: &rm.ProperInterval[rm.DVCount]{LowerUnbounded: true, Upper: count},
			want:  childrenWithout(all, "lower"),
			fresh: func() any { return &rm.ProperInterval[rm.DVCount]{} },
		},
		{
			name:  "Proper_interval of an Integer, open upper side, zero bound omitted",
			value: &rm.ProperInterval[rm.Integer]{Lower: 1, UpperUnbounded: true},
			want:  childrenWithout(all, "upper"),
			fresh: func() any { return &rm.ProperInterval[rm.Integer]{} },
		},
		{
			name:  "Proper_interval of a Go int, open lower side, zero bound omitted",
			value: &rm.ProperInterval[int]{LowerUnbounded: true, Upper: 1},
			want:  childrenWithout(all, "lower"),
			fresh: func() any { return &rm.ProperInterval[int]{} },
		},
		{
			name:  "Proper_interval of a Go float32, open upper side, zero bound omitted",
			value: &rm.ProperInterval[float32]{Lower: 1.5, UpperUnbounded: true},
			want:  childrenWithout(all, "upper"),
			fresh: func() any { return &rm.ProperInterval[float32]{} },
		},
		{
			name:  "Point_interval of DV_QUANTITY, open upper side, empty bound omitted",
			value: &rm.PointInterval[rm.DVQuantity]{Lower: q, UpperUnbounded: true},
			want:  childrenWithout(all, "upper"),
			fresh: func() any { return &rm.PointInterval[rm.DVQuantity]{} },
		},
		// Regression pins: the encoders already left out a nil or typed-nil
		// interface-typed bound before the open-side rule, whatever its flag.
		// These two cases keep it that way.
		{
			name:  "interface-typed bound, nil on an open side, omitted",
			value: &rm.DVInterval[rm.DVOrdered]{Lower: &q, UpperUnbounded: true},
			want:  childrenWithout(all, "upper"),
		},
		{
			name:  "interface-typed bound, typed nil on an open side, omitted",
			value: &rm.DVInterval[rm.DVOrdered]{Lower: nilQuantity, LowerUnbounded: true, Upper: &q},
			want:  childrenWithout(all, "lower"),
		},
		{
			name:  "interface-typed bound, all-zero concrete on an open side, kept",
			value: &rm.DVInterval[rm.DVOrdered]{Lower: rm.DVQuantity{}, LowerUnbounded: true, Upper: &q},
			want:  all,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := canxml.Marshal(tc.value)
			if err != nil {
				t.Fatalf("Marshal(%T) error: %v", tc.value, err)
			}
			if got := rootChildren(t, b); !slices.Equal(got, tc.want) {
				t.Errorf("Marshal(%T) children = %q, want %q\nwire: %s", tc.value, got, tc.want, b)
			}
			if tc.fresh == nil {
				return
			}
			back := tc.fresh()
			if err := canxml.Unmarshal(b, back); err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", b, err)
			}
			if !reflect.DeepEqual(back, tc.value) {
				t.Errorf("round trip = %#v, want %#v\nwire: %s", back, tc.value, b)
			}
		})
	}
}

// childrenWithout returns names with the named ones removed, order kept.
func childrenWithout(names []string, drop ...string) []string {
	return slices.DeleteFunc(slices.Clone(names), func(n string) bool {
		return slices.Contains(drop, n)
	})
}

// rootChildren lists the local names of the root element's child elements,
// in document order.
func rootChildren(t *testing.T, b []byte) []string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(b))
	var names []string
	depth := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return names
		}
		if err != nil {
			t.Fatalf("read XML token: %v: %s", err, b)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 {
				names = append(names, el.Name.Local)
			}
		case xml.EndElement:
			depth--
		}
	}
}
