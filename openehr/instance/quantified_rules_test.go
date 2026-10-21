package instance_test

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// The OPTs below leave a value open that an RM rule narrows, for the
// REQ-107 Precedence 3 rules on quantified values: DV_QUANTIFIED
// Magnitude_status_valid on a magnitude_status, the DV_AMOUNT rules
// Accuracy_is_percent_validity and Accuracy_validity on a percentage
// accuracy, the lower bound -1 of a precision, and the 32-bit range of an
// RM Integer on a DV_ORDINAL's value. Where nothing is left of the OPT's
// constraint, Generate fails with ErrConstraintUnsatisfiable. Neither
// validator evaluates Magnitude_status_valid or the accuracy rules, so the
// tests read the values.

// rmMagnitudeStatuses are the strings DV_QUANTIFIED Magnitude_status_valid
// admits.
var rmMagnitudeStatuses = []string{"=", "<", ">", "<=", ">=", "~"}

// quantifiedHolders are a value of each DV_QUANTIFIED class the generator
// builds, with the attributes its floor needs, and how to read its
// magnitude_status. attrs are the attributes the case adds.
var quantifiedHolders = []struct {
	name   string
	value  func(attrs ...string) string
	status func(v rm.DataValue) *string
}{
	{"DV_QUANTITY", func(a ...string) string { return quantityValue("4", a...) }, func(v rm.DataValue) *string { return v.(*rm.DVQuantity).MagnitudeStatus }},
	{"DV_COUNT", func(a ...string) string { return countValue("4", a...) }, func(v rm.DataValue) *string { return v.(*rm.DVCount).MagnitudeStatus }},
	{"DV_PROPORTION", func(a ...string) string { return proportionValue("4", a...) }, func(v rm.DataValue) *string { return v.(*rm.DVProportion).MagnitudeStatus }},
	{"DV_DATE", func(a ...string) string { return optNode("DV_DATE", "", a...) }, func(v rm.DataValue) *string { return v.(*rm.DVDate).MagnitudeStatus }},
	{"DV_TIME", func(a ...string) string { return optNode("DV_TIME", "", a...) }, func(v rm.DataValue) *string { return v.(*rm.DVTime).MagnitudeStatus }},
	{"DV_DATE_TIME", func(a ...string) string { return optNode("DV_DATE_TIME", "", a...) }, func(v rm.DataValue) *string { return v.(*rm.DVDateTime).MagnitudeStatus }},
	{"DV_DURATION", func(a ...string) string { return optNode("DV_DURATION", "", a...) }, func(v rm.DataValue) *string { return v.(*rm.DVDuration).MagnitudeStatus }},
}

// cString is a C_PRIMITIVE_OBJECT whose C_STRING carries body.
func cString(body string) string { return optPrimitive("STRING", "C_STRING", body) }

// TestREQ107_MagnitudeStatusIsOneTheRMAdmits is the REQ-107 Precedence 3
// check that a magnitude_status the OPT constrains with a C_STRING is one
// DV_QUANTIFIED Magnitude_status_valid admits, on each DV_QUANTIFIED class:
// an open C_STRING, or a pattern that admits any string, gives the default
// =; a pattern that admits only some of the strings gives the first of
// them its own example matches; a list keeps its member, ExampleFill its
// first, where Magnitude_status_valid admits it, and gives the first
// member it does admit otherwise. It holds under every policy, both value
// fills and both compile modes, and the root passes both validators.
func TestREQ107_MagnitudeStatusIsOneTheRMAdmits(t *testing.T) {
	cases := []struct {
		name string
		body string // the C_STRING body
		// want is the status every setting must give, and wantExample the
		// one ExampleFill must give where the settings differ.
		want, wantExample string
	}{
		{name: "open C_STRING", body: "", want: "="},
		{name: "pattern .*", body: "<pattern>.*</pattern>", want: "="},
		{name: "pattern that admits only some", body: "<pattern>[&lt;&gt;]=?</pattern>", want: "<"},
		{name: "list of two admitted strings", body: "<list>~</list><list>&lt;</list>", wantExample: "~"},
		{name: "list whose first member is not admitted", body: "<list>about</list><list>&lt;</list>", want: "<"},
	}
	for _, h := range quantifiedHolders {
		for _, tc := range cases {
			t.Run(h.name+"/"+tc.name, func(t *testing.T) {
				opt := optTemplate("ELEMENT", optSingle("value", h.value(optSingle("magnitude_status", cString(tc.body)))))
				generateEvery(t, opt, func(t *testing.T, call string, out any) {
					got := h.status(out.(*rm.Element).Value)
					if got == nil {
						t.Fatalf("%s: %s.magnitude_status is unset, want one Magnitude_status_valid admits", call, h.name)
					}
					if !slices.Contains(rmMagnitudeStatuses, *got) {
						t.Errorf("%s: %s.magnitude_status = %q, want one of %v (Magnitude_status_valid)", call, h.name, *got, rmMagnitudeStatuses)
					}
					if tc.want != "" && *got != tc.want {
						t.Errorf("%s: %s.magnitude_status = %q, want %q", call, h.name, *got, tc.want)
					}
					if tc.wantExample != "" && strings.Contains(call, "example") && *got != tc.wantExample {
						t.Errorf("%s: %s.magnitude_status = %q, want the list's first member %q", call, h.name, *got, tc.wantExample)
					}
				})
			})
		}
	}
}

// generateUnsatisfiable runs Generate on opt under every policy, both value
// fills and both compile modes, and fails t unless each call returns no
// root and an error wrapping ErrConstraintUnsatisfiable that names where,
// the RM type and the attribute, and the OPT path, and none of the values
// in valueFree.
func generateUnsatisfiable(t *testing.T, opt, where, path string, valueFree ...string) {
	t.Helper()
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range guardOptions() {
			call := fmt.Sprintf("Generate(%v, %v, implicit attributes %v)", opts.Policy, opts.ValueFill, implicit)
			out, err := instance.Generate(t.Context(), c, opts)
			if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
				t.Errorf("%s error = %v, want one wrapping ErrConstraintUnsatisfiable", call, err)
				continue
			}
			if out != nil {
				t.Errorf("%s returned %T, want no root", call, out)
			}
			if msg := err.Error(); !strings.Contains(msg, where+" at "+path) {
				t.Errorf("%s error = %q, want it to name %s at %s", call, msg, where, path)
			}
			for _, v := range valueFree {
				if strings.Contains(err.Error(), v) {
					t.Errorf("%s error = %q, want no value in it, but it names %q", call, err, v)
				}
			}
		}
	}
}

// TestREQ107_UnsatisfiableMagnitudeStatusFailsGenerate is the REQ-107
// Errors check that a C_STRING on a magnitude_status that admits none of
// the strings Magnitude_status_valid admits makes Generate return no root
// and an error wrapping ErrConstraintUnsatisfiable that names the RM type,
// the attribute and the OPT path, and not the value.
func TestREQ107_UnsatisfiableMagnitudeStatusFailsGenerate(t *testing.T) {
	opt := optTemplate("ELEMENT", optSingle("value", quantityValue("4", optSingle("magnitude_status", cString("<list>about</list>")))))
	generateUnsatisfiable(t, opt, "DV_QUANTITY.magnitude_status", "/value/magnitude_status", "about")
}

// amountHolders are a value of each DV_AMOUNT class the generator builds,
// with the attributes its floor needs, and how to read its accuracy and
// accuracy_is_percent.
var amountHolders = []struct {
	name     string
	value    func(attrs ...string) string
	accuracy func(v rm.DataValue) (*rm.Real, *bool)
}{
	{"DV_QUANTITY", func(a ...string) string { return quantityValue("4", a...) }, func(v rm.DataValue) (*rm.Real, *bool) {
		q := v.(*rm.DVQuantity)
		return q.Accuracy, q.AccuracyIsPercent
	}},
	{"DV_COUNT", func(a ...string) string { return countValue("4", a...) }, func(v rm.DataValue) (*rm.Real, *bool) {
		c := v.(*rm.DVCount)
		return c.Accuracy, c.AccuracyIsPercent
	}},
	{"DV_PROPORTION", func(a ...string) string { return proportionValue("4", a...) }, func(v rm.DataValue) (*rm.Real, *bool) {
		p := v.(*rm.DVProportion)
		return p.Accuracy, p.AccuracyIsPercent
	}},
	{"DV_DURATION", func(a ...string) string { return optNode("DV_DURATION", "", a...) }, func(v rm.DataValue) (*rm.Real, *bool) {
		d := v.(*rm.DVDuration)
		return d.Accuracy, d.AccuracyIsPercent
	}},
}

// realRange is a C_REAL over lower..upper, both ends included.
func realRange(lower, upper string) string {
	return optPrimitive("REAL", "C_REAL", "<range><lower_included>true</lower_included><upper_included>true</upper_included>"+
		"<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>"+
		"<lower>"+lower+"</lower><upper>"+upper+"</upper></range>")
}

// dvAnyBoolean is a C_BOOLEAN that admits true and false.
var dvAnyBoolean = optPrimitive("BOOLEAN", "C_BOOLEAN", "<true_valid>true</true_valid><false_valid>true</false_valid>")

// TestREQ107_PercentageAccuracyIsMoreThan0AndAtMost100 is the REQ-107
// Precedence 3 check that the accuracy and accuracy_is_percent the walk
// writes on each DV_AMOUNT class meet Accuracy_is_percent_validity (an
// accuracy of 0 is no percentage) and Accuracy_validity (a percentage is
// at most 100): a percentage accuracy the OPT leaves open is narrowed to
// more than 0 and at most 100, inside the OPT's constraint, to 1 where the
// example is 0, and to the middle of what the range leaves where 1 and 100
// are outside it; an accuracy the OPT pins to no such value makes
// accuracy_is_percent false where its C_BOOLEAN admits false; and an
// accuracy that is no percentage is left alone. It holds under every
// policy, both value fills and both compile modes, and the root passes
// both validators.
func TestREQ107_PercentageAccuracyIsMoreThan0AndAtMost100(t *testing.T) {
	cases := []struct {
		name              string
		accuracy, percent string // the C_REAL and the C_BOOLEAN
		wantPercent       bool
		lower, upper      float64 // the C_REAL's range, which the accuracy keeps to
		want, wantExample float64 // the accuracy every setting gives, and ExampleFill; 0 for none
	}{
		{name: "range from 0 to 200", accuracy: realRange("0", "200"), percent: dvOnlyTrue, wantPercent: true, lower: 0, upper: 200, wantExample: 1},
		{name: "range from 0 to 0.5", accuracy: realRange("0", "0.5"), percent: dvOnlyTrue, wantPercent: true, lower: 0, upper: 0.5, wantExample: 0.25},
		{name: "unbounded", accuracy: optPrimitive("REAL", "C_REAL", ""), percent: dvOnlyTrue, wantPercent: true, lower: math.Inf(-1), upper: math.Inf(1), wantExample: 1},
		{name: "list of 150 beside a free accuracy_is_percent", accuracy: dvReal("150"), percent: dvAnyBoolean, lower: 150, upper: 150, want: 150},
		{name: "list of 0 beside a free accuracy_is_percent", accuracy: dvReal("0"), percent: dvAnyBoolean, lower: 0, upper: 0},
		{name: "list of 150, no percentage", accuracy: dvReal("150"), percent: dvOnlyFalse, lower: 150, upper: 150, want: 150},
	}
	for _, h := range amountHolders {
		for _, tc := range cases {
			t.Run(h.name+"/"+tc.name, func(t *testing.T) {
				opt := optTemplate("ELEMENT", optSingle("value", h.value(
					optSingle("accuracy", tc.accuracy), optSingle("accuracy_is_percent", tc.percent))))
				generateEvery(t, opt, func(t *testing.T, call string, out any) {
					accuracy, percent := h.accuracy(out.(*rm.Element).Value)
					if accuracy == nil || percent == nil {
						t.Fatalf("%s: %s accuracy %v, accuracy_is_percent %v, want both set", call, h.name, accuracy, percent)
					}
					a := float64(*accuracy)
					if *percent != tc.wantPercent {
						t.Errorf("%s: %s.accuracy_is_percent = %t, want %t", call, h.name, *percent, tc.wantPercent)
					}
					if *percent && !(a > 0 && a <= 100) {
						t.Errorf("%s: %s.accuracy = %v as a percentage, want more than 0 and at most 100 (Accuracy_is_percent_validity, Accuracy_validity)", call, h.name, a)
					}
					if a < tc.lower || a > tc.upper {
						t.Errorf("%s: %s.accuracy = %v, want it inside the OPT's %v..%v", call, h.name, a, tc.lower, tc.upper)
					}
					if tc.want != 0 && a != tc.want {
						t.Errorf("%s: %s.accuracy = %v, want %v", call, h.name, a, tc.want)
					}
					if tc.wantExample != 0 && strings.Contains(call, "example") && a != tc.wantExample {
						t.Errorf("%s: %s.accuracy = %v, want %v", call, h.name, a, tc.wantExample)
					}
				})
			})
		}
	}
}

// TestREQ107_UnsatisfiablePercentageAccuracyFailsGenerate is the REQ-107
// Errors check that an OPT that holds accuracy_is_percent at true and
// admits no accuracy more than 0 and at most 100 makes Generate return no
// root and an error wrapping ErrConstraintUnsatisfiable that names the RM
// type, the attribute and the OPT path, and no value.
func TestREQ107_UnsatisfiablePercentageAccuracyFailsGenerate(t *testing.T) {
	for _, accuracy := range []struct{ name, constraint, value string }{
		{"list of 150", dvReal("150"), "150"},
		{"range below 0", realRange("-5", "0"), "-5"},
	} {
		t.Run(accuracy.name, func(t *testing.T) {
			opt := optTemplate("ELEMENT", optSingle("value", quantityValue("4",
				optSingle("accuracy", accuracy.constraint), optSingle("accuracy_is_percent", dvOnlyTrue))))
			generateUnsatisfiable(t, opt, "DV_QUANTITY.accuracy", "/value/accuracy", accuracy.value)
		})
	}
}

// intRange is a C_INTEGER over lower..upper, both ends included; an empty
// end is unbounded.
func intRange(lower, upper string) string { return intRangeOver(lower, upper, true) }

// intAbove is a C_INTEGER over lower..upper, lower excluded and upper
// included.
func intAbove(lower, upper string) string { return intRangeOver(lower, upper, false) }

// intRangeOver is a C_INTEGER over lower..upper, upper included and lower
// where lowerIncluded says so; an empty end is unbounded.
func intRangeOver(lower, upper string, lowerIncluded bool) string {
	body := fmt.Sprintf("<range><lower_included>%t</lower_included><upper_included>true</upper_included>", lowerIncluded)
	if lower == "" {
		body += "<lower_unbounded>true</lower_unbounded>"
	} else {
		body += "<lower_unbounded>false</lower_unbounded><lower>" + lower + "</lower>"
	}
	if upper == "" {
		body += "<upper_unbounded>true</upper_unbounded>"
	} else {
		body += "<upper_unbounded>false</upper_unbounded><upper>" + upper + "</upper>"
	}
	return optPrimitive("INTEGER", "C_INTEGER", body+"</range>")
}

// TestREQ107_PrecisionBelowMinusOneIsNarrowed is the REQ-107 Precedence 3
// check that a C_INTEGER on the precision of a DV_QUANTITY or a
// DV_PROPORTION that reaches below -1, the least precision the RM admits
// (-1 is no limit, § REQ-112), is narrowed to -1 and above: a range from
// -10 to 10 gives -1 under ExampleFill and a draw from -1 to 10 under
// RandomFill; a list keeps the members from -1 up; a range bounded above
// alone keeps its upper end as ExampleFill's value, as its own example
// value is; and a range that excludes -1 keeps it excluded. It holds under every policy, both value fills and both
// compile modes, and the root passes both validators.
func TestREQ107_PrecisionBelowMinusOneIsNarrowed(t *testing.T) {
	holders := []struct {
		name      string
		value     func(attrs ...string) string
		precision func(v rm.DataValue) *rm.Integer
	}{
		{"DV_QUANTITY", func(a ...string) string { return quantityValue("4", a...) }, func(v rm.DataValue) *rm.Integer { return v.(*rm.DVQuantity).Precision }},
		{"DV_PROPORTION", func(a ...string) string { return proportionValue("4", a...) }, func(v rm.DataValue) *rm.Integer { return v.(*rm.DVProportion).Precision }},
	}
	cases := []struct {
		name        string
		constraint  string
		lower       rm.Integer
		upper       rm.Integer
		wantExample rm.Integer
	}{
		{name: "range from -10 to 10", constraint: intRange("-10", "10"), lower: -1, upper: 10, wantExample: -1},
		{name: "list of -5 and 2", constraint: optPrimitive("INTEGER", "C_INTEGER", "<list>-5</list><list>2</list>"), lower: 2, upper: 2, wantExample: 2},
		{name: "range up to 5", constraint: intRange("", "5"), lower: -1, upper: 5, wantExample: 5},
		{name: "range above -1 to past the Integer range", constraint: intAbove("-1", "5000000000"), lower: 0, upper: math.MaxInt32, wantExample: 0},
	}
	for _, h := range holders {
		for _, tc := range cases {
			t.Run(h.name+"/"+tc.name, func(t *testing.T) {
				opt := optTemplate("ELEMENT", optSingle("value", h.value(optSingle("precision", tc.constraint))))
				generateEvery(t, opt, func(t *testing.T, call string, out any) {
					p := h.precision(out.(*rm.Element).Value)
					if p == nil {
						t.Fatalf("%s: %s.precision is unset", call, h.name)
					}
					if *p < tc.lower || *p > tc.upper {
						t.Errorf("%s: %s.precision = %d, want %d..%d", call, h.name, *p, tc.lower, tc.upper)
					}
					if strings.Contains(call, "example") && *p != tc.wantExample {
						t.Errorf("%s: %s.precision = %d, want %d", call, h.name, *p, tc.wantExample)
					}
				})
			})
		}
	}
	t.Run("range below -1", func(t *testing.T) {
		opt := optTemplate("ELEMENT", optSingle("value", quantityValue("4", optSingle("precision", intRange("-10", "-2")))))
		generateUnsatisfiable(t, opt, "DV_QUANTITY.precision", "/value/precision", "-10", "-2")
	})
}

// TestREQ107_OrdinalValueIsAnInteger is the REQ-107 Precedence 3 check
// that a DV_ORDINAL's value, a 32-bit RM Integer, is never truncated: a
// C_DV_ORDINAL keeps the pairs whose value is an Integer, and the walk
// writes the first of them under ExampleFill and one of them under
// RandomFill, with its symbol; a C_INTEGER on the value bounded above
// alone past the Integer range gives the greatest Integer under
// ExampleFill, the end on the side it bounds, one bounded below alone the
// least, and one that excludes the least Integer the one above it. It holds under every policy, both value fills and both
// compile modes, and the root passes both validators.
func TestREQ107_OrdinalValueIsAnInteger(t *testing.T) {
	t.Run("C_DV_ORDINAL with a pair past the Integer range", func(t *testing.T) {
		opt := optTemplate("ELEMENT", optSingle("value", ordinalPairs("5000000000/at0001", "3/at0002")))
		generateEvery(t, opt, func(t *testing.T, call string, out any) {
			o := dvValue[*rm.DVOrdinal](t, out)
			if o.Value != 3 || codeString(o.Symbol.DefiningCode) != "local::at0002" {
				t.Errorf("%s: DV_ORDINAL = %d %s, want 3 local::at0002", call, o.Value, codeString(o.Symbol.DefiningCode))
			}
		})
	})
	t.Run("C_DV_ORDINAL with no pair inside the Integer range", func(t *testing.T) {
		opt := optTemplate("ELEMENT", optSingle("value", ordinalPairs("5000000000/at0001", "-5000000000/at0002")))
		generateUnsatisfiable(t, opt, "DV_ORDINAL.value", "/value", "5000000000")
	})
	for _, tc := range []struct {
		name        string
		constraint  string
		wantExample rm.Integer
	}{
		{"C_INTEGER up to 5000000000", intRange("", "5000000000"), math.MaxInt32},
		{"C_INTEGER from -5000000000", intRange("-5000000000", ""), math.MinInt32},
		{"C_INTEGER above the least Integer to past the range", intAbove("-2147483648", "5000000000"), math.MinInt32 + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opt := optTemplate("ELEMENT", optSingle("value", optNode("DV_ORDINAL", "",
				optSingle("value", tc.constraint), optSingle("symbol", optCodedText("local", "at0001")))))
			generateEvery(t, opt, func(t *testing.T, call string, out any) {
				o := dvValue[*rm.DVOrdinal](t, out)
				if strings.Contains(call, "example") && o.Value != tc.wantExample {
					t.Errorf("%s: DV_ORDINAL.value = %d, want %d", call, o.Value, tc.wantExample)
				}
			})
		})
	}
}
