package canjson_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// jsonIntervalBoundCases are interval objects in which a side's open flag
// depends on its bound, each with the flags REQ-169 says the decoded interval
// carries. Every reader decodes them, whether it holds its bounds by value or
// by reference.
var jsonIntervalBoundCases = []struct {
	name    string
	members []jsonMember
	want    intervalFlags
}{
	{
		name:    "only an upper bound: open below",
		members: []jsonMember{{"upper", "$upper"}},
		want:    intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:    "only a lower bound: open above",
		members: []jsonMember{{"lower", "$lower"}},
		want:    intervalFlags{LowerIncluded: true, UpperUnbounded: true},
	},
	{
		name:    "a null lower bound reads as absent: open below",
		members: []jsonMember{{"lower", "null"}, {"upper", "$upper"}},
		want:    intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:    "no bound and no flag on either side: open on both",
		members: []jsonMember{{"lower_included", "null"}},
		want:    intervalFlags{LowerUnbounded: true, UpperUnbounded: true},
	},
	{
		name:    "an open side written true beside its bound is read as written",
		members: []jsonMember{{"lower", "$lower"}, {"lower_unbounded", "true"}, {"upper", "$upper"}},
		want:    intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
}

// jsonClosedSideCases are interval objects with a side written bounded,
// `*_unbounded: false`, whose bound is absent or null. side names it.
var jsonClosedSideCases = []struct {
	name    string
	members []jsonMember
	side    string
	// want is the flags a reader that holds its bounds by reference decodes.
	want intervalFlags
}{
	{
		name:    "lower written bounded, lower absent",
		members: []jsonMember{{"lower_unbounded", "false"}, {"upper", "$upper"}},
		side:    "lower",
		want:    closedFlags,
	},
	{
		name:    "lower written bounded, lower null",
		members: []jsonMember{{"lower", "null"}, {"lower_unbounded", "false"}, {"upper", "$upper"}},
		side:    "lower",
		want:    closedFlags,
	},
	{
		name:    "upper written bounded before a null upper",
		members: []jsonMember{{"upper_unbounded", "false"}, {"upper", "null"}, {"lower", "$lower"}},
		side:    "upper",
		want:    closedFlags,
	},
	{
		name:    "upper written bounded and excluded, upper absent",
		members: []jsonMember{{"lower", "$lower"}, {"upper_unbounded", "false"}, {"upper_included", "false"}},
		side:    "upper",
		want:    intervalFlags{LowerIncluded: true},
	},
}

// jsonDecodeModes are the three ways a canonical-JSON decode can run.
func jsonDecodeModes() map[string][]canjson.DecoderOption {
	return map[string][]canjson.DecoderOption{
		"default":       nil,
		"WithUnmatched": {canjson.WithUnmatched(func(string) {})},
		"WithStrict":    {canjson.WithStrict()},
	}
}

// TestREQ169IntervalUnboundedFollowsItsBound pins the reading of an absent
// `*_unbounded` (REQ-169): false beside a bound, and true where the bound is
// absent or null too, so an interval that gives only an upper bound is open
// below; the `*_included` default then follows the side's open flag. A flag
// that is present is read as written, `*_unbounded: true` beside a bound among
// them. Every reader is covered, by value and by reference.
//
// Can-fail control: read an absent `*_unbounded` as false whatever the bound,
// as before, and every case here that lacks a bound reads a closed side.
func TestREQ169IntervalUnboundedFollowsItsBound(t *testing.T) {
	for _, r := range jsonIntervalReaders() {
		for _, tc := range jsonIntervalBoundCases {
			t.Run(r.name+"/"+tc.name, func(t *testing.T) {
				doc := fmt.Sprintf(r.doc, jsonIntervalBody(tc.members, r.lower, r.upper))
				got, err := r.decode([]byte(doc))
				if err != nil {
					t.Fatalf("canjson.Unmarshal(%s) = %v, want no error", doc, err)
				}
				if got == nil {
					t.Fatalf("canjson.Unmarshal(%s) holds no interval", doc)
				}
				if *got != tc.want {
					t.Errorf("canjson.Unmarshal(%s) flags = %+v, want %+v", doc, *got, tc.want)
				}
			})
		}
	}
}

// TestREQ169IntervalClosedSideWithNoBound pins the refusal of a side written
// `*_unbounded: false` whose bound is absent or null (REQ-169). Where the SDK
// holds the bound by value, the Go value cannot hold the absence, so every
// mode refuses the document with canjson.ErrInvalidShape and an error that
// names the side's flag. Where it holds the bound as an interface, the side
// decodes as written, with no bound.
//
// Can-fail control: drop the refusal and the by-value readers decode a zero
// bound on a closed side, so the error assertions fail.
func TestREQ169IntervalClosedSideWithNoBound(t *testing.T) {
	for _, r := range jsonIntervalReaders() {
		for _, tc := range jsonClosedSideCases {
			doc := fmt.Sprintf(r.doc, jsonIntervalBody(tc.members, r.lower, r.upper))
			if r.boundByRef {
				t.Run(r.name+"/"+tc.name+"/decoded with no bound", func(t *testing.T) {
					for mode, opts := range jsonDecodeModes() {
						got, err := r.decode([]byte(doc), opts...)
						if err != nil {
							t.Fatalf("canjson.Unmarshal(%s, %s) = %v, want no error: the bound is held by reference", doc, mode, err)
						}
						if got == nil || *got != tc.want {
							t.Errorf("canjson.Unmarshal(%s, %s) flags = %s, want %+v", doc, mode, sprintFlags(got), tc.want)
						}
					}
				})
				continue
			}
			t.Run(r.name+"/"+tc.name+"/refused", func(t *testing.T) {
				for mode, opts := range jsonDecodeModes() {
					_, err := r.decode([]byte(doc), opts...)
					if !errors.Is(err, canjson.ErrInvalidShape) {
						t.Fatalf("canjson.Unmarshal(%s, %s) = %v, want an error wrapping canjson.ErrInvalidShape", doc, mode, err)
					}
					if flag := tc.side + "_unbounded"; !strings.Contains(err.Error(), flag) {
						t.Errorf("canjson.Unmarshal(%s, %s) = %v, want the error to name %s", doc, mode, err, flag)
					}
				}
			})
		}
	}
}

// TestREQ169IntervalClosedSideWithNoBoundOnTheStream pins the same refusal on
// Decoder.Decode, with and without the member options, whose route reads the
// whole value before decoding it.
func TestREQ169IntervalClosedSideWithNoBoundOnTheStream(t *testing.T) {
	const doc = `{"_type":"DV_INTERVAL","lower_unbounded":false,"upper":` + jsonCount5 + `}`
	for mode, opts := range jsonDecodeModes() {
		var v rm.DVInterval[rm.DVCount]
		err := canjson.NewDecoder(strings.NewReader(doc), opts...).Decode(&v)
		if !errors.Is(err, canjson.ErrInvalidShape) {
			t.Errorf("Decoder.Decode(%s, %s) = %v, want an error wrapping canjson.ErrInvalidShape", doc, mode, err)
		}
	}
}

// TestREQ169IntervalBoundByReferenceDecodesWithNoBound pins that a side written
// bounded with no bound leaves an interface-held bound nil: nothing is filled
// in for it. The encoder writes the side back as it was read: the flag false
// and no bound.
func TestREQ169IntervalBoundByReferenceDecodesWithNoBound(t *testing.T) {
	const doc = `{"_type":"DV_INTERVAL","lower_unbounded":false,"upper":` + jsonCount5 + `}`
	var v rm.DVInterval[rm.DVOrdered]
	if err := canjson.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("canjson.Unmarshal(%s) = %v, want no error", doc, err)
	}
	if v.Lower != nil {
		t.Errorf("canjson.Unmarshal(%s) lower = %#v, want nil: the document gives no lower bound", doc, v.Lower)
	}
	if got := baseFlags(v.Interval); got != closedFlags {
		t.Errorf("canjson.Unmarshal(%s) flags = %+v, want %+v", doc, got, closedFlags)
	}
	out, err := canjson.Marshal(&v)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("re-read %s: %v", out, err)
	}
	if _, ok := got["lower"]; ok || got["lower_unbounded"] != false || got["lower_included"] != true {
		t.Errorf("re-encoded %s, want lower_unbounded false, lower_included true and no lower member", out)
	}
}

// TestREQ169IntervalOnlyUpperBoundRoundTrips pins that an interval that gives
// only an upper bound, read as open below, re-encodes with `lower_unbounded`
// true, `lower_included` false and no lower member, by value and by
// reference.
func TestREQ169IntervalOnlyUpperBoundRoundTrips(t *testing.T) {
	const doc = `{"_type":"DV_INTERVAL","upper":` + jsonCount5 + `}`
	for _, tc := range []struct {
		name string
		into any
	}{
		{name: "DV_INTERVAL<DV_COUNT>", into: new(rm.DVInterval[rm.DVCount])},
		{name: "DV_INTERVAL<DV_ORDERED>", into: new(rm.DVInterval[rm.DVOrdered])},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := canjson.Unmarshal([]byte(doc), tc.into); err != nil {
				t.Fatalf("canjson.Unmarshal(%s) = %v", doc, err)
			}
			out, err := canjson.Marshal(tc.into)
			if err != nil {
				t.Fatalf("canjson.Marshal: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("re-read %s: %v", out, err)
			}
			if _, ok := got["lower"]; ok {
				t.Errorf("re-encoded %s carries a lower member, want none: the side is open", out)
			}
			if got["lower_unbounded"] != true || got["lower_included"] != false {
				t.Errorf("re-encoded %s: lower_unbounded = %v, lower_included = %v, want true and false", out, got["lower_unbounded"], got["lower_included"])
			}
		})
	}
}
