package composition_test

import (
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestREQ101_REQ111_BuildReachesANameFixedByTheConceptCode pins that a value
// set under a node whose name the template fixes by one code reaches the
// skeleton's node (REQ-101; REQ-111 § Node names and name predicates, issue
// 455). A copy of IDCR - Vital Signs Encounter.v1 fixes the pulse rate
// ELEMENT at0004's name to the pulse archetype's concept code at0000, whose
// term text, Pulse/Heart beat, is the node's name and its compiled path's
// name predicate; the skeleton names the ELEMENT so, and Build writes the
// quantity there.
func TestREQ101_REQ111_BuildReachesANameFixedByTheConceptCode(t *testing.T) {
	raw, err := os.ReadFile(fixtures.TemplateOpt("IDCR - Vital Signs Encounter.v1"))
	if err != nil {
		t.Fatalf("read IDCR - Vital Signs Encounter.v1: %v", err)
	}
	const listed = "<code_list>at1027</code_list>"
	if n := strings.Count(string(raw), listed); n != 1 {
		t.Fatalf("IDCR - Vital Signs Encounter.v1 lists at1027 %d times, want 1 — fixture changed?", n)
	}
	c := compileInline(t, strings.Replace(string(raw), listed, "<code_list>at0000</code_list>", 1))
	const element = "/content[openEHR-EHR-SECTION.vital_signs.v1]/items[openEHR-EHR-OBSERVATION.pulse.v1]/data/events[at0003]/data/items[at0004,'Pulse/Heart beat']"
	if _, err := c.NodeAt(element); err != nil {
		t.Fatalf("NodeAt(%s) = %v, want the pulse rate ELEMENT named by the concept code", element, err)
	}
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.SetQuantity(element+"/value", 72, "/min"); err != nil {
		t.Fatalf("SetQuantity: %v", err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build = %v, want the quantity written under the ELEMENT named Pulse/Heart beat", err)
	}
	got, err := rmpath.ItemAtPath(comp, element+"/value")
	if err != nil {
		t.Fatalf("ItemAtPath(%s/value) = %v, want the quantity Build wrote", element, err)
	}
	if q, ok := got.(*rm.DVQuantity); !ok || float64(q.Magnitude) != 72 || q.Units != "/min" {
		t.Errorf("ItemAtPath(%s/value) = %#v, want 72 /min", element, got)
	}
}
