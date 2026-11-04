package aom14_test

// REQ-111 (clinical-modeling.md § REQ-111, Reading the model, "What the
// reader tolerates"): aom14.ReadOperationalTemplate reads every OPT the
// deprecated parser reads in its default mode, as the parser reads it, but an
// integer the model's 32-bit Integer cannot hold, which it refuses in every
// mode. REQ-100 (§ REQ-100, Strict parse mode): given canxml.WithStrict it
// refuses the shapes the parser's strict mode refuses. REQ-170 (wire.md
// § REQ-170): what a tolerance leaves out is unmatched content, which
// canxml.WithUnmatched reports, once, at the path it has in the input.
//
// The fixtures under testdata/reader are SDK-authored: each *.read.opt is the
// reading of the fixture of its name, written as canonical XML decodes it.

import (
	"bytes"
	"errors"
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
// otherwise than the parser: a C_BOOLEAN flag left out or empty, which it
// reads as false; an assumed_value that is empty, which it reads as false or
// 0, or that is t or f, which it reads as a boolean; an empty list entry or
// range bound, which it reads as 0; and a bare type attribute, which it reads
// as the type. Each document holds that one shape, so nothing else makes the
// reader read it a second time. Strict mode refuses the shapes its default
// mode alone reads.
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
	for _, name := range []string{"shapes.opt", "values.opt", "values-every-mode.opt", "namespace-v2.opt"} {
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
// the parser read, while the largest it holds reads.
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

// TestREQ111_CanonicalXMLKeepsNoneOfTheReadersReadings pins that a canxml
// decode called directly reads the fixtures as written: it refuses the
// shapes and values, and reads nothing of the definition in the namespace the
// schemas declare.
func TestREQ111_CanonicalXMLKeepsNoneOfTheReadersReadings(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"shapes.opt", "values.opt", "values-every-mode.opt"} {
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
	return definition + "/attributes[1]/children[" + strconv.Itoa(n) + "]/attributes[1]/children[1]/attributes[1]/children[1]/item[1]"
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

// itemDoc returns a template whose definition holds item, the <item> of a
// C_PRIMITIVE_OBJECT.
func itemDoc(item string) string {
	return definitionDoc(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
		`<existence><lower>0</lower><upper>1</upper></existence><children xsi:type="C_PRIMITIVE_OBJECT">` +
		`<rm_type_name>INTEGER</rm_type_name><occurrences><lower>1</lower><upper>1</upper></occurrences><node_id/>` +
		item + `</children></attributes>`)
}
