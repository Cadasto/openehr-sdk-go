package instance_test

import (
	"encoding/json"
	"fmt"
	mrand "math/rand/v2"
	"testing"
	"time"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// dateTimeAttrs are the attributes the OPT gives a DV_DATE_TIME node: a
// value constrained by a pattern-only C_DATE_TIME, the one form the
// template parser reads (it reads no C_DATE_TIME range), or none, so the
// node carries no primitive constraint.
func dateTimeAttrs(constrained bool) []string {
	if !constrained {
		return nil
	}
	return []string{optSingle("value",
		optPrimitive("DATE_TIME", "C_DATE_TIME", "<pattern>yyyy-mm-ddTHH:MM:SS</pattern>"))}
}

// dateTimeNode is a DV_DATE_TIME node with dateTimeAttrs.
func dateTimeNode(constrained bool) string {
	return optNode("DV_DATE_TIME", "", dateTimeAttrs(constrained)...)
}

// dateTimePlacements put a DV_DATE_TIME where the generator fills it: as
// an ELEMENT value under a CLUSTER, as an ELEMENT value in an
// OBSERVATION's event, and as the template root. values returns every
// value the generator wrote on such a DV_DATE_TIME.
var dateTimePlacements = []struct {
	name   string
	opt    func(constrained bool) string
	values func(t *testing.T, out any) []string
}{
	{
		name: "ELEMENT under CLUSTER",
		opt: func(constrained bool) string {
			return optTemplate("CLUSTER", optMultiple("items",
				optNode("ELEMENT", "at0001", optSingle("value", dateTimeNode(constrained)))))
		},
		values: elementDateTimes,
	},
	{
		name: "ELEMENT under OBSERVATION",
		opt: func(constrained bool) string {
			return optTemplate("OBSERVATION", optSingle("data", optNode("HISTORY", "at0001",
				optMultiple("events", optNode("POINT_EVENT", "at0002",
					optSingle("data", optNode("ITEM_TREE", "at0003",
						optMultiple("items", optNode("ELEMENT", "at0004",
							optSingle("value", dateTimeNode(constrained)))))))))))
		},
		values: elementDateTimes,
	},
	{
		name: "root",
		opt: func(constrained bool) string {
			return optTemplate("DV_DATE_TIME", dateTimeAttrs(constrained)...)
		},
		values: func(t *testing.T, out any) []string {
			dt, ok := out.(*rm.DVDateTime)
			if !ok {
				t.Fatalf("generated root is %T, want *rm.DVDateTime", out)
			}
			return []string{dt.Value}
		},
	},
}

// elementDateTimes returns the value of every DV_DATE_TIME that is an
// ELEMENT's value in the generated tree.
func elementDateTimes(t *testing.T, out any) []string {
	t.Helper()
	b, err := canjson.Marshal(out)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	var values []string
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if n["_type"] == "ELEMENT" {
				if dv, ok := n["value"].(map[string]any); ok && dv["_type"] == "DV_DATE_TIME" {
					s, _ := dv["value"].(string)
					values = append(values, s)
				}
			}
			for _, child := range n {
				walk(child)
			}
		case []any:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(doc)
	return values
}

// dateTimeConstraint returns the C_DATE_TIME the compiled template carries.
func dateTimeConstraint(t *testing.T, c *templatecompile.Compiled) constraints.CDateTime {
	t.Helper()
	var found []constraints.CDateTime
	var walk func(n *tcimpl.CompiledNode)
	walk = func(n *tcimpl.CompiledNode) {
		if dt, ok := n.PrimitiveConstraint().(constraints.CDateTime); ok {
			found = append(found, dt)
		}
		for _, attr := range n.Attributes() {
			for _, child := range attr.Children() {
				walk(child)
			}
		}
	}
	walk(c.Root())
	if len(found) != 1 {
		t.Fatalf("compiled template carries %d C_DATE_TIME constraints, want 1", len(found))
	}
	return found[0]
}

// TestREQ107_ConstrainedDateTimeFollowsValueFill is the REQ-107 check that a
// DV_DATE_TIME whose OPT node carries a primitive constraint is valued as the
// ValueFill in force says, not from the clock: under ExampleFill it is the
// constraint's example value, and under RandomFill a draw the constraint
// accepts that is not the clock. It holds under both policies, with and
// without the implicit attributes, nested and as the root.
func TestREQ107_ConstrainedDateTimeFollowsValueFill(t *testing.T) {
	clock := defaultsNow.Format(time.RFC3339)
	for _, place := range dateTimePlacements {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, place.opt(true), implicit)
			pc := dateTimeConstraint(t, c)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
					t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", place.name, implicit, policy, fill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, instance.Options{
							Policy:      policy,
							ValueFill:   fill,
							ValueSource: mrand.NewPCG(1, 2),
							Now:         defaultsNow,
						})
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						values := place.values(t, out)
						if len(values) != 1 {
							t.Fatalf("generated %d constrained DV_DATE_TIME values, want 1", len(values))
						}
						got := values[0]
						if fill == instance.ExampleFill {
							if want := pc.ExampleValue(); got != want {
								t.Errorf("DV_DATE_TIME.value = %q, want the constraint's example value %q", got, want)
							}
							return
						}
						if got == clock {
							t.Errorf("DV_DATE_TIME.value = %q, the clock, want a RandomFill draw", got)
						}
						if v := pc.Validate(got); len(v) != 0 {
							t.Errorf("DV_DATE_TIME.value = %q, which the constraint rejects: %v", got, v)
						}
					})
				}
			}
		}
	}
}

// TestREQ107_UnconstrainedDateTimeTakesTheClock is the REQ-107 check that a
// DV_DATE_TIME whose OPT node carries no primitive constraint takes its
// value from the clock, under both policies and both value fills, with and
// without the implicit attributes, nested and as the root.
func TestREQ107_UnconstrainedDateTimeTakesTheClock(t *testing.T) {
	clock := defaultsNow.Format(time.RFC3339)
	for _, place := range dateTimePlacements {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, place.opt(false), implicit)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
					t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", place.name, implicit, policy, fill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, instance.Options{
							Policy:      policy,
							ValueFill:   fill,
							ValueSource: mrand.NewPCG(1, 2),
							Now:         defaultsNow,
						})
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						values := place.values(t, out)
						if len(values) != 1 {
							t.Fatalf("generated %d unconstrained DV_DATE_TIME values, want 1", len(values))
						}
						if values[0] != clock {
							t.Errorf("DV_DATE_TIME.value = %q, want the clock %q", values[0], clock)
						}
					})
				}
			}
		}
	}
}
