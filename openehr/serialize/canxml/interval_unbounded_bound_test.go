package canxml_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// xmlIntervalBoundCases are interval elements in which a side's open flag
// depends on its bound, each with the flags REQ-169 says the decoded interval
// carries. Every reader decodes them, whether it holds its bounds by value or
// by reference.
var xmlIntervalBoundCases = []struct {
	name     string
	children []xmlChild
	want     intervalFlags
}{
	{
		name:     "only an upper bound: open below",
		children: []xmlChild{{"upper", "$upper"}},
		want:     intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:     "only a lower bound: open above",
		children: []xmlChild{{"lower", "$lower"}},
		want:     intervalFlags{LowerIncluded: true, UpperUnbounded: true},
	},
	{
		name:     "no bound and no flag on either side: open on both",
		children: nil,
		want:     intervalFlags{LowerUnbounded: true, UpperUnbounded: true},
	},
	{
		name:     "an open side written true beside its bound is read as written",
		children: []xmlChild{{"lower", "$lower"}, {"upper", "$upper"}, {"lower_unbounded", "true"}},
		want:     intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
}

// xmlClosedSideCases are interval elements with a side written bounded,
// `*_unbounded` false, whose bound element is absent. side names it.
var xmlClosedSideCases = []struct {
	name     string
	children []xmlChild
	side     string
	// want is the flags a reader that holds its bounds by reference decodes.
	want intervalFlags
}{
	{
		name:     "lower written bounded, lower absent",
		children: []xmlChild{{"upper", "$upper"}, {"lower_unbounded", "false"}},
		side:     "lower",
		want:     closedFlags,
	},
	{
		name:     "upper written bounded and excluded, upper absent",
		children: []xmlChild{{"lower", "$lower"}, {"upper_unbounded", "false"}, {"upper_included", "false"}},
		side:     "upper",
		want:     intervalFlags{LowerIncluded: true},
	},
}

// xmlDecodeModes are the three ways a canonical-XML decode can run.
func xmlDecodeModes() map[string][]canxml.DecoderOption {
	return map[string][]canxml.DecoderOption{
		"default":       nil,
		"WithUnmatched": {canxml.WithUnmatched(func(string) {})},
		"WithStrict":    {canxml.WithStrict()},
	}
}

// TestREQ169IntervalUnboundedFollowsItsBound pins the reading of an absent
// `*_unbounded` in canonical XML (REQ-169): false beside a bound, and true
// where the bound element is absent too, so an interval that gives only an
// upper bound is open below; the `*_included` default then follows the side's
// open flag. A flag that is present is read as written, `*_unbounded` true
// beside a bound among them. Every reader is covered, by value and by
// reference.
//
// Can-fail control: read an absent `*_unbounded` as false whatever the bound,
// as before, and every case here that lacks a bound reads a closed side.
func TestREQ169IntervalUnboundedFollowsItsBound(t *testing.T) {
	for _, r := range xmlIntervalReaders() {
		for _, tc := range xmlIntervalBoundCases {
			t.Run(r.name+"/"+tc.name, func(t *testing.T) {
				doc := fmt.Sprintf(r.doc, xmlIntervalBody(tc.children, r.lower, r.upper))
				got, err := r.decode([]byte(doc))
				if err != nil {
					t.Fatalf("canxml.Unmarshal(%s) = %v, want no error", doc, err)
				}
				if got != tc.want {
					t.Errorf("canxml.Unmarshal(%s) flags = %+v, want %+v", doc, got, tc.want)
				}
			})
		}
	}
}

// TestREQ169IntervalClosedSideWithNoBound pins the refusal of a side written
// `*_unbounded` false whose bound element is absent (REQ-169). Where the SDK
// holds the bound by value, every mode refuses the document with
// canxml.ErrInvalidShape and an error that names the side's flag. Where it
// holds the bound as an interface, the side decodes as written, with no bound.
//
// Can-fail control: drop the refusal and the by-value readers decode a zero
// bound on a closed side, so the error assertions fail.
func TestREQ169IntervalClosedSideWithNoBound(t *testing.T) {
	for _, r := range xmlIntervalReaders() {
		for _, tc := range xmlClosedSideCases {
			doc := fmt.Sprintf(r.doc, xmlIntervalBody(tc.children, r.lower, r.upper))
			if r.boundByRef {
				t.Run(r.name+"/"+tc.name+"/decoded with no bound", func(t *testing.T) {
					for mode, opts := range xmlDecodeModes() {
						got, err := r.decode([]byte(doc), opts...)
						if err != nil {
							t.Fatalf("canxml.Unmarshal(%s, %s) = %v, want no error: the bound is held by reference", doc, mode, err)
						}
						if got != tc.want {
							t.Errorf("canxml.Unmarshal(%s, %s) flags = %+v, want %+v", doc, mode, got, tc.want)
						}
					}
				})
				continue
			}
			t.Run(r.name+"/"+tc.name+"/refused", func(t *testing.T) {
				for mode, opts := range xmlDecodeModes() {
					_, err := r.decode([]byte(doc), opts...)
					if !errors.Is(err, canxml.ErrInvalidShape) {
						t.Fatalf("canxml.Unmarshal(%s, %s) = %v, want an error wrapping canxml.ErrInvalidShape", doc, mode, err)
					}
					if flag := tc.side + "_unbounded"; !strings.Contains(err.Error(), flag) {
						t.Errorf("canxml.Unmarshal(%s, %s) = %v, want the error to name %s", doc, mode, err, flag)
					}
				}
			})
		}
	}
}

// TestREQ169IntervalOnlyUpperBoundRoundTrips pins that an interval that gives
// only an upper bound, read as open below, re-encodes with `lower_unbounded`
// true, `lower_included` false and no lower element, by value and by
// reference.
func TestREQ169IntervalOnlyUpperBoundRoundTrips(t *testing.T) {
	doc := `<dv_interval` + xmlNS + `>` + xmlCount5 + `</dv_interval>`
	for _, tc := range []struct {
		name string
		into any
	}{
		{name: "DV_INTERVAL<DV_COUNT>", into: new(rm.DVInterval[rm.DVCount])},
		{name: "DV_INTERVAL<DV_ORDERED>", into: new(rm.DVInterval[rm.DVOrdered])},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := canxml.Unmarshal([]byte(doc), tc.into); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v", doc, err)
			}
			out, err := canxml.Marshal(tc.into)
			if err != nil {
				t.Fatalf("canxml.Marshal: %v", err)
			}
			got := string(out)
			if strings.Contains(got, "<lower>") || strings.Contains(got, "<lower ") {
				t.Errorf("re-encoded %s carries a lower element, want none: the side is open", got)
			}
			for _, want := range []string{"<lower_unbounded>true</lower_unbounded>", "<lower_included>false</lower_included>"} {
				if !strings.Contains(got, want) {
					t.Errorf("re-encoded %s lacks %s", got, want)
				}
			}
		})
	}
}

// TestREQ169IntervalBoundByReferenceRoundTripsWithNoBound pins that a side
// written bounded with no bound leaves an interface-held bound nil, and that
// the encoder writes the side back as it was read: the flag false and no
// bound element.
func TestREQ169IntervalBoundByReferenceRoundTripsWithNoBound(t *testing.T) {
	doc := `<dv_interval` + xmlNS + `>` + xmlCount5 + `<lower_unbounded>false</lower_unbounded></dv_interval>`
	var v rm.DVInterval[rm.DVOrdered]
	if err := canxml.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("canxml.Unmarshal(%s) = %v, want no error", doc, err)
	}
	if v.Lower != nil {
		t.Errorf("canxml.Unmarshal(%s) lower = %#v, want nil: the document gives no lower bound", doc, v.Lower)
	}
	out, err := canxml.Marshal(&v)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	got := string(out)
	if strings.Contains(got, "<lower>") || strings.Contains(got, "<lower ") {
		t.Errorf("re-encoded %s carries a lower element, want none", got)
	}
	for _, want := range []string{"<lower_unbounded>false</lower_unbounded>", "<lower_included>true</lower_included>"} {
		if !strings.Contains(got, want) {
			t.Errorf("re-encoded %s lacks %s", got, want)
		}
	}
}
