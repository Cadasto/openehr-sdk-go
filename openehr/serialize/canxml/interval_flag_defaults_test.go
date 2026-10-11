package canxml_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
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

// xmlChild is one child element of an interval element. A value of "$lower"
// or "$upper" stands for the reader's whole element for that bound.
type xmlChild struct{ name, value string }

// xmlIntervalFlagCases are the interval elements every reader decodes, each
// with the flags REQ-169 says the decoded interval carries. The children come
// in the order listed, so a case can place a flag before or after the
// `*_unbounded` element its default depends on.
var xmlIntervalFlagCases = []struct {
	name     string
	children []xmlChild
	want     intervalFlags
}{
	{
		name:     "both bounds and no flags",
		children: []xmlChild{{"lower", "$lower"}, {"upper", "$upper"}},
		want:     closedFlags,
	},
	{
		name:     "both bounds, unbounded written false, no included (the upstream corpus shape)",
		children: []xmlChild{{"lower", "$lower"}, {"upper", "$upper"}, {"lower_unbounded", "false"}, {"upper_unbounded", "false"}},
		want:     closedFlags,
	},
	{
		name:     "open lower side and no included",
		children: []xmlChild{{"upper", "$upper"}, {"lower_unbounded", "true"}},
		want:     intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:     "open upper side and no included",
		children: []xmlChild{{"lower", "$lower"}, {"upper_unbounded", "true"}},
		want:     intervalFlags{LowerIncluded: true, UpperUnbounded: true},
	},
	{
		name:     "both sides open and no included",
		children: []xmlChild{{"lower_unbounded", "true"}, {"upper_unbounded", "true"}},
		want:     intervalFlags{LowerUnbounded: true, UpperUnbounded: true},
	},
	{
		name:     "open side marked after every other element",
		children: []xmlChild{{"upper_included", "true"}, {"upper", "$upper"}, {"lower_unbounded", "true"}},
		want:     intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:     "open side marked before every other element",
		children: []xmlChild{{"upper_unbounded", "true"}, {"lower", "$lower"}, {"lower_included", "false"}},
		want:     intervalFlags{UpperUnbounded: true},
	},
	{
		name:     "bounded sides written as excluded",
		children: []xmlChild{{"lower", "$lower"}, {"upper", "$upper"}, {"lower_included", "false"}, {"upper_included", "false"}},
		want:     intervalFlags{},
	},
	{
		name:     "one bounded side written as excluded, the other defaulted",
		children: []xmlChild{{"lower", "$lower"}, {"upper", "$upper"}, {"lower_included", "false"}},
		want:     intervalFlags{UpperIncluded: true},
	},
	{
		name:     "included open lower side, included before unbounded",
		children: []xmlChild{{"upper", "$upper"}, {"lower_included", "true"}, {"lower_unbounded", "true"}},
		want:     intervalFlags{LowerIncluded: true, LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:     "included open lower side, included after unbounded",
		children: []xmlChild{{"upper", "$upper"}, {"lower_unbounded", "true"}, {"lower_included", "true"}},
		want:     intervalFlags{LowerIncluded: true, LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name:     "included open upper side",
		children: []xmlChild{{"lower", "$lower"}, {"upper_unbounded", "true"}, {"upper_included", "true"}},
		want:     intervalFlags{LowerIncluded: true, UpperIncluded: true, UpperUnbounded: true},
	},
	{
		name: "all four flags written",
		children: []xmlChild{
			{"lower", "$lower"},
			{"upper", "$upper"},
			{"lower_unbounded", "false"},
			{"upper_unbounded", "false"},
			{"lower_included", "true"},
			{"upper_included", "false"},
		},
		want: intervalFlags{LowerIncluded: true},
	},
}

// xmlIntervalReader is one place a canonical-XML document holds an interval:
// the document around the interval's children (at the %s verb), the two bound
// elements, and how to read the decoded flags back.
type xmlIntervalReader struct {
	name         string
	doc          string
	lower, upper string
	decode       func(doc []byte) (intervalFlags, error)
}

const (
	xmlNS        = ` xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`
	xmlCount1    = `<lower xsi:type="DV_COUNT"><magnitude>1</magnitude></lower>`
	xmlCount5    = `<upper xsi:type="DV_COUNT"><magnitude>5</magnitude></upper>`
	xmlQuantity1 = `<lower xsi:type="DV_QUANTITY"><magnitude>1</magnitude><units>mm</units></lower>`
	xmlQuantity5 = `<upper xsi:type="DV_QUANTITY"><magnitude>5</magnitude><units>mm</units></upper>`
	xmlIntLower0 = `<lower>0</lower>`
	xmlIntUpper5 = `<upper>5</upper>`
	xmlIntLower3 = `<lower>3</lower>`
	xmlIntUpper3 = `<upper>3</upper>`
	xmlRealLower = `<lower>0.5</lower>`
	xmlRealUpper = `<upper>9.5</upper>`
)

func xmlIntervalReaders() []xmlIntervalReader {
	return []xmlIntervalReader{
		{
			name: "DV_INTERVAL<DV_COUNT>", doc: `<dv_interval` + xmlNS + `>%s</dv_interval>`, lower: xmlCount1, upper: xmlCount5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.DVInterval[rm.DVCount]
				err := canxml.Unmarshal(doc, &v)
				return baseFlags(v.Interval), err
			},
		},
		{
			name:  "DV_INTERVAL in ELEMENT.value",
			doc:   `<element` + xmlNS + ` archetype_node_id="at0001"><name><value>x</value></name><value xsi:type="DV_INTERVAL">%s</value></element>`,
			lower: xmlCount1, upper: xmlCount5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.Element
				if err := canxml.Unmarshal(doc, &v); err != nil {
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
			doc:   `<reference_range` + xmlNS + `><meaning><value>normal</value></meaning><range>%s</range></reference_range>`,
			lower: xmlCount1, upper: xmlCount5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.ReferenceRange[rm.DVCount]
				err := canxml.Unmarshal(doc, &v)
				return baseFlags(v.Range.Interval), err
			},
		},
		{
			name:  "DV_QUANTITY.normal_range",
			doc:   `<dv_quantity` + xmlNS + `><normal_range>%s</normal_range><magnitude>3</magnitude><units>mm</units></dv_quantity>`,
			lower: xmlQuantity1, upper: xmlQuantity5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.DVQuantity
				if err := canxml.Unmarshal(doc, &v); err != nil {
					return intervalFlags{}, err
				}
				if v.NormalRange == nil {
					return intervalFlags{}, errors.New("DV_QUANTITY.normal_range decoded as nil")
				}
				return baseFlags(v.NormalRange.Interval), nil
			},
		},
		{
			name: "Proper_interval<Integer>", doc: `<proper_interval` + xmlNS + `>%s</proper_interval>`, lower: xmlIntLower0, upper: xmlIntUpper5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.ProperInterval[rm.Integer]
				err := canxml.Unmarshal(doc, &v)
				return baseFlags(v.Interval), err
			},
		},
		{
			name: "Point_interval<Integer>", doc: `<point_interval` + xmlNS + `>%s</point_interval>`, lower: xmlIntLower3, upper: xmlIntUpper3,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.PointInterval[rm.Integer]
				err := canxml.Unmarshal(doc, &v)
				// Point_interval declares the four flags again, so its own
				// fields carry them.
				return intervalFlags{
					LowerIncluded: v.LowerIncluded, LowerUnbounded: v.LowerUnbounded,
					UpperIncluded: v.UpperIncluded, UpperUnbounded: v.UpperUnbounded,
				}, err
			},
		},
		{
			name: "BASE Interval held directly", doc: `<interval` + xmlNS + `>%s</interval>`, lower: xmlIntLower0, upper: xmlIntUpper5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v rm.Interval[int]
				err := canxml.Unmarshal(doc, &v)
				return baseFlags(v), err
			},
		},
		{
			name:  "AOM 1.4 C_COMPLEX_OBJECT.occurrences",
			doc:   `<c_complex_object` + xmlNS + `><rm_type_name>OBSERVATION</rm_type_name><occurrences>%s</occurrences><node_id>at0000</node_id></c_complex_object>`,
			lower: xmlIntLower0, upper: xmlIntUpper5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CComplexObject
				err := canxml.Unmarshal(doc, &v)
				return baseFlags(v.Occurrences), err
			},
		},
		{
			name:  "AOM 1.4 C_SINGLE_ATTRIBUTE.existence",
			doc:   `<c_single_attribute` + xmlNS + `><rm_attribute_name>items</rm_attribute_name><existence>%s</existence></c_single_attribute>`,
			lower: xmlIntLower0, upper: xmlIntUpper5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CSingleAttribute
				err := canxml.Unmarshal(doc, &v)
				return baseFlags(v.Existence), err
			},
		},
		{
			name: "AOM 1.4 C_MULTIPLE_ATTRIBUTE.cardinality.interval",
			doc: `<c_multiple_attribute` + xmlNS + `><rm_attribute_name>items</rm_attribute_name><cardinality>` +
				`<is_ordered>true</is_ordered><is_unique>false</is_unique><interval>%s</interval></cardinality></c_multiple_attribute>`,
			lower: xmlIntLower0, upper: xmlIntUpper5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CMultipleAttribute
				err := canxml.Unmarshal(doc, &v)
				return baseFlags(v.Cardinality.Interval), err
			},
		},
		{
			name: "AOM 1.4 C_INTEGER.range", doc: `<c_integer` + xmlNS + `><range>%s</range></c_integer>`, lower: xmlIntLower0, upper: xmlIntUpper5,
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CInteger
				if err := canxml.Unmarshal(doc, &v); err != nil {
					return intervalFlags{}, err
				}
				if v.Range == nil {
					return intervalFlags{}, errors.New("C_INTEGER.range decoded as nil")
				}
				return baseFlags(*v.Range), nil
			},
		},
		{
			name: "AOM 1.4 C_REAL.range", doc: `<c_real` + xmlNS + `><range>%s</range></c_real>`, lower: xmlRealLower, upper: xmlRealUpper,
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CReal
				if err := canxml.Unmarshal(doc, &v); err != nil {
					return intervalFlags{}, err
				}
				if v.Range == nil {
					return intervalFlags{}, errors.New("C_REAL.range decoded as nil")
				}
				return baseFlags(*v.Range), nil
			},
		},
		{
			name:  "AOM 1.4 C_QUANTITY_ITEM.magnitude",
			doc:   `<c_quantity_item` + xmlNS + `><magnitude>%s</magnitude><units>mm</units></c_quantity_item>`,
			lower: xmlRealLower, upper: xmlRealUpper,
			decode: func(doc []byte) (intervalFlags, error) {
				var v aom14.CQuantityItem
				err := canxml.Unmarshal(doc, &v)
				return baseFlags(v.Magnitude), err
			},
		},
	}
}

// xmlIntervalBody renders the children of one interval element.
func xmlIntervalBody(children []xmlChild, lower, upper string) string {
	var b strings.Builder
	for _, c := range children {
		switch c.value {
		case "$lower":
			b.WriteString(lower)
		case "$upper":
			b.WriteString(upper)
		default:
			fmt.Fprintf(&b, "<%s>%s</%s>", c.name, c.value, c.name)
		}
	}
	return b.String()
}

// TestREQ169IntervalAbsentFlagDefaults pins how canonical XML reads an
// interval boundary flag the document leaves out (REQ-169, REQ-056): an absent
// `*_unbounded` reads as false, and an absent `*_included` reads as true on a
// side not marked unbounded and as false on a side marked unbounded. A flag
// that is present is read as written, an included open side among them, and no
// absent flag is a decode error. The default for `*_included` follows its
// side's `*_unbounded` wherever that element sits among the children. Every
// reader is covered: DV_INTERVAL at a typed and at a polymorphic slot,
// REFERENCE_RANGE and a normal range, Proper_interval, Point_interval, BASE
// Interval held directly, and the AOM 1.4 constraint intervals.
//
// Can-fail control: drop the absent-included default and the cases with no
// `*_included` element read false; let the default override a written flag and
// the excluded and included-open-side cases fail.
func TestREQ169IntervalAbsentFlagDefaults(t *testing.T) {
	for _, r := range xmlIntervalReaders() {
		for _, tc := range xmlIntervalFlagCases {
			t.Run(r.name+"/"+tc.name, func(t *testing.T) {
				doc := fmt.Sprintf(r.doc, xmlIntervalBody(tc.children, r.lower, r.upper))
				got, err := r.decode([]byte(doc))
				if err != nil {
					t.Fatalf("canxml.Unmarshal(%s) = %v, want no error: an absent flag is never a decode error", doc, err)
				}
				if got != tc.want {
					t.Errorf("canxml.Unmarshal(%s) flags = %+v, want %+v", doc, got, tc.want)
				}
			})
		}
	}
}

// TestREQ169IntervalLegacyXMLFlagNamesCountAsPresent pins that a flag written
// in the Go field spelling SDK v0.28.0 and earlier used for BASE Interval
// (REQ-056) is a present flag under REQ-169: it is read as written, and only
// the flags the document leaves out in both spellings take a default.
func TestREQ169IntervalLegacyXMLFlagNamesCountAsPresent(t *testing.T) {
	doc := `<c_complex_object` + xmlNS + `><rm_type_name>OBSERVATION</rm_type_name><occurrences>` +
		`<Lower>1</Lower><Upper>5</Upper><LowerIncluded>false</LowerIncluded><UpperUnbounded>false</UpperUnbounded>` +
		`</occurrences><node_id>at0000</node_id></c_complex_object>`
	var v aom14.CComplexObject
	if err := canxml.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("canxml.Unmarshal(%s): %v", doc, err)
	}
	want := intervalFlags{UpperIncluded: true}
	if got := baseFlags(v.Occurrences); got != want {
		t.Errorf("canxml.Unmarshal(%s) flags = %+v, want %+v", doc, got, want)
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
		v := rm.DVInterval[rm.DVCount]{LowerUnbounded: true, UpperUnbounded: true}
		doc := `<dv_interval` + xmlNS + `>` + xmlCount1 + xmlCount5 + `</dv_interval>`
		if err := canxml.Unmarshal([]byte(doc), &v); err != nil {
			t.Fatalf("canxml.Unmarshal: %v", err)
		}
		if got := baseFlags(v.Interval); got != closedFlags {
			t.Errorf("flags after decoding %s into a receiver holding %+v = %+v, want %+v", doc, stale, got, closedFlags)
		}
	})
	t.Run("Point_interval", func(t *testing.T) {
		v := rm.PointInterval[rm.Integer]{LowerUnbounded: true, UpperUnbounded: true}
		doc := `<point_interval` + xmlNS + `>` + xmlIntLower3 + xmlIntUpper3 + `</point_interval>`
		if err := canxml.Unmarshal([]byte(doc), &v); err != nil {
			t.Fatalf("canxml.Unmarshal: %v", err)
		}
		got := intervalFlags{LowerIncluded: v.LowerIncluded, LowerUnbounded: v.LowerUnbounded, UpperIncluded: v.UpperIncluded, UpperUnbounded: v.UpperUnbounded}
		if got != closedFlags {
			t.Errorf("flags after decoding %s into a receiver holding %+v = %+v, want %+v", doc, stale, got, closedFlags)
		}
	})
	t.Run("AOM 1.4 occurrences", func(t *testing.T) {
		v := aom14.CComplexObject{Occurrences: rm.Interval[aom14.Integer]{LowerUnbounded: true, UpperUnbounded: true}}
		doc := `<c_complex_object` + xmlNS + `><rm_type_name>OBSERVATION</rm_type_name><occurrences>` +
			`<lower>1</lower><upper>1</upper></occurrences><node_id>at0000</node_id></c_complex_object>`
		if err := canxml.Unmarshal([]byte(doc), &v); err != nil {
			t.Fatalf("canxml.Unmarshal: %v", err)
		}
		if got := baseFlags(v.Occurrences); got != closedFlags {
			t.Errorf("flags after decoding %s into a receiver holding %+v = %+v, want %+v", doc, stale, got, closedFlags)
		}
	})
}

// corpusDVIntervals returns the inner XML of every element of an XML document
// that carries xsi:type="DV_INTERVAL", in document order.
func corpusDVIntervals(t *testing.T, data []byte) []string {
	t.Helper()
	const xsiNS = "http://www.w3.org/2001/XMLSchema-instance"
	dec := xml.NewDecoder(bytes.NewReader(data))
	var out []string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("read XML token: %v", err)
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		isInterval := slices.ContainsFunc(el.Attr, func(a xml.Attr) bool {
			return a.Name.Space == xsiNS && a.Name.Local == "type" && a.Value == "DV_INTERVAL"
		})
		if !isInterval {
			continue
		}
		var raw struct {
			Inner string `xml:",innerxml"`
		}
		if err := dec.DecodeElement(&raw, &el); err != nil {
			t.Fatalf("read DV_INTERVAL element: %v", err)
		}
		out = append(out, raw.Inner)
	}
}

// TestREQ169CorpusIntervalsDecodeClosed decodes the DV_INTERVAL values of the
// upstream canonical-XML documents that give both bounds and leave out
// `lower_included` and `upper_included`, 25 in all, and expects every one of
// them closed (REQ-169, REQ-056). Each file holds no other DV_INTERVAL, so the
// count pins that the walk saw them all. Each interval is decoded on its own,
// as written in the file, because two of the documents also carry a
// DV_MULTIMEDIA the XML codec does not decode.
func TestREQ169CorpusIntervalsDecodeClosed(t *testing.T) {
	for _, tc := range []struct {
		rel  string
		want int
	}{
		{rel: "compositions/Demonstration.v1.xml", want: 15},
		{rel: "compositions/TestPerson.v2.xml", want: 7},
		{rel: "rm/test_all_types.v1.xml", want: 3},
	} {
		t.Run(tc.rel, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), tc.rel))
			if err != nil {
				t.Fatalf("read %s: %v", tc.rel, err)
			}
			inners := corpusDVIntervals(t, data)
			if len(inners) != tc.want {
				t.Fatalf("%s holds %d DV_INTERVAL values, want %d", tc.rel, len(inners), tc.want)
			}
			for i, inner := range inners {
				doc := `<dv_interval` + xmlNS + `>` + inner + `</dv_interval>`
				var v rm.DVInterval[rm.DVOrdered]
				if err := canxml.Unmarshal([]byte(doc), &v); err != nil {
					t.Fatalf("%s: DV_INTERVAL #%d: canxml.Unmarshal(%s): %v", tc.rel, i, doc, err)
				}
				if got := baseFlags(v.Interval); got != closedFlags {
					t.Errorf("%s: DV_INTERVAL #%d flags = %+v, want %+v (both bounds given, no included element): %s", tc.rel, i, got, closedFlags, inner)
				}
			}
		})
	}
}

// xmlEmptyFlagCases place one flag element with no boolean in it ("$empty")
// on a bounded side and beside an open side, each with the flags the decoded
// interval carries when that element reads as a flag the document leaves out.
var xmlEmptyFlagCases = []struct {
	name     string
	flag     string
	children []xmlChild
	want     intervalFlags
}{
	{
		name: "lower_included on a bounded side", flag: "lower_included",
		children: []xmlChild{{"lower", "$lower"}, {"upper", "$upper"}, {"lower_included", "$empty"}},
		want:     closedFlags,
	},
	{
		name: "lower_included on an open side", flag: "lower_included",
		children: []xmlChild{{"upper", "$upper"}, {"lower_unbounded", "true"}, {"lower_included", "$empty"}},
		want:     intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
	{
		name: "upper_included on a bounded side", flag: "upper_included",
		children: []xmlChild{{"lower", "$lower"}, {"upper", "$upper"}, {"upper_included", "$empty"}},
		want:     closedFlags,
	},
	{
		name: "upper_included on an open side", flag: "upper_included",
		children: []xmlChild{{"lower", "$lower"}, {"upper_unbounded", "true"}, {"upper_included", "$empty"}},
		want:     intervalFlags{LowerIncluded: true, UpperUnbounded: true},
	},
	{
		name: "lower_unbounded on a bounded side", flag: "lower_unbounded",
		children: []xmlChild{{"lower", "$lower"}, {"upper", "$upper"}, {"lower_unbounded", "$empty"}},
		want:     closedFlags,
	},
	{
		name: "lower_unbounded beside an open upper side", flag: "lower_unbounded",
		children: []xmlChild{{"lower", "$lower"}, {"lower_unbounded", "$empty"}, {"upper_unbounded", "true"}},
		want:     intervalFlags{LowerIncluded: true, UpperUnbounded: true},
	},
	{
		name: "upper_unbounded on a bounded side", flag: "upper_unbounded",
		children: []xmlChild{{"lower", "$lower"}, {"upper", "$upper"}, {"upper_unbounded", "$empty"}},
		want:     closedFlags,
	},
	{
		name: "upper_unbounded beside an open lower side", flag: "upper_unbounded",
		children: []xmlChild{{"lower_unbounded", "true"}, {"upper", "$upper"}, {"upper_unbounded", "$empty"}},
		want:     intervalFlags{LowerUnbounded: true, UpperIncluded: true},
	},
}

// legacyFlagName is the Go field spelling SDK v0.28.0 and earlier wrote for
// each BASE Interval flag.
var legacyFlagName = map[string]string{
	"lower_included":  "LowerIncluded",
	"lower_unbounded": "LowerUnbounded",
	"upper_included":  "UpperIncluded",
	"upper_unbounded": "UpperUnbounded",
}

// xmlEmptyFlagSpellings are the three ways an element can carry no boolean.
var xmlEmptyFlagSpellings = []struct {
	name string
	elem func(name string) string
}{
	{"self-closing", func(n string) string { return "<" + n + "/>" }},
	{"empty", func(n string) string { return "<" + n + "></" + n + ">" }},
	{"whitespace-only", func(n string) string { return "<" + n + "> \n\t </" + n + ">" }},
}

// xmlEmptyFlagReaders are the readers the empty-flag rule is checked on:
// DV_INTERVAL, BASE Interval and two AOM 1.4 constraint intervals. legacy
// marks the ones decoded by BASE Interval, which also reads the Go field
// names SDK v0.28.0 and earlier wrote.
func xmlEmptyFlagReaders(t *testing.T) []struct {
	r      xmlIntervalReader
	legacy bool
} {
	t.Helper()
	want := map[string]bool{
		"DV_INTERVAL<DV_COUNT>":                false,
		"BASE Interval held directly":          true,
		"AOM 1.4 C_INTEGER.range":              true,
		"AOM 1.4 C_COMPLEX_OBJECT.occurrences": true,
	}
	var out []struct {
		r      xmlIntervalReader
		legacy bool
	}
	for _, r := range xmlIntervalReaders() {
		if legacy, ok := want[r.name]; ok {
			out = append(out, struct {
				r      xmlIntervalReader
				legacy bool
			}{r, legacy})
		}
	}
	if len(out) != len(want) {
		t.Fatalf("found %d of the %d empty-flag readers", len(out), len(want))
	}
	return out
}

// TestREQ169EmptyXMLFlagReadsAsAbsent pins the REQ-169 rule for canonical XML
// (REQ-056) that an empty or whitespace-only flag element carries no boolean
// and is read as an absent flag: a self-closing, an empty and a
// whitespace-only element of each of the four flags, on a bounded side and
// beside an open side, decode with no error and with the flag's default. BASE
// Interval reads the same of the v0.28.0 Go field spelling.
//
// Can-fail control: read an empty element as a present false and the
// included-on-a-bounded-side cases fail; parse the untrimmed text and every
// whitespace-only case is refused.
func TestREQ169EmptyXMLFlagReadsAsAbsent(t *testing.T) {
	for _, er := range xmlEmptyFlagReaders(t) {
		for _, tc := range xmlEmptyFlagCases {
			for _, sp := range xmlEmptyFlagSpellings {
				names := []string{tc.flag}
				if er.legacy {
					names = append(names, legacyFlagName[tc.flag])
				}
				for _, name := range names {
					t.Run(er.r.name+"/"+tc.name+"/"+sp.name+"/"+name, func(t *testing.T) {
						var b strings.Builder
						for _, c := range tc.children {
							if c.value == "$empty" {
								b.WriteString(sp.elem(name))
								continue
							}
							b.WriteString(xmlIntervalBody([]xmlChild{c}, er.r.lower, er.r.upper))
						}
						doc := fmt.Sprintf(er.r.doc, b.String())
						got, err := er.r.decode([]byte(doc))
						if err != nil {
							t.Fatalf("canxml.Unmarshal(%s) = %v, want no error: an empty flag element reads as absent", doc, err)
						}
						if got != tc.want {
							t.Errorf("canxml.Unmarshal(%s) flags = %+v, want %+v", doc, got, tc.want)
						}
					})
				}
			}
		}
	}
}

// TestREQ169XMLFlagTextIsTrimmedAndParsed pins the other side of the
// empty-flag rule: a flag element that does carry text is read as written once
// its surrounding whitespace is trimmed, and text that is no boolean stays a
// decode error rather than reading as absent.
func TestREQ169XMLFlagTextIsTrimmedAndParsed(t *testing.T) {
	for _, er := range xmlEmptyFlagReaders(t) {
		r := er.r
		t.Run(r.name+"/padded false is read as written", func(t *testing.T) {
			doc := fmt.Sprintf(r.doc, r.lower+r.upper+"<lower_included>\n  false \t</lower_included>")
			got, err := r.decode([]byte(doc))
			if err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v, want no error", doc, err)
			}
			if want := (intervalFlags{UpperIncluded: true}); got != want {
				t.Errorf("canxml.Unmarshal(%s) flags = %+v, want %+v", doc, got, want)
			}
		})
		t.Run(r.name+"/text that is no boolean is refused", func(t *testing.T) {
			doc := fmt.Sprintf(r.doc, r.lower+r.upper+"<lower_included>maybe</lower_included>")
			if _, err := r.decode([]byte(doc)); err == nil {
				t.Errorf("canxml.Unmarshal(%s) succeeded, want an error for a flag that is no boolean", doc)
			}
		})
	}
}
