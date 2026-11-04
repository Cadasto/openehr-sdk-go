package aom14_test

// REQ-111 (clinical-modeling.md § REQ-111, Reading the model, "What the
// reader tolerates"): aom14.ReadOperationalTemplate reads every OPT the
// deprecated parser reads in its default mode, as the parser reads it, but an
// integer the model's 32-bit Integer cannot hold, a match_negated of true and
// an infinite integer bound that admits no integer, which it refuses in every
// mode. REQ-100 (§ REQ-100, Strict parse mode): given canxml.WithStrict it
// refuses the shapes the parser's strict mode refuses, and the other readings
// of the parser's default mode. REQ-170
// (wire.md § REQ-170): what a tolerance leaves out is unmatched content, which
// canxml.WithUnmatched reports, once, at the path it has in the input, or in
// the element a reading moves it into.
//
// The fixtures under testdata/reader are SDK-authored: each *.read.opt is the
// reading of the fixture of its name, written as canonical XML decodes it.

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

const (
	// nsV1 is the namespace canonical XML reads.
	nsV1 = "http://schemas.openehr.org/v1"
	// nsV2 is the namespace the AM Release-1.4 schemas declare.
	nsV2 = "http://schemas.openehr.org/v2"
	// nsForeign is a namespace of no openEHR schema.
	nsForeign = "urn:x-openehr-sdk-go:test:foreign"
	// definition is the path of the definition of a template.
	definition = "/template[1]/definition[1]"
)

// readerFixture returns the bytes of testdata/reader/name.
func readerFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "reader", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// decoded decodes data with canxml.Unmarshal, which reads it as written.
func decoded(t *testing.T, data []byte) *aom14.OperationalTemplate {
	t.Helper()
	ot := new(aom14.OperationalTemplate)
	if err := canxml.Unmarshal(data, ot); err != nil {
		t.Fatalf("canxml.Unmarshal: %v", err)
	}
	return ot
}

// encoded returns the canonical XML of ot, which tells two readings apart.
func encoded(t *testing.T, ot *aom14.OperationalTemplate) string {
	t.Helper()
	data, err := canxml.Marshal(ot)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	return string(data)
}

// modes are the reader's two modes.
var modes = map[string][]canxml.DecoderOption{
	"default": nil,
	"strict":  {canxml.WithStrict()},
}

// readAs reads data with opts and fails the test unless its reading is
// want's, and returns the items canxml.WithUnmatched reports, in order.
func readAs(t *testing.T, data []byte, want *aom14.OperationalTemplate, opts ...canxml.DecoderOption) []string {
	t.Helper()
	var unmatched []string
	opts = append(slices.Clone(opts), canxml.WithUnmatched(func(path string) { unmatched = append(unmatched, path) }))
	got, err := aom14.ReadOperationalTemplate(bytes.NewReader(data), opts...)
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	if g, w := encoded(t, got), encoded(t, want); g != w {
		t.Errorf("ReadOperationalTemplate reads\n%s\nwant\n%s", g, w)
	}
	return unmatched
}

// TestREQ111_ReaderReadsTheNamespaceTheSchemasDeclare pins that, in every
// mode, an element in the http://schemas.openehr.org/v2 namespace reads as
// the element of that name in the http://schemas.openehr.org/v1 one, under
// the default namespace and under a prefix, after a byte-order mark too.
func TestREQ111_ReaderReadsTheNamespaceTheSchemasDeclare(t *testing.T) {
	t.Parallel()
	data := readerFixture(t, "namespace-v2.opt")
	want := decoded(t, bytes.ReplaceAll(data, []byte(nsV2), []byte(nsV1)))
	if n := len(want.Definition.Attributes); n != 1 {
		t.Fatalf("the v1 reading of the fixture holds %d attributes, want 1", n)
	}
	inputs := map[string][]byte{
		"as written":              data,
		"after a byte-order mark": append([]byte{0xEF, 0xBB, 0xBF}, data...),
	}
	for mode, opts := range modes {
		for name, in := range inputs {
			t.Run(mode+"/"+name, func(t *testing.T) {
				t.Parallel()
				if unmatched := readAs(t, in, want, opts...); len(unmatched) > 0 {
					t.Errorf("WithUnmatched reported %q, want nothing", unmatched)
				}
			})
		}
	}
}

// TestREQ111_ReaderReadsARootInAnotherNamespace pins that the default mode
// reads a root element in a namespace other than none, the v1 one and the v2
// one, and every element in that namespace, as in the v1 one, under the
// default namespace and under a prefix, as the deprecated parser, which reads
// no namespace, does; strict mode refuses it as a foreign namespace.
func TestREQ111_ReaderReadsARootInAnotherNamespace(t *testing.T) {
	t.Parallel()
	v2 := readerFixture(t, "namespace-v2.opt")
	foreign := bytes.ReplaceAll(v2, []byte(nsV2), []byte(nsForeign))
	want := decoded(t, bytes.ReplaceAll(v2, []byte(nsV2), []byte(nsV1)))
	prefixed := `<f:template xmlns:f="` + nsForeign + `"><f:template_id><f:value>t</f:value></f:template_id></f:template>`
	cases := map[string]struct {
		in   []byte
		want *aom14.OperationalTemplate
	}{
		"under the default namespace": {foreign, want},
		"after a byte-order mark":     {append([]byte{0xEF, 0xBB, 0xBF}, foreign...), want},
		"under a prefix":              {[]byte(prefixed), decoded(t, []byte(`<template xmlns="`+nsV1+`"><template_id><value>t</value></template_id></template>`))},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if unmatched := readAs(t, tc.in, tc.want); len(unmatched) > 0 {
				t.Errorf("WithUnmatched reported %q, want nothing", unmatched)
			}
			_, err := aom14.ReadOperationalTemplate(bytes.NewReader(tc.in), canxml.WithStrict())
			if !errors.Is(err, canxml.ErrNamespace) {
				t.Errorf("ReadOperationalTemplate(WithStrict) = %v, want an error wrapping canxml.ErrNamespace", err)
			}
		})
	}
}

// TestREQ111_ReaderReadsWhatElseTheParserReads pins the default mode's
// reading of the other shapes and values the parser's default mode reads: a
// C_COMPLEX_OBJECT that holds an archetype id, as a C_ARCHETYPE_ROOT; a
// <children> typed with a primitive class, as a C_PRIMITIVE_OBJECT whose item
// of that class holds the members but the rm_type_name, occurrences and
// node_id, a C_BOOLEAN without its true_valid among them; an <item> typed
// C_CODE_PHRASE, C_DV_QUANTITY, C_DV_ORDINAL or CONSTRAINT_REF, as the object
// that holds it, of the item's class, and one typed C_PRIMITIVE_OBJECT, whose
// own item that object then holds; and a value of a C_REAL, or a bound of a
// quantity's magnitude or precision, that Go reads and XML Schema's double
// does not, as the number Go reads. WithUnmatched reports what the reading
// leaves out, each item once: a primitive leaf's attributes, in its item, and
// the members of an item that its object has, in that object.
func TestREQ111_ReaderReadsWhatElseTheParserReads(t *testing.T) {
	t.Parallel()
	data := readerFixture(t, "parser-readings.opt")
	want := decoded(t, readerFixture(t, "parser-readings.read.opt"))
	got := readAs(t, data, want)
	wantUnmatched := []string{
		leaf(2) + "/item[1]/attributes[1]",
		leaf(4) + "/rm_type_name[2]",
		leaf(8) + "/rm_type_name[2]",
		leaf(8) + "/occurrences[2]",
		leaf(8) + "/node_id[2]",
	}
	if !slices.Equal(got, wantUnmatched) {
		t.Errorf("WithUnmatched reported\n%q\nwant\n%q", got, wantUnmatched)
	}
}

// TestREQ111_ReaderKeepsTheNamespacesOfAnItemItTakesOff pins that a
// namespace an <item> declares, which the default mode reads as the object
// that holds it, still binds what the item holds, so that it keeps its names.
func TestREQ111_ReaderKeepsTheNamespacesOfAnItemItTakesOff(t *testing.T) {
	t.Parallel()
	const xsi = "http://www.w3.org/2001/XMLSchema-instance"
	doc := itemDoc(`<item xmlns:p="` + xsi + `" p:type="C_PRIMITIVE_OBJECT"><item p:type="C_STRING"><list>a</list></item></item>`)
	want := itemDoc(`<item xsi:type="C_STRING"><list>a</list></item>`)
	readAs(t, []byte(doc), decoded(t, []byte(want)))
}

// TestREQ111_ReaderReadsAnItemThatRebindsANamespace pins that the default
// mode reads an <item> it takes off as the parser reads it where the item
// declares a namespace that the object that holds it binds otherwise or does
// not bind: the default namespace, a prefix bound around it to another
// namespace, or one it binds to the v2 namespace. What the item holds keeps
// the names the item gives it, and WithUnmatched reports the member of an
// item that its object has, in that object. Strict mode refuses the items.
func TestREQ111_ReaderReadsAnItemThatRebindsANamespace(t *testing.T) {
	t.Parallel()
	data := readerFixture(t, "item-namespaces.opt")
	want := decoded(t, readerFixture(t, "item-namespaces.read.opt"))
	got := readAs(t, data, want)
	if want := []string{definition + "/attributes[1]/children[2]/attributes[1]/children[1]/rm_type_name[2]"}; !slices.Equal(got, want) {
		t.Errorf("WithUnmatched reported\n%q\nwant\n%q", got, want)
	}
	if _, err := aom14.ReadOperationalTemplate(bytes.NewReader(data), canxml.WithStrict()); !errors.Is(err, typereg.ErrTypeMismatch) {
		t.Errorf("ReadOperationalTemplate(WithStrict) = %v, want an error wrapping typereg.ErrTypeMismatch", err)
	}

	// The prefix q, bound to one namespace on the object and to another on
	// its item, binds nothing the item holds: the inner item's code phrase
	// keeps its codes.
	t.Run("a prefix that binds nothing the item holds", func(t *testing.T) {
		t.Parallel()
		value := func(children string) string {
			return definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
				`<existence><lower>0</lower><upper>1</upper></existence>` + children + `</attributes>`)
		}
		doc := value(`<children xmlns:q="urn:x-q1" xsi:type="C_PRIMITIVE_OBJECT">` +
			`<rm_type_name>CODE_PHRASE</rm_type_name><occurrences><lower>1</lower><upper>1</upper></occurrences><node_id/>` +
			`<item xmlns:q="urn:x-q2" xsi:type="C_PRIMITIVE_OBJECT"><item xsi:type="C_CODE_PHRASE"><code_list>at1</code_list></item></item></children>`)
		want := value(`<children xsi:type="C_CODE_PHRASE"><rm_type_name>CODE_PHRASE</rm_type_name>` +
			`<occurrences><lower>1</lower><upper>1</upper></occurrences><node_id/><code_list>at1</code_list></children>`)
		readAs(t, []byte(doc), decoded(t, []byte(want)))
	})
}

// TestREQ111_ReaderReadsAChildrenOfAnAttributeClassAsALeaf pins that the
// default mode reads a <children> typed with a class of the constraint model
// a <children> cannot hold, C_SINGLE_ATTRIBUTE or C_MULTIPLE_ATTRIBUTE, as a
// C_COMPLEX_OBJECT leaf with the rm_type_name, occurrences and node_id it
// holds, as the parser reads it. WithUnmatched reports the rest, each item
// once: the leaf's attributes and the members of an attribute it holds.
// TestREQ111_ReaderStrictModeRefusesTheParsersOtherReadings pins strict
// mode's refusal.
func TestREQ111_ReaderReadsAChildrenOfAnAttributeClassAsALeaf(t *testing.T) {
	t.Parallel()
	data := readerFixture(t, "attribute-children.opt")
	want := decoded(t, readerFixture(t, "attribute-children.read.opt"))
	got := readAs(t, data, want)
	value := func(n int) string {
		return definition + "/attributes[1]/children[" + strconv.Itoa(n) + "]/attributes[1]/children[1]"
	}
	wantUnmatched := []string{
		value(1) + "/rm_attribute_name[1]",
		value(1) + "/existence[1]",
		value(1) + "/children[1]",
		value(2) + "/attributes[1]",
		value(2) + "/cardinality[1]",
	}
	if !slices.Equal(got, wantUnmatched) {
		t.Errorf("WithUnmatched reported\n%q\nwant\n%q", got, wantUnmatched)
	}
}

// TestREQ111_ReaderReadsADecimalIntegerBoundInEveryMode pins that, in every
// mode, a bound of a C_INTEGER range or of a quantity's precision written as
// a decimal number, which the parser reads as a float, is read as the integer
// bound that admits the same integers: a lower bound rounded up and an upper
// one down, its side included where the rounding changes it, and keeping its
// flag where the number is a whole one.
func TestREQ111_ReaderReadsADecimalIntegerBoundInEveryMode(t *testing.T) {
	t.Parallel()
	fixture := readerFixture(t, "integer-bounds.opt")
	fixtureWant := decoded(t, readerFixture(t, "integer-bounds.read.opt"))
	integer := func(rng string) string {
		return itemDoc(`<item xsi:type="C_INTEGER"><range>` + rng + `</range></item>`)
	}
	cases := map[string]struct{ doc, want string }{
		"a lower bound, excluded, rounded up": {
			integer(`<lower_included>false</lower_included><lower>2.5</lower><upper>9</upper>`),
			integer(`<lower_included>true</lower_included><lower>3</lower><upper>9</upper>`),
		},
		"an upper bound rounded down": {
			integer(`<lower>0</lower><upper>2.5</upper>`),
			integer(`<lower>0</lower><upper>2</upper><upper_included>true</upper_included>`),
		},
		"negative bounds, an upper one excluded": {
			integer(`<upper_included>false</upper_included><lower>-2.5</lower><upper>-0.5</upper>`),
			integer(`<upper_included>true</upper_included><lower>-2</lower><upper>-1</upper><lower_included>true</lower_included>`),
		},
		"a lower bound rounded up to zero": {
			integer(`<lower> -0.5 </lower><upper>1e1</upper>`),
			integer(`<lower>0</lower><upper>10</upper><lower_included>true</lower_included>`),
		},
		"a quantity's precision": {
			quantityDoc(`<precision><lower_included></lower_included><lower>0.5</lower><upper>2.5</upper></precision>`),
			quantityDoc(`<precision><lower_included>true</lower_included><lower>1</lower><upper>2</upper><upper_included>true</upper_included></precision>`),
		},
		"a quantity's precision of whole numbers": {
			quantityDoc(`<precision><lower>0.0</lower><upper_included>false</upper_included><upper>2.0</upper></precision>`),
			quantityDoc(`<precision><lower>0</lower><upper_included>false</upper_included><upper>2</upper></precision>`),
		},
	}
	for mode, opts := range modes {
		t.Run(mode+"/integer-bounds.opt", func(t *testing.T) {
			t.Parallel()
			if unmatched := readAs(t, fixture, fixtureWant, opts...); len(unmatched) > 0 {
				t.Errorf("WithUnmatched reported %q, want nothing", unmatched)
			}
		})
		for name, tc := range cases {
			t.Run(mode+"/"+name, func(t *testing.T) {
				t.Parallel()
				if unmatched := readAs(t, []byte(tc.doc), decoded(t, []byte(tc.want)), opts...); len(unmatched) > 0 {
					t.Errorf("WithUnmatched reported %q, want nothing", unmatched)
				}
			})
		}
	}
}

// TestREQ111_ReaderReadsAQuantityAssumedValueItCannotHoldAsAbsent pins that,
// in every mode, an assumed_value of a C_DV_QUANTITY that the model cannot
// hold, which the parser ignores, is read as absent: a magnitude that is no
// number, a precision that is no integer, or a bound of its normal range of a
// type that range cannot hold, under a prefix declared around it. An assumed
// value the model holds is kept. The default mode reports what it leaves out;
// strict mode leaves it out unreported. Each case alone reads too, so that a
// failure strict mode alone meets first is read as well, and one whose
// elements are under a prefix bound around it.
func TestREQ111_ReaderReadsAQuantityAssumedValueItCannotHoldAsAbsent(t *testing.T) {
	t.Parallel()
	data := readerFixture(t, "quantity-assumed-value.opt")
	want := decoded(t, readerFixture(t, "quantity-assumed-value.read.opt"))
	assumed := func(n int) string {
		return definition + "/attributes[1]/children[" + strconv.Itoa(n) + "]/attributes[1]/children[1]/assumed_value[1]"
	}
	wantUnmatched := map[string][]string{
		"default": {assumed(1), assumed(2), assumed(3)},
		"strict":  nil,
	}
	alone := map[string]string{
		"a magnitude that is no number": `<assumed_value><magnitude>abc</magnitude><units>kg</units></assumed_value>`,
		"a normal range bound of another type": `<assumed_value><magnitude>1</magnitude><units>kg</units>` +
			`<normal_range><lower xsi:type="DV_COUNT"><magnitude>0</magnitude></lower></normal_range></assumed_value>`,
		"a magnitude that is no number, under a prefix bound around it": `<q:assumed_value><q:magnitude>abc</q:magnitude><q:units>kg</q:units></q:assumed_value>`,
	}
	quantity := func(assumed string) string {
		return definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
			`<existence><lower>0</lower><upper>1</upper></existence><children xmlns:q="` + nsV1 + `" xsi:type="C_DV_QUANTITY">` +
			`<rm_type_name>DV_QUANTITY</rm_type_name><occurrences><lower>1</lower><upper>1</upper></occurrences><node_id/>` +
			`<list><units>kg</units></list>` + assumed + `</children></attributes>`)
	}
	for mode, opts := range modes {
		t.Run(mode+"/quantity-assumed-value.opt", func(t *testing.T) {
			t.Parallel()
			if got := readAs(t, data, want, opts...); !slices.Equal(got, wantUnmatched[mode]) {
				t.Errorf("WithUnmatched reported\n%q\nwant\n%q", got, wantUnmatched[mode])
			}
		})
		for name, av := range alone {
			t.Run(mode+"/"+name, func(t *testing.T) {
				t.Parallel()
				readAs(t, []byte(quantity(av)), decoded(t, []byte(quantity(""))), opts...)
			})
		}
	}
}

// TestREQ111_ReaderStrictModeRefusesTheParsersOtherReadings pins that
// canxml.WithStrict refuses each shape and value
// TestREQ111_ReaderReadsWhatElseTheParserReads reads, and a <children> of an
// attribute class, and that the default mode reads it, in a document of the
// v1 namespace and in one of the v2 namespace, which every mode reads a
// second time.
func TestREQ111_ReaderStrictModeRefusesTheParsersOtherReadings(t *testing.T) {
	t.Parallel()
	const lossy = "0.10000000000000000555111512312578270211815834045410156251"
	cases := []struct {
		name string
		doc  string
		want error
	}{
		{"a C_COMPLEX_OBJECT that holds an archetype id", archetypeIDDoc("C_COMPLEX_OBJECT"), canxml.ErrUnknownElement},
		{"a <children> of a primitive class", childDoc(` xsi:type="C_STRING"`), canxml.ErrUnknownElement},
		{"a <children> of a primitive class with no member", childlessDoc(` xsi:type="C_BOOLEAN"`), typereg.ErrTypeMismatch},
		{"an <item> typed C_CODE_PHRASE", itemDoc(`<item xsi:type="C_CODE_PHRASE"><code_list>at1</code_list></item>`), typereg.ErrTypeMismatch},
		{"an <item> typed C_DV_QUANTITY", itemDoc(`<item xsi:type="C_DV_QUANTITY"><list><units>kg</units></list></item>`), typereg.ErrTypeMismatch},
		{"an <item> typed C_DV_ORDINAL", itemDoc(`<item xsi:type="C_DV_ORDINAL"></item>`), typereg.ErrTypeMismatch},
		{"an <item> typed CONSTRAINT_REF", itemDoc(`<item xsi:type="CONSTRAINT_REF"><reference>ac0001</reference></item>`), typereg.ErrTypeMismatch},
		{"an <item> typed C_PRIMITIVE_OBJECT", itemDoc(`<item xsi:type="C_PRIMITIVE_OBJECT"><item xsi:type="C_STRING"><list>a</list></item></item>`), typereg.ErrTypeMismatch},
		{"a C_REAL list entry of an infinity Go reads", itemDoc(`<item xsi:type="C_REAL"><list>Inf</list></item>`), canxml.ErrInvalidShape},
		{"a C_REAL range bound in hexadecimal", itemDoc(`<item xsi:type="C_REAL"><range><lower>0x1p-2</lower><upper>1</upper></range></item>`), canxml.ErrInvalidShape},
		{"a C_REAL assumed value of more digits than a double holds", itemDoc(`<item xsi:type="C_REAL"><assumed_value>` + lossy + `</assumed_value></item>`), canxml.ErrInvalidShape},
		{"a quantity's magnitude bound of an infinity Go reads", quantityDoc(`<magnitude><lower>0</lower><upper>Inf</upper></magnitude>`), canxml.ErrInvalidShape},
		{"a <children> of an attribute class", childDoc(` xsi:type="C_SINGLE_ATTRIBUTE"`), canxml.ErrUnknownElement},
		{"a <children> of an attribute class with no member", childlessDoc(` xsi:type="C_MULTIPLE_ATTRIBUTE"`), typereg.ErrTypeMismatch},
	}
	for _, tc := range cases {
		for ns, doc := range map[string]string{"v1": tc.doc, "v2": strings.Replace(tc.doc, nsV1, nsV2, 1)} {
			t.Run(ns+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				if _, err := aom14.ReadOperationalTemplate(strings.NewReader(doc)); err != nil {
					t.Fatalf("ReadOperationalTemplate: %v, want the default mode to read it", err)
				}
				_, err := aom14.ReadOperationalTemplate(strings.NewReader(doc), canxml.WithStrict())
				if !errors.Is(err, tc.want) {
					t.Errorf("ReadOperationalTemplate(WithStrict) = %v, want an error wrapping %v", err, tc.want)
				}
			})
		}
	}
}

// TestREQ111_ReaderLeavesOutARealBoundOnASideWrittenUnbounded pins that, in
// every mode, a bound of a C_INTEGER or C_REAL range or of a quantity's
// magnitude or precision that Go reads as a number and canonical XML does not
// is left out on a side the range writes unbounded, whose bound the parser
// does not read.
func TestREQ111_ReaderLeavesOutARealBoundOnASideWrittenUnbounded(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ doc, want string }{
		"a C_REAL range": {
			itemDoc(`<item xsi:type="C_REAL"><range><lower>0</lower><upper_unbounded>true</upper_unbounded><upper>Inf</upper></range></item>`),
			itemDoc(`<item xsi:type="C_REAL"><range><lower>0</lower><upper_unbounded>true</upper_unbounded></range></item>`),
		},
		"a quantity's magnitude": {
			quantityDoc(`<magnitude><lower_unbounded>true</lower_unbounded><lower>0x1p0</lower><upper>9</upper></magnitude>`),
			quantityDoc(`<magnitude><lower_unbounded>true</lower_unbounded><upper>9</upper></magnitude>`),
		},
		"a quantity's precision": {
			quantityDoc(`<precision><lower>0</lower><upper_unbounded>true</upper_unbounded><upper>2.0</upper></precision>`),
			quantityDoc(`<precision><lower>0</lower><upper_unbounded>true</upper_unbounded></precision>`),
		},
		"a C_INTEGER range of a decimal bound": {
			itemDoc(`<item xsi:type="C_INTEGER"><range><lower_unbounded>true</lower_unbounded><lower>2.5</lower><upper>9</upper></range></item>`),
			itemDoc(`<item xsi:type="C_INTEGER"><range><lower_unbounded>true</lower_unbounded><upper>9</upper></range></item>`),
		},
	}
	for mode, opts := range modes {
		for name, tc := range cases {
			t.Run(mode+"/"+name, func(t *testing.T) {
				t.Parallel()
				readAs(t, []byte(tc.doc), decoded(t, []byte(tc.want)), opts...)
			})
		}
	}
}

// TestREQ111_ReaderRefusesMatchNegatedInEveryMode pins that, in every mode,
// an attribute whose match_negated is true is refused, which the parser
// ignores and so reads with its meaning inverted, an attribute the default
// mode types among them, while one whose match_negated is false reads.
func TestREQ111_ReaderRefusesMatchNegatedInEveryMode(t *testing.T) {
	t.Parallel()
	doc := func(attrs, value string) string {
		return definitionDoc(`<attributes` + attrs + `><rm_attribute_name>items</rm_attribute_name>` +
			`<existence><lower>0</lower><upper>1</upper></existence><match_negated>` + value + `</match_negated></attributes>`)
	}
	kinds := map[string]string{
		"a C_SINGLE_ATTRIBUTE":   ` xsi:type="C_SINGLE_ATTRIBUTE"`,
		"a C_MULTIPLE_ATTRIBUTE": ` xsi:type="C_MULTIPLE_ATTRIBUTE"`,
		"an untyped attribute":   ``,
	}
	for mode, opts := range modes {
		for kind, attrs := range kinds {
			if mode == "strict" && attrs == "" {
				continue // strict mode refuses an attribute with no type
			}
			t.Run(mode+"/"+kind, func(t *testing.T) {
				t.Parallel()
				for _, value := range []string{"true", "1"} {
					_, err := aom14.ReadOperationalTemplate(strings.NewReader(doc(attrs, value)), opts...)
					if !errors.Is(err, canxml.ErrInvalidShape) {
						t.Errorf("ReadOperationalTemplate(match_negated %s) = %v, want an error wrapping canxml.ErrInvalidShape", value, err)
					}
				}
				if _, err := aom14.ReadOperationalTemplate(strings.NewReader(doc(attrs, "false")), opts...); err != nil {
					t.Errorf("ReadOperationalTemplate(match_negated false): %v", err)
				}
			})
		}
	}
}

// TestREQ111_ReaderReadsTheShapesTheParserReads pins the default mode's
// reading of the shapes the schemas do not allow inside the definition: an
// <attributes>, a <children> or an <item> that names no type in the XSI
// namespace, a type with white space around it, a <children> of no class of
// the constraint model, read as a leaf, and an <item> of no primitive class.
// WithUnmatched reports what the reading leaves out, each item once: the
// leaf's attributes, the items, the bare type attribute and the type under a
// prefix bound to another namespace.
func TestREQ111_ReaderReadsTheShapesTheParserReads(t *testing.T) {
	t.Parallel()
	data := readerFixture(t, "shapes.opt")
	want := decoded(t, readerFixture(t, "shapes.read.opt"))
	got := readAs(t, data, want)
	items := definition + "/attributes[1]"
	wantUnmatched := []string{
		items + "/children[1]/attributes[1]/children[1]/attributes[1]/@type",
		items + "/children[1]/attributes[1]/children[1]/attributes[1]/children[1]/item[1]",
		items + "/children[2]/attributes[1]/@type",
		items + "/children[3]/attributes[1]",
		items + "/children[5]/attributes[1]/children[1]/attributes[1]/children[1]/item[1]",
	}
	if !slices.Equal(got, wantUnmatched) {
		t.Errorf("WithUnmatched reported\n%q\nwant\n%q", got, wantUnmatched)
	}
}

// TestREQ111_ReaderReadsTheValuesTheParserReads pins the default mode's
// reading of the values the schemas do not allow: a C_INTEGER or C_REAL list
// entry that is no number of its type is left out, an empty one included; a
// range bound that is none is left out, its side unbounded and included
// unless the range says otherwise; a timezone_validity that names no kind is
// left out. WithUnmatched reports each left-out element once.
func TestREQ111_ReaderReadsTheValuesTheParserReads(t *testing.T) {
	t.Parallel()
	data := readerFixture(t, "values.opt")
	want := decoded(t, readerFixture(t, "values.read.opt"))
	got := readAs(t, data, want)
	wantUnmatched := []string{
		item(1) + "/list[2]",
		item(1) + "/list[3]",
		item(1) + "/list[4]",
		item(2) + "/range[1]/lower[1]",
		item(3) + "/range[1]/lower[1]",
		item(4) + "/list[2]",
		item(4) + "/range[1]/upper[1]",
		item(5) + "/timezone_validity[1]",
	}
	if !slices.Equal(got, wantUnmatched) {
		t.Errorf("WithUnmatched reported\n%q\nwant\n%q", got, wantUnmatched)
	}
}

// TestREQ111_ReaderReadsValuesInEveryMode pins the readings that hold in
// every mode: a C_BOOLEAN flag the item leaves out or leaves empty is true;
// an assumed_value of a C_BOOLEAN, C_INTEGER or C_REAL that holds no value of
// its type is absent, a boolean read in any letter case; a bound on a side
// written unbounded, or of a quantity's magnitude, that holds no number is
// left out; an empty timezone_validity is absent. The default mode reports
// what it leaves out; strict mode, which refuses unmatched content, leaves it
// out unreported.
func TestREQ111_ReaderReadsValuesInEveryMode(t *testing.T) {
	t.Parallel()
	data := readerFixture(t, "values-every-mode.opt")
	want := decoded(t, readerFixture(t, "values-every-mode.read.opt"))
	quantity := definition + "/attributes[1]/children[6]/attributes[1]/children[1]/list[1]"
	wantUnmatched := map[string][]string{
		"default": {
			item(1) + "/assumed_value[1]",
			item(4) + "/range[1]/lower[1]",
			item(4) + "/assumed_value[1]",
			item(5) + "/assumed_value[1]",
			quantity + "/magnitude[1]/lower[1]",
			item(7) + "/timezone_validity[1]",
		},
		"strict": nil,
	}
	for mode, opts := range modes {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			if got := readAs(t, data, want, opts...); !slices.Equal(got, wantUnmatched[mode]) {
				t.Errorf("WithUnmatched reported\n%q\nwant\n%q", got, wantUnmatched[mode])
			}
		})
	}
}

// TestREQ111_ReaderFindsWhatCanonicalXMLReadsWithoutAFailure pins the
// readings of a document that canonical XML decodes without a failure, but
// otherwise than the parser: a C_BOOLEAN flag left out or empty, or one that
// stands only in an element the item holds or in the XSI namespace, which it
// reads as false and the reading as absent; an assumed_value that is empty, which it reads as false or
// 0, or that is t or f, which it reads as a boolean; an empty list entry or
// range bound, which it reads as 0; a bare type attribute, which it reads as
// the type; and, which the default mode alone reads, a C_COMPLEX_OBJECT that
// holds an archetype id, which it skips, and a root element in another
// namespace, whose content it skips. Each document holds that one shape, so
// nothing else makes the reader read it a second time. Strict mode refuses
// the shapes its default mode alone reads.
func TestREQ111_ReaderFindsWhatCanonicalXMLReadsWithoutAFailure(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, doc, want string
		strictRefuses   bool
	}{
		{
			name: "a C_BOOLEAN without its true_valid",
			doc:  itemDoc(`<item xsi:type="C_BOOLEAN"><false_valid>true</false_valid></item>`),
			want: itemDoc(`<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>true</false_valid></item>`),
		},
		{
			name: "a C_BOOLEAN with an empty false_valid",
			doc:  itemDoc(`<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid/></item>`),
			want: itemDoc(`<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>true</false_valid></item>`),
		},
		{
			name:          "a C_BOOLEAN whose flags stand in an element it holds",
			doc:           itemDoc(`<item xsi:type="C_BOOLEAN"><x><true_valid>false</true_valid><false_valid>false</false_valid></x></item>`),
			want:          itemDoc(`<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>true</false_valid></item>`),
			strictRefuses: true,
		},
		{
			name:          "a C_BOOLEAN whose flags are in the XSI namespace",
			doc:           itemDoc(`<item xsi:type="C_BOOLEAN"><xsi:true_valid>false</xsi:true_valid><xsi:false_valid>false</xsi:false_valid></item>`),
			want:          itemDoc(`<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>true</false_valid></item>`),
			strictRefuses: true,
		},
		{
			name: "a C_BOOLEAN that assumes t",
			doc:  itemDoc(`<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>true</false_valid><assumed_value>t</assumed_value></item>`),
			want: itemDoc(`<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>true</false_valid></item>`),
		},
		{
			name: "a C_BOOLEAN with an empty assumed_value",
			doc:  itemDoc(`<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>true</false_valid><assumed_value/></item>`),
			want: itemDoc(`<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>true</false_valid></item>`),
		},
		{
			name: "a C_INTEGER with an empty assumed_value",
			doc:  itemDoc(`<item xsi:type="C_INTEGER"><list>1</list><assumed_value></assumed_value></item>`),
			want: itemDoc(`<item xsi:type="C_INTEGER"><list>1</list></item>`),
		},
		{
			name:          "an empty C_INTEGER list entry",
			doc:           itemDoc(`<item xsi:type="C_INTEGER"><list>1</list><list/></item>`),
			want:          itemDoc(`<item xsi:type="C_INTEGER"><list>1</list></item>`),
			strictRefuses: true,
		},
		{
			name:          "an empty C_REAL range bound",
			doc:           itemDoc(`<item xsi:type="C_REAL"><range><lower>0.5</lower><upper></upper></range></item>`),
			want:          itemDoc(`<item xsi:type="C_REAL"><range><upper_included>true</upper_included><upper_unbounded>true</upper_unbounded><lower>0.5</lower></range></item>`),
			strictRefuses: true,
		},
		{
			name: "an empty bound of a quantity's precision",
			doc: definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
				`<existence><lower>0</lower><upper>1</upper></existence><children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name>` +
				`<occurrences><lower>1</lower><upper>1</upper></occurrences><node_id/><list><precision><lower/><upper>2</upper></precision><units>kg</units></list></children></attributes>`),
			want: definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
				`<existence><lower>0</lower><upper>1</upper></existence><children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name>` +
				`<occurrences><lower>1</lower><upper>1</upper></occurrences><node_id/><list><precision><lower_included>true</lower_included><lower_unbounded>true</lower_unbounded><upper>2</upper></precision><units>kg</units></list></children></attributes>`),
		},
		{
			name:          "an <attributes> typed by a bare type attribute",
			doc:           attributeDoc(` type="C_MULTIPLE_ATTRIBUTE"`),
			want:          attributeDoc(` xsi:type="C_SINGLE_ATTRIBUTE"`),
			strictRefuses: true,
		},
		{
			name:          "a C_COMPLEX_OBJECT that holds an archetype id",
			doc:           archetypeIDDoc("C_COMPLEX_OBJECT"),
			want:          archetypeIDDoc("C_ARCHETYPE_ROOT"),
			strictRefuses: true,
		},
		{
			name:          "a root in another namespace",
			doc:           `<template xmlns="` + nsForeign + `"><template_id><value>t</value></template_id></template>`,
			want:          `<template xmlns="` + nsV1 + `"><template_id><value>t</value></template_id></template>`,
			strictRefuses: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			readAs(t, []byte(tc.doc), decoded(t, []byte(tc.want)))
			got, err := aom14.ReadOperationalTemplate(strings.NewReader(tc.doc), canxml.WithStrict())
			switch {
			case tc.strictRefuses && err == nil:
				t.Errorf("ReadOperationalTemplate(WithStrict) = nil error, want a refusal")
			case !tc.strictRefuses && err != nil:
				t.Errorf("ReadOperationalTemplate(WithStrict): %v", err)
			case !tc.strictRefuses:
				if g, w := encoded(t, got), encoded(t, decoded(t, []byte(tc.want))); g != w {
					t.Errorf("ReadOperationalTemplate(WithStrict) reads\n%s\nwant\n%s", g, w)
				}
			}
		})
	}
}

// TestREQ111_ReaderReportsEachItemOnce pins that a caller's report functions
// see each item once when the reader decodes the input a second time: the
// unmatched content of the tolerated input, and the mandatory members it
// leaves out.
func TestREQ111_ReaderReportsEachItemOnce(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"shapes.opt", "values.opt", "values-every-mode.opt", "namespace-v2.opt", "parser-readings.opt",
		"integer-bounds.opt", "quantity-assumed-value.opt", "attribute-children.opt", "item-namespaces.opt",
		"infinite-integer-bounds.opt",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var unmatched, missing []string
			_, err := aom14.ReadOperationalTemplate(bytes.NewReader(readerFixture(t, name)),
				canxml.WithUnmatched(func(path string) { unmatched = append(unmatched, path) }),
				canxml.WithMissing(func(pointer string) { missing = append(missing, pointer) }))
			if err != nil {
				t.Fatalf("ReadOperationalTemplate: %v", err)
			}
			for what, got := range map[string][]string{"WithUnmatched": unmatched, "WithMissing": missing} {
				sorted := slices.Sorted(slices.Values(got))
				if compact := slices.Compact(slices.Clone(sorted)); len(compact) != len(sorted) {
					t.Errorf("%s reported an item more than once: %q", what, got)
				}
			}
		})
	}
	// The first decode meets an unmatched element and an object without its
	// occurrences before the <attributes> that names no type stops it.
	t.Run("items before what the first decode stops at", func(t *testing.T) {
		t.Parallel()
		doc := definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>a</rm_attribute_name>` +
			`<existence><lower>0</lower><upper>1</upper></existence><children xsi:type="C_COMPLEX_OBJECT">` +
			`<rm_type_name>ELEMENT</rm_type_name><node_id>at0001</node_id></children></attributes><x-extra/>` +
			`<attributes><rm_attribute_name>b</rm_attribute_name><existence><lower>0</lower><upper>1</upper></existence></attributes>`)
		var unmatched, missing []string
		_, err := aom14.ReadOperationalTemplate(strings.NewReader(doc),
			canxml.WithUnmatched(func(path string) { unmatched = append(unmatched, path) }),
			canxml.WithMissing(func(pointer string) { missing = append(missing, pointer) }))
		if err != nil {
			t.Fatalf("ReadOperationalTemplate: %v", err)
		}
		if want := []string{definition + "/x-extra[1]"}; !slices.Equal(unmatched, want) {
			t.Errorf("WithUnmatched reported %q, want %q", unmatched, want)
		}
		const occurrences = "/definition/attributes/0/children/0/occurrences"
		if n := strings.Count(strings.Join(missing, " "), occurrences); n != 1 {
			t.Errorf("WithMissing reported %q, want %q once", missing, occurrences)
		}
	})
}

// TestREQ100_ReaderStrictModeRefusesWhatTheParsersStrictModeRefuses pins
// that canxml.WithStrict refuses each shape and value the default mode reads
// in a weaker form, and that the default mode reads it. A bare type attribute
// is no type in the XSI namespace, and an empty list entry or range bound no
// number, which canonical XML reads as its type and as 0.
func TestREQ100_ReaderStrictModeRefusesWhatTheParsersStrictModeRefuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		doc  string
		want error
	}{
		{"an <attributes> with no type", attributeDoc(``), typereg.ErrMissingType},
		{"an <attributes> with an empty type", attributeDoc(` xsi:type=""`), typereg.ErrMissingType},
		{"an <attributes> typed under another namespace", attributeDoc(` xmlns:xsi="urn:x-other" xsi:type="C_SINGLE_ATTRIBUTE"`), typereg.ErrMissingType},
		{"an <attributes> with a bare type alone", attributeDoc(` type="C_SINGLE_ATTRIBUTE"`), typereg.ErrMissingType},
		{"an <attributes> with a padded type", attributeDoc(` xsi:type=" C_SINGLE_ATTRIBUTE "`), typereg.ErrUnknownType},
		{"a <children> with no type", childDoc(``), typereg.ErrMissingType},
		{"a <children> with a bare type alone", childDoc(` type="C_COMPLEX_OBJECT"`), typereg.ErrMissingType},
		{"a <children> of no class", childDoc(` xsi:type="C_FUTURE_OBJECT"`), typereg.ErrUnknownType},
		{"an <item> with no type", itemDoc(`<item><list>1</list></item>`), typereg.ErrMissingType},
		{"an <item> with a bare type alone", itemDoc(`<item type="C_INTEGER"><list>1</list></item>`), typereg.ErrMissingType},
		{"an <item> of no primitive class", itemDoc(`<item xsi:type="C_FUTURE"><list>1</list></item>`), typereg.ErrUnknownType},
		{"a C_INTEGER list entry that is no integer", itemDoc(`<item xsi:type="C_INTEGER"><list>one</list></item>`), canxml.ErrInvalidShape},
		{"an empty C_INTEGER list entry", itemDoc(`<item xsi:type="C_INTEGER"><list/></item>`), canxml.ErrInvalidShape},
		{"an empty C_REAL list entry", itemDoc(`<item xsi:type="C_REAL"><list></list></item>`), canxml.ErrInvalidShape},
		{"a C_INTEGER range bound that is no integer", itemDoc(`<item xsi:type="C_INTEGER"><range><lower>x</lower><upper>9</upper></range></item>`), canxml.ErrInvalidShape},
		{"an empty C_REAL range bound", itemDoc(`<item xsi:type="C_REAL"><range><lower/><upper>9.0</upper></range></item>`), canxml.ErrInvalidShape},
		{"a timezone_validity that names no kind", itemDoc(`<item xsi:type="C_TIME"><pattern>HH:MM</pattern><timezone_validity>sometimes</timezone_validity></item>`), canxml.ErrInvalidShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := aom14.ReadOperationalTemplate(strings.NewReader(tc.doc)); err != nil {
				t.Fatalf("ReadOperationalTemplate: %v, want the default mode to read it", err)
			}
			_, err := aom14.ReadOperationalTemplate(strings.NewReader(tc.doc), canxml.WithStrict())
			if !errors.Is(err, tc.want) {
				t.Errorf("ReadOperationalTemplate(WithStrict) = %v, want an error wrapping %v", err, tc.want)
			}
		})
	}
}

// TestREQ111_ReaderRefusesAnIntegerPastThirtyTwoBits pins that, in every
// mode, an integer the model's 32-bit Integer cannot hold is refused, which
// the parser read, and a range or precision bound written as a decimal that
// rounds to one, while the largest it holds reads.
func TestREQ111_ReaderRefusesAnIntegerPastThirtyTwoBits(t *testing.T) {
	t.Parallel()
	const big = "5000000000"
	refused := map[string]string{
		"a list entry":     itemDoc(`<item xsi:type="C_INTEGER"><list>` + big + `</list></item>`),
		"a range bound":    itemDoc(`<item xsi:type="C_INTEGER"><range><lower>0</lower><upper>` + big + `</upper></range></item>`),
		"an assumed value": itemDoc(`<item xsi:type="C_INTEGER"><assumed_value>` + big + `</assumed_value></item>`),
		"an occurrences upper": definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>a</rm_attribute_name>` +
			`<existence><lower>0</lower><upper>1</upper></existence><children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>` +
			`<occurrences><lower>0</lower><upper>` + big + `</upper></occurrences><node_id>at0001</node_id></children></attributes>`),
		"an existence lower": definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>a</rm_attribute_name>` +
			`<existence><lower>-` + big + `</lower><upper>1</upper></existence></attributes>`),
		"a quantity's precision bound": quantityDoc(`<precision><lower>0</lower><upper>` + big + `</upper></precision>`),
		// A decimal bound past it once rounded is refused as that integer is.
		"a range bound written as a decimal":     itemDoc(`<item xsi:type="C_INTEGER"><range><lower>0</lower><upper>` + big + `.5</upper></range></item>`),
		"a precision bound written as a decimal": quantityDoc(`<precision><lower>-` + big + `.5</lower><upper>2</upper></precision>`),
	}
	for mode, opts := range modes {
		for name, doc := range refused {
			t.Run(mode+"/"+name, func(t *testing.T) {
				t.Parallel()
				_, err := aom14.ReadOperationalTemplate(strings.NewReader(doc), opts...)
				if !errors.Is(err, canxml.ErrInvalidShape) || !errors.Is(err, strconv.ErrRange) {
					t.Errorf("ReadOperationalTemplate = %v, want an error wrapping canxml.ErrInvalidShape and strconv.ErrRange", err)
				}
				if err != nil && strings.Contains(err.Error(), big) {
					t.Errorf("error %q carries the value", err)
				}
			})
		}
		t.Run(mode+"/the largest integer it holds", func(t *testing.T) {
			t.Parallel()
			doc := itemDoc(`<item xsi:type="C_INTEGER"><list>2147483647</list></item>`)
			if _, err := aom14.ReadOperationalTemplate(strings.NewReader(doc), opts...); err != nil {
				t.Errorf("ReadOperationalTemplate: %v", err)
			}
		})
	}
}

// TestREQ111_ReaderReadsAnInfiniteIntegerBoundThatAdmitsEveryInteger pins
// that, in every mode, a bound of a C_INTEGER range or of a quantity's
// precision written as an infinity that admits every integer on its side,
// negative on the lower side and positive on the upper one, in any spelling
// strconv.ParseFloat reads, is read as that side unbounded: the bound is left
// out, an unbounded flag the range writes reads true, and the side is
// included unless the range says otherwise. The default mode reports each
// bound it leaves out; strict mode leaves it out unreported. An infinity that
// admits no integer, on a side written unbounded, is left out as any bound
// there is.
//
// Can-fail control: refuse every infinite bound, as the reader did before,
// and each case is refused.
func TestREQ111_ReaderReadsAnInfiniteIntegerBoundThatAdmitsEveryInteger(t *testing.T) {
	t.Parallel()
	fixture := readerFixture(t, "infinite-integer-bounds.opt")
	fixtureWant := decoded(t, readerFixture(t, "infinite-integer-bounds.read.opt"))
	fixtureUnmatched := []string{
		item(1) + "/range[1]/lower[1]",
		item(2) + "/range[1]/upper[1]",
		definition + "/attributes[1]/children[3]/attributes[1]/children[1]/list[1]/precision[1]/lower[1]",
		definition + "/attributes[1]/children[3]/attributes[1]/children[1]/list[1]/precision[1]/upper[1]",
	}
	integer := func(rng string) string {
		return itemDoc(`<item xsi:type="C_INTEGER"><range>` + rng + `</range></item>`)
	}
	itemRange := definition + "/attributes[1]/children[1]/item[1]/range[1]"
	precision := definition + "/attributes[1]/children[1]/list[1]/precision[1]"
	cases := map[string]struct {
		doc, want string
		// unmatched is what the default mode reports.
		unmatched []string
	}{
		"a lower bound of -Inf": {
			integer(`<lower>-Inf</lower><upper>9</upper>`),
			integer(`<lower_included>true</lower_included><lower_unbounded>true</lower_unbounded><upper>9</upper>`),
			[]string{itemRange + "/lower[1]"},
		},
		"an upper bound of +Infinity, written bounded and excluded": {
			integer(`<upper_included>false</upper_included><upper_unbounded>false</upper_unbounded><lower>0</lower><upper> +Infinity </upper>`),
			integer(`<upper_included>false</upper_included><upper_unbounded>true</upper_unbounded><lower>0</lower>`),
			[]string{itemRange + "/upper[1]"},
		},
		"an upper bound of INF, its included flag empty": {
			integer(`<upper_included/><lower>0</lower><upper>INF</upper>`),
			integer(`<upper_included>true</upper_included><upper_unbounded>true</upper_unbounded><lower>0</lower>`),
			[]string{itemRange + "/upper[1]"},
		},
		"a quantity's precision of -infinity and inf": {
			quantityDoc(`<precision><lower>-infinity</lower><upper>inf</upper></precision>`),
			quantityDoc(`<precision><lower_included>true</lower_included><upper_included>true</upper_included>` +
				`<lower_unbounded>true</lower_unbounded><upper_unbounded>true</upper_unbounded></precision>`),
			[]string{precision + "/lower[1]", precision + "/upper[1]"},
		},
		"an infinity that admits no integer on a side written unbounded": {
			integer(`<lower_unbounded>true</lower_unbounded><lower>+Inf</lower><upper>9</upper>`),
			integer(`<lower_unbounded>true</lower_unbounded><upper>9</upper>`),
			[]string{itemRange + "/lower[1]"},
		},
	}
	for mode, opts := range modes {
		t.Run(mode+"/infinite-integer-bounds.opt", func(t *testing.T) {
			t.Parallel()
			want := fixtureUnmatched
			if mode == "strict" {
				want = nil
			}
			if got := readAs(t, fixture, fixtureWant, opts...); !slices.Equal(got, want) {
				t.Errorf("WithUnmatched reported %q, want %q", got, want)
			}
		})
		for name, tc := range cases {
			t.Run(mode+"/"+name, func(t *testing.T) {
				t.Parallel()
				want := tc.unmatched
				if mode == "strict" {
					want = nil
				}
				if got := readAs(t, []byte(tc.doc), decoded(t, []byte(tc.want)), opts...); !slices.Equal(got, want) {
					t.Errorf("WithUnmatched reported %q, want %q", got, want)
				}
			})
		}
	}
}

// TestREQ111_ReaderRefusesAnInfiniteIntegerBoundThatAdmitsNoInteger pins
// that, in every mode, a bound of a C_INTEGER range or of a quantity's
// precision written as an infinity that admits no integer, positive on the
// lower side or negative on the upper one, on a side the range does not write
// unbounded, is refused with the refusal of the decode that reads it as an
// integer: an error that wraps canxml.ErrInvalidShape, with the
// *strconv.NumError of ParseInt beneath it, which wraps strconv.ErrSyntax,
// not strconv.ErrRange as an integer past 32 bits does, and holds the
// infinity as written. The error's text carries none of it.
//
// Can-fail control: read every infinite bound as unbounded on its side, and
// each case reads.
func TestREQ111_ReaderRefusesAnInfiniteIntegerBoundThatAdmitsNoInteger(t *testing.T) {
	t.Parallel()
	integer := func(rng string) string {
		return itemDoc(`<item xsi:type="C_INTEGER"><range>` + rng + `</range></item>`)
	}
	cases := map[string]struct {
		doc     string
		written string
		sign    int
	}{
		"a C_INTEGER lower bound of +Inf":                 {integer(`<lower>+Inf</lower><upper>9</upper>`), "+Inf", 1},
		"a C_INTEGER lower bound of Infinity, included":   {integer(`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><lower>Infinity</lower><upper>9</upper>`), "Infinity", 1},
		"a C_INTEGER upper bound of -Infinity":            {integer(`<lower>0</lower><upper> -Infinity </upper>`), "-Infinity", -1},
		"a quantity's precision lower bound of INF":       {quantityDoc(`<precision><lower>INF</lower><upper>2</upper></precision>`), "INF", 1},
		"a quantity's precision upper bound of -inf":      {quantityDoc(`<precision><lower>0</lower><upper>-inf</upper></precision>`), "-inf", -1},
		"a C_INTEGER range, the other bound -Inf as well": {integer(`<lower>-Inf</lower><upper>-Inf</upper>`), "-Inf", -1},
	}
	for mode, opts := range modes {
		for name, tc := range cases {
			t.Run(mode+"/"+name, func(t *testing.T) {
				t.Parallel()
				_, err := aom14.ReadOperationalTemplate(strings.NewReader(tc.doc), opts...)
				if !errors.Is(err, canxml.ErrInvalidShape) || !errors.Is(err, strconv.ErrSyntax) || errors.Is(err, strconv.ErrRange) {
					t.Fatalf("ReadOperationalTemplate = %v, want an error wrapping canxml.ErrInvalidShape and strconv.ErrSyntax, not strconv.ErrRange", err)
				}
				ne, ok := errors.AsType[*strconv.NumError](err)
				if !ok || ne == nil || ne.Func != "ParseInt" || ne.Num != tc.written {
					t.Fatalf("ReadOperationalTemplate = %v, whose *strconv.NumError is %#v; want ParseInt's of %q", err, ne, tc.written)
				}
				if f, perr := strconv.ParseFloat(ne.Num, 64); perr != nil || !math.IsInf(f, tc.sign) {
					t.Errorf("the refused bound %q reads as %v (%v), want an infinity of sign %d", ne.Num, f, perr, tc.sign)
				}
				if strings.Contains(err.Error(), tc.written) {
					t.Errorf("error %q carries the bound", err)
				}
			})
		}
	}
}

// TestREQ111_CanonicalXMLKeepsNoneOfTheReadersReadings pins that a canxml
// decode called directly reads the fixtures as written: it refuses the
// shapes and values, and reads nothing of the definition in the namespace the
// schemas declare.
func TestREQ111_CanonicalXMLKeepsNoneOfTheReadersReadings(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"shapes.opt", "values.opt", "values-every-mode.opt", "parser-readings.opt",
		"integer-bounds.opt", "quantity-assumed-value.opt", "attribute-children.opt", "item-namespaces.opt",
		"infinite-integer-bounds.opt",
	} {
		var ot aom14.OperationalTemplate
		if err := canxml.Unmarshal(readerFixture(t, name), &ot); err == nil {
			t.Errorf("canxml.Unmarshal(%s) = nil, want the error the reader's readings lift", name)
		}
	}
	var ot aom14.OperationalTemplate
	if err := canxml.Unmarshal(readerFixture(t, "namespace-v2.opt"), &ot); err != nil {
		t.Fatalf("canxml.Unmarshal(namespace-v2.opt): %v", err)
	}
	if len(ot.Definition.Attributes) != 0 || ot.TemplateID.Value != "" {
		t.Errorf("canxml.Unmarshal(namespace-v2.opt) read the foreign namespace: %d attributes, template id %q", len(ot.Definition.Attributes), ot.TemplateID.Value)
	}
}

// item returns the path of the item of the n-th ELEMENT of a values fixture.
func item(n int) string {
	return leaf(n) + "/item[1]"
}

// leaf returns the path of the leaf of the n-th ELEMENT of a values fixture,
// the child of the child of its value.
func leaf(n int) string {
	return definition + "/attributes[1]/children[" + strconv.Itoa(n) + "]/attributes[1]/children[1]/attributes[1]/children[1]"
}

// archetypeIDDoc returns a template whose definition holds a <children> of
// class that holds an archetype id.
func archetypeIDDoc(class string) string {
	return definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
		`<existence><lower>0</lower><upper>1</upper></existence><children xsi:type="` + class + `"><rm_type_name>CLUSTER</rm_type_name>` +
		`<occurrences><lower>0</lower><upper>1</upper></occurrences><node_id>at0000</node_id>` +
		`<archetype_id><value>openEHR-EHR-CLUSTER.inner.v0</value></archetype_id></children></attributes>`)
}

// quantityDoc returns a template whose definition holds a C_DV_QUANTITY
// whose one list entry holds members.
func quantityDoc(members string) string {
	return definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
		`<existence><lower>0</lower><upper>1</upper></existence><children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name>` +
		`<occurrences><lower>1</lower><upper>1</upper></occurrences><node_id/><list>` + members + `<units>kg</units></list></children></attributes>`)
}

// definitionDoc returns a template whose definition, a CLUSTER, holds
// content.
func definitionDoc(content string) string {
	return `<template xmlns="` + nsV1 + `" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<template_id><value>t</value></template_id><definition><rm_type_name>CLUSTER</rm_type_name>` +
		`<occurrences><lower>1</lower><upper>1</upper></occurrences><node_id>at0000</node_id>` + content +
		`<archetype_id><value>openEHR-EHR-CLUSTER.t.v0</value></archetype_id></definition></template>`
}

// attributeDoc returns a template whose definition holds an <attributes>
// whose start tag carries attrs.
func attributeDoc(attrs string) string {
	return definitionDoc(`<attributes` + attrs + `><rm_attribute_name>items</rm_attribute_name>` +
		`<existence><lower>0</lower><upper>1</upper></existence></attributes>`)
}

// childDoc returns a template whose definition holds a <children> whose
// start tag carries attrs.
func childDoc(attrs string) string {
	return definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
		`<existence><lower>0</lower><upper>1</upper></existence><children` + attrs + `><rm_type_name>ELEMENT</rm_type_name>` +
		`<occurrences><lower>0</lower><upper>1</upper></occurrences><node_id>at0001</node_id></children></attributes>`)
}

// childlessDoc returns a template whose definition holds an empty <children>
// whose start tag carries attrs.
func childlessDoc(attrs string) string {
	return definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
		`<existence><lower>0</lower><upper>1</upper></existence><children` + attrs + `/></attributes>`)
}

// itemDoc returns a template whose definition holds item, the <item> of a
// C_PRIMITIVE_OBJECT.
func itemDoc(item string) string {
	return definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
		`<existence><lower>0</lower><upper>1</upper></existence><children xsi:type="C_PRIMITIVE_OBJECT">` +
		`<rm_type_name>INTEGER</rm_type_name><occurrences><lower>1</lower><upper>1</upper></occurrences><node_id/>` +
		item + `</children></attributes>`)
}
