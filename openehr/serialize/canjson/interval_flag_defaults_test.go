package canjson_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// intervalFlags holds the four boundary flags of a decoded interval.
type intervalFlags struct {
	LowerIncluded, LowerUnbounded, UpperIncluded, UpperUnbounded bool
}

// closedFlags are the flags of a closed interval: both bounds included and
// neither side open.
var closedFlags = intervalFlags{LowerIncluded: true, UpperIncluded: true}

func baseFlags[T any](iv rm.Interval[T]) intervalFlags {
	return intervalFlags{
		LowerIncluded:  iv.LowerIncluded,
		LowerUnbounded: iv.LowerUnbounded,
		UpperIncluded:  iv.UpperIncluded,
		UpperUnbounded: iv.UpperUnbounded,
	}
}

// jsonMember is one member of an interval object. A value of "$lower" or
// "$upper" stands for the reader's spelling of that bound.
type jsonMember struct{ name, value string }

// jsonIntervalFlagCases are the interval objects every reader decodes, each
// with the flags REQ-169 says the decoded interval carries. The members come
// in the order listed, so a case can place a flag before or after the
// `*_unbounded` member its default depends on.
var jsonIntervalFlagCases = []struct {
	name    string
	members []jsonMember
	want    intervalFlags
}{
	{
		name:    "both bounds and no flags",
		members: []jsonMember{{"lower", "$lower"}, {"upper", "$upper"}},
		want:    closedFlags,
	},
	{
		name:    "both bounds, unbounded written false, no included (the upstream corpus shape)",
		members: []jsonMember{{"lower", "$lower"}, {"lower_unbounded", "false"}, {"upper", "$upper"}, {"upper_unbounded", "false"}},
		want:    closedFlags,
	},
	{
		name:    "open lower side and no included",
		members: []jsonMember{{"lower_unbounded", "true"}, {"upper", "$upper"}},
		want:    intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:    "open upper side and no included",
		members: []jsonMember{{"lower", "$lower"}, {"upper_unbounded", "true"}},
		want:    intervalFlags{LowerIncluded: true, UpperUnbounded: true},
	},
	{
		name:    "both sides open and no included",
		members: []jsonMember{{"lower_unbounded", "true"}, {"upper_unbounded", "true"}},
		want:    intervalFlags{LowerUnbounded: true, UpperUnbounded: true},
	},
	{
		name:    "open side marked after every other member",
		members: []jsonMember{{"upper_included", "true"}, {"upper", "$upper"}, {"lower_unbounded", "true"}},
		want:    intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:    "open side marked before every other member",
		members: []jsonMember{{"upper_unbounded", "true"}, {"lower", "$lower"}, {"lower_included", "false"}},
		want:    intervalFlags{UpperUnbounded: true},
	},
	{
		name:    "bounded sides written as excluded",
		members: []jsonMember{{"lower", "$lower"}, {"lower_included", "false"}, {"upper", "$upper"}, {"upper_included", "false"}},
		want:    intervalFlags{},
	},
	{
		name:    "one bounded side written as excluded, the other defaulted",
		members: []jsonMember{{"lower", "$lower"}, {"lower_included", "false"}, {"upper", "$upper"}},
		want:    intervalFlags{UpperIncluded: true},
	},
	{
		name:    "included open lower side, included before unbounded",
		members: []jsonMember{{"lower_included", "true"}, {"lower_unbounded", "true"}, {"upper", "$upper"}},
		want:    intervalFlags{LowerIncluded: true, LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:    "included open lower side, included after unbounded",
		members: []jsonMember{{"lower_unbounded", "true"}, {"lower_included", "true"}, {"upper", "$upper"}},
		want:    intervalFlags{LowerIncluded: true, LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:    "included open upper side",
		members: []jsonMember{{"lower", "$lower"}, {"upper_unbounded", "true"}, {"upper_included", "true"}},
		want:    intervalFlags{LowerIncluded: true, UpperIncluded: true, UpperUnbounded: true},
	},
	{
		name: "all four flags written",
		members: []jsonMember{
			{"lower", "$lower"},
			{"lower_included", "true"},
			{"lower_unbounded", "false"},
			{"upper", "$upper"},
			{"upper_included", "false"},
			{"upper_unbounded", "false"},
		},
		want: intervalFlags{LowerIncluded: true},
	},
	{
		name: "null flags read as absent",
		members: []jsonMember{
			{"lower", "$lower"},
			{"lower_included", "null"},
			{"lower_unbounded", "null"},
			{"upper", "$upper"},
			{"upper_included", "null"},
			{"upper_unbounded", "null"},
		},
		want: closedFlags,
	},
}

// jsonIntervalReader is one place a canonical-JSON document holds an
// interval: the document around the interval's members (at the %s verb), the
// spelling of the two bounds, and how to read the decoded flags back.
type jsonIntervalReader struct {
	name         string
	doc          string
	lower, upper string
	decode       func(doc []byte) (intervalFlags, error)
}

const (
	jsonCount1    = `{"_type":"DV_COUNT","magnitude":1}`
	jsonCount5    = `{"_type":"DV_COUNT","magnitude":5}`
	jsonQuantity1 = `{"_type":"DV_QUANTITY","magnitude":1,"units":"mm"}`
	jsonQuantity5 = `{"_type":"DV_QUANTITY","magnitude":5,"units":"mm"}`
)

func jsonIntervalReaders() []jsonIntervalReader {
	return []jsonIntervalReader{
		{
			name: "DV_INTERVAL<DV_COUNT>", doc: `{"_type":"DV_INTERVAL",%s}`, lower: jsonCount1, upper: jsonCount5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.DVInterval[rm.DVCount]
				err := canjson.Unmarshal(doc, &v)
				return baseFlags(v.Interval), err
			},
		},
		{
			name:  "DV_INTERVAL in ELEMENT.value",
			doc:   `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"x"},"value":{"_type":"DV_INTERVAL",%s}}`,
			lower: jsonCount1, upper: jsonCount5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.Element
				if err := canjson.Unmarshal(doc, &v); err != nil {
					return intervalFlags{}, err
				}
				iv, ok := v.Value.(*rm.DVInterval[rm.DVOrdered])
				if !ok || iv == nil {
					return intervalFlags{}, fmt.Errorf("ELEMENT.value decoded as %T, want *rm.DVInterval[rm.DVOrdered]", v.Value)
				}
				return baseFlags(iv.Interval), nil
			},
		},
		{
			name:  "REFERENCE_RANGE.range",
			doc:   `{"_type":"REFERENCE_RANGE","meaning":{"_type":"DV_TEXT","value":"normal"},"range":{"_type":"DV_INTERVAL",%s}}`,
			lower: jsonCount1, upper: jsonCount5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.ReferenceRange[rm.DVCount]
				err := canjson.Unmarshal(doc, &v)
				return baseFlags(v.Range.Interval), err
			},
		},
		{
			name:  "DV_QUANTITY.normal_range",
			doc:   `{"_type":"DV_QUANTITY","magnitude":3,"units":"mm","normal_range":{"_type":"DV_INTERVAL",%s}}`,
			lower: jsonQuantity1, upper: jsonQuantity5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.DVQuantity
				if err := canjson.Unmarshal(doc, &v); err != nil {
					return intervalFlags{}, err
				}
				if v.NormalRange == nil {
					return intervalFlags{}, fmt.Errorf("DV_QUANTITY.normal_range decoded as nil")
				}
				return baseFlags(v.NormalRange.Interval), nil
			},
		},
		{
			name: "Proper_interval<Integer>", doc: `{"_type":"Proper_interval",%s}`, lower: "1", upper: "5",
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.ProperInterval[rm.Integer]
				err := canjson.Unmarshal(doc, &v)
				return baseFlags(v.Interval), err
			},
		},
		{
			name: "Point_interval<Integer>", doc: `{"_type":"Point_interval",%s}`, lower: "3", upper: "3",
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.PointInterval[rm.Integer]
				err := canjson.Unmarshal(doc, &v)
				// Point_interval declares the four flags again, so its own
				// fields carry them.
				return intervalFlags{
					LowerIncluded: v.LowerIncluded, LowerUnbounded: v.LowerUnbounded,
					UpperIncluded: v.UpperIncluded, UpperUnbounded: v.UpperUnbounded,
				}, err
			},
		},
		{
			name: "BASE Interval held directly", doc: `{%s}`, lower: "1", upper: "5",
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.Interval[int]
				err := canjson.Unmarshal(doc, &v)
				return baseFlags(v), err
			},
		},
		{
			name:  "AOM 1.4 C_COMPLEX_OBJECT.occurrences",
			doc:   `{"_type":"C_COMPLEX_OBJECT","rm_type_name":"OBSERVATION","node_id":"at0000","occurrences":{%s}}`,
			lower: "0", upper: "5",
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CComplexObject
				err := canjson.Unmarshal(doc, &v)
				return baseFlags(v.Occurrences), err
			},
		},
		{
			name:  "AOM 1.4 C_SINGLE_ATTRIBUTE.existence",
			doc:   `{"_type":"C_SINGLE_ATTRIBUTE","rm_attribute_name":"items","existence":{%s}}`,
			lower: "0", upper: "1",
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CSingleAttribute
				err := canjson.Unmarshal(doc, &v)
				return baseFlags(v.Existence), err
			},
		},
		{
			name:  "AOM 1.4 C_MULTIPLE_ATTRIBUTE.cardinality.interval",
			doc:   `{"_type":"C_MULTIPLE_ATTRIBUTE","rm_attribute_name":"items","cardinality":{"_type":"CARDINALITY","is_ordered":true,"is_unique":false,"interval":{%s}}}`,
			lower: "0", upper: "9",
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CMultipleAttribute
				err := canjson.Unmarshal(doc, &v)
				return baseFlags(v.Cardinality.Interval), err
			},
		},
		{
			name: "AOM 1.4 C_INTEGER.range", doc: `{"_type":"C_INTEGER","range":{%s}}`, lower: "0", upper: "100",
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CInteger
				if err := canjson.Unmarshal(doc, &v); err != nil {
					return intervalFlags{}, err
				}
				if v.Range == nil {
					return intervalFlags{}, fmt.Errorf("C_INTEGER.range decoded as nil")
				}
				return baseFlags(*v.Range), nil
			},
		},
		{
			name: "AOM 1.4 C_REAL.range", doc: `{"_type":"C_REAL","range":{%s}}`, lower: "0.5", upper: "9.5",
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CReal
				if err := canjson.Unmarshal(doc, &v); err != nil {
					return intervalFlags{}, err
				}
				if v.Range == nil {
					return intervalFlags{}, fmt.Errorf("C_REAL.range decoded as nil")
				}
				return baseFlags(*v.Range), nil
			},
		},
		{
			name: "AOM 1.4 C_QUANTITY_ITEM.magnitude", doc: `{"_type":"C_QUANTITY_ITEM","magnitude":{%s},"units":"mm"}`, lower: "0", upper: "250",
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CQuantityItem
				err := canjson.Unmarshal(doc, &v)
				return baseFlags(v.Magnitude), err
			},
		},
	}
}

// jsonIntervalBody renders the members of one interval object.
func jsonIntervalBody(members []jsonMember, lower, upper string) string {
	parts := make([]string, 0, len(members))
	for _, m := range members {
		v := m.value
		switch v {
		case "$lower":
			v = lower
		case "$upper":
			v = upper
		}
		parts = append(parts, strconv.Quote(m.name)+":"+v)
	}
	return strings.Join(parts, ",")
}

// TestREQ169IntervalAbsentFlagDefaults pins how canonical JSON reads an
// interval boundary flag the document leaves out (REQ-169, REQ-052): an absent
// `*_unbounded` reads as false, and an absent `*_included` reads as true on a
// side not marked unbounded and as false on a side marked unbounded. A flag
// that is present is read as written, an included open side among them, and no
// absent flag is a decode error. The default for `*_included` follows its
// side's `*_unbounded` wherever that member sits in the object. Every reader
// is covered: DV_INTERVAL at a typed and at a polymorphic slot, REFERENCE_RANGE
// and a normal range, Proper_interval, Point_interval, BASE Interval held
// directly, and the AOM 1.4 constraint intervals.
//
// Can-fail control: drop the absent-included default and the cases with no
// `*_included` member read false; let the default override a written flag and
// the excluded and included-open-side cases fail.
func TestREQ169IntervalAbsentFlagDefaults(t *testing.T) {
	for _, r := range jsonIntervalReaders() {
		for _, tc := range jsonIntervalFlagCases {
			t.Run(r.name+"/"+tc.name, func(t *testing.T) {
				doc := fmt.Sprintf(r.doc, jsonIntervalBody(tc.members, r.lower, r.upper))
				got, err := r.decode([]byte(doc))
				if err != nil {
					t.Fatalf("canjson.Unmarshal(%s) = %v, want no error: an absent flag is never a decode error", doc, err)
				}
				if got != tc.want {
					t.Errorf("canjson.Unmarshal(%s) flags = %+v, want %+v", doc, got, tc.want)
				}
			})
		}
	}
}

// TestREQ169IntervalAbsentFlagsOverwriteReusedReceiver pins that the defaults
// of REQ-169 are what the document says, not what the receiver held: an absent
// `*_unbounded` reads as false and an absent `*_included` as true even when a
// value decoded into earlier set the opposite.
//
// Can-fail control: drop the absent-unbounded default and the open flags the
// receiver held survive the decode.
func TestREQ169IntervalAbsentFlagsOverwriteReusedReceiver(t *testing.T) {
	stale := intervalFlags{LowerUnbounded: true, UpperUnbounded: true}

	t.Run("DV_INTERVAL", func(t *testing.T) {
		v := rm.DVInterval[rm.DVCount]{Interval: rm.Interval[rm.DVCount]{LowerUnbounded: true, UpperUnbounded: true}}
		doc := `{"_type":"DV_INTERVAL","lower":` + jsonCount1 + `,"upper":` + jsonCount5 + `}`
		if err := canjson.Unmarshal([]byte(doc), &v); err != nil {
			t.Fatalf("canjson.Unmarshal: %v", err)
		}
		if got := baseFlags(v.Interval); got != closedFlags {
			t.Errorf("flags after decoding %s into a receiver holding %+v = %+v, want %+v", doc, stale, got, closedFlags)
		}
	})
	t.Run("Point_interval", func(t *testing.T) {
		v := rm.PointInterval[rm.Integer]{LowerUnbounded: true, UpperUnbounded: true}
		doc := `{"_type":"Point_interval","lower":3,"upper":3}`
		if err := canjson.Unmarshal([]byte(doc), &v); err != nil {
			t.Fatalf("canjson.Unmarshal: %v", err)
		}
		got := intervalFlags{LowerIncluded: v.LowerIncluded, LowerUnbounded: v.LowerUnbounded, UpperIncluded: v.UpperIncluded, UpperUnbounded: v.UpperUnbounded}
		if got != closedFlags {
			t.Errorf("flags after decoding %s into a receiver holding %+v = %+v, want %+v", doc, stale, got, closedFlags)
		}
	})
	t.Run("AOM 1.4 occurrences", func(t *testing.T) {
		v := aom14.CComplexObject{Occurrences: rm.Interval[aom14.Integer]{LowerUnbounded: true, UpperUnbounded: true}}
		doc := `{"_type":"C_COMPLEX_OBJECT","rm_type_name":"OBSERVATION","node_id":"at0000","occurrences":{"lower":1,"upper":1}}`
		if err := canjson.Unmarshal([]byte(doc), &v); err != nil {
			t.Fatalf("canjson.Unmarshal: %v", err)
		}
		if got := baseFlags(v.Occurrences); got != closedFlags {
			t.Errorf("flags after decoding %s into a receiver holding %+v = %+v, want %+v", doc, stale, got, closedFlags)
		}
	})
}

// dvIntervalFlags walks a decoded RM value and returns the flags of every
// DV_INTERVAL it holds, in walk order. reflect is fine here: the
// no-reflection rule binds library code, not tests.
func dvIntervalFlags(v any) []intervalFlags {
	rmPkg := reflect.TypeFor[rm.Element]().PkgPath()
	var out []intervalFlags
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem())
			}
		case reflect.Struct:
			if t := rv.Type(); t.PkgPath() == rmPkg && strings.HasPrefix(t.Name(), "DVInterval[") {
				out = append(out, intervalFlags{
					LowerIncluded:  rv.FieldByName("LowerIncluded").Bool(),
					LowerUnbounded: rv.FieldByName("LowerUnbounded").Bool(),
					UpperIncluded:  rv.FieldByName("UpperIncluded").Bool(),
					UpperUnbounded: rv.FieldByName("UpperUnbounded").Bool(),
				})
			}
			for i := range rv.NumField() {
				walk(rv.Field(i))
			}
		case reflect.Slice, reflect.Array:
			for i := range rv.Len() {
				walk(rv.Index(i))
			}
		case reflect.Map:
			for it := rv.MapRange(); it.Next(); {
				walk(it.Value())
			}
		}
	}
	walk(reflect.ValueOf(v))
	return out
}

// TestREQ169CorpusIntervalsDecodeClosed decodes the upstream canonical-JSON
// documents whose DV_INTERVAL values give both bounds and leave out
// `lower_included` and `upper_included`, 25 in all, and expects every one of
// them closed (REQ-169, REQ-052; PROBE-105 meets the same upstream data).
// Each file holds no other DV_INTERVAL, so the count pins that the walk saw
// them all.
func TestREQ169CorpusIntervalsDecodeClosed(t *testing.T) {
	for _, tc := range []struct {
		rel  string
		into func() any
		want int
	}{
		{rel: "compositions/Demonstration.v1.json", into: func() any { return new(rm.Composition) }, want: 15},
		{rel: "compositions/TestPerson.v2.json", into: func() any { return new(rm.Person) }, want: 7},
		{rel: "crossformat/test_all_types/canonical.json", into: func() any { return new(rm.Composition) }, want: 3},
	} {
		t.Run(tc.rel, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), tc.rel))
			if err != nil {
				t.Fatalf("read %s: %v", tc.rel, err)
			}
			v := tc.into()
			if err := canjson.Unmarshal(data, v); err != nil {
				t.Fatalf("canjson.Unmarshal(%s): %v", tc.rel, err)
			}
			got := dvIntervalFlags(v)
			if len(got) != tc.want {
				t.Fatalf("%s holds %d DV_INTERVAL values, want %d", tc.rel, len(got), tc.want)
			}
			for i, f := range got {
				if f != closedFlags {
					t.Errorf("%s: DV_INTERVAL #%d flags = %+v, want %+v (both bounds given, no included member)", tc.rel, i, f, closedFlags)
				}
			}
		})
	}
}
