package simplified_test

// bench_test.go: FLAT codec baselines for the move of the canonical-JSON path
// to encoding/json/v2 (PR 171; results in docs/specifications/research-strands.md,
// STRAND-04).
//
// This package stays on encoding/json, so these benchmarks are not measuring a
// package that changes. They measure what the change reaches indirectly: the
// FLAT codec builds and reads rm.Composition values, so every generated type
// the migration touches is on this path. A regression here after the codec
// moves is a regression in the generated types, not in the FLAT codec.

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// minFlatCorpusBodies is a floor on how many FLAT conformance bodies the codec
// must accept before the corpus sweep is worth measuring. It plays the part
// minParityDocuments plays for the parity net in openehr/serialize/canjson: the
// per-operation figure is a cost over a set discovered at setup, so the set has
// to be prevented from shrinking quietly. 30 of the 34 bodies clear it today,
// and the floor sits at that count because a drop of even one body moves the
// number and invalidates the comparison the baseline exists for. (26 cleared
// it before the composer's real path became its carrier, ADR 0029, which
// brought in the four bodies whose composer carries a reference, identifiers
// or a relationship; the floor had stayed at 24.)
const minFlatCorpusBodies = 30

// benchFlatBody is the body the per-body benchmarks measure (10 164 bytes).
//
// The migration named ehrbase_conformance_party_related.json, the largest
// body in the corpus at 17 648 bytes. When the baseline was recorded that body
// could not be used: it carries composer party sub-structure
// (`composer/_identifier:0|assigner`), which the decoder then refused, so this
// was the largest body that both decoded and re-encoded. The composer's real
// path now carries it (ADR 0029) and the larger body decodes too; the measured
// body stays this one, so per-body figures stay comparable with the baseline.
const benchFlatBody = "ehrbase_conformance_Element_feeder_audit"

// benchFlatTarget builds the one operational template the whole FLAT
// conformance corpus instantiates. Every step fails the benchmark rather than
// returning a zero value, so a corpus or template change surfaces as a setup
// failure instead of a suspiciously fast measurement.
func benchFlatTarget(b *testing.B) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	b.Helper()
	opt, err := template.ParseFile(fixtures.FlatConformanceOpt())
	if err != nil {
		b.Fatalf("parse corpus OPT: %v", err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		b.Fatalf("compile corpus OPT: %v", err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		b.Fatalf("build corpus Web Template: %v", err)
	}
	return wt, compiled
}

// benchFlatCorpus reads the FLAT conformance corpus and returns the bodies this
// SDK's codec accepts end to end, together with their total size.
//
// The corpus holds 34 bodies and the codec accepts 30 of them today. The other
// four exercise upstream key families the codec declines by design: the
// reference's in-context keys for RM attributes the Web Template this SDK
// builds does not model (an ISM transition's `_reason`, an interval event's
// `math_function`, a HISTORY's `origin`) and the deferred `_wf_definition`.
// The split is discovered here rather than written down, so a codec that
// learns one of those families widens the sweep on its own.
//
// The accepted count is the denominator of every figure the sweep reports, so
// it is not left implicit: [minFlatCorpusBodies] fails the benchmark if the set
// shrinks, and the sweep reports the count as a bodies/op metric so a reader
// comparing two runs can see at a glance whether they measured the same corpus.
func benchFlatCorpus(b *testing.B, wt *webtemplate.WebTemplate, compiled *templatecompile.Compiled) (bodies [][]byte, totalBytes int64) {
	b.Helper()
	names, err := fixtures.ListFlatConformance()
	if err != nil {
		b.Fatalf("list FLAT conformance corpus: %v", err)
	}
	for _, name := range names {
		raw, err := os.ReadFile(fixtures.FlatConformanceFlat(name))
		if err != nil {
			b.Fatalf("read FLAT body %s: %v", name, err)
		}
		// A body the codec declines is left out of the sweep rather than
		// measured as an error path; the doc comment above says which
		// families those are and why the list is discovered, not written.
		comp, err := simplified.UnmarshalFlat(raw, wt, simplified.WithTemplate(compiled))
		if err != nil {
			continue
		}
		if _, err := simplified.MarshalFlat(comp, wt); err != nil {
			continue
		}
		bodies = append(bodies, raw)
		totalBytes += int64(len(raw))
	}
	if len(bodies) < minFlatCorpusBodies {
		b.Fatalf("%d of the %d bodies in the FLAT conformance corpus decoded and re-encoded, want at least %d; the sweep below would measure a smaller corpus than the baseline it is compared against",
			len(bodies), len(names), minFlatCorpusBodies)
	}
	return bodies, totalBytes
}

// BenchmarkFlatCorpusRoundTrip measures one sweep of the FLAT conformance
// corpus, decoding and re-encoding every body the codec accepts. One b.Loop()
// pass is a whole sweep, so the per-op figure is the cost of the corpus rather
// than of one body.
func BenchmarkFlatCorpusRoundTrip(b *testing.B) {
	wt, compiled := benchFlatTarget(b)
	bodies, totalBytes := benchFlatCorpus(b, wt, compiled)
	b.SetBytes(totalBytes)
	b.ReportAllocs()
	for b.Loop() {
		for _, raw := range bodies {
			comp, err := simplified.UnmarshalFlat(raw, wt, simplified.WithTemplate(compiled))
			if err != nil {
				b.Fatalf("UnmarshalFlat: %v", err)
			}
			if _, err := simplified.MarshalFlat(comp, wt); err != nil {
				b.Fatalf("MarshalFlat: %v", err)
			}
		}
	}
	// The denominator, reported beside ns/op: one operation is one sweep of
	// this many bodies, and two runs are only comparable at the same count.
	// Reported after the loop because the first b.Loop() call resets the timer,
	// which also clears custom metrics registered before it.
	b.ReportMetric(float64(len(bodies)), "bodies/op")
}

// BenchmarkUnmarshalFlat measures FLAT decode of a single large body, the
// composition-building half of the round trip in isolation.
func BenchmarkUnmarshalFlat(b *testing.B) {
	wt, compiled := benchFlatTarget(b)
	raw, err := os.ReadFile(fixtures.FlatConformanceFlat(benchFlatBody))
	if err != nil {
		b.Fatalf("read %s: %v", benchFlatBody, err)
	}
	if _, err := simplified.UnmarshalFlat(raw, wt, simplified.WithTemplate(compiled)); err != nil {
		b.Fatalf("setup decode of %s: %v", benchFlatBody, err)
	}
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := simplified.UnmarshalFlat(raw, wt, simplified.WithTemplate(compiled)); err != nil {
			b.Fatalf("UnmarshalFlat: %v", err)
		}
	}
}

// BenchmarkMarshalFlat measures FLAT encode of the composition decoded from the
// same body, the other half of the round trip.
func BenchmarkMarshalFlat(b *testing.B) {
	wt, compiled := benchFlatTarget(b)
	raw, err := os.ReadFile(fixtures.FlatConformanceFlat(benchFlatBody))
	if err != nil {
		b.Fatalf("read %s: %v", benchFlatBody, err)
	}
	comp, err := simplified.UnmarshalFlat(raw, wt, simplified.WithTemplate(compiled))
	if err != nil {
		b.Fatalf("setup decode of %s: %v", benchFlatBody, err)
	}
	if _, err := simplified.MarshalFlat(comp, wt); err != nil {
		b.Fatalf("setup encode of %s: %v", benchFlatBody, err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := simplified.MarshalFlat(comp, wt); err != nil {
			b.Fatalf("MarshalFlat: %v", err)
		}
	}
}
