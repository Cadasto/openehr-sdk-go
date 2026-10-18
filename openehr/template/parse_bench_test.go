package template_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// BenchmarkParseFile measures a lenient parse of three corpus OPTs of
// growing size: BMI (small), Demonstration.v1 (large, many primitive
// constraints) and clinical_notes.v0 (slot-heavy).
func BenchmarkParseFile(b *testing.B) {
	for _, id := range []string{"BMI", "Demonstration.v1", "clinical_notes.v0"} {
		path := fixtures.TemplateOpt(id)
		b.Run(id, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := template.ParseFile(path); err != nil {
					b.Fatalf("ParseFile(%s): %v", id, err)
				}
			}
		})
	}
}
