package termgen

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// PropertyDef is one <Property> of PropertyUnitData.xml, the pin's
// property-unit table: the property's openEHR code (its openEHR attribute, a
// code of the property group), its name (its Text attribute), and the UCUM
// units the table lists for it.
type PropertyDef struct {
	Code  string
	Name  string
	Units []UnitDef
}

// UnitDef is one unit of a [PropertyDef]: its UCUM code, and whether the table
// marks it a primary unit of the property.
type UnitDef struct {
	UCUM    string
	Primary bool
}

// xmlPropertyUnits mirrors PropertyUnitData.xml for encoding/xml. Its root
// carries a namespace of its own; the field tags name no namespace, so they
// match the local names whatever the namespace is. A <Unit> names its
// property by the property's id attribute, not by its openEHR code.
type xmlPropertyUnits struct {
	XMLName    xml.Name      `xml:"PropertyUnits"`
	Properties []xmlProperty `xml:"Property"`
	Units      []xmlUnit     `xml:"Unit"`
}

type xmlProperty struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"Text,attr"`
	Code string `xml:"openEHR,attr"`
}

type xmlUnit struct {
	PropertyID string `xml:"property_id,attr"`
	Text       string `xml:"Text,attr"`
	Primary    string `xml:"primary,attr"`
	UCUM       string `xml:"UCUM,attr"`
}

// ParsePropertyUnits decodes PropertyUnitData.xml and returns its properties
// in source order, each with the UCUM codes of its units once each, in the
// order the table first lists them, and primary when any of its rows is. A
// unit row with no UCUM code contributes none, so a property may have no
// units. A UCUM code is taken without the white space around it, which no UCUM
// code carries.
//
// It refuses a document that is not the table, or holds anything after it;
// a table with no properties; a property with no id or no openEHR code, or
// one whose id or code another property already has; a unit naming no
// property; and a primary flag other than "true" or "false". Every refusal
// names what it refuses: the table is vendored from upstream, so a bad one is
// a version-bump surprise the maintainer has to read.
func ParsePropertyUnits(r io.Reader) ([]PropertyDef, error) {
	var doc xmlPropertyUnits
	dec := xml.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode the property-unit table: %w", err)
	}
	if err := expectEOF(dec, "PropertyUnits"); err != nil {
		return nil, err
	}
	if len(doc.Properties) == 0 {
		return nil, errors.New("the property-unit table has no properties — a renamed element would otherwise generate an empty table")
	}

	props := make([]PropertyDef, 0, len(doc.Properties))
	byID := make(map[string]int, len(doc.Properties))
	codes := make(map[string]bool, len(doc.Properties))
	for _, p := range doc.Properties {
		switch {
		case p.Code == "":
			return nil, fmt.Errorf("property id %q has no openEHR attribute — its code is what a C_DV_QUANTITY property names", p.ID)
		case p.ID == "":
			return nil, fmt.Errorf("property %q has no id attribute — its units name it by id", p.Code)
		case codes[p.Code]:
			return nil, fmt.Errorf("property code %q appears twice — its units would be ambiguous", p.Code)
		}
		if _, ok := byID[p.ID]; ok {
			return nil, fmt.Errorf("property id %q appears twice — the units naming it would be ambiguous", p.ID)
		}
		codes[p.Code] = true
		byID[p.ID] = len(props)
		props = append(props, PropertyDef{Code: p.Code, Name: p.Name})
	}

	// seen maps a property's index and a UCUM code to that unit's index in
	// the property's units, so a repeated code joins its first row.
	type unitKey struct {
		prop int
		ucum string
	}
	seen := make(map[unitKey]int, len(doc.Units))
	for _, u := range doc.Units {
		i, ok := byID[u.PropertyID]
		if !ok {
			return nil, fmt.Errorf("unit %q names property id %q, which the table does not list", u.Text, u.PropertyID)
		}
		var primary bool
		switch u.Primary {
		case "true":
			primary = true
		case "false":
		default:
			return nil, fmt.Errorf("unit %q of property %q has primary %q, want \"true\" or \"false\"", u.Text, props[i].Code, u.Primary)
		}
		ucum := strings.TrimSpace(u.UCUM)
		if ucum == "" {
			continue
		}
		key := unitKey{prop: i, ucum: ucum}
		if j, ok := seen[key]; ok {
			props[i].Units[j].Primary = props[i].Units[j].Primary || primary
			continue
		}
		seen[key] = len(props[i].Units)
		props[i].Units = append(props[i].Units, UnitDef{UCUM: ucum, Primary: primary})
	}
	return props, nil
}
