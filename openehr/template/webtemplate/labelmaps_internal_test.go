package webtemplate

import (
	"os"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// A built tree is read-only, so the value children of an ELEMENT node hold
// the ELEMENT's own LocalizedNames and LocalizedDescriptions maps rather than
// copies of them (REQ-106). Demonstration.v1 has ELEMENTs of both kinds that
// give value children: ones admitting several value types, whose children
// carry the ELEMENT's nodeId, and ones constraining no value, whose children
// carry none.
func TestValueChildSharesElementLabelMaps(t *testing.T) {
	const id = "Demonstration.v1"
	raw, err := os.ReadFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	opt, err := fixtures.ReadOperationalTemplate(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", id, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile %s: %v", id, err)
	}
	wt, err := Build(c)
	if err != nil {
		t.Fatalf("Build(%s): %v", id, err)
	}

	var constrained, unconstrained int
	var walk func(n *Node)
	walk = func(n *Node) {
		for _, ch := range n.Children {
			if n.RMType == "ELEMENT" && ch.AQLPath == n.AQLPath+"/value" {
				if !sameMap(ch.LocalizedNames, n.LocalizedNames) {
					t.Errorf("value child %q of %s holds a copy of its ELEMENT's localizedNames, want the same map", ch.ID, n.AQLPath)
				}
				if !sameMap(ch.LocalizedDescriptions, n.LocalizedDescriptions) {
					t.Errorf("value child %q of %s holds a copy of its ELEMENT's localizedDescriptions, want the same map", ch.ID, n.AQLPath)
				}
				if n.LocalizedNames != nil && n.LocalizedDescriptions != nil {
					if ch.NodeID != "" {
						constrained++
					} else {
						unconstrained++
					}
				}
			}
			walk(ch)
		}
	}
	walk(wt.Tree)
	// Nil maps compare as the same map whether copied or not, so the check
	// needs value children whose ELEMENT carries both maps, of each kind.
	if constrained == 0 || unconstrained == 0 {
		t.Fatalf("%s gave %d constrained and %d unconstrained value children under an ELEMENT with both label maps, want at least one of each", id, constrained, unconstrained)
	}
}

// sameMap reports whether a and b are the same map, not merely equal ones.
func sameMap(a, b map[string]string) bool {
	return reflect.ValueOf(a).UnsafePointer() == reflect.ValueOf(b).UnsafePointer()
}
