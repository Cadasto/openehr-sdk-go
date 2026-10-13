package validation

// rmfloor_chained_bench_test.go: the cost of template validation now that it
// includes the RM floor (ADR 0025), against each walk on its own. The
// composition is decoded and the OPT compiled before the timer starts.

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// BenchmarkValidateComposition measures, per corpus composition, the
// template walk alone (what ValidateComposition ran before it included the
// floor), the floor alone (ValidateRM) and the two chained
// (ValidateComposition).
func BenchmarkValidateComposition(b *testing.B) {
	for _, id := range []string{"BMI", "Demonstration.v1"} {
		raw, err := os.ReadFile(fixtures.CompositionJSON(id))
		if err != nil {
			b.Fatalf("read %s: %v", id, err)
		}
		var comp rm.Composition
		if err := json.Unmarshal(raw, &comp); err != nil {
			b.Fatalf("decode %s: %v", id, err)
		}
		opt, err := template.ParseFile(fixtures.TemplateOpt(id))
		if err != nil {
			b.Fatalf("parse %s: %v", id, err)
		}
		c, err := templatecompile.Compile(opt)
		if err != nil {
			b.Fatalf("compile %s: %v", id, err)
		}
		b.Run(id+"/template", func(b *testing.B) {
			for b.Loop() {
				w := newWalker(c)
				w.walkNode(c.Root(), &comp, "/")
				_ = resultFromIssues(w.issues)
			}
		})
		b.Run(id+"/floor", func(b *testing.B) {
			for b.Loop() {
				_ = ValidateRM(&comp)
			}
		})
		b.Run(id+"/chained", func(b *testing.B) {
			for b.Loop() {
				_ = ValidateComposition(&comp, c)
			}
		})
	}
}
