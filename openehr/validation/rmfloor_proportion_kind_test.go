package validation_test

// rmfloor_proportion_kind_test.go: REQ-112 DV_PROPORTION — the
// proportion-kind rules of the RM floor. The pinned RM declares five
// invariants on DV_PROPORTION beside its precision ones: Type_validity (type
// is a PROPORTION_KIND, 0 to 4), Unitary_validity (a unitary proportion has
// denominator 1), Percent_validity (a percent has denominator 100),
// Fraction_validity (a fraction or an integer fraction has whole-number
// operands) and Valid_denominator (no denominator is 0). Each breached rule
// is its own rm_invariant issue on the DV_PROPORTION, told apart by Rule.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The rules of DV_PROPORTION, as the floor names them in Issue.Rule.
const (
	proportionType      = "DV_PROPORTION.Type_validity"
	proportionUnitary   = "DV_PROPORTION.Unitary_validity"
	proportionPercent   = "DV_PROPORTION.Percent_validity"
	proportionFraction  = "DV_PROPORTION.Fraction_validity"
	proportionZero      = "DV_PROPORTION.Valid_denominator"
	proportionPrecision = "DV_PROPORTION.Precision_validity"
)

// decodeProportion decodes, through the type registry, the canonical JSON
// DV_PROPORTION that carries members, as a server receives it.
func decodeProportion(t *testing.T, members string) any {
	t.Helper()
	body := `{"_type":"DV_PROPORTION",` + members + `}`
	v, err := typereg.Default.Decode([]byte(body))
	if err != nil {
		t.Fatalf("typereg.Decode(%s): %v", body, err)
	}
	if _, ok := v.(*rm.DVProportion); !ok {
		t.Fatalf("typereg.Decode(%s) = %T, want *rm.DVProportion", body, v)
	}
	return v
}

// TestValidateRM_REQ112_DVProportionKindRules pins each proportion-kind rule
// with a value that breaks it and one that keeps it, and how the rules
// combine on one value: every rule the value breaks is one issue of its own,
// and no rule is reported twice. Each issue is rm_invariant at the
// DV_PROPORTION, on the RM layer, with the rule as Rule.
func TestValidateRM_REQ112_DVProportionKindRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		members string
		want    []string
	}{
		// Type_validity: valid_proportion_kind (type).
		{name: "type 5 is no proportion kind", members: `"numerator":1,"denominator":2,"type":5`, want: []string{proportionType}},
		{name: "type -1 is no proportion kind", members: `"numerator":1,"denominator":2,"type":-1`, want: []string{proportionType}},
		{name: "type 0 is a ratio", members: `"numerator":1.5,"denominator":2.5,"type":0`},
		{name: "type 4 is an integer fraction", members: `"numerator":3,"denominator":2,"type":4`},

		// Unitary_validity: type = pk_unitary implies denominator = 1.
		{name: "unitary over denominator 2", members: `"numerator":1,"denominator":2,"type":1`, want: []string{proportionUnitary}},
		{name: "unitary over denominator 1", members: `"numerator":0.5,"denominator":1,"type":1`},

		// Percent_validity: type = pk_percent implies denominator = 100.
		{name: "percent over denominator 1", members: `"numerator":1,"denominator":1,"type":2`, want: []string{proportionPercent}},
		{name: "percent over denominator 100", members: `"numerator":12.5,"denominator":100,"type":2`},

		// Fraction_validity: (type = pk_fraction or type =
		// pk_integer_fraction) implies is_integral, read as whole-number
		// operands. One issue however many operands are not whole.
		{name: "fraction with a fractional numerator", members: `"numerator":1.5,"denominator":2,"type":3`, want: []string{proportionFraction}},
		{name: "fraction with a fractional denominator", members: `"numerator":1,"denominator":2.5,"type":3`, want: []string{proportionFraction}},
		{name: "fraction with both operands fractional", members: `"numerator":1.5,"denominator":2.5,"type":3`, want: []string{proportionFraction}},
		{name: "integer fraction with a fractional denominator", members: `"numerator":3,"denominator":2.5,"type":4`, want: []string{proportionFraction}},
		{name: "fraction with whole operands", members: `"numerator":1,"denominator":2,"type":3`},

		// Valid_denominator: denominator /= 0.0.
		{name: "ratio over denominator 0", members: `"numerator":1,"denominator":0,"type":0`, want: []string{proportionZero}},
		{name: "ratio over a negative denominator", members: `"numerator":1,"denominator":-4,"type":0`},

		// Combinations: each breached rule reports once.
		{name: "unitary over denominator 0", members: `"numerator":1,"denominator":0,"type":1`, want: []string{proportionUnitary, proportionZero}},
		{name: "percent over denominator 0", members: `"numerator":1,"denominator":0,"type":2`, want: []string{proportionPercent, proportionZero}},
		{name: "fraction over denominator 0", members: `"numerator":1,"denominator":0,"type":3`, want: []string{proportionZero}},
		{name: "no kind over denominator 0", members: `"numerator":1,"denominator":0,"type":7`, want: []string{proportionType, proportionZero}},
		{name: "no kind leaves the kind rules alone", members: `"numerator":1.5,"denominator":2.5,"type":7`, want: []string{proportionType}},
		{name: "fraction of precision 0 with a fractional numerator", members: `"numerator":1.5,"denominator":2,"type":3,"precision":0`, want: []string{proportionPrecision, proportionFraction}},
		{name: "percent of precision 0 with a fractional numerator", members: `"numerator":12.5,"denominator":100,"type":2,"precision":0`, want: []string{proportionPrecision}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := validation.ValidateRM(decodeProportion(t, tc.members))
			var got []string
			for _, i := range r.Issues {
				if i.Code != "rm_invariant" {
					t.Errorf("ValidateRM({%s}) reports %s at %q (%s), want only rm_invariant", tc.members, i.Code, i.Path, i.Rule)
					continue
				}
				if i.Path != "/" || i.Layer != validation.LayerRM || i.Detail == "" {
					t.Errorf("ValidateRM({%s}) rm_invariant %s at %q on layer %s with Detail %q, want it at / on the RM layer with a Detail", tc.members, i.Rule, i.Path, i.Layer, i.Detail)
				}
				got = append(got, i.Rule)
			}
			slices.Sort(got)
			want := slices.Sorted(slices.Values(tc.want))
			if !slices.Equal(got, want) {
				t.Errorf("ValidateRM({%s}) rules = %q, want %q", tc.members, got, want)
			}
			if r.OK != (len(want) == 0) {
				t.Errorf("ValidateRM({%s}).OK = %t, want %t", tc.members, r.OK, len(want) == 0)
			}
		})
	}
}

// TestValidateComposition_REQ112_PercentOverDenominatorOne is the issue-291
// reproduction: a composition holding the percent
// {"numerator":1,"denominator":1,"type":2}, which openEHR servers refuse
// under Percent_validity, is not valid under the RM rules alone nor under
// its template, and each reports the breach once, at the value.
func TestValidateComposition_REQ112_PercentOverDenominatorOne(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	proportion, ok := decodeProportion(t, `"numerator":1,"denominator":1,"type":2`).(rm.DataValue)
	if !ok {
		t.Fatal("a decoded *rm.DVProportion is no rm.DataValue")
	}
	deviceElement(t, comp).Value = proportion
	path := devicePath + "/value"
	for name, r := range map[string]validation.Result{
		"ValidateRM":          validation.ValidateRM(comp),
		"ValidateComposition": validation.ValidateComposition(comp, c),
	} {
		if r.OK {
			t.Errorf("%s(composition holding a percent over denominator 1).OK = true, want false", name)
		}
		n := 0
		for _, i := range r.Issues {
			if i.Path == path && i.Code == "rm_invariant" && i.Layer == validation.LayerRM && i.Rule == proportionPercent {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s(composition holding a percent over denominator 1) reports rm_invariant %s at %s %d times, want once; issues %q", name, proportionPercent, path, n, issueKeys(r.Issues))
		}
	}
}
