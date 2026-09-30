package canjson_test

import (
	"bytes"
	"encoding/json/jsontext"
	"reflect"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestREQ052OpenIntervalSideEmptyBound pins the REQ-052 rule that an open
// interval side carries no bound: the encoder omits the `lower` / `upper`
// member of a side marked unbounded when its bound is empty (nil, or the zero
// value of a concrete bound type). A non-empty bound beside its own open flag,
// and any bound on a bounded side, is emitted as it stands. The other members
// keep their order. REQ-112 reads the same shape on the floor.
//
// Each case lists the top-level members the wire must carry, in order. The
// concrete cases also decode the output back and compare it with the input,
// so dropping an empty bound loses nothing (round-trip fidelity is semantic,
// never a byte comparison).
func TestREQ052OpenIntervalSideEmptyBound(t *testing.T) {
	q := rm.DVQuantity{Magnitude: 5, Units: "mmol/L"}
	count := rm.DVCount{Magnitude: 3}
	when := rm.DVDateTime{Value: "2026-10-01T08:00:00Z"}
	var nilQuantity *rm.DVQuantity

	dvInterval := []string{"_type", "lower", "lower_included", "lower_unbounded", "upper", "upper_included", "upper_unbounded"}
	properInterval := slices.Clone(dvInterval)
	pointInterval := []string{"_type", "lower", "upper", "lower_included", "lower_unbounded", "upper_included", "upper_unbounded"}

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
			want:  without(dvInterval, "upper"),
			fresh: func() any { return &rm.DVInterval[rm.DVQuantity]{} },
		},
		{
			name:  "DV_QUANTITY, open lower side, empty bound omitted",
			value: &rm.DVInterval[rm.DVQuantity]{LowerUnbounded: true, Upper: q, UpperIncluded: true},
			want:  without(dvInterval, "lower"),
			fresh: func() any { return &rm.DVInterval[rm.DVQuantity]{} },
		},
		{
			name:  "DV_QUANTITY, both sides open, both empty bounds omitted",
			value: &rm.DVInterval[rm.DVQuantity]{LowerUnbounded: true, UpperUnbounded: true},
			want:  without(dvInterval, "lower", "upper"),
			fresh: func() any { return &rm.DVInterval[rm.DVQuantity]{} },
		},
		{
			name:  "DV_COUNT, open upper side, empty bound omitted",
			value: &rm.DVInterval[rm.DVCount]{Lower: count, LowerIncluded: true, UpperUnbounded: true},
			want:  without(dvInterval, "upper"),
			fresh: func() any { return &rm.DVInterval[rm.DVCount]{} },
		},
		{
			name:  "DV_COUNT, open lower side, empty bound omitted",
			value: &rm.DVInterval[rm.DVCount]{LowerUnbounded: true, Upper: count},
			want:  without(dvInterval, "lower"),
			fresh: func() any { return &rm.DVInterval[rm.DVCount]{} },
		},
		{
			name:  "DV_DATE_TIME, open upper side, empty bound omitted",
			value: &rm.DVInterval[rm.DVDateTime]{Lower: when, LowerIncluded: true, UpperUnbounded: true},
			want:  without(dvInterval, "upper"),
			fresh: func() any { return &rm.DVInterval[rm.DVDateTime]{} },
		},
		{
			name:  "DV_DATE_TIME, open lower side, empty bound omitted",
			value: &rm.DVInterval[rm.DVDateTime]{LowerUnbounded: true, Upper: when},
			want:  without(dvInterval, "lower"),
			fresh: func() any { return &rm.DVInterval[rm.DVDateTime]{} },
		},
		{
			name:  "non-empty bound beside its own open flag is kept",
			value: &rm.DVInterval[rm.DVQuantity]{Lower: q, LowerUnbounded: true, Upper: q, UpperUnbounded: true},
			want:  dvInterval,
			fresh: func() any { return &rm.DVInterval[rm.DVQuantity]{} },
		},
		{
			name:  "zero DV_COUNT on a bounded side is a real bound and is kept",
			value: &rm.DVInterval[rm.DVCount]{Lower: rm.DVCount{}, LowerIncluded: true, Upper: count},
			want:  dvInterval,
			fresh: func() any { return &rm.DVInterval[rm.DVCount]{} },
		},
		{
			name:  "Proper_interval of DV_COUNT, open upper side, empty bound omitted",
			value: &rm.ProperInterval[rm.DVCount]{Lower: count, UpperUnbounded: true},
			want:  without(properInterval, "upper"),
			fresh: func() any { return &rm.ProperInterval[rm.DVCount]{} },
		},
		{
			name:  "Proper_interval of DV_COUNT, open lower side, empty bound omitted",
			value: &rm.ProperInterval[rm.DVCount]{LowerUnbounded: true, Upper: count},
			want:  without(properInterval, "lower"),
			fresh: func() any { return &rm.ProperInterval[rm.DVCount]{} },
		},
		{
			name:  "Proper_interval of an Integer, open upper side, zero bound omitted",
			value: &rm.ProperInterval[rm.Integer]{Lower: 1, UpperUnbounded: true},
			want:  without(properInterval, "upper"),
			fresh: func() any { return &rm.ProperInterval[rm.Integer]{} },
		},
		{
			name:  "Point_interval of DV_QUANTITY, open upper side, empty bound omitted",
			value: &rm.PointInterval[rm.DVQuantity]{Lower: q, UpperUnbounded: true},
			want:  without(pointInterval, "upper"),
			fresh: func() any { return &rm.PointInterval[rm.DVQuantity]{} },
		},
		{
			name:  "interface-typed bound, nil on an open side, omitted",
			value: &rm.DVInterval[rm.DVOrdered]{Lower: &q, UpperUnbounded: true},
			want:  without(dvInterval, "upper"),
		},
		{
			name:  "interface-typed bound, typed nil on an open side, omitted",
			value: &rm.DVInterval[rm.DVOrdered]{Lower: nilQuantity, LowerUnbounded: true, Upper: &q},
			want:  without(dvInterval, "lower"),
		},
		{
			name:  "interface-typed bound, all-zero concrete on an open side, kept",
			value: &rm.DVInterval[rm.DVOrdered]{Lower: rm.DVQuantity{}, LowerUnbounded: true, Upper: &q},
			want:  dvInterval,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := canjson.Marshal(tc.value)
			if err != nil {
				t.Fatalf("Marshal(%T) error: %v", tc.value, err)
			}
			if got := topLevelMembers(t, b); !slices.Equal(got, tc.want) {
				t.Errorf("Marshal(%T) members = %q, want %q\nwire: %s", tc.value, got, tc.want, b)
			}
			if tc.fresh == nil {
				return
			}
			back := tc.fresh()
			if err := canjson.Unmarshal(b, back); err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", b, err)
			}
			if !reflect.DeepEqual(back, tc.value) {
				t.Errorf("round trip = %#v, want %#v\nwire: %s", back, tc.value, b)
			}
		})
	}
}

// without returns members with the named ones removed, order kept.
func without(members []string, drop ...string) []string {
	return slices.DeleteFunc(slices.Clone(members), func(m string) bool {
		return slices.Contains(drop, m)
	})
}

// topLevelMembers lists the member names of the JSON object b, in wire
// order.
func topLevelMembers(t *testing.T, b []byte) []string {
	t.Helper()
	dec := jsontext.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.ReadToken(); err != nil || tok.Kind() != '{' {
		t.Fatalf("wire is not a JSON object (token %v, error %v): %s", tok, err, b)
	}
	var names []string
	for dec.PeekKind() != '}' {
		tok, err := dec.ReadToken()
		if err != nil {
			t.Fatalf("read member name: %v: %s", err, b)
		}
		names = append(names, tok.String())
		if err := dec.SkipValue(); err != nil {
			t.Fatalf("skip member %q: %v: %s", tok.String(), err, b)
		}
	}
	return names
}
