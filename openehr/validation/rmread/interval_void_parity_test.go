package rmread

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/xml"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ112IntervalBoundVoidEncoderParity keeps the canonical encoders on the
// reading of an empty interval bound that REQ-112 gives the floor and
// [rm.IsEmptyIntervalBound] implements: a bound of a concrete type is empty
// exactly when it is that type's zero value. The oracle is reflection's
// IsZero, so it is independent of the generated predicate the encoders call.
//
// For each concrete bound type it encodes an interval whose two sides are
// both open and carry the same bound: the zero value, then a value with one
// leaf field set, for every leaf field. Canonical JSON and XML must omit the
// bound exactly when it is the zero value. Together with
// TestIntervalBoundVoidPredicates, which proves the predicate sees every
// field, a field the predicate misses, or an encoder that stops using it,
// fails here.
func TestREQ112IntervalBoundVoidEncoderParity(t *testing.T) {
	cases := []struct {
		zero   any
		isVoid func(any) bool
		encode func(t *testing.T, bound any) (jsonOmits, xmlOmits bool)
	}{
		{rm.DVCount{}, isZeroValue, openBoundOmitted[rm.DVCount]},
		{rm.DVDate{}, isZeroValue, openBoundOmitted[rm.DVDate]},
		{rm.DVDateTime{}, isZeroValue, openBoundOmitted[rm.DVDateTime]},
		{rm.DVDuration{}, isZeroValue, openBoundOmitted[rm.DVDuration]},
		{rm.DVOrdinal{}, isZeroValue, openBoundOmitted[rm.DVOrdinal]},
		{rm.DVProportion{}, isZeroValue, openBoundOmitted[rm.DVProportion]},
		{rm.DVQuantity{}, isZeroValue, openBoundOmitted[rm.DVQuantity]},
		{rm.DVScale{}, isZeroValue, openBoundOmitted[rm.DVScale]},
		{rm.DVTime{}, isZeroValue, openBoundOmitted[rm.DVTime]},
	}
	if got, want := len(cases), len(typedIntervals)/2; got != want {
		t.Fatalf("%d bound types compared, want %d (one per typed DV_INTERVAL instantiation)", got, want)
	}
	for _, tc := range cases {
		typ := reflect.TypeOf(tc.zero)
		t.Run(typ.Name(), func(t *testing.T) {
			check := func(sample string, bound any) {
				t.Helper()
				want := tc.isVoid(bound)
				jsonOmits, xmlOmits := tc.encode(t, bound)
				if jsonOmits != want {
					t.Errorf("%s %s: canonical JSON omits the open bound = %v, but the bound is empty = %v", typ.Name(), sample, jsonOmits, want)
				}
				if xmlOmits != want {
					t.Errorf("%s %s: canonical XML omits the open bound = %v, but the bound is empty = %v", typ.Name(), sample, xmlOmits, want)
				}
			}
			check("zero value", tc.zero)
			for _, f := range leafFields(typ, nil, "") {
				v := reflect.New(typ).Elem()
				field := v.FieldByIndex(f.index)
				nz, err := encodableNonZeroValue(field.Type())
				if err != nil {
					t.Fatalf("%s.%s: %v", typ.Name(), f.name, err)
				}
				field.Set(nz)
				check("with only "+f.name+" set", v.Interface())
			}
		})
	}
}

// TestREQ112IntervalBoundVoidOrderedEncoderParity is the same parity for the
// bare DV_INTERVAL, whose bound is typed by the DV_ORDERED interface
// (REQ-112, REQ-052, REQ-056). The encoders must agree with the reading
// wire.md gives there: only a nil or a typed-nil pointer is empty, and a
// zero value, or a pointer to one, behind the interface is a bound. Each
// concrete bound type contributes its typed-nil pointer, its zero value and a
// pointer to its zero value.
func TestREQ112IntervalBoundVoidOrderedEncoderParity(t *testing.T) {
	samples := []orderedSample{{name: "nil interface", bound: nil, empty: true}}
	samples = append(samples, orderedSamples[rm.DVCount](t, "DV_COUNT")...)
	samples = append(samples, orderedSamples[rm.DVDate](t, "DV_DATE")...)
	samples = append(samples, orderedSamples[rm.DVDateTime](t, "DV_DATE_TIME")...)
	samples = append(samples, orderedSamples[rm.DVDuration](t, "DV_DURATION")...)
	samples = append(samples, orderedSamples[rm.DVOrdinal](t, "DV_ORDINAL")...)
	samples = append(samples, orderedSamples[rm.DVProportion](t, "DV_PROPORTION")...)
	samples = append(samples, orderedSamples[rm.DVQuantity](t, "DV_QUANTITY")...)
	samples = append(samples, orderedSamples[rm.DVScale](t, "DV_SCALE")...)
	samples = append(samples, orderedSamples[rm.DVTime](t, "DV_TIME")...)
	if got, want := len(samples), 1+3*len(typedIntervals)/2; got != want {
		t.Fatalf("%d DV_ORDERED samples, want %d (nil, then three per typed DV_INTERVAL instantiation)", got, want)
	}
	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			want := s.empty
			jsonOmits, xmlOmits := openBoundOmitted[rm.DVOrdered](t, s.bound)
			if jsonOmits != want {
				t.Errorf("DV_INTERVAL<DV_ORDERED> with a %s bound: canonical JSON omits the open bound = %v, but the bound is empty = %v", s.name, jsonOmits, want)
			}
			if xmlOmits != want {
				t.Errorf("DV_INTERVAL<DV_ORDERED> with a %s bound: canonical XML omits the open bound = %v, but the bound is empty = %v", s.name, xmlOmits, want)
			}
		})
	}
}

// orderedSample is one bound for the bare DV_INTERVAL.
type orderedSample struct {
	name  string
	bound rm.DVOrdered
	// empty is whether the bound is empty: only nil and a typed-nil pointer.
	empty bool
}

// orderedSamples returns T's typed-nil pointer, zero value and pointer to a
// zero value, each held in the DV_ORDERED interface.
func orderedSamples[T rm.DVOrdered](t *testing.T, name string) []orderedSample {
	t.Helper()
	var zero T
	nilPtr, ok := any((*T)(nil)).(rm.DVOrdered)
	if !ok {
		t.Fatalf("*%s does not implement DV_ORDERED", name)
	}
	zeroPtr, ok := any(new(T)).(rm.DVOrdered)
	if !ok {
		t.Fatalf("*%s does not implement DV_ORDERED", name)
	}
	return []orderedSample{
		{name: name + " typed nil", bound: nilPtr, empty: true},
		{name: name + " zero value", bound: zero},
		{name: name + " pointer to a zero value", bound: zeroPtr},
	}
}

// isZeroValue is the parity test's oracle for a concrete bound: the value is
// its type's zero value.
func isZeroValue(v any) bool { return reflect.ValueOf(v).IsZero() }

// encodableNonZeroValue is nonZeroValue with one change the encoders need:
// a slice is empty but not nil. It is still not the zero value, and a
// one-element slice of zero elements may not encode (a zero TERM_MAPPING's
// `match` is an empty Character, which canonical JSON refuses).
func encodableNonZeroValue(t reflect.Type) (reflect.Value, error) {
	if t.Kind() == reflect.Slice {
		return reflect.MakeSlice(t, 0, 0), nil
	}
	return nonZeroValue(t)
}

// openBoundOmitted encodes a DV_INTERVAL whose two sides are both open and
// carry bound, and reports whether canonical JSON and canonical XML each
// left the bound out. Both sides must agree within one encoding.
func openBoundOmitted[T rm.DVOrdered](t *testing.T, bound any) (jsonOmits, xmlOmits bool) {
	t.Helper()
	b, ok := bound.(T)
	if !ok && bound != nil {
		t.Fatalf("bound %T is not a %T", bound, b)
	}
	iv := &rm.DVInterval[T]{Lower: b, LowerUnbounded: true, Upper: b, UpperUnbounded: true}

	js, err := canjson.Marshal(iv)
	if err != nil {
		t.Fatalf("canjson.Marshal(%T): %v", iv, err)
	}
	members := jsonMembers(t, js)
	if members["lower"] != members["upper"] {
		t.Fatalf("canonical JSON treats the two open sides differently: %s", js)
	}

	xs, err := canxml.Marshal(iv)
	if err != nil {
		t.Fatalf("canxml.Marshal(%T): %v", iv, err)
	}
	children := xmlChildren(t, xs)
	if children["lower"] != children["upper"] {
		t.Fatalf("canonical XML treats the two open sides differently: %s", xs)
	}
	return !members["upper"], !children["upper"]
}

// jsonMembers reports which member names the JSON object b carries.
func jsonMembers(t *testing.T, b []byte) map[string]bool {
	t.Helper()
	dec := jsontext.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.ReadToken(); err != nil || tok.Kind() != '{' {
		t.Fatalf("wire is not a JSON object (token %v, error %v): %s", tok, err, b)
	}
	names := map[string]bool{}
	for dec.PeekKind() != '}' {
		tok, err := dec.ReadToken()
		if err != nil {
			t.Fatalf("read member name: %v: %s", err, b)
		}
		names[tok.String()] = true
		if err := dec.SkipValue(); err != nil {
			t.Fatalf("skip member %q: %v: %s", tok.String(), err, b)
		}
	}
	return names
}

// xmlChildren reports which child element names the root element of b
// carries.
func xmlChildren(t *testing.T, b []byte) map[string]bool {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(b))
	names := map[string]bool{}
	depth := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return names
		}
		if err != nil {
			t.Fatalf("read XML token: %v: %s", err, b)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 {
				names[el.Name.Local] = true
			}
		case xml.EndElement:
			depth--
		}
	}
}
