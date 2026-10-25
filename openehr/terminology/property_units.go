package terminology

import (
	"iter"
	"slices"
)

// PropertyUnit is one unit of measure a [QuantityProperty] lists: its UCUM
// code, and whether the pinned table marks it a primary unit of the property.
type PropertyUnit struct {
	UCUM    string
	Primary bool
}

// QuantityProperty is one physical property of the pinned property-unit
// table, PropertyUnitData.xml, such as Length or Pressure, with the units of
// measure the table lists for it. Its code is a code of the [Property] group,
// the code a C_DV_QUANTITY constraint's property names: openehr::122 is
// Length. Every property is generated from the pin (see openehr_gen.go); a
// nil pointer is inert, with every method reporting absence.
//
// The units are UCUM codes, once each, in the order the table first lists
// them. The table gives some units no UCUM code; those are left out, so a
// property can have no units. A property can have one primary unit, several,
// or none.
type QuantityProperty struct {
	code, name string
	units      []PropertyUnit
}

// newQuantityProperty builds a property from its openEHR code, its name and
// its units in source order. Only the generated table calls it, and its
// precondition (each UCUM code at most once) is what the generator
// guarantees (see internal/termgen), so it trusts its input.
func newQuantityProperty(code, name string, units []PropertyUnit) *QuantityProperty {
	return &QuantityProperty{code: code, name: name, units: units}
}

// Code returns the property's openEHR code, e.g. "122".
func (p *QuantityProperty) Code() string {
	if p == nil {
		return ""
	}
	return p.code
}

// Name returns the name the pinned table gives the property, e.g. "Length".
func (p *QuantityProperty) Name() string {
	if p == nil {
		return ""
	}
	return p.name
}

// Len returns the number of units of the property.
func (p *QuantityProperty) Len() int {
	if p == nil {
		return 0
	}
	return len(p.units)
}

// All yields the property's units in the order the pinned table lists them.
// A nil property yields nothing.
func (p *QuantityProperty) All() iter.Seq[PropertyUnit] {
	if p == nil {
		return func(func(PropertyUnit) bool) {}
	}
	return slices.Values(p.units)
}

// Has reports whether ucum is the UCUM code of one of the property's units.
// The comparison is exact, because UCUM codes are case-sensitive: for
// Frequency, Has("MHz") and Has("mHz") are true and Has("mhz") is false.
func (p *QuantityProperty) Has(ucum string) bool {
	if p == nil {
		return false
	}
	return slices.ContainsFunc(p.units, func(u PropertyUnit) bool { return u.UCUM == ucum })
}

// QuantityProperties yields every property of the pinned property-unit table
// in source order.
func QuantityProperties() iter.Seq[*QuantityProperty] {
	return slices.Values(quantityProperties)
}

// QuantityPropertyByCode returns the property whose openEHR code is code, such
// as "122" for Length, and nil and false when the pinned table lists no such
// property. A property the table lists with no UCUM unit is found, with no
// units.
func QuantityPropertyByCode(code string) (*QuantityProperty, bool) {
	for _, p := range quantityProperties {
		if p.Code() == code {
			return p, true
		}
	}
	return nil, false
}
