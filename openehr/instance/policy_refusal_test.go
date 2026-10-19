package instance_test

import (
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// undefinedPolicies are Policy values REQ-107 does not define: one past
// Required, a negative one and one far out of range.
var undefinedPolicies = []instance.Policy{instance.Policy(3), instance.Policy(-1), instance.Policy(99)}

// countingSource is a math/rand/v2 Source that counts its draws.
type countingSource struct {
	src   mrand.Source
	draws int
}

func (s *countingSource) Uint64() uint64 {
	s.draws++
	return s.src.Uint64()
}

// TestREQ107_GenerateRefusesAnUndefinedPolicy is the REQ-107 check of
// Contract Inputs: Generate returns an error wrapping ErrUnsupportedPolicy
// that names the value, and no root, for a Policy that is none of Large,
// Medium and Required. It refuses before it builds anything, so it draws
// no uid from UIDSource and no value from ValueSource. It holds on a
// COMPOSITION root and on roots of other classes, under both value fills.
func TestREQ107_GenerateRefusesAnUndefinedPolicy(t *testing.T) {
	roots := []struct {
		name     string
		compiled *templatecompile.Compiled
	}{
		{name: "COMPOSITION vital_signs", compiled: compileFixture(t, "vital_signs")},
		{name: "CLUSTER", compiled: compileOPTText(t, optTemplate("CLUSTER"), true)},
		{name: "ELEMENT", compiled: compileOPTText(t, optTemplate("ELEMENT"), true)},
	}
	for _, root := range roots {
		for _, policy := range undefinedPolicies {
			for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
				t.Run(fmt.Sprintf("%s/policy=%d/%v", root.name, int(policy), fill), func(t *testing.T) {
					uids := 0
					source := &countingSource{src: mrand.NewPCG(1, 2)}
					opts := instance.Options{
						Policy:    policy,
						ValueFill: fill,
						Now:       defaultsNow,
						Territory: "NL",
						Composer:  testComposer(),
						UIDSource: func() *rm.HierObjectID {
							uids++
							return &rm.HierObjectID{Value: fmt.Sprintf("uid-%d", uids)}
						},
						ValueSource: source,
					}
					call := fmt.Sprintf("Generate(%s, Policy(%d), %v)", root.name, int(policy), fill)
					out, err := instance.Generate(t.Context(), root.compiled, opts)
					if !errors.Is(err, instance.ErrUnsupportedPolicy) {
						t.Errorf("%s error = %v, want one wrapping ErrUnsupportedPolicy", call, err)
					} else if value := strconv.Itoa(int(policy)); !strings.Contains(err.Error(), value) {
						t.Errorf("%s error = %q, want it to name the value %s", call, err, value)
					}
					if out != nil {
						t.Errorf("%s root = %T, want none", call, out)
					}
					if uids != 0 {
						t.Errorf("%s drew %d uids, want none: nothing is built", call, uids)
					}
					if source.draws != 0 {
						t.Errorf("%s drew %d values from ValueSource, want none: nothing is built", call, source.draws)
					}
				})
			}
		}
	}
}
