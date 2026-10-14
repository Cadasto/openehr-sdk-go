package validation

// engine_bench_test.go: the cost of the one walk with no template against the
// floor it replaces, on the corpus compositions BenchmarkValidateComposition
// measures. The composition is decoded before the timer starts.

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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
