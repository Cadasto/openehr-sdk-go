package validation

// engine_bench_test.go: the cost of the one walk against the walks it
// replaces, on the corpus compositions BenchmarkValidateComposition measures:
// with no template, against the floor; with a template, against the template
// walk and the floor run one after the other. The composition is decoded, and
// its template compiled, before the timer starts.

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// BenchmarkEngineWithoutTemplate measures, per corpus composition, the floor
// alone (ValidateRM) and the one walk with no template, which returns the
// same result.
func BenchmarkEngineWithoutTemplate(b *testing.B) {
	for _, id := range []string{"BMI", "Demonstration.v1"} {
		raw, err := os.ReadFile(fixtures.CompositionJSON(id))
		if err != nil {
			b.Fatalf("read %s: %v", id, err)
		}
		var comp rm.Composition
		if err := json.Unmarshal(raw, &comp); err != nil {
			b.Fatalf("decode %s: %v", id, err)
		}
		b.Run(id+"/floor", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = ValidateRM(&comp)
			}
		})
		b.Run(id+"/engine", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = runEngine(&comp, nil, Limits{})
			}
		})
	}
}

// BenchmarkEngineWithTemplate measures, per corpus composition and its
// template, the two walks (Validate: the template walk, then the floor) and
// the one walk with the template.
func BenchmarkEngineWithTemplate(b *testing.B) {
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
		b.Run(id+"/two-walks", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = Validate(&comp, c)
			}
		})
		b.Run(id+"/engine", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = runEngine(&comp, c, Limits{})
			}
		})
	}
}
