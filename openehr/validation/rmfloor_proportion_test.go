package validation_test

// rmfloor_proportion_test.go: unit pins for the DV_PROPORTION.precision
// arm of the REQ-112 RM floor. DV_PROPORTION.precision carries the same
// RM reading as DV_QUANTITY.precision — decimal places, 0 integral,
// -1 "no limit" — so only precision < -1 is out of range.
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
// DV_PROPORTION: out-of-range precision surfaces exactly one
// `rm_invariant` issue whose Detail names DV_PROPORTION; -1, 0 and an
// absent precision report OK. Numerator / denominator / type invariants
// are deliberately outside this test — this is the precision axis only.
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
			name:  "DV_PROPORTION precision=0 means integral",
			input: &rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: precision(0)},
			desc:  "&rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: 0}",
		},
		{
			name:  "DV_PROPORTION precision absent",
			input: &rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0},
			desc:  "&rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: nil}",
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
				t.Fatalf("ValidateRM(%s).OK = true, want false (precision out of range); issues=%+v", tc.desc, r.Issues)
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
		})
	}
}
