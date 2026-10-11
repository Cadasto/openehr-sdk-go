package template_test

import (
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-169 and REQ-100 § Intervals: the OPT parser reads an omitted
// boundary flag by the one reading wire.md § REQ-169 gives, in both parse
// modes. A Multiplicity (occurrences, existence, cardinality) is closed on
// each bounded side, so an excluded end becomes the whole number next to
// it, and an interval that leaves no whole number is refused like an
// inverted one. A numeric range keeps its included flags (REQ-103).
//
// The fixtures are synthetic and inline: the vendored corpus omits an
// included flag only on an unbounded side, and excludes no occurrences,
// existence or cardinality end, so it cannot show these readings.

// intervalParsers lists both parse modes; each reads intervals the same way.
var intervalParsers = []struct {
	name  string
	parse func(io.Reader) (*template.OperationalTemplate, error)
}{
	{name: "ParseOPT", parse: template.ParseOPT},
	{name: "ParseOPTStrict", parse: template.ParseOPTStrict},
}

// wireInterval spells an OPT interval element by element. An empty field
// leaves that element out of the XML.
type wireInterval struct {
	lowerIncluded, upperIncluded   string
	lowerUnbounded, upperUnbounded string
	lower, upper                   string
}

func (w wireInterval) xml() string {
	var sb strings.Builder
	for _, e := range []struct{ name, value string }{
		{"lower_included", w.lowerIncluded},
		{"upper_included", w.upperIncluded},
		{"lower_unbounded", w.lowerUnbounded},
		{"upper_unbounded", w.upperUnbounded},
		{"lower", w.lower},
		{"upper", w.upper},
	} {
		if e.value == "" {
			continue
		}
		sb.WriteString("<" + e.name + ">" + e.value + "</" + e.name + ">")
	}
	return sb.String()
}

// multiplicityOPT holds one C_MULTIPLE_ATTRIBUTE with one child. The
// {{attribute}} slot takes the attribute's existence or cardinality, and
// the {{child}} slot takes the child's occurrences.
const multiplicityOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>interval_flags</value></template_id>
  <concept>interval_flags</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      {{attribute}}
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>SECTION</rm_type_name>
        <node_id>at0001</node_id>
        {{child}}
      </children>
    </attributes>
  </definition>
</template>`

// multiplicitySites lists the three places an OPT carries a Multiplicity.
var multiplicitySites = []struct {
	name      string
	attribute string // XML for the {{attribute}} slot; {{interval}} marks the interval
	child     string // XML for the {{child}} slot; {{interval}} marks the interval
	context   string // how the parser names the site in a refusal
	get       func(t *testing.T, opt *template.OperationalTemplate) *template.Multiplicity
}{
	{
		name:    "occurrences",
		child:   `<occurrences>{{interval}}</occurrences>`,
		context: `occurrences on "at0001" (SECTION)`,
		get: func(t *testing.T, opt *template.OperationalTemplate) *template.Multiplicity {
			t.Helper()
			children := contentAttribute(t, opt).Children()
			if len(children) != 1 {
				t.Fatalf("content attribute has %d children, want 1", len(children))
			}
			co, ok := children[0].(*template.ComplexObject)
			if !ok {
				t.Fatalf("content child is %T, want *template.ComplexObject", children[0])
			}
			return co.Occurrences()
		},
	},
	{
		name:      "existence",
		attribute: `<existence>{{interval}}</existence>`,
		context:   `existence on attribute "content"`,
		get: func(t *testing.T, opt *template.OperationalTemplate) *template.Multiplicity {
			t.Helper()
			return contentAttribute(t, opt).Existence()
		},
	},
	{
		name: "cardinality",
		attribute: `<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique>` +
			`<interval>{{interval}}</interval></cardinality>`,
		context: `cardinality on attribute "content"`,
		get: func(t *testing.T, opt *template.OperationalTemplate) *template.Multiplicity {
			t.Helper()
			return contentAttribute(t, opt).ChildMultiplicity()
		},
	},
}

func multiplicityBody(attribute, child, ivXML string) string {
	return strings.NewReplacer(
		"{{attribute}}", strings.ReplaceAll(attribute, "{{interval}}", ivXML),
		"{{child}}", strings.ReplaceAll(child, "{{interval}}", ivXML),
	).Replace(multiplicityOPT)
}

func contentAttribute(t *testing.T, opt *template.OperationalTemplate) *template.Attribute {
	t.Helper()
	root, ok := opt.Root().(*template.ComplexObject)
	if !ok {
		t.Fatalf("Root() is %T, want *template.ComplexObject", opt.Root())
	}
	attrs := root.Attributes()
	if len(attrs) != 1 {
		t.Fatalf("root has %d attributes, want 1", len(attrs))
	}
	return attrs[0]
}

// bounds is a comparable snapshot of a Multiplicity's four accessors.
type bounds struct {
	lower, upper                   int
	lowerUnbounded, upperUnbounded bool
}

func boundsOf(m *template.Multiplicity) bounds {
	return bounds{
		lower:          m.Lower(),
		upper:          m.Upper(),
		lowerUnbounded: m.LowerUnbounded(),
		upperUnbounded: m.UpperUnbounded(),
	}
}

func TestREQ169_REQ100_MultiplicityFlags(t *testing.T) {
	cases := []struct {
		name string
		iv   wireInterval
		want bounds
	}{
		{
			// An omitted included flag on a bounded side is a closed end.
			name: "omitted_included_bounded_is_closed",
			iv:   wireInterval{lowerUnbounded: "false", upperUnbounded: "false", lower: "1", upper: "3"},
			want: bounds{lower: 1, upper: 3},
		},
		{
			// An omitted unbounded flag reads as false, so both sides are bounded.
			name: "omitted_all_flags_is_closed",
			iv:   wireInterval{lower: "1", upper: "3"},
			want: bounds{lower: 1, upper: 3},
		},
		{
			name: "omitted_included_on_open_upper",
			iv:   wireInterval{lowerUnbounded: "false", upperUnbounded: "true", lower: "0"},
			want: bounds{lower: 0, upperUnbounded: true},
		},
		{
			name: "explicit_closed",
			iv: wireInterval{
				lowerIncluded: "true", upperIncluded: "true",
				lowerUnbounded: "false", upperUnbounded: "false", lower: "1", upper: "3",
			},
			want: bounds{lower: 1, upper: 3},
		},
		{
			name: "excluded_lower", // >0..3 is 1..3
			iv: wireInterval{
				lowerIncluded:  "false",
				lowerUnbounded: "false", upperUnbounded: "false", lower: "0", upper: "3",
			},
			want: bounds{lower: 1, upper: 3},
		},
		{
			name: "excluded_upper", // 0..<3 is 0..2
			iv: wireInterval{
				upperIncluded:  "false",
				lowerUnbounded: "false", upperUnbounded: "false", lower: "0", upper: "3",
			},
			want: bounds{lower: 0, upper: 2},
		},
		{
			name: "excluded_lower_open_upper", // >0..* is 1..*
			iv: wireInterval{
				lowerIncluded:  "false",
				lowerUnbounded: "false", upperUnbounded: "true", lower: "0",
			},
			want: bounds{lower: 1, upperUnbounded: true},
		},
		{
			name: "both_excluded", // >0..<3 is 1..2
			iv: wireInterval{
				lowerIncluded: "false", upperIncluded: "false",
				lowerUnbounded: "false", upperUnbounded: "false", lower: "0", upper: "3",
			},
			want: bounds{lower: 1, upper: 2},
		},
		{
			name: "excluded_upper_leaves_point", // 2..<3 is 2..2
			iv: wireInterval{
				upperIncluded:  "false",
				lowerUnbounded: "false", upperUnbounded: "false", lower: "2", upper: "3",
			},
			want: bounds{lower: 2, upper: 2},
		},
		{
			// An excluded end on an unbounded side moves no bound.
			name: "excluded_on_open_upper_has_no_effect",
			iv: wireInterval{
				lowerIncluded: "true", upperIncluded: "false",
				lowerUnbounded: "false", upperUnbounded: "true", lower: "2",
			},
			want: bounds{lower: 2, upperUnbounded: true},
		},
		{
			name: "excluded_on_open_lower_has_no_effect",
			iv: wireInterval{
				lowerIncluded:  "false",
				lowerUnbounded: "true", upperUnbounded: "false", upper: "3",
			},
			want: bounds{upper: 3, lowerUnbounded: true},
		},
		{
			// An included open side is read as written and changes nothing.
			name: "included_open_upper_has_no_effect",
			iv: wireInterval{
				lowerIncluded: "true", upperIncluded: "true",
				lowerUnbounded: "false", upperUnbounded: "true", lower: "0",
			},
			want: bounds{lower: 0, upperUnbounded: true},
		},
		{
			name: "included_open_both_has_no_effect",
			iv: wireInterval{
				lowerIncluded: "true", upperIncluded: "true",
				lowerUnbounded: "true", upperUnbounded: "true",
			},
			want: bounds{lowerUnbounded: true, upperUnbounded: true},
		},
	}
	for _, p := range intervalParsers {
		for _, site := range multiplicitySites {
			for _, tc := range cases {
				t.Run(p.name+"/"+site.name+"/"+tc.name, func(t *testing.T) {
					opt, err := p.parse(strings.NewReader(multiplicityBody(site.attribute, site.child, tc.iv.xml())))
					if err != nil {
						t.Fatalf("%s(%s %s) error = %v, want nil", p.name, site.name, tc.iv.xml(), err)
					}
					m := site.get(t, opt)
					if m == nil {
						t.Fatalf("%s(%s %s) Multiplicity = nil, want %+v", p.name, site.name, tc.iv.xml(), tc.want)
					}
					if got := boundsOf(m); got != tc.want {
						t.Errorf("%s(%s %s) Multiplicity = %+v, want %+v", p.name, site.name, tc.iv.xml(), got, tc.want)
					}
				})
			}
		}
	}
}

// An interval that leaves no whole number once its excluded ends are
// closed is refused exactly like an inverted one, and so is an excluded
// end whose closed end an int cannot hold. Each refusal wraps
// ErrInvalidOPT and names the site. The inverted-interval message gives
// the closed bounds it compared and the interval as the OPT wrote it.
func TestREQ100_MultiplicityRefusedLikeInverted(t *testing.T) {
	cases := []struct {
		name    string
		iv      wireInterval
		wantMsg string
	}{
		{
			// The control: an inverted interval was refused before
			// excluded ends were read. Omitted included flags read as
			// closed ends, so the written form has no > or <.
			name:    "inverted",
			iv:      wireInterval{lowerUnbounded: "false", upperUnbounded: "false", lower: "5", upper: "2"},
			wantMsg: "inverted interval lower=5 > upper=2 (written 5..2)",
		},
		{
			name: "both_excluded_adjacent", // >2..<3
			iv: wireInterval{
				lowerIncluded: "false", upperIncluded: "false",
				lowerUnbounded: "false", upperUnbounded: "false", lower: "2", upper: "3",
			},
			wantMsg: "inverted interval lower=3 > upper=2 (written >2..<3)",
		},
		{
			name: "excluded_lower_at_upper", // >2..2
			iv: wireInterval{
				lowerIncluded: "false", upperIncluded: "true",
				lowerUnbounded: "false", upperUnbounded: "false", lower: "2", upper: "2",
			},
			wantMsg: "inverted interval lower=3 > upper=2 (written >2..2)",
		},
		{
			name: "excluded_upper_at_lower", // 2..<2
			iv: wireInterval{
				lowerIncluded: "true", upperIncluded: "false",
				lowerUnbounded: "false", upperUnbounded: "false", lower: "2", upper: "2",
			},
			wantMsg: "inverted interval lower=2 > upper=1 (written 2..<2)",
		},
		{
			// The closed end of ">largest int" is one past the int
			// range, so it would wrap around to the smallest int.
			name: "excluded_lower_at_max_int",
			iv: wireInterval{
				lowerIncluded:  "false",
				lowerUnbounded: "false", upperUnbounded: "true", lower: strconv.Itoa(math.MaxInt),
			},
			wantMsg: "closed lower bound overflows int: excluded lower=" + strconv.Itoa(math.MaxInt),
		},
		{
			name: "excluded_upper_at_min_int",
			iv: wireInterval{
				upperIncluded:  "false",
				lowerUnbounded: "true", upperUnbounded: "false", upper: strconv.Itoa(math.MinInt),
			},
			wantMsg: "closed upper bound overflows int: excluded upper=" + strconv.Itoa(math.MinInt),
		},
	}
	for _, p := range intervalParsers {
		for _, site := range multiplicitySites {
			for _, tc := range cases {
				t.Run(p.name+"/"+site.name+"/"+tc.name, func(t *testing.T) {
					opt, err := p.parse(strings.NewReader(multiplicityBody(site.attribute, site.child, tc.iv.xml())))
					if err == nil {
						t.Fatalf("%s(%s %s) = %v, nil; want an error wrapping ErrInvalidOPT",
							p.name, site.name, tc.iv.xml(), opt)
					}
					if !errors.Is(err, template.ErrInvalidOPT) {
						t.Errorf("%s(%s %s) error = %v, want errors.Is(err, ErrInvalidOPT)",
							p.name, site.name, tc.iv.xml(), err)
					}
					for _, want := range []string{site.context, tc.wantMsg} {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("%s(%s %s) error = %q, want it to contain %q",
								p.name, site.name, tc.iv.xml(), err, want)
						}
					}
				})
			}
		}
	}
}

// An empty or whitespace-only lower_included or upper_included element
// reads like an omitted one, so REQ-169's default applies: a closed end on
// a bounded side, no effect on an unbounded side. Read as a written false,
// it would close a bounded end one step inwards.
func TestREQ169_REQ100_EmptyIncludedFlagReadsAsOmitted(t *testing.T) {
	const (
		closedBounds = `<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
			`<lower>1</lower><upper>3</upper>`
		openLower = `<lower_unbounded>true</lower_unbounded><upper_unbounded>false</upper_unbounded><upper>3</upper>`
		openUpper = `<lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower>`
	)
	cases := []struct {
		name  string
		ivXML string
		want  bounds
	}{
		{name: "lower_self_closing_bounded", ivXML: `<lower_included/>` + closedBounds, want: bounds{lower: 1, upper: 3}},
		{name: "lower_empty_bounded", ivXML: `<lower_included></lower_included>` + closedBounds, want: bounds{lower: 1, upper: 3}},
		{name: "lower_whitespace_bounded", ivXML: "<lower_included> \n\t</lower_included>" + closedBounds, want: bounds{lower: 1, upper: 3}},
		{name: "upper_self_closing_bounded", ivXML: `<upper_included/>` + closedBounds, want: bounds{lower: 1, upper: 3}},
		{name: "upper_empty_bounded", ivXML: `<upper_included></upper_included>` + closedBounds, want: bounds{lower: 1, upper: 3}},
		{name: "upper_whitespace_bounded", ivXML: "<upper_included>  </upper_included>" + closedBounds, want: bounds{lower: 1, upper: 3}},
		{name: "both_self_closing_bounded", ivXML: `<lower_included/><upper_included/>` + closedBounds, want: bounds{lower: 1, upper: 3}},
		{name: "lower_self_closing_unbounded", ivXML: `<lower_included/>` + openLower, want: bounds{upper: 3, lowerUnbounded: true}},
		{name: "upper_self_closing_unbounded", ivXML: `<upper_included/>` + openUpper, want: bounds{lower: 1, upperUnbounded: true}},
		{
			// A written value beside an empty sibling is still read as written.
			name:  "lower_written_false_upper_empty",
			ivXML: `<lower_included>false</lower_included><upper_included/>` + closedBounds,
			want:  bounds{lower: 2, upper: 3},
		},
	}
	for _, p := range intervalParsers {
		for _, site := range multiplicitySites {
			for _, tc := range cases {
				t.Run(p.name+"/"+site.name+"/"+tc.name, func(t *testing.T) {
					opt, err := p.parse(strings.NewReader(multiplicityBody(site.attribute, site.child, tc.ivXML)))
					if err != nil {
						t.Fatalf("%s(%s %q) error = %v, want nil", p.name, site.name, tc.ivXML, err)
					}
					m := site.get(t, opt)
					if m == nil {
						t.Fatalf("%s(%s %q) Multiplicity = nil, want %+v", p.name, site.name, tc.ivXML, tc.want)
					}
					if got := boundsOf(m); got != tc.want {
						t.Errorf("%s(%s %q) Multiplicity = %+v, want %+v", p.name, site.name, tc.ivXML, got, tc.want)
					}
				})
			}
		}
	}
}

// A lower_included or upper_included element whose text is not a boolean
// still fails the parse in both modes.
func TestREQ100_NonBooleanIncludedFlagRefused(t *testing.T) {
	ivXML := `<lower_included>maybe</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>false</upper_unbounded><lower>1</lower><upper>3</upper>`
	for _, p := range intervalParsers {
		for _, site := range multiplicitySites {
			t.Run(p.name+"/"+site.name, func(t *testing.T) {
				_, err := p.parse(strings.NewReader(multiplicityBody(site.attribute, site.child, ivXML)))
				if !errors.Is(err, template.ErrInvalidOPT) {
					t.Errorf("%s(%s %q) error = %v, want errors.Is(err, ErrInvalidOPT)", p.name, site.name, ivXML, err)
				}
			})
		}
	}
}

// numericRangeSites lists every primitive constraint whose range the
// parser reads from OPT XML. {{interval}} marks the range.
var numericRangeSites = []struct {
	name  string
	child string
	get   func(t *testing.T, p constraints.PrimitiveConstraint) constraints.NumericRange
}{
	{
		name: "C_INTEGER",
		child: `<children xsi:type="C_INTEGER"><rm_type_name>INTEGER</rm_type_name><node_id/>` +
			`<range>{{interval}}</range></children>`,
		get: integerRange,
	},
	{
		name: "C_PRIMITIVE_OBJECT/C_INTEGER",
		child: `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>INTEGER</rm_type_name><node_id/>` +
			`<item xsi:type="C_INTEGER"><range>{{interval}}</range></item></children>`,
		get: integerRange,
	},
	{
		name: "C_REAL",
		child: `<children xsi:type="C_REAL"><rm_type_name>REAL</rm_type_name><node_id/>` +
			`<range>{{interval}}</range></children>`,
		get: func(t *testing.T, p constraints.PrimitiveConstraint) constraints.NumericRange {
			t.Helper()
			c, ok := p.(constraints.CReal)
			if !ok {
				t.Fatalf("PrimitiveConstraint() = %T, want constraints.CReal", p)
			}
			return c.Range
		},
	},
	{
		name: "C_DV_QUANTITY/magnitude",
		child: `<children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name><node_id/>` +
			`<list><magnitude>{{interval}}</magnitude><units>kg</units></list></children>`,
		get: func(t *testing.T, p constraints.PrimitiveConstraint) constraints.NumericRange {
			t.Helper()
			return quantityUnit(t, p).Magnitude
		},
	},
	{
		name: "C_DV_QUANTITY/precision",
		child: `<children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name><node_id/>` +
			`<list><precision>{{interval}}</precision><units>kg</units></list></children>`,
		get: func(t *testing.T, p constraints.PrimitiveConstraint) constraints.NumericRange {
			t.Helper()
			return quantityUnit(t, p).Precision
		},
	},
}

func integerRange(t *testing.T, p constraints.PrimitiveConstraint) constraints.NumericRange {
	t.Helper()
	c, ok := p.(constraints.CInteger)
	if !ok {
		t.Fatalf("PrimitiveConstraint() = %T, want constraints.CInteger", p)
	}
	return c.Range
}

func quantityUnit(t *testing.T, p constraints.PrimitiveConstraint) constraints.QuantityUnit {
	t.Helper()
	c, ok := p.(constraints.DvQuantity)
	if !ok {
		t.Fatalf("PrimitiveConstraint() = %T, want constraints.DvQuantity", p)
	}
	if len(c.Units) != 1 {
		t.Fatalf("DvQuantity.Units = %+v, want one entry", c.Units)
	}
	return c.Units[0]
}

func TestREQ169_REQ103_NumericRangeFlags(t *testing.T) {
	cases := []struct {
		name string
		iv   wireInterval
		want constraints.NumericRange
	}{
		{
			name: "omitted_included_bounded_is_inclusive",
			iv:   wireInterval{lowerUnbounded: "false", upperUnbounded: "false", lower: "1", upper: "9"},
			want: constraints.NumericRange{Lower: 1, Upper: 9, LowerInclusive: true, UpperInclusive: true},
		},
		{
			// An omitted unbounded flag reads as false, so both sides are bounded.
			name: "omitted_all_flags_is_inclusive",
			iv:   wireInterval{lower: "1", upper: "9"},
			want: constraints.NumericRange{Lower: 1, Upper: 9, LowerInclusive: true, UpperInclusive: true},
		},
		{
			name: "omitted_included_open_upper_is_exclusive",
			iv:   wireInterval{lowerUnbounded: "false", upperUnbounded: "true", lower: "1"},
			want: constraints.NumericRange{Lower: 1, LowerInclusive: true, UpperUnbounded: true},
		},
		{
			name: "omitted_included_open_lower_is_exclusive",
			iv:   wireInterval{lowerUnbounded: "true", upperUnbounded: "false", upper: "9"},
			want: constraints.NumericRange{Upper: 9, UpperInclusive: true, LowerUnbounded: true},
		},
		{
			name: "omitted_included_open_both_is_exclusive",
			iv:   wireInterval{lowerUnbounded: "true", upperUnbounded: "true"},
			want: constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true},
		},
		{
			name: "explicit_included_bounded",
			iv: wireInterval{
				lowerIncluded: "true", upperIncluded: "true",
				lowerUnbounded: "false", upperUnbounded: "false", lower: "1", upper: "9",
			},
			want: constraints.NumericRange{Lower: 1, Upper: 9, LowerInclusive: true, UpperInclusive: true},
		},
		{
			name: "explicit_excluded_bounded",
			iv: wireInterval{
				lowerIncluded: "false", upperIncluded: "false",
				lowerUnbounded: "false", upperUnbounded: "false", lower: "1", upper: "9",
			},
			want: constraints.NumericRange{Lower: 1, Upper: 9},
		},
		{
			// An included open side is read as written.
			name: "explicit_included_open_both",
			iv: wireInterval{
				lowerIncluded: "true", upperIncluded: "true",
				lowerUnbounded: "true", upperUnbounded: "true",
			},
			want: constraints.NumericRange{
				LowerInclusive: true, UpperInclusive: true,
				LowerUnbounded: true, UpperUnbounded: true,
			},
		},
	}
	for _, p := range intervalParsers {
		for _, site := range numericRangeSites {
			for _, tc := range cases {
				t.Run(p.name+"/"+site.name+"/"+tc.name, func(t *testing.T) {
					child := strings.ReplaceAll(site.child, "{{interval}}", tc.iv.xml())
					body := strings.Replace(primitiveOPTTemplate, "%s", child, 1)
					opt, err := p.parse(strings.NewReader(body))
					if err != nil {
						t.Fatalf("%s(%s %s) error = %v, want nil", p.name, site.name, tc.iv.xml(), err)
					}
					got := site.get(t, firstChildPrimitive(t, opt))
					if got != tc.want {
						t.Errorf("%s(%s %s) range = %+v, want %+v", p.name, site.name, tc.iv.xml(), got, tc.want)
					}
				})
			}
		}
	}
}
