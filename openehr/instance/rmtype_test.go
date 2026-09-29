package instance

import (
	"errors"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// REQ-107: the generator materialises each OPT rm_type_name, generic
// intervals included, as its concrete RM value.
func TestNewRMForOPTType_generics(t *testing.T) {
	cases := []struct {
		declared string
		check    func(any) bool
	}{
		{"DV_INTERVAL<DV_QUANTITY>", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVQuantity]); return ok }},
		{"DV_INTERVAL<DV_COUNT>", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVCount]); return ok }},
		{"DV_INTERVAL<DV_DATE_TIME>", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVDateTime]); return ok }},
		{"DV_INTERVAL<DV_DATE>", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVDate]); return ok }},
		{"DV_INTERVAL<DV_TIME>", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVTime]); return ok }},
		{"DV_INTERVAL<DV_PROPORTION>", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVProportion]); return ok }},
		{"DV_INTERVAL<DV_ORDERED>", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVOrdered]); return ok }},
		{"DV_TEXT", func(v any) bool { _, ok := v.(*rm.DVText); return ok }},
		{"DV_INTERVAL", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVOrdered]); return ok }},
	}
	for _, tc := range cases {
		t.Run(tc.declared, func(t *testing.T) {
			v, err := newRMForOPTType(tc.declared)
			if err != nil {
				t.Fatalf("newRMForOPTType: %v", err)
			}
			if !tc.check(v) {
				t.Fatalf("unexpected concrete type %T", v)
			}
		})
	}
}

func TestNewRMForOPTType_unknownGeneric(t *testing.T) {
	_, err := newRMForOPTType("DV_INTERVAL<NOT_A_TYPE>")
	if !errors.Is(err, rmwrite.ErrUnknownRMType) {
		t.Fatalf("want ErrUnknownRMType, got %v", err)
	}
}

func TestParseBMMGeneric(t *testing.T) {
	base, param, ok := parseBMMGeneric("DV_INTERVAL<DV_QUANTITY>")
	if !ok || base != "DV_INTERVAL" || param != "DV_QUANTITY" {
		t.Fatalf("parseBMMGeneric = %q %q %v, want DV_INTERVAL DV_QUANTITY true", base, param, ok)
	}
	if _, _, ok := parseBMMGeneric("DV_TEXT"); ok {
		t.Fatal("expected non-generic DV_TEXT to return ok=false")
	}
	if _, _, ok := parseBMMGeneric("DV_INTERVAL<DV_QUANTITY>junk"); ok {
		t.Fatal("expected trailing junk after generic param to return ok=false")
	}
	if _, _, ok := parseBMMGeneric("DV_INTERVAL<>"); ok {
		t.Fatal("expected empty generic param to return ok=false")
	}
	base, param, ok = parseBMMGeneric("DV_INTERVAL<DV_INTERVAL<DV_QUANTITY>>")
	if !ok || base != "DV_INTERVAL" || param != "DV_INTERVAL<DV_QUANTITY>" {
		t.Fatalf("nested generic = %q %q %v, want DV_INTERVAL / DV_INTERVAL<DV_QUANTITY> / true — the closer is the last '>'", base, param, ok)
	}
}

// TestIntervalInstantiationParity checks that the generator builds and
// seeds every DV_INTERVAL instantiation the reader covers (REQ-107). The
// canonical set is the one TestTypedIntervalReaderParity
// (openehr/validation/rmread) derives: one DV_INTERVAL<X> per registered
// type X that implements rm.DVOrdered, plus the bare
// DV_INTERVAL<DV_ORDERED>. For each name, newRMForOPTType must build the
// instantiation rmnames gives that name, and populatePrimitiveDefault must
// seed it open on both sides: the state the bound writes and the endpoint
// pass start from. A new DV_ORDERED descendant fails here until the
// generator gains it. The flags are read back through rmread so the check
// does not share the generator's own interval switch.
func TestIntervalInstantiationParity(t *testing.T) {
	const bare = "DV_INTERVAL<DV_ORDERED>"
	names := []string{bare}
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.DVOrdered); ok {
			names = append(names, "DV_INTERVAL<"+name+">")
		}
	}
	if len(names) == 1 {
		t.Fatal("registry yields no DVOrdered implementers; registrations missing?")
	}
	slices.Sort(names)

	g := &generator{}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			v, err := newRMForOPTType(name)
			if err != nil {
				t.Fatalf("newRMForOPTType(%q): %v", name, err)
			}
			if name == bare {
				if _, ok := v.(*rm.DVInterval[rm.DVOrdered]); !ok {
					t.Fatalf("newRMForOPTType(%q) built %T, want *rm.DVInterval[rm.DVOrdered]", name, v)
				}
			} else if got, ok := rmnames.TypedIntervalName(v); !ok || got != name {
				t.Fatalf("newRMForOPTType(%q) built %T, which rmnames names (%q, %v)", name, v, got, ok)
			}
			g.populatePrimitiveDefault(v)
			for _, attr := range []string{"lower_unbounded", "upper_unbounded"} {
				got, ok := rmread.ReadSingle(v, "DV_INTERVAL", attr)
				if !ok || got != true {
					t.Errorf("after populatePrimitiveDefault, ReadSingle(%T, %q) = (%v, %v), want (true, true)", v, attr, got, ok)
				}
			}
		})
	}
}
