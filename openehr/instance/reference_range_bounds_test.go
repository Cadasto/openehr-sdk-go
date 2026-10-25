package instance_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The OPTs below put a DV_COUNT or a DV_QUANTITY under an ELEMENT with one
// REFERENCE_RANGE in its other_reference_ranges, for the REQ-107 Precedence
// 2 rule that an RM rule wins over the OPT: a bound of a REFERENCE_RANGE's
// range carries no normal_range and no other_reference_ranges, so
// REFERENCE_RANGE Range_is_simple holds, which neither validator evaluates.
// The OPT names those two attributes on the bounds, as optional and as
// required, the other_reference_ranges by their cardinality alone, and the
// generator writes neither. A bound whose OPT requires the normal_range
// therefore fails the template validator, which reports that attribute
// absent and nothing else: a list that only its cardinality asks a member
// of is not required by the template while it is absent (REQ-102).

// boundReferenceRanges is a bound's other_reference_ranges over children,
// with a cardinality of 1..* and no existence.
func boundReferenceRanges(children ...string) string {
	return optCardinal("other_reference_ranges", 1, -1, children...)
}

// refRangeNode is a REFERENCE_RANGE whose meaning a C_STRING pins to
// "critical" and whose range is rng.
func refRangeNode(rng string) string {
	return optNode("REFERENCE_RANGE", "",
		optSingle("meaning", optNode("DV_TEXT", "", optSingle("value", guardString("critical")))),
		optSingle("range", rng))
}

// simpleBoundClass is one ordered class a REFERENCE_RANGE's bounds take
// in the cases below.
type simpleBoundClass struct {
	class string
	// value is an ordered value of the class with magnitude n and the
	// given attributes.
	value func(n string, attrs ...string) string
	// text renders a bound of magnitude n as boundString renders it.
	text func(n string) string
}

// simpleBoundClasses are the two classes the cases run over, held by
// pointer in the DV_ORDERED interval a REFERENCE_RANGE's range is.
var simpleBoundClasses = []simpleBoundClass{
	{
		class: "DV_COUNT",
		value: countValue,
		text:  func(n string) string { return "*DV_COUNT " + n },
	},
	{
		class: "DV_QUANTITY",
		value: quantityValue,
		text:  func(n string) string { return "*DV_QUANTITY " + n + " mm" },
	},
}

// simpleBoundShape is one way the OPT names normal_range or
// other_reference_ranges on both bounds of a REFERENCE_RANGE's range.
type simpleBoundShape struct {
	name string
	// attrs returns the attributes the OPT gives each bound of class c.
	attrs func(c simpleBoundClass) []string
	// requiresNormalRange is set where the OPT requires the bound's
	// normal_range, which the generator leaves absent all the same.
	requiresNormalRange bool
}

// simpleBoundShapes name the two attributes as optional, as required, and
// as required with no child, which the generator builds from the BMM.
var simpleBoundShapes = []simpleBoundShape{
	{name: "optional normal_range", attrs: func(c simpleBoundClass) []string {
		return []string{guardSingle("normal_range", guardExistence01, ordInterval(c.class, c.value("1"), c.value("9")))}
	}},
	{name: "required normal_range", requiresNormalRange: true, attrs: func(c simpleBoundClass) []string {
		return []string{optSingle("normal_range", ordInterval(c.class, c.value("1"), c.value("9")))}
	}},
	{name: "required normal_range with no child", requiresNormalRange: true, attrs: func(simpleBoundClass) []string {
		return []string{optSingle("normal_range")}
	}},
	{name: "optional other_reference_ranges", attrs: func(c simpleBoundClass) []string {
		return []string{guardMultiple("other_reference_ranges", guardExistence01, refRangeNode(ordInterval(c.class, c.value("3"), "")))}
	}},
	{name: "required other_reference_ranges", attrs: func(c simpleBoundClass) []string {
		return []string{boundReferenceRanges(refRangeNode(ordInterval(c.class, c.value("3"), "")))}
	}},
	{name: "required other_reference_ranges with no child", attrs: func(simpleBoundClass) []string {
		return []string{boundReferenceRanges()}
	}},
	{name: "both required", requiresNormalRange: true, attrs: func(c simpleBoundClass) []string {
		return []string{
			optSingle("normal_range", ordInterval(c.class, c.value("1"), c.value("9"))),
			boundReferenceRanges(refRangeNode(ordInterval(c.class, c.value("3"), ""))),
		}
	}},
}

// simpleBoundCase is one ELEMENT value the cases below generate, and the
// errors the template validator is expected to report on the root.
type simpleBoundCase struct {
	dvCase
	// expected are the template validator's reports of an attribute the OPT
	// requires and the generator leaves absent.
	expected []expectedIssue
}

// normalRangeAbsent expects the template validator's report that the
// normal_range of the bound at side, lower or upper, of the range of the
// first REFERENCE_RANGE is absent.
func normalRangeAbsent(side string) expectedIssue {
	suffix := "/range/" + side + "/normal_range"
	return expectedIssue{
		name: "the " + side + " bound's required normal_range absent",
		match: func(iss validation.Issue) bool {
			return iss.Code == "required" && strings.HasSuffix(iss.Path, suffix)
		},
	}
}

// runSimpleBoundCases generates each case under every policy, both value
// fills, and with and without the implicit attributes, checks what it
// asserts, and checks the root against both validators, which report only
// what the case expects.
func runSimpleBoundCases(t *testing.T, cases []simpleBoundCase) {
	t.Helper()
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			for _, opts := range optionsFor(nil) {
				opts.Language = dvLanguage
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v, want a root", err)
					}
					tc.check(t, out)
					checkValidBut(t, out, c, tc.expected...)
				})
			}
		}
	}
}

// referenceRangeBounds returns the bounds of the one REFERENCE_RANGE in the
// other_reference_ranges of v, a DV_COUNT or a DV_QUANTITY.
func referenceRangeBounds(t *testing.T, v rm.DataValue) (lower, upper rm.DVOrdered) {
	t.Helper()
	var ranges []rm.DVInterval[rm.DVOrdered]
	switch o := v.(type) {
	case *rm.DVCount:
		for _, r := range o.OtherReferenceRanges {
			ranges = append(ranges, r.Range)
		}
	case *rm.DVQuantity:
		for _, r := range o.OtherReferenceRanges {
			ranges = append(ranges, r.Range)
		}
	default:
		t.Fatalf("ELEMENT.value is %T, want a *rm.DVCount or a *rm.DVQuantity", v)
	}
	if len(ranges) != 1 {
		t.Fatalf("other_reference_ranges holds %d members, want 1", len(ranges))
	}
	return ranges[0].Lower, ranges[0].Upper
}

// TestREQ107_ReferenceRangeBoundsCarryNoReferenceRanges is the REQ-107
// Precedence 2 check that a bound of a REFERENCE_RANGE's range carries no
// normal_range and no other_reference_ranges, whether the OPT names them
// optional, required or required with no child, so REFERENCE_RANGE
// Range_is_simple holds. The range keeps both bounds at the values the OPT
// pins, and the value around it takes nothing of them. A DV_COUNT's
// other_reference_ranges holds the REFERENCE_RANGE, over DV_COUNT and over
// DV_QUANTITY bounds. It holds under every policy, both value fills, and
// with and without the implicit attributes; the root passes the RM floor,
// and the template validator reports only the normal_range the OPT requires
// of a bound, which the RM keeps absent.
func TestREQ107_ReferenceRangeBoundsCarryNoReferenceRanges(t *testing.T) {
	var cases []simpleBoundCase
	for _, class := range simpleBoundClasses {
		for _, shape := range simpleBoundShapes {
			rng := ordInterval(class.class, class.value("2", shape.attrs(class)...), class.value("8", shape.attrs(class)...))
			want := "normal_status none; normal_range none; other_reference_ranges [critical: [" + class.text("2") + " .. " + class.text("8") + "]]"
			tc := simpleBoundCase{dvCase: orderedCase(class.class+" bounds with "+shape.name, class.value("4", ordReferenceRange(rng)), want)}
			if shape.requiresNormalRange {
				tc.expected = []expectedIssue{normalRangeAbsent("lower"), normalRangeAbsent("upper")}
			}
			cases = append(cases, tc)
		}
	}
	runSimpleBoundCases(t, cases)
}

// TestREQ107_ReferenceRangeBoundKeepsItsNormalStatus is the REQ-107
// Precedence 2 check that a bound of a REFERENCE_RANGE's range keeps the
// normal_status the OPT pins on it where the generator writes no
// normal_range on it: a normal_status alone is not a reference range, and
// Range_is_simple reads only the normal_range and the other_reference_ranges.
// It holds under every policy, both value fills, and with and without the
// implicit attributes; the root passes the RM floor, and the template
// validator reports only the normal_range the OPT requires of each bound.
func TestREQ107_ReferenceRangeBoundKeepsItsNormalStatus(t *testing.T) {
	bound := func(n string) string {
		return countValue(n, ordNormalStatus, optSingle("normal_range", ordInterval("DV_COUNT", countValue("1"), countValue("9"))))
	}
	runSimpleBoundCases(t, []simpleBoundCase{{
		name:  "DV_COUNT bounds with a normal_status and a normal_range",
		value: countValue("4", ordReferenceRange(ordInterval("DV_COUNT", bound("2"), bound("8")))),
		check: func(t *testing.T, out any) {
			t.Helper()
			el, ok := out.(*rm.Element)
			if !ok {
				t.Fatalf("generated root is %T, want *rm.Element", out)
			}
			checkRangesSimple(t, el.Value)
			lower, upper := referenceRangeBounds(t, el.Value)
			for _, side := range []struct {
				name  string
				bound rm.DVOrdered
			}{{"lower", lower}, {"upper", upper}} {
				c, ok := side.bound.(*rm.DVCount)
				if !ok {
					t.Fatalf("range %s is %T, want a *rm.DVCount", side.name, side.bound)
				}
				if c.NormalStatus == nil || codeString(*c.NormalStatus) != "openehr_normal_statuses::N" {
					t.Errorf("range %s normal_status = %v, want openehr_normal_statuses::N", side.name, c.NormalStatus)
				}
				if c.NormalRange != nil {
					t.Errorf("range %s normal_range = %s, want none", side.name, intervalString(c.NormalRange))
				}
			}
		},
		expected: []expectedIssue{normalRangeAbsent("lower"), normalRangeAbsent("upper")},
	}})
}
