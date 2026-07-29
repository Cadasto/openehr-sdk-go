package templatecompile

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// Two nodes may legally share an AQL path: AOM 1.4 C_SINGLE_ATTRIBUTE
// alternatives, and sibling nodes carrying the same node_id. Their
// descendants then share paths too, but sit under different attribute
// objects (one per parent), so the currentAttr test cannot admit them —
// dupDepth does. Registration is post-order, so a descendant of the second
// node registers while the *first* node's subtree is already in byPath.
func TestRegisterPath_AdmitsDescendantsOfSharedPath(t *testing.T) {
	c := &Compiled{byPath: make(map[string]*CompiledNode)}
	w := walker{
		compiled: c,
		pathAttr: make(map[string]*template.Attribute),
		seenPath: make(map[string]bool),
	}

	altOne := &template.Attribute{}
	altTwo := &template.Attribute{}

	// First alternative's descendant registers normally.
	w.currentAttr = altOne
	first := &CompiledNode{aqlPath: "/value/value", rmTypeName: "STRING"}
	if err := w.registerPath(first); err != nil {
		t.Fatalf("first descendant: %v", err)
	}

	// Second alternative's descendant: same path, different attribute
	// object. Outside a shared-path subtree this is the OPT bug the guard
	// exists for.
	w.currentAttr = altTwo
	clash := &CompiledNode{aqlPath: "/value/value", rmTypeName: "STRING"}
	if err := w.registerPath(clash); err == nil {
		t.Fatal("want ErrInvalidInput for a cross-attribute duplicate outside a shared-path subtree")
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

// dupDepth must cover only the descent, never the node's own registration,
// so a genuine cross-attribute collision at the node itself still fails.
func TestRegisterPath_SelfRegistrationStillGuarded(t *testing.T) {
	c := &Compiled{byPath: make(map[string]*CompiledNode)}
	w := walker{
		compiled: c,
		pathAttr: make(map[string]*template.Attribute),
		seenPath: make(map[string]bool),
	}

	w.currentAttr = &template.Attribute{}
	if err := w.registerPath(&CompiledNode{aqlPath: "/name", rmTypeName: "DV_TEXT"}); err != nil {
		t.Fatalf("first: %v", err)
	}

	// dupDepth back at 0 (the caller drops it before registering the node
	// itself) — an unrelated subtree colliding here is still an OPT bug.
	w.currentAttr = &template.Attribute{}
	if err := w.registerPath(&CompiledNode{aqlPath: "/name", rmTypeName: "DV_CODED_TEXT"}); err == nil {
		t.Fatal("want ErrInvalidInput: cross-attribute collision at the node itself")
	}
}
