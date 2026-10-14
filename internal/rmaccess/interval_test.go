package rmaccess

// interval_test.go: interval bounds as the walks read them (REQ-112, with
// REQ-052 and REQ-056 for the encoders). The access reads an open side's
// bound as absent exactly when [rm.IsEmptyIntervalBound] says it is empty,
// every typed DV_INTERVAL instantiation is covered with both bounds read,
// and the canonical encoders omit an open side's bound by the same test.

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// typedInterval is one typed DV_INTERVAL instantiation: the RM name of its
// bound type, and the interval in the value and the pointer form.
type typedInterval struct {
	bound          string
	value, pointer any
}

// typed returns the typed DV_INTERVAL instantiation over T.
func typed[T rm.DVOrdered]() typedInterval {
	var zero T
	name, _ := rm.RMTypeName(zero)
	return typedInterval{bound: name, value: rm.DVInterval[T]{}, pointer: &rm.DVInterval[T]{}}
}

// typedIntervals lists every typed DV_INTERVAL instantiation. It is written
// out by hand because a DVInterval[T] value cannot be built from a registry
// name without reflection; TestAccess_REQ112_TypedIntervalParity ties it to
// the registry.
var typedIntervals = []typedInterval{
	typed[rm.DVCount](),
	typed[rm.DVDate](),
	typed[rm.DVDateTime](),
	typed[rm.DVDuration](),
	typed[rm.DVOrdinal](),
	typed[rm.DVProportion](),
	typed[rm.DVQuantity](),
	typed[rm.DVScale](),
	typed[rm.DVTime](),
}

// TestAccess_REQ112_TypedIntervalParity checks that the access covers every
// typed DV_INTERVAL instantiation and reads both its bounds (REQ-112): the
// floor walks into a typed interval only when the access covers it and gives
// back its bounds, so an instantiation left out leaves that interval's
// bounds unchecked. The cases must cover every DV_ORDERED descendant the live
// registry derives, in the value and the pointer form. A new descendant fails
// here until the list and the generated access both gain it.
func TestAccess_REQ112_TypedIntervalParity(t *testing.T) {
	var fromRegistry []string
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.DVOrdered); ok {
			fromRegistry = append(fromRegistry, name)
		}
	}
	if len(fromRegistry) == 0 {
		t.Fatal("registry yields no DVOrdered implementers; registrations missing?")
	}
	slices.Sort(fromRegistry)

	var covered []string
	for _, iv := range typedIntervals {
		covered = append(covered, iv.bound)
		for _, form := range []any{iv.value, iv.pointer} {
			if !Covers(form) {
				t.Errorf("Covers(%T) = false, want true: the floor stops at this interval", form)
			}
			for _, attr := range []string{"lower", "upper"} {
				got, ok := Single(form, attr)
				gotName, _ := rm.RMTypeName(got)
				if !ok || gotName != iv.bound {
					t.Errorf("Single(%T, %q) = (%T, %t), want a %s bound and true", form, attr, got, ok, iv.bound)
				}
			}
		}
	}
	slices.Sort(covered)
	if !slices.Equal(fromRegistry, covered) {
		t.Errorf("typed DV_INTERVAL drift:\n  registry-derived: %v\n  listed:           %v", fromRegistry, covered)
	}
}

// TestAccess_REQ112_IntervalOpenSide pins how the access reads an interval
// bound (REQ-112, REQ-102): a bounded side is present whatever its value; an
// open side (its `*_unbounded` flag set) is absent when its bound is empty,
// and present when a real bound stands beside the flag. Both the concrete and
// the bare (interface-typed) forms are covered.
func TestAccess_REQ112_IntervalOpenSide(t *testing.T) {
	q := rm.DVQuantity{Magnitude: 3.5, Units: "mm"}
	var nilQ *rm.DVQuantity
	cases := []struct {
		name     string
		interval any
		attr     string
		want     any // the bound read back; ignored when absent
		present  bool
	}{
		// Concrete form.
		{name: "concrete bounded", interval: &rm.DVInterval[rm.DVQuantity]{Lower: q, Upper: q}, attr: "lower", want: q, present: true},
		{name: "concrete bounded, zero bound", interval: &rm.DVInterval[rm.DVQuantity]{Upper: q}, attr: "lower", want: rm.DVQuantity{}, present: true},
		{name: "concrete open, empty bound", interval: &rm.DVInterval[rm.DVQuantity]{LowerUnbounded: true, Upper: q}, attr: "lower"},
		{name: "concrete open, real bound", interval: &rm.DVInterval[rm.DVQuantity]{Lower: q, LowerUnbounded: true}, attr: "lower", want: q, present: true},
		{name: "concrete open upper, empty bound", interval: &rm.DVInterval[rm.DVQuantity]{Lower: q, UpperUnbounded: true}, attr: "upper"},
		{name: "concrete open upper, real bound", interval: &rm.DVInterval[rm.DVQuantity]{Upper: q, UpperUnbounded: true}, attr: "upper", want: q, present: true},
		{name: "concrete value form, open, empty bound", interval: rm.DVInterval[rm.DVDate]{UpperUnbounded: true}, attr: "upper"},
		// Bare form.
		{name: "bare bounded", interval: &rm.DVInterval[rm.DVOrdered]{Lower: &q, Upper: &q}, attr: "lower", want: &q, present: true},
		{name: "bare bounded, nil bound", interval: &rm.DVInterval[rm.DVOrdered]{Upper: &q}, attr: "lower", want: nil, present: true},
		{name: "bare open, nil bound", interval: &rm.DVInterval[rm.DVOrdered]{LowerUnbounded: true, Upper: &q}, attr: "lower"},
		{name: "bare open, typed-nil bound", interval: &rm.DVInterval[rm.DVOrdered]{Lower: nilQ, LowerUnbounded: true}, attr: "lower"},
		{name: "bare open, zero concrete bound", interval: &rm.DVInterval[rm.DVOrdered]{Lower: rm.DVQuantity{}, LowerUnbounded: true}, attr: "lower", want: rm.DVQuantity{}, present: true},
		{name: "bare open, real bound", interval: &rm.DVInterval[rm.DVOrdered]{Lower: &q, LowerUnbounded: true}, attr: "lower", want: &q, present: true},
		{name: "bare open upper, nil bound", interval: &rm.DVInterval[rm.DVOrdered]{Lower: &q, UpperUnbounded: true}, attr: "upper"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, present := Single(tc.interval, tc.attr)
			if present != tc.present {
				t.Fatalf("Single(%T, %q) present = %t, want %t (value %#v)", tc.interval, tc.attr, present, tc.present, got)
			}
			if present && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Single(%T, %q) = %#v, want %#v", tc.interval, tc.attr, got, tc.want)
			}
		})
	}
}

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
	if got, want := len(cases), len(typedIntervals); got != want {
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
	if got, want := len(samples), 1+3*len(typedIntervals); got != want {
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

// TestIntervalBoundVoidPredicates guards the field-by-field Void test of
// each concrete bound type, [rm.IsEmptyIntervalBound] (REQ-112). The zero
// value must be Void, and a value with any one exported field set to
// non-zero, reached through struct-typed fields too such as DV_ORDINAL's
// `symbol`, must not be. The fields are enumerated by reflection, so a field
// added to an RM type later fails here until the generated predicate covers
// it. Without that, a real bound carrying only the new field would read as
// Void beside its `*_unbounded` flag and go unchecked.
func TestIntervalBoundVoidPredicates(t *testing.T) {
	cases := []struct {
		zero   any
		isVoid func(any) bool
	}{
		{rm.DVCount{}, func(v any) bool { return rm.IsEmptyIntervalBound(v.(rm.DVCount)) }},
		{rm.DVDate{}, func(v any) bool { return rm.IsEmptyIntervalBound(v.(rm.DVDate)) }},
		{rm.DVDateTime{}, func(v any) bool { return rm.IsEmptyIntervalBound(v.(rm.DVDateTime)) }},
		{rm.DVDuration{}, func(v any) bool { return rm.IsEmptyIntervalBound(v.(rm.DVDuration)) }},
		{rm.DVOrdinal{}, func(v any) bool { return rm.IsEmptyIntervalBound(v.(rm.DVOrdinal)) }},
		{rm.DVProportion{}, func(v any) bool { return rm.IsEmptyIntervalBound(v.(rm.DVProportion)) }},
		{rm.DVQuantity{}, func(v any) bool { return rm.IsEmptyIntervalBound(v.(rm.DVQuantity)) }},
		{rm.DVScale{}, func(v any) bool { return rm.IsEmptyIntervalBound(v.(rm.DVScale)) }},
		{rm.DVTime{}, func(v any) bool { return rm.IsEmptyIntervalBound(v.(rm.DVTime)) }},
	}
	if got, want := len(cases), len(typedIntervals); got != want {
		t.Fatalf("%d bound types guarded, want %d (one per typed DV_INTERVAL instantiation)", got, want)
	}
	for _, tc := range cases {
		typ := reflect.TypeOf(tc.zero)
		t.Run(typ.Name(), func(t *testing.T) {
			if !tc.isVoid(tc.zero) {
				t.Errorf("the zero %s reports non-Void, want Void", typ.Name())
			}
			for _, f := range leafFields(typ, nil, "") {
				v := reflect.New(typ).Elem()
				field := v.FieldByIndex(f.index)
				nz, err := nonZeroValue(field.Type())
				if err != nil {
					t.Fatalf("%s.%s: %v", typ.Name(), f.name, err)
				}
				field.Set(nz)
				if tc.isVoid(v.Interface()) {
					t.Errorf("%s with only %s set reports Void, want non-Void: the predicate misses that field", typ.Name(), f.name)
				}
			}
		})
	}
}

// leafField is one settable field of a bound type, reached through any
// struct-typed fields on the way.
type leafField struct {
	index []int
	name  string
}

// leafFields lists the exported fields of typ, descending into
// struct-typed (non-pointer) fields so a field of a nested struct counts
// on its own.
func leafFields(typ reflect.Type, prefix []int, prefixName string) []leafField {
	var out []leafField
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		index := append(append([]int(nil), prefix...), i)
		name := f.Name
		if prefixName != "" {
			name = prefixName + "." + f.Name
		}
		if f.Type.Kind() == reflect.Struct {
			out = append(out, leafFields(f.Type, index, name)...)
			continue
		}
		out = append(out, leafField{index: index, name: name})
	}
	return out
}

// nonZeroValue returns a value of type t that is not t's zero value. An
// interface-typed field takes the first registered RM concrete that
// implements it.
func nonZeroValue(t reflect.Type) (reflect.Value, error) {
	v := reflect.New(t).Elem()
	switch t.Kind() {
	case reflect.Pointer:
		return reflect.New(t.Elem()), nil
	case reflect.Slice:
		return reflect.MakeSlice(t, 1, 1), nil
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	case reflect.Interface:
		for _, name := range typereg.Default.Names() {
			ctor, _ := typereg.Default.Lookup(name)
			if c := reflect.ValueOf(ctor()); c.Type().Implements(t) {
				v.Set(c)
				return v, nil
			}
		}
		return v, fmt.Errorf("no registered RM concrete implements %s", t)
	default:
		return v, fmt.Errorf("no non-zero sample for kind %s (%s)", t.Kind(), strings.TrimSpace(t.String()))
	}
	return v, nil
}
