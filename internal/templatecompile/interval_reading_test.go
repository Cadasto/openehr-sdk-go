package templatecompile_test

// interval_reading_test.go: the occurrences, existence and cardinality
// intervals and the temporal ranges an OPT writes, read by
// aom14.ReadOperationalTemplate and compiled by Compile, in each of the
// reader's modes: every boundary flag read as wire.md § REQ-169 reads it, and
// every Multiplicity closed as § REQ-111 Reading the model closes it. The
// corpus excludes no occurrences, existence or cardinality end and leaves out
// no bound on a side it writes bounded, so its goldens show none of these
// readings.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// readerModes are the two modes of the model's reader. Each reads an
// interval the same way.
var readerModes = []struct {
	name string
	opts []canxml.DecoderOption
}{
	{name: "default"},
	{name: "strict", opts: []canxml.DecoderOption{canxml.WithStrict()}},
}

// writtenInterval spells an OPT interval element by element. An empty field
// leaves that element out.
type writtenInterval struct {
	lowerIncluded, upperIncluded   string
	lowerUnbounded, upperUnbounded string
	lower, upper                   string
}

func (w writtenInterval) xml() string {
	var b strings.Builder
	for _, e := range []struct{ name, value string }{
		{"lower_included", w.lowerIncluded},
		{"upper_included", w.upperIncluded},
		{"lower_unbounded", w.lowerUnbounded},
		{"upper_unbounded", w.upperUnbounded},
		{"lower", w.lower},
		{"upper", w.upper},
	} {
		if e.value != "" {
			b.WriteString("<" + e.name + ">" + e.value + "</" + e.name + ">")
		}
	}
	return b.String()
}

// contentOPT is a COMPOSITION whose content, a C_MULTIPLE_ATTRIBUTE, holds
// one child. {{attribute}} takes the attribute's existence or cardinality,
// and {{child}} the child: a SECTION at0001 with {{occurrences}}, unless a
// test gives another child.
const contentOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>interval_reading</value></template_id>
  <concept>interval_reading</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      {{attribute}}
      {{child}}
    </attributes>
  </definition>
</template>`

// sectionChild is the default child of contentOPT.
const sectionChild = `<children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>SECTION</rm_type_name>
        <node_id>at0001</node_id>
        {{occurrences}}
      </children>`

// multiplicitySite is one of the three places an OPT writes an interval the
// compile step closes into a Multiplicity.
type multiplicitySite struct {
	name      string
	attribute string // the {{attribute}} of contentOPT; {{interval}} marks the interval
	child     string // the {{occurrences}} of sectionChild; {{interval}} marks the interval
	get       func(content *templatecompile.CompiledAttribute) *template.Multiplicity
}

var multiplicitySites = []multiplicitySite{
	{
		name:  "occurrences",
		child: `<occurrences>{{interval}}</occurrences>`,
		get: func(content *templatecompile.CompiledAttribute) *template.Multiplicity {
			return content.Children()[0].Occurrences()
		},
	},
	{
		name:      "existence",
		attribute: `<existence>{{interval}}</existence>`,
		get:       (*templatecompile.CompiledAttribute).Existence,
	},
	{
		name: "cardinality",
		attribute: `<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique>` +
			`<interval>{{interval}}</interval></cardinality>`,
		get: (*templatecompile.CompiledAttribute).ChildMultiplicity,
	},
}

// body is contentOPT with the interval ivXML at the site.
func (s multiplicitySite) body(ivXML string) string {
	child := strings.ReplaceAll(sectionChild, "{{occurrences}}", strings.ReplaceAll(s.child, "{{interval}}", ivXML))
	return strings.NewReplacer(
		"{{attribute}}", strings.ReplaceAll(s.attribute, "{{interval}}", ivXML),
		"{{child}}", child,
	).Replace(contentOPT)
}

// readAndCompile reads body with the reader's options and compiles it in the
// compile step's default mode.
func readAndCompile(body string, opts ...canxml.DecoderOption) (*templatecompile.Compiled, error) {
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(body), opts...)
	if err != nil {
		return nil, err
	}
	return templatecompile.Compile(model)
}

// closedBounds is a comparable snapshot of a Multiplicity's four accessors.
type closedBounds struct {
	lower, upper                   int
	lowerUnbounded, upperUnbounded bool
}

func closedBoundsOf(m *template.Multiplicity) closedBounds {
	return closedBounds{lower: m.Lower(), upper: m.Upper(), lowerUnbounded: m.LowerUnbounded(), upperUnbounded: m.UpperUnbounded()}
}

// TestREQ111_CompileClosesTheIntervalsTheReaderReads pins § REQ-111 Reading
// the model on each Multiplicity an OPT writes, from the XML to the compiled
// tree, in both of the reader's modes: a flag the OPT leaves out, or writes
// empty, is read as wire.md § REQ-169 reads it, an excluded end on a bounded
// side becomes the closed end it denotes, and an unbounded side keeps no
// bound whatever its included flag says.
func TestREQ111_CompileClosesTheIntervalsTheReaderReads(t *testing.T) {
	const (
		bothBounded = `<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>3</upper>`
		openLower   = `<lower_unbounded>true</lower_unbounded><upper_unbounded>false</upper_unbounded><upper>3</upper>`
		openUpper   = `<lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower>`
	)
	cases := []struct {
		name  string
		ivXML string
		want  closedBounds
	}{
		{name: "included flags left out on bounded sides", ivXML: writtenInterval{lowerUnbounded: "false", upperUnbounded: "false", lower: "1", upper: "3"}.xml(), want: closedBounds{lower: 1, upper: 3}},
		{name: "every flag left out", ivXML: writtenInterval{lower: "1", upper: "3"}.xml(), want: closedBounds{lower: 1, upper: 3}},
		{name: "included flag left out on an open side", ivXML: writtenInterval{lowerUnbounded: "false", upperUnbounded: "true", lower: "0"}.xml(), want: closedBounds{upperUnbounded: true}},
		{name: "every flag written closed", ivXML: writtenInterval{lowerIncluded: "true", upperIncluded: "true", lowerUnbounded: "false", upperUnbounded: "false", lower: "1", upper: "3"}.xml(), want: closedBounds{lower: 1, upper: 3}},
		{name: ">0..3 is 1..3", ivXML: writtenInterval{lowerIncluded: "false", lowerUnbounded: "false", upperUnbounded: "false", lower: "0", upper: "3"}.xml(), want: closedBounds{lower: 1, upper: 3}},
		{name: "0..<3 is 0..2", ivXML: writtenInterval{upperIncluded: "false", lowerUnbounded: "false", upperUnbounded: "false", lower: "0", upper: "3"}.xml(), want: closedBounds{upper: 2}},
		{name: ">0..* is 1..*", ivXML: writtenInterval{lowerIncluded: "false", lowerUnbounded: "false", upperUnbounded: "true", lower: "0"}.xml(), want: closedBounds{lower: 1, upperUnbounded: true}},
		{name: ">0..<3 is 1..2", ivXML: writtenInterval{lowerIncluded: "false", upperIncluded: "false", lowerUnbounded: "false", upperUnbounded: "false", lower: "0", upper: "3"}.xml(), want: closedBounds{lower: 1, upper: 2}},
		{name: "2..<3 is 2..2", ivXML: writtenInterval{upperIncluded: "false", lowerUnbounded: "false", upperUnbounded: "false", lower: "2", upper: "3"}.xml(), want: closedBounds{lower: 2, upper: 2}},
		{name: "an excluded end on an open upper side moves no bound", ivXML: writtenInterval{lowerIncluded: "true", upperIncluded: "false", lowerUnbounded: "false", upperUnbounded: "true", lower: "2"}.xml(), want: closedBounds{lower: 2, upperUnbounded: true}},
		{name: "an excluded end on an open lower side moves no bound", ivXML: writtenInterval{lowerIncluded: "false", lowerUnbounded: "true", upperUnbounded: "false", upper: "3"}.xml(), want: closedBounds{upper: 3, lowerUnbounded: true}},
		{name: "an included open upper side", ivXML: writtenInterval{lowerIncluded: "true", upperIncluded: "true", lowerUnbounded: "false", upperUnbounded: "true", lower: "0"}.xml(), want: closedBounds{upperUnbounded: true}},
		{name: "two included open sides", ivXML: writtenInterval{lowerIncluded: "true", upperIncluded: "true", lowerUnbounded: "true", upperUnbounded: "true"}.xml(), want: closedBounds{lowerUnbounded: true, upperUnbounded: true}},
		{name: "only an upper bound is open below", ivXML: writtenInterval{upper: "3"}.xml(), want: closedBounds{upper: 3, lowerUnbounded: true}},
		{name: "only a lower bound is open above", ivXML: writtenInterval{lower: "1"}.xml(), want: closedBounds{lower: 1, upperUnbounded: true}},
		{name: "no bound and no flag is open on both sides", ivXML: writtenInterval{}.xml(), want: closedBounds{lowerUnbounded: true, upperUnbounded: true}},
		{name: "an unbounded flag written beside its bound is read as written", ivXML: writtenInterval{lowerUnbounded: "true", lower: "1", upper: "3"}.xml(), want: closedBounds{lower: 1, upper: 3, lowerUnbounded: true}},
		{name: "an empty lower_included on a bounded side", ivXML: `<lower_included/>` + bothBounded, want: closedBounds{lower: 1, upper: 3}},
		{name: "a whitespace-only upper_included on a bounded side", ivXML: "<upper_included> \n\t</upper_included>" + bothBounded, want: closedBounds{lower: 1, upper: 3}},
		{name: "an empty lower_included on an open side", ivXML: `<lower_included/>` + openLower, want: closedBounds{upper: 3, lowerUnbounded: true}},
		{name: "an empty upper_included on an open side", ivXML: `<upper_included></upper_included>` + openUpper, want: closedBounds{lower: 1, upperUnbounded: true}},
		{name: "a written false beside an empty included flag", ivXML: `<lower_included>false</lower_included><upper_included/>` + bothBounded, want: closedBounds{lower: 2, upper: 3}},
		{name: "whitespace-only unbounded flags", ivXML: "<lower_unbounded> </lower_unbounded><upper_unbounded>  </upper_unbounded><lower>1</lower><upper>3</upper>", want: closedBounds{lower: 1, upper: 3}},
		{name: "a written true beside a whitespace-only unbounded flag", ivXML: "<lower_unbounded> </lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower>", want: closedBounds{lower: 1, upperUnbounded: true}},
		{name: ">2147483646..* closes to the largest Integer", ivXML: writtenInterval{lowerIncluded: "false", lowerUnbounded: "false", upperUnbounded: "true", lower: "2147483646"}.xml(), want: closedBounds{lower: 2147483647, upperUnbounded: true}},
		{name: "*..<-2147483647 closes to the smallest Integer", ivXML: writtenInterval{upperIncluded: "false", lowerUnbounded: "true", upperUnbounded: "false", upper: "-2147483647"}.xml(), want: closedBounds{upper: -2147483648, lowerUnbounded: true}},
		{name: "an excluded largest Integer on an open lower side moves no bound", ivXML: writtenInterval{lowerIncluded: "false", lowerUnbounded: "true", upperUnbounded: "true", lower: "2147483647"}.xml(), want: closedBounds{lower: 2147483647, lowerUnbounded: true, upperUnbounded: true}},
	}
	for _, mode := range readerModes {
		for _, site := range multiplicitySites {
			for _, tc := range cases {
				t.Run(mode.name+"/"+site.name+"/"+tc.name, func(t *testing.T) {
					c, err := readAndCompile(site.body(tc.ivXML), mode.opts...)
					if err != nil {
						t.Fatalf("read and compile %s %s = %v, want the template", site.name, tc.ivXML, err)
					}
					m := site.get(c.Root().Attribute("content"))
					if m == nil {
						t.Fatalf("%s %s: Multiplicity = nil, want %+v", site.name, tc.ivXML, tc.want)
					}
					if got := closedBoundsOf(m); got != tc.want {
						t.Errorf("%s %s: Multiplicity = %+v, want %+v", site.name, tc.ivXML, got, tc.want)
					}
				})
			}
		}
	}
}

// TestREQ111_CompileRefusesAnIntervalClosingLeavesEmpty pins the refusals of
// § REQ-111 Reading the model on each Multiplicity an OPT writes, in both of
// the reader's modes: an inverted interval, one that closing its excluded
// ends leaves empty, and one with an excluded end whose closed end the
// model's 32-bit Integer cannot hold, whatever the width of Go's int, are
// refused with an error wrapping ErrInvalidInput.
func TestREQ111_CompileRefusesAnIntervalClosingLeavesEmpty(t *testing.T) {
	cases := []struct {
		name string
		iv   writtenInterval
	}{
		{name: "5..2", iv: writtenInterval{lowerUnbounded: "false", upperUnbounded: "false", lower: "5", upper: "2"}},
		{name: ">2..<3", iv: writtenInterval{lowerIncluded: "false", upperIncluded: "false", lowerUnbounded: "false", upperUnbounded: "false", lower: "2", upper: "3"}},
		{name: ">2..2", iv: writtenInterval{lowerIncluded: "false", upperIncluded: "true", lowerUnbounded: "false", upperUnbounded: "false", lower: "2", upper: "2"}},
		{name: "2..<2", iv: writtenInterval{lowerIncluded: "true", upperIncluded: "false", lowerUnbounded: "false", upperUnbounded: "false", lower: "2", upper: "2"}},
		{name: ">2147483647..*", iv: writtenInterval{lowerIncluded: "false", lowerUnbounded: "false", upperUnbounded: "true", lower: "2147483647"}},
		{name: "*..<-2147483648", iv: writtenInterval{upperIncluded: "false", lowerUnbounded: "true", upperUnbounded: "false", upper: "-2147483648"}},
	}
	for _, mode := range readerModes {
		for _, site := range multiplicitySites {
			for _, tc := range cases {
				t.Run(mode.name+"/"+site.name+"/"+tc.name, func(t *testing.T) {
					_, err := readAndCompile(site.body(tc.iv.xml()), mode.opts...)
					if !errors.Is(err, templatecompile.ErrInvalidInput) {
						t.Errorf("read and compile %s %s = %v, want an error wrapping ErrInvalidInput", site.name, tc.iv.xml(), err)
					}
				})
			}
		}
	}
}

// temporalSites are the primitive constraints whose range is temporal, each
// as a child of contentOPT with a bound of its type. {{interval}} marks the
// range.
var temporalSites = []struct {
	name, child, bound string
}{
	{name: "C_DATE", bound: "2004-05-20", child: `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>DATE</rm_type_name><node_id/>` +
		`<item xsi:type="C_DATE"><range>{{interval}}</range></item></children>`},
	{name: "C_TIME", bound: "09:30:00", child: `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>TIME</rm_type_name><node_id/>` +
		`<item xsi:type="C_TIME"><range>{{interval}}</range></item></children>`},
	{name: "C_DATE_TIME", bound: "2004-05-20T10:00:00Z", child: `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>DATE_TIME</rm_type_name><node_id/>` +
		`<item xsi:type="C_DATE_TIME"><range>{{interval}}</range></item></children>`},
	{name: "C_DURATION", bound: "PT1H", child: `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>DURATION</rm_type_name><node_id/>` +
		`<item xsi:type="C_DURATION"><range>{{interval}}</range></item></children>`},
}

// childBody is contentOPT whose one child is child, the {{interval}} in it
// replaced with ivXML.
func childBody(child, ivXML string) string {
	return strings.NewReplacer(
		"{{attribute}}", "",
		"{{child}}", strings.ReplaceAll(child, "{{interval}}", ivXML),
	).Replace(contentOPT)
}

// temporalRangeOf returns the range of the temporal constraint p.
func temporalRangeOf(t *testing.T, p constraints.PrimitiveConstraint) constraints.TemporalRange {
	t.Helper()
	switch c := p.(type) {
	case constraints.CDate:
		return c.Range
	case constraints.CTime:
		return c.Range
	case constraints.CDateTime:
		return c.Range
	case constraints.CDuration:
		return c.Range
	}
	t.Fatalf("PrimitiveConstraint() = %T, want a temporal constraint", p)
	return constraints.TemporalRange{}
}

// TestREQ111_CompileReadsATemporalRangeSideByItsBound pins wire.md § REQ-169
// on the temporal ranges an OPT writes, from the XML to the compiled tree, in
// both of the reader's modes: an unbounded flag the range leaves out is read
// as false beside a bound and as true where the bound is left out too, and a
// side written bounded with no bound is refused with an error wrapping
// canxml.ErrInvalidShape, since the model holds the bound by value.
func TestREQ111_CompileReadsATemporalRangeSideByItsBound(t *testing.T) {
	for _, mode := range readerModes {
		for _, site := range temporalSites {
			read := []struct {
				name string
				iv   writtenInterval
				want constraints.TemporalRange
			}{
				{name: "only an upper bound is open below", iv: writtenInterval{upper: site.bound}, want: constraints.TemporalRange{Upper: site.bound, UpperInclusive: true, LowerUnbounded: true}},
				{name: "only a lower bound is open above", iv: writtenInterval{lower: site.bound}, want: constraints.TemporalRange{Lower: site.bound, LowerInclusive: true, UpperUnbounded: true}},
			}
			for _, tc := range read {
				t.Run(mode.name+"/"+site.name+"/"+tc.name, func(t *testing.T) {
					c, err := readAndCompile(childBody(site.child, tc.iv.xml()), mode.opts...)
					if err != nil {
						t.Fatalf("read and compile %s %s = %v, want the template", site.name, tc.iv.xml(), err)
					}
					leaf := c.Root().Attribute("content").Children()[0]
					if got := temporalRangeOf(t, leaf.PrimitiveConstraint()); got != tc.want {
						t.Errorf("%s %s: range = %+v, want %+v", site.name, tc.iv.xml(), got, tc.want)
					}
				})
			}
			refused := []struct {
				name string
				iv   writtenInterval
				side string
			}{
				{name: "lower written bounded without a lower bound", iv: writtenInterval{lowerUnbounded: "false", upper: site.bound}, side: "lower"},
				{name: "upper written bounded without an upper bound", iv: writtenInterval{upperUnbounded: "false", lower: site.bound}, side: "upper"},
				{name: "both written bounded without bounds", iv: writtenInterval{lowerUnbounded: "false", upperUnbounded: "false"}, side: "lower"},
			}
			for _, tc := range refused {
				t.Run(mode.name+"/"+site.name+"/"+tc.name, func(t *testing.T) {
					_, err := readAndCompile(childBody(site.child, tc.iv.xml()), mode.opts...)
					if !errors.Is(err, canxml.ErrInvalidShape) {
						t.Fatalf("read and compile %s %s = %v, want an error wrapping canxml.ErrInvalidShape", site.name, tc.iv.xml(), err)
					}
					if flag := tc.side + "_unbounded"; !strings.Contains(err.Error(), flag) {
						t.Errorf("read and compile %s %s = %v, want the error to name %s", site.name, tc.iv.xml(), err, flag)
					}
				})
			}
		}
	}
}
