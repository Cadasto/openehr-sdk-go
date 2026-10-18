package instance_test

// generate_bench_test.go: the cost of generating an instance, which every
// composition builder pays when it starts from a skeleton: on
// Demonstration.v1, a large corpus template, and on vital_signs, a common
// clinical one, under both policies. The OPT is parsed and compiled before
// the timer starts; the clock is fixed and the uids come from a counter, so
// every iteration does the same work.

import (
	"strconv"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// BenchmarkGenerate measures Generate on Demonstration.v1 and vital_signs,
// under Minimal and Example, with the example fill.
func BenchmarkGenerate(b *testing.B) {
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, id := range []string{"Demonstration.v1", "vital_signs"} {
		opt, err := template.ParseFile(fixtures.TemplateOpt(id))
		if err != nil {
			b.Fatalf("parse %s: %v", id, err)
		}
		c, err := templatecompile.Compile(opt)
		if err != nil {
			b.Fatalf("compile %s: %v", id, err)
		}
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			b.Run(id+"/"+policy.String(), func(b *testing.B) {
				uid := 0
				opts := instance.Options{
					Policy:    policy,
					Territory: "NL",
					Composer:  testComposer(),
					Now:       now,
					UIDSource: func() *rm.HierObjectID {
						uid++
						return &rm.HierObjectID{Value: "uid-" + strconv.Itoa(uid)}
					},
				}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := instance.Generate(b.Context(), c, opts); err != nil {
						b.Fatalf("Generate %s (%s): %v", id, policy, err)
					}
				}
			})
		}
	}
}
