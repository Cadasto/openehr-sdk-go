package templateprobes

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The normalised element comparison of PROBE-111's round-trip leg
// (REQ-176 § Round trip, the maintainer's decision D4, and the rewrites of
// REQ-176 § Shapes the pinned BMM lacks).

const (
	probe111OpenEHRNamespace = "http://schemas.openehr.org/v1"
	probe111XSINamespace     = "http://www.w3.org/2001/XMLSchema-instance"
	probe111XMLWhiteSpace    = " \t\r\n"
)

var probe111XSIType = xml.Name{Space: probe111XSINamespace, Local: "type"}

// probe111Element is one element of a document, read for the comparison:
// its name, its attributes other than namespace declarations (an `xsi:type`
// held as the qualified name it denotes, `{namespace}local`), its text and
// its child elements.
type probe111Element struct {
	name     xml.Name
	attrs    map[xml.Name]string
	text     string
	children []*probe111Element
}

// probe111CompareDocuments returns the first difference between the input
// document and the re-encoded one, after the input's rewrites and the
// normalisation of both, as a path and a sentence; "" when they agree.
func probe111CompareDocuments(input, reEncoded []byte) (string, error) {
	in, err := probe111Parse(input)
	if err != nil {
		return "", fmt.Errorf("read the input: %w", err)
	}
	out, err := probe111Parse(reEncoded)
	if err != nil {
		return "", fmt.Errorf("read the re-encoded document: %w", err)
	}
	probe111Rewrite(in)
	probe111Normalise(in, true)
	probe111Normalise(out, true)
	return probe111Diff(in, out, "/"+in.name.Local+"[1]"), nil
}

// probe111Parse reads doc into elements, keeping the namespace declarations
// in scope so an `xsi:type` value resolves to the name it denotes.
func probe111Parse(doc []byte) (*probe111Element, error) {
	d := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(doc, []byte("\xef\xbb\xbf"))))
	type frame struct {
		el    *probe111Element
		scope map[string]string
	}
	var stack []frame
	var root *probe111Element
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			scope := map[string]string{}
			if len(stack) > 0 {
				scope = stack[len(stack)-1].scope
			}
			scope = probe111DeclaredScope(scope, tk.Attr)
			el := &probe111Element{name: tk.Name, attrs: map[xml.Name]string{}}
			for _, a := range tk.Attr {
				if a.Name.Space == "xmlns" || a.Name.Space == "" && a.Name.Local == "xmlns" {
					continue
				}
				v := a.Value
				if a.Name == probe111XSIType {
					v = probe111QualifiedName(v, scope)
				}
				el.attrs[a.Name] = v
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1].el
				parent.children = append(parent.children, el)
			} else if root == nil {
				root = el
			}
			stack = append(stack, frame{el: el, scope: scope})
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].el.text += string(tk)
			}
		}
	}
	if root == nil {
		return nil, errors.New("no root element")
	}
	return root, nil
}

// probe111DeclaredScope returns the namespace bindings in scope on an
// element: the parent's, with the element's own declarations over them.
func probe111DeclaredScope(parent map[string]string, attrs []xml.Attr) map[string]string {
	var scope map[string]string
	for _, a := range attrs {
		prefix, ok := "", false
		switch {
		case a.Name.Space == "xmlns":
			prefix, ok = a.Name.Local, true
		case a.Name.Space == "" && a.Name.Local == "xmlns":
			ok = true
		}
		if !ok {
			continue
		}
		if scope == nil {
			scope = make(map[string]string, len(parent)+1)
			maps.Copy(scope, parent)
		}
		scope[prefix] = a.Value
	}
	if scope == nil {
		return parent
	}
	return scope
}

// probe111QualifiedName gives the `{namespace}local` form of a QName value:
// an unprefixed name takes the default namespace in scope, a prefixed one
// the namespace its prefix is bound to. A prefix bound to nothing keeps the
// value as written.
func probe111QualifiedName(value string, scope map[string]string) string {
	value = strings.Trim(value, probe111XMLWhiteSpace)
	prefix, local, prefixed := strings.Cut(value, ":")
	if !prefixed {
		return "{" + scope[""] + "}" + value
	}
	ns, ok := scope[prefix]
	if !ok {
		return value
	}
	return "{" + ns + "}" + local
}

func probe111OpenEHR(local string) xml.Name {
	return xml.Name{Space: probe111OpenEHRNamespace, Local: local}
}

// probe111TimezoneValidityCodes are the integer codes the encoder writes
// for the words a `timezone_validity` may carry (REQ-176 § Shapes; the
// maintainer's decision D5).
var probe111TimezoneValidityCodes = map[string]string{
	"mandatory":  "1001",
	"optional":   "1002",
	"prohibited": "1003",
	"disallowed": "1003",
}

// probe111OperatorKindCodes are the AOM 1.4 OPERATOR_KIND codes the encoder
// writes for the item names an `operator` may carry (REQ-176 § Shapes).
var probe111OperatorKindCodes = map[string]string{
	"equal":                 "2001",
	"not_equal":             "2002",
	"less_than_or_equal":    "2003",
	"less_than":             "2004",
	"greater_than_or_equal": "2005",
	"greater_than":          "2006",
	"matches":               "2007",
	"not":                   "2010",
	"and":                   "2011",
	"or":                    "2012",
	"xor":                   "2013",
	"implies":               "2014",
	"for_all":               "2015",
	"exists":                "2016",
	"plus":                  "2020",
	"minus":                 "2021",
	"multiply":              "2022",
	"divide":                "2023",
	"exponent":              "2024",
}

// probe111Rewrite applies to the input the rewrites the encoder makes
// (REQ-176 § Shapes the pinned BMM lacks): `match_negated` is read and
// dropped, never written; a `timezone_validity` or `operator` word is written
// as its integer code; `T_ARCHETYPE_ROOT` is written as `C_ARCHETYPE_ROOT`;
// and the DV_CODED_TEXT shape of an ORDINAL or SCALE `symbol` is written with
// no `value` where its `value` is empty.
func probe111Rewrite(el *probe111Element) {
	kept := el.children[:0]
	for _, c := range el.children {
		if c.name == probe111OpenEHR("match_negated") {
			continue
		}
		if el.name == probe111OpenEHR("symbol") && c.name == probe111OpenEHR("value") &&
			len(c.children) == 0 && len(c.attrs) == 0 && c.text == "" {
			continue
		}
		kept = append(kept, c)
	}
	el.children = kept
	switch el.name {
	case probe111OpenEHR("timezone_validity"):
		if code, ok := probe111TimezoneValidityCodes[strings.Trim(el.text, probe111XMLWhiteSpace)]; ok {
			el.text = code
		}
	case probe111OpenEHR("operator"):
		if code, ok := probe111OperatorKindCodes[strings.Trim(el.text, probe111XMLWhiteSpace)]; ok {
			el.text = code
		}
	}
	if el.attrs[probe111XSIType] == "{"+probe111OpenEHRNamespace+"}T_ARCHETYPE_ROOT" {
		el.attrs[probe111XSIType] = "{" + probe111OpenEHRNamespace + "}C_ARCHETYPE_ROOT"
	}
	for _, c := range el.children {
		probe111Rewrite(c)
	}
}

// probe111Normalise applies the comparison's normalisation to both sides
// (REQ-176 § Round trip): the root's `xsi:type` set aside, white space
// between elements set aside, absent interval flags written with their
// defaults, each Hash member's entries in the order of their keys, and a
// SCALE's value before its symbol.
func probe111Normalise(el *probe111Element, root bool) {
	if root {
		delete(el.attrs, probe111XSIType)
	}
	if len(el.children) > 0 && strings.Trim(el.text, probe111XMLWhiteSpace) == "" {
		el.text = ""
	}
	probe111DefaultIntervalFlags(el)
	probe111SortHashEntries(el)
	probe111ScaleValueFirst(el)
	for _, c := range el.children {
		probe111Normalise(c, false)
	}
}

// probe111ScaleValueFirst puts the `value` of each SCALE that el, a
// C_DV_SCALE, holds in `assumed_value` or `list` before its `symbol`.
// OpenehrProfile.xsd admits the two in either order, Archetype Designer
// writing the symbol first, and the encoder writes the value first, the
// order of an ORDINAL and of the RM's DV_SCALE.
func probe111ScaleValueFirst(el *probe111Element) {
	if el.attrs[probe111XSIType] != "{"+probe111OpenEHRNamespace+"}C_DV_SCALE" {
		return
	}
	for _, c := range el.children {
		if c.name != probe111OpenEHR("assumed_value") && c.name != probe111OpenEHR("list") {
			continue
		}
		n := len(c.children)
		if n >= 2 && c.children[n-2].name == probe111OpenEHR("symbol") && c.children[n-1].name == probe111OpenEHR("value") {
			c.children[n-2], c.children[n-1] = c.children[n-1], c.children[n-2]
		}
	}
}

// probe111IntervalOrder is the order of an interval's children in the
// AM Release-1.4 schemas: the four flags of BASE Interval, then the bounds
// its extensions add.
var probe111IntervalOrder = []string{"lower_included", "upper_included", "lower_unbounded", "upper_unbounded", "lower", "upper"}

// probe111DefaultIntervalFlags gives an interval each boundary flag it leaves
// out, with the default the SDK reads for it on decode: an absent
// `*_unbounded` is true where that side carries no bound and false where it
// does, and an absent `*_included` is true on a side not unbounded and false
// on one unbounded. An empty or white-space-only flag reads as absent. An
// interval is an element with a boundary flag among its children.
func probe111DefaultIntervalFlags(el *probe111Element) {
	isFlag := func(c *probe111Element) bool {
		return c.name.Space == probe111OpenEHRNamespace && slices.Contains(probe111IntervalOrder[:4], c.name.Local)
	}
	if !slices.ContainsFunc(el.children, isFlag) {
		return
	}
	el.children = slices.DeleteFunc(el.children, func(c *probe111Element) bool {
		return isFlag(c) && len(c.children) == 0 && strings.Trim(c.text, probe111XMLWhiteSpace) == ""
	})
	child := func(local string) *probe111Element {
		for _, c := range el.children {
			if c.name == probe111OpenEHR(local) {
				return c
			}
		}
		return nil
	}
	for _, side := range []string{"lower", "upper"} {
		var unbounded bool
		if f := child(side + "_unbounded"); f != nil {
			t := strings.Trim(f.text, probe111XMLWhiteSpace)
			unbounded = t == "true" || t == "1"
		} else {
			b := child(side)
			unbounded = b == nil || len(b.children) == 0 && strings.Trim(b.text, probe111XMLWhiteSpace) == ""
			probe111InsertIntervalFlag(el, side+"_unbounded", unbounded)
		}
		if child(side+"_included") == nil {
			probe111InsertIntervalFlag(el, side+"_included", !unbounded)
		}
	}
}

// probe111InsertIntervalFlag inserts the flag named local, holding value,
// where the schema's order puts it among the interval's children.
func probe111InsertIntervalFlag(el *probe111Element, local string, value bool) {
	rank := func(name string) int {
		if i := slices.Index(probe111IntervalOrder, name); i >= 0 {
			return i
		}
		return len(probe111IntervalOrder)
	}
	at := slices.IndexFunc(el.children, func(c *probe111Element) bool { return rank(c.name.Local) > rank(local) })
	if at < 0 {
		at = len(el.children)
	}
	flag := &probe111Element{name: probe111OpenEHR(local), attrs: map[xml.Name]string{}, text: strconv.FormatBool(value)}
	el.children = slices.Insert(el.children, at, flag)
}

// probe111HashKey reports whether child is an entry of a Hash member of
// parent, and its key. The Hash members of the model (REQ-176 § Round
// trip): ARCHETYPE_TERM.items and ANNOTATION.items, the String maps of
// RESOURCE_DESCRIPTION, RESOURCE_DESCRIPTION_ITEM and TRANSLATION_DETAILS,
// each a StringDictionaryItem keyed by its `id` attribute; and
// RESOURCE_DESCRIPTION.details and `translations`, keyed by their language
// code. T_VIEW's constraint `items` carry an `id` too, but they are a List,
// so their order counts.
func probe111HashKey(parent, child *probe111Element) (string, bool) {
	if child.name.Space != probe111OpenEHRNamespace {
		return "", false
	}
	byID := false
	switch child.name.Local {
	case "items":
		byID = slices.Contains([]string{"annotations", "term_definitions", "items"}, parent.name.Local)
	case "original_author":
		byID = parent.name.Local == "description"
	case "other_details":
		byID = slices.Contains([]string{"description", "details", "translations"}, parent.name.Local)
	case "original_resource_uri":
		byID = parent.name.Local == "details"
	case "author":
		byID = parent.name.Local == "translations"
	case "details":
		if parent.name.Local == "description" {
			return probe111LanguageKey(child)
		}
	case "translations":
		return probe111LanguageKey(child)
	}
	if !byID {
		return "", false
	}
	id, ok := child.attrs[xml.Name{Local: "id"}]
	return id, ok
}

// probe111LanguageKey is the key of a RESOURCE_DESCRIPTION_ITEM or a
// TRANSLATION_DETAILS: its language's code string.
func probe111LanguageKey(el *probe111Element) (string, bool) {
	for _, c := range el.children {
		if c.name != probe111OpenEHR("language") {
			continue
		}
		for _, cc := range c.children {
			if cc.name == probe111OpenEHR("code_string") {
				return strings.Trim(cc.text, probe111XMLWhiteSpace), true
			}
		}
	}
	return "", false
}

// probe111SortHashEntries orders each run of a Hash member's entries among
// el's children by key, so the entries compare as a set. An entry keeps its
// place among entries of the same key.
func probe111SortHashEntries(el *probe111Element) {
	for i := 0; i < len(el.children); {
		j := i + 1
		if _, ok := probe111HashKey(el, el.children[i]); ok {
			for j < len(el.children) && el.children[j].name == el.children[i].name {
				if _, ok := probe111HashKey(el, el.children[j]); !ok {
					break
				}
				j++
			}
			slices.SortStableFunc(el.children[i:j], func(a, b *probe111Element) int {
				ka, _ := probe111HashKey(el, a)
				kb, _ := probe111HashKey(el, b)
				return strings.Compare(ka, kb)
			})
		}
		i = j
	}
}

// probe111Decimal is the lexical form of an XML Schema decimal or double
// with no special value: the text the comparison reads as a number.
var probe111Decimal = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// probe111Booleans maps each literal of an XML Schema boolean to its value.
var probe111Booleans = map[string]bool{"true": true, "1": true, "false": false, "0": false}

// probe111SameText reports whether in, the input's text, and out, the
// re-encoded text, are equal, or both numbers of the same value, or both
// booleans of the same value. A number or a boolean is read as XML Schema
// reads it, its white space collapsed, so `true` followed by a line break and
// indentation is `true`. out is a boolean when it is `true` or `false`, the
// only form the encoder writes a boolean in and never the form of a number;
// in is then read as a boolean in any of its literals, so `1` is `true` and
// `0` is `false`.
func probe111SameText(in, out string) bool {
	if in == out {
		return true
	}
	ta, tb := strings.Trim(in, probe111XMLWhiteSpace), strings.Trim(out, probe111XMLWhiteSpace)
	if tb == "true" || tb == "false" {
		if v, ok := probe111Booleans[ta]; ok {
			return v == (tb == "true")
		}
	}
	if !probe111Decimal.MatchString(ta) || !probe111Decimal.MatchString(tb) {
		return false
	}
	ra, okA := new(big.Rat).SetString(ta)
	rb, okB := new(big.Rat).SetString(tb)
	return okA && okB && ra.Cmp(rb) == 0
}

// probe111Diff returns the first difference between in and out, the
// element at path here; "" when they agree. An `xsi:type` on in that out
// carries none is set aside: the encoder writes no `xsi:type` where the
// element's slot holds a single class, so a redundant one the input wrote is
// no lost content (REQ-176 § Round trip). An `xsi:type` out carries with
// another value, or one out adds, still differs.
func probe111Diff(in, out *probe111Element, here string) string {
	if in.name != out.name {
		return fmt.Sprintf("%s: the re-encoded document has %s there", here, out.name.Local)
	}
	names := make([]xml.Name, 0, len(in.attrs)+len(out.attrs))
	for n := range in.attrs {
		names = append(names, n)
	}
	for n := range out.attrs {
		if _, ok := in.attrs[n]; !ok {
			names = append(names, n)
		}
	}
	slices.SortFunc(names, func(a, b xml.Name) int {
		if c := strings.Compare(a.Space, b.Space); c != 0 {
			return c
		}
		return strings.Compare(a.Local, b.Local)
	})
	for _, n := range names {
		iv, inHas := in.attrs[n]
		ov, outHas := out.attrs[n]
		switch {
		case !outHas && n == probe111XSIType:
			// Set aside: the element's slot holds a single class.
		case !outHas:
			return fmt.Sprintf("%s/@%s: absent from the re-encoded document", here, n.Local)
		case !inHas:
			return fmt.Sprintf("%s/@%s: the re-encoded document adds %s", here, n.Local, probe111Quote(ov))
		case iv != ov:
			return fmt.Sprintf("%s/@%s: %s, re-encoded as %s", here, n.Local, probe111Quote(iv), probe111Quote(ov))
		}
	}
	if !probe111SameText(in.text, out.text) {
		return fmt.Sprintf("%s: text %s, re-encoded as %s", here, probe111Quote(in.text), probe111Quote(out.text))
	}
	seen := map[string]int{}
	for i := range max(len(in.children), len(out.children)) {
		if i >= len(out.children) {
			c := in.children[i]
			return fmt.Sprintf("%s/%s[%d]: absent from the re-encoded document", here, c.name.Local, seen[c.name.Local]+1)
		}
		if i >= len(in.children) {
			return fmt.Sprintf("%s: the re-encoded document adds %s after the input's last child", here, out.children[i].name.Local)
		}
		c := in.children[i]
		seen[c.name.Local]++
		if d := probe111Diff(c, out.children[i], fmt.Sprintf("%s/%s[%d]", here, c.name.Local, seen[c.name.Local])); d != "" {
			return d
		}
	}
	return ""
}

// probe111Quote quotes a text for a difference, cut to a readable length.
func probe111Quote(s string) string {
	const limit = 80
	if r := []rune(s); len(r) > limit {
		return fmt.Sprintf("%q…", string(r[:limit]))
	}
	return fmt.Sprintf("%q", s)
}
