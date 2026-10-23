package instance_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_IntegerOnTheOrdinalItselfStaysInsideItsValue is the REQ-107
// Precedence 3 and Errors check that a C_INTEGER an OPT puts on a
// DV_ORDINAL itself, a primitive leaf in place of a C_DV_ORDINAL, gives the
// ordinal a value inside the 32-bit range of an RM Integer, as a C_INTEGER
// on its value does: a range that reaches past that range is narrowed to
// it, the example of one bounded above alone staying at its upper end, and
// one with no value inside it makes Generate return an error wrapping
// ErrConstraintUnsatisfiable that names DV_ORDINAL.value and the OPT path,
// and no root. It holds under every policy, both value fills, and with and
// without the implicit attributes. The template validator reports a
// C_INTEGER on a DV_ORDINAL as a primitive of the wrong type, so only the
// RM floor is checked.
func TestREQ107_IntegerOnTheOrdinalItselfStaysInsideItsValue(t *testing.T) {
	cases := []struct {
		name string
		body string // the C_INTEGER body
		// want is the ordinal's value under ExampleFill, and wantRandom
		// under RandomFill; unsatisfiable is set where Generate is to fail.
		want, wantRandom rm.Integer
		unsatisfiable    bool
	}{
		{
			// The draw lies within 100 of the greatest Integer, the window
			// the sampler draws from a range bounded above alone.
			name:       "at most 5000000000",
			body:       instance.IntRange(instance.NoEnd, instance.Closed(5000000000)),
			want:       2147483647,
			wantRandom: 2147483624,
		},
		{
			name:          "listing 5000000000 alone",
			body:          instance.IntList(5000000000),
			unsatisfiable: true,
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", optPrimitive("DV_ORDINAL", "C_INTEGER", tc.body))), implicit)
			for _, opts := range optionsFor(nil) {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if tc.unsatisfiable {
						if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
							t.Fatalf("Generate: error %v, want one wrapping ErrConstraintUnsatisfiable", err)
						}
						if want := "DV_ORDINAL.value at /value"; !strings.Contains(err.Error(), want) {
							t.Errorf("Generate: error %q, want it to name %q", err, want)
						}
						if out != nil {
							t.Errorf("Generate: root %T alongside the error, want none", out)
						}
						return
					}
					if err != nil {
						t.Fatalf("Generate: %v, want a root", err)
					}
					want := tc.want
					if opts.ValueFill == instance.RandomFill {
						want = tc.wantRandom
					}
					if got := dvValue[*rm.DVOrdinal](t, out).Value; got != want {
						t.Errorf("DV_ORDINAL.value = %d, want %d", got, want)
					}
					noFloorErrors(t, out)
				})
			}
		}
	}
}
