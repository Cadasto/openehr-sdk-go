package aom14_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// benchTemplate is the largest operational template vendored under
// testkit/corpus/templates (384 122 bytes of XML).
const benchTemplate = "Demonstration.v1.opt"

// BenchmarkDecodeOperationalTemplateJSON measures the canonical-JSON decode
// of the largest operational template the corpus holds, which the corpus
// holds as XML only: the setup reads it from XML and writes it as canonical
// JSON once, and decodes that once, as the control a template that stopped
// decoding would fail rather than quietly measuring an error path. b.SetBytes
// reports against the JSON document.
func BenchmarkDecodeOperationalTemplateJSON(b *testing.B) {
	raw, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), "templates", benchTemplate))
	if err != nil {
		b.Fatalf("read %s: %v", benchTemplate, err)
	}
	var opt aom14.OperationalTemplate
	if err := canxml.Unmarshal(raw, &opt); err != nil {
		b.Fatalf("read %s from XML: %v", benchTemplate, err)
	}
	payload, err := canjson.Marshal(&opt)
	if err != nil {
		b.Fatalf("write %s as canonical JSON: %v", benchTemplate, err)
	}
	if err := canjson.Unmarshal(payload, new(aom14.OperationalTemplate)); err != nil {
		b.Fatalf("setup decode of %s: %v", benchTemplate, err)
	}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for b.Loop() {
		var got aom14.OperationalTemplate
		if err := canjson.Unmarshal(payload, &got); err != nil {
			b.Fatalf("Unmarshal: %v", err)
		}
	}
}
