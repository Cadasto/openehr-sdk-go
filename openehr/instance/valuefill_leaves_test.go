package instance_test

import (
	"fmt"
	mrand "math/rand/v2"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// leafConstraint returns the one primitive constraint of type T the
// compiled template carries.
func leafConstraint[T constraints.PrimitiveConstraint](t *testing.T, c *templatecompile.Compiled) T {
	t.Helper()
	var found []T
	var walk func(n *tcimpl.CompiledNode)
	walk = func(n *tcimpl.CompiledNode) {
		if pc, ok := n.PrimitiveConstraint().(T); ok {
			found = append(found, pc)
		}
		for _, attr := range n.Attributes() {
			for _, child := range attr.Children() {
				walk(child)
			}
		}
	}
	walk(c.Root())
	if len(found) != 1 {
		t.Fatalf("compiled template carries %d constraints of type %T, want 1", len(found), *new(T))
	}
	return found[0]
}

// TestREQ107_RandomFillDrawsEachLeafKind is the REQ-107 check that RandomFill
// values a constrained leaf with a draw from its constraint, for a C_DATE,
// a C_TIME, a C_STRING list and a C_CODE_PHRASE list: over eight seeds the
// values vary and each passes the constraint, and ExampleFill gives the
// constraint's example value. It holds under both policies and both
// compile modes.
func TestREQ107_RandomFillDrawsEachLeafKind(t *testing.T) {
	element := func(value string) string { return optTemplate("ELEMENT", optSingle("value", value)) }
	cases := []struct {
		name string
		opt  string
		// leaf reads the generated leaf value; check validates it against
		// the compiled constraint and returns the constraint's example.
		leaf  func(out any) any
		check func(t *testing.T, c *templatecompile.Compiled, v any) (example any, violations int)
	}{
		{
			name: "C_DATE",
			opt:  element(optNode("DV_DATE", "", optSingle("value", optPrimitive("DATE", "C_DATE", "")))),
			leaf: func(out any) any { return out.(*rm.Element).Value.(*rm.DVDate).Value },
			check: func(t *testing.T, c *templatecompile.Compiled, v any) (any, int) {
				pc := leafConstraint[constraints.CDate](t, c)
				return pc.ExampleValue(), len(pc.Validate(v))
			},
		},
		{
			name: "C_TIME",
			opt:  element(optNode("DV_TIME", "", optSingle("value", optPrimitive("TIME", "C_TIME", "")))),
			leaf: func(out any) any { return out.(*rm.Element).Value.(*rm.DVTime).Value },
			check: func(t *testing.T, c *templatecompile.Compiled, v any) (any, int) {
				pc := leafConstraint[constraints.CTime](t, c)
				return pc.ExampleValue(), len(pc.Validate(v))
			},
		},
		{
			name: "C_STRING list",
			opt: element(optNode("DV_TEXT", "", optStringAttr("value",
				"<list>a</list><list>b</list><list>c</list><list>d</list><list>e</list><list>f</list>"))),
			leaf: func(out any) any { return out.(*rm.Element).Value.(*rm.DVText).Value },
			check: func(t *testing.T, c *templatecompile.Compiled, v any) (any, int) {
				pc := leafConstraint[constraints.CString](t, c)
				return pc.ExampleValue(), len(pc.Validate(v))
			},
		},
		{
			name: "C_CODE_PHRASE list",
			opt:  element(optCodedText("local", "at0001", "at0002", "at0003", "at0004", "at0005", "at0006")),
			leaf: func(out any) any {
				dc := out.(*rm.Element).Value.(*rm.DVCodedText).DefiningCode
				return constraints.CodedTermRef{Terminology: dc.TerminologyID.Value, CodeString: dc.CodeString}
			},
			check: func(t *testing.T, c *templatecompile.Compiled, v any) (any, int) {
				pc := leafConstraint[constraints.CodePhrase](t, c)
				return pc.ExampleValue(), len(pc.Validate(v))
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", tc.name, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
					if err != nil {
						t.Fatalf("Generate(ExampleFill): %v", err)
					}
					got := tc.leaf(out)
					if example, _ := tc.check(t, c, got); got != example {
						t.Errorf("ExampleFill leaf = %v, want the constraint's example value %v", got, example)
					}
					seen := map[any]bool{}
					for seed := range uint64(8) {
						out, err := instance.Generate(t.Context(), c, instance.Options{
							Policy: policy, Now: defaultsNow,
							ValueFill: instance.RandomFill, ValueSource: mrand.NewPCG(seed, seed),
						})
						if err != nil {
							t.Fatalf("Generate(RandomFill, seed %d): %v", seed, err)
						}
						v := tc.leaf(out)
						if _, n := tc.check(t, c, v); n != 0 {
							t.Errorf("RandomFill seed %d leaf = %v, which the constraint rejects", seed, v)
						}
						seen[v] = true
					}
					if len(seen) < 2 {
						t.Errorf("RandomFill leaf took %d distinct values over 8 seeds, want them to vary", len(seen))
					}
				})
			}
		}
	}
}
