package canjson_test

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
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
			name:  "Proper_interval of a Go int, open lower side, zero bound omitted",
			value: &rm.ProperInterval[int]{LowerUnbounded: true, Upper: 1},
			want:  without(properInterval, "lower"),
			fresh: func() any { return &rm.ProperInterval[int]{} },
		},
		{
			name:  "Proper_interval of a Go float32, open upper side, zero bound omitted",
			value: &rm.ProperInterval[float32]{Lower: 1.5, UpperUnbounded: true},
			want:  without(properInterval, "upper"),
			fresh: func() any { return &rm.ProperInterval[float32]{} },
		},
		{
			name:  "Point_interval of DV_QUANTITY, open upper side, empty bound omitted",
			value: &rm.PointInterval[rm.DVQuantity]{Lower: q, UpperUnbounded: true},
			want:  without(pointInterval, "upper"),
			fresh: func() any { return &rm.PointInterval[rm.DVQuantity]{} },
		},
		// The typed Point_interval and Proper_interval carry the same rule as
		// DV_INTERVAL through their own generated marshallers, one arm per
		// open-side combination, so each combination is pinned on its own.
		{
			name:  "Point_interval of DV_QUANTITY, open lower side, empty bound omitted",
			value: &rm.PointInterval[rm.DVQuantity]{LowerUnbounded: true, Upper: q},
			want:  without(pointInterval, "lower"),
			fresh: func() any { return &rm.PointInterval[rm.DVQuantity]{} },
		},
		{
			name:  "Point_interval of DV_QUANTITY, both sides open, both empty bounds omitted",
			value: &rm.PointInterval[rm.DVQuantity]{LowerUnbounded: true, UpperUnbounded: true},
			want:  without(pointInterval, "lower", "upper"),
			fresh: func() any { return &rm.PointInterval[rm.DVQuantity]{} },
		},
		{
			name:  "Point_interval of DV_QUANTITY, bounded sides keep both bounds",
			value: &rm.PointInterval[rm.DVQuantity]{Lower: q, Upper: q},
			want:  pointInterval,
			fresh: func() any { return &rm.PointInterval[rm.DVQuantity]{} },
		},
		{
			name:  "Proper_interval of DV_QUANTITY, open upper side, empty bound omitted",
			value: &rm.ProperInterval[rm.DVQuantity]{Lower: q, UpperUnbounded: true},
			want:  without(properInterval, "upper"),
			fresh: func() any { return &rm.ProperInterval[rm.DVQuantity]{} },
		},
		{
			name:  "Proper_interval of DV_QUANTITY, open lower side, empty bound omitted",
			value: &rm.ProperInterval[rm.DVQuantity]{LowerUnbounded: true, Upper: q},
			want:  without(properInterval, "lower"),
			fresh: func() any { return &rm.ProperInterval[rm.DVQuantity]{} },
		},
		{
			name:  "Proper_interval of DV_QUANTITY, both sides open, both empty bounds omitted",
			value: &rm.ProperInterval[rm.DVQuantity]{LowerUnbounded: true, UpperUnbounded: true},
			want:  without(properInterval, "lower", "upper"),
			fresh: func() any { return &rm.ProperInterval[rm.DVQuantity]{} },
		},
		{
			name:  "Proper_interval of DV_QUANTITY, bounded sides keep both bounds",
			value: &rm.ProperInterval[rm.DVQuantity]{Lower: q, Upper: q},
			want:  properInterval,
			fresh: func() any { return &rm.ProperInterval[rm.DVQuantity]{} },
		},
		// Regression pins: the encoders already left out a nil or typed-nil
		// interface-typed bound before the open-side rule, whatever its flag.
		// These two cases keep it that way.
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

// TestREQ052PointIntervalOuterFlagsWin pins which flags a Point_interval
// encodes. The class re-declares the four Boolean flags beside the embedded
// Interval's own, so a value carries two sets. The outer set is the one the
// encoder reads, both for the open-side omission and for the flag members it
// writes; a flag set only on the embedded Interval is never emitted and never
// opens a side. The shape stays as generated, and this test records that
// behaviour rather than a wish.
func TestREQ052PointIntervalOuterFlagsWin(t *testing.T) {
	q := rm.DVQuantity{Magnitude: 5, Units: "mmol/L"}
	pointInterval := []string{"_type", "lower", "upper", "lower_included", "lower_unbounded", "upper_included", "upper_unbounded"}
	allFalse := map[string]bool{"lower_unbounded": false, "upper_unbounded": false, "lower_included": false, "upper_included": false}

	cases := []struct {
		name      string
		value     *rm.PointInterval[rm.DVQuantity]
		wantNames []string
		wantFlags map[string]bool
	}{
		{
			name: "embedded lower_unbounded alone does not open the lower side",
			value: &rm.PointInterval[rm.DVQuantity]{
				Interval: rm.Interval[rm.DVQuantity]{Upper: q, LowerUnbounded: true},
			},
			wantNames: pointInterval,
			wantFlags: allFalse,
		},
		{
			name: "embedded upper_unbounded alone does not open the upper side",
			value: &rm.PointInterval[rm.DVQuantity]{
				Interval: rm.Interval[rm.DVQuantity]{Lower: q, UpperUnbounded: true},
			},
			wantNames: pointInterval,
			wantFlags: allFalse,
		},
		{
			name: "embedded included flags are not emitted",
			value: &rm.PointInterval[rm.DVQuantity]{
				Interval: rm.Interval[rm.DVQuantity]{Lower: q, Upper: q, LowerIncluded: true, UpperIncluded: true},
			},
			wantNames: pointInterval,
			wantFlags: allFalse,
		},
		{
			name: "outer flag wins over a disagreeing embedded flag",
			value: &rm.PointInterval[rm.DVQuantity]{
				LowerUnbounded: true,
				Interval:       rm.Interval[rm.DVQuantity]{Upper: q, LowerIncluded: true},
			},
			wantNames: without(pointInterval, "lower"),
			wantFlags: map[string]bool{"lower_unbounded": true, "upper_unbounded": false, "lower_included": false, "upper_included": false},
		},
		{
			name: "outer flags are emitted as they stand beside embedded ones",
			value: &rm.PointInterval[rm.DVQuantity]{
				UpperUnbounded: true,
				LowerIncluded:  true,
				Interval:       rm.Interval[rm.DVQuantity]{Lower: q, UpperIncluded: true},
			},
			wantNames: without(pointInterval, "upper"),
			wantFlags: map[string]bool{"lower_unbounded": false, "upper_unbounded": true, "lower_included": true, "upper_included": false},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := canjson.Marshal(tc.value)
			if err != nil {
				t.Fatalf("Marshal(%T) error: %v", tc.value, err)
			}
			if got := topLevelMembers(t, b); !slices.Equal(got, tc.wantNames) {
				t.Errorf("Marshal(%+v) members = %q, want %q\nwire: %s", *tc.value, got, tc.wantNames, b)
			}
			var wire map[string]any
			if err := json.Unmarshal(b, &wire); err != nil {
				t.Fatalf("decode wire %s: %v", b, err)
			}
			for flag, want := range tc.wantFlags {
				if got, _ := wire[flag].(bool); got != want {
					t.Errorf("Marshal(%+v) %s = %v, want %v\nwire: %s", *tc.value, flag, got, want, b)
				}
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
