package rmjsonwalk_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// BenchmarkWalk measures a walk with both hooks on three corpus compositions,
// a small, a middling and the largest the corpus holds, read as COMPOSITION
// roots: the cost canjson's WithMissing and WithUnmatched, and ValidateJSON,
// add to a decode.
func BenchmarkWalk(b *testing.B) {
	m, err := rmjsonwalk.Default()
	if err != nil {
		b.Fatalf("Default(): %v", err)
	}
	hooks := rmjsonwalk.Hooks{
		Unmatched: func(string) error { return nil },
		Missing:   func(string) error { return nil },
	}
	root := rmjsonwalk.Root{Type: "COMPOSITION"}
	for _, id := range []string{"body_weight", "BMI", "Demonstration.v1"} {
		data, err := os.ReadFile(fixtures.CompositionJSON(id))
		if err != nil {
			b.Fatalf("read %s: %v", id, err)
		}
		b.Run(id, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if err := m.Walk(data, root, hooks); err != nil {
					b.Fatalf("Walk(%s): %v", id, err)
				}
			}
		})
	}
}
