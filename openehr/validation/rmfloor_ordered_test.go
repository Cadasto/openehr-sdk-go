package validation_test

// rmfloor_ordered_test.go: REQ-112 — the RM floor walks DV_ORDINAL, DV_SCALE,
// REFERENCE_RANGE and EHR_ACCESS like any other modelled class, so their
// RM-mandatory attributes are checked by the required-set walk and the
// catalogue evaluators run on what sits below them. Each row lists the exact
// findings, by code and path.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// codedSymbol returns a complete DV_CODED_TEXT symbol for code.
func codedSymbol(code string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        code,
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: code},
	}
}

// scoreElement returns a complete ELEMENT holding value.
func scoreElement(value rm.DataValue) *rm.Element {
	return &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "score"}, Value: value}
}

// TestValidateRM_OrdinalAndScaleWalked checks that a DV_ORDINAL or DV_SCALE
// held as an ELEMENT value is walked (REQ-112): symbol is RM-mandatory on
// both, so an absent one is `required` at /value/symbol, and the symbol
// itself is walked, so its CODE_PHRASE is checked too. Interval bounds of
// these types are covered by TestValidateRM_TypedIntervalBoundsWalked.
func TestValidateRM_OrdinalAndScaleWalked(t *testing.T) {
	emptyCode := codedSymbol("at1")
	emptyCode.DefiningCode.CodeString = ""
	cases := []struct {
		name string
		root any
		want []string
	}{
		{
			name: "DV_ORDINAL without symbol",
			root: scoreElement(&rm.DVOrdinal{Value: 2}),
			want: []string{"required /value/symbol"},
		},
		{
			name: "DV_SCALE without symbol",
			root: scoreElement(&rm.DVScale{Value: 2.5}),
			want: []string{"required /value/symbol"},
		},
		{
			name: "DV_ORDINAL whose symbol has an empty code_string",
			root: scoreElement(&rm.DVOrdinal{Value: 2, Symbol: emptyCode}),
			want: []string{
				"required /value/symbol/defining_code/code_string",
				"rm_invariant /value/symbol/defining_code",
			},
		},
		{
			name: "DV_ORDINAL complete",
			root: scoreElement(&rm.DVOrdinal{Value: 2, Symbol: codedSymbol("at2")}),
		},
		{
			name: "DV_SCALE complete, value form",
			root: scoreElement(rm.DVScale{Value: 0.5, Symbol: codedSymbol("at1")}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findingsOf(validation.ValidateRM(tc.root)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) findings = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestValidateRM_ReferenceRangesWalked checks that the walk reaches a
// REFERENCE_RANGE under DV_ORDERED.other_reference_ranges, on the typed and
// the bare instantiations, and checks it (REQ-112): meaning and range are
// RM-mandatory, and range is an interval whose bounds are walked and ordered
// like any other. The DV_DATE rows show that the date/time types reach their
// DV_ORDERED attributes too, normal_status included.
func TestValidateRM_ReferenceRangesWalked(t *testing.T) {
	badPrecision := rm.Integer(-5)
	openUpper := func(lower rm.DVOrdered) rm.DVInterval[rm.DVOrdered] {
		return rm.DVInterval[rm.DVOrdered]{Lower: lower, LowerIncluded: true, UpperUnbounded: true}
	}
	cases := []struct {
		name string
		root any
		want []string
	}{
		{
			name: "DV_QUANTITY, empty meaning and a fault inside the range bound",
			root: &rm.DVQuantity{Magnitude: 5, Units: "mmol/L", OtherReferenceRanges: []rm.ReferenceRange[rm.DVQuantity]{{
				Meaning: rm.DVText{},
				Range:   openUpper(rm.DVQuantity{Magnitude: 3, Units: "mmol/L", Precision: &badPrecision}),
			}}},
			want: []string{
				"required /other_reference_ranges[0]/meaning",
				"rm_invariant /other_reference_ranges[0]/range/lower",
			},
		},
		{
			name: "DV_QUANTITY, range absent",
			root: &rm.DVQuantity{Magnitude: 5, Units: "mmol/L", OtherReferenceRanges: []rm.ReferenceRange[rm.DVQuantity]{{
				Meaning: rm.DVText{Value: "critical"},
			}}},
			want: []string{"required /other_reference_ranges[0]/range"},
		},
		{
			name: "DV_COUNT, range bounds inverted",
			root: &rm.DVCount{Magnitude: 3, OtherReferenceRanges: []rm.ReferenceRange[rm.DVCount]{{
				Meaning: rm.DVText{Value: "normal"},
				Range: rm.DVInterval[rm.DVOrdered]{
					Lower: rm.DVCount{Magnitude: 10}, LowerIncluded: true,
					Upper: rm.DVCount{Magnitude: 5}, UpperIncluded: true,
				},
			}}},
			want: []string{"rm_invariant /other_reference_ranges[0]/range"},
		},
		{
			name: "DV_DATE in an ELEMENT, empty date inside the range bound",
			root: scoreElement(&rm.DVDate{Value: "2026-09-29", OtherReferenceRanges: []rm.ReferenceRange[rm.DVOrdered]{{
				Meaning: rm.DVText{Value: "valid period"},
				Range:   openUpper(rm.DVDate{}),
			}}}),
			want: []string{"required /value/other_reference_ranges[0]/range/lower/value"},
		},
		{
			name: "DV_DATE in an ELEMENT, normal_status with an empty code_string",
			root: scoreElement(&rm.DVDate{Value: "2026-09-29", NormalStatus: &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr_normal_statuses"}}}),
			want: []string{
				"required /value/normal_status/code_string",
				"rm_invariant /value/normal_status",
			},
		},
		{
			name: "DV_ORDINAL, a complete reference range",
			root: &rm.DVOrdinal{Value: 1, Symbol: codedSymbol("at1"), OtherReferenceRanges: []rm.ReferenceRange[rm.DVOrdered]{{
				Meaning: rm.DVText{Value: "normal"},
				Range:   openUpper(rm.DVOrdinal{Value: 0, Symbol: codedSymbol("at0")}),
			}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findingsOf(validation.ValidateRM(tc.root)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) findings = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestValidateRMEHRAccess_Walked checks that the floor walks an EHR_ACCESS
// like any other LOCATABLE (REQ-112): its RM-mandatory name and
// archetype_node_id are checked, and the ARCHETYPED under its
// archetype_details is checked as on any LOCATABLE.
func TestValidateRMEHRAccess_Walked(t *testing.T) {
	details := &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-EHR_ACCESS.generic.v1"}, RMVersion: "1.1.0"}
	cases := []struct {
		name   string
		access *rm.EHRAccess
		want   []string
	}{
		{
			name:   "name absent",
			access: &rm.EHRAccess{ArchetypeNodeID: "openEHR-EHR-EHR_ACCESS.generic.v1", ArchetypeDetails: details},
			want:   []string{"required /name"},
		},
		{
			name:   "archetype_node_id absent",
			access: &rm.EHRAccess{Name: rm.DVText{Value: "EHR Access"}, ArchetypeDetails: details},
			want:   []string{"required /archetype_node_id"},
		},
		{
			name: "incomplete ARCHETYPED",
			access: &rm.EHRAccess{
				ArchetypeNodeID:  "openEHR-EHR-EHR_ACCESS.generic.v1",
				Name:             rm.DVText{Value: "EHR Access"},
				ArchetypeDetails: &rm.Archetyped{},
			},
			want: []string{
				"required /archetype_details/archetype_id/value",
				"rm_version_valid /archetype_details/rm_version",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findingsOf(validation.ValidateRMEHRAccess(tc.access)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRMEHRAccess(%s) findings = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
