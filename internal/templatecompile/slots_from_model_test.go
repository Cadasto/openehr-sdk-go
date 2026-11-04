package templatecompile

// slots_from_model_test.go: the compile step's slot rules, built from the
// operational template model's includes and excludes, held equal to the
// deprecated parser's over every corpus slot, and the archetype-id patterns
// and path predicate read from the same assertions.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// slotAssertionPatterns returns the patterns of the compiled assertions.
func slotAssertionPatterns(as []constraints.SlotAssertion) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.Pattern())
	}
	return out
}

func TestREQ111_SlotRulesFromModelMatchParserOverCorpus(t *testing.T) {
	var slots, includes, excludes int
	for _, p := range corpusModelNodePairs(t) {
		parsed, ok := p.parsed.(*template.Slot)
		if !ok {
			continue
		}
		model, ok := p.model.(*aom14.ArchetypeSlot)
		if !ok {
			t.Errorf("%s: the parser reads a slot where the model holds a %T", p.where, p.model)
			continue
		}
		slots++
		want, got := parsed.SlotRules(), slotRulesFromModel(model)
		includes += len(got.Includes)
		excludes += len(got.Excludes)
		if got.RMTypeName != want.RMTypeName {
			t.Errorf("%s: slot rules RMTypeName = %q; want the parser's %q", p.where, got.RMTypeName, want.RMTypeName)
		}
		if g, w := slotAssertionPatterns(got.Includes), slotAssertionPatterns(want.Includes); !slices.Equal(g, w) {
			t.Errorf("%s: slot rules include patterns = %q; want the parser's %q", p.where, g, w)
		}
		if g, w := slotAssertionPatterns(got.Excludes), slotAssertionPatterns(want.Excludes); !slices.Equal(g, w) {
			t.Errorf("%s: slot rules exclude patterns = %q; want the parser's %q", p.where, g, w)
		}
		if got.RawIncludeCount != want.RawIncludeCount {
			t.Errorf("%s: slot rules RawIncludeCount = %d; want the parser's %d", p.where, got.RawIncludeCount, want.RawIncludeCount)
		}

		// Each corpus assertion gives one pattern, and every one compiles,
		// so the patterns of the includes and excludes are those of the
		// rules, and the predicate is the first include's pattern with its
		// backslashes stripped.
		if g, w := slotPatterns(model.Includes), slotAssertionPatterns(want.Includes); !slices.Equal(g, w) {
			t.Errorf("%s: slotPatterns(includes) = %q; want the parser's rule patterns %q", p.where, g, w)
		}
		if g, w := slotPatterns(model.Excludes), slotAssertionPatterns(want.Excludes); !slices.Equal(g, w) {
			t.Errorf("%s: slotPatterns(excludes) = %q; want the parser's rule patterns %q", p.where, g, w)
		}
		wantPredicate := ""
		if len(want.Includes) > 0 {
			wantPredicate = strings.ReplaceAll(want.Includes[0].Pattern(), `\`, "")
		}
		if got := slotPathPredicateFromModel(model); got != wantPredicate {
			t.Errorf("%s: slotPathPredicateFromModel = %q; want %q", p.where, got, wantPredicate)
		}
	}
	if slots == 0 || includes == 0 || excludes == 0 {
		t.Errorf("the corpus walk compared %d slots, %d includes and %d excludes; want some of each", slots, includes, excludes)
	}
}

// matchesAssertion returns an include or exclude as the corpus writes one:
// the string expression, when stringExpr is not empty, and the expression tree
// that matches archetype_id/value against pattern, when pattern is not empty.
func matchesAssertion(stringExpr, pattern string) aom14.Assertion {
	var a aom14.Assertion
	if stringExpr != "" {
		a.StringExpression = new(stringExpr)
	}
	if pattern != "" {
		a.Expression = &aom14.ExprBinaryOperator{
			Type:         "Boolean",
			Operator:     aom14.OperatorKindMatches,
			LeftOperand:  &aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: &aom14.TypedString{Value: "archetype_id/value", Type: "xs:string"}},
			RightOperand: &aom14.ExprLeaf{Type: "C_STRING", ReferenceType: "constraint", Item: &aom14.CString{Pattern: new(pattern)}},
		}
	}
	return a
}

// TestREQ111_SlotPatternsFromModel pins how the compile step reads an
// assertion's archetype-id pattern from the model: the text form first, then
// the expression tree, each trimmed and stripped of its slashes, one pattern
// listed once; an expression that is not the archetype_id/value match gives
// none.
func TestREQ111_SlotPatternsFromModel(t *testing.T) {
	cases := []struct {
		name      string
		assertion aom14.Assertion
		want      []string
	}{
		{
			name:      "string expression and expression tree giving one pattern",
			assertion: matchesAssertion(`archetype_id/value matches {/openEHR-EHR-CLUSTER\.device\.v1/}`, `openEHR-EHR-CLUSTER\.device\.v1`),
			want:      []string{`openEHR-EHR-CLUSTER\.device\.v1`},
		},
		{
			name:      "padded pattern trimmed",
			assertion: matchesAssertion(`archetype_id/value matches { /openEHR-EHR-CLUSTER\..*\.v1/ }`, ""),
			want:      []string{`openEHR-EHR-CLUSTER\..*\.v1`},
		},
		{
			name:      "expression tree only",
			assertion: matchesAssertion("", " openEHR-EHR-ITEM_TREE\\..*\\.v1 "),
			want:      []string{`openEHR-EHR-ITEM_TREE\..*\.v1`},
		},
		{
			name:      "the two forms differing give both, the text form first",
			assertion: matchesAssertion(`archetype_id/value matches {/a\.v1/}`, `b\.v1`),
			want:      []string{`a\.v1`, `b\.v1`},
		},
		{
			name: "an expression tree whose right operand names its type String gives its C_STRING's pattern",
			assertion: aom14.Assertion{Expression: &aom14.ExprBinaryOperator{
				Operator:     aom14.OperatorKindMatches,
				LeftOperand:  &aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: "archetype_id/value"},
				RightOperand: &aom14.ExprLeaf{Type: "String", ReferenceType: "constraint", Item: &aom14.CString{Pattern: new(`openEHR-EHR-CLUSTER\.device\.v1`)}},
			}},
			want: []string{`openEHR-EHR-CLUSTER\.device\.v1`},
		},
		{
			name: "an expression tree whose right operand is no C_STRING gives none",
			assertion: aom14.Assertion{Expression: &aom14.ExprBinaryOperator{
				Operator:     aom14.OperatorKindMatches,
				LeftOperand:  &aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: "archetype_id/value"},
				RightOperand: &aom14.ExprLeaf{Type: "C_STRING", ReferenceType: "constraint", Item: "openEHR-EHR-CLUSTER.device.v1"},
			}},
			want: nil,
		},
		{
			name: "an expression tree on another attribute gives none",
			assertion: aom14.Assertion{Expression: &aom14.ExprBinaryOperator{
				Operator:     aom14.OperatorKindMatches,
				LeftOperand:  &aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: "template_id/value"},
				RightOperand: &aom14.ExprLeaf{Type: "C_STRING", ReferenceType: "constraint", Item: &aom14.CString{Pattern: new("x")}},
			}},
			want: nil,
		},
		{
			name: "an expression tree with another operator gives none",
			assertion: aom14.Assertion{Expression: &aom14.ExprBinaryOperator{
				Operator:     aom14.OperatorKindEqual,
				LeftOperand:  &aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: "archetype_id/value"},
				RightOperand: &aom14.ExprLeaf{Type: "C_STRING", ReferenceType: "constraint", Item: &aom14.CString{Pattern: new("x")}},
			}},
			want: nil,
		},
		{
			name:      "a string expression with no match gives none",
			assertion: matchesAssertion("archetype_id/value is anything", ""),
			want:      nil,
		},
		{
			name:      "an empty assertion gives none",
			assertion: aom14.Assertion{},
			want:      nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := assertionSlotPatterns(tc.assertion); !slices.Equal(got, tc.want) {
				t.Errorf("assertionSlotPatterns = %q; want %q", got, tc.want)
			}
		})
	}
}

// TestREQ111_SlotRulesAndPredicateFromModel pins the slot rules, the patterns
// and the path predicate of slots the corpus does not hold: an assertion
// that gives no pattern, one whose pattern does not compile, an empty one, and
// a slot with no include.
func TestREQ111_SlotRulesAndPredicateFromModel(t *testing.T) {
	slot := &aom14.ArchetypeSlot{
		RMTypeName: " CLUSTER ",
		Includes: []aom14.Assertion{
			{},
			matchesAssertion("archetype_id/value is anything", ""),
			matchesAssertion(`archetype_id/value matches {/openEHR-EHR-CLUSTER\.(a|b\.v1/}`, ""),
			matchesAssertion(`archetype_id/value matches {/openEHR-EHR-CLUSTER\.device\.v1/}`, ""),
		},
		Excludes: []aom14.Assertion{matchesAssertion(`archetype_id/value matches {/.*/}`, "")},
	}
	rules := slotRulesFromModel(slot)
	if rules.RMTypeName != "CLUSTER" {
		t.Errorf("slotRulesFromModel RMTypeName = %q; want %q", rules.RMTypeName, "CLUSTER")
	}
	if got, want := slotAssertionPatterns(rules.Includes), []string{`openEHR-EHR-CLUSTER\.device\.v1`}; !slices.Equal(got, want) {
		t.Errorf("slotRulesFromModel include patterns = %q; want %q, the pattern that does not compile left out", got, want)
	}
	if got, want := slotAssertionPatterns(rules.Excludes), []string{".*"}; !slices.Equal(got, want) {
		t.Errorf("slotRulesFromModel exclude patterns = %q; want %q", got, want)
	}
	if rules.RawIncludeCount != 3 {
		t.Errorf("slotRulesFromModel RawIncludeCount = %d; want 3, the empty assertion not counted", rules.RawIncludeCount)
	}
	if got, want := slotPatterns(slot.Includes), []string{`openEHR-EHR-CLUSTER\.(a|b\.v1`, `openEHR-EHR-CLUSTER\.device\.v1`}; !slices.Equal(got, want) {
		t.Errorf("slotPatterns(includes) = %q; want %q", got, want)
	}
	if got, want := slotPathPredicateFromModel(slot), "openEHR-EHR-CLUSTER.(a|b.v1"; got != want {
		t.Errorf("slotPathPredicateFromModel = %q; want %q", got, want)
	}
	if got := slotPathPredicateFromModel(&aom14.ArchetypeSlot{RMTypeName: "CLUSTER"}); got != "" {
		t.Errorf("slotPathPredicateFromModel(no include) = %q; want empty", got)
	}
	if got := slotPathPredicateFromModel(&aom14.ArchetypeSlot{Includes: []aom14.Assertion{{}}}); got != "" {
		t.Errorf("slotPathPredicateFromModel(an include with no pattern) = %q; want empty", got)
	}
	if got := slotRulesFromModel(&aom14.ArchetypeSlot{RMTypeName: "CLUSTER"}); got.Includes != nil || got.Excludes != nil || got.RawIncludeCount != 0 {
		t.Errorf("slotRulesFromModel(no assertion) = %+v; want no rules", got)
	}
}
