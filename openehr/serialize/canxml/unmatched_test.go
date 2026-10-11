package canxml_test

// unmatched_test.go — REQ-170 (wire.md § REQ-170, Unmatched content on
// decode) for canonical XML, on the REQ-056 codec. Content is unmatched when
// it is an element or attribute that names no attribute of the RM type its
// element decodes as, other than `xsi:type` and namespace declarations; an
// element in a namespace other than the openEHR one; or a `Hash` element. The
// default ignores it; WithUnmatched reports every item, in document order, by
// its path; WithStrict refuses the first with canxml.ErrUnknownElement.

import (
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// xmlPlanted is the value every extension item carries. No error text and no
// path may contain it: a path names the item, never its value.
const xmlPlanted = "planted-secret-value"

// xmlExt marks where an extension item goes in an input template.
const xmlExt = "{{ext}}"

// ucNS declares the openEHR default namespace and the XSI prefix.
const ucNS = `xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`

// xmlItem is one extension item: an element, or an attribute when attr is
// set, named name and sitting at path.
type xmlItem struct {
	name string
	attr bool
	path string
}

// el and at build the extension items of a case.
func el(name, path string) xmlItem { return xmlItem{name: name, path: path} }
func at(name, path string) xmlItem { return xmlItem{name: name, attr: true, path: path} }

// xmlCase is one input carrying extension items. tmpl holds one xmlExt per
// item, in the same order, and the placeholders appear in document order, so
// the items' paths are also the order a report must follow. sharedNames marks
// a case whose items count one another in their `name[n]`, so an item's path
// holds only with the items before it in place.
type xmlCase struct {
	name        string
	tmpl        string
	items       []xmlItem
	into        func() any
	sharedNames bool
}

// render fills tmpl's placeholders with the case's items, or, with withExt
// false, removes them: the stripped input is what the extended one must
// decode to. An attribute carries xmlPlanted as its value; an element carries
// it as text and inside a child element, so an unmatched element is one item
// whatever it holds.
func (c xmlCase) render(t *testing.T, withExt bool) []byte {
	t.Helper()
	parts := strings.Split(c.tmpl, xmlExt)
	if len(parts) != len(c.items)+1 {
		t.Fatalf("%s: %d placeholders, %d items; want one item per placeholder", c.name, len(parts)-1, len(c.items))
	}
	var b strings.Builder
	for i, p := range parts {
		b.WriteString(p)
		if i == len(c.items) || !withExt {
			continue
		}
		it := c.items[i]
		if it.attr {
			b.WriteString(" " + it.name + "=" + strconv.Quote(xmlPlanted))
		} else {
			b.WriteString("<" + it.name + ">" + xmlPlanted + "<x-inner>" + xmlPlanted + "</x-inner></" + it.name + ">")
		}
	}
	return []byte(b.String())
}

// paths returns the paths of the case's items, in document order.
func (c xmlCase) paths() []string {
	out := make([]string, len(c.items))
	for i, it := range c.items {
		out[i] = it.path
	}
	return out
}

// Shared fragments of the composition template.
const (
	xText     = `<value>n</value>`
	xLanguage = `<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>`
	xTime     = `<value>2026-01-01T00:00:00Z</value>`
)

// xmlCases puts extension items everywhere REQ-170's table reaches in
// canonical XML: the root of a COMPOSITION, inside an ENTRY, an EVENT and a
// data value (each in a polymorphic slot), in a narrow slot that falls back
// to its declared parent, on and inside primitive elements, in an interval,
// its boundary flag and its bound (a by-value bound and one that is itself a
// polymorphic slot), and in an AOM 1.4 value, its constraint intervals and
// their primitive bound. One case repeats element names to pin `name[n]`.
func xmlCases() []xmlCase {
	p := "/composition[1]"
	entry := p + "/content[1]"
	event := entry + "/data[1]/events[1]"
	tree := event + "/data[1]"
	value1 := tree + "/items[1]/value[1]"
	value2 := tree + "/items[2]/value[1]"
	composition := `<composition ` + ucNS + ` archetype_node_id="openEHR-EHR-COMPOSITION.encounter.v1"` + xmlExt + `>` +
		`<name>` + xText + `</name>` + xmlExt + xLanguage +
		`<territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory>` +
		`<category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category>` +
		`<composer xsi:type="PARTY_SELF"/>` +
		`<content xsi:type="OBSERVATION" archetype_node_id="openEHR-EHR-OBSERVATION.body_weight.v2"` + xmlExt + `>` +
		`<name>` + xText + `</name>` + xmlExt + xLanguage +
		`<encoding><terminology_id><value>IANA_character-sets</value></terminology_id><code_string>UTF-8</code_string></encoding>` +
		`<subject xsi:type="PARTY_SELF"/>` +
		`<data xsi:type="HISTORY" archetype_node_id="at0002"><name>` + xText + `</name><origin>` + xTime + `</origin>` +
		`<events xsi:type="POINT_EVENT" archetype_node_id="at0003">` + xmlExt + `<name>` + xText + `</name><time>` + xTime + `</time>` +
		`<data xsi:type="ITEM_TREE" archetype_node_id="at0001"><name><value>tree</value>` + xmlExt + `</name>` +
		`<items xsi:type="ELEMENT" archetype_node_id="at0004"><name>` + xText + `</name>` +
		`<value xsi:type="DV_QUANTITY"` + xmlExt + `>` + xmlExt +
		`<magnitude` + xmlExt + `>80.5</magnitude><units>k` + xmlExt + `g</units>` +
		`<normal_range>` + xmlExt +
		`<lower xsi:type="DV_QUANTITY">` + xmlExt + `<magnitude>50</magnitude><units>kg</units></lower>` +
		`<upper xsi:type="DV_QUANTITY"><magnitude>120</magnitude><units>kg</units></upper>` +
		`<lower_unbounded` + xmlExt + `>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`</normal_range></value></items>` +
		`<items xsi:type="ELEMENT" archetype_node_id="at0005"><name>` + xText + `</name>` +
		`<value xsi:type="DV_INTERVAL"><lower xsi:type="DV_QUANTITY">` + xmlExt + `<magnitude>1</magnitude><units>kg</units></lower>` +
		`<upper xsi:type="DV_QUANTITY"><magnitude>2</magnitude><units>kg</units></upper></value></items>` +
		`</data></events></data></content>` + xmlExt + `</composition>`

	return []xmlCase{
		{
			name: "composition",
			tmpl: composition,
			items: []xmlItem{
				at("x-root-attr", p+"/@x-root-attr"),
				el("x-root", p+"/x-root[1]"),
				at("x-entry-attr", entry+"/@x-entry-attr"),
				el("x-entry", entry+"/x-entry[1]"),
				el("x-event", event+"/x-event[1]"),
				el("x-narrow-slot", tree+"/name[1]/x-narrow-slot[1]"),
				at("x-value-attr", value1+"/@x-value-attr"),
				el("x-value", value1+"/x-value[1]"),
				at("x-leaf-attr", value1+"/magnitude[1]/@x-leaf-attr"),
				el("x-in-leaf", value1+"/units[1]/x-in-leaf[1]"),
				el("x-interval", value1+"/normal_range[1]/x-interval[1]"),
				el("x-bound", value1+"/normal_range[1]/lower[1]/x-bound[1]"),
				at("x-flag-attr", value1+"/normal_range[1]/lower_unbounded[1]/@x-flag-attr"),
				el("x-slot-bound", value2+"/lower[1]/x-slot-bound[1]"),
				el("x-root-tail", p+"/x-root-tail[1]"),
			},
			into: func() any { return new(rm.Composition) },
		},
		{
			// AOM 1.4: a C_COMPLEX_OBJECT, its `occurrences` (the abstract BASE
			// Interval, whose Integer bound decodes as a primitive), a
			// C_SINGLE_ATTRIBUTE in a polymorphic slot, and that attribute's
			// `existence` interval.
			name: "AOM 1.4 value",
			tmpl: `<definition ` + ucNS + xmlExt + `><rm_type_name>OBSERVATION</rm_type_name>` +
				`<occurrences>` + xmlExt + `<lower` + xmlExt + `>1</lower><upper>1</upper>` +
				`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded></occurrences>` +
				`<node_id>at0000</node_id>` +
				`<attributes xsi:type="C_SINGLE_ATTRIBUTE">` + xmlExt + `<rm_attribute_name>data</rm_attribute_name>` +
				`<existence>` + xmlExt + `<lower>1</lower><upper>1</upper></existence></attributes></definition>`,
			items: []xmlItem{
				at("x-object", "/definition[1]/@x-object"),
				el("x-occurrences", "/definition[1]/occurrences[1]/x-occurrences[1]"),
				at("x-bound", "/definition[1]/occurrences[1]/lower[1]/@x-bound"),
				el("x-attribute", "/definition[1]/attributes[1]/x-attribute[1]"),
				el("x-existence", "/definition[1]/attributes[1]/existence[1]/x-existence[1]"),
			},
			into: func() any { return new(aom14.CComplexObject) },
		},
		{
			// `name[n]` counts the earlier siblings of the same name: an
			// extension between two `items` does not move the second, and two
			// extensions of one name are [1] and [2] wherever they sit.
			name: "repeated names",
			tmpl: `<item_tree ` + ucNS + ` archetype_node_id="at0001"><name><value>tree</value></name>` +
				`<items xsi:type="ELEMENT" archetype_node_id="at1"><name><value>a</value></name></items>` + xmlExt +
				`<items xsi:type="ELEMENT" archetype_node_id="at2"><name><value>b</value>` + xmlExt + xmlExt + `</name>` + xmlExt + `</items>` +
				xmlExt + `</item_tree>`,
			items: []xmlItem{
				el("x-note", "/item_tree[1]/x-note[1]"),
				el("x-note", "/item_tree[1]/items[2]/name[1]/x-note[1]"),
				el("x-note", "/item_tree[1]/items[2]/name[1]/x-note[2]"),
				el("x-other", "/item_tree[1]/items[2]/x-other[1]"),
				el("x-note", "/item_tree[1]/x-note[2]"),
			},
			into:        func() any { return new(rm.ItemTree) },
			sharedNames: true,
		},
	}
}

// unmarshalCollecting decodes in into a fresh value with WithUnmatched (and
// the extra options) and returns the value, the paths reported and the error.
func unmarshalCollecting(into func() any, in []byte, opts ...canxml.DecoderOption) (any, []string, error) {
	var got []string
	v := into()
	opts = append(opts, canxml.WithUnmatched(func(p string) { got = append(got, p) }))
	err := canxml.Unmarshal(in, v, opts...)
	return v, got, err
}

// assertXMLStrictRefusal checks every facet REQ-170 gives a strict refusal:
// it wraps canxml.ErrUnknownElement, and canxml.ErrNamespace exactly when
// foreign is set; it comes back on its own, not inside a dispatch envelope;
// it names the first unmatched item's path; it carries no value from the
// input; and it does not match canxml.ErrInvalidShape.
func assertXMLStrictRefusal(t *testing.T, call string, err error, first string, foreign bool) {
	t.Helper()
	if !errors.Is(err, canxml.ErrUnknownElement) {
		t.Fatalf("%s err = %v; want errors.Is(_, canxml.ErrUnknownElement)", call, err)
	}
	if got := errors.Is(err, canxml.ErrNamespace); got != foreign {
		t.Errorf("%s err = %v; errors.Is(_, canxml.ErrNamespace) = %v, want %v", call, err, got, foreign)
	}
	if de, ok := errors.AsType[*canxml.DecodeError](err); ok && de != nil {
		t.Errorf("%s err = %v; want the refusal alone, not inside a *canxml.DecodeError", call, err)
	}
	if !strings.Contains(err.Error(), strconv.Quote(first)) {
		t.Errorf("%s err = %q; want it to name the first unmatched item %s", call, err, strconv.Quote(first))
	}
	if strings.Contains(err.Error(), xmlPlanted) {
		t.Errorf("%s err = %q; it carries the item's value %q, want the path alone", call, err, xmlPlanted)
	}
	if errors.Is(err, canxml.ErrInvalidShape) {
		t.Errorf("%s err = %v; an unknown element must not match canxml.ErrInvalidShape", call, err)
	}
}

// assertNoPlantedValue fails when a reported path carries the planted value.
func assertNoPlantedValue(t *testing.T, paths []string) {
	t.Helper()
	for _, p := range paths {
		if strings.Contains(p, xmlPlanted) {
			t.Errorf("reported path %q carries the item's value %q; want names only", p, xmlPlanted)
		}
	}
}

// TestREQ170_XMLUnmatchedContentDefaultReportStrict drives each case through
// the three modes on canxml.Unmarshal.
//
//   - Default: the input decodes, with no error, to the same value as the
//     input without its extension items.
//   - Report: the function receives each extension item once, in document
//     order, by its path from the root, and the decode returns the same value
//     as the default.
//   - Strict: the decode fails at the first extension item (see
//     assertXMLStrictRefusal); the stripped input passes.
//   - Report and strict together: the function sees the first item, then the
//     decode is refused.
func TestREQ170_XMLUnmatchedContentDefaultReportStrict(t *testing.T) {
	for _, c := range xmlCases() {
		t.Run(c.name, func(t *testing.T) {
			in, stripped := c.render(t, true), c.render(t, false)

			want := c.into()
			if err := canxml.Unmarshal(stripped, want); err != nil {
				t.Fatalf("canxml.Unmarshal(stripped input) = %v; the case's base input must decode", err)
			}

			t.Run("default", func(t *testing.T) {
				got := c.into()
				if err := canxml.Unmarshal(in, got); err != nil {
					t.Fatalf("canxml.Unmarshal = %v; want nil: unmatched content is ignored by default", err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("canxml.Unmarshal decoded\n%#v\nwant the value of the input without its extension items\n%#v", got, want)
				}
			})

			t.Run("report", func(t *testing.T) {
				got, paths, err := unmarshalCollecting(c.into, in)
				if err != nil {
					t.Fatalf("canxml.Unmarshal(WithUnmatched) = %v; want nil: the report option does not refuse", err)
				}
				if !slices.Equal(paths, c.paths()) {
					t.Errorf("WithUnmatched reported\n%q\nwant (each item once, in document order)\n%q", paths, c.paths())
				}
				assertNoPlantedValue(t, paths)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("canxml.Unmarshal(WithUnmatched) decoded\n%#v\nwant the default's value\n%#v", got, want)
				}
			})

			t.Run("strict", func(t *testing.T) {
				err := canxml.Unmarshal(in, c.into(), canxml.WithStrict())
				assertXMLStrictRefusal(t, "canxml.Unmarshal(WithStrict)", err, c.items[0].path, false)
				if err := canxml.Unmarshal(stripped, c.into(), canxml.WithStrict()); err != nil {
					t.Errorf("canxml.Unmarshal(stripped input, WithStrict) = %v; want nil: nothing in it is unmatched", err)
				}
			})

			t.Run("report and strict", func(t *testing.T) {
				_, paths, err := unmarshalCollecting(c.into, in, canxml.WithStrict())
				assertXMLStrictRefusal(t, "canxml.Unmarshal(WithStrict, WithUnmatched)", err, c.items[0].path, false)
				if !slices.Equal(paths, c.paths()[:1]) {
					t.Errorf("WithUnmatched under WithStrict reported %q; want %q, the item the decode stops at", paths, c.paths()[:1])
				}
			})
		})
	}
}

// TestREQ170_XMLStrictRefusesEachItemKind decodes, one input at a time, each
// case's template carrying a single one of its extension items, and requires
// strict to refuse it. Each guard is so shown to refuse on its own, not only
// as the first item of a document. A case whose items count one another in
// `name[n]` is left to the test above.
func TestREQ170_XMLStrictRefusesEachItemKind(t *testing.T) {
	for _, c := range xmlCases() {
		if c.sharedNames {
			continue
		}
		parts := strings.Split(c.tmpl, xmlExt)
		for i, it := range c.items {
			t.Run(c.name+"/"+strconv.Itoa(i)+"/"+it.name, func(t *testing.T) {
				// Keep the i-th placeholder only; the others render as nothing.
				var b strings.Builder
				for j, part := range parts {
					b.WriteString(part)
					if j == i {
						b.WriteString(xmlExt)
					}
				}
				single := xmlCase{name: c.name, tmpl: b.String(), items: []xmlItem{it}, into: c.into}
				err := canxml.Unmarshal(single.render(t, true), c.into(), canxml.WithStrict())
				assertXMLStrictRefusal(t, "canxml.Unmarshal(WithStrict)", err, it.path, false)
			})
		}
	}
}

// TestREQ170_XMLForeignNamespace pins that an element in a namespace other
// than the openEHR one is unmatched even when its local name is that of an RM
// attribute, and that an element in no namespace is an openEHR one. A foreign
// element's strict refusal wraps canxml.ErrNamespace as well as
// canxml.ErrUnknownElement; a foreign attribute's wraps only the latter.
func TestREQ170_XMLForeignNamespace(t *testing.T) {
	const ext = `xmlns:ext="urn:example:extension"`
	base := func(attr, inValue string) []byte {
		return []byte(`<element ` + ucNS + ` ` + ext + ` archetype_node_id="at1"` + attr + `><name><value>n</value></name>` +
			`<value xsi:type="DV_QUANTITY"><magnitude>80.5</magnitude><units>kg</units>` + inValue + `</value></element>`)
	}
	want := new(rm.Element)
	if err := canxml.Unmarshal(base("", ""), want); err != nil {
		t.Fatalf("canxml.Unmarshal(base input) = %v; want nil", err)
	}

	for _, tc := range []struct {
		name    string
		in      []byte
		path    string
		foreign bool
	}{
		{
			// Matched by local name alone, this element would overwrite the
			// units read before it.
			name:    "element named like an RM attribute",
			in:      base("", `<ext:units>`+xmlPlanted+`</ext:units>`),
			path:    "/element[1]/value[1]/units[2]",
			foreign: true,
		},
		{
			name:    "element named like nothing",
			in:      base("", `<ext:note>`+xmlPlanted+`</ext:note>`),
			path:    "/element[1]/value[1]/note[1]",
			foreign: true,
		},
		{
			name:    "foreign default namespace",
			in:      base("", `<units xmlns="urn:example:extension">`+xmlPlanted+`</units>`),
			path:    "/element[1]/value[1]/units[2]",
			foreign: true,
		},
		{
			// The element's namespace name is "b", which is also a prefix
			// bound to the openEHR namespace. A name is resolved once, so a
			// tracked decode sees the element as foreign, as the default does.
			name:    "namespace name spelled like a bound prefix",
			in:      base("", `<p:units xmlns:p="b" xmlns:b="http://schemas.openehr.org/v1">`+xmlPlanted+`</p:units>`),
			path:    "/element[1]/value[1]/units[2]",
			foreign: true,
		},
		{
			// Matched by local name alone, this attribute would overwrite the
			// archetype node id.
			name: "attribute named like an RM attribute",
			in:   base(` ext:archetype_node_id="`+xmlPlanted+`"`, ""),
			path: "/element[1]/@archetype_node_id",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := new(rm.Element)
			if err := canxml.Unmarshal(tc.in, got); err != nil {
				t.Fatalf("canxml.Unmarshal = %v; want nil", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("canxml.Unmarshal decoded\n%#v\nwant the value without the foreign item\n%#v", got, want)
			}
			_, paths, err := unmarshalCollecting(func() any { return new(rm.Element) }, tc.in)
			if err != nil {
				t.Fatalf("canxml.Unmarshal(WithUnmatched) = %v; want nil", err)
			}
			if wantPaths := []string{tc.path}; !slices.Equal(paths, wantPaths) {
				t.Errorf("WithUnmatched reported %q; want %q", paths, wantPaths)
			}
			err = canxml.Unmarshal(tc.in, new(rm.Element), canxml.WithStrict())
			assertXMLStrictRefusal(t, "canxml.Unmarshal(WithStrict)", err, tc.path, tc.foreign)
		})
	}

	t.Run("no namespace is the openEHR one", func(t *testing.T) {
		in := []byte(`<element archetype_node_id="at1"><name><value>n</value></name>` +
			`<value xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="DV_QUANTITY"><magnitude>80.5</magnitude><units>kg</units></value></element>`)
		got := new(rm.Element)
		if err := canxml.Unmarshal(in, got, canxml.WithStrict()); err != nil {
			t.Fatalf("canxml.Unmarshal(no namespace, WithStrict) = %v; want nil: an element in no namespace is an openEHR one", err)
		}
		if q, ok := got.Value.(*rm.DVQuantity); !ok || q.Units != "kg" {
			t.Errorf("Value = %#v; want a DV_QUANTITY in kg", got.Value)
		}
	})
}

// TestREQ170_XMLHashElements pins the rule for `Hash` members, which the
// codec does not model: on a resource-metadata type, by default a Hash
// element is skipped, the report option reports it, and strict refuses it
// like unmatched content (canxml.ErrUnknownElement, not canxml.ErrNamespace).
func TestREQ170_XMLHashElements(t *testing.T) {
	in := []byte(`<description ` + ucNS + `>` +
		`<original_author id="name">` + xmlPlanted + `</original_author>` +
		`<original_author id="email">` + xmlPlanted + `</original_author>` +
		`<lifecycle_state>published</lifecycle_state>` +
		`<other_details><item id="k">` + xmlPlanted + `</item></other_details>` +
		`</description>`)
	wantPaths := []string{
		"/description[1]/original_author[1]",
		"/description[1]/original_author[2]",
		"/description[1]/other_details[1]",
	}

	var got rm.ResourceDescription
	if err := canxml.Unmarshal(in, &got); err != nil {
		t.Fatalf("canxml.Unmarshal = %v; want nil: a Hash element is skipped by default", err)
	}
	if want := (rm.ResourceDescription{LifecycleState: "published"}); !reflect.DeepEqual(got, want) {
		t.Errorf("canxml.Unmarshal decoded %#v; want %#v, the Hash members left out", got, want)
	}

	_, paths, err := unmarshalCollecting(func() any { return new(rm.ResourceDescription) }, in)
	if err != nil {
		t.Fatalf("canxml.Unmarshal(WithUnmatched) = %v; want nil", err)
	}
	if !slices.Equal(paths, wantPaths) {
		t.Errorf("WithUnmatched reported %q; want %q", paths, wantPaths)
	}

	err = canxml.Unmarshal(in, new(rm.ResourceDescription), canxml.WithStrict())
	assertXMLStrictRefusal(t, "canxml.Unmarshal(WithStrict)", err, wantPaths[0], false)
}

// TestREQ170_XMLAnyMemberContent pins how the content of a BMM `Any` member is
// read, here the AOM 1.4 `assumed_value`, which the codec cannot decode. The
// element names an attribute of its type, so it is matched; `Any` declares no
// attributes, so each attribute (bar `xsi:type`) and child element it carries
// is unmatched. The default decodes as before, the report option names each
// item, and strict refuses the first.
func TestREQ170_XMLAnyMemberContent(t *testing.T) {
	in := []byte(`<definition ` + ucNS + `><rm_type_name>DV_TEXT</rm_type_name><node_id>at1</node_id>` +
		`<assumed_value xsi:type="DV_TEXT" x-attr="` + xmlPlanted + `"><value>` + xmlPlanted + `</value></assumed_value></definition>`)
	wantPaths := []string{"/definition[1]/assumed_value[1]/@x-attr", "/definition[1]/assumed_value[1]/value[1]"}

	want := new(aom14.CComplexObject)
	if err := canxml.Unmarshal(in, want); err != nil {
		t.Fatalf("canxml.Unmarshal = %v; want nil", err)
	}
	got, paths, err := unmarshalCollecting(func() any { return new(aom14.CComplexObject) }, in)
	if err != nil {
		t.Fatalf("canxml.Unmarshal(WithUnmatched) = %v; want nil", err)
	}
	if !slices.Equal(paths, wantPaths) {
		t.Errorf("WithUnmatched reported %q; want %q", paths, wantPaths)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("canxml.Unmarshal(WithUnmatched) decoded\n%#v\nwant the default's value\n%#v", got, want)
	}
	err = canxml.Unmarshal(in, new(aom14.CComplexObject), canxml.WithStrict())
	assertXMLStrictRefusal(t, "canxml.Unmarshal(WithStrict)", err, wantPaths[0], false)
}

// TestREQ170_XMLDecoderUnmatchedOptions drives the streaming entry point: a
// Decoder takes the same options, and the paths it reports are relative to
// the value being decoded, so the second value of a stream reports from its
// own root. A nil option and a nil report function change nothing.
func TestREQ170_XMLDecoderUnmatchedOptions(t *testing.T) {
	stream := `<dv_text ` + ucNS + `><x-one>` + xmlPlanted + `</x-one><value>a</value></dv_text>` +
		`<dv_text ` + ucNS + `><value>b</value><x-two>` + xmlPlanted + `</x-two></dv_text>`

	var got []string
	dec := canxml.NewDecoder(strings.NewReader(stream), nil, canxml.WithUnmatched(func(p string) { got = append(got, p) }))
	var first, second rm.DVText
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("Decode(first value) = %v; want nil", err)
	}
	if err := dec.Decode(&second); err != nil {
		t.Fatalf("Decode(second value) = %v; want nil", err)
	}
	if want := []string{"/dv_text[1]/x-one[1]", "/dv_text[1]/x-two[1]"}; !slices.Equal(got, want) {
		t.Errorf("Decoder with WithUnmatched reported %q; want %q", got, want)
	}
	if first.Value != "a" || second.Value != "b" {
		t.Errorf("Decoder decoded %q and %q; want \"a\" and \"b\"", first.Value, second.Value)
	}

	strict := canxml.NewDecoder(strings.NewReader(stream), canxml.WithStrict())
	err := strict.Decode(new(rm.DVText))
	assertXMLStrictRefusal(t, "Decoder(WithStrict).Decode", err, "/dv_text[1]/x-one[1]", false)

	plain := canxml.NewDecoder(strings.NewReader(stream), canxml.WithUnmatched(nil))
	if err := plain.Decode(new(rm.DVText)); err != nil {
		t.Errorf("Decoder(WithUnmatched(nil)).Decode = %v; want nil: a nil function is the default", err)
	}
}

// TestREQ170_XMLConcurrentDecodesKeepTheirOwnReports runs decodes in
// parallel, each with its own options and its own extension element: some
// report, some refuse, some take the default. Each must see exactly its own
// item. The tracking state is held per decoder, so concurrent decodes never
// see each other's items or policy. Run it under -race.
func TestREQ170_XMLConcurrentDecodesKeepTheirOwnReports(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 12 {
		name := "x-decode-" + strconv.Itoa(i)
		in := []byte(`<element ` + ucNS + ` archetype_node_id="at1"><name><value>n</value></name>` +
			`<value xsi:type="DV_QUANTITY"><magnitude>1</magnitude><units>kg</units><` + name + `/></value></element>`)
		path := "/element[1]/value[1]/" + name + "[1]"
		wg.Go(func() {
			for range 200 {
				switch i % 3 {
				case 0:
					var got []string
					err := canxml.Unmarshal(in, new(rm.Element), canxml.WithUnmatched(func(p string) { got = append(got, p) }))
					if err != nil || !slices.Equal(got, []string{path}) {
						t.Errorf("%s: canxml.Unmarshal(WithUnmatched) = %v, reported %q; want nil and [%q]", name, err, got, path)
						return
					}
				case 1:
					err := canxml.Unmarshal(in, new(rm.Element), canxml.WithStrict())
					if !errors.Is(err, canxml.ErrUnknownElement) || !strings.Contains(err.Error(), strconv.Quote(path)) {
						t.Errorf("%s: canxml.Unmarshal(WithStrict) = %v; want a refusal naming %q", name, err, path)
						return
					}
				default:
					if err := canxml.Unmarshal(in, new(rm.Element)); err != nil {
						t.Errorf("%s: canxml.Unmarshal = %v; want nil: the default ignores the item", name, err)
						return
					}
				}
			}
		})
	}
	wg.Wait()
}

// TestREQ170_XMLUnknownTypeFailsInEveryMode pins that an `xsi:type` naming no
// RM class is a dispatch failure in every mode, never unmatched content: the
// slot cannot be decoded at all, so nothing under it is reported either.
func TestREQ170_XMLUnknownTypeFailsInEveryMode(t *testing.T) {
	in := []byte(`<element ` + ucNS + ` archetype_node_id="at1"><name><value>n</value></name>` +
		`<value xsi:type="NO_SUCH_CLASS" x-attr="` + xmlPlanted + `"><x-ext>` + xmlPlanted + `</x-ext></value></element>`)
	for _, mode := range []string{"default", "report", "strict"} {
		t.Run(mode, func(t *testing.T) {
			var reported []string
			report := canxml.WithUnmatched(func(p string) { reported = append(reported, p) })
			var opts []canxml.DecoderOption
			switch mode {
			case "report":
				opts = []canxml.DecoderOption{report}
			case "strict":
				opts = []canxml.DecoderOption{canxml.WithStrict(), report}
			}
			err := canxml.Unmarshal(in, new(rm.Element), opts...)
			if !errors.Is(err, typereg.ErrUnknownType) {
				t.Fatalf("canxml.Unmarshal(%s) err = %v; want errors.Is(_, typereg.ErrUnknownType)", mode, err)
			}
			if errors.Is(err, canxml.ErrUnknownElement) {
				t.Errorf("canxml.Unmarshal(%s) err = %v; an unknown `xsi:type` is a dispatch failure, not an unknown element", mode, err)
			}
			if len(reported) != 0 {
				t.Errorf("canxml.Unmarshal(%s) reported %q; want nothing: the slot is never decoded", mode, reported)
			}
		})
	}
}

// TestREQ170_XMLStrictNeverRefusesAbsentContent pins that strict covers
// unmatched content only: an absent element, mandatory or not, and an interval
// boundary flag REQ-169 defaults, decode under WithStrict, and the flags take
// their defaults.
func TestREQ170_XMLStrictNeverRefusesAbsentContent(t *testing.T) {
	t.Run("interval without flags", func(t *testing.T) {
		in := []byte(`<dv_interval ` + ucNS + `><lower xsi:type="DV_QUANTITY"><magnitude>1</magnitude><units>kg</units></lower>` +
			`<upper xsi:type="DV_QUANTITY"><magnitude>2</magnitude><units>kg</units></upper></dv_interval>`)
		var iv rm.DVInterval[rm.DVQuantity]
		if err := canxml.Unmarshal(in, &iv, canxml.WithStrict()); err != nil {
			t.Fatalf("canxml.Unmarshal(%s, WithStrict) = %v; want nil: an absent flag is not unmatched", in, err)
		}
		if !iv.LowerIncluded || !iv.UpperIncluded || iv.LowerUnbounded || iv.UpperUnbounded {
			t.Errorf("flags = included %v/%v, unbounded %v/%v; want both included and neither unbounded (REQ-169)",
				iv.LowerIncluded, iv.UpperIncluded, iv.LowerUnbounded, iv.UpperUnbounded)
		}
	})
	t.Run("composition without its mandatory elements", func(t *testing.T) {
		in := []byte(`<composition ` + ucNS + `/>`)
		if err := canxml.Unmarshal(in, new(rm.Composition), canxml.WithStrict()); err != nil {
			t.Errorf("canxml.Unmarshal(%s, WithStrict) = %v; want nil: an absent mandatory element is not unmatched", in, err)
		}
	})
}

// TestREQ170_XMLRegisteredTypeCensus decodes, for every registered library
// type (RM and AOM 1.4) with an UnmarshalXML method, an element carrying its
// `xsi:type`, one extension attribute and one extension element, and
// requires the report to name exactly those two. It is the control that no
// generated decoder skips unmatched content unreported.
func TestREQ170_XMLRegisteredTypeCensus(t *testing.T) {
	const libraryPrefix = "github.com/cadasto/openehr-sdk-go/openehr/"
	checked := 0
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		v := ctor()
		if _, ok := v.(xml.Unmarshaler); !ok {
			continue // a primitive, which encoding/xml decodes itself
		}
		rt := reflect.TypeOf(v)
		for rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		if !strings.HasPrefix(rt.PkgPath(), libraryPrefix) {
			continue // a test fixture registered elsewhere; not a shipped type
		}
		checked++
		in := `<x ` + ucNS + ` xsi:type="` + name + `" x-attr="` + xmlPlanted + `"><x-census>` + xmlPlanted + `</x-census></x>`
		var got []string
		err := canxml.Unmarshal([]byte(in), v, canxml.WithUnmatched(func(p string) { got = append(got, p) }))
		if err != nil {
			t.Errorf("%s: canxml.Unmarshal(%s, WithUnmatched) = %v; want nil", name, in, err)
			continue
		}
		if want := []string{"/x[1]/@x-attr", "/x[1]/x-census[1]"}; !slices.Equal(got, want) {
			t.Errorf("%s: WithUnmatched reported %q; want %q", name, got, want)
		}
	}
	if checked < 100 {
		t.Errorf("census covered %d library types; want the full RM + AOM 1.4 inventory", checked)
	}
}

// xmlCorpusExcluded names the corpus documents the codec cannot decode in any
// mode, with the reason. Each fails on a defect outside REQ-170, so it says
// nothing about unmatched content.
var xmlCorpusExcluded = map[string]string{
	"compositions/Demonstration.v1.xml":        "DV_MULTIMEDIA `data` (base64 text) does not decode: a known codec defect outside REQ-170",
	"compositions/TestPerson.v2.xml":           "DV_MULTIMEDIA `data` (base64 text) does not decode: a known codec defect outside REQ-170",
	"crossformat/consult_record/canonical.xml": "DV_MULTIMEDIA `data` (base64 text) does not decode: a known codec defect outside REQ-170",
}

// xmlCorpusUnmatched is every unmatched item the vendored canonical-XML
// corpus carries, by document. The EHRbase family-history export writes the
// values of ARCHETYPE_ID's functions (`qualifiedRmEntity`, `domainConcept`,
// …) as child elements; ARCHETYPE_ID declares only `value` as an attribute
// in the pinned BASE schema, so each is unmatched. The corpus is vendored
// as-is, so the test pins the set rather than editing the documents.
var xmlCorpusUnmatched = func() map[string][]string {
	derived := []string{"qualifiedRmEntity", "domainConcept", "rmOriginator", "rmName", "rmEntity", "versionId"}
	var paths []string
	for _, owner := range []string{"/composition[1]/archetype_details[1]/archetype_id[1]", "/composition[1]/content[1]/archetype_details[1]/archetype_id[1]"} {
		for _, d := range derived {
			paths = append(paths, owner+"/"+d+"[1]")
		}
	}
	return map[string][]string{"crossformat/family_history/canonical.xml": paths}
}()

// TestREQ170_XMLCorpusUnderReportAndStrict decodes every canonical-XML
// document of the corpus (compositions/, rm/ and the cross-format sets) with
// the report option and pins the exact set of unmatched items
// (xmlCorpusUnmatched). Every other document decodes under strict; each
// pinned one is refused at its first item.
func TestREQ170_XMLCorpusUnderReportAndStrict(t *testing.T) {
	root := fixtures.CorpusRoot()
	var rels []string
	for _, dir := range []string{"compositions", "rm", "crossformat"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".xml") {
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				rels = append(rels, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if len(rels) < 20 {
		t.Fatalf("the corpus listed %d canonical-XML documents; want the whole set", len(rels))
	}
	seen := map[string]bool{}
	for _, rel := range rels {
		t.Run(rel, func(t *testing.T) {
			seen[rel] = true
			if reason, ok := xmlCorpusExcluded[rel]; ok {
				t.Skip(reason)
			}
			body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatal(err)
			}
			factory, ok := fixtures.FactoryForXMLBody(body)
			if !ok {
				t.Fatalf("no factory for the root element of %s", rel)
			}
			_, got, err := unmarshalCollecting(factory, body)
			if err != nil {
				t.Fatalf("canxml.Unmarshal(WithUnmatched) = %v; want nil", err)
			}
			want := xmlCorpusUnmatched[rel]
			if !slices.Equal(got, want) {
				t.Errorf("WithUnmatched reported\n%q\nwant\n%q", got, want)
			}
			err = canxml.Unmarshal(body, factory(), canxml.WithStrict())
			if want == nil {
				if err != nil {
					t.Errorf("canxml.Unmarshal(WithStrict) = %v; want nil: the document carries no unmatched item", err)
				}
				return
			}
			assertXMLStrictRefusal(t, "canxml.Unmarshal(WithStrict)", err, want[0], false)
		})
	}
	for rel := range xmlCorpusUnmatched {
		if !seen[rel] {
			t.Errorf("xmlCorpusUnmatched pins %s, which the corpus no longer holds: remove the entry", rel)
		}
	}
	for rel := range xmlCorpusExcluded {
		if !seen[rel] {
			t.Errorf("xmlCorpusExcluded names %s, which the corpus no longer holds: remove the entry", rel)
		}
	}
}

// TestREQ170_XMLSentinelsStayDistinct pins that canxml.ErrUnknownElement,
// canxml.ErrNamespace and canxml.ErrInvalidShape are distinct values, and
// that an `xmi:type` refusal under strict stays canxml.ErrInvalidShape and
// does not match canxml.ErrUnknownElement.
func TestREQ170_XMLSentinelsStayDistinct(t *testing.T) {
	sentinels := map[string]error{
		"ErrUnknownElement": canxml.ErrUnknownElement,
		"ErrNamespace":      canxml.ErrNamespace,
		"ErrInvalidShape":   canxml.ErrInvalidShape,
	}
	for a, ea := range sentinels {
		for b, eb := range sentinels {
			if a != b && errors.Is(ea, eb) {
				t.Errorf("errors.Is(canxml.%s, canxml.%s) = true; want the sentinels distinct", a, b)
			}
		}
	}

	in := []byte(`<element ` + ucNS + ` xmlns:xmi="http://www.omg.org/XMI" archetype_node_id="at1"><name><value>n</value></name>` +
		`<value xmi:type="DV_QUANTITY"><magnitude>1</magnitude><units>kg</units></value></element>`)
	err := canxml.Unmarshal(in, new(rm.Element), canxml.WithStrict())
	if !errors.Is(err, canxml.ErrInvalidShape) || errors.Is(err, canxml.ErrUnknownElement) {
		t.Errorf("canxml.Unmarshal(xmi:type, WithStrict) err = %v; want canxml.ErrInvalidShape and not canxml.ErrUnknownElement", err)
	}
}
