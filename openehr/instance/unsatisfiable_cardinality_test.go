package instance_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
)

// optSectionChild is a SECTION child at nodeID whose occurrences are
// lower..upper (upper -1 is unbounded), with attrs.
func optSectionChild(nodeID string, lower, upper int, attrs ...string) string {
	return optOccurring("C_COMPLEX_OBJECT", "SECTION", nodeID, lower, upper, attrs...)
}

// checkUnsatisfiableDetail fails t unless err wraps ErrConstraintUnsatisfiable,
// wraps neither ErrSlotFillUnsupported nor ErrArchetypeIDMissing, and reads
// as the sentinel followed by detail.
func checkUnsatisfiableDetail(t *testing.T, call string, err error, detail string) {
	t.Helper()
	if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
		t.Errorf("%s error = %v, want one wrapping ErrConstraintUnsatisfiable", call, err)
		return
	}
	for _, other := range []error{instance.ErrSlotFillUnsupported, instance.ErrArchetypeIDMissing} {
		if errors.Is(err, other) {
			t.Errorf("%s error = %v, want it not to wrap %v", call, err, other)
		}
	}
	got, ok := strings.CutPrefix(err.Error(), instance.ErrConstraintUnsatisfiable.Error()+": ")
	if !ok || got != detail {
		t.Errorf("%s error = %q, want %q followed by %q", call, err, instance.ErrConstraintUnsatisfiable, detail)
	}
}

// TestREQ107_UnsatisfiableCardinalityIsRefused is the REQ-107 Errors check
// that a list the OPT declares multi-valued, whose cardinality upper bound
// is bounded and less than the sum of the occurrences lower bounds of its
// allowed children, makes Generate return an error wrapping
// ErrConstraintUnsatisfiable that names the RM type, the attribute and the
// OPT path, and no root, under every policy, both value fills and both
// compile modes: no instance holds every member those children ask for
// within that bound (ADL 1.4 VCOC, AOM 2 VACMCO). The control rows still
// generate a root that passes the RM floor and the template validator: a
// sum equal to the bound, a child the OPT leaves optional, and children
// that state no occurrences, which add nothing to the sum.
func TestREQ107_UnsatisfiableCardinalityIsRefused(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// detail is what the error says after the sentinel's own text; ""
		// marks a control row, which must generate.
		detail string
	}{
		{
			name:   "SECTION items at cardinality 0..1 over two 1..1 children",
			opt:    optTemplate("SECTION", optCardinal("items", 0, 1, optSectionChild("at0001", 1, 1), optSectionChild("at0002", 1, 1))),
			detail: "SECTION.items at /: the occurrences lower bounds of its children add up to 2, past its cardinality upper bound 1",
		},
		{
			name: "COMPOSITION content at cardinality 1..1 over children whose lower bounds add up to 8",
			opt: optTemplate("COMPOSITION", optCardinal("content", 1, 1,
				optSectionChild("at0001", 2, 2), optSectionChild("at0002", 3, 3), optSectionChild("at0003", 3, -1))),
			detail: "COMPOSITION.content at /: the occurrences lower bounds of its children add up to 8, past its cardinality upper bound 1",
		},
		{
			name:   "a nested SECTION's items at cardinality 1..2 over a 2..2 and a 1..1 child",
			opt:    optTemplate("SECTION", optCardinal("items", 1, 1, optSectionChild("at0001", 1, 1, optCardinal("items", 1, 2, optSectionChild("at0002", 2, 2), optSectionChild("at0003", 1, 1))))),
			detail: "SECTION.items at /items[at0001]: the occurrences lower bounds of its children add up to 3, past its cardinality upper bound 2",
		},
		{
			name: "control: SECTION items at cardinality 0..2 over two 1..1 children",
			opt:  optTemplate("SECTION", optCardinal("items", 0, 2, optSectionChild("at0001", 1, 1), optSectionChild("at0002", 1, 1))),
		},
		{
			name: "control: SECTION items at cardinality 0..1 over a 1..1 and a 0..1 child",
			opt:  optTemplate("SECTION", optCardinal("items", 0, 1, optSectionChild("at0001", 1, 1), optSectionChild("at0002", 0, 1))),
		},
		{
			name: "control: SECTION items at cardinality 0..1 over two children that state no occurrences",
			opt:  optTemplate("SECTION", optCardinal("items", 0, 1, optNode("SECTION", "at0001"), optNode("SECTION", "at0002"))),
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if tc.detail == "" {
						if err != nil {
							t.Fatalf("Generate: %v, want a root", err)
						}
						checkValid(t, out, c)
						return
					}
					checkUnsatisfiableDetail(t, "Generate", err, tc.detail)
					if out != nil {
						t.Errorf("Generate returned %T with the error, want no root", out)
					}
				})
			}
		}
	}
}

// TestREQ107_UnsatisfiableCardinalityIsRefusedWhereVisited is the REQ-107
// Errors and Exceptions check that a list whose children's occurrences lower
// bounds add up past its cardinality upper bound is refused only where the
// walk visits it: held by an optional SECTION, it makes Generate return an
// error wrapping ErrConstraintUnsatisfiable and no root under Large and
// Medium, which build that SECTION, and Required, which builds no member of
// an optional child, returns a root without the list that passes the RM
// floor and the template validator. It holds under both value fills and
// both compile modes.
func TestREQ107_UnsatisfiableCardinalityIsRefusedWhereVisited(t *testing.T) {
	opt := optTemplate("SECTION", optCardinal("items", 0, 1,
		optSectionChild("at0001", 0, 1, optCardinal("items", 0, 1, optSectionChild("at0002", 1, 1), optSectionChild("at0003", 1, 1)))))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range guardOptions() {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if opts.Policy == instance.Required {
					if err != nil {
						t.Fatalf("Generate: %v, want a root", err)
					}
					if n := len(sectionItems(t, out)); n != 0 {
						t.Errorf("Generate wrote %d members in the root's items, want none", n)
					}
					checkValid(t, out, c)
					return
				}
				checkUnsatisfiableDetail(t, "Generate", err,
					"SECTION.items at /items[at0001]: the occurrences lower bounds of its children add up to 2, past its cardinality upper bound 1")
				if out != nil {
					t.Errorf("Generate returned %T with the error, want no root", out)
				}
			})
		}
	}
}
