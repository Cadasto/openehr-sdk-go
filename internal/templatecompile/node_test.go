package templatecompile

// node_test.go: RootTermItem asks the nearest enclosing archetype root
// alone, where Term and TermItem walk on to the roots above it.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// nearestArchetypeRoot returns the nearest node at or above n that has an
// archetype id, or nil when none does.
func nearestArchetypeRoot(n *CompiledNode) *CompiledNode {
	for cur := n; cur != nil; cur = cur.parent {
		if cur.archetypeID != "" {
			return cur
		}
	}
	return nil
}

// rootTermTally counts what checkRootTermItem compared: every question,
// those the nearest root answers, and those only a root above it answers,
// where RootTermItem and TermItem part ways.
type rootTermTally struct {
	asked, nearest, beyond int
}

// checkRootTermItem compares RootTermItem with the item of the nearest
// archetype root's own term for every code and key termProbes gives at n,
// then does the same below each of n's attributes.
func checkRootTermItem(t *testing.T, opt string, n *CompiledNode, tally *rootTermTally) {
	t.Helper()
	root := nearestArchetypeRoot(n)
	codes, keys := termProbes(n)
	for _, code := range codes {
		for _, key := range keys {
			want, wantOK := "", false
			if root != nil {
				if term, defined := root.terms[code]; defined {
					want, wantOK = term.Items[key]
				}
			}
			got, gotOK := RootTermItem(n, code, key)
			if got != want || gotOK != wantOK {
				t.Errorf("%s %s: RootTermItem(%q, %q) = (%q, %t), want (%q, %t), the nearest archetype root's own term", opt, n.aqlPath, code, key, got, gotOK, want, wantOK)
			}
			tally.asked++
			if wantOK {
				tally.nearest++
			} else if _, beyond := TermItem(n, code, key); beyond {
				tally.beyond++
			}
		}
	}
	for _, a := range n.attributes {
		for _, c := range a.children {
			checkRootTermItem(t, opt, c, tally)
		}
	}
}

// RootTermItem answers, at every node of four corpus templates, with the
// item of the term the nearest enclosing archetype root defines, and finds
// nothing for a code only a root further up defines, which TermItem still
// finds there.
func TestRootTermItemAsksTheNearestArchetypeRootOnly(t *testing.T) {
	t.Parallel()
	var tally rootTermTally
	for _, id := range []string{"BMI", "vital_signs", "Demonstration.v1", "clinical_notes.v0"} {
		opt, err := template.ParseFile(fixtures.TemplateOpt(id))
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", id, err)
		}
		c, err := Compile(opt)
		if err != nil {
			t.Fatalf("Compile(%s): %v", id, err)
		}
		checkRootTermItem(t, id, c.root, &tally)
	}
	// A comparison that never finds an item at the nearest root, or never
	// meets a code only a root further up defines, would not show that
	// RootTermItem stops at the nearest root.
	if tally.nearest == 0 || tally.beyond == 0 {
		t.Fatalf("asked %d, %d at the nearest root, %d only beyond it: the templates no longer exercise RootTermItem", tally.asked, tally.nearest, tally.beyond)
	}
	t.Logf("asked %d, %d at the nearest root, %d only beyond it", tally.asked, tally.nearest, tally.beyond)
}
