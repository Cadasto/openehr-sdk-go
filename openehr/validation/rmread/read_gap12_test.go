package rmread_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// TestReadSingle_DataValuesGAP12 covers the SDK-GAP-12 read-side
// accessors that mirror the new writers (DV_COUNT, DV_QUANTITY,
// DV_PROPORTION, DV_URI, DV_PARSABLE) and the generic DV_INTERVAL<T>
// reader across several T's — present scalars report ok=true.
func TestReadSingle_DataValuesGAP12(t *testing.T) {
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

// TestReadSingle_DataValuesGAP12_unknownAttr — an attr the reader does
// not recognise reports ok=false so the walker can flag it.
func TestReadSingle_DataValuesGAP12_unknownAttr(t *testing.T) {
	if _, ok := rmread.ReadSingle(&rm.DVCount{}, "DV_COUNT", "no_such_attr"); ok {
		t.Error("ReadSingle(DV_COUNT, no_such_attr) ok=true, want false")
	}
}
