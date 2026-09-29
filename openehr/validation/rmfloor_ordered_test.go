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
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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
			// A typed interval holds its bounds by value, so this reaches
			// the value-form DV_COUNT reader of other_reference_ranges.
			name: "DV_COUNT, empty meaning on a reference range of its normal_range lower bound",
			root: &rm.DVCount{Magnitude: 3, NormalRange: &rm.DVInterval[rm.DVCount]{
				Lower: rm.DVCount{Magnitude: 1, OtherReferenceRanges: []rm.ReferenceRange[rm.DVCount]{{
					Meaning: rm.DVText{},
					Range:   openUpper(rm.DVCount{Magnitude: 0}),
				}}}, LowerIncluded: true,
				Upper: rm.DVCount{Magnitude: 5}, UpperIncluded: true,
			}},
			want: []string{"required /normal_range/lower/other_reference_ranges[0]/meaning"},
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

// TestValidateRM_ReferenceRangeOnDecodedBound is the wire-path twin of the
// normal_range row above (REQ-112): canjson decodes a typed interval's bound
// by value, so a reference range on it is reached through the value-form
// reader, and its empty meaning is reported at its path.
func TestValidateRM_ReferenceRangeOnDecodedBound(t *testing.T) {
	const body = `{"_type":"DV_COUNT","magnitude":3,"normal_range":{"_type":"DV_INTERVAL",` +
		`"lower":{"_type":"DV_COUNT","magnitude":1,"other_reference_ranges":[{"_type":"REFERENCE_RANGE",` +
		`"meaning":{"_type":"DV_TEXT","value":""},` +
		`"range":{"_type":"DV_INTERVAL","lower_unbounded":true,"upper_unbounded":true,"lower_included":false,"upper_included":false}}]},` +
		`"upper":{"_type":"DV_COUNT","magnitude":5},` +
		`"lower_included":true,"upper_included":true,"lower_unbounded":false,"upper_unbounded":false}}`
	var count rm.DVCount
	if err := canjson.Unmarshal([]byte(body), &count); err != nil {
		t.Fatalf("canjson.Unmarshal: %v", err)
	}
	want := []string{"required /normal_range/lower/other_reference_ranges[0]/meaning"}
	if got := findingsOf(validation.ValidateRM(&count)); !slices.Equal(got, want) {
		t.Errorf("ValidateRM(decoded DV_COUNT) findings = %q, want %q", got, want)
	}
}

// TestValidateRM_ReferenceRangeOnEveryValueFormBound checks that the walk
// reaches other_reference_ranges on each DV_ORDERED concrete held by value,
// as the bound of a typed interval is (REQ-112): every row plants an empty
// meaning on the lower bound's reference range and expects it at its path.
func TestValidateRM_ReferenceRangeOnEveryValueFormBound(t *testing.T) {
	open := rm.DVInterval[rm.DVOrdered]{LowerUnbounded: true, UpperUnbounded: true}
	bare := []rm.ReferenceRange[rm.DVOrdered]{{Meaning: rm.DVText{}, Range: open}}
	cases := []struct {
		name string
		root any
	}{
		{"DV_COUNT", &rm.DVInterval[rm.DVCount]{
			LowerIncluded: true, UpperUnbounded: true,
			Lower: rm.DVCount{Magnitude: 1, OtherReferenceRanges: []rm.ReferenceRange[rm.DVCount]{{Meaning: rm.DVText{}, Range: open}}},
		}},
		{"DV_QUANTITY", &rm.DVInterval[rm.DVQuantity]{
			LowerIncluded: true, UpperUnbounded: true,
			Lower: rm.DVQuantity{Magnitude: 1, Units: "mm", OtherReferenceRanges: []rm.ReferenceRange[rm.DVQuantity]{{Meaning: rm.DVText{}, Range: open}}},
		}},
		{"DV_PROPORTION", &rm.DVInterval[rm.DVProportion]{
			LowerIncluded: true, UpperUnbounded: true,
			Lower: rm.DVProportion{Numerator: 1, Denominator: 2, OtherReferenceRanges: []rm.ReferenceRange[rm.DVProportion]{{Meaning: rm.DVText{}, Range: open}}},
		}},
		{"DV_ORDINAL", &rm.DVInterval[rm.DVOrdinal]{
			LowerIncluded: true, UpperUnbounded: true,
			Lower: rm.DVOrdinal{Value: 1, Symbol: codedSymbol("at1"), OtherReferenceRanges: bare},
		}},
		{"DV_SCALE", &rm.DVInterval[rm.DVScale]{
			LowerIncluded: true, UpperUnbounded: true,
			Lower: rm.DVScale{Value: 0.5, Symbol: codedSymbol("at1"), OtherReferenceRanges: bare},
		}},
		{"DV_DATE", &rm.DVInterval[rm.DVDate]{
			LowerIncluded: true, UpperUnbounded: true,
			Lower: rm.DVDate{Value: "2026-01-01", OtherReferenceRanges: bare},
		}},
		{"DV_TIME", &rm.DVInterval[rm.DVTime]{
			LowerIncluded: true, UpperUnbounded: true,
			Lower: rm.DVTime{Value: "08:00:00", OtherReferenceRanges: bare},
		}},
		{"DV_DATE_TIME", &rm.DVInterval[rm.DVDateTime]{
			LowerIncluded: true, UpperUnbounded: true,
			Lower: rm.DVDateTime{Value: "2026-01-01T00:00:00Z", OtherReferenceRanges: bare},
		}},
		{"DV_DURATION", &rm.DVInterval[rm.DVDuration]{
			LowerIncluded: true, UpperUnbounded: true,
			Lower: rm.DVDuration{Value: "P1D", OtherReferenceRanges: bare},
		}},
	}
	want := []string{"required /lower/other_reference_ranges[0]/meaning"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findingsOf(validation.ValidateRM(tc.root)); !slices.Equal(got, want) {
				t.Errorf("ValidateRM(DV_INTERVAL<%s> with a reference range on its lower bound) findings = %q, want %q", tc.name, got, want)
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

// TestValidateRM_ScaleSymbolMayHaveNoCode pins the DV_SCALE symbol exemption
// (REQ-112). The RM lets a scale value have no code: its symbol is then a
// DV_CODED_TEXT carrying the terminology_id and a blank code_string. So the
// floor reports neither the CODE_PHRASE row's rm_invariant nor the `required`
// code_string on a DV_SCALE symbol's defining_code. Nothing else is exempt: a
// DV_ORDINAL symbol, a blank terminology_id, a CODE_PHRASE anywhere else under
// the same DV_SCALE, and the symbol's own value are all reported as before.
func TestValidateRM_ScaleSymbolMayHaveNoCode(t *testing.T) {
	noCode := func(text string) rm.DVCodedText {
		return rm.DVCodedText{Value: text, DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}}}
	}
	blankTarget := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}}
	withBlankMapping := noCode("very slight")
	withBlankMapping.Mappings = []rm.TermMapping{{Match: "=", Target: blankTarget}}
	cases := []struct {
		name string
		root any
		want []string
	}{
		{
			name: "DV_SCALE symbol with a terminology_id and a blank code_string",
			root: scoreElement(&rm.DVScale{Value: 0.5, Symbol: noCode("very very slight")}),
		},
		{
			name: "the same symbol on an interval bound",
			root: &rm.DVInterval[rm.DVScale]{
				Lower: rm.DVScale{Value: 0.5, Symbol: noCode("very very slight")}, LowerIncluded: true,
				Upper: rm.DVScale{Value: 2, Symbol: codedSymbol("at2")}, UpperIncluded: true,
			},
		},
		{
			name: "DV_SCALE symbol with a blank code_string and a blank value",
			root: scoreElement(&rm.DVScale{Value: 0.5, Symbol: noCode("")}),
			want: []string{"required /value/symbol/value"},
		},
		{
			name: "DV_ORDINAL symbol with a blank code_string",
			root: scoreElement(&rm.DVOrdinal{Value: 1, Symbol: noCode("mild")}),
			want: []string{
				"required /value/symbol/defining_code/code_string",
				"rm_invariant /value/symbol/defining_code",
			},
		},
		{
			name: "DV_SCALE symbol with a blank terminology_id and a blank code_string",
			root: scoreElement(&rm.DVScale{Value: 0.5, Symbol: rm.DVCodedText{Value: "slight"}}),
			want: []string{"required /value/symbol/defining_code"},
		},
		{
			name: "DV_SCALE symbol with a blank terminology_id and a code",
			root: scoreElement(&rm.DVScale{Value: 0.5, Symbol: rm.DVCodedText{Value: "slight", DefiningCode: rm.CodePhrase{CodeString: "at1"}}}),
			want: []string{"required /value/symbol/defining_code/terminology_id"},
		},
		{
			name: "DV_SCALE normal_status with a blank code_string",
			root: scoreElement(&rm.DVScale{Value: 0.5, Symbol: codedSymbol("at1"), NormalStatus: &blankTarget}),
			want: []string{
				"required /value/normal_status/code_string",
				"rm_invariant /value/normal_status",
			},
		},
		{
			name: "a mapping target with a blank code_string under a DV_SCALE symbol",
			root: scoreElement(&rm.DVScale{Value: 0.5, Symbol: withBlankMapping}),
			want: []string{
				"required /value/symbol/mappings[0]/target/code_string",
				"rm_invariant /value/symbol/mappings[0]/target",
			},
		},
		{
			// A DV_SCALE and a non-scale code in one walk: the scale is
			// reached first, and its exemption must not spill onto the
			// DV_ORDINAL bound below it.
			name: "DV_ORDINAL with no code inside a DV_SCALE's other_reference_ranges",
			root: scoreElement(&rm.DVScale{Value: 0.5, Symbol: noCode("slight"), OtherReferenceRanges: []rm.ReferenceRange[rm.DVOrdered]{{
				Meaning: rm.DVText{Value: "normal"},
				Range:   rm.DVInterval[rm.DVOrdered]{Lower: rm.DVOrdinal{Value: 1, Symbol: noCode("mild")}, LowerIncluded: true, UpperUnbounded: true},
			}}}),
			want: []string{
				"required /value/other_reference_ranges[0]/range/lower/symbol/defining_code/code_string",
				"rm_invariant /value/other_reference_ranges[0]/range/lower/symbol/defining_code",
			},
		},
		{
			name: "CLUSTER with a DV_SCALE with no code beside a DV_ORDINAL with no code",
			root: &rm.Cluster{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "scores"}, Items: []rm.Item{
				scoreElement(&rm.DVScale{Value: 0.5, Symbol: noCode("slight")}),
				scoreElement(&rm.DVOrdinal{Value: 1, Symbol: noCode("mild")}),
			}},
			want: []string{
				"required /items[1]/value/symbol/defining_code/code_string",
				"rm_invariant /items[1]/value/symbol/defining_code",
			},
		},
		{
			name: "DV_CODED_TEXT value with a blank code_string",
			root: scoreElement(&rm.DVCodedText{Value: "slight", DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}}}),
			want: []string{
				"required /value/defining_code/code_string",
				"rm_invariant /value/defining_code",
			},
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

// TestValidateRM_AccuracyWalked checks that the walk reaches the optional
// accuracy of the date and time types, a DV_DURATION node, and checks it
// (REQ-112): a DV_DURATION's value is RM-mandatory, so an empty one inside a
// date's accuracy is `required` at its path. A Real accuracy on a DV_AMOUNT
// type has nothing below it to check.
func TestValidateRM_AccuracyWalked(t *testing.T) {
	half := rm.Real(0.5)
	cases := []struct {
		name string
		root any
		want []string
	}{
		{
			name: "DV_DATE with an empty accuracy duration",
			root: scoreElement(&rm.DVDate{Value: "2026-09-29", Accuracy: &rm.DVDuration{}}),
			want: []string{"required /value/accuracy/value"},
		},
		{
			name: "DV_DATE_TIME with a complete accuracy duration",
			root: scoreElement(&rm.DVDateTime{Value: "2026-09-29T10:00:00Z", Accuracy: &rm.DVDuration{Value: "PT1H"}}),
		},
		{
			name: "DV_TIME as a bound with an empty accuracy duration",
			root: &rm.DVInterval[rm.DVTime]{
				Lower: rm.DVTime{Value: "08:00:00", Accuracy: &rm.DVDuration{}}, LowerIncluded: true,
				UpperUnbounded: true,
			},
			want: []string{"required /lower/accuracy/value"},
		},
		{
			name: "DV_QUANTITY with a Real accuracy",
			root: scoreElement(&rm.DVQuantity{Magnitude: 5, Units: "mm", Accuracy: &half}),
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
