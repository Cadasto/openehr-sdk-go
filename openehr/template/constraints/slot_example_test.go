package constraints_test

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// refArchetypeIDShapeRE and refExampleFromPattern are the example synthesis
// as it stood before NewSlotAssertion computed the example once: a copy kept
// as the reference the cached example must equal.
var refArchetypeIDShapeRE = regexp.MustCompile(`^openEHR-[A-Za-z]+-[A-Za-z0-9_]+\.[A-Za-z0-9_-]+\.v[0-9]+$`)

func refExampleFromPattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	if before, _, ok := strings.Cut(pattern, "|"); ok {
		return refExampleFromPattern(before)
	}
	simplified := strings.ReplaceAll(pattern, `(-[a-zA-Z0-9_]+)*`, "")
	simplified = strings.ReplaceAll(simplified, `\.`, ".")
	if !refArchetypeIDShapeRE.MatchString(simplified) {
		return ""
	}
	if re, err := regexp.Compile(`\A(?:` + pattern + `)\z`); err == nil && re.MatchString(simplified) {
		return simplified
	}
	return ""
}

// slotExampleEdgePatterns cover the shapes the synthesis treats differently,
// beyond what the corpus OPTs carry.
var slotExampleEdgePatterns = []string{
	`openEHR-EHR-CLUSTER\.device\.v1`,                                // escaped dots
	`openEHR-EHR-CLUSTER.device.v1`,                                  // unescaped dots
	`openEHR-EHR-CLUSTER\.device(-[a-zA-Z0-9_]+)*\.v1`,               // specialisation suffix group
	`openEHR-EHR-CLUSTER\.a\.v1(-[a-zA-Z0-9_]+)*`,                    // suffix group at the end
	`openEHR-EHR-CLUSTER\.device\.v1|openEHR-EHR-CLUSTER\.other\.v1`, // alternation: the first wins
	`x|openEHR-EHR-CLUSTER\.device\.v1`,                              // first alternative yields no id
	`|openEHR-EHR-CLUSTER\.device\.v1`,                               // empty first alternative
	`openEHR-EHR-CLUSTER\.device\.v1|`,                               // empty last alternative
	`openEHR-EHR-CLUSTER\.device\.v1||b`,                             // empty middle alternative
	`(openEHR-EHR-CLUSTER\.a\.v1|openEHR-EHR-CLUSTER\.b\.v1)`,        // alternation inside a group
	`openEHR-EHR-CLUSTER\.(a|b)\.v1`,                                 // group holding an alternation
	`openEHR-EHR-CLUSTER\.a\.v1\|b`,                                  // escaped pipe
	`openEHR-EHR-CLUSTER\.a\.v[0-9]+`,                                // character class
	`openEHR-EHR-CLUSTER\.[a-z_]+\.v1`,                               // character class in the concept
	`(?i)openEHR-EHR-CLUSTER\.a\.v1`,                                 // flag group
	"  openEHR-EHR-CLUSTER\\.a\\.v1\t",                               // padded
	`openEHR-EHR-CLUSTER`,                                            // not an archetype id shape
	`.*`,
	`.+`,
	`(.*)`,
}

// corpusSlotPatterns returns every include and exclude pattern of every
// corpus OPT that parses.
func corpusSlotPatterns(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(fixtures.CorpusRoot(), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".opt") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk corpus: %v", err)
	}
	var out []string
	for _, path := range paths {
		opt, err := template.ParseFile(path)
		if err != nil {
			continue // an OPT the parser refuses carries no slot pattern
		}
		walkSlotNodes(opt.Root(), func(s *template.Slot) {
			for _, a := range s.ParsedIncludes() {
				out = append(out, a.Pattern())
			}
			for _, a := range s.ParsedExcludes() {
				out = append(out, a.Pattern())
			}
		})
	}
	return out
}

func walkSlotNodes(n template.Node, fn func(*template.Slot)) {
	if s, ok := n.(*template.Slot); ok {
		fn(s)
	}
	if o, ok := n.(template.ObjectNode); ok {
		for _, a := range o.Attributes() {
			for _, c := range a.Children() {
				walkSlotNodes(c, fn)
			}
		}
	}
}

// REQ-103: the example archetype id NewSlotAssertion computes once equals
// what the synthesis gives when it runs on every call, for every slot
// pattern in the corpus and for the edge shapes above.
func TestSlotRules_ExampleArchetypeIDMatchesUncachedSynthesis(t *testing.T) {
	corpus := corpusSlotPatterns(t)
	if len(corpus) == 0 {
		t.Fatal("no slot pattern found in the corpus OPTs")
	}
	patterns := slices.Concat(slotExampleEdgePatterns, corpus)
	var synthesised, empty int
	for _, p := range patterns {
		a, err := constraints.NewSlotAssertion(p)
		if err != nil {
			t.Errorf("NewSlotAssertion(%q): %v", p, err)
			continue
		}
		rules := constraints.SlotRules{RMTypeName: "CLUSTER", Includes: []constraints.SlotAssertion{a}}
		got := rules.ExampleArchetypeID()
		want := refExampleFromPattern(a.Pattern())
		if got != want {
			t.Errorf("ExampleArchetypeID() for %q = %q, want %q", p, got, want)
		}
		if want == "" {
			empty++
		} else {
			synthesised++
		}
	}
	t.Logf("compared %d patterns (%d from the corpus): %d synthesise an id, %d do not",
		len(patterns), len(corpus), synthesised, empty)
	// Both outcomes must occur, or the comparison proves nothing for one.
	if synthesised == 0 || empty == 0 {
		t.Errorf("compared %d patterns: %d synthesise an id, %d do not; want both above zero",
			len(patterns), synthesised, empty)
	}
}

// The zero SlotAssertion carries no pattern, so it gives no example.
func TestSlotRules_ExampleArchetypeIDZeroAssertion(t *testing.T) {
	rules := constraints.SlotRules{RMTypeName: "CLUSTER", Includes: []constraints.SlotAssertion{{}}}
	if got := rules.ExampleArchetypeID(); got != "" {
		t.Errorf("ExampleArchetypeID() with a zero include = %q, want empty", got)
	}
}

func BenchmarkSlotRulesExampleArchetypeID(b *testing.B) {
	for _, p := range []string{
		`openEHR-EHR-CLUSTER\.device(-[a-zA-Z0-9_]+)*\.v1`,
		`openEHR-EHR-CLUSTER\.health_event(-[a-zA-Z0-9_]+)*\.v1|openEHR-EHR-CLUSTER\.issue(-[a-zA-Z0-9_]+)*\.v1`,
	} {
		a, err := constraints.NewSlotAssertion(p)
		if err != nil {
			b.Fatalf("NewSlotAssertion(%q): %v", p, err)
		}
		rules := constraints.SlotRules{RMTypeName: "CLUSTER", Includes: []constraints.SlotAssertion{a}}
		name := "single"
		if strings.Contains(p, "|") {
			name = "alternation"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if rules.ExampleArchetypeID() == "" {
					b.Fatalf("ExampleArchetypeID() for %q is empty", p)
				}
			}
		})
	}
}
