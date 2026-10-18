package webtemplate_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// benchBuildTemplates are the templates BenchmarkBuild measures, smallest
// first: BMI.opt is 85 KB, clinical_notes.v0.opt 113 KB and
// Demonstration.v1.opt 384 KB on disk.
var benchBuildTemplates = []string{"BMI", "clinical_notes.v0", "Demonstration.v1"}

// BenchmarkBuild measures building the Web Template of a compiled template.
// Parsing and compiling happen once, before the timed loop, so the figure is
// the cost of Build alone. A template that no longer parses, compiles or
// builds fails the benchmark rather than measuring an error path.
func BenchmarkBuild(b *testing.B) {
	for _, id := range benchBuildTemplates {
		b.Run(id, func(b *testing.B) {
			opt, err := template.ParseFile(fixtures.TemplateOpt(id))
			if err != nil {
				b.Fatalf("parse %s: %v", id, err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				b.Fatalf("compile %s: %v", id, err)
			}
			if _, err := webtemplate.Build(c); err != nil {
				b.Fatalf("setup build of %s: %v", id, err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := webtemplate.Build(c); err != nil {
					b.Fatalf("Build(%s): %v", id, err)
				}
			}
		})
	}
}
