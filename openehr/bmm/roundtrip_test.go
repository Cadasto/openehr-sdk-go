package bmm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

// TestRoundTrip asserts that for every pinned BMM file, the loader
// produces a model that — once re-serialised — parses back into a
// deeply-equal model. This validates the symmetric Load → MarshalJSON
// → Load contract over the full corpus, exercising every _type
// discriminator that appears in the wild.
func TestRoundTrip(t *testing.T) {
	files := []string{
		"openehr_base_1.3.0.bmm.json",
		"openehr_rm_1.2.0.bmm.json",
		"openehr_am_1.4.0.bmm.json",
		"openehr_am_2.4.0.bmm.json",
		"openehr_lang_1.1.0.bmm.json",
		"openehr_term_3.1.0.bmm.json",
	}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			s1 := loadFixture(t, f)
			b, err := json.Marshal(s1)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			s2, err := Load(bytes.NewReader(b))
			if err != nil {
				t.Fatalf("Re-load: %v\nbytes head: %s", err, head(b, 400))
			}
			if !reflect.DeepEqual(s1, s2) {
				// Narrow the diff: spot-check a few maps for the most
				// likely culprits.
				if len(s1.ClassDefinitions) != len(s2.ClassDefinitions) {
					t.Errorf("ClassDefinitions count differs: %d vs %d",
						len(s1.ClassDefinitions), len(s2.ClassDefinitions))
				}
				if len(s1.PrimitiveTypes) != len(s2.PrimitiveTypes) {
					t.Errorf("PrimitiveTypes count differs: %d vs %d",
						len(s1.PrimitiveTypes), len(s2.PrimitiveTypes))
				}
				// Find one class where shape differs.
				for name, c1 := range s1.ClassDefinitions {
					c2, ok := s2.ClassDefinitions[name]
					if !ok {
						t.Errorf("class %q missing after round-trip", name)
						break
					}
					if !reflect.DeepEqual(c1, c2) {
						b1, _ := json.MarshalIndent(c1, "", "  ")
						b2, _ := json.MarshalIndent(c2, "", "  ")
						t.Errorf("class %q diff:\nfirst:\n%s\n\nsecond:\n%s", name, b1, b2)
						break
					}
				}
				t.Fatalf("DeepEqual failed for %s", f)
			}
		})
	}
}

// TestRoundTrip_singleClass exercises round-trip stability for one
// carefully-chosen class that touches every property variant.
func TestRoundTrip_singleClass(t *testing.T) {
	// RESOURCE_ANNOTATIONS contains a deeply-nested GenericProperty
	// with multi-level generic_parameter_defs — the trickiest shape.
	s := loadFixture(t, "openehr_base_1.3.0.bmm.json")
	cls, ok := s.ClassDefinitions["RESOURCE_ANNOTATIONS"]
	if !ok {
		t.Fatal("RESOURCE_ANNOTATIONS missing")
	}
	b, err := json.Marshal(cls)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	cls2, err := decodeClass(b, "test")
	if err != nil {
		t.Fatalf("decodeClass: %v\nbytes: %s", err, b)
	}
	if !reflect.DeepEqual(cls, cls2) {
		b1, _ := json.MarshalIndent(cls, "", "  ")
		b2, _ := json.MarshalIndent(cls2, "", "  ")
		t.Fatalf("RESOURCE_ANNOTATIONS diff:\nfirst:\n%s\n\nsecond:\n%s", b1, b2)
	}
}

func head(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
