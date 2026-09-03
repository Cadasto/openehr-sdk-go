package rmread_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// TestReadSingle_DataValues covers the REQ-110 read-side
// accessors that mirror the new writers (DV_COUNT, DV_QUANTITY,
// DV_PROPORTION, DV_URI, DV_PARSABLE) and the generic DV_INTERVAL<T>
// reader across several T's — present scalars report ok=true.
func TestReadSingle_DataValues(t *testing.T) {
	prec := rm.Integer(2)
	cases := []struct {
		name   string
		parent any
		rmType string
		attr   string
	}{
		{"DVCount.magnitude", &rm.DVCount{Magnitude: 5}, "DV_COUNT", "magnitude"},
		{"DVQuantity.magnitude", &rm.DVQuantity{Magnitude: 1.5}, "DV_QUANTITY", "magnitude"},
		{"DVQuantity.units", &rm.DVQuantity{Units: "kg"}, "DV_QUANTITY", "units"},
		{"DVProportion.numerator", &rm.DVProportion{Numerator: 1, Denominator: 2}, "DV_PROPORTION", "numerator"},
		{"DVProportion.denominator", &rm.DVProportion{Numerator: 1, Denominator: 2}, "DV_PROPORTION", "denominator"},
		{"DVProportion.precision", &rm.DVProportion{Precision: &prec}, "DV_PROPORTION", "precision"},
		{"DVURI.value", &rm.DVURI{Value: "http://example.com"}, "DV_URI", "value"},
		{"DVParsable.value", &rm.DVParsable{Value: "x", Formalism: "text/plain"}, "DV_PARSABLE", "value"},
		{"DVParsable.formalism", &rm.DVParsable{Value: "x", Formalism: "text/plain"}, "DV_PARSABLE", "formalism"},
		{"DVInterval[DVQuantity].lower", &rm.DVInterval[rm.DVQuantity]{}, "DV_INTERVAL", "lower"},
		{"DVInterval[DVQuantity].lower_unbounded", &rm.DVInterval[rm.DVQuantity]{}, "DV_INTERVAL", "lower_unbounded"},
		{"DVInterval[DVCount].upper", &rm.DVInterval[rm.DVCount]{}, "DV_INTERVAL", "upper"},
		{"DVInterval[DVDate].upper_included", &rm.DVInterval[rm.DVDate]{}, "DV_INTERVAL", "upper_included"},
		{"DVInterval[DVDateTime].lower", &rm.DVInterval[rm.DVDateTime]{}, "DV_INTERVAL", "lower"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := rmread.ReadSingle(tc.parent, tc.rmType, tc.attr); !ok {
				t.Errorf("ReadSingle(%s, %q) ok=false, want true", tc.rmType, tc.attr)
			}
		})
	}
}

// TestReadSingle_DataValues_unknownAttr — an attr the reader does
// not recognise reports ok=false so the walker can flag it.
func TestReadSingle_DataValues_unknownAttr(t *testing.T) {
	if _, ok := rmread.ReadSingle(&rm.DVCount{}, "DV_COUNT", "no_such_attr"); ok {
		t.Error("ReadSingle(DV_COUNT, no_such_attr) ok=true, want false")
	}
}

// TestReadMultiple_DVTextMappings covers the DV_TEXT / DV_CODED_TEXT
// `mappings` container (REQ-112): rminfo declares it a multiple of
// TERM_MAPPING, so the RM-floor walker must be able to read it and descend
// into each element. Elements are boxed as `*rm.TermMapping` so the
// walker's rmTypeInfo switch recognises them as TERM_MAPPING nodes.
func TestReadMultiple_DVTextMappings(t *testing.T) {
	mappings := []rm.TermMapping{{
		Match:  rm.Character("="),
		Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "73211009"},
	}}
	cases := []struct {
		name   string
		parent any
		rmType string
	}{
		{"DVText", &rm.DVText{Value: "x", Mappings: mappings}, "DV_TEXT"},
		{"DVCodedText", &rm.DVCodedText{DVText: rm.DVText{Value: "x", Mappings: mappings}}, "DV_CODED_TEXT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, ok := rmread.ReadMultiple(tc.parent, tc.rmType, "mappings")
			if !ok {
				t.Fatalf("ReadMultiple(%s, mappings) ok=false, want true", tc.rmType)
			}
			if len(items) != 1 {
				t.Fatalf("ReadMultiple(%s, mappings) returned %d items, want 1", tc.rmType, len(items))
			}
			if _, isPtr := items[0].(*rm.TermMapping); !isPtr {
				t.Errorf("ReadMultiple(%s, mappings)[0] type = %T, want *rm.TermMapping", tc.rmType, items[0])
			}
		})
	}
}

// TestReadMultiple_DVTextMappingsEmpty — an absent or empty `mappings`
// still reports the attribute as addressable (ok=true, zero items), per
// the ReadMultiple contract. The absent-vs-empty distinction the
// `mappings_valid` invariant needs is read from Go slice nilness by the
// floor's own evaluator, not from this signal.
func TestReadMultiple_DVTextMappingsEmpty(t *testing.T) {
	items, ok := rmread.ReadMultiple(&rm.DVText{Value: "x"}, "DV_TEXT", "mappings")
	if !ok {
		t.Error("ReadMultiple(DV_TEXT, mappings) on a nil slice ok=false, want true (attr addressable even when absent)")
	}
	if len(items) != 0 {
		t.Errorf("ReadMultiple(DV_TEXT, mappings) on a nil slice returned %d items, want 0", len(items))
	}
}

// TestReadSingle_TermMapping covers TERM_MAPPING's own attributes
// (REQ-112): `match` (Character), `purpose` (DV_CODED_TEXT) and `target`
// (CODE_PHRASE). Populated attributes report ok=true; the RM zero of each
// reports ok=false so the walker flags the RM-mandatory ones absent.
func TestReadSingle_TermMapping(t *testing.T) {
	tm := &rm.TermMapping{
		Match:   rm.Character("="),
		Purpose: &rm.DVCodedText{DVText: rm.DVText{Value: "billing"}},
		Target:  rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "73211009"},
	}
	for _, attr := range []string{"match", "purpose", "target"} {
		if _, ok := rmread.ReadSingle(tm, "TERM_MAPPING", attr); !ok {
			t.Errorf("ReadSingle(TERM_MAPPING, %q) ok=false, want true", attr)
		}
	}
	if v, ok := rmread.ReadSingle(tm, "TERM_MAPPING", "target"); !ok {
		t.Error("ReadSingle(TERM_MAPPING, target) ok=false, want true")
	} else if cp, isCP := v.(rm.CodePhrase); !isCP || cp.CodeString != "73211009" {
		t.Errorf("ReadSingle(TERM_MAPPING, target) unexpected: %#v", v)
	}

	empty := &rm.TermMapping{}
	for _, attr := range []string{"match", "purpose", "target"} {
		if _, ok := rmread.ReadSingle(empty, "TERM_MAPPING", attr); ok {
			t.Errorf("ReadSingle(zero TERM_MAPPING, %q) ok=true, want false", attr)
		}
	}
	if _, ok := rmread.ReadSingle(tm, "TERM_MAPPING", "no_such_attr"); ok {
		t.Error("ReadSingle(TERM_MAPPING, no_such_attr) ok=true, want false")
	}
}
