package template_test

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// parseModes are the two OPT parse entry points. REQ-100 has both read a
// CONSTRAINT_REF the same way.
var parseModes = []struct {
	name  string
	parse func(io.Reader) (*template.OperationalTemplate, error)
}{
	{name: "ParseOPT", parse: template.ParseOPT},
	{name: "ParseOPTStrict", parse: template.ParseOPTStrict},
}

// constraintRefChild is a CONSTRAINT_REF child as Ocean Template Designer
// writes it; %s is the reference element, or nothing.
const constraintRefChild = `<children xsi:type="CONSTRAINT_REF">
		<rm_type_name>CODE_PHRASE</rm_type_name>
		<occurrences>
			<lower_included>true</lower_included>
			<upper_included>true</upper_included>
			<lower_unbounded>false</lower_unbounded>
			<upper_unbounded>false</upper_unbounded>
			<lower>1</lower>
			<upper>1</upper>
		</occurrences>
		<node_id />
		%s
	</children>`

func constraintRefOPT(reference string) string {
	child := strings.Replace(constraintRefChild, "%s", reference, 1)
	return strings.Replace(primitiveOPTTemplate, "%s", child, 1)
}

// firstChild returns the only child of the root's only attribute.
func firstChild(t *testing.T, tmpl *template.OperationalTemplate) *template.ComplexObject {
	t.Helper()
	root, ok := tmpl.Root().(*template.ComplexObject)
	if !ok {
		t.Fatalf("Root() = %T, want *template.ComplexObject", tmpl.Root())
	}
	attrs := root.Attributes()
	if len(attrs) != 1 || len(attrs[0].Children()) != 1 {
		t.Fatalf("root has %d attributes, want 1 with 1 child", len(attrs))
	}
	co, ok := attrs[0].Children()[0].(*template.ComplexObject)
	if !ok {
		t.Fatalf("child = %T, want *template.ComplexObject", attrs[0].Children()[0])
	}
	return co
}

// REQ-100 / REQ-103: both parse modes read a CONSTRAINT_REF as a leaf of the
// RM type it constrains, carrying a ConstraintRef with its reference.
func TestREQ100_ConstraintRefReadInBothModes(t *testing.T) {
	t.Parallel()
	references := []struct {
		name, element, want string
	}{
		{name: "reference", element: "<reference>ac0001</reference>", want: "ac0001"},
		{name: "reference with surrounding space", element: "<reference>\n\t\tac0002 </reference>", want: "ac0002"},
	}
	for _, mode := range parseModes {
		for _, ref := range references {
			t.Run(mode.name+"/"+ref.name, func(t *testing.T) {
				t.Parallel()
				tmpl, err := mode.parse(strings.NewReader(constraintRefOPT(ref.element)))
				if err != nil {
					t.Fatalf("%s(CONSTRAINT_REF) error = %v, want nil", mode.name, err)
				}
				co := firstChild(t, tmpl)
				if co.RMTypeName() != "CODE_PHRASE" {
					t.Errorf("RMTypeName() = %q, want CODE_PHRASE", co.RMTypeName())
				}
				if n := len(co.Attributes()); n != 0 {
					t.Errorf("len(Attributes()) = %d, want 0 (a leaf)", n)
				}
				if occ := co.Occurrences(); occ == nil || occ.Lower() != 1 || occ.Upper() != 1 || occ.UpperUnbounded() {
					t.Errorf("Occurrences() = %+v, want 1..1", occ)
				}
				want := constraints.ConstraintRef{Reference: ref.want}
				if got := co.PrimitiveConstraint(); got != want {
					t.Errorf("PrimitiveConstraint() = %#v, want %#v", got, want)
				}
			})
		}
	}
}

// REQ-100: a CONSTRAINT_REF without a reference is malformed. Strict mode
// refuses it with ErrInvalidOPT, as it refuses a C_PRIMITIVE_OBJECT without
// its item; lenient mode keeps the leaf with an empty reference.
func TestREQ100_ConstraintRefWithoutReference(t *testing.T) {
	t.Parallel()
	missing := []struct{ name, element string }{
		{name: "no reference element", element: ""},
		{name: "empty reference element", element: "<reference/>"},
		{name: "blank reference", element: "<reference> \n </reference>"},
	}
	for _, tc := range missing {
		t.Run("ParseOPTStrict/"+tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := template.ParseOPTStrict(strings.NewReader(constraintRefOPT(tc.element)))
			if !errors.Is(err, template.ErrInvalidOPT) {
				t.Errorf("ParseOPTStrict(CONSTRAINT_REF, %s) error = %v, want errors.Is(err, ErrInvalidOPT)", tc.name, err)
			}
		})
		t.Run("ParseOPT/"+tc.name, func(t *testing.T) {
			t.Parallel()
			tmpl, err := template.ParseOPT(strings.NewReader(constraintRefOPT(tc.element)))
			if err != nil {
				t.Fatalf("ParseOPT(CONSTRAINT_REF, %s) error = %v, want nil", tc.name, err)
			}
			want := constraints.ConstraintRef{}
			if got := firstChild(t, tmpl).PrimitiveConstraint(); got != want {
				t.Errorf("PrimitiveConstraint() = %#v, want %#v", got, want)
			}
		})
	}
}

// constraintRefCorpus lists, in document order, the references of the
// CONSTRAINT_REF nodes in the vendored OPTs that carry them.
var constraintRefCorpus = []struct {
	template string
	want     []string
}{
	{template: "Demonstration.v1", want: []string{"ac0001", "ac0003", "ac0001", "ac0003", "ac0001", "ac0003", "ac0001", "ac0003"}},
	{template: "TestPerson.v2", want: []string{"ac0001", "ac0007", "ac0004"}},
	{template: "Address.v2", want: []string{"ac0004"}},
}

// constraintRefLeaves walks the tree in document order and returns every
// node carrying a ConstraintRef, with that constraint.
func constraintRefLeaves(n template.Node) (rmTypes, refs []string) {
	if co, ok := n.(*template.ComplexObject); ok {
		if cr, ok := co.PrimitiveConstraint().(constraints.ConstraintRef); ok {
			rmTypes = append(rmTypes, co.RMTypeName())
			refs = append(refs, cr.Reference)
		}
	}
	on, ok := n.(template.ObjectNode)
	if !ok {
		return rmTypes, refs
	}
	for _, a := range on.Attributes() {
		for _, c := range a.Children() {
			ts, rs := constraintRefLeaves(c)
			rmTypes = append(rmTypes, ts...)
			refs = append(refs, rs...)
		}
	}
	return rmTypes, refs
}

// REQ-100 / REQ-103: every CONSTRAINT_REF in the vendored OPTs reaches the
// tree as a CODE_PHRASE leaf carrying its ac reference, in both parse modes.
func TestREQ100_ConstraintRefCorpus(t *testing.T) {
	t.Parallel()
	modes := []struct {
		name  string
		parse func(string) (*template.OperationalTemplate, error)
	}{
		{name: "ParseFile", parse: template.ParseFile},
		{name: "ParseFileStrict", parse: template.ParseFileStrict},
	}
	for _, mode := range modes {
		for _, tc := range constraintRefCorpus {
			t.Run(mode.name+"/"+tc.template, func(t *testing.T) {
				t.Parallel()
				tmpl, err := mode.parse(fixtures.TemplateOpt(tc.template))
				if err != nil {
					t.Fatalf("%s(%s) error = %v, want nil", mode.name, tc.template, err)
				}
				rmTypes, refs := constraintRefLeaves(tmpl.Root())
				if !slices.Equal(refs, tc.want) {
					t.Errorf("%s(%s) ConstraintRef references = %v, want %v", mode.name, tc.template, refs, tc.want)
				}
				for i, rt := range rmTypes {
					if rt != "CODE_PHRASE" {
						t.Errorf("%s(%s) ConstraintRef leaf %d RMTypeName() = %q, want CODE_PHRASE", mode.name, tc.template, i, rt)
					}
				}
			})
		}
	}
}

// compiledConstraintRefs is constraintRefLeaves over the compiled tree.
func compiledConstraintRefs(n *templatecompile.CompiledNode) []string {
	var refs []string
	if cr, ok := n.PrimitiveConstraint().(constraints.ConstraintRef); ok {
		refs = append(refs, cr.Reference)
	}
	for _, a := range n.Attributes() {
		for _, c := range a.Children() {
			refs = append(refs, compiledConstraintRefs(c)...)
		}
	}
	return refs
}

// REQ-103: compiling the template threads each ConstraintRef through to
// CompiledNode.PrimitiveConstraint unchanged.
func TestREQ103_ConstraintRefThreadedThroughCompile(t *testing.T) {
	t.Parallel()
	for _, tc := range constraintRefCorpus {
		t.Run(tc.template, func(t *testing.T) {
			t.Parallel()
			tmpl, err := template.ParseFile(fixtures.TemplateOpt(tc.template))
			if err != nil {
				t.Fatalf("ParseFile(%s) error = %v", tc.template, err)
			}
			compiled, err := templatecompile.Compile(tmpl)
			if err != nil {
				t.Fatalf("Compile(%s) error = %v", tc.template, err)
			}
			if got := compiledConstraintRefs(compiled.Root()); !slices.Equal(got, tc.want) {
				t.Errorf("Compile(%s) ConstraintRef references = %v, want %v", tc.template, got, tc.want)
			}
		})
	}
}
