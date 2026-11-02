package aom14_test

import (
	"encoding/xml"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ056_QuantityItemUnitsSystemRoundTrips pins that canonical XML writes
// C_QUANTITY_ITEM.units_system and units_display_name after units, in the
// order the AM 1.4 schema's xs:sequence gives, and reads both back in every
// decode mode, strict included, which refuses an element the type does not
// declare (REQ-056).
func TestREQ056_QuantityItemUnitsSystemRoundTrips(t *testing.T) {
	const (
		system  = "http://unitsofmeasure.org"
		display = "kg"
	)
	in := &aom14.CQuantityItem{
		Units:            new("kg"),
		UnitsSystem:      new(system),
		UnitsDisplayName: new(display),
	}
	b, err := canxml.Marshal(in)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	var doc struct {
		Children []struct {
			XMLName xml.Name
		} `xml:",any"`
	}
	if err := xml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("reading %s: %v", b, err)
	}
	var got []string
	for _, c := range doc.Children {
		got = append(got, c.XMLName.Local)
	}
	// magnitude is mandatory in the AOM 1.4 BMM, so it is always written.
	want := []string{"magnitude", "units", "units_system", "units_display_name"}
	if !slices.Equal(got, want) {
		t.Errorf("canxml.Marshal wrote child elements %q in %s; want %q", got, b, want)
	}
	for _, strict := range []bool{false, true} {
		var opts []canxml.DecoderOption
		if strict {
			opts = append(opts, canxml.WithStrict())
		}
		var back aom14.CQuantityItem
		if err := canxml.Unmarshal(b, &back, opts...); err != nil {
			t.Fatalf("canxml.Unmarshal(%s) (strict %v) = %v; want nil", b, strict, err)
		}
		if back.UnitsSystem == nil || *back.UnitsSystem != system {
			t.Errorf("canxml.Unmarshal(%s) (strict %v): UnitsSystem = %v; want %q", b, strict, back.UnitsSystem, system)
		}
		if back.UnitsDisplayName == nil || *back.UnitsDisplayName != display {
			t.Errorf("canxml.Unmarshal(%s) (strict %v): UnitsDisplayName = %v; want %q", b, strict, back.UnitsDisplayName, display)
		}
	}
}
