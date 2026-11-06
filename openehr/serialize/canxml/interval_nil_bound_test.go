package canxml_test

// interval_nil_bound_test.go: REQ-169 (wire.md § REQ-169) for an interval
// bound sent as xsi:nil, canonical XML's null. A null bound is an absent one:
// an absent `*_unbounded` on its side reads as true and an absent
// `*_included` as false, and a side marked `*_unbounded` false refuses it
// where the bound is held by value, as canonical JSON's null does (REQ-056).

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// nilledBound returns bound, one bound element, nilled and empty: its start
// tag, attributes kept, with xsi:nil set and closed at once.
func nilledBound(bound string) string {
	return bound[:strings.Index(bound, ">")] + ` xsi:nil="true"/>`
}

// nilBoundCases are interval children that send one bound nilled ($nil), the
// other bound with a value, and maybe the nilled side's `*_unbounded`.
var nilBoundCases = []struct {
	name     string
	children []xmlChild
}{
	{name: "nilled lower", children: []xmlChild{{"lower", "$nil"}, {"upper", "$upper"}}},
	{name: "nilled upper", children: []xmlChild{{"lower", "$lower"}, {"upper", "$nil"}}},
	{name: "nilled lower marked bounded", children: []xmlChild{{"lower", "$nil"}, {"upper", "$upper"}, {"lower_unbounded", "false"}}},
	{name: "nilled upper marked bounded", children: []xmlChild{{"lower", "$lower"}, {"upper", "$nil"}, {"upper_unbounded", "false"}}},
}

// TestREQ169NilledBoundReadsAsAbsent decodes, for every interval reader, each
// case with the bound nilled and with the bound's element left out, with and
// without WithStrict: the two decode to the same flags and the same refusal,
// none where the side is not marked bounded, and canxml.ErrInvalidShape where
// it is and the reader holds the bound by value. A nilled bound held as an
// interface needs no xsi:type.
func TestREQ169NilledBoundReadsAsAbsent(t *testing.T) {
	for _, r := range xmlIntervalReaders() {
		for _, tc := range nilBoundCases {
			nilled := nilledBound(r.lower)
			if strings.Contains(tc.name, "upper") {
				nilled = nilledBound(r.upper)
			}
			forms := map[string]string{"nilled": nilled}
			if r.boundByRef {
				side := "lower"
				if strings.Contains(tc.name, "upper") {
					side = "upper"
				}
				forms["nilled, no xsi:type"] = `<` + side + ` xsi:nil="true"/>`
			}
			for form, bound := range forms {
				t.Run(r.name+"/"+tc.name+"/"+form, func(t *testing.T) {
					var withNil, without strings.Builder
					for _, c := range tc.children {
						if c.value == "$nil" {
							withNil.WriteString(bound)
							continue
						}
						one := xmlIntervalBody([]xmlChild{c}, r.lower, r.upper)
						withNil.WriteString(one)
						without.WriteString(one)
					}
					docNil := fmt.Sprintf(r.doc, withNil.String())
					docAbsent := fmt.Sprintf(r.doc, without.String())
					for _, opts := range [][]canxml.DecoderOption{nil, {canxml.WithStrict()}} {
						got, errNil := r.decode([]byte(docNil), opts...)
						want, errAbsent := r.decode([]byte(docAbsent), opts...)
						if (errNil == nil) != (errAbsent == nil) || got != want {
							t.Errorf("%d options: canxml.Unmarshal(%s) = %+v, %v; want what the document without the bound gives, %+v, %v", len(opts), docNil, got, errNil, want, errAbsent)
						}
						bounded := strings.HasSuffix(tc.name, "marked bounded")
						switch {
						case bounded && !r.boundByRef && !errors.Is(errNil, canxml.ErrInvalidShape):
							t.Errorf("%d options: canxml.Unmarshal(%s) = %v; want canxml.ErrInvalidShape: a side marked bounded has no bound, which a bound held by value cannot leave out", len(opts), docNil, errNil)
						case (!bounded || r.boundByRef) && errNil != nil:
							t.Errorf("%d options: canxml.Unmarshal(%s) = %v; want nil", len(opts), docNil, errNil)
						}
					}
				})
			}
		}
	}
}
