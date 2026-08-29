package aqlprobes

import (
	"slices"
	"strings"
	"testing"
)

// TestConformanceSuitesVarsPrefixHeader pins the reconstruction table's
// internal contract: Vars are BY DEFINITION the leading Header columns in
// column order (the reader substitutes them positionally), and the remaining
// columns carry execution semantics this corpus never asserts. An entry whose
// Vars drift from its Header would substitute the wrong column into the
// template silently — rows would still reconstruct, just from the wrong cell —
// so the drift has to fail here, by name, not surface as a puzzling
// admissibility failure.
func TestConformanceSuitesVarsPrefixHeader(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range conformanceSuites {
		t.Run(s.key(), func(t *testing.T) {
			if seen[s.key()] {
				t.Fatalf("duplicate reconstruction-table entry for %s", s.key())
			}
			seen[s.key()] = true
			if len(s.Vars) == 0 {
				t.Fatal("entry substitutes no column at all — nothing to reconstruct")
			}
			if len(s.Vars) >= len(s.Header) {
				t.Fatalf("Vars (%d) leave no Header column (%d) for the suite's result artefact",
					len(s.Vars), len(s.Header))
			}
			if !slices.Equal(s.Vars, s.Header[:len(s.Vars)]) {
				t.Fatalf("Vars %v are not the leading Header columns %v",
					s.Vars, s.Header[:len(s.Vars)])
			}
			// Every substituted column must appear in the template, or the
			// entry reads a cell it then throws away.
			for _, v := range s.Vars {
				if !strings.Contains(s.Template, v) {
					t.Errorf("Template %q never uses the substituted column %s", s.Template, v)
				}
			}
		})
	}
}
