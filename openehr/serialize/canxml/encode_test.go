package canxml_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestMarshalDVQuantityNoXSITypeAtRoot asserts that a top-level
// DV_QUANTITY emitted in isolation does NOT carry `xsi:type` —
// the caller already knows the concrete type. Polymorphic
// discriminators are reserved for descendants under polymorphic
// parent slots.
func TestMarshalDVQuantityNoXSITypeAtRoot(t *testing.T) {
	q := &rm.DVQuantity{Magnitude: 80.5, Units: "kg"}
	got, err := canxml.Marshal(q)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(got), "xsi:type") {
		t.Errorf("root must not carry xsi:type in isolation: %s", got)
	}
	if !strings.Contains(string(got), `<dv_quantity`) {
		t.Errorf("root element must be <dv_quantity ...>: %s", got)
	}
	if !strings.Contains(string(got), `xmlns="`+canxml.NSDefault+`"`) {
		t.Errorf("root must declare default namespace: %s", got)
	}
}

// TestMarshalDVQuantityChildOrder asserts that mandatory child
// elements are emitted in BMM property declaration order (the order
// the generator emits struct fields).
func TestMarshalDVQuantityChildOrder(t *testing.T) {
	q := &rm.DVQuantity{Magnitude: 80.5, Units: "kg"}
	got, err := canxml.Marshal(q)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(got)
	mi := strings.Index(s, "<magnitude>")
	ui := strings.Index(s, "<units>")
	if mi < 0 || ui < 0 {
		t.Fatalf("missing mandatory child elements in: %s", s)
	}
	if mi >= ui {
		t.Errorf("BMM order violated: magnitude must precede units in:\n%s", s)
	}
}

// TestMarshalDVQuantityOmitsNilOptional asserts that nil-pointer
// optional fields encode as ABSENT (no element), not as empty tags.
func TestMarshalDVQuantityOmitsNilOptional(t *testing.T) {
	q := &rm.DVQuantity{Magnitude: 80.5, Units: "kg"}
	got, err := canxml.Marshal(q)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, banned := range []string{
		"<accuracy", "<magnitude_status", "<normal_range",
		"<normal_status", "<other_reference_ranges", "<precision",
		"<units_display_name", "<units_system",
	} {
		if strings.Contains(string(got), banned) {
			t.Errorf("nil-pointer optional %q must be absent: %s", banned, got)
		}
	}
}

// TestMarshalCompositionPolymorphicXSIType asserts that polymorphic
// children carry `xsi:type="<BMM_CLASS>"` as the first attribute on
// every concrete value boundary inside the document.
func TestMarshalCompositionPolymorphicXSIType(t *testing.T) {
	c := &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            rm.DVText{Value: "body_weight"},
		Language:        rm.CodePhrase{CodeString: "en"},
		Territory:       rm.CodePhrase{CodeString: "GB"},
		Category:        rm.DVCodedText{DVText: rm.DVText{Value: "event"}},
		Composer:        &rm.PartySelf{},
		Content: []rm.ContentItem{
			&rm.Observation{
				ArchetypeNodeID: "openEHR-EHR-OBSERVATION.body_weight.v2",
				Name:            rm.DVText{Value: "Body weight"},
				Language:        rm.CodePhrase{CodeString: "en"},
				Encoding:        rm.CodePhrase{CodeString: "UTF-8"},
				Subject:         &rm.PartySelf{},
			},
		},
	}
	got, err := canxml.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(got)
	// Composer is a polymorphic slot (Party interface) → xsi:type required.
	if !strings.Contains(s, `<composer xsi:type="PARTY_SELF"`) {
		t.Errorf("composer must carry xsi:type=\"PARTY_SELF\": %s", s)
	}
	// Content slot is []ContentItem → polymorphic per item.
	if !strings.Contains(s, `<content xsi:type="OBSERVATION"`) {
		t.Errorf("content[0] must carry xsi:type=\"OBSERVATION\": %s", s)
	}
	// Root has no xsi:type.
	if strings.HasPrefix(s, `<composition xsi:type=`) {
		t.Errorf("root composition must NOT carry xsi:type: %s", s)
	}
}

// TestMarshalEmptyContainerOmitted asserts that an empty container
// with cardinality.lower == 0 is emitted as ABSENT (zero elements),
// not as a wrapper.
func TestMarshalEmptyContainerOmitted(t *testing.T) {
	c := &rm.Composition{
		ArchetypeNodeID: "x",
		Name:            rm.DVText{Value: "x"},
		Language:        rm.CodePhrase{CodeString: "en"},
		Territory:       rm.CodePhrase{CodeString: "GB"},
		Category:        rm.DVCodedText{DVText: rm.DVText{Value: "event"}},
		Composer:        &rm.PartySelf{},
		Content:         nil,
	}
	got, err := canxml.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(got), "<content") {
		t.Errorf("empty content must be absent: %s", got)
	}
}

// TestBMMNameOnConcreteTypes asserts that every concrete RM type
// implements canxml.BMMNamer with the expected discriminator.
func TestBMMNameOnConcreteTypes(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{&rm.DVQuantity{}, "DV_QUANTITY"},
		{&rm.DVText{}, "DV_TEXT"},
		{&rm.DVCodedText{}, "DV_CODED_TEXT"},
		{&rm.Composition{}, "COMPOSITION"},
		{&rm.Observation{}, "OBSERVATION"},
		{&rm.PartySelf{}, "PARTY_SELF"},
	}
	for _, tc := range cases {
		got, ok := canxml.BMMNameOf(tc.v)
		if !ok {
			t.Errorf("%T does not implement BMMNamer", tc.v)
			continue
		}
		if got != tc.want {
			t.Errorf("%T.BMMName() = %q; want %q", tc.v, got, tc.want)
		}
	}
}
