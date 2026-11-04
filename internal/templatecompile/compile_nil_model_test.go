package templatecompile_test

// compile_nil_model_test.go: how Compile reads a model built in code that
// holds a nil pointer where the model's decode writes a value: it refuses the
// model or reads the part as absent, and never panics.

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ111_CompileRefusesNilModelParts pins that Compile refuses, with an
// error wrapping ErrInvalidInput, a model holding a nil pointer where it
// needs a value: a C_PRIMITIVE_OBJECT's item that is a nil pointer to a
// primitive class, which is no item missing from the model but an item with
// nothing in it, and a default value that is a nil pointer to a data value.
func TestREQ111_CompileRefusesNilModelParts(t *testing.T) {
	primitive := func(item aom14.CPrimitive) *aom14.OperationalTemplate {
		p := &aom14.CPrimitiveObject{RMTypeName: "STRING", Occurrences: once, Item: item}
		return modelOf(single("context", complexObject("EVENT_CONTEXT", "", single("other_context", p))))
	}
	cases := []struct {
		name  string
		model *aom14.OperationalTemplate
	}{
		{name: "a nil pointer to a C_BOOLEAN item", model: primitive((*aom14.CBoolean)(nil))},
		{name: "a nil pointer to a C_INTEGER item", model: primitive((*aom14.CInteger)(nil))},
		{name: "a nil pointer to a C_REAL item", model: primitive((*aom14.CReal)(nil))},
		{name: "a nil pointer to a C_STRING item", model: primitive((*aom14.CString)(nil))},
		{name: "a nil pointer to a C_DATE item", model: primitive((*aom14.CDate)(nil))},
		{name: "a nil pointer to a C_TIME item", model: primitive((*aom14.CTime)(nil))},
		{name: "a nil pointer to a C_DATE_TIME item", model: primitive((*aom14.CDateTime)(nil))},
		{name: "a nil pointer to a C_DURATION item", model: primitive((*aom14.CDuration)(nil))},
		{name: "a default value that is a nil pointer to a DV_CODED_TEXT", model: func() *aom14.OperationalTemplate {
			m := modelOf(single("category", complexObject("DV_CODED_TEXT", "")))
			m.Constraints = &aom14.TConstraint{Attributes: []aom14.TAttribute{{
				DifferentialPath: "[openEHR-EHR-COMPOSITION.model.v1]",
				RMAttributeName:  "category",
				Children:         []aom14.TComplexObject{{RMTypeName: "DV_CODED_TEXT", DefaultValue: (*rm.DVCodedText)(nil)}},
			}}}
			return m
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := templatecompile.Compile(tc.model); !errors.Is(err, templatecompile.ErrInvalidInput) {
				t.Fatalf("Compile error = %v, want an error wrapping ErrInvalidInput", err)
			}
		})
	}
}

// TestREQ111_CompileReadsNilSlotExpressionPartsAsNoPattern pins that a slot
// include whose expression tree holds a nil pointer where the match
// `archetype_id/value matches {pattern}` has a part gives no pattern, as any
// other tree that is not that match does: Compile admits the slot, which
// counts the include and lists no pattern for it.
func TestREQ111_CompileReadsNilSlotExpressionPartsAsNoPattern(t *testing.T) {
	left := func() *aom14.ExprLeaf {
		return &aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: &aom14.TypedString{Value: "archetype_id/value"}}
	}
	right := func() *aom14.ExprLeaf {
		return &aom14.ExprLeaf{Type: "C_STRING", ReferenceType: "constraint", Item: &aom14.CString{Pattern: new(".*")}}
	}
	match := func(l, r aom14.ExprItem) *aom14.ExprBinaryOperator {
		return &aom14.ExprBinaryOperator{Type: "Boolean", Operator: aom14.OperatorKindMatches, LeftOperand: l, RightOperand: r}
	}
	cases := []struct {
		name string
		expr aom14.ExprItem
	}{
		{name: "a nil pointer to the operator", expr: (*aom14.ExprBinaryOperator)(nil)},
		{name: "a nil pointer to the left operand", expr: match((*aom14.ExprLeaf)(nil), right())},
		{name: "a nil pointer to the right operand", expr: match(left(), (*aom14.ExprLeaf)(nil))},
		{name: "a nil pointer to the left operand's text", expr: match(
			&aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: (*aom14.TypedString)(nil)}, right())},
		{name: "a nil pointer to the right operand's C_STRING", expr: match(
			left(), &aom14.ExprLeaf{Type: "C_STRING", ReferenceType: "constraint", Item: (*aom14.CString)(nil)})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A slot with no node_id, so its path predicate is read from
			// its include as well.
			slot := &aom14.ArchetypeSlot{RMTypeName: "SECTION", Occurrences: once, Includes: []aom14.Assertion{{Expression: tc.expr}}}
			c, err := templatecompile.Compile(modelOf(multiple("content", slot)))
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			node, err := c.NodeAt("/content[@1]")
			if err != nil {
				t.Fatalf("NodeAt(/content[@1]): %v", err)
			}
			if got := node.SlotIncludes(); len(got) != 0 {
				t.Errorf("SlotIncludes() = %q, want none", got)
			}
			if rules := node.SlotRules(); rules.RawIncludeCount != 1 || !rules.IncludesDroppedUnparsed() {
				t.Errorf("SlotRules() = %+v, want the one include counted and given no pattern", rules)
			}
		})
	}
}
