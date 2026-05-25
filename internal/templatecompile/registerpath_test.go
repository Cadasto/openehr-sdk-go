package templatecompile

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// registerPath rejects duplicate AQL paths registered under different
// wire attributes (OPT bug), while admitting legal C_SINGLE_ATTRIBUTE
// alternatives under the same attribute.
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
