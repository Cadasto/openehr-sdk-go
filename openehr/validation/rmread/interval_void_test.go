package rmread

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestIntervalBoundVoidPredicates guards the field-by-field Void test of
// each concrete bound type (REQ-112). The zero value must be Void, and a
// value with any one exported field set to non-zero — reached through
// struct-typed fields too, such as DV_ORDINAL's `symbol` — must not be.
// The fields are enumerated by reflection, so a field added to an RM type
// later fails here until its predicate covers it. Without that, a real
// bound carrying only the new field would read as Void beside its
// `*_unbounded` flag and go unchecked.
func TestIntervalBoundVoidPredicates(t *testing.T) {
	cases := []struct {
		zero   any
		isVoid func(any) bool
	}{
		{rm.DVCount{}, func(v any) bool { return isVoidDVCount(v.(rm.DVCount)) }},
		{rm.DVDate{}, func(v any) bool { return isVoidDVDate(v.(rm.DVDate)) }},
		{rm.DVDateTime{}, func(v any) bool { return isVoidDVDateTime(v.(rm.DVDateTime)) }},
		{rm.DVDuration{}, func(v any) bool { return isVoidDVDuration(v.(rm.DVDuration)) }},
		{rm.DVOrdinal{}, func(v any) bool { return isVoidDVOrdinal(v.(rm.DVOrdinal)) }},
		{rm.DVProportion{}, func(v any) bool { return isVoidDVProportion(v.(rm.DVProportion)) }},
		{rm.DVQuantity{}, func(v any) bool { return isVoidDVQuantity(v.(rm.DVQuantity)) }},
		{rm.DVScale{}, func(v any) bool { return isVoidDVScale(v.(rm.DVScale)) }},
		{rm.DVTime{}, func(v any) bool { return isVoidDVTime(v.(rm.DVTime)) }},
	}
	if got, want := len(cases), len(typedIntervals)/2; got != want {
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

// TestIntervalBoundVoidOrdered pins the Void test of the bare form's
// interface-typed bound (REQ-112): only a nil or typed-nil interface is
// Void. A zero-valued concrete behind the interface is a value, as wire.md
// reads an interface-typed bound.
func TestIntervalBoundVoidOrdered(t *testing.T) {
	cases := []struct {
		name string
		v    rm.DVOrdered
		want bool
	}{
		{name: "nil interface", v: nil, want: true},
		{name: "typed nil", v: (*rm.DVQuantity)(nil), want: true},
		{name: "zero concrete value", v: rm.DVQuantity{}, want: false},
		{name: "zero concrete pointer", v: &rm.DVQuantity{}, want: false},
		{name: "real bound", v: &rm.DVCount{Magnitude: 3}, want: false},
	}
	for _, tc := range cases {
		if got := isVoidOrdered(tc.v); got != tc.want {
			t.Errorf("isVoidOrdered(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestReadSingle_IntervalOpenSide pins how the interval readers report a
// bound (REQ-112, REQ-102): a bounded side is present whatever its value;
// an open side (its `*_unbounded` flag set) is absent when its bound is
// Void, and present when a real bound stands beside the flag. Both the
// concrete and the bare (interface-typed) forms are covered.
func TestReadSingle_IntervalOpenSide(t *testing.T) {
	q := rm.DVQuantity{Magnitude: 3.5, Units: "mm"}
	var nilQ *rm.DVQuantity
	cases := []struct {
		name     string
		interval any
		attr     string
		want     any // the bound read back; ignored when absent
		wantOK   bool
	}{
		// Concrete form.
		{name: "concrete bounded", interval: &rm.DVInterval[rm.DVQuantity]{Lower: q, Upper: q}, attr: "lower", want: q, wantOK: true},
		{name: "concrete bounded, zero bound", interval: &rm.DVInterval[rm.DVQuantity]{Upper: q}, attr: "lower", want: rm.DVQuantity{}, wantOK: true},
		{name: "concrete open, Void bound", interval: &rm.DVInterval[rm.DVQuantity]{LowerUnbounded: true, Upper: q}, attr: "lower", wantOK: false},
		{name: "concrete open, real bound", interval: &rm.DVInterval[rm.DVQuantity]{Lower: q, LowerUnbounded: true}, attr: "lower", want: q, wantOK: true},
		{name: "concrete open upper, Void bound", interval: &rm.DVInterval[rm.DVQuantity]{Lower: q, UpperUnbounded: true}, attr: "upper", wantOK: false},
		{name: "concrete open upper, real bound", interval: &rm.DVInterval[rm.DVQuantity]{Upper: q, UpperUnbounded: true}, attr: "upper", want: q, wantOK: true},
		{name: "concrete value form, open, Void bound", interval: rm.DVInterval[rm.DVDate]{UpperUnbounded: true}, attr: "upper", wantOK: false},
		// Bare form.
		{name: "bare bounded", interval: &rm.DVInterval[rm.DVOrdered]{Lower: &q, Upper: &q}, attr: "lower", want: &q, wantOK: true},
		{name: "bare bounded, nil bound", interval: &rm.DVInterval[rm.DVOrdered]{Upper: &q}, attr: "lower", want: nil, wantOK: true},
		{name: "bare open, nil bound", interval: &rm.DVInterval[rm.DVOrdered]{LowerUnbounded: true, Upper: &q}, attr: "lower", wantOK: false},
		{name: "bare open, typed-nil bound", interval: &rm.DVInterval[rm.DVOrdered]{Lower: nilQ, LowerUnbounded: true}, attr: "lower", wantOK: false},
		{name: "bare open, zero concrete bound", interval: &rm.DVInterval[rm.DVOrdered]{Lower: rm.DVQuantity{}, LowerUnbounded: true}, attr: "lower", want: rm.DVQuantity{}, wantOK: true},
		{name: "bare open, real bound", interval: &rm.DVInterval[rm.DVOrdered]{Lower: &q, LowerUnbounded: true}, attr: "lower", want: &q, wantOK: true},
		{name: "bare open upper, nil bound", interval: &rm.DVInterval[rm.DVOrdered]{Lower: &q, UpperUnbounded: true}, attr: "upper", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ReadSingle(tc.interval, "DV_INTERVAL", tc.attr)
			if ok != tc.wantOK {
				t.Fatalf("ReadSingle(%T, %q) ok = %v, want %v (value %#v)", tc.interval, tc.attr, ok, tc.wantOK, got)
			}
			if ok && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ReadSingle(%T, %q) = %#v, want %#v", tc.interval, tc.attr, got, tc.want)
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
