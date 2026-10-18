package templatecompile

// termitem_test.go: TermItem reads one item of a term without copying the
// term's items, and answers exactly as reading that item from the term
// CompiledNode.Term returns does. The test runs inside the package so it
// can list the codes every enclosing archetype root defines.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// absentCode and absentKey are a code no corpus root defines and an item
// key no corpus term carries.
const (
	absentCode = "at-no-such-code"
	absentKey  = "no-such-item"
)

// termProbes returns the codes and item keys to ask of n: n's own node id,
// every code an enclosing archetype root of n defines, an absent code, every
// item key those roots' terms carry, and an absent key.
func termProbes(n *CompiledNode) (codes, keys []string) {
	codes = []string{n.nodeID, absentCode}
	keys = []string{absentKey}
	for cur := n; cur != nil; cur = cur.parent {
		for code, term := range cur.terms {
			codes = append(codes, code)
			for key := range term.Items {
				keys = append(keys, key)
			}
		}
	}
	slices.Sort(codes)
	slices.Sort(keys)
	return slices.Compact(codes), slices.Compact(keys)
}

// termItemTally counts what checkTermItem compared: every question, those
// whose item was found, and those whose term only a root above the nearest
// one defines.
type termItemTally struct {
	asked, found, beyondNearest int
}

// checkTermItem compares TermItem with Term for every code and key
// termProbes gives at n, then does the same below each of n's attributes.
func checkTermItem(t *testing.T, opt string, n *CompiledNode, tally *termItemTally) {
	t.Helper()
	codes, keys := termProbes(n)
	for _, code := range codes {
		term, defined := n.Term(code, "")
		for _, key := range keys {
			want, wantOK := "", false
			if defined {
				want, wantOK = term.Items[key]
			}
			got, gotOK := TermItem(n, code, key)
			if got != want || gotOK != wantOK {
				t.Errorf("%s %s: TermItem(%q, %q) = (%q, %t), want (%q, %t) as Term gives", opt, n.aqlPath, code, key, got, gotOK, want, wantOK)
			}
			tally.asked++
			if wantOK {
				tally.found++
				if _, nearest := nearestRoot(n).terms[code]; !nearest {
					tally.beyondNearest++
				}
			}
		}
	}
	for _, a := range n.attributes {
		for _, c := range a.children {
			checkTermItem(t, opt, c, tally)
		}
	}
}

// nearestRoot returns the nearest node at or above n that carries terms,
// or nil when none does.
func nearestRoot(n *CompiledNode) *CompiledNode {
	for cur := n; cur != nil; cur = cur.parent {
		if cur.terms != nil {
			return cur
		}
	}
	return nil
}

// TermItem answers as Term does, item by item, at every node of four corpus
// templates: for the node's own id, for every code an enclosing archetype
// root defines (the nearest one or one further up), and for a code and an
// item key that are absent.
func TestTermItemMatchesTerm(t *testing.T) {
	t.Parallel()
	var tally termItemTally
	for _, id := range []string{"BMI", "vital_signs", "Demonstration.v1", "clinical_notes.v0"} {
		opt, err := template.ParseFile(fixtures.TemplateOpt(id))
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", id, err)
		}
		c, err := Compile(opt)
		if err != nil {
			t.Fatalf("Compile(%s): %v", id, err)
		}
		checkTermItem(t, id, c.root, &tally)
	}
	// A comparison that never finds an item, never misses one, or never
	// needs a root above the nearest would not show TermItem walks the
	// parent chain as Term does.
	if tally.found == 0 || tally.found == tally.asked || tally.beyondNearest == 0 {
		t.Fatalf("asked %d, found %d, %d beyond the nearest root: the templates no longer exercise TermItem", tally.asked, tally.found, tally.beyondNearest)
	}
	t.Logf("asked %d, found %d, %d beyond the nearest root", tally.asked, tally.found, tally.beyondNearest)
}
