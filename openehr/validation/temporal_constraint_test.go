package validation_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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
)

// REQ-102, REQ-103 — the template validator reports a temporal value its
// OPT's ADL 1.4 pattern or range refuses, as a primitive_pattern_mismatch or
// a primitive_out_of_range: a DV_DATE with a day under yyyy-??-XX, a
// DV_DURATION outside the range of exactly 24 hours or of 0 to 150 years, and
// one with a designator its pattern does not name. A value both refuse gets
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
