package template_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func TestParseFile_VitalSigns_SlotAssertionsParsed(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var deviceSlot *template.Slot
	walkSlots(opt.Root(), func(s *template.Slot) {
		for _, inc := range s.ParsedIncludes() {
			if inc.MatchesArchetypeID("openEHR-EHR-CLUSTER.device.v1") {
				deviceSlot = s
			}
		}
	})
	if deviceSlot == nil {
		t.Fatal("expected a CLUSTER.device slot with parsed includes in vital_signs.opt")
	}
	if !deviceSlot.AllowsArchetypeID("openEHR-EHR-CLUSTER.device.v1") {
		t.Error("CLUSTER.device slot should allow matching archetype id")
	}
	if deviceSlot.AllowsArchetypeID("openEHR-EHR-OBSERVATION.heart_rate.v1") {
		t.Error("CLUSTER.device slot should reject unrelated archetype id")
	}
}

func TestSlot_AllowsRMTypePrefix(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var checked bool
	walkSlots(opt.Root(), func(s *template.Slot) {
		if len(s.ParsedIncludes()) > 0 {
			return // prefix fallback only when no parsed includes
		}
		checked = true
		id := "openEHR-EHR-" + s.RMTypeName() + ".example.v1"
		if !s.AllowsRMType(id) {
			t.Errorf("slot %s: AllowsRMType prefix fallback failed for %q", s.NodeID(), id)
		}
	})
	if !checked {
		t.Skip("fixture has no slots without parsed includes")
	}
}

// Demonstration.v1.opt is the only fixture carrying <excludes>: an
// ELEMENT slot pairs a closed includes list with the editor's
// auto-generated catch-all `.*` exclude. The catch-all must be parsed
// but must not reject the slot's own includes.
func TestParseFile_Demonstration_ExcludesParsed(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("Demonstration.v1"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	const included = "openEHR-EHR-ELEMENT.ctg_codes.v1"
	var slot *template.Slot
	walkSlots(opt.Root(), func(s *template.Slot) {
		if slot != nil || len(s.ParsedExcludes()) == 0 {
			return
		}
		for _, inc := range s.ParsedIncludes() {
			if inc.MatchesArchetypeID(included) {
				slot = s
				return
			}
		}
	})
	if slot == nil {
		t.Fatal("expected a slot with parsed includes and excludes in Demonstration.v1.opt")
	}
	// The included id fits despite the catch-all exclude.
	if !slot.AllowsArchetypeID(included) {
		t.Errorf("AllowsArchetypeID(%q) = false, want true (catch-all exclude must not reject includes)", included)
	}
	// A non-included id is still rejected.
	if slot.AllowsArchetypeID("openEHR-EHR-ELEMENT.something_else.v1") {
		t.Error("non-included archetype id should be rejected")
	}
}

func walkSlots(n template.Node, fn func(*template.Slot)) {
	switch v := n.(type) {
	case template.ObjectNode:
		for _, a := range v.Attributes() {
			for _, c := range a.Children() {
				if s, ok := c.(*template.Slot); ok {
					fn(s)
				}
				walkSlots(c, fn)
			}
		}
	}
}
