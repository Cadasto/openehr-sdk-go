package canxml_test

import (
	"bytes"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestRoundTripStableSimpleValues — encode → decode → encode produces
// byte-stable output for representative leaf types and a small
// composition shape. The encoder is the canonical form (compact, BMM
// order, xsi:type at polymorphic boundaries); the decoder consumes
// that form and re-encoding MUST produce the same bytes.
func TestRoundTripStableSimpleValues(t *testing.T) {
	cases := []struct {
		name string
		in   any
		into func() any
	}{
		{
			name: "DV_QUANTITY",
			in:   &rm.DVQuantity{Magnitude: 80.5, Units: "kg"},
			into: func() any { return new(rm.DVQuantity) },
		},
		{
			name: "DV_TEXT",
			in:   &rm.DVText{Value: "hello"},
			into: func() any { return new(rm.DVText) },
		},
		{
			name: "Composition-with-polymorphic-composer",
			in: &rm.Composition{
				ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
				Name:            &rm.DVText{Value: "x"},
				Language:        rm.CodePhrase{CodeString: "en"},
				Territory:       rm.CodePhrase{CodeString: "GB"},
				Category:        rm.DVCodedText{DVText: rm.DVText{Value: "event"}},
				Composer:        &rm.PartySelf{},
			},
			into: func() any { return new(rm.Composition) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b1, err := canxml.Marshal(tc.in)
			if err != nil {
				t.Fatalf("first Marshal: %v", err)
			}
			v2 := tc.into()
			if err := canxml.Unmarshal(b1, v2); err != nil {
				t.Fatalf("Unmarshal: %v\nbody: %s", err, b1)
			}
			b2, err := canxml.Marshal(v2)
			if err != nil {
				t.Fatalf("second Marshal: %v", err)
			}
			if !bytes.Equal(b1, b2) {
				t.Errorf("round-trip not byte-stable:\n--- b1 ---\n%s\n--- b2 ---\n%s", b1, b2)
			}
		})
	}
}
