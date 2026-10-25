package validation_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// dvDateChild is a DV_DATE whose value a C_DATE of the given body
// constrains, the shape the test_all_types OPT writes.
func dvDateChild(body string) string {
	return `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_DATE</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>` + shortNameChild("DATE", "C_DATE", body) + `
        </attributes>
      </children>`
}

// durationRange is the body of an AOM 1.4 range from lower to upper, both
// included.
func durationRange(lower, upper string) string {
	return `<range>
            <lower_included>true</lower_included><upper_included>true</upper_included>
            <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
            <lower>` + lower + `</lower><upper>` + upper + `</upper>
          </range>`
}

// Temporal OPT children: the shapes and constraints the vendored OPTs write.
var (
	// partialDateChild admits a DV_DATE with no day (test_all_types).
	partialDateChild = dvDateChild("<pattern>yyyy-??-XX</pattern>")
	// widthChild admits a DURATION of exactly 24 hours (constrain_test).
	widthChild = shortNameChild("DURATION", "C_DURATION", durationRange("PT24H", "PT24H"))
	// ageChild admits a DURATION of 0 to 150 years, in years (family history).
	ageChild = shortNameChild("DURATION", "C_DURATION", "<pattern>PY</pattern>"+durationRange("P0Y", "P150Y"))
	// minutesChild admits a DURATION in minutes (minimal_instruction).
	minutesChild = shortNameChild("DURATION", "C_DURATION", "<pattern>PTM</pattern>")
	// stampChild admits a DATE_TIME with no seconds after 2004-05-20.
	stampChild = shortNameChild("DATE_TIME", "C_DATE_TIME", "<pattern>yyyy-mm-ddThh:mm:XX</pattern>"+`<range>
            <lower_included>false</lower_included>
            <lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded>
            <lower>2004-05-20T10:15:30</lower>
          </range>`)
	// zonedTimeChild admits a TIME with a zone: a synthetic C_TIME with the
	// ITS-XML timezone_validity 1001. No vendored OPT writes 1001 on a
	// C_TIME; sdk_crosscheck_values.v1 writes it on a C_DATE_TIME (at0008).
	zonedTimeChild = shortNameChild("TIME", "C_TIME", "<timezone_validity>1001</timezone_validity>")
	// localStampChild admits a DATE_TIME with no zone (timezone_validity
	// 1003).
	localStampChild = shortNameChild("DATE_TIME", "C_DATE_TIME", "<pattern>yyyy-mm-ddThh:mm:ss</pattern><timezone_validity>1003</timezone_validity>")
)

// REQ-102, REQ-103 — the template validator reports a temporal value its
// OPT's ADL 1.4 pattern or range refuses, as a primitive_pattern_mismatch or
// a primitive_out_of_range: a DV_DATE with a day under yyyy-??-XX, a
// DV_DURATION outside the range of exactly 24 hours or of 0 to 150 years, one
// with a designator its pattern does not name, and a DV_TIME or a
// DV_DATE_TIME that breaks its timezone_validity. A value both refuse gets
// both findings.
func TestValidate_REQ102_REQ103_TemporalPatternAndRange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		child string
		value rm.DataValue
		want  []pinnedIssue
	}{
		{name: "date with no day", child: partialDateChild, value: &rm.DVDate{Value: "2019-01"}},
		{
			name: "date with a day", child: partialDateChild, value: &rm.DVDate{Value: "20190114"},
			want: []pinnedIssue{templateFinding("pattern_mismatch", "/value/value")},
		},
		{name: "width of 24 hours", child: widthChild, value: &rm.DVDuration{Value: "PT24H"}},
		{name: "width of one day", child: widthChild, value: &rm.DVDuration{Value: "P1D"}},
		{
			name: "width of zero", child: widthChild, value: &rm.DVDuration{Value: "P0D"},
			want: []pinnedIssue{templateFinding("out_of_range", "/value")},
		},
		{name: "age in years", child: ageChild, value: &rm.DVDuration{Value: "P45Y"}},
		{
			name: "age in days", child: ageChild, value: &rm.DVDuration{Value: "P45D"},
			want: []pinnedIssue{templateFinding("pattern_mismatch", "/value")},
		},
		{
			name: "age over 150 years", child: ageChild, value: &rm.DVDuration{Value: "P151Y"},
			want: []pinnedIssue{templateFinding("out_of_range", "/value")},
		},
		{
			name: "age in hours and over the range", child: ageChild, value: &rm.DVDuration{Value: "-PT1H"},
			want: []pinnedIssue{templateFinding("out_of_range", "/value"), templateFinding("pattern_mismatch", "/value")},
		},
		{name: "minutes", child: minutesChild, value: &rm.DVDuration{Value: "PT5M"}},
		{
			name: "minutes and seconds", child: minutesChild, value: &rm.DVDuration{Value: "PT5M30S"},
			want: []pinnedIssue{templateFinding("pattern_mismatch", "/value")},
		},
		{name: "date-time with no seconds after the bound", child: stampChild, value: &rm.DVDateTime{Value: "2004-05-20T10:16Z"}},
		{
			name: "date-time at the excluded bound minute", child: stampChild, value: &rm.DVDateTime{Value: "2004-05-20T10:15"},
			want: []pinnedIssue{templateFinding("out_of_range", "/value")},
		},
		{
			name: "date-time with seconds", child: stampChild, value: &rm.DVDateTime{Value: "2004-05-20T10:16:00"},
			want: []pinnedIssue{templateFinding("pattern_mismatch", "/value")},
		},
		{name: "time with a zone where one is mandatory", child: zonedTimeChild, value: &rm.DVTime{Value: "10:30:00+01:00"}},
		{
			name: "time with no zone where one is mandatory", child: zonedTimeChild, value: &rm.DVTime{Value: "10:30:00"},
			want: []pinnedIssue{templateFinding("pattern_mismatch", "/value")},
		},
		{name: "date-time with no zone where one is disallowed", child: localStampChild, value: &rm.DVDateTime{Value: "2004-05-20T10:16:00"}},
		{
			name: "date-time with a zone where one is disallowed", child: localStampChild, value: &rm.DVDateTime{Value: "2004-05-20T10:16:00Z"},
			want: []pinnedIssue{templateFinding("pattern_mismatch", "/value")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(tc.child))
			got := pinnedIssues(validation.Validate(valueElement(tc.value), c).Issues)
			sortPinned(tc.want)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%+v) issues =\n  %+v\nwant\n  %+v", tc.value, got, tc.want)
			}
		})
	}
}

// REQ-107, REQ-103 — the generator values a temporal leaf so its output
// validates under both value fills: the example and the random draw keep to
// the OPT's pattern, and a range the draw cannot see falls back to the
// example, which keeps to it too.
func TestGenerate_REQ107_REQ103_TemporalLeavesValidate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, child string }{
		{"partial date", partialDateChild},
		{"width", widthChild},
		{"age", ageChild},
		{"minutes", minutesChild},
		{"date-time", stampChild},
		{"zoned time", zonedTimeChild},
		{"local date-time", localStampChild},
	} {
		c := mustCompileInline(t, elementValueOPT(tc.child))
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			t.Run(fmt.Sprintf("%s/%s", tc.name, fill), func(t *testing.T) {
				t.Parallel()
				out, err := instance.Generate(t.Context(), c, instance.Options{
					Policy:      instance.Large,
					ValueFill:   fill,
					ValueSource: rand.NewPCG(7, 1),
				})
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				el, ok := out.(*rm.Element)
				if !ok {
					t.Fatalf("Generate root = %T, want *rm.Element", out)
				}
				if got := issueKeys(validation.Validate(el, c).Issues); len(got) != 0 {
					t.Errorf("Validate(generated %+v) issues = %q, want none", el.Value, got)
				}
			})
		}
	}
}

// orderCase is one temporal type for TestREQ103TemporalRangeAgreesWithREQ112Order.
type orderCase struct {
	name string
	// admits reports whether the type's constraint with the range r
	// admits s.
	admits func(r constraints.TemporalRange, s string) bool
	// above reports whether the floor puts a above b: whether the
	// DV_INTERVAL from a to b breaks Limits_consistent.
	above func(a, b string) bool
	// clockAbove reports whether a is above b by the clock-local magnitude
	// of the RM's Compare (REQ-123), with no zone applied.
	clockAbove func(a, b string) bool
	// zoned reports whether s carries a time zone; nil for a type with
	// no zones.
	zoned   func(s string) bool
	samples []string
}

// limitsBroken reports whether the closed interval from lower to upper
// breaks Limits_consistent under the RM floor (REQ-112).
func limitsBroken[T rm.DVOrdered](lower, upper T) bool {
	return len(limitsIssues(validation.ValidateRM(rm.NewClosedDVInterval(lower, upper)))) > 0
}

// TestREQ103TemporalRangeAgreesWithREQ112Order holds a REQ-103 TemporalRange
// to the order REQ-112 gives the two bounds of a DV_INTERVAL, which the floor
// reports through Limits_consistent. For each pair of samples a and b, a
// range from b, included, admits a exactly when the floor does not put b
// above a; a range from b, excluded, exactly when it puts a above b; a range
// up to b, included, exactly when it does not put a above b; and a range up
// to b, excluded, exactly when it puts b above a. A zoned value and one
// without a zone are not ordered: the floor puts neither above the other,
// and each range admits a. The samples mix full and partial
// values, both layouts, zones on neither, one or both sides, a leap second,
// and the to_seconds averages of a year and a month. For times and
// date-times some zoned pairs order one way by their clocks and the other as
// instants, so a clock-local range check fails the test.
func TestREQ103TemporalRangeAgreesWithREQ112Order(t *testing.T) {
	t.Parallel()
	tests := []orderCase{
		{
			name: "DV_DATE",
			admits: func(r constraints.TemporalRange, s string) bool {
				return len(constraints.CDate{Range: r}.Validate(s)) == 0
			},
			above: func(a, b string) bool { return limitsBroken(rm.DVDate{Value: a}, rm.DVDate{Value: b}) },
			clockAbove: func(a, b string) bool {
				return (&rm.DVDate{Value: a}).Compare(rm.DVDate{Value: b}) > 0
			},
			samples: []string{
				"2004", "2004-05", "2004-05-01", "2004-05-20", "20040521", "200406", "2005-01-01",
				"0001-01-01", "2024-02-29",
			},
		},
		{
			name: "DV_TIME",
			admits: func(r constraints.TemporalRange, s string) bool {
				return len(constraints.CTime{Range: r}.Validate(s)) == 0
			},
			above: func(a, b string) bool { return limitsBroken(rm.DVTime{Value: a}, rm.DVTime{Value: b}) },
			clockAbove: func(a, b string) bool {
				return (&rm.DVTime{Value: a}).Compare(rm.DVTime{Value: b}) > 0
			},
			zoned: func(s string) bool { return (&rm.DVTime{Value: s}).Timezone() != "" },
			samples: []string{
				"09", "09:30", "09:30:00", "09:30:00.5", "0930", "23:59:60",
				"09:30:00+05:00", "09:00:00-05:00", "00:00:00Z", "14:00Z", "08:00:00Z", "07:30:00Z",
				"09:30:00+02:00", "0930+0200", "10:00:00+0200", "23:59:60Z", "09:30+05:30",
			},
		},
		{
			name: "DV_DATE_TIME",
			admits: func(r constraints.TemporalRange, s string) bool {
				return len(constraints.CDateTime{Range: r}.Validate(s)) == 0
			},
			above: func(a, b string) bool { return limitsBroken(rm.DVDateTime{Value: a}, rm.DVDateTime{Value: b}) },
			clockAbove: func(a, b string) bool {
				return (&rm.DVDateTime{Value: a}).Compare(rm.DVDateTime{Value: b}) > 0
			},
			zoned: func(s string) bool { return (&rm.DVDateTime{Value: s}).Timezone() != "" },
			samples: []string{
				"2004-05-20", "2004-05-20T00", "2004-05-20T00:00:00", "20040520T100000", "2004-05",
				"2004-05-20T23:59:59.9", "2004", "2004-05-20T10:00:00+02:00", "2004-05-20T09:00:00Z",
				"2004-05-20T08:00:00Z", "2004-05-21T00:30:00+01:00", "2004-05-20T23:30Z",
				"20040520T100000-0500", "2016-12-31T23:59:60Z", "2017-01-01T00:59:60+01:00",
			},
		},
		{
			name: "DV_DURATION",
			admits: func(r constraints.TemporalRange, s string) bool {
				return len(constraints.CDuration{Range: r}.Validate(s)) == 0
			},
			above: func(a, b string) bool { return limitsBroken(rm.DVDuration{Value: a}, rm.DVDuration{Value: b}) },
			clockAbove: func(a, b string) bool {
				return (&rm.DVDuration{Value: a}).Compare(rm.DVDuration{Value: b}) > 0
			},
			samples: []string{
				"P0D", "PT24H", "P1D", "P1Y", "P12M", "P365D", "P366D", "P52W", "-P1D", "PT0.5S",
				"P4Y", "P48M", "P1461D", "-PT0S",
				// A month and a year at exactly their Time_definitions
				// averages, so each pair compares equal.
				"P1M", "P30DT10H4M48S", "P365DT5H45M36S",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var admitted, refused, unordered, clockDisagrees int
			for _, b := range tc.samples {
				from := constraints.TemporalRange{Lower: b, LowerInclusive: true, UpperUnbounded: true}
				after := constraints.TemporalRange{Lower: b, UpperUnbounded: true}
				upTo := constraints.TemporalRange{LowerUnbounded: true, Upper: b, UpperInclusive: true}
				before := constraints.TemporalRange{LowerUnbounded: true, Upper: b}
				for _, a := range tc.samples {
					aAbove, bAbove := tc.above(a, b), tc.above(b, a)
					wantFrom, wantAfter, wantUpTo, wantBefore := !bAbove, aAbove, !aAbove, bAbove
					if tc.zoned != nil && tc.zoned(a) != tc.zoned(b) {
						unordered++
						if aAbove || bAbove {
							t.Errorf("the floor orders %q against %q, want a zoned value not ordered against one without a zone", a, b)
						}
						wantFrom, wantAfter, wantUpTo, wantBefore = true, true, true, true
					} else if aAbove != tc.clockAbove(a, b) {
						clockDisagrees++
					}
					for _, check := range []struct {
						r    constraints.TemporalRange
						want bool
					}{{from, wantFrom}, {after, wantAfter}, {upTo, wantUpTo}, {before, wantBefore}} {
						got := tc.admits(check.r, a)
						if got != check.want {
							t.Errorf("%s range %s: Validate(%q) admits = %t, want %t (the floor puts %q above %q: %t, %q above %q: %t)",
								tc.name, check.r, a, got, check.want, a, b, aAbove, b, a, bAbove)
						}
						if got {
							admitted++
						} else {
							refused++
						}
					}
				}
			}
			if admitted == 0 || refused == 0 {
				t.Errorf("%s samples: the ranges admit %d and refuse %d, want at least one of each", tc.name, admitted, refused)
			}
			if tc.zoned != nil && (unordered == 0 || clockDisagrees == 0) {
				t.Errorf("%s samples: %d unordered pairs and %d pairs whose clocks and instants disagree, want at least one of each", tc.name, unordered, clockDisagrees)
			}
		})
	}
}
