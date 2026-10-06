package constraints_test

// value_free_test.go: REQ-168. A Violation's Code and Detail never repeat the
// value passed to Validate, nor any part of it; that value sits in Value and
// nowhere else. Each case plants markers that no constraint text in this file
// contains, so a marker found in Code or Detail can only have been copied from
// the input.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// Markers that no constraint text in these tests contains.
const (
	markerString    = "MARKER-1a2b"
	markerTerm      = "MARKER-TERM"
	markerCode      = "MARKER-CODE"
	markerUnits     = "MARKER-UNITS"
	markerInt       = 987654321
	markerReal      = 987654.25
	markerOrdinal   = 424242
	markerPrecision = 7
)

var (
	// closedRange prints as [0..100] and precisionRange as [0..3]: neither
	// contains the digits of a marker.
	closedRange    = constraints.NumericRange{Lower: 0, Upper: 100, LowerInclusive: true, UpperInclusive: true}
	precisionRange = constraints.NumericRange{Lower: 0, Upper: 3, LowerInclusive: true, UpperInclusive: true}

	quantityConstraint = constraints.DvQuantity{Units: []constraints.QuantityUnit{
		{Units: "kg", Magnitude: closedRange, Precision: precisionRange},
		{Units: "g", Magnitude: closedRange},
	}}

	ordinalConstraint = constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
		{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0001"}},
		{Value: 2, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0002"}},
	}}

	markerSymbol = constraints.CodedTermRef{Terminology: markerTerm, CodeString: markerCode}
)

// valueFreeCase is one input that fails exactly one clause of a constraint.
type valueFreeCase struct {
	name       string
	constraint constraints.PrimitiveConstraint
	input      any
	wantCode   constraints.ViolationCode
	// wantValue is what Value.Reveal must return: the part of the input the
	// failing clause tested.
	wantValue any
	// needles are the printed forms of every submitted value in input; none
	// may appear in Code or Detail.
	needles []string
}

// valueFreeCases covers each row of REQ-168 § What Value holds, and each
// clause of each validator that tests the input.
func valueFreeCases() []valueFreeCase {
	return []valueFreeCase{
		// CBoolean: a boolean has no value unique enough to search for, so
		// these rows check Value only.
		{
			name:       "CBoolean true not allowed",
			constraint: constraints.CBoolean{FalseValid: true},
			input:      true,
			wantCode:   constraints.CodeNotInList,
			wantValue:  true,
		},
		{
			name:       "CBoolean false not allowed",
			constraint: constraints.CBoolean{TrueValid: true},
			input:      false,
			wantCode:   constraints.CodeNotInList,
			wantValue:  false,
		},

		// CInteger: Value is the argument as passed, not the int64 the
		// validator compares.
		{
			name:       "CInteger not in list",
			constraint: constraints.CInteger{List: []int64{1, 2, 3}},
			input:      markerInt,
			wantCode:   constraints.CodeNotInList,
			wantValue:  markerInt,
			needles:    []string{strconv.Itoa(markerInt)},
		},
		{
			name:       "CInteger outside range",
			constraint: constraints.CInteger{Range: closedRange},
			input:      markerInt,
			wantCode:   constraints.CodeOutOfRange,
			wantValue:  markerInt,
			needles:    []string{strconv.Itoa(markerInt)},
		},
		{
			name:       "CInteger outside range, int32 argument",
			constraint: constraints.CInteger{Range: closedRange},
			input:      int32(markerInt),
			wantCode:   constraints.CodeOutOfRange,
			wantValue:  int32(markerInt),
			needles:    []string{strconv.Itoa(markerInt)},
		},

		// CReal: an integer argument is widened to float64 for the check, so
		// its float form is a needle too.
		{
			name:       "CReal not in list",
			constraint: constraints.CReal{List: []float64{1.5, 2.5}},
			input:      markerReal,
			wantCode:   constraints.CodeNotInList,
			wantValue:  markerReal,
			needles:    []string{fmt.Sprint(markerReal)},
		},
		{
			name:       "CReal outside range",
			constraint: constraints.CReal{Range: closedRange},
			input:      markerReal,
			wantCode:   constraints.CodeOutOfRange,
			wantValue:  markerReal,
			needles:    []string{fmt.Sprint(markerReal)},
		},
		{
			name:       "CReal outside range, int argument",
			constraint: constraints.CReal{Range: closedRange},
			input:      markerInt,
			wantCode:   constraints.CodeOutOfRange,
			wantValue:  markerInt,
			needles:    []string{strconv.Itoa(markerInt), fmt.Sprint(float64(markerInt))},
		},

		// CString: a pattern compiled by NewCString and one compiled on first
		// use take different paths to the same clause.
		{
			name:       "CString not in list",
			constraint: constraints.CString{List: []string{"alpha", "beta"}},
			input:      markerString,
			wantCode:   constraints.CodeNotInList,
			wantValue:  markerString,
			needles:    []string{markerString},
		},
		{
			name:       "CString pattern mismatch, pattern compiled by NewCString",
			constraint: constraints.NewCString("[a-z]+", nil, ""),
			input:      markerString,
			wantCode:   constraints.CodePatternMismatch,
			wantValue:  markerString,
			needles:    []string{markerString},
		},
		{
			name:       "CString pattern mismatch, pattern compiled on first use",
			constraint: constraints.CString{Pattern: "[a-z]+"},
			input:      markerString,
			wantCode:   constraints.CodePatternMismatch,
			wantValue:  markerString,
			needles:    []string{markerString},
		},

		// Temporal validators.
		{
			name:       "CDate not a valid date",
			constraint: constraints.CDate{},
			input:      markerString,
			wantCode:   constraints.CodeInvalidValue,
			wantValue:  markerString,
			needles:    []string{markerString},
		},
		{
			name:       "CTime not a valid time",
			constraint: constraints.CTime{},
			input:      markerString,
			wantCode:   constraints.CodeInvalidValue,
			wantValue:  markerString,
			needles:    []string{markerString},
		},
		{
			name:       "CDateTime not a valid date-time",
			constraint: constraints.CDateTime{},
			input:      markerString,
			wantCode:   constraints.CodeInvalidValue,
			wantValue:  markerString,
			needles:    []string{markerString},
		},
		{
			name:       "CDuration not a valid duration",
			constraint: constraints.CDuration{},
			input:      markerString,
			wantCode:   constraints.CodeInvalidValue,
			wantValue:  markerString,
			needles:    []string{markerString},
		},

		// CodePhrase: Value is the one field of the input the clause tested,
		// and neither field may appear in Detail.
		{
			name:       "CodePhrase terminology mismatch",
			constraint: constraints.CodePhrase{Terminology: "openehr"},
			input:      markerSymbol,
			wantCode:   constraints.CodeInvalidValue,
			wantValue:  markerTerm,
			needles:    []string{markerTerm, markerCode},
		},
		{
			name:       "CodePhrase code not in list",
			constraint: constraints.CodePhrase{Terminology: "openehr", CodeList: []string{"433", "434"}},
			input:      constraints.CodedTermRef{Terminology: "openehr", CodeString: markerCode},
			wantCode:   constraints.CodeNotInList,
			wantValue:  markerCode,
			needles:    []string{markerCode},
		},
		{
			name:       "CodePhrase code not in list, bare string",
			constraint: constraints.CodePhrase{Terminology: "openehr", CodeList: []string{"433", "434"}},
			input:      markerCode,
			wantCode:   constraints.CodeNotInList,
			wantValue:  markerCode,
			needles:    []string{markerCode},
		},

		// DvQuantity: the constraint's own units entry may appear in Detail;
		// the input's magnitude, units and precision may not.
		{
			name:       "DvQuantity magnitude outside range",
			constraint: quantityConstraint,
			input:      constraints.QuantityValue{Magnitude: markerReal, Units: "kg", Precision: -1},
			wantCode:   constraints.CodeOutOfRange,
			wantValue:  markerReal,
			needles:    []string{fmt.Sprint(markerReal)},
		},
		{
			name:       "DvQuantity precision outside range",
			constraint: quantityConstraint,
			input:      constraints.QuantityValue{Magnitude: 50, Units: "kg", Precision: markerPrecision},
			wantCode:   constraints.CodeOutOfRange,
			wantValue:  markerPrecision,
			needles:    []string{strconv.Itoa(markerPrecision)},
		},
		{
			name:       "DvQuantity units not enumerated",
			constraint: quantityConstraint,
			input:      constraints.QuantityValue{Magnitude: markerReal, Units: markerUnits, Precision: markerPrecision},
			wantCode:   constraints.CodeUnitUnknown,
			wantValue:  markerUnits,
			needles:    []string{markerUnits, fmt.Sprint(markerReal), strconv.Itoa(markerPrecision)},
		},

		// CDvOrdinal: Value is the argument as passed, an int or a whole
		// OrdinalSymbol.
		{
			name:       "CDvOrdinal value not in list",
			constraint: ordinalConstraint,
			input:      markerOrdinal,
			wantCode:   constraints.CodeNotInList,
			wantValue:  markerOrdinal,
			needles:    []string{strconv.Itoa(markerOrdinal)},
		},
		{
			name:       "CDvOrdinal pair not in list",
			constraint: ordinalConstraint,
			input:      constraints.OrdinalSymbol{Value: markerOrdinal, Symbol: markerSymbol},
			wantCode:   constraints.CodeNotInList,
			wantValue:  constraints.OrdinalSymbol{Value: markerOrdinal, Symbol: markerSymbol},
			needles:    []string{strconv.Itoa(markerOrdinal), markerTerm, markerCode},
		},
		{
			name:       "CDvOrdinal listed value with an unlisted symbol",
			constraint: ordinalConstraint,
			input:      constraints.OrdinalSymbol{Value: 1, Symbol: markerSymbol},
			wantCode:   constraints.CodeNotInList,
			wantValue:  constraints.OrdinalSymbol{Value: 1, Symbol: markerSymbol},
			needles:    []string{markerTerm, markerCode},
		},
	}
}

// TestREQ168_ViolationDetailIsValueFree fails when a validator copies the
// input, or any field of it, into Code or Detail, or leaves it out of Value.
func TestREQ168_ViolationDetailIsValueFree(t *testing.T) {
	t.Parallel()
	for _, tc := range valueFreeCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.constraint.Validate(tc.input)
			if len(got) != 1 {
				t.Fatalf("%T.Validate(%#v) = %d violation(s), want 1: %+v", tc.constraint, tc.input, len(got), got)
			}
			v := got[0]
			if v.Code != tc.wantCode {
				t.Errorf("%T.Validate(%#v) Code = %q, want %q", tc.constraint, tc.input, v.Code, tc.wantCode)
			}
			if v.Detail == "" {
				t.Errorf("%T.Validate(%#v) Detail is empty, want a message naming the failed clause", tc.constraint, tc.input)
			}
			for _, needle := range tc.needles {
				if strings.Contains(string(v.Code), needle) {
					t.Errorf("%T.Validate(%#v) Code = %q, which repeats the submitted value %q", tc.constraint, tc.input, v.Code, needle)
				}
				if strings.Contains(v.Detail, needle) {
					t.Errorf("%T.Validate(%#v) Detail = %q, which repeats the submitted value %q", tc.constraint, tc.input, v.Detail, needle)
				}
			}
			if reveal := v.Value.Reveal(); reveal != tc.wantValue {
				t.Errorf("%T.Validate(%#v) Value.Reveal() = %#v (%T), want %#v (%T)", tc.constraint, tc.input, reveal, reveal, tc.wantValue, tc.wantValue)
			}
		})
	}
}

// emptyValueCase is one input that no clause of the constraint tests: the
// argument has the wrong Go type, or the constraint itself is at fault.
type emptyValueCase struct {
	name       string
	constraint constraints.PrimitiveConstraint
	input      any
	wantCode   constraints.ViolationCode
}

func emptyValueCases() []emptyValueCase {
	return []emptyValueCase{
		{name: "CBoolean wrong type", constraint: constraints.CBoolean{TrueValid: true}, input: markerString, wantCode: constraints.CodeWrongType},
		{name: "CInteger wrong type", constraint: constraints.CInteger{Range: closedRange}, input: markerString, wantCode: constraints.CodeWrongType},
		{name: "CInteger uint64 too large for int64", constraint: constraints.CInteger{Range: closedRange}, input: ^uint64(0), wantCode: constraints.CodeWrongType},
		{name: "CReal wrong type", constraint: constraints.CReal{Range: closedRange}, input: markerString, wantCode: constraints.CodeWrongType},
		{name: "CString wrong type", constraint: constraints.CString{List: []string{"alpha"}}, input: markerInt, wantCode: constraints.CodeWrongType},
		{name: "CDate wrong type", constraint: constraints.CDate{}, input: markerInt, wantCode: constraints.CodeWrongType},
		{name: "CTime wrong type", constraint: constraints.CTime{}, input: markerInt, wantCode: constraints.CodeWrongType},
		{name: "CDateTime wrong type", constraint: constraints.CDateTime{}, input: markerInt, wantCode: constraints.CodeWrongType},
		{name: "CDuration wrong type", constraint: constraints.CDuration{}, input: markerInt, wantCode: constraints.CodeWrongType},
		{name: "CodePhrase wrong type", constraint: constraints.CodePhrase{Terminology: "openehr"}, input: markerInt, wantCode: constraints.CodeWrongType},
		{name: "DvQuantity wrong type", constraint: quantityConstraint, input: markerString, wantCode: constraints.CodeWrongType},
		{name: "DvQuantity without units, wrong type", constraint: constraints.DvQuantity{}, input: markerString, wantCode: constraints.CodeWrongType},
		{name: "CDvOrdinal wrong type", constraint: ordinalConstraint, input: markerString, wantCode: constraints.CodeWrongType},
		{name: "CDvOrdinal without values, wrong type", constraint: constraints.CDvOrdinal{}, input: markerString, wantCode: constraints.CodeWrongType},
		{name: "CString unparseable pattern", constraint: constraints.CString{Pattern: "("}, input: markerString, wantCode: constraints.CodeInvalidValue},
		{name: "CString unparseable pattern from NewCString", constraint: constraints.NewCString("(", nil, ""), input: markerString, wantCode: constraints.CodeInvalidValue},
	}
}

// TestREQ168_ViolationValueIsEmptyWhenNoClauseTestedTheInput pins the cases
// where Value holds nothing: CodeWrongType, and a pattern that does not parse.
func TestREQ168_ViolationValueIsEmptyWhenNoClauseTestedTheInput(t *testing.T) {
	t.Parallel()
	for _, tc := range emptyValueCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.constraint.Validate(tc.input)
			if len(got) != 1 {
				t.Fatalf("%T.Validate(%#v) = %d violation(s), want 1: %+v", tc.constraint, tc.input, len(got), got)
			}
			v := got[0]
			if v.Code != tc.wantCode {
				t.Errorf("%T.Validate(%#v) Code = %q, want %q", tc.constraint, tc.input, v.Code, tc.wantCode)
			}
			if needle := fmt.Sprint(tc.input); strings.Contains(v.Detail, needle) {
				t.Errorf("%T.Validate(%#v) Detail = %q, which repeats the submitted value %q", tc.constraint, tc.input, v.Detail, needle)
			}
			if reveal := v.Value.Reveal(); reveal != nil {
				t.Errorf("%T.Validate(%#v) Value.Reveal() = %#v (%T), want nil", tc.constraint, tc.input, reveal, reveal)
			}
			if s := v.Value.String(); s != "" {
				t.Errorf("%T.Validate(%#v) Value.String() = %q, want \"\"", tc.constraint, tc.input, s)
			}
		})
	}
}

// TestREQ168_ViolationsStayComparable compares every violation the cases above
// produce with a copy of itself. == on two Violations panics when Value holds
// a value whose type cannot be compared, such as a slice.
func TestREQ168_ViolationsStayComparable(t *testing.T) {
	t.Parallel()
	var produced []constraints.Violation
	for _, tc := range valueFreeCases() {
		produced = append(produced, tc.constraint.Validate(tc.input)...)
	}
	for _, tc := range emptyValueCases() {
		produced = append(produced, tc.constraint.Validate(tc.input)...)
	}
	if len(produced) == 0 {
		t.Fatal("the cases produced no violation, so nothing was compared")
	}
	for _, v := range produced {
		eq, panicked := equalsCopy(v)
		if panicked != nil {
			t.Errorf("Violation{Code: %q, Detail: %q} == its copy panicked: %v", v.Code, v.Detail, panicked)
			continue
		}
		if !eq {
			t.Errorf("Violation{Code: %q, Detail: %q} == its copy = false, want true", v.Code, v.Detail)
		}
	}
}

// equalsCopy compares v with a copy of itself and returns a panic instead of
// raising it.
func equalsCopy(v constraints.Violation) (eq bool, panicked any) {
	defer func() { panicked = recover() }()
	w := v
	return v == w, nil
}
