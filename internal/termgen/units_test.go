package termgen

import (
	"slices"
	"strings"
	"testing"
)

// propertyUnitsFixture is a stand-in for PropertyUnitData.xml in the pin's
// own shape: a namespaced PropertyUnits root, the properties first, then the
// units, each unit naming its property by the property's id attribute, not
// by its openEHR code. Length lists a unit twice (as the pin lists um), a
// unit with no UCUM attribute, and a UCUM code inside white space (as the pin
// spells statfarad). Frequency has two primary units and Work none at all.
// Shared by the parser, renderer and Run tests.
const propertyUnitsFixture = `<?xml version="1.0" standalone="yes"?>

<PropertyUnits xmlns="http://tempuri.org/PropertyUnits.xsd">
  <Property id="0" Text="Length" openEHR="122" />
  <Property id="29" Text="Pressure" openEHR="125" />
  <Property id="60" Text="Frequency" openEHR="382" />
  <Property id="61" Text="Work" openEHR="130" />
  <Unit property_id="0" Text="cm" name="centimetre" conversion="0.01" primary="false" UCUM="cm"/>
  <Unit property_id="0" Text="µm" conversion="1e-06" primary="false" UCUM="um"/>
  <Unit property_id="29" Text="mm[Hg]" conversion="133.3" primary="false"
        UCUM="mm[Hg]"/>
  <Unit property_id="0" Text="m" conversion="1" primary="true" UCUM="m"/>
  <Unit property_id="0" Text="lightyear" conversion="0" primary="false" />
  <Unit property_id="0" Text="micron" conversion="1e-06" primary="false" UCUM="um"/>
  <Unit property_id="0" Text="km" conversion="1000" primary="false" UCUM=" km "/>
  <Unit property_id="60" Text="/min" conversion="1" primary="true" UCUM="/min"/>
  <Unit property_id="60" Text="Hz" conversion="60" primary="true" UCUM="Hz"/>
  <Unit property_id="61" Text="{work}" conversion="0" primary="false" />
</PropertyUnits>`

// REQ-034: every property of the table, in source order, with the UCUM codes
// of its units once each and in source order, the primary ones marked. A unit
// with no UCUM code contributes none, so a property can have no units; a
// UCUM code is taken without the white space around it, which no UCUM code
// carries.
func TestParsePropertyUnitsReadsTheTable(t *testing.T) {
	t.Parallel()
	got, err := ParsePropertyUnits(strings.NewReader(propertyUnitsFixture))
	if err != nil {
		t.Fatalf("ParsePropertyUnits(propertyUnitsFixture) = _, %v; want no error", err)
	}
	want := []PropertyDef{
		{Code: "122", Name: "Length", Units: []UnitDef{{UCUM: "cm"}, {UCUM: "um"}, {UCUM: "m", Primary: true}, {UCUM: "km"}}},
		{Code: "125", Name: "Pressure", Units: []UnitDef{{UCUM: "mm[Hg]"}}},
		{Code: "382", Name: "Frequency", Units: []UnitDef{{UCUM: "/min", Primary: true}, {UCUM: "Hz", Primary: true}}},
		{Code: "130", Name: "Work"},
	}
	if len(got) != len(want) {
		t.Fatalf("ParsePropertyUnits = %d properties, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Code != w.Code || g.Name != w.Name || !slices.Equal(g.Units, w.Units) {
			t.Errorf("property %d = %+v, want %+v", i, g, w)
		}
	}
}

// REQ-034: a UCUM code the table lists twice for one property is one unit,
// at its first place, and primary when either row says so.
func TestParsePropertyUnitsMergesARepeatedUnit(t *testing.T) {
	t.Parallel()
	const doc = `<PropertyUnits>
		<Property id="7" Text="Electric Potential" openEHR="374" />
		<Unit property_id="7" Text="mV" primary="false" UCUM="mV"/>
		<Unit property_id="7" Text="V" primary="false" UCUM="V"/>
		<Unit property_id="7" Text="millivolt" primary="true" UCUM="mV"/>
	</PropertyUnits>`
	got, err := ParsePropertyUnits(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ParsePropertyUnits(mV twice) = _, %v; want no error", err)
	}
	want := []UnitDef{{UCUM: "mV", Primary: true}, {UCUM: "V"}}
	if len(got) != 1 || !slices.Equal(got[0].Units, want) {
		t.Errorf("ParsePropertyUnits(mV twice) = %+v, want one property with units %+v", got, want)
	}
}

// TestParsePropertyUnitsRefusesABrokenTable walks the invariants the
// generated property-unit table relies on. Each case must come back as an
// error naming what is wrong, never a panic or a table with a property or a
// unit quietly missing.
func TestParsePropertyUnitsRefusesABrokenTable(t *testing.T) {
	t.Parallel()
	const length = `<Property id="0" Text="Length" openEHR="122" />`
	doc := func(body string) string { return `<PropertyUnits>` + body + `</PropertyUnits>` }
	tests := []struct {
		name  string
		in    string
		wants []string // substrings the message must carry
	}{
		{
			name:  "another root element",
			in:    `<terminology name="openehr" version="9.9.9"></terminology>`,
			wants: []string{"PropertyUnits"},
		},
		{
			// encoding/xml stops at the first element; a second one
			// would vanish past the sha256 pin.
			name:  "a trailing element after the root",
			in:    doc(length) + `<PropertyUnits></PropertyUnits>`,
			wants: []string{"trailing", "PropertyUnits"},
		},
		{
			name:  "no properties",
			in:    doc(``),
			wants: []string{"no properties"},
		},
		{
			// A renamed Unit element leaves the properties without units,
			// which would switch the property check off.
			name:  "units under a renamed element",
			in:    doc(length + `<UnitDef property_id="0" Text="m" primary="true" UCUM="m"/>`),
			wants: []string{"no UCUM unit"},
		},
		{
			// So does a renamed UCUM attribute.
			name:  "units with a renamed UCUM attribute",
			in:    doc(length + `<Unit property_id="0" Text="m" primary="true" ucum="m"/>`),
			wants: []string{"no UCUM unit"},
		},
		{
			name:  "a property with no openEHR code",
			in:    doc(`<Property id="0" Text="Length" />`),
			wants: []string{`"0"`, "openEHR"},
		},
		{
			name:  "a property with no id",
			in:    doc(`<Property Text="Length" openEHR="122" />`),
			wants: []string{`"122"`, "id"},
		},
		{
			name:  "two properties sharing an id",
			in:    doc(length + `<Property id="0" Text="Mass" openEHR="124" />`),
			wants: []string{`"0"`, "twice"},
		},
		{
			name:  "two properties sharing an openEHR code",
			in:    doc(length + `<Property id="1" Text="Length again" openEHR="122" />`),
			wants: []string{`"122"`, "twice"},
		},
		{
			// The unit's property_id is the property's id attribute; one
			// naming no property belongs to nothing the table exposes.
			name:  "a unit naming no property",
			in:    doc(length + `<Unit property_id="9" Text="kg" primary="true" UCUM="kg"/>`),
			wants: []string{`"9"`, `"kg"`, "property"},
		},
		{
			name:  "a unit whose primary flag is not a boolean",
			in:    doc(length + `<Unit property_id="0" Text="m" primary="yes" UCUM="m"/>`),
			wants: []string{`"m"`, "primary", `"yes"`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParsePropertyUnits(strings.NewReader(tc.in))
			if err == nil {
				t.Fatalf("ParsePropertyUnits(%s) = %+v, nil; want a refusal", tc.name, got)
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("ParsePropertyUnits(%s) error = %q; want it to name %s", tc.name, err, want)
				}
			}
		})
	}
}
