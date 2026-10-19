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
// Large, a negative one and one far out of range.
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

// countedOptions are the settings TestREQ107_GenerateRefusesAnUndefinedPolicy
// runs a case under: policy and fill, with a UIDSource that counts into
// uids and a ValueSource that counts its draws.
func countedOptions(policy instance.Policy, fill instance.ValueFill, uids *int, source *countingSource) instance.Options {
	return instance.Options{
		Policy:    policy,
		ValueFill: fill,
		Now:       defaultsNow,
		Territory: "NL",
		Composer:  testComposer(),
		UIDSource: func() *rm.HierObjectID {
			*uids++
			return &rm.HierObjectID{Value: fmt.Sprintf("uid-%d", *uids)}
		},
		ValueSource: source,
	}
}

// TestREQ107_GenerateRefusesAnUndefinedPolicy is the REQ-107 check of
// Contract Inputs: Generate returns an error wrapping ErrUnsupportedPolicy
// that names the value, and no root, for a Policy that is none of Medium,
// Required and Large. It refuses before it builds anything, so it draws
// no uid from UIDSource and no value from ValueSource. It holds on a
// COMPOSITION root and on roots of other classes, under both value fills.
// The COMPOSITION and the OBSERVATION each draw a uid when they are built,
// which a control run under Large shows, so their counters would catch a
// refusal made after building; a CLUSTER or ELEMENT root draws neither, so
// those rows check the error and the missing root only.
func TestREQ107_GenerateRefusesAnUndefinedPolicy(t *testing.T) {
	roots := []struct {
		name     string
		compiled *templatecompile.Compiled
		draws    bool // building the root draws a uid
	}{
		{name: "COMPOSITION vital_signs", compiled: compileFixture(t, "vital_signs"), draws: true},
		{name: "OBSERVATION", compiled: compileOPTText(t, optTemplate("OBSERVATION"), true), draws: true},
		{name: "CLUSTER", compiled: compileOPTText(t, optTemplate("CLUSTER"), true)},
		{name: "ELEMENT", compiled: compileOPTText(t, optTemplate("ELEMENT"), true)},
	}
	for _, root := range roots {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			if root.draws {
				t.Run(fmt.Sprintf("%s/control Large/%v", root.name, fill), func(t *testing.T) {
					uids := 0
					source := &countingSource{src: mrand.NewPCG(1, 2)}
					if _, err := instance.Generate(t.Context(), root.compiled, countedOptions(instance.Large, fill, &uids, source)); err != nil {
						t.Fatalf("Generate(%s, Large, %v): %v", root.name, fill, err)
					}
					if uids == 0 {
						t.Errorf("Generate(%s, Large, %v) drew no uid, want at least one: the counter cannot see a build", root.name, fill)
					}
				})
			}
			for _, policy := range undefinedPolicies {
				t.Run(fmt.Sprintf("%s/policy=%d/%v", root.name, int(policy), fill), func(t *testing.T) {
					uids := 0
					source := &countingSource{src: mrand.NewPCG(1, 2)}
					call := fmt.Sprintf("Generate(%s, Policy(%d), %v)", root.name, int(policy), fill)
					out, err := instance.Generate(t.Context(), root.compiled, countedOptions(policy, fill, &uids, source))
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
