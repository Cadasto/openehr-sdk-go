package template_test

import (
	"os"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

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

// compiledConstraintRefs walks the compiled tree in document order and
// returns the reference of every node carrying a ConstraintRef.
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
			raw, err := os.ReadFile(fixtures.TemplateOpt(tc.template))
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", tc.template, err)
			}
			tmpl, err := fixtures.ReadOperationalTemplate(raw)
			if err != nil {
				t.Fatalf("ReadOperationalTemplate(%s) error = %v", tc.template, err)
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
