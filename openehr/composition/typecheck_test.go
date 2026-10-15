package composition

import (
	"errors"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestCheckRMTypeTypedInterval pins the bound-aware DV_INTERVAL
// matching of the REQ-101 builder type check (ADR 0013 review round):
// a compiled node declares the ITS-JSON parameterised name, the
// registry reverses the bare name, and rmnames.TypedIntervalName
// bridges the two — bound-checked, so a DV_INTERVAL<DV_COUNT> value
// cannot pass a DV_INTERVAL<DV_QUANTITY> node. Typed-nil pointers fail
// as unrecognised.
func TestCheckRMTypeTypedInterval(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("Demonstration.v1"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := tcimpl.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	nodes := c.AllByRMType("DV_INTERVAL<DV_QUANTITY>")
	if len(nodes) == 0 {
		t.Fatal("Demonstration.v1 has no DV_INTERVAL<DV_QUANTITY> node — fixture drift")
	}
	node := nodes[0]

	if err := checkRMType(node, rm.DVInterval[rm.DVQuantity]{}); err != nil {
		t.Errorf("value form: checkRMType = %v, want nil", err)
	}
	if err := checkRMType(node, &rm.DVInterval[rm.DVQuantity]{}); err != nil {
		t.Errorf("pointer form: checkRMType = %v, want nil", err)
	}
	if err := checkRMType(node, rm.DVInterval[rm.DVCount]{}); !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("bound mismatch (DV_COUNT at DV_QUANTITY node): err = %v, want ErrTypeMismatch", err)
	}
	if err := checkRMType(node, (*rm.DVInterval[rm.DVQuantity])(nil)); !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("typed-nil: err = %v, want ErrTypeMismatch (unrecognised)", err)
	}
}

// compiledNodeAt compiles the corpus template name and returns its node at
// path, after checking that the node still has the RM type rmType.
func compiledNodeAt(t *testing.T, name, path, rmType string) *tcimpl.CompiledNode {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", name, err)
	}
	c, err := tcimpl.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", name, err)
	}
	node, err := c.NodeAt(path)
	if err != nil {
		t.Fatalf("%s NodeAt(%s): %v", name, path, err)
	}
	if got := node.RMTypeName(); got != rmType {
		t.Fatalf("%s node %s has RM type %q, want %q (fixture drift)", name, path, got, rmType)
	}
	return node
}

// TestREQ101_CheckRMTypeConformance pins the conformance rule of the
// builder type check on corpus nodes: a value whose RM type is the node's
// type, or descends from it in the pinned class hierarchy, passes, so a
// DV_CODED_TEXT passes a DV_TEXT node and a POINT_EVENT or an
// INTERVAL_EVENT passes the abstract EVENT node. Every other value is
// refused with ErrTypeMismatch: one whose type does not conform (a DV_TEXT
// at a DV_CODED_TEXT node, since the inheritance runs the other way), a
// typed-nil, a nil and a value that is not an RM type.
func TestREQ101_CheckRMTypeConformance(t *testing.T) {
	t.Parallel()
	text := compiledNodeAt(t, "Test_dv_text_open_constraint.v0", "/context/other_context/items[at0002]/value", "DV_TEXT")
	coded := compiledNodeAt(t, "Test_dv_text_open_constraint.v0", "/category", "DV_CODED_TEXT")
	quantity := compiledNodeAt(t, "vital_signs",
		"/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value", "DV_QUANTITY")
	event := compiledNodeAt(t, "vital_signs",
		"/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]", "EVENT")

	codedText := rm.DVCodedText{
		Value:        "Lorem",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "12345"},
	}
	cases := []struct {
		name   string
		node   *tcimpl.CompiledNode
		v      any
		accept bool
	}{
		{name: "DV_TEXT at DV_TEXT", node: text, v: &rm.DVText{Value: "x"}, accept: true},
		{name: "DV_CODED_TEXT at DV_TEXT", node: text, v: &codedText, accept: true},
		{name: "DV_CODED_TEXT value form at DV_TEXT", node: text, v: codedText, accept: true},
		{name: "POINT_EVENT at EVENT", node: event, v: &rm.PointEvent[rm.ItemStructure]{}, accept: true},
		{name: "INTERVAL_EVENT at EVENT", node: event, v: &rm.IntervalEvent[rm.ItemStructure]{}, accept: true},
		{name: "DV_TEXT at DV_QUANTITY", node: quantity, v: &rm.DVText{Value: "x"}},
		{name: "DV_TEXT at DV_CODED_TEXT", node: coded, v: &rm.DVText{Value: "x"}},
		{name: "DV_QUANTITY at EVENT", node: event, v: &rm.DVQuantity{Magnitude: 1, Units: "mm"}},
		{name: "HISTORY at EVENT", node: event, v: &rm.History[rm.ItemStructure]{}},
		{name: "typed-nil DV_CODED_TEXT at DV_TEXT", node: text, v: (*rm.DVCodedText)(nil)},
		{name: "nil at DV_TEXT", node: text, v: nil},
		{name: "non-RM value at DV_TEXT", node: text, v: "Lorem"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkRMType(tc.node, tc.v)
			switch {
			case tc.accept && err != nil:
				t.Errorf("checkRMType(%s node, %T) = %v, want nil", tc.node.RMTypeName(), tc.v, err)
			case !tc.accept && !errors.Is(err, ErrTypeMismatch):
				t.Errorf("checkRMType(%s node, %T) = %v, want ErrTypeMismatch", tc.node.RMTypeName(), tc.v, err)
			}
		})
	}
}
