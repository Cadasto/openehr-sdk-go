package instance_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// The OPTs below put one ordered data value under an ELEMENT and constrain
// the attributes every DV_ORDERED class declares, for the REQ-107 rules on
// leaf values, Placeholders and Intervals: a normal_range, a normal_status
// and an other_reference_ranges member, a REFERENCE_RANGE with a meaning
// and a range. They also put a DV_ORDINAL's and a DV_SCALE's symbol and
// value, and a DV_PROPORTION's accuracy_is_percent and magnitude_status,
// under nodes of their own. Each attribute comes out written as the OPT
// constrains it.

// ordInterval is a DV_INTERVAL<boundRM> whose lower and upper bounds are
// the OPT nodes lower and upper; an empty one leaves that bound out, so
// the side stays open.
func ordInterval(boundRM, lower, upper string) string {
	var attrs []string
	if lower != "" {
		attrs = append(attrs, optSingle("lower", lower))
	}
	if upper != "" {
		attrs = append(attrs, optSingle("upper", upper))
	}
	return optNode("DV_INTERVAL&lt;"+boundRM+"&gt;", "", attrs...)
}

// ordNormalStatus is a normal_status the OPT pins to N, the code of the
// openEHR normal statuses code set for a value in its normal range, so RM
// Normal_status_validity holds.
var ordNormalStatus = optSingle("normal_status", optCodePhrase("openehr_normal_statuses", "N"))

// ordReferenceRange is an other_reference_ranges with one REFERENCE_RANGE,
// whose meaning a C_STRING pins to "critical" and whose range is rng.
func ordReferenceRange(rng string) string {
	return optMultiple("other_reference_ranges", optNode("REFERENCE_RANGE", "",
		optSingle("meaning", optNode("DV_TEXT", "", optSingle("value", guardString("critical")))),
		optSingle("range", rng)))
}

// ordinalBound is a DV_ORDINAL whose C_DV_ORDINAL lists value with the
// local code alone.
func ordinalBound(value, code string) string { return ordinalPairs(value + "/" + code) }

// ordinalPairs is a DV_ORDINAL whose C_DV_ORDINAL lists the given
// value/local-code pairs.
func ordinalPairs(pairs ...string) string {
	var b strings.Builder
	b.WriteString(`<children xsi:type="C_DV_ORDINAL"><rm_type_name>DV_ORDINAL</rm_type_name><node_id></node_id>`)
	for _, p := range pairs {
		value, code, _ := strings.Cut(p, "/")
		fmt.Fprintf(&b, `<list><value>%s</value><symbol><value></value><defining_code><terminology_id><value>local</value></terminology_id><code_string>%s</code_string></defining_code></symbol></list>`, value, code)
	}
	b.WriteString(`</children>`)
	return b.String()
}

// scaleValue is a DV_SCALE whose value a C_REAL and whose symbol a
// C_CODE_PHRASE in local each pin to one member.
func scaleValue(value, code string, attrs ...string) string {
	return optNode("DV_SCALE", "", append([]string{
		optSingle("value", dvReal(value)),
		optSingle("symbol", optCodedText("local", code)),
	}, attrs...)...)
}

// ordinalValue is a DV_ORDINAL the OPT constrains as a node of its own: a
// C_INTEGER pins its value and a C_CODE_PHRASE in local its symbol.
func ordinalValue(value, code string, attrs ...string) string {
	return optNode("DV_ORDINAL", "", append([]string{
		optSingle("value", dvInteger(value)),
		optSingle("symbol", optCodedText("local", code)),
	}, attrs...)...)
}

// countValue is a DV_COUNT whose magnitude a C_INTEGER pins to n.
func countValue(n string, attrs ...string) string {
	return optNode("DV_COUNT", "", append([]string{optSingle("magnitude", dvInteger(n))}, attrs...)...)
}

// quantityValue is a DV_QUANTITY of magnitude m in mm.
func quantityValue(m string, attrs ...string) string {
	return optNode("DV_QUANTITY", "", append([]string{
		optSingle("magnitude", dvReal(m)), optSingle("units", guardString("mm")),
	}, attrs...)...)
}

// proportionValue is a unitary DV_PROPORTION of numerator n.
func proportionValue(n string, attrs ...string) string {
	return optNode("DV_PROPORTION", "", append([]string{
		optSingle("numerator", dvReal(n)), optSingle("denominator", dvReal("1")), optSingle("type", dvInteger("1")),
	}, attrs...)...)
}

// boundString renders a bound of a DV_INTERVAL as the checks compare it:
// its RM type and the value the order reads, with a leading * for a bound
// held by pointer, and "open" for no bound.
func boundString(b any) string {
	switch v := b.(type) {
	case nil:
		return "open"
	case rm.DVCount:
		return fmt.Sprintf("DV_COUNT %d", v.Magnitude)
	case rm.DVQuantity:
		return fmt.Sprintf("DV_QUANTITY %v %s", v.Magnitude, v.Units)
	case rm.DVProportion:
		return fmt.Sprintf("DV_PROPORTION %v/%v type %d", v.Numerator, v.Denominator, v.Type)
	case rm.DVOrdinal:
		return fmt.Sprintf("DV_ORDINAL %d %s", v.Value, codeString(v.Symbol.DefiningCode))
	case rm.DVScale:
		return fmt.Sprintf("DV_SCALE %v %s", v.Value, codeString(v.Symbol.DefiningCode))
	case rm.DVDate:
		return "DV_DATE " + v.Value
	case rm.DVTime:
		return "DV_TIME " + v.Value
	case rm.DVDateTime:
		return "DV_DATE_TIME " + v.Value
	case rm.DVDuration:
		return "DV_DURATION " + v.Value
	case *rm.DVCount:
		return "*" + boundString(*v)
	case *rm.DVQuantity:
		return "*" + boundString(*v)
	case *rm.DVProportion:
		return "*" + boundString(*v)
	case *rm.DVOrdinal:
		return "*" + boundString(*v)
	case *rm.DVScale:
		return "*" + boundString(*v)
	case *rm.DVDate:
		return "*" + boundString(*v)
	case *rm.DVTime:
		return "*" + boundString(*v)
	case *rm.DVDateTime:
		return "*" + boundString(*v)
	case *rm.DVDuration:
		return "*" + boundString(*v)
	}
	return fmt.Sprintf("%T", b)
}

// codeString renders a code phrase as terminology::code.
func codeString(cp rm.CodePhrase) string {
	return cp.TerminologyID.Value + "::" + cp.CodeString
}

// intervalString renders iv as [lower .. upper], or "none" for no
// interval. An open side renders as "open" where it holds no bound: nil
// over DV_ORDERED, the zero value over a concrete class.
func intervalString[T rm.DVOrdered](iv *rm.DVInterval[T]) string {
	if iv == nil {
		return "none"
	}
	side := func(b T, unbounded bool) string {
		switch {
		case !unbounded:
			return boundString(b)
		case any(b) == nil || reflect.ValueOf(b).IsZero():
			return "open"
		}
		return "open, holding " + boundString(b)
	}
	return "[" + side(iv.Lower, iv.LowerUnbounded) + " .. " + side(iv.Upper, iv.UpperUnbounded) + "]"
}

// orderedString renders the DV_ORDERED attributes of v: its normal_status,
// its normal_range and its other_reference_ranges, each member as its
// meaning and its range.
func orderedString(v rm.DataValue) string {
	var status *rm.CodePhrase
	var normal string
	var refs []string
	addRefs := func(meaning rm.DVTextLike, rng rm.DVInterval[rm.DVOrdered]) {
		text := "none"
		if meaning != nil && !rm.IsTypedNil(meaning) {
			text = meaning.GetValue()
		}
		refs = append(refs, text+": "+intervalString(&rng))
	}
	switch o := v.(type) {
	case *rm.DVCount:
		status, normal = o.NormalStatus, intervalString(o.NormalRange)
		for _, r := range o.OtherReferenceRanges {
			addRefs(r.Meaning, r.Range)
		}
	case *rm.DVQuantity:
		status, normal = o.NormalStatus, intervalString(o.NormalRange)
		for _, r := range o.OtherReferenceRanges {
			addRefs(r.Meaning, r.Range)
		}
	case *rm.DVProportion:
		status, normal = o.NormalStatus, intervalString(o.NormalRange)
		for _, r := range o.OtherReferenceRanges {
			addRefs(r.Meaning, r.Range)
		}
	case *rm.DVOrdinal:
		status, normal = o.NormalStatus, intervalString(o.NormalRange)
		for _, r := range o.OtherReferenceRanges {
			addRefs(r.Meaning, r.Range)
		}
	case *rm.DVScale:
		status, normal = o.NormalStatus, intervalString(o.NormalRange)
		for _, r := range o.OtherReferenceRanges {
			addRefs(r.Meaning, r.Range)
		}
	case *rm.DVDate:
		status, normal = o.NormalStatus, intervalString(o.NormalRange)
		for _, r := range o.OtherReferenceRanges {
			addRefs(r.Meaning, r.Range)
		}
	case *rm.DVTime:
		status, normal = o.NormalStatus, intervalString(o.NormalRange)
		for _, r := range o.OtherReferenceRanges {
			addRefs(r.Meaning, r.Range)
		}
	case *rm.DVDateTime:
		status, normal = o.NormalStatus, intervalString(o.NormalRange)
		for _, r := range o.OtherReferenceRanges {
			addRefs(r.Meaning, r.Range)
		}
	case *rm.DVDuration:
		status, normal = o.NormalStatus, intervalString(o.NormalRange)
		for _, r := range o.OtherReferenceRanges {
			addRefs(r.Meaning, r.Range)
		}
	default:
		return fmt.Sprintf("%T is no ordered value", v)
	}
	statusText := "none"
	if status != nil {
		statusText = codeString(*status)
	}
	return fmt.Sprintf("normal_status %s; normal_range %s; other_reference_ranges [%s]", statusText, normal, strings.Join(refs, "; "))
}

// simpleBound reports whether b, a bound of a REFERENCE_RANGE's range, is
// simple as DV_ORDERED is_simple reads it: it has no normal_range and no
// other_reference_ranges. A side with no bound is simple.
func simpleBound(b rm.DVOrdered) bool {
	switch v := b.(type) {
	case nil:
		return true
	case *rm.DVCount:
		return v.NormalRange == nil && v.OtherReferenceRanges == nil
	case *rm.DVQuantity:
		return v.NormalRange == nil && v.OtherReferenceRanges == nil
	case *rm.DVProportion:
		return v.NormalRange == nil && v.OtherReferenceRanges == nil
	case *rm.DVOrdinal:
		return v.NormalRange == nil && v.OtherReferenceRanges == nil
	case *rm.DVScale:
		return v.NormalRange == nil && v.OtherReferenceRanges == nil
	case *rm.DVDate:
		return v.NormalRange == nil && v.OtherReferenceRanges == nil
	case *rm.DVTime:
		return v.NormalRange == nil && v.OtherReferenceRanges == nil
	case *rm.DVDateTime:
		return v.NormalRange == nil && v.OtherReferenceRanges == nil
	case *rm.DVDuration:
		return v.NormalRange == nil && v.OtherReferenceRanges == nil
	}
	return false
}

// checkRangesSimple fails t unless the range of every member of v's
// other_reference_ranges holds simple bounds, so REFERENCE_RANGE
// Range_is_simple holds, which the RM floor does not evaluate.
func checkRangesSimple(t *testing.T, v rm.DataValue) {
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
	case *rm.DVProportion:
		for _, r := range o.OtherReferenceRanges {
			ranges = append(ranges, r.Range)
		}
	case *rm.DVOrdinal:
		for _, r := range o.OtherReferenceRanges {
			ranges = append(ranges, r.Range)
		}
	case *rm.DVScale:
		for _, r := range o.OtherReferenceRanges {
			ranges = append(ranges, r.Range)
		}
	case *rm.DVDate:
		for _, r := range o.OtherReferenceRanges {
			ranges = append(ranges, r.Range)
		}
	case *rm.DVTime:
		for _, r := range o.OtherReferenceRanges {
			ranges = append(ranges, r.Range)
		}
	case *rm.DVDateTime:
		for _, r := range o.OtherReferenceRanges {
			ranges = append(ranges, r.Range)
		}
	case *rm.DVDuration:
		for _, r := range o.OtherReferenceRanges {
			ranges = append(ranges, r.Range)
		}
	}
	for i, r := range ranges {
		lowerSimple := r.LowerUnbounded || simpleBound(r.Lower)
		upperSimple := r.UpperUnbounded || simpleBound(r.Upper)
		if !lowerSimple || !upperSimple {
			t.Errorf("other_reference_ranges[%d].range = %s breaks REFERENCE_RANGE Range_is_simple", i, intervalString(&r))
		}
	}
}

// orderedCase returns a dvCase for value, an ordered value the OPT gives a
// normal_range, a normal_status and an other_reference_ranges member, and
// whose DV_ORDERED attributes must render as want (orderedString).
func orderedCase(name, value, want string) dvCase {
	return dvCase{
		name:  name,
		value: value,
		check: func(t *testing.T, out any) {
			t.Helper()
			el, ok := out.(*rm.Element)
			if !ok {
				t.Fatalf("generated root is %T, want *rm.Element", out)
			}
			if got := orderedString(el.Value); got != want {
				t.Errorf("%s:\n got %s\nwant %s", name, got, want)
			}
			checkRangesSimple(t, el.Value)
		},
	}
}

// TestREQ107_OrderedValueAttributesAreWritten is the REQ-107 Leaf values
// and Intervals check that the normal_range, the normal_status and an
// other_reference_ranges member the OPT constrains on an ordered value are
// written, for each of the nine DV_ORDERED classes the generator builds.
// The OPT names each interval as DV_INTERVAL over the value's own class,
// and each range is held in the interval type the value's field holds:
// over its own class for a DV_COUNT, a DV_QUANTITY and a DV_PROPORTION,
// and over DV_ORDERED for the others, which then holds each bound by
// pointer. A side the OPT names no bound for stays open, with no bound. A
// REFERENCE_RANGE takes the meaning its C_STRING pins, and its range is
// held over DV_ORDERED, with bounds that are simple, so REFERENCE_RANGE
// Range_is_simple holds. It holds under every policy, both value fills,
// and with and without the implicit attributes, and the root passes both
// validators.
func TestREQ107_OrderedValueAttributesAreWritten(t *testing.T) {
	const status = "normal_status openehr_normal_statuses::N; "
	ordered := func(class, lower, upper, refLower, refUpper string) []string {
		return []string{
			optSingle("normal_range", ordInterval(class, lower, upper)),
			ordNormalStatus,
			ordReferenceRange(ordInterval(class, refLower, refUpper)),
		}
	}
	runDataValueCases(t, []dvCase{
		orderedCase("DV_COUNT",
			countValue("4", ordered("DV_COUNT", countValue("1"), countValue("9"), countValue("2"), "")...),
			status+"normal_range [DV_COUNT 1 .. DV_COUNT 9]; other_reference_ranges [critical: [*DV_COUNT 2 .. open]]"),
		orderedCase("DV_QUANTITY",
			quantityValue("4", ordered("DV_QUANTITY", quantityValue("1"), quantityValue("9"), "", quantityValue("20"))...),
			status+"normal_range [DV_QUANTITY 1 mm .. DV_QUANTITY 9 mm]; other_reference_ranges [critical: [open .. *DV_QUANTITY 20 mm]]"),
		orderedCase("DV_PROPORTION",
			proportionValue("4", ordered("DV_PROPORTION", proportionValue("1"), proportionValue("9"), proportionValue("2"), proportionValue("5"))...),
			status+"normal_range [DV_PROPORTION 1/1 type 1 .. DV_PROPORTION 9/1 type 1]; "+
				"other_reference_ranges [critical: [*DV_PROPORTION 2/1 type 1 .. *DV_PROPORTION 5/1 type 1]]"),
		orderedCase("DV_ORDINAL",
			ordinalValue("2", "at0002", ordered("DV_ORDINAL", ordinalBound("1", "at0001"), ordinalBound("3", "at0003"), ordinalBound("3", "at0003"), "")...),
			status+"normal_range [*DV_ORDINAL 1 local::at0001 .. *DV_ORDINAL 3 local::at0003]; "+
				"other_reference_ranges [critical: [*DV_ORDINAL 3 local::at0003 .. open]]"),
		orderedCase("DV_SCALE",
			scaleValue("1.5", "at0002", ordered("DV_SCALE", scaleValue("0.5", "at0001"), scaleValue("2.5", "at0003"), "", scaleValue("4", "at0004"))...),
			status+"normal_range [*DV_SCALE 0.5 local::at0001 .. *DV_SCALE 2.5 local::at0003]; "+
				"other_reference_ranges [critical: [open .. *DV_SCALE 4 local::at0004]]"),
		orderedCase("DV_DATE",
			optNode("DV_DATE", "", ordered("DV_DATE", optNode("DV_DATE", ""), optNode("DV_DATE", ""), optNode("DV_DATE", ""), "")...),
			status+"normal_range [*DV_DATE 2020-01-01 .. *DV_DATE 2020-01-01]; other_reference_ranges [critical: [*DV_DATE 2020-01-01 .. open]]"),
		orderedCase("DV_TIME",
			optNode("DV_TIME", "", ordered("DV_TIME", optNode("DV_TIME", ""), "", optNode("DV_TIME", ""), optNode("DV_TIME", ""))...),
			status+"normal_range [*DV_TIME 12:00:00 .. open]; other_reference_ranges [critical: [*DV_TIME 12:00:00 .. *DV_TIME 12:00:00]]"),
		orderedCase("DV_DATE_TIME",
			optNode("DV_DATE_TIME", "", ordered("DV_DATE_TIME", optNode("DV_DATE_TIME", ""), optNode("DV_DATE_TIME", ""), optNode("DV_DATE_TIME", ""), "")...),
			status+"normal_range [*DV_DATE_TIME 2021-03-04T05:06:07Z .. *DV_DATE_TIME 2021-03-04T05:06:07Z]; "+
				"other_reference_ranges [critical: [*DV_DATE_TIME 2021-03-04T05:06:07Z .. open]]"),
		orderedCase("DV_DURATION",
			optNode("DV_DURATION", "", ordered("DV_DURATION", "", optNode("DV_DURATION", ""), optNode("DV_DURATION", ""), optNode("DV_DURATION", ""))...),
			status+"normal_range [open .. *DV_DURATION P0D]; other_reference_ranges [critical: [*DV_DURATION P0D .. *DV_DURATION P0D]]"),
	})
}

// TestREQ107_OrdinalAndScaleTakeTheirSymbolAndValue is the REQ-107 Leaf
// values and Placeholders check that a DV_ORDINAL or a DV_SCALE the OPT
// constrains as a node of its own takes the symbol and the value the OPT
// constrains there: a symbol the OPT names as a DV_CODED_TEXT takes the
// code its C_CODE_PHRASE pins, a value takes the member its C_INTEGER or
// C_REAL lists, and a symbol the OPT names with no constraint takes the
// placeholder code at0000 in local. It holds under every policy, both value
// fills, and with and without the implicit attributes, and the root passes
// both validators.
func TestREQ107_OrdinalAndScaleTakeTheirSymbolAndValue(t *testing.T) {
	ordinal := func(value rm.Integer, code string) func(t *testing.T, out any) {
		return func(t *testing.T, out any) {
			t.Helper()
			o := dvValue[*rm.DVOrdinal](t, out)
			if o.Value != value || codeString(o.Symbol.DefiningCode) != code {
				t.Errorf("DV_ORDINAL = %d %s, want %d %s", o.Value, codeString(o.Symbol.DefiningCode), value, code)
			}
		}
	}
	scale := func(value rm.Real, code string) func(t *testing.T, out any) {
		return func(t *testing.T, out any) {
			t.Helper()
			s := dvValue[*rm.DVScale](t, out)
			if s.Value != value || codeString(s.Symbol.DefiningCode) != code {
				t.Errorf("DV_SCALE = %v %s, want %v %s", s.Value, codeString(s.Symbol.DefiningCode), value, code)
			}
		}
	}
	runDataValueCases(t, []dvCase{
		{name: "DV_ORDINAL", value: ordinalValue("3", "at0003"), check: ordinal(3, "local::at0003")},
		{
			name:  "DV_ORDINAL symbol named with no constraint",
			value: optNode("DV_ORDINAL", "", optSingle("value", dvInteger("2")), optSingle("symbol", optNode("DV_CODED_TEXT", ""))),
			check: ordinal(2, "local::at0000"),
		},
		{
			// The known-gap row TestREQ107_KnownGapUnwritableAttributes
			// pinned before: the symbol alone, as a node of its own. The
			// value the OPT leaves silent keeps the zero the RM type holds.
			name:  "DV_ORDINAL symbol alone as a node of its own",
			value: optNode("DV_ORDINAL", "", optSingle("symbol", optNode("DV_CODED_TEXT", ""))),
			check: ordinal(0, "local::at0000"),
		},
		{name: "DV_SCALE", value: scaleValue("1.5", "at0003"), check: scale(1.5, "local::at0003")},
		{
			name:  "DV_SCALE symbol named with no constraint",
			value: optNode("DV_SCALE", "", optSingle("value", dvReal("0.5")), optSingle("symbol", optNode("DV_CODED_TEXT", ""))),
			check: scale(0.5, "local::at0000"),
		},
	})
}

// TestREQ107_ProportionAmountAttributesAreWritten is the REQ-107 Leaf
// values check that a DV_PROPORTION's accuracy, accuracy_is_percent and
// magnitude_status the OPT constrains are written as their constraints
// pin them. It holds under every policy, both value fills, and with and
// without the implicit attributes, and the root passes both validators.
func TestREQ107_ProportionAmountAttributesAreWritten(t *testing.T) {
	runDataValueCases(t, []dvCase{
		{
			name: "DV_PROPORTION",
			value: proportionValue("4",
				optSingle("accuracy", dvReal("5")),
				optSingle("accuracy_is_percent", dvOnlyTrue),
				optSingle("magnitude_status", guardString("&lt;="))),
			check: func(t *testing.T, out any) {
				t.Helper()
				p := dvValue[*rm.DVProportion](t, out)
				if p.Accuracy == nil || *p.Accuracy != 5 {
					t.Errorf("DV_PROPORTION.accuracy = %v, want 5", p.Accuracy)
				}
				if p.AccuracyIsPercent == nil || !*p.AccuracyIsPercent {
					t.Errorf("DV_PROPORTION.accuracy_is_percent = %v, want true", p.AccuracyIsPercent)
				}
				checkHeldString(t, "DV_PROPORTION.magnitude_status", p.MagnitudeStatus, "<=")
			},
		},
	})
}

// TestREQ107_OrderedRangesTheOPTNamesWithNoChild is the REQ-107 RM
// defaults check that a normal_range the template requires and names with
// no child is built from its BMM type, a DV_INTERVAL open on both sides,
// and an other_reference_ranges it requires and names with no child gets
// one REFERENCE_RANGE with the open-string example "example" as its
// meaning and a range open on both sides. Each range is held in the
// interval type the value's field holds. It holds under every policy, both
// value fills, and with and without the implicit attributes, and the root
// passes both validators.
func TestREQ107_OrderedRangesTheOPTNamesWithNoChild(t *testing.T) {
	bare := []string{optSingle("normal_range"), optMultiple("other_reference_ranges")}
	const want = "normal_status none; normal_range [open .. open]; other_reference_ranges [example: [open .. open]]"
	runDataValueCases(t, []dvCase{
		orderedCase("DV_COUNT", countValue("4", bare...), want),
		orderedCase("DV_ORDINAL", ordinalValue("2", "at0002", bare...), want),
	})
}
