package simplified

// REQ-140 — encode must not silently lose a populated attribute. A childless
// Web Template node whose RM type has no FLAT spelling (a container the
// template left empty, a leaf type the codec does not map, a malformed type
// name) used to be skipped whatever the composition held there. Now a
// populated value at such a node is a typed error naming the FLAT path and the
// RM type, and an absent one is still skipped.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// unspelledSecret is a value the refusal must never echo.
const unspelledSecret = "do-not-echo-this-value"

// unspelledWT carries one ordinary DV_TEXT leaf beside a childless node typed
// nodeType, sitting at path under the EVALUATION, with inputs when withInputs.
func unspelledWT(nodeType, path string, withInputs bool) *webtemplate.WebTemplate {
	const entry = "/content[openEHR-EHR-EVALUATION.test.v1]"
	odd := &webtemplate.Node{ID: "odd", RMType: nodeType, Max: 1, AQLPath: entry + path}
	if withInputs {
		odd.Inputs = []webtemplate.Input{{Type: "TEXT"}}
	}
	return &webtemplate.WebTemplate{Tree: &webtemplate.Node{
		ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
		Children: []*webtemplate.Node{{
			ID: "ev", RMType: "EVALUATION", NodeID: "openEHR-EHR-EVALUATION.test.v1", Max: 1,
			AQLPath: entry,
			Children: []*webtemplate.Node{
				{
					ID: "t", RMType: "DV_TEXT", NodeID: "at0002", Max: 1,
					AQLPath: entry + "/data[at0001]/items[at0002]/value",
					Inputs:  []webtemplate.Input{{Type: "TEXT"}},
				},
				odd,
			},
		}},
	}}
}

// unspelledComp holds the DV_TEXT leaf plus, when populated, the value the odd
// node addresses: a CLUSTER at at0005, an interval at at0003's value, and a
// party subject.
func unspelledComp(populated bool) *rm.Composition {
	items := []rm.Item{&rm.Element{
		ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "t"}, Value: &rm.DVText{Value: "kept"},
	}}
	ev := &rm.Evaluation{
		ArchetypeNodeID: "openEHR-EHR-EVALUATION.test.v1", Name: rm.DVText{Value: "ev"},
	}
	if populated {
		items = append(items,
			&rm.Cluster{
				ArchetypeNodeID: "at0005", Name: rm.DVText{Value: "c"},
				Items: []rm.Item{&rm.Element{
					ArchetypeNodeID: "at0006", Name: rm.DVText{Value: "e"}, Value: &rm.DVText{Value: unspelledSecret},
				}},
			},
			&rm.Element{
				ArchetypeNodeID: "at0003", Name: rm.DVText{Value: "iv"},
				Value: &rm.DVInterval[rm.DVCount]{
					Lower: rm.DVCount{Magnitude: 1}, Upper: rm.DVCount{Magnitude: 9},
					LowerIncluded: true, UpperIncluded: true,
				},
			},
		)
		name := unspelledSecret
		ev.Subject = &rm.PartyIdentified{Name: &name}
	}
	ev.Data = &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "tree"}, Items: items}
	return &rm.Composition{
		Name:      rm.DVText{Value: "t"},
		Language:  rm.CodePhrase{CodeString: "en"},
		Territory: rm.CodePhrase{CodeString: "NL"},
		Content:   []rm.ContentItem{ev},
	}
}

// unspelledNodes are the shapes the refusal covers.
var unspelledNodes = []struct {
	name       string
	nodeType   string
	path       string
	withInputs bool
}{
	{name: "childless container", nodeType: "CLUSTER", path: "/data[at0001]/items[at0005]"},
	{name: "malformed interval type", nodeType: "DV INTERVAL<DV_COUNT>", path: "/data[at0001]/items[at0003]/value"},
	{name: "unmapped leaf type with inputs", nodeType: "PARTY", path: "/subject", withInputs: true},
}

// TestUnspelledNodeRefusedWhenPopulated — REQ-140. A populated value at a node
// the codec cannot spell is ErrUnsupportedDatatype naming the node's FLAT path
// and RM type, never the value.
func TestUnspelledNodeRefusedWhenPopulated(t *testing.T) {
	for _, tc := range unspelledNodes {
		t.Run(tc.name, func(t *testing.T) {
			b, err := MarshalFlat(unspelledComp(true), unspelledWT(tc.nodeType, tc.path, tc.withInputs))
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("MarshalFlat = (%s, %v), want ErrUnsupportedDatatype", b, err)
			}
			msg := err.Error()
			if !strings.Contains(msg, `"root/ev/odd"`) {
				t.Errorf("MarshalFlat error %q does not name the FLAT path %q", msg, "root/ev/odd")
			}
			if !strings.Contains(msg, tc.nodeType) {
				t.Errorf("MarshalFlat error %q does not name the RM type %q", msg, tc.nodeType)
			}
			if strings.Contains(msg, unspelledSecret) {
				t.Errorf("MarshalFlat error %q echoes the value", msg)
			}
		})
	}
}

// TestUnspelledNodeSkippedWhenAbsent — REQ-140. The same nodes with nothing
// under them in the composition are still skipped: the rest encodes.
func TestUnspelledNodeSkippedWhenAbsent(t *testing.T) {
	for _, tc := range unspelledNodes {
		t.Run(tc.name, func(t *testing.T) {
			b, err := MarshalFlat(unspelledComp(false), unspelledWT(tc.nodeType, tc.path, tc.withInputs))
			if err != nil {
				t.Fatalf("MarshalFlat: %v", err)
			}
			flat := flatMap(t, b)
			if got := flat["root/ev/t"]; got != "kept" {
				t.Errorf("root/ev/t = %#v, want %q", got, "kept")
			}
			for k := range flat {
				if strings.HasPrefix(k, "root/ev/odd") {
					t.Errorf("MarshalFlat wrote %q for a node with nothing under it", k)
				}
			}
		})
	}
}
