package templatecompile_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// strictModel is a model whose definition, a COMPOSITION archetype root,
// holds category, a DV_CODED_TEXT whose defining_code is a CONSTRAINT_REF
// with reference ref, and whose constraints give the category the default
// value addressed by path.
func strictModel(ref, path string) *aom14.OperationalTemplate {
	once := rm.Interval[aom14.Integer]{Lower: 1, Upper: 1, LowerIncluded: true, UpperIncluded: true}
	code := &aom14.ConstraintRef{RMTypeName: "CODE_PHRASE", Occurrences: once, Reference: ref}
	category := &aom14.CComplexObject{RMTypeName: "DV_CODED_TEXT", Occurrences: once, Attributes: []aom14.CAttribute{
		&aom14.CSingleAttribute{RMAttributeName: "defining_code", Existence: once, Children: []aom14.CObject{code}},
	}}
	return &aom14.OperationalTemplate{
		TemplateID: rm.TemplateID{Value: "strict"},
		Definition: aom14.CArchetypeRoot{
			CComplexObject: aom14.CComplexObject{RMTypeName: "COMPOSITION", NodeID: "at0000", Occurrences: once, Attributes: []aom14.CAttribute{
				&aom14.CSingleAttribute{RMAttributeName: "category", Existence: once, Children: []aom14.CObject{category}},
			}},
			ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-COMPOSITION.strict.v1"},
		},
		Constraints: &aom14.TConstraint{Attributes: []aom14.TAttribute{{
			DifferentialPath: path,
			RMAttributeName:  "category",
			Children: []aom14.TComplexObject{{
				RMTypeName:   "DV_CODED_TEXT",
				DefaultValue: &rm.DVCodedText{Value: "event"},
			}},
		}}},
	}
}

// TestREQ111_WithStrict pins that WithStrict reaches the compile step: by
// default Compile reads a constraint reference with no reference with an
// empty one and skips a default value that addresses no node, and with
// WithStrict it refuses each with an error wrapping ErrInvalidInput.
func TestREQ111_WithStrict(t *testing.T) {
	const root = "[openEHR-EHR-COMPOSITION.strict.v1]"
	cases := []struct {
		name        string
		model       *aom14.OperationalTemplate
		wantDefault bool // whether the category carries the default value
	}{
		{name: "a constraint reference with no reference", model: strictModel("", root), wantDefault: true},
		{name: "a default value that addresses no node", model: strictModel("ac0001", root+"/context")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := templatecompile.Compile(tc.model)
			if err != nil {
				t.Fatalf("Compile = %v, want no error", err)
			}
			category, err := c.NodeAt("/category")
			if err != nil {
				t.Fatalf("NodeAt(/category) = %v", err)
			}
			if got := category.DefaultValue() != nil; got != tc.wantDefault {
				t.Errorf("category carries a default value: %t, want %t", got, tc.wantDefault)
			}
			code, err := c.NodeAt("/category/defining_code")
			if err != nil {
				t.Fatalf("NodeAt(/category/defining_code) = %v", err)
			}
			if _, ok := code.PrimitiveConstraint().(constraints.ConstraintRef); !ok {
				t.Errorf("PrimitiveConstraint() = %#v, want a ConstraintRef", code.PrimitiveConstraint())
			}

			if _, err := templatecompile.Compile(tc.model, templatecompile.WithStrict()); !errors.Is(err, templatecompile.ErrInvalidInput) {
				t.Errorf("Compile(WithStrict) error = %v, want an error wrapping ErrInvalidInput", err)
			}
		})
	}
}
