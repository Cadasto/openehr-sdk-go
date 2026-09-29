package rmread

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestHandles_EveryLocatableAndOrdered checks that rmread models every
// registered concrete LOCATABLE and DV_ORDERED, plus REFERENCE_RANGE, the
// element type of DV_ORDERED.other_reference_ranges (REQ-112). The RM floor
// reads a node's attributes only when Handles accepts it; any other type is
// an opaque leaf whose RM-mandatory attributes go unchecked. A class added to
// the registry without a reader fails here.
func TestHandles_EveryLocatableAndOrdered(t *testing.T) {
	var checked int
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		v := ctor()
		_, locatable := v.(rm.Locatable)
		_, ordered := v.(rm.DVOrdered)
		if !locatable && !ordered && name != "REFERENCE_RANGE" {
			continue
		}
		checked++
		if !Handles(v) {
			t.Errorf("Handles(%T) = false for registered %s, want true: the RM floor treats it as an opaque leaf", v, name)
		}
	}
	if checked == 0 {
		t.Fatal("the registry yields no LOCATABLE or DV_ORDERED concretes; registrations missing?")
	}
}

// orderedBody returns a canonical-JSON body of RM type rmType carrying all
// three DV_ORDERED attributes: a normal_status, an open normal_range and one
// other_reference_ranges entry.
func orderedBody(rmType string) []byte {
	const openInterval = `{"_type":"DV_INTERVAL","lower_unbounded":true,"upper_unbounded":true,` +
		`"lower_included":false,"upper_included":false}`
	return []byte(`{"_type":"` + rmType + `",` +
		`"normal_status":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr_normal_statuses"},"code_string":"N"},` +
		`"normal_range":` + openInterval + `,` +
		`"other_reference_ranges":[{"_type":"REFERENCE_RANGE","meaning":{"_type":"DV_TEXT","value":"critical"},"range":` + openInterval + `}]}`)
}

// TestReadDVOrderedAttributesParity checks that every registered DV_ORDERED
// concrete reads the three attributes DV_ORDERED declares (REQ-112):
// normal_status, normal_range and the other_reference_ranges container,
// whose elements must be REFERENCE_RANGE nodes the walk can descend into.
// Each case is decoded from JSON, so the readers are exercised on a real
// value; the zero value reads the two single attributes as absent and the
// container as present but empty.
func TestReadDVOrderedAttributesParity(t *testing.T) {
	var checked int
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.DVOrdered); !ok {
			continue
		}
		checked++
		t.Run(name, func(t *testing.T) {
			v, err := typereg.Default.Decode(orderedBody(name))
			if err != nil {
				t.Fatalf("typereg.Decode(%s with DV_ORDERED attributes): %v", name, err)
			}
			if got, ok := ReadSingle(v, name, "normal_status"); !ok {
				t.Errorf("ReadSingle(%T, normal_status) = (%v, false), want present", v, got)
			}
			if got, ok := ReadSingle(v, name, "normal_range"); !ok || !Handles(got) {
				t.Errorf("ReadSingle(%T, normal_range) = (%T, %v), want a modelled interval and true", v, got, ok)
			}
			ranges, ok := ReadMultiple(v, name, "other_reference_ranges")
			if !ok || len(ranges) != 1 {
				t.Fatalf("ReadMultiple(%T, other_reference_ranges) = (%d items, %v), want (1, true)", v, len(ranges), ok)
			}
			if gotName, _ := rm.RMTypeName(ranges[0]); gotName != "REFERENCE_RANGE" || !Handles(ranges[0]) {
				t.Errorf("other_reference_ranges[0] = %T (RM type %q, modelled %v), want a modelled REFERENCE_RANGE", ranges[0], gotName, Handles(ranges[0]))
			}
			if got, ok := ReadSingle(ranges[0], "REFERENCE_RANGE", "meaning"); !ok {
				t.Errorf("ReadSingle(%T, meaning) = (%v, false), want present", ranges[0], got)
			}

			zero := ctor()
			for _, attr := range []string{"normal_status", "normal_range"} {
				if got, ok := ReadSingle(zero, name, attr); ok {
					t.Errorf("ReadSingle(zero %T, %q) = (%v, true), want absent", zero, attr, got)
				}
			}
			if got, ok := ReadMultiple(zero, name, "other_reference_ranges"); !ok || len(got) != 0 {
				t.Errorf("ReadMultiple(zero %T, other_reference_ranges) = (%d items, %v), want (0, true)", zero, len(got), ok)
			}
		})
	}
	if checked == 0 {
		t.Fatal("the registry yields no DV_ORDERED concretes; registrations missing?")
	}
}

// TestReadSingle_OrdinalAndScale pins the DV_ORDINAL and DV_SCALE readers
// (REQ-112). symbol (DV_CODED_TEXT) and value are RM-mandatory. symbol is
// value-typed and reads as absent when it carries neither a value nor a
// code; value is an Integer or Real and always reads as present, as
// DV_COUNT.magnitude does.
func TestReadSingle_OrdinalAndScale(t *testing.T) {
	symbol := rm.DVCodedText{
		Value:        "mild",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at1"},
	}
	cases := []struct {
		name   string
		parent any
		attr   string
		ok     bool
	}{
		{"DV_ORDINAL symbol set", &rm.DVOrdinal{Value: 1, Symbol: symbol}, "symbol", true},
		{"DV_ORDINAL symbol absent", &rm.DVOrdinal{Value: 1}, "symbol", false},
		{"DV_ORDINAL value zero", rm.DVOrdinal{Symbol: symbol}, "value", true},
		{"DV_SCALE symbol set", &rm.DVScale{Value: 0.5, Symbol: symbol}, "symbol", true},
		{"DV_SCALE symbol absent", rm.DVScale{Value: 0.5}, "symbol", false},
		{"DV_SCALE value zero", &rm.DVScale{Symbol: symbol}, "value", true},
		{"DV_ORDINAL unknown attribute", &rm.DVOrdinal{}, "magnitude", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := ReadSingle(tc.parent, "", tc.attr); ok != tc.ok {
				t.Errorf("ReadSingle(%T, %q) = (%v, %v), want ok=%v", tc.parent, tc.attr, got, ok, tc.ok)
			}
		})
	}
}

// TestReadSingle_ReferenceRange pins the REFERENCE_RANGE reader (REQ-112).
// meaning (DV_TEXT) and range (DV_INTERVAL) are RM-mandatory. range is
// value-typed: an omitted range decodes to an interval with no bound and
// neither side open, so that zero interval reads as absent. Any interval that
// sets a bound or opens a side reads as present, as a pointer the walk
// descends into.
func TestReadSingle_ReferenceRange(t *testing.T) {
	meaning := rm.DVText{Value: "critical"}
	open := rm.DVInterval[rm.DVOrdered]{LowerUnbounded: true, UpperUnbounded: true}
	bounded := rm.DVInterval[rm.DVOrdered]{Lower: rm.DVCount{Magnitude: 1}, UpperUnbounded: true}
	cases := []struct {
		name   string
		parent any
		attr   string
		ok     bool
	}{
		{"meaning set", &rm.ReferenceRange[rm.DVOrdered]{Meaning: meaning}, "meaning", true},
		{"meaning absent", &rm.ReferenceRange[rm.DVOrdered]{}, "meaning", false},
		{"meaning empty", rm.ReferenceRange[rm.DVOrdered]{Meaning: rm.DVText{}}, "meaning", false},
		{"range zero", &rm.ReferenceRange[rm.DVOrdered]{Meaning: meaning}, "range", false},
		{"range open on both sides", &rm.ReferenceRange[rm.DVOrdered]{Range: open}, "range", true},
		{"range with a bound", rm.ReferenceRange[rm.DVOrdered]{Range: bounded}, "range", true},
		{"unknown attribute", &rm.ReferenceRange[rm.DVOrdered]{}, "value", false},
		// The typed instantiations DV_COUNT, DV_QUANTITY and DV_PROPORTION
		// carry, in value and pointer form.
		{"REFERENCE_RANGE<DV_COUNT> meaning, value form", rm.ReferenceRange[rm.DVCount]{Meaning: meaning}, "meaning", true},
		{"REFERENCE_RANGE<DV_COUNT> range, pointer form", &rm.ReferenceRange[rm.DVCount]{Range: open}, "range", true},
		{"REFERENCE_RANGE<DV_QUANTITY> range, value form", rm.ReferenceRange[rm.DVQuantity]{Range: bounded}, "range", true},
		{"REFERENCE_RANGE<DV_QUANTITY> meaning, pointer form", &rm.ReferenceRange[rm.DVQuantity]{Meaning: meaning}, "meaning", true},
		{"REFERENCE_RANGE<DV_PROPORTION> meaning, value form", rm.ReferenceRange[rm.DVProportion]{Meaning: meaning}, "meaning", true},
		{"REFERENCE_RANGE<DV_PROPORTION> range, pointer form", &rm.ReferenceRange[rm.DVProportion]{Range: open}, "range", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ReadSingle(tc.parent, "REFERENCE_RANGE", tc.attr)
			if ok != tc.ok {
				t.Errorf("ReadSingle(%T, %q) = (%v, %v), want ok=%v", tc.parent, tc.attr, got, ok, tc.ok)
			}
			if ok && tc.attr == "range" {
				if _, isPtr := got.(*rm.DVInterval[rm.DVOrdered]); !isPtr {
					t.Errorf("ReadSingle(%T, range) = %T, want *rm.DVInterval[rm.DVOrdered]", tc.parent, got)
				}
			}
		})
	}
}

// TestReadSingle_EHRAccess pins the EHR_ACCESS reader (REQ-112): the
// RM-mandatory LOCATABLE archetype_node_id and name, and the optional
// settings, read like any other LOCATABLE's.
func TestReadSingle_EHRAccess(t *testing.T) {
	full := &rm.EHRAccess{ArchetypeNodeID: "openEHR-EHR-EHR_ACCESS.generic.v1", Name: rm.DVText{Value: "EHR Access"}}
	for _, attr := range []string{"archetype_node_id", "name"} {
		if got, ok := ReadSingle(full, "EHR_ACCESS", attr); !ok {
			t.Errorf("ReadSingle(EHR_ACCESS with %s set, %q) = (%v, false), want present", attr, attr, got)
		}
		if got, ok := ReadSingle(rm.EHRAccess{}, "EHR_ACCESS", attr); ok {
			t.Errorf("ReadSingle(zero EHR_ACCESS, %q) = (%v, true), want absent", attr, got)
		}
	}
	if got, ok := ReadSingle(full, "EHR_ACCESS", "settings"); ok {
		t.Errorf("ReadSingle(EHR_ACCESS without settings, settings) = (%v, true), want absent", got)
	}
}

// TestReadAccuracyWhereDeclared checks that every registered DV_ORDERED
// concrete whose rminfo attributes include accuracy reads it (REQ-112):
// DV_DATE, DV_TIME and DV_DATE_TIME carry a DV_DURATION there, a node the
// walk must descend into, and the DV_AMOUNT types a Real. Each case is decoded
// from JSON with accuracy set, and the zero value reads it as absent. The
// cases come from the registry and rminfo, so a type that gains the attribute
// fails here until its reader does.
func TestReadAccuracyWhereDeclared(t *testing.T) {
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not list attributes")
	}
	var checked int
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.DVOrdered); !ok || !slices.Contains(lister.AttributeNames(name), "accuracy") {
			continue
		}
		checked++
		t.Run(name, func(t *testing.T) {
			attrType, _ := rminfo.Default.AttributeRMType(name, "accuracy")
			accuracy := `0.5`
			if attrType == "DV_DURATION" {
				accuracy = `{"_type":"DV_DURATION","value":"PT1H"}`
			}
			v, err := typereg.Default.Decode([]byte(`{"_type":"` + name + `","accuracy":` + accuracy + `}`))
			if err != nil {
				t.Fatalf("typereg.Decode(%s with accuracy): %v", name, err)
			}
			got, ok := ReadSingle(v, name, "accuracy")
			if !ok {
				t.Fatalf("ReadSingle(%T, accuracy) = (%v, false), want present", v, got)
			}
			if gotName, _ := rm.RMTypeName(got); attrType == "DV_DURATION" && (gotName != "DV_DURATION" || !Handles(got)) {
				t.Errorf("ReadSingle(%T, accuracy) = %T, want a modelled DV_DURATION", v, got)
			}
			if got, ok := ReadSingle(ctor(), name, "accuracy"); ok {
				t.Errorf("ReadSingle(zero %s, accuracy) = (%v, true), want absent", name, got)
			}
		})
	}
	if checked == 0 {
		t.Fatal("no registered DV_ORDERED declares accuracy in rminfo; the parity check asserts nothing")
	}
}
