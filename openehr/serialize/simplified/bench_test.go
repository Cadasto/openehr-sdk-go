package simplified_test

// bench_test.go: FLAT codec baselines for the move of the canonical-JSON path
// to encoding/json/v2 (docs/plans/2026-09-14-json-v2-migration.md, phase 3.5).
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

// benchFlatBody is the largest body in the FLAT conformance corpus that this
// SDK's codec accepts end to end (10 164 bytes).
//
// The migration plan names ehrbase_conformance_party_related.json, the largest
// body in the corpus at 17 648 bytes. That body cannot be used: it carries
// composer party sub-structure (`composer/_identifier:0|assigner`), which no
// ctx/ short form can express and which the decoder refuses at the ADR 0015
// boundary, so there is nothing to measure. This is the largest body that both
// decodes and re-encodes.
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
// The corpus holds 34 bodies and the codec accepts 24 of them today. The other
// ten exercise upstream key families the codec declines by design: unmodelled
// Web Template paths, PARTY_PROXY and party sub-structure on the composer, and
// bare values for EVENT and DV_PROPORTION. The split is discovered here rather
// than written down, so a codec that learns one of those families widens the
// sweep on its own. Because the accepted count is part of what one sweep costs,
// a ns/op figure only compares against another run over the same accepted set.
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
	if len(bodies) == 0 {
		b.Fatalf("no body in the %d-body FLAT conformance corpus decoded and re-encoded", len(names))
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
