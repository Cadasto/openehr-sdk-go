package templatecompile_test

// compile_bench_test.go: the cost of reading a template into the model and
// compiling it, which every caller of Compile pays once per template: on the
// smallest and the largest corpus templates a validation benchmark uses, and
// on clinical_notes.v0, a slot-heavy template the parse benchmark also uses.
// The OPT's bytes are read from disk before the timer starts.

import (
	"bytes"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// BenchmarkCompile measures aom14.ReadOperationalTemplate followed by Compile
// on the corpus templates BMI, Demonstration.v1 and clinical_notes.v0.
func BenchmarkCompile(b *testing.B) {
	for _, id := range []string{"BMI", "Demonstration.v1", "clinical_notes.v0"} {
		raw, err := os.ReadFile(fixtures.TemplateOpt(id))
		if err != nil {
			b.Fatalf("read %s: %v", id, err)
		}
		b.Run(id, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				model, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
				if err != nil {
					b.Fatalf("read %s: %v", id, err)
				}
				if _, err := templatecompile.Compile(model); err != nil {
					b.Fatalf("compile %s: %v", id, err)
				}
			}
		})
	}
}
