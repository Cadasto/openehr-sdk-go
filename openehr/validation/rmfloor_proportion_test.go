package validation_test

// rmfloor_proportion_test.go: unit pins for the two DV_PROPORTION.precision
// arms of the REQ-112 RM floor.
//
//   - Range: DV_PROPORTION.precision carries the same RM reading as
//     DV_QUANTITY.precision — decimal places, 0 integral, -1 "no limit" —
//     so only precision < -1 is out of range.
//   - Integrality: the BMM invariant `Precision_validity: precision = 0
//     implies is_integral`, read through `Is_integral_validity`, so a
//     precision of 0 requires whole-number operands. Any other in-range
//     precision (-1 included) leaves the operands unconstrained.
//
// The table's last case is a control on DV_QUANTITY: it proves the two
// evaluators are independent, so deleting the DV_PROPORTION check fails
// the DV_PROPORTION cases while the DV_QUANTITY case still passes.

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestValidateRM_DVProportionPrecision pins the precision floor on
// DV_PROPORTION: an out-of-range precision, and a precision of 0 over a
// fractional operand, each surface exactly one `rm_invariant` issue whose
// Detail names DV_PROPORTION; -1, 0 over whole-number operands, another
// in-range precision, and an absent precision report OK. The
// type-specific denominator invariants (Valid_denominator,
// Unitary_validity, Percent_validity, Fraction_validity) are deliberately
// outside this test — they are not in the floor.
func TestValidateRM_DVProportionPrecision(t *testing.T) {
	t.Parallel()

	precision := func(n int) *rm.Integer {
		v := rm.Integer(n)
		return &v
	}

	cases := []struct {
		name string
		// input is the RM root handed to ValidateRM.
		input any
		// desc renders input in failure messages — the raw struct would
		// print the Precision pointer as an address, which diagnoses
		// nothing.
		desc string
		// wantIssue is true when the case must surface exactly one
		// rm_invariant issue.
		wantIssue bool
		// wantType is the RM type name the Detail must name when
		// wantIssue is true.
		wantType string
		// wantOperand, when set, is the operand name the Detail must end
		// with — the check appends the non-integral operands last, so the
		// suffix is what distinguishes "numerator" from "denominator"
		// without matching the constraint sentence, which names both.
		wantOperand string
		// wantNoValue, when set, is the rendering of the offending operand
		// that must NOT appear anywhere in the Detail (REQ-093: the
		// diagnostic names the attribute, not the value).
		wantNoValue string
	}{
		{
			name:      "DV_PROPORTION precision=-2 out of range",
			input:     &rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: precision(-2)},
			desc:      "&rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: -2}",
			wantIssue: true,
			wantType:  "DV_PROPORTION",
		},
		{
			name:  "DV_PROPORTION precision=-1 means no limit",
			input: &rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: precision(-1)},
			desc:  "&rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: -1}",
		},
		{
			name:  "DV_PROPORTION precision=0 with whole-number operands",
			input: &rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: precision(0)},
			desc:  "&rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: 0}",
		},
		{
			name:  "DV_PROPORTION precision absent",
			input: &rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0},
			desc:  "&rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: nil}",
		},
		{
			// Precision_validity: precision = 0 implies is_integral, and
			// Is_integral_validity reads that as whole-number operands.
			name:        "DV_PROPORTION precision=0 with fractional numerator",
			input:       &rm.DVProportion{Numerator: 1.5, Denominator: 2, Type: 0, Precision: precision(0)},
			desc:        "&rm.DVProportion{Numerator: 1.5, Denominator: 2, Type: 0, Precision: 0}",
			wantIssue:   true,
			wantType:    "DV_PROPORTION",
			wantOperand: "numerator",
			wantNoValue: "1.5",
		},
		{
			name:        "DV_PROPORTION precision=0 with fractional denominator",
			input:       &rm.DVProportion{Numerator: 1, Denominator: 2.5, Type: 0, Precision: precision(0)},
			desc:        "&rm.DVProportion{Numerator: 1, Denominator: 2.5, Type: 0, Precision: 0}",
			wantIssue:   true,
			wantType:    "DV_PROPORTION",
			wantOperand: "denominator",
			wantNoValue: "2.5",
		},
		{
			// Both operands fractional is still ONE issue for the node —
			// the invariant is breached once, and the Detail names both
			// operands rather than the check emitting two issues.
			name:        "DV_PROPORTION precision=0 with both operands fractional",
			input:       &rm.DVProportion{Numerator: 1.5, Denominator: 2.5, Type: 0, Precision: precision(0)},
			desc:        "&rm.DVProportion{Numerator: 1.5, Denominator: 2.5, Type: 0, Precision: 0}",
			wantIssue:   true,
			wantType:    "DV_PROPORTION",
			wantOperand: "numerator, denominator",
			wantNoValue: "1.5",
		},
		{
			// Value arm of asDVProportion: a DV_PROPORTION handed to
			// ValidateRM by value, not by pointer, dispatches the same
			// evaluator.
			name:        "DV_PROPORTION by value, precision=0 with fractional numerator",
			input:       rm.DVProportion{Numerator: 1.5, Denominator: 2, Type: 0, Precision: precision(0)},
			desc:        "rm.DVProportion{Numerator: 1.5, Denominator: 2, Type: 0, Precision: 0} (by value)",
			wantIssue:   true,
			wantType:    "DV_PROPORTION",
			wantOperand: "numerator",
			wantNoValue: "1.5",
		},
		{
			// -1 is "no limit", so it constrains neither operand: the RM
			// invariant is keyed on precision = 0 alone.
			name:  "DV_PROPORTION precision=-1 leaves fractional operands alone",
			input: &rm.DVProportion{Numerator: 1.5, Denominator: 2.5, Type: 0, Precision: precision(-1)},
			desc:  "&rm.DVProportion{Numerator: 1.5, Denominator: 2.5, Type: 0, Precision: -1}",
		},
		{
			// Likewise any positive precision — 2 decimal places is exactly
			// the case fractional operands are recorded for.
			name:  "DV_PROPORTION precision=2 leaves fractional operands alone",
			input: &rm.DVProportion{Numerator: 1.5, Denominator: 2.5, Type: 0, Precision: precision(2)},
			desc:  "&rm.DVProportion{Numerator: 1.5, Denominator: 2.5, Type: 0, Precision: 2}",
		},
		{
			// Control: the DV_QUANTITY check is a separate evaluator and
			// must keep firing on its own carrier.
			name:      "control DV_QUANTITY precision=-2 still fires",
			input:     &rm.DVQuantity{Magnitude: 1.0, Units: "mg", Precision: precision(-2)},
			desc:      `&rm.DVQuantity{Magnitude: 1.0, Units: "mg", Precision: -2}`,
			wantIssue: true,
			wantType:  "DV_QUANTITY",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := validation.ValidateRM(tc.input)

			if !tc.wantIssue {
				if !r.OK {
					t.Fatalf("ValidateRM(%s).OK = false, want true; issues=%+v", tc.desc, r.Issues)
				}
				return
			}

			if r.OK {
				t.Fatalf("ValidateRM(%s).OK = true, want false (the RM precision floor is breached — range or Precision_validity); issues=%+v", tc.desc, r.Issues)
			}

			var invariants []validation.Issue
			for _, issue := range r.Issues {
				if issue.Code == "rm_invariant" {
					invariants = append(invariants, issue)
				}
			}
			if len(invariants) != 1 {
				t.Fatalf("ValidateRM(%s) rm_invariant issue count = %d, want 1; issues=%+v", tc.desc, len(invariants), r.Issues)
			}
			if got := invariants[0].Detail; !strings.Contains(got, tc.wantType) {
				t.Errorf("ValidateRM(%s) rm_invariant Detail = %q, want it to name %s", tc.desc, got, tc.wantType)
			}
			if got := invariants[0].Detail; !strings.Contains(got, "precision") {
				t.Errorf("ValidateRM(%s) rm_invariant Detail = %q, want it to name the precision attribute", tc.desc, got)
			}
			if tc.wantOperand != "" {
				if got := invariants[0].Detail; !strings.HasSuffix(got, tc.wantOperand) {
					t.Errorf("ValidateRM(%s) rm_invariant Detail = %q, want it to end by naming the non-integral operand %q", tc.desc, got, tc.wantOperand)
				}
			}
			if tc.wantNoValue != "" {
				if got := invariants[0].Detail; strings.Contains(got, tc.wantNoValue) {
					t.Errorf("ValidateRM(%s) rm_invariant Detail = %q, want it NOT to echo the operand value %q (REQ-093)", tc.desc, got, tc.wantNoValue)
				}
			}
		})
	}
}
