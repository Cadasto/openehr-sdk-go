package template_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// BenchmarkParseFile measures a lenient parse of three corpus OPTs of
// growing size: BMI (small), Demonstration.v1 (large, many primitive
// constraints) and clinical_notes.v0 (slot-heavy).
func BenchmarkParseFile(b *testing.B) {
	for _, id := range []string{"BMI", "Demonstration.v1", "clinical_notes.v0"} {
		path := fixtures.TemplateOpt(id)
		b.Run(id, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := template.ParseFile(path); err != nil {
					b.Fatalf("ParseFile(%s): %v", id, err)
				}
			}
		})
	}
}

// BenchmarkSlotAllowsArchetypeID measures the read-only slot checks the
// validator and generator run per candidate archetype id.
func BenchmarkSlotAllowsArchetypeID(b *testing.B) {
	opt, err := template.ParseFile(fixtures.TemplateOpt("vital_signs"))
	if err != nil {
		b.Fatalf("ParseFile: %v", err)
	}
	var slot *template.Slot
	walkSlots(opt.Root(), func(s *template.Slot) {
		if slot == nil && len(s.ParsedIncludes()) > 0 {
			slot = s
		}
	})
	if slot == nil {
		b.Fatal("no slot with parsed includes in vital_signs")
	}
	const id = "openEHR-EHR-CLUSTER.device.v1"
	b.Run("AllowsArchetypeID", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if !slot.AllowsArchetypeID(id) {
				b.Fatalf("AllowsArchetypeID(%q) = false", id)
			}
		}
	})
	b.Run("AllowsRMType", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if !slot.AllowsRMType(id) {
				b.Fatalf("AllowsRMType(%q) = false", id)
			}
		}
	})
}
