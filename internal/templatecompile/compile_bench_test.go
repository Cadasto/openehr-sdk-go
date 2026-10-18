package templatecompile_test

// compile_bench_test.go: the cost of compiling a template, which every caller
// of Compile pays once per template: on the smallest and the largest corpus
// templates a validation benchmark uses, and on clinical_notes.v0, a
// slot-heavy template the parse benchmark also uses. The OPT is parsed before
// the timer starts.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// BenchmarkCompile measures Compile on the corpus templates BMI,
// Demonstration.v1 and clinical_notes.v0.
func BenchmarkCompile(b *testing.B) {
	for _, id := range []string{"BMI", "Demonstration.v1", "clinical_notes.v0"} {
		opt, err := template.ParseFile(fixtures.TemplateOpt(id))
		if err != nil {
			b.Fatalf("parse %s: %v", id, err)
		}
		b.Run(id, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := templatecompile.Compile(opt); err != nil {
					b.Fatalf("compile %s: %v", id, err)
				}
			}
		})
	}
}
