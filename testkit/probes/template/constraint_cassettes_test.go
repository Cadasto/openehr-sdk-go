package templateprobes_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templatecompile/walk"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-103 — primitive constraints on vendored Test_dv_* OPTs accept ExampleValue
// (REQ-107). Templates in constraintExampleValueExcluded are omitted.
func TestConstraintTemplates_CompiledExampleValues(t *testing.T) {
	ids, err := fixtures.ConstraintTemplateIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("no constraint template cassettes discovered")
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			if fixtures.ConstraintExampleValueExcluded(id) {
				t.Skip("ExampleValue walk skipped (REQ-107 gap)")
			}
			opt, err := template.ParseFile(fixtures.TemplateOpt(id))
			if err != nil {
				t.Fatal(err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatal(err)
			}
			var failures []string
			err = walk.Walk(c, walk.VisitorFunc{
				Pre: func(ctx *walk.Context) error {
					pc := ctx.Node().PrimitiveConstraint()
					if pc == nil {
						return nil
					}
					v := pc.ExampleValue()
					if viol := pc.Validate(v); len(viol) != 0 {
						failures = append(failures, fmt.Sprintf("%s: %v", ctx.Path(), viol))
					}
					return nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(failures) > 0 {
				t.Fatalf("primitive ExampleValue failures:\n%s", strings.Join(failures, "\n"))
			}
		})
	}
}
