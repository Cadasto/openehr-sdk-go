package canxml_test

// REQ-108 (clinical-modeling.md § REQ-108, Canonical-XML decode depth) and
// REQ-056 (wire.md § REQ-056, the codec's decode options): with
// canxml.WithMaxDepth(n), canxml.Unmarshal and a canxml.Decoder refuse, during
// the decode, an element nested deeper than n, the root element at depth 1,
// with a *canxml.DecodeError that wraps canxml.ErrLimitExceeded and names the
// depth and the element's path but none of its content. Without the option a
// decode has no depth limit.

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// depthModes are the ways a caller decodes canonical XML under
// canxml.WithMaxDepth(n): Unmarshal, and a Decoder over each of the stream
// readers, each with no other option, and both with each option that tracks
// the decode. A Decoder given no other option over a *bytes.Reader would seek
// back in it to decode a value again, and over any other reader would keep
// the value's bytes to do so.
func depthModes(t *testing.T, n int) map[string]func(data []byte, v any) error {
	t.Helper()
	return limitModes(t, canxml.WithMaxDepth(n))
}

// limitModes are the modes of depthModes, each given limit among its
// options; a nil limit is no option at all, which a decode skips.
func limitModes(t *testing.T, limit canxml.DecoderOption) map[string]func(data []byte, v any) error {
	t.Helper()
	unmarshal := func(opts ...canxml.DecoderOption) func([]byte, any) error {
		all := slices.Concat(opts, []canxml.DecoderOption{limit})
		return func(data []byte, v any) error { return canxml.Unmarshal(data, v, all...) }
	}
	decoder := func(reader func([]byte) io.Reader, opts ...canxml.DecoderOption) func([]byte, any) error {
		all := slices.Concat(opts, []canxml.DecoderOption{limit})
		return func(data []byte, v any) error { return canxml.NewDecoder(reader(data), all...).Decode(v) }
	}
	report := canxml.WithUnmatched(func(p string) { t.Errorf("reported %s; want nothing", p) })
	missing := canxml.WithMissing(func(string) {})
	modes := map[string]func([]byte, any) error{
		"Unmarshal":               unmarshal(),
		"Unmarshal WithUnmatched": unmarshal(report),
		"Unmarshal WithStrict":    unmarshal(canxml.WithStrict()),
		"Unmarshal WithMissing":   unmarshal(missing),
		"Decoder WithUnmatched":   decoder(streamReaders["whole"], report),
		"Decoder WithStrict":      decoder(streamReaders["whole"], canxml.WithStrict()),
		"Decoder WithMissing":     decoder(streamReaders["whole"], missing),
	}
	for name, reader := range streamReaders {
		modes["Decoder, "+name] = decoder(reader)
	}
	return modes
}

// nestedClusters returns an ITEM_TREE that holds levels CLUSTER items, each
// inside the one before, and innermost inside the last. The innermost CLUSTER
// is at depth levels+1, the root at depth 1.
func nestedClusters(levels int, innermost string) []byte {
	var b strings.Builder
	b.WriteString(`<item_tree ` + ucNS + ` archetype_node_id="at0">`)
	for range levels {
		b.WriteString(`<items xsi:type="CLUSTER" archetype_node_id="at1">`)
	}
	b.WriteString(innermost)
	for range levels {
		b.WriteString(`</items>`)
	}
	b.WriteString(`</item_tree>`)
	return []byte(b.String())
}

// clusterPath returns the path of the item of nestedClusters at depth d.
func clusterPath(d int) string {
	return "/item_tree[1]" + strings.Repeat("/items[1]", d-1)
}

// checkDepthRefusal checks that err refuses the element at path, nested at
// depth, one past limit: a *canxml.DecodeError on its own, outside any slot
// envelope, that wraps canxml.ErrLimitExceeded and whose text names the path,
// the depth and the limit, and nothing else.
func checkDepthRefusal(t *testing.T, mode string, err error, path string, depth, limit int) {
	t.Helper()
	if !errors.Is(err, canxml.ErrLimitExceeded) {
		t.Errorf("%s: err = %.300v; want errors.Is(_, canxml.ErrLimitExceeded)", mode, err)
		return
	}
	de, ok := errors.AsType[*canxml.DecodeError](err)
	if !ok || de == nil {
		t.Errorf("%s: err = %T %.300v; want a *canxml.DecodeError", mode, err, err)
		return
	}
	if de.Path != path || de.Type != "" {
		t.Errorf("%s: DecodeError{Path: %q, Type: %q}; want {Path: %q}, the refused element on its own", mode, de.Path, de.Type, path)
	}
	want := fmt.Sprintf("decode %s: canxml: decode limit exceeded: element depth (%d > %d)", path, depth, limit)
	if msg := err.Error(); msg != want {
		t.Errorf("%s: err.Error() = %q; want %q", mode, msg, want)
	}
}

// TestREQ108_REQ056_XMLMaxDepthRefusesOnePastTheBound pins the bound in every
// mode: a document whose deepest element sits at depth n decodes under
// WithMaxDepth(n), and under WithMaxDepth(n-1) is refused at that element,
// named by its path from the root, with its depth and the limit.
//
// Can-fail control: take out the depth check in the token source, or count
// from 0 rather than from the root at 1, and the second half fails.
func TestREQ108_REQ056_XMLMaxDepthRefusesOnePastTheBound(t *testing.T) {
	const levels = 4 // the innermost CLUSTER is at depth 5
	in := nestedClusters(levels, "")
	for mode, decode := range depthModes(t, levels+1) {
		if err := decode(in, new(rm.ItemTree)); err != nil {
			t.Errorf("%s, WithMaxDepth(%d), at the bound: err = %v; want nil", mode, levels+1, err)
		}
	}
	for mode, decode := range depthModes(t, levels) {
		err := decode(in, new(rm.ItemTree))
		checkDepthRefusal(t, mode+", one past the bound", err, clusterPath(levels+1), levels+1, levels)
	}
}

// TestREQ108_XMLMaxDepthRefusesBeforeTheContent pins that the refusal comes as
// the start tag one past the bound is read, before anything decodes the
// element: an item whose `xsi:type` names no class and whose content carries
// text is refused for its depth, not for its type, and the error carries
// neither its type nor its text.
//
// Can-fail control: without the limit the same document fails on the item's
// type, which shows the decode reaches the item; a check made only once the
// element's decoder has read its discriminator fails here the same way.
func TestREQ108_XMLMaxDepthRefusesBeforeTheContent(t *testing.T) {
	const levels = 3
	in := nestedClusters(levels, `<items xsi:type="NO_SUCH_CLASS" archetype_node_id="at2"><value>Jane Doe 7777</value></items>`)
	if err := canxml.Unmarshal(in, new(rm.ItemTree)); !errors.Is(err, typereg.ErrUnknownType) {
		t.Fatalf("Unmarshal without a limit: err = %v; want typereg.ErrUnknownType, the innermost item's type", err)
	}
	for mode, decode := range depthModes(t, levels+1) {
		err := decode(in, new(rm.ItemTree))
		if errors.Is(err, typereg.ErrUnknownType) {
			t.Errorf("%s: err = %v; want the item refused for its depth before its type is read", mode, err)
			continue
		}
		checkDepthRefusal(t, mode, err, clusterPath(levels+2), levels+2, levels+1)
		if err == nil {
			continue
		}
		for _, v := range []string{"NO_SUCH_CLASS", "Jane", "Doe", "7777"} {
			if strings.Contains(err.Error(), v) {
				t.Errorf("%s: err.Error() = %q; the text carries %q from the document", mode, err.Error(), v)
			}
		}
	}
}

// TestREQ108_XMLMaxDepthPathStaysShort pins that the refusal's path keeps at
// most eight segments at each end, with a `/...` segment in place of the rest,
// so the error stays short however deep the limit lets a document nest: a
// path of sixteen segments is written whole, one of seventeen or more is cut.
//
// Can-fail control: write the refused element's whole path and the cases at
// depth 17 and 1001 fail.
func TestREQ108_XMLMaxDepthPathStaysShort(t *testing.T) {
	cut := "/item_tree[1]" + strings.Repeat("/items[1]", 7) + "/..." + strings.Repeat("/items[1]", 8)
	for _, tc := range []struct {
		limit int
		path  string
	}{
		{limit: 15, path: clusterPath(16)},
		{limit: 16, path: cut},
		{limit: 1000, path: cut},
	} {
		in := nestedClusters(2*tc.limit, "")
		for mode, decode := range depthModes(t, tc.limit) {
			err := decode(in, new(rm.ItemTree))
			checkDepthRefusal(t, fmt.Sprintf("%s, WithMaxDepth(%d)", mode, tc.limit), err, tc.path, tc.limit+1, tc.limit)
		}
	}
}

// TestREQ108_XMLNoMaxDepthWithoutALimit pins that WithMaxDepth(n) with n <= 0
// sets no limit, as leaving the option out does: a document nested 301 deep
// decodes in every mode.
//
// Can-fail control: read 0 as a limit of 0, or a negative n as a small limit,
// and the decode refuses the root or the first item.
func TestREQ108_XMLNoMaxDepthWithoutALimit(t *testing.T) {
	in := nestedClusters(300, "")
	for _, n := range []int{0, -1} {
		for mode, decode := range depthModes(t, n) {
			if err := decode(in, new(rm.ItemTree)); err != nil {
				t.Errorf("%s, WithMaxDepth(%d): err = %.300v; want nil", mode, n, err)
			}
		}
	}
}

// TestREQ108_XMLMaxDepthTheSmallerLimitWins pins that, given more than one
// canxml.WithMaxDepth, a decode keeps the smallest positive limit, in either
// order, and that an n <= 0 neither sets a limit nor lifts one already given.
// A document nested well past every limit is refused at the element one past
// the limit that holds.
//
// Can-fail control: keep the last WithMaxDepth given, as it once did, or let
// a non-positive n clear the limit, and the cases that give the larger or the
// non-positive option last fail.
func TestREQ108_XMLMaxDepthTheSmallerLimitWins(t *testing.T) {
	in := nestedClusters(8, "")
	for _, tc := range []struct {
		name   string
		limits []int
		want   int // the limit that holds
	}{
		{"the smaller first", []int{3, 5}, 3},
		{"the smaller last", []int{5, 3}, 3},
		{"equal limits", []int{4, 4}, 4},
		{"three, the smallest in the middle", []int{5, 3, 4}, 3},
		{"a limit, then none", []int{3, 0}, 3},
		{"none, then a limit", []int{0, 3}, 3},
		{"a limit, then a negative one", []int{3, -1}, 3},
		{"a negative one, then a limit", []int{-1, 3}, 3},
	} {
		opts := make([]canxml.DecoderOption, len(tc.limits))
		for i, n := range tc.limits {
			opts[i] = canxml.WithMaxDepth(n)
		}
		decode := map[string]func() error{
			"Unmarshal": func() error { return canxml.Unmarshal(in, new(rm.ItemTree), opts...) },
			"Decoder": func() error {
				return canxml.NewDecoder(streamReaders["whole"](in), opts...).Decode(new(rm.ItemTree))
			},
		}
		for mode, run := range decode {
			checkDepthRefusal(t, fmt.Sprintf("%s, %s %v", mode, tc.name, tc.limits), run(), clusterPath(tc.want+1), tc.want+1, tc.want)
		}
	}
}

// prefixSlot returns an ARCHETYPE_SLOT whose include's left operand declares
// the `xs` prefix that its item's `xsi:type` uses, so that a decode given no
// option stops at the item and decodes the slot a second time, from its
// start, through a decoder that keeps every declaration in scope. The slot's
// deepest element, the right operand's pattern at depth 6, comes after that
// item; nothing before it is deeper than 5.
func prefixSlot() []byte {
	return []byte(`<archetype_slot xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<rm_type_name>CLUSTER</rm_type_name><occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></occurrences>` +
		`<node_id>at0001</node_id>` +
		`<includes><expression xsi:type="EXPR_BINARY_OPERATOR"><type>Boolean</type><operator>2007</operator>` +
		`<precedence_overridden>false</precedence_overridden>` +
		`<left_operand xsi:type="EXPR_LEAF" xmlns:xs="` + xsdNamespace + `"><type>String</type>` +
		`<item xsi:type="xs:string">archetype_id/value</item><reference_type>attribute</reference_type></left_operand>` +
		`<right_operand xsi:type="EXPR_LEAF"><type>String</type><item xsi:type="C_STRING"><pattern>.*</pattern></item>` +
		`<reference_type>constraint</reference_type></right_operand></expression></includes>` +
		`</archetype_slot>`)
}

// TestREQ108_XMLMaxDepthBindsTheSecondDecode pins the bound on a value whose
// `xsi:type` prefix is declared on an element around the attribute, which a
// decode given no option decodes a second time: Unmarshal does so from the
// document's start, a Decoder by seeking back in a *bytes.Reader or from the
// bytes it kept. At the bound the slot decodes, its item read through the
// prefix; one below it the pattern after that item is refused.
//
// Can-fail control: bound only the first pass of a decode given no option,
// which stops at the item before the pattern, and the second pass decodes the
// pattern with no limit, so the refusal fails.
func TestREQ108_XMLMaxDepthBindsTheSecondDecode(t *testing.T) {
	in := prefixSlot()
	want := &aom14.TypedString{Value: "archetype_id/value", Type: "xs:string"}
	for mode, decode := range depthModes(t, 6) {
		var slot aom14.ArchetypeSlot
		if err := decode(in, &slot); err != nil {
			t.Errorf("%s, WithMaxDepth(6), at the bound: err = %v; want nil", mode, err)
			continue
		}
		if len(slot.Includes) != 1 {
			t.Errorf("%s: the slot holds %d includes; want 1", mode, len(slot.Includes))
			continue
		}
		op, ok := slot.Includes[0].Expression.(*aom14.ExprBinaryOperator)
		if !ok {
			t.Errorf("%s: the include's expression = %T; want *aom14.ExprBinaryOperator", mode, slot.Includes[0].Expression)
			continue
		}
		leaf, ok := op.LeftOperand.(*aom14.ExprLeaf)
		if !ok || !reflect.DeepEqual(leaf.Item, any(want)) {
			t.Errorf("%s: the left operand = %#v; want an EXPR_LEAF whose item is %#v", mode, op.LeftOperand, want)
		}
	}
	const pattern = "/archetype_slot[1]/includes[1]/expression[1]/right_operand[1]/item[1]/pattern[1]"
	for mode, decode := range depthModes(t, 5) {
		err := decode(in, new(aom14.ArchetypeSlot))
		checkDepthRefusal(t, mode+", WithMaxDepth(5)", err, pattern, 6, 5)
	}
}

// TestREQ108_XMLDecoderMaxDepthRefusalStays pins the stream Decoder after a
// refusal: its stream stands inside the refused element, so every later
// Decode returns the same refusal, even where a value within the bound would
// follow, with and without an option that tracks the decode. Before it, the
// depth counts from the root of each value: a stream of values that each
// reach the bound decodes.
//
// Can-fail control: let the token source read on after a refusal and the
// second Decode of the refused stream decodes a fragment of the refused value
// or the value after it.
func TestREQ108_XMLDecoderMaxDepthRefusalStays(t *testing.T) {
	for name, reader := range streamReaders {
		for _, extra := range []struct {
			name string
			opts []canxml.DecoderOption
		}{{name: "no other option"}, {name: "WithStrict", opts: []canxml.DecoderOption{canxml.WithStrict()}}} {
			mode := name + ", " + extra.name
			opts := append([]canxml.DecoderOption{canxml.WithMaxDepth(3)}, extra.opts...)

			fits := slices.Concat(nestedClusters(2, ""), nestedClusters(2, ""), nestedClusters(2, ""))
			dec := canxml.NewDecoder(reader(fits), opts...)
			for i := range 3 {
				if err := dec.Decode(new(rm.ItemTree)); err != nil {
					t.Errorf("%s: Decode of value %d at the bound = %v; want nil", mode, i+1, err)
				}
			}

			refused := slices.Concat(nestedClusters(5, ""), nestedClusters(1, ""))
			dec = canxml.NewDecoder(reader(refused), opts...)
			first := dec.Decode(new(rm.ItemTree))
			checkDepthRefusal(t, mode+", the deep value", first, clusterPath(4), 4, 3)
			second := dec.Decode(new(rm.ItemTree))
			checkDepthRefusal(t, mode+", the Decode after the refusal", second, clusterPath(4), 4, 3)
		}
	}
}

// xmlDepth is encoding/xml's own depth limit, 10 000 nested elements: it does
// not decode an element nested deeper, the root at depth 1.
const xmlDepth = 10_000

// cutClusterPath is the path of a depth refusal of an item of nestedClusters
// nested deeper than sixteen, cut short in the middle.
var cutClusterPath = "/item_tree[1]" + strings.Repeat("/items[1]", 7) + "/..." + strings.Repeat("/items[1]", 8)

// nestedConstraints returns an operational template whose definition holds
// levels C_SINGLE_ATTRIBUTEs, each holding a C_COMPLEX_OBJECT that holds the
// next. The template is at depth 1 and its definition at 2; the attribute of
// level k is at depth 2k+1 and its object at 2k+2. Each object writes its own
// members after the attribute it holds, so its first child is that attribute:
// a decode that reads a primitive member through encoding/xml, as one given no
// option does, would otherwise be refused at the object's first member, one
// level down, rather than at the next attribute.
func nestedConstraints(levels int) []byte {
	var b strings.Builder
	b.WriteString(`<template ` + ucNS + `><template_id><value>t</value></template_id>`)
	b.WriteString(`<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>`)
	for range levels {
		b.WriteString(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>a</rm_attribute_name>`)
		b.WriteString(`<children xsi:type="C_COMPLEX_OBJECT">`)
	}
	for range levels {
		b.WriteString(`<rm_type_name>CLUSTER</rm_type_name><node_id>at0001</node_id></children></attributes>`)
	}
	b.WriteString(`</definition></template>`)
	return []byte(b.String())
}

// TestREQ108_XMLRefusesPastEncodingXMLsDepthLimit pins the refusal of an
// element nested past encoding/xml's own depth limit, 10 000 elements, in
// every mode, with no WithMaxDepth and with one larger than that limit: a
// *canxml.DecodeError on its own that wraps canxml.ErrLimitExceeded and names
// the element's depth, the limit and its path, cut short in the middle, and
// none of its content, as a WithMaxDepth refusal does, rather than one slot
// envelope per level around an error no sentinel matches. An RM tree of
// CLUSTER items and an operational template's constraint objects nest that
// deep.
//
// Can-fail control: let a decode return encoding/xml's error as it comes, and
// the error wraps no sentinel and names every slot it passed through, 330 044
// bytes for the items and 469 950 for the template.
func TestREQ108_XMLRefusesPastEncodingXMLsDepthLimit(t *testing.T) {
	items := nestedClusters(xmlDepth+8, "")
	// The attribute of level 4999 is at depth 9 999 and its object at
	// 10 000; the attribute of level 5000 is the first element past the limit.
	template := nestedConstraints(5008)
	templateCut := "/template[1]/definition[1]" + strings.Repeat("/attributes[1]/children[1]", 3) +
		"/..." + strings.Repeat("/children[1]/attributes[1]", 4)
	for _, limit := range []struct {
		name string
		opt  canxml.DecoderOption
	}{
		{name: "no WithMaxDepth"},
		{name: "WithMaxDepth(20000)", opt: canxml.WithMaxDepth(20_000)},
	} {
		for mode, decode := range limitModes(t, limit.opt) {
			mode = mode + ", " + limit.name
			checkDepthRefusal(t, mode+", items", decode(items, new(rm.ItemTree)), cutClusterPath, xmlDepth+1, xmlDepth)
			checkDepthRefusal(t, mode+", template", decode(template, new(aom14.OperationalTemplate)), templateCut, xmlDepth+1, xmlDepth)
		}
	}
}

// TestREQ108_XMLDecoderRefusalPastEncodingXMLsLimitStays pins a stream Decoder
// after it refuses an element nested past encoding/xml's own limit: its
// stream stands inside that element, so every later Decode returns the same
// refusal, with and without an option that tracks the decode. A Decoder given
// no option that decoded an earlier value a second time, for a prefix declared
// around its `xsi:type`, refuses a later deep value the same way.
//
// Can-fail control: let a later Decode read on through the plain encoding/xml
// decoder and it returns encoding/xml's bare error; follow no element in the
// decode a Decoder given no option starts after decoding a value again, and
// the deep value's refusal names no path.
func TestREQ108_XMLDecoderRefusalPastEncodingXMLsLimitStays(t *testing.T) {
	deep := nestedClusters(xmlDepth+8, "")
	for name, reader := range streamReaders {
		for _, extra := range []struct {
			name string
			opts []canxml.DecoderOption
		}{{name: "no option"}, {name: "WithStrict", opts: []canxml.DecoderOption{canxml.WithStrict()}}} {
			mode := name + ", " + extra.name
			dec := canxml.NewDecoder(reader(slices.Concat(deep, nestedClusters(1, ""))), extra.opts...)
			checkDepthRefusal(t, mode+", the deep value", dec.Decode(new(rm.ItemTree)), cutClusterPath, xmlDepth+1, xmlDepth)
			checkDepthRefusal(t, mode+", the Decode after the refusal", dec.Decode(new(rm.ItemTree)), cutClusterPath, xmlDepth+1, xmlDepth)
		}

		dec := canxml.NewDecoder(reader(slices.Concat(prefixSlot(), deep)))
		if err := dec.Decode(new(aom14.ArchetypeSlot)); err != nil {
			t.Errorf("%s: Decode of the slot whose prefix is declared around its item = %v; want nil", name, err)
			continue
		}
		checkDepthRefusal(t, name+", the deep value after one decoded again", dec.Decode(new(rm.ItemTree)), cutClusterPath, xmlDepth+1, xmlDepth)
	}
}

// TestREQ108_XMLLimitSentinelStaysDistinct pins that canxml.ErrLimitExceeded
// is a sentinel of its own, matching none of the codec's other sentinels.
func TestREQ108_XMLLimitSentinelStaysDistinct(t *testing.T) {
	sentinels := map[string]error{
		"ErrLimitExceeded":  canxml.ErrLimitExceeded,
		"ErrInvalidShape":   canxml.ErrInvalidShape,
		"ErrUnknownElement": canxml.ErrUnknownElement,
		"ErrNamespace":      canxml.ErrNamespace,
	}
	for a, ea := range sentinels {
		for b, eb := range sentinels {
			if a != b && (a == "ErrLimitExceeded" || b == "ErrLimitExceeded") && errors.Is(ea, eb) {
				t.Errorf("errors.Is(canxml.%s, canxml.%s) = true; want the sentinels distinct", a, b)
			}
		}
	}
}
