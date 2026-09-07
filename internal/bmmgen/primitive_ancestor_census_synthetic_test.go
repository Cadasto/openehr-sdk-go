package bmmgen

// STRAND-13 evidence, second half: the real-schema census
// (primitive_ancestor_census_test.go) reports exactly one drop, and one
// positive case cannot show that the traversal behind it is right. A
// regression in transitiveAncestors or in the declared-property filter could
// just as easily empty the census — which reads as "the strand is closed" — or
// inflate it. These cases drive the same census core over hand-built schemas
// that a vendored openEHR schema does not contain: deeper ancestry, a property
// redeclared at each level, a second ancestor branch, a cycle, and a dangling
// ancestor name.
//
// The synthetic classes are real *bmm.SimpleClass values, so every case goes
// through the same classProperties and Class.Ancestors() path the schema does.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// syntheticClass is one hand-built BMM class: the property names it declares
// and the ancestor names it lists.
type syntheticClass struct {
	props     []string
	ancestors []string
}

// syntheticLookupBudget caps how many name resolutions one case may make —
// far above what any case below needs (the largest walks a handful of names).
// It is the cycle guard: transitiveAncestors terminates on a cyclic schema only
// because it keeps a visited set, and without that set the walk would recurse
// until the stack gave out rather than fail a test. Past the budget the lookup
// reports a miss, the walk unwinds, and the case fails on the overflow flag.
const syntheticLookupBudget = 200

// syntheticSchema resolves synthetic classes, answers the primitive-mapped
// question, and counts resolutions for the budget above.
type syntheticSchema struct {
	classes   map[string]syntheticClass
	primitive map[string]bool
	lookups   int
	overflow  bool
}

func (s *syntheticSchema) lookup(name string) (bmm.Class, bool) {
	s.lookups++
	if s.lookups > syntheticLookupBudget {
		s.overflow = true
		return nil, false
	}
	sc, ok := s.classes[name]
	if !ok {
		return nil, false
	}
	c := &bmm.SimpleClass{}
	c.Name = name
	c.Ancestors_ = sc.ancestors
	for _, prop := range sc.props {
		p := &bmm.SingleProperty{TypeName: "String"}
		p.Name = prop
		if c.Properties == nil {
			c.Properties = map[string]bmm.Property{}
		}
		c.Properties[prop] = p
		c.PropertyOrder = append(c.PropertyOrder, prop)
	}
	return c, true
}

func (s *syntheticSchema) isPrimitive(name string) bool { return s.primitive[name] }

func TestCensusPrimitiveAncestorDropsSynthetic(t *testing.T) { // STRAND-13
	cases := []struct {
		name string
		// guards names the regression the case is here to catch.
		guards    string
		classes   []string
		schema    map[string]syntheticClass
		primitive []string
		want      []string
	}{
		{
			name:   "transitive ancestry reaches a primitive two hops up",
			guards: "a transitiveAncestors that only walked direct ancestors would drop C.v via P entirely, because P is B's parent, not C's",
			// C -> B -> P, and only P declares v.
			classes:   []string{"B", "C"},
			primitive: []string{"P"},
			schema: map[string]syntheticClass{
				"P": {props: []string{"v"}},
				"B": {ancestors: []string{"P"}},
				"C": {ancestors: []string{"B"}},
			},
			want: []string{"B.v via P", "C.v via P"},
		},
		{
			name:   "a class redeclaring the property is not a drop",
			guards: "skipping the declared-property filter would report C.v via P even though C declares v itself and the generator emits it — the DV_DATE shape, which is why the real census reports one drop and not five",
			// C -> P, both declare v.
			classes:   []string{"C"},
			primitive: []string{"P"},
			schema: map[string]syntheticClass{
				"P": {props: []string{"v"}},
				"C": {props: []string{"v"}, ancestors: []string{"P"}},
			},
			want: nil,
		},
		{
			name:   "a non-primitive ancestor redeclaring the property shields its descendants",
			guards: "skipping the non-primitive-ancestor arm of the declared-property filter would report C.v via P, even though C inherits a planned v from B; D shows the case is not vacuously empty",
			// B -> P and B redeclares v, so nothing under B is dropped;
			// C -> B is shielded only by B's declaration. D -> P has
			// nothing in between and is dropped.
			classes:   []string{"B", "C", "D"},
			primitive: []string{"P"},
			schema: map[string]syntheticClass{
				"P": {props: []string{"v"}},
				"B": {props: []string{"v"}, ancestors: []string{"P"}},
				"C": {ancestors: []string{"B"}},
				"D": {ancestors: []string{"P"}},
			},
			want: []string{"D.v via P"},
		},
		{
			name:   "a primitive reached through the second ancestor is still counted",
			guards: "a walk that took only the first name in ancestors would miss C.v via P — multiple inheritance is the normal shape in the RM (DV_DATE lists DV_TEMPORAL before Iso8601_date)",
			// C -> [X, P]; X declares nothing.
			classes:   []string{"C", "X"},
			primitive: []string{"P"},
			schema: map[string]syntheticClass{
				"P": {props: []string{"v"}},
				"X": {},
				"C": {ancestors: []string{"X", "P"}},
			},
			want: []string{"C.v via P"},
		},
		{
			name:   "a cyclic schema terminates and reports each drop once",
			guards: "losing the visited set in transitiveAncestors: the walk then re-enters A -> B -> A without end, trips the lookup budget, and this case fails on overflow instead of hanging",
			// A -> [B, P] and B -> A: a cycle with a primitive hanging
			// off it. A malformed schema, not one openEHR publishes —
			// which is exactly why only a synthetic case can pin it.
			classes:   []string{"A", "B"},
			primitive: []string{"P"},
			schema: map[string]syntheticClass{
				"P": {props: []string{"v"}},
				"A": {ancestors: []string{"B", "P"}},
				"B": {ancestors: []string{"A"}},
			},
			want: []string{"A.v via P", "B.v via P"},
		},
		{
			name:   "an ancestor name with no definition is skipped",
			guards: "a walk that stopped at the first ancestor name it cannot resolve would lose C.v via P, which sits behind the dangling name in C's ancestor list",
			// Missing is named but not defined; D names nothing else.
			classes:   []string{"C", "D"},
			primitive: []string{"P"},
			schema: map[string]syntheticClass{
				"P": {props: []string{"v"}},
				"C": {ancestors: []string{"Missing", "P"}},
				"D": {ancestors: []string{"Missing"}},
			},
			want: []string{"C.v via P"},
		},
		{
			name:      "a primitive ancestor that declares nothing is not a drop",
			guards:    "keying the census off primitive ancestry alone rather than off the primitive's declared properties — most primitive-mapped ancestors (Temporal, for one) declare nothing and must produce no entry",
			classes:   []string{"C"},
			primitive: []string{"P"},
			schema: map[string]syntheticClass{
				"P": {},
				"C": {ancestors: []string{"P"}},
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &syntheticSchema{classes: tc.schema, primitive: map[string]bool{}}
			for _, name := range tc.primitive {
				s.primitive[name] = true
			}
			got := censusPrimitiveAncestorDrops(tc.classes, s.lookup, s.isPrimitive)
			if s.overflow {
				t.Fatalf("censusPrimitiveAncestorDrops(%v) made more than %d name lookups: the ancestor walk is not terminating.\nthis case guards: %s", tc.classes, syntheticLookupBudget, tc.guards)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("censusPrimitiveAncestorDrops(%v) = %v, want %v\nthis case guards: %s", tc.classes, got, tc.want, tc.guards)
			}
		})
	}
}
