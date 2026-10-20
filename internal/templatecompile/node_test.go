package templatecompile

// node_test.go: RootTerm asks the nearest enclosing archetype root alone,
// where Term walks on to the roots above it.

import (
	"maps"
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

// rootTermTally counts what checkRootTerm compared: every question, those
// the nearest root answers, and those only a root above it answers, where
// RootTerm and Term part ways.
type rootTermTally struct {
	asked, nearest, beyond int
}

// checkRootTerm compares RootTerm with the nearest archetype root's own
// terms for every code termProbes gives at n, then does the same below
// each of n's attributes.
func checkRootTerm(t *testing.T, opt string, n *CompiledNode, tally *rootTermTally) {
	t.Helper()
	root := nearestArchetypeRoot(n)
	codes, _ := termProbes(n)
	for _, code := range codes {
		var want template.ArchetypeTerm
		wantOK := false
		if root != nil {
			want, wantOK = root.terms[code]
		}
		got, gotOK := n.RootTerm(code, "")
		if gotOK != wantOK || got.Code != want.Code || !maps.Equal(got.Items, want.Items) {
			t.Errorf("%s %s: RootTerm(%q) = (%+v, %t), want (%+v, %t), the nearest archetype root's own term", opt, n.aqlPath, code, got, gotOK, want, wantOK)
		}
		tally.asked++
		if wantOK {
			tally.nearest++
		} else if _, beyond := n.Term(code, ""); beyond {
			tally.beyond++
		}
	}
	for _, a := range n.attributes {
		for _, c := range a.children {
			checkRootTerm(t, opt, c, tally)
		}
	}
}

// RootTerm answers, at every node of four corpus templates, with the term
// the nearest enclosing archetype root defines, and finds nothing for a
// code only a root further up defines, which Term still finds there.
func TestRootTermAsksTheNearestArchetypeRootOnly(t *testing.T) {
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
		checkRootTerm(t, id, c.root, &tally)
	}
	// A comparison that never finds a term at the nearest root, or never
	// meets a code only a root further up defines, would not show that
	// RootTerm stops at the nearest root.
	if tally.nearest == 0 || tally.beyond == 0 {
		t.Fatalf("asked %d, %d at the nearest root, %d only beyond it: the templates no longer exercise RootTerm", tally.asked, tally.nearest, tally.beyond)
	}
	t.Logf("asked %d, %d at the nearest root, %d only beyond it", tally.asked, tally.nearest, tally.beyond)
}

// RootTerm returns a copy of the term's items, as Term does.
func TestRootTermCopiesItems(t *testing.T) {
	t.Parallel()
	n := &CompiledNode{archetypeID: "openEHR-EHR-CLUSTER.x.v1", terms: map[string]template.ArchetypeTerm{
		"at0001": {Code: "at0001", Items: map[string]string{"text": "Term"}},
	}}
	got, ok := n.RootTerm("at0001", "")
	if !ok {
		t.Fatal(`RootTerm("at0001") found nothing`)
	}
	got.Items["text"] = "changed"
	if again, _ := n.RootTerm("at0001", ""); again.Items["text"] != "Term" {
		t.Errorf("RootTerm items after a caller changed them = %q, want %q", again.Items["text"], "Term")
	}
}
