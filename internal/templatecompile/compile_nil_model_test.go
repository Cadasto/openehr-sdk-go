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

// TestREQ111_CompileReadsNilPrimitiveItemAsNoItem pins that Compile reads a
// C_PRIMITIVE_OBJECT whose item is a nil pointer to a primitive class as one
// with no item: in its default mode as no constraint, and with
// Options.Strict as a value it refuses with an error wrapping
// ErrInvalidInput.
func TestREQ111_CompileReadsNilPrimitiveItemAsNoItem(t *testing.T) {
	const path = "/context/other_context"
	primitive := func(item aom14.CPrimitive) *aom14.OperationalTemplate {
		p := &aom14.CPrimitiveObject{RMTypeName: "STRING", Occurrences: once, Item: item}
		return modelOf(single("context", complexObject("EVENT_CONTEXT", "", single("other_context", p))))
	}
	cases := []struct {
		name string
		item aom14.CPrimitive
	}{
		{name: "a nil pointer to a C_BOOLEAN item", item: (*aom14.CBoolean)(nil)},
		{name: "a nil pointer to a C_INTEGER item", item: (*aom14.CInteger)(nil)},
		{name: "a nil pointer to a C_REAL item", item: (*aom14.CReal)(nil)},
		{name: "a nil pointer to a C_STRING item", item: (*aom14.CString)(nil)},
		{name: "a nil pointer to a C_DATE item", item: (*aom14.CDate)(nil)},
		{name: "a nil pointer to a C_TIME item", item: (*aom14.CTime)(nil)},
		{name: "a nil pointer to a C_DATE_TIME item", item: (*aom14.CDateTime)(nil)},
		{name: "a nil pointer to a C_DURATION item", item: (*aom14.CDuration)(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := templatecompile.Compile(primitive(tc.item))
			if err != nil {
				t.Fatalf("Compile = %v, want no error", err)
			}
			n, err := c.NodeAt(path)
			if err != nil {
				t.Fatalf("NodeAt(%s) = %v", path, err)
			}
			if got := n.PrimitiveConstraint(); got != nil {
				t.Errorf("PrimitiveConstraint() = %#v, want none", got)
			}
			if _, err := templatecompile.Compile(primitive(tc.item), templatecompile.Options{Strict: true}); !errors.Is(err, templatecompile.ErrInvalidInput) {
				t.Errorf("Compile(Strict) error = %v, want an error wrapping ErrInvalidInput", err)
			}
		})
	}
}

// TestREQ111_CompileRefusesNilDefaultValue pins that Compile refuses, in
// both of its modes and with an error wrapping ErrInvalidInput, a default
// value that is a nil pointer to a data value, which does not encode as
// canonical JSON.
func TestREQ111_CompileRefusesNilDefaultValue(t *testing.T) {
	m := modelOf(single("category", complexObject("DV_CODED_TEXT", "")))
	m.Constraints = &aom14.TConstraint{Attributes: []aom14.TAttribute{{
		DifferentialPath: "[openEHR-EHR-COMPOSITION.model.v1]",
		RMAttributeName:  "category",
		Children:         []aom14.TComplexObject{{RMTypeName: "DV_CODED_TEXT", DefaultValue: (*rm.DVCodedText)(nil)}},
	}}}
	for _, strict := range []bool{false, true} {
		if _, err := templatecompile.Compile(m, templatecompile.Options{Strict: strict}); !errors.Is(err, templatecompile.ErrInvalidInput) {
			t.Errorf("Compile(Strict: %t) error = %v, want an error wrapping ErrInvalidInput", strict, err)
		}
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
