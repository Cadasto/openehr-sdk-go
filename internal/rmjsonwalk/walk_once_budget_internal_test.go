package rmjsonwalk

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// The budget the one pass documents for the hook calls it holds back: the
// larger of budgetFloor bytes and the document's length, each call counted as
// its pointer's length plus heldPerCall bytes.
const (
	budgetFloor = 64 << 10
	heldPerCall = 24
)

// clusterDoc returns a CLUSTER nested depth deep in the items of other
// CLUSTERs. The innermost holds elements ELEMENTs that each leave out both
// of the ELEMENT's mandatory members, then strays members no RM class has,
// each named "x" and five digits, then pad spaces before it closes.
func clusterDoc(depth, elements, strays, pad int) []byte {
	const open = `{"_type":"CLUSTER","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"x"},"items":[`
	var b strings.Builder
	for range depth {
		b.WriteString(open)
	}
	b.WriteString(open)
	for i := range elements {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"_type":"ELEMENT"}`)
	}
	b.WriteByte(']')
	for i := range strays {
		fmt.Fprintf(&b, `,"x%05d":1`, i)
	}
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteByte('}')
	for range depth {
		b.WriteString("]}")
	}
	return []byte(b.String())
}

var errHookStop = errors.New("hook stops the walk")

// TestREQ108_REQ052_OnePassGivesUpOnAReportPastItsBudget pins that a hostile
// document cannot make the one pass hold a hook call for each of its
// countless unmatched or missing members when the hook would stop the walk at
// the first, as canjson's WithStrict does (REQ-052): the one pass gives up
// once the calls it holds pass their budget (REQ-108), and Walk calls the
// erroring hook exactly once, as the two passes do.
func TestREQ108_REQ052_OnePassGivesUpOnAReportPastItsBudget(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	cases := []struct {
		name string
		doc  []byte
		hook func(h *Hooks, fn func(string) error)
	}{
		{
			name: "unmatched members, 50 levels deep",
			doc:  clusterDoc(50, 0, 20000, 0),
			hook: func(h *Hooks, fn func(string) error) { h.Unmatched = fn },
		},
		{
			name: "elements missing their mandatory members",
			doc:  clusterDoc(0, 20000, 0, 0),
			hook: func(h *Hooks, fn func(string) error) { h.Missing = fn },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var called int
			var h Hooks
			tc.hook(&h, func(string) error { called++; return errHookStop })
			root := Root{Type: "CLUSTER"}
			if _, once := m.walkOnce(tc.doc, root, h); once {
				t.Errorf("walkOnce on a %d-byte document whose every member is reported completed, want it to give up past its budget", len(tc.doc))
			}
			if called != 0 {
				t.Errorf("walkOnce called the hook %d times, want none", called)
			}
			called = 0
			if err := m.Walk(tc.doc, root, h); !errors.Is(err, errHookStop) {
				t.Errorf("Walk error = %v, want the hook's %v", err, errHookStop)
			}
			if called != 1 {
				t.Errorf("Walk called the erroring hook %d times, want 1", called)
			}
		})
	}
}

// TestREQ108_REQ052_OnePassBudget pins the budget the one pass holds hook
// calls to (REQ-108): the larger of 64 KiB and the document's length, each
// call counted as its pointer's length plus 24 bytes. A report just under it
// completes in one pass, one call more gives the pass up, and either way Walk
// reports what walkIndexed reports (REQ-052).
func TestREQ108_REQ052_OnePassBudget(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	// Each stray member of an unnested CLUSTER is reported as "/x" and five
	// digits.
	underFloor := budgetFloor / (len("/x00000") + heldPerCall)
	cases := []struct {
		name        string
		strays, pad int
		once        bool
	}{
		{name: "just under 64 KiB", strays: underFloor, once: true},
		{name: "just over 64 KiB", strays: underFloor + 1, once: false},
		{name: "over 64 KiB, under the document's length", strays: 2 * underFloor, pad: 2 * budgetFloor, once: true},
		{name: "over the document's length", strays: 4 * underFloor, pad: 2 * budgetFloor, once: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := clusterDoc(0, 0, tc.strays, tc.pad)
			root := Root{Type: "CLUSTER"}
			var want []string
			if err := m.walkIndexed(doc, root, logHooks(&want)); err != nil {
				t.Fatalf("walkIndexed: %v", err)
			}
			var held int
			for _, p := range want {
				held += len(strings.TrimPrefix(p, "unmatched ")) + heldPerCall
			}
			budget := max(budgetFloor, len(doc))
			if (held <= budget) != tc.once {
				t.Fatalf("the case holds %d bytes against a budget of %d: it does not test what its name says", held, budget)
			}
			if _, once := m.walkOnce(doc, root, logHooks(new([]string))); once != tc.once {
				t.Errorf("walkOnce completed = %t holding %d bytes against a budget of %d, want %t", once, held, budget, tc.once)
			}
			var got []string
			if err := m.Walk(doc, root, logHooks(&got)); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			if !slices.Equal(got, want) {
				t.Errorf("Walk reported %d calls, want the %d walkIndexed reports, in the same order", len(got), len(want))
			}
		})
	}
}
