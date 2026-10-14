package validation

// engine_bench_test.go: the cost of the one walk against the walks it
// replaces, on the corpus compositions BenchmarkValidateComposition measures
// and on a synthetic composition an order of magnitude larger than the
// largest of them: with no template, against the floor; with a template,
// against the template walk and the floor run one after the other. The
// composition is decoded, and its template compiled, before the timer
// starts.

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// syntheticID is the corpus composition the synthetic one is built from,
// the largest the corpus holds, and syntheticCopies how many copies of its
// content the synthetic one holds.
const (
	syntheticID     = "Demonstration.v1"
	syntheticCopies = 10
)

// syntheticName names the synthetic composition in the benchmarks.
const syntheticName = syntheticID + "x10"

// benchCase is one composition a benchmark validates, with its compiled
// template.
type benchCase struct {
	name string
	comp *rm.Composition
	c    *templatecompile.Compiled
}

// decodeComposition decodes the corpus composition of the template id, as
// it is decoded afresh each call, so two calls share no value.
func decodeComposition(tb testing.TB, raw []byte, id string) *rm.Composition {
	tb.Helper()
	var comp rm.Composition
	if err := json.Unmarshal(raw, &comp); err != nil {
		tb.Fatalf("decode %s: %v", id, err)
	}
	return &comp
}

// syntheticComposition returns the corpus composition syntheticID holding
// its content syntheticCopies times. Each copy is decoded afresh, so no two
// copies share a value, and each item carries its own uid, so the copies
// are distinct items with one archetype, which the template binds to the
// same node. The rest of the composition is held once.
func syntheticComposition(tb testing.TB) *rm.Composition {
	tb.Helper()
	raw, err := os.ReadFile(fixtures.CompositionJSON(syntheticID))
	if err != nil {
		tb.Fatalf("read %s: %v", syntheticID, err)
	}
	comp := decodeComposition(tb, raw, syntheticID)
	comp.Content = nil
	for copyIndex := range syntheticCopies {
		for itemIndex, item := range decodeComposition(tb, raw, syntheticID).Content {
			obs, ok := item.(*rm.Observation)
			if !ok {
				tb.Fatalf("%s content[%d] is %T, want an OBSERVATION", syntheticID, itemIndex, item)
			}
			obs.UID = &rm.HierObjectID{Value: fmt.Sprintf("8c0f5b0e-1d2a-4c3b-9e4f-%012d", copyIndex*100+itemIndex)}
			comp.Content = append(comp.Content, obs)
		}
	}
	return comp
}

// benchCases returns the corpus compositions BMI and syntheticID and the
// synthetic composition, each with its compiled template.
func benchCases(tb testing.TB) []benchCase {
	tb.Helper()
	var out []benchCase
	for _, id := range []string{"BMI", syntheticID} {
		raw, err := os.ReadFile(fixtures.CompositionJSON(id))
		if err != nil {
			tb.Fatalf("read %s: %v", id, err)
		}
		out = append(out, benchCase{name: id, comp: decodeComposition(tb, raw, id), c: compileFixture(tb, id)})
	}
	return append(out, benchCase{name: syntheticName, comp: syntheticComposition(tb), c: out[1].c})
}

// compileFixture parses and compiles the corpus template id.
func compileFixture(tb testing.TB, id string) *templatecompile.Compiled {
	tb.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOpt(id))
	if err != nil {
		tb.Fatalf("parse %s: %v", id, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		tb.Fatalf("compile %s: %v", id, err)
	}
	return c
}

// REQ-171 § Acceptance (benchmark) — the synthetic composition is an order
// of magnitude larger than the corpus composition it is built from, measured
// as the number of values the one walk visits with no template, which is
// every value of the composition that has an RM rule to run or an attribute
// to read: between 9.5 and 10.5 times as many. Its content holds the same
// defects in each copy, so the RM rules report syntheticCopies times the
// issues they report on the composition it is built from.
func TestREQ171_SyntheticCompositionIsTenTimesTheLargestCorpusComposition(t *testing.T) {
	cases := benchCases(t)
	visits := func(comp *rm.Composition) int {
		_, e := engineWalk(comp, nil, Limits{})
		return e.visits
	}
	base, synthetic := visits(cases[1].comp), visits(cases[2].comp)
	ratio := float64(synthetic) / float64(base)
	t.Logf("%s: %d values visited; %s: %d, %.2f times as many", syntheticID, base, syntheticName, synthetic, ratio)
	if ratio < 9.5 || ratio > 10.5 {
		t.Errorf("%s visits %d values, %.2f times the %d of %s, want about ten times", syntheticName, synthetic, ratio, base, syntheticID)
	}
	baseIssues, syntheticIssues := len(ValidateRM(cases[1].comp).Issues), len(ValidateRM(cases[2].comp).Issues)
	if baseIssues == 0 || syntheticIssues != syntheticCopies*baseIssues {
		t.Errorf("ValidateRM(%s) reports %d issues, want %d times the %d of %s", syntheticName, syntheticIssues, syntheticCopies, baseIssues, syntheticID)
	}
}

// BenchmarkEngineWithoutTemplate measures, per composition, the floor alone
// (ValidateRM) and the one walk with no template, which returns the same
// result.
func BenchmarkEngineWithoutTemplate(b *testing.B) {
	for _, bc := range benchCases(b) {
		b.Run(bc.name+"/floor", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = ValidateRM(bc.comp)
			}
		})
		b.Run(bc.name+"/engine", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = runEngine(bc.comp, nil, Limits{})
			}
		})
	}
}

// BenchmarkEngineWithTemplate measures, per composition and its template,
// the two walks (Validate: the template walk, then the floor) and the one
// walk with the template.
func BenchmarkEngineWithTemplate(b *testing.B) {
	for _, bc := range benchCases(b) {
		b.Run(bc.name+"/two-walks", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = Validate(bc.comp, bc.c)
			}
		})
		b.Run(bc.name+"/engine", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = runEngine(bc.comp, bc.c, Limits{})
			}
		})
	}
}
