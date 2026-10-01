package canjson_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestREQ052IntervalFailedDecodeKeepsWhatWasRead pins the state an interval
// receiver is left in when its decode fails (REQ-052). The interval classes
// decode in place: members read before the failure stay on the receiver, and
// members not yet read keep their previous values. That holds for the bound,
// for Point_interval's own four flags, and for a `_type` mismatch or a
// duplicate member, which fail only after the earlier members were read.
func TestREQ052IntervalFailedDecodeKeepsWhatWasRead(t *testing.T) {
	type point = rm.PointInterval[rm.Integer]
	start := func() *point {
		return &point{Interval: rm.Interval[rm.Integer]{Lower: 1}, UpperUnbounded: true}
	}
	cases := []struct {
		name string
		json string
		want point
	}{
		{
			name: "a bad upper bound after lower and lower_included",
			json: `{"_type":"Point_interval","lower":5,"lower_included":true,"upper":"x"}`,
			want: point{Interval: rm.Interval[rm.Integer]{Lower: 5}, LowerIncluded: true, UpperUnbounded: true},
		},
		{
			name: "a _type mismatch found after the members were read",
			json: `{"_type":"Proper_interval","lower":5,"lower_included":true,"upper":"x"}`,
			want: point{Interval: rm.Interval[rm.Integer]{Lower: 5}, LowerIncluded: true, UpperUnbounded: true},
		},
		{
			name: "a duplicate member",
			json: `{"_type":"Point_interval","lower":5,"lower_included":true,"lower":6}`,
			want: point{Interval: rm.Interval[rm.Integer]{Lower: 5}, LowerIncluded: true, UpperUnbounded: true},
		},
		{
			name: "a duplicate flag",
			json: `{"_type":"Point_interval","lower":5,"lower_included":true,"upper_unbounded":true,"upper_unbounded":false}`,
			want: point{Interval: rm.Interval[rm.Integer]{Lower: 5}, LowerIncluded: true, UpperUnbounded: true},
		},
		{
			name: "a bad flag after a good one",
			json: `{"_type":"Point_interval","lower_included":true,"upper_included":"zz"}`,
			want: point{Interval: rm.Interval[rm.Integer]{Lower: 1}, LowerIncluded: true, UpperUnbounded: true},
		},
		{
			name: "a different _type read before any member",
			json: `{"_type":"DV_INTERVAL","lower":{"_type":"DV_COUNT","magnitude":5},"lower_included":true,"upper":"x"}`,
			want: point{Interval: rm.Interval[rm.Integer]{Lower: 1}, UpperUnbounded: true},
		},
	}
	for _, tc := range cases {
		t.Run("Point_interval: "+tc.name, func(t *testing.T) {
			got := start()
			if err := canjson.Unmarshal([]byte(tc.json), got); err == nil {
				t.Fatal("Unmarshal succeeded, want a failure")
			}
			if *got != tc.want {
				t.Errorf("receiver after the failed decode = %+v, want %+v", *got, tc.want)
			}
		})
	}

	t.Run("Proper_interval: a bad upper bound", func(t *testing.T) {
		got := &rm.ProperInterval[rm.Integer]{Interval: rm.Interval[rm.Integer]{Lower: 1, UpperUnbounded: true}}
		in := `{"_type":"Proper_interval","lower":5,"lower_included":true,"upper":"x"}`
		if err := canjson.Unmarshal([]byte(in), got); err == nil {
			t.Fatal("Unmarshal succeeded, want a failure")
		}
		want := rm.Interval[rm.Integer]{Lower: 5, LowerIncluded: true, UpperUnbounded: true}
		if got.Interval != want {
			t.Errorf("receiver after the failed decode = %+v, want %+v", got.Interval, want)
		}
	})

	t.Run("DV_INTERVAL: a bad upper bound", func(t *testing.T) {
		got := &rm.DVInterval[rm.DVCount]{}
		in := `{"_type":"DV_INTERVAL","lower":{"_type":"DV_COUNT","magnitude":5},"lower_included":true,"upper":"x"}`
		if err := canjson.Unmarshal([]byte(in), got); err == nil {
			t.Fatal("Unmarshal succeeded, want a failure")
		}
		if got.Lower.Magnitude != 5 || !got.LowerIncluded {
			t.Errorf("receiver after the failed decode = lower %+v, lower_included %v, want magnitude 5 and true", got.Lower, got.LowerIncluded)
		}
	})
}
