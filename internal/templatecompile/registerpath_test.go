package templatecompile

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// REQ-100 — registerPath rejects duplicate AQL paths registered under
// different wire attributes (OPT bug), while admitting legal
// C_SINGLE_ATTRIBUTE alternatives under the same attribute.
func TestRegisterPath_DuplicateFromDifferentAttribute(t *testing.T) {
	c := &Compiled{byPath: make(map[string]*CompiledNode)}
	w := walker{compiled: c, pathAttr: make(map[string]*template.Attribute)}

	attrA := &template.Attribute{}
	attrB := &template.Attribute{}

	node1 := &CompiledNode{aqlPath: "/name", rmTypeName: "DV_TEXT"}
	w.currentAttr = attrA
	if err := w.registerPath(node1); err != nil {
		t.Fatalf("first registerPath: %v", err)
	}

	node2 := &CompiledNode{aqlPath: "/name", rmTypeName: "DV_CODED_TEXT"}
	w.currentAttr = attrB
	err := w.registerPath(node2)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("registerPath duplicate from different attr = %v, want ErrInvalidInput", err)
	}
}

// REQ-116 — two nodes may legally share an AQL path: AOM 1.4
// C_SINGLE_ATTRIBUTE alternatives, and sibling nodes carrying the same
// node_id. Their descendants then share paths too, but sit under different
// attribute objects (one per parent), so the currentAttr test cannot admit
// them — dupDepth does. Registration is post-order, so a descendant of the
// second node registers while the *first* node's subtree is already in
// byPath.
func TestRegisterPath_AdmitsDescendantsOfSharedPath(t *testing.T) {
	c := &Compiled{byPath: make(map[string]*CompiledNode)}
	w := walker{
		compiled: c,
		pathAttr: make(map[string]*template.Attribute),
		seenPath: make(map[string]bool),
	}

	// First alternative's descendant registers normally.
	w.currentAttr = &template.Attribute{}
	first := &CompiledNode{aqlPath: "/value/value", rmTypeName: "STRING"}
	if err := w.registerPath(first); err != nil {
		t.Fatalf("first descendant: %v", err)
	}

	// Second alternative's descendant: same path, different attribute
	// object. Outside a shared-path subtree this is the OPT bug the guard
	// exists for — and the node's own registration always runs at
	// dupDepth == 0, so it stays guarded.
	w.currentAttr = &template.Attribute{}
	clash := &CompiledNode{aqlPath: "/value/value", rmTypeName: "STRING"}
	if err := w.registerPath(clash); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-attribute duplicate outside a shared-path subtree = %v, want ErrInvalidInput", err)
	}

	// Inside one, it is expected and admitted; byPath keeps the first.
	w.dupDepth++
	if err := w.registerPath(clash); err != nil {
		t.Fatalf("descendant inside shared-path subtree: %v", err)
	}
	w.dupDepth--

	if got := c.byPath["/value/value"]; got != first {
		t.Errorf("byPath[/value/value] = %p, want the first registrant %p", got, first)
	}
}
