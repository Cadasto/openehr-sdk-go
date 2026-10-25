package terminology

import (
	"slices"
	"testing"
)

// REQ-034: a property answers its units in source order, the primary ones
// marked, and membership by exact UCUM code.
func TestQuantityPropertyLookups(t *testing.T) {
	t.Parallel()
	p := newQuantityProperty("382", "Frequency", []PropertyUnit{
		{UCUM: "/min", Primary: true}, {UCUM: "MHz"}, {UCUM: "mHz"},
	})
	if got := p.Code(); got != "382" {
		t.Errorf("Code() = %q, want 382", got)
	}
	if got := p.Name(); got != "Frequency" {
		t.Errorf("Name() = %q, want Frequency", got)
	}
	if got := p.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}
	want := []PropertyUnit{{UCUM: "/min", Primary: true}, {UCUM: "MHz"}, {UCUM: "mHz"}}
	if got := slices.Collect(p.All()); !slices.Equal(got, want) {
		t.Errorf("All() = %+v, want %+v in source order", got, want)
	}
	tests := []struct {
		ucum string
		want bool
	}{
		{"/min", true},
		// UCUM codes are case-sensitive: MHz is megahertz, mHz millihertz,
		// and mhz is neither.
		{"MHz", true},
		{"mHz", true},
		{"mhz", false},
		{"MHZ", false},
		{"Hz", false},
		{"", false},
		{" /min", false},
	}
	for _, tc := range tests {
		if got := p.Has(tc.ucum); got != tc.want {
			t.Errorf("Has(%q) = %v, want %v", tc.ucum, got, tc.want)
		}
	}
}

// REQ-034, REQ-025: a nil property reports absence and never panics.
func TestNilQuantityPropertyIsInert(t *testing.T) {
	t.Parallel()
	var p *QuantityProperty
	if p.Has("m") || p.Len() != 0 || p.Code() != "" || p.Name() != "" {
		t.Error("a nil *QuantityProperty must answer zero values")
	}
	if n := len(slices.Collect(p.All())); n != 0 {
		t.Errorf("nil All yields %d units, want 0", n)
	}
}

// REQ-034: the property registry accessors range over the generated table.
// The test swaps the package table for its own and restores it after, so it
// cannot run in parallel with any other.
func TestQuantityPropertyRegistryUsesTheTable(t *testing.T) {
	save := quantityProperties
	t.Cleanup(func() { quantityProperties = save })
	a := newQuantityProperty("1", "a", []PropertyUnit{{UCUM: "m", Primary: true}})
	b := newQuantityProperty("2", "b", nil)
	quantityProperties = []*QuantityProperty{a, b}

	if got := slices.Collect(QuantityProperties()); !slices.Equal(got, []*QuantityProperty{a, b}) {
		t.Errorf("QuantityProperties() yields %d properties, want the two installed in source order", len(got))
	}
	if p, ok := QuantityPropertyByCode("2"); !ok || p != b {
		t.Errorf("QuantityPropertyByCode(%q) = %v, %v; want the installed property", "2", p, ok)
	}
	// A listed property with no units is found, and reports no units.
	if p, ok := QuantityPropertyByCode("2"); !ok || p.Len() != 0 {
		t.Errorf("QuantityPropertyByCode(%q) = %v, %v; want it found with no units", "2", p, ok)
	}
	if p, ok := QuantityPropertyByCode("3"); ok || p != nil {
		t.Errorf("QuantityPropertyByCode(%q) = %v, %v; want nil, false for a code the table does not list", "3", p, ok)
	}
}
