package lint_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	aqlprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/aql"
)

// BenchmarkLintSuite lints every query of the vendored upstream AQL suite
// corpus (the PROBE-107 input) that parses, once per iteration, with no
// options. The queries are parsed before the timed loop, so the numbers are
// the lint pass alone: one op is one pass over the whole suite.
func BenchmarkLintSuite(b *testing.B) {
	c, err := aqlprobes.ReadSuiteCorpus(fixtures.AQLSuitesRoot())
	if err != nil {
		b.Fatalf("ReadSuiteCorpus: %v", err)
	}
	var docs []*parse.Document
	for _, q := range c.Queries {
		doc, err := parse.Parse(q.Query)
		if err != nil {
			continue
		}
		docs = append(docs, doc)
	}
	if len(docs) == 0 {
		b.Fatal("the suite corpus yielded no query that parses")
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, doc := range docs {
			lint.Lint(doc, nil)
		}
	}
	b.ReportMetric(float64(len(docs)), "queries/op")
}

// BenchmarkLintValid lints one typical query, the lint corpus's valid.aql: a
// single projected path under an archetype-scoped OBSERVATION.
func BenchmarkLintValid(b *testing.B) {
	src, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), "aql", "lint", "valid.aql"))
	if err != nil {
		b.Fatalf("read valid.aql: %v", err)
	}
	doc, err := parse.Parse(string(src))
	if err != nil {
		b.Fatalf("Parse(valid.aql): %v", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		lint.Lint(doc, nil)
	}
}
