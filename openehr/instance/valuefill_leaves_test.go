package instance_test

import (
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"strings"
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
// constraint's example value. It holds under every policy and both
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
			for _, policy := range everyPolicy {
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

// TestREQ107_RandomFillDrawsRoutedPrimitiveLeaves is the REQ-107 Leaf values
// check that RandomFill values a primitive leaf on a secondary attribute of a
// data value, which the generator writes through the template-instance
// writer, with a draw from its constraint, as it does a leaf on the main
// value: a DV_QUANTITY's precision over a range of integers, a DV_QUANTITY's,
// a DV_COUNT's and a DV_DURATION's accuracy over a range of reals, and a
// DV_QUANTITY's accuracy_is_percent and a DV_STATE's is_terminal where the
// constraint admits both Booleans. Over eight seeds the values vary and each
// passes the constraint, and ExampleFill gives the constraint's example
// value. It holds under every policy and both compile modes.
func TestREQ107_RandomFillDrawsRoutedPrimitiveLeaves(t *testing.T) {
	const bothBooleans = "<true_valid>true</true_valid><false_valid>true</false_valid>"
	integers := optPrimitive("INTEGER", "C_INTEGER", "<range>"+optInterval(0, 50)+"</range>")
	reals := optPrimitive("REAL", "C_REAL", "<range>"+optInterval(0, 50)+"</range>")
	booleans := optPrimitive("BOOLEAN", "C_BOOLEAN", bothBooleans)
	cases := []struct {
		name  string
		value string
		// leaf reads the generated leaf value; constraint is the
		// compiled constraint it is checked against.
		leaf       func(v rm.DataValue) any
		constraint func(t *testing.T, c *templatecompile.Compiled) constraints.PrimitiveConstraint
	}{
		{
			name:  "DV_QUANTITY precision",
			value: optNode("DV_QUANTITY", "", optSingle("precision", integers)),
			leaf:  func(v rm.DataValue) any { return int64(*v.(*rm.DVQuantity).Precision) },
			constraint: func(t *testing.T, c *templatecompile.Compiled) constraints.PrimitiveConstraint {
				return leafConstraint[constraints.CInteger](t, c)
			},
		},
		{
			name:  "DV_QUANTITY accuracy",
			value: optNode("DV_QUANTITY", "", optSingle("accuracy", reals)),
			leaf:  func(v rm.DataValue) any { return float64(*v.(*rm.DVQuantity).Accuracy) },
			constraint: func(t *testing.T, c *templatecompile.Compiled) constraints.PrimitiveConstraint {
				return leafConstraint[constraints.CReal](t, c)
			},
		},
		{
			name:  "DV_COUNT accuracy",
			value: optNode("DV_COUNT", "", optSingle("accuracy", reals)),
			leaf:  func(v rm.DataValue) any { return float64(*v.(*rm.DVCount).Accuracy) },
			constraint: func(t *testing.T, c *templatecompile.Compiled) constraints.PrimitiveConstraint {
				return leafConstraint[constraints.CReal](t, c)
			},
		},
		{
			name:  "DV_DURATION accuracy",
			value: optNode("DV_DURATION", "", optSingle("accuracy", reals)),
			leaf:  func(v rm.DataValue) any { return float64(*v.(*rm.DVDuration).Accuracy) },
			constraint: func(t *testing.T, c *templatecompile.Compiled) constraints.PrimitiveConstraint {
				return leafConstraint[constraints.CReal](t, c)
			},
		},
		{
			name:  "DV_QUANTITY accuracy_is_percent",
			value: optNode("DV_QUANTITY", "", optSingle("accuracy_is_percent", booleans)),
			leaf:  func(v rm.DataValue) any { return *v.(*rm.DVQuantity).AccuracyIsPercent },
			constraint: func(t *testing.T, c *templatecompile.Compiled) constraints.PrimitiveConstraint {
				return leafConstraint[constraints.CBoolean](t, c)
			},
		},
		{
			name:  "DV_STATE is_terminal",
			value: optNode("DV_STATE", "", optSingle("is_terminal", booleans)),
			leaf:  func(v rm.DataValue) any { return v.(*rm.DVState).IsTerminal },
			constraint: func(t *testing.T, c *templatecompile.Compiled) constraints.PrimitiveConstraint {
				return leafConstraint[constraints.CBoolean](t, c)
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			pc := tc.constraint(t, c)
			for _, policy := range everyPolicy {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", tc.name, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
					if err != nil {
						t.Fatalf("Generate(ExampleFill): %v", err)
					}
					example := pc.ExampleValue()
					if got := tc.leaf(out.(*rm.Element).Value); got != example {
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
						v := tc.leaf(out.(*rm.Element).Value)
						if violations := pc.Validate(v); len(violations) != 0 {
							t.Errorf("RandomFill seed %d leaf = %v, which the constraint rejects: %v", seed, v, violations)
						}
						seen[v] = true
					}
					if len(seen) < 2 {
						t.Errorf("RandomFill leaf took %d distinct values over 8 seeds, want them to vary: it is the constraint's draw, not its example value %v", len(seen), example)
					}
				})
			}
		}
	}
}

// TestREQ107_IntegerLeafIsNarrowedToTheIntegerRange is the REQ-107
// Precedence 3 check that an RM type limit narrows a value the OPT leaves
// open: an RM Integer is 32 bits wide, so a C_INTEGER on an Integer attribute
// (a DV_QUANTITY's precision) whose range reaches past 32 bits is valued from
// the part of the range inside the Integer range, under ExampleFill and under
// RandomFill alike, and Generate does not fail where the OPT admits such a
// value. Where the range and the Integer range share no value, below or above
// it, Generate returns an error wrapping ErrConstraintUnsatisfiable that says
// the constraint is outside the 32-bit Integer range. It holds under every
// policy and both compile modes.
func TestREQ107_IntegerLeafIsNarrowedToTheIntegerRange(t *testing.T) {
	const (
		maxInteger = 1<<31 - 1
		past       = 5_000_000_000
		above      = 3_000_000_000
	)
	precision := func(lower, upper int) string {
		return optNode("DV_QUANTITY", "", optSingle("precision",
			optPrimitive("INTEGER", "C_INTEGER", "<range>"+optInterval(lower, upper)+"</range>")))
	}
	generate := func(t *testing.T, c *templatecompile.Compiled, policy instance.Policy, fill instance.ValueFill, seed uint64) (any, error) {
		t.Helper()
		return instance.Generate(t.Context(), c, instance.Options{
			Policy: policy, Now: defaultsNow, ValueFill: fill, ValueSource: mrand.NewPCG(seed, seed),
		})
	}
	admitted := []struct {
		name         string
		value        string
		lower, upper int64 // the values the OPT and the Integer range share
	}{
		{"range past the upper end", precision(0, past), 0, maxInteger},
		{"range with no upper bound near the upper end", precision(maxInteger-20, -1), maxInteger - 20, maxInteger},
	}
	for _, tc := range admitted {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			for _, policy := range everyPolicy {
				for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
					t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, policy, fill), func(t *testing.T) {
						seen := map[rm.Integer]bool{}
						for seed := range uint64(8) {
							out, err := generate(t, c, policy, fill, seed)
							if err != nil {
								t.Fatalf("Generate(seed %d): %v, want a root: the OPT admits values inside the Integer range", seed, err)
							}
							q := out.(*rm.Element).Value.(*rm.DVQuantity)
							if q.Precision == nil || int64(*q.Precision) < tc.lower || int64(*q.Precision) > tc.upper {
								t.Fatalf("DV_QUANTITY.precision = %v (seed %d), want a value within %d..%d", q.Precision, seed, tc.lower, tc.upper)
							}
							seen[*q.Precision] = true
							checkValid(t, out, c)
						}
						if fill == instance.RandomFill && len(seen) < 2 {
							t.Errorf("precision took %d distinct values over 8 seeds, want them to vary", len(seen))
						}
					})
				}
			}
		}
	}

	refused := []struct {
		name  string
		value string
	}{
		{"range above the Integer range", precision(above, past)},
		{"range below the Integer range", precision(-past, -above)},
	}
	for _, tc := range refused {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			for _, policy := range everyPolicy {
				for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
					t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, policy, fill), func(t *testing.T) {
						out, err := generate(t, c, policy, fill, 1)
						if err == nil {
							t.Fatalf("Generate = %T, nil error, want an error: no value of the range is an Integer", out)
						}
						if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
							t.Errorf("Generate error = %v, want one wrapping ErrConstraintUnsatisfiable", err)
						}
						for _, want := range []string{"DV_QUANTITY.precision", "32-bit Integer range"} {
							if !strings.Contains(err.Error(), want) {
								t.Errorf("Generate error = %q, want it to say %q", err, want)
							}
						}
					})
				}
			}
		}
	}
}
