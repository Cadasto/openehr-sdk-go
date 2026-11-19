package templatecompile

// model.go: what the walk reads of the operational template model, in the
// respects the corpus needs: each constraint object's kind and identifiers,
// each attribute's name, kind and intervals, a node's template-level name,
// and an archetype root's terms and bindings, each trimmed, with blank
// entries left out.

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// object is what the walk reads of one constraint object of the model
// before it compiles it: the fields its AQL path segment and its node need,
// trimmed, and the parts that only some kinds have.
type object struct {
	o           aom14.CObject
	rmTypeName  string
	nodeID      string
	archetypeID string // an archetype root's only
	nodeName    string // a complex object's only ([nodeNameFromModel])

	// terms is an archetype root's term definitions ([termsFromModel]);
	// nil for every other kind.
	terms map[string]template.ArchetypeTerm

	// occurrences is the object's occurrences interval; nil for a slot,
	// whose occurrences the compiled node does not carry.
	occurrences *rm.Interval[aom14.Integer]

	// complex holds the attributes of a complex object, an archetype root
	// among them; nil for every other kind.
	complex *aom14.CComplexObject
	root    *aom14.CArchetypeRoot // nil unless the object is an archetype root
	slot    *aom14.ArchetypeSlot  // nil unless the object is a slot
}

// objectOf returns what the walk reads of the constraint object o, held by
// an attribute of parent, nil for the definition. A code that names o reads
// its term text from the terms of o's own archetype: o's own where o is an
// archetype root with an archetype id, else those of parent's nearest
// enclosing archetype root ([nodeNameFromModel]). An error wraps
// ErrInvalidInput when o is nil or a nil pointer, and reports a kind the
// compile does not handle, which the model's decode does not produce,
// without wrapping it.
func objectOf(o aom14.CObject, parent *CompiledNode) (object, error) {
	switch v := o.(type) {
	case *aom14.CArchetypeRoot:
		if v != nil {
			archetypeID := trimmed(v.ArchetypeID.Value)
			terms := termsFromModel(v.TermDefinitions)
			termText := rootTermText(parent)
			if archetypeID != "" {
				termText = func(key string) string { return terms[key].Items["text"] }
			}
			ob := complexOf(o, &v.CComplexObject, termText)
			ob.root = v
			ob.archetypeID = archetypeID
			ob.terms = terms
			return ob, nil
		}
	case *aom14.CComplexObject:
		if v != nil {
			return complexOf(o, v, rootTermText(parent)), nil
		}
	case *aom14.TComplexObject:
		if v != nil {
			return complexOf(o, &v.CComplexObject, rootTermText(parent)), nil
		}
	case *aom14.ArchetypeSlot:
		if v != nil {
			return object{o: o, rmTypeName: trimmed(v.RMTypeName), nodeID: trimmed(v.NodeID), slot: v}, nil
		}
	case *aom14.CPrimitiveObject:
		if v != nil {
			return leafOf(o, v.RMTypeName, v.NodeID, &v.Occurrences), nil
		}
	case *aom14.CCodePhrase:
		if v != nil {
			return leafOf(o, v.RMTypeName, v.NodeID, &v.Occurrences), nil
		}
	case *aom14.CCodeReference:
		if v != nil {
			return leafOf(o, v.RMTypeName, v.NodeID, &v.Occurrences), nil
		}
	case *aom14.CDVQuantity:
		if v != nil {
			return leafOf(o, v.RMTypeName, v.NodeID, &v.Occurrences), nil
		}
	case *aom14.CDVOrdinal:
		if v != nil {
			return leafOf(o, v.RMTypeName, v.NodeID, &v.Occurrences), nil
		}
	case *aom14.CDVScale:
		if v != nil {
			return leafOf(o, v.RMTypeName, v.NodeID, &v.Occurrences), nil
		}
	case *aom14.CDVState:
		if v != nil {
			return leafOf(o, v.RMTypeName, v.NodeID, &v.Occurrences), nil
		}
	case *aom14.ConstraintRef:
		if v != nil {
			return leafOf(o, v.RMTypeName, v.NodeID, &v.Occurrences), nil
		}
	case *aom14.ArchetypeInternalRef:
		if v != nil {
			return leafOf(o, v.RMTypeName, v.NodeID, &v.Occurrences), nil
		}
	case nil:
	default:
		return object{}, fmt.Errorf("templatecompile: constraint object %T is of a kind Compile does not handle", o)
	}
	return object{}, fmt.Errorf("%w: constraint object missing (%T)", ErrInvalidInput, o)
}

// complexOf returns what the walk reads of the complex object c, which o
// holds: an archetype root's, a template's own complex object's, or c
// itself. termText reads the terms of o's own archetype
// ([nodeNameFromModel]).
func complexOf(o aom14.CObject, c *aom14.CComplexObject, termText func(key string) string) object {
	return object{
		o:           o,
		rmTypeName:  trimmed(c.RMTypeName),
		nodeID:      trimmed(c.NodeID),
		nodeName:    nodeNameFromModel(c.Attributes, termText),
		occurrences: &c.Occurrences,
		complex:     c,
	}
}

// leafOf returns what the walk reads of the constraint object o, a kind
// with no attributes, from its RM type name, node id and occurrences.
func leafOf(o aom14.CObject, rmTypeName, nodeID string, occurrences *rm.Interval[aom14.Integer]) object {
	return object{
		o:           o,
		rmTypeName:  trimmed(rmTypeName),
		nodeID:      trimmed(nodeID),
		occurrences: occurrences,
	}
}

// modelAttribute is what the walk reads of one attribute of the model.
type modelAttribute struct {
	key       aom14.CAttribute // the model's attribute, which identifies it in the walk
	name      string           // trimmed; the caller gives it its compiled name ([attributeName])
	card      template.Cardinality
	existence *rm.Interval[aom14.Integer]
	// cardinality is a multiple attribute's cardinality interval; nil for
	// a single attribute.
	cardinality *rm.Interval[aom14.Integer]
	children    []aom14.CObject
}

// attributeOf returns what the walk reads of the attribute a. An error
// wraps ErrInvalidInput when a is nil or a nil pointer, and reports a kind
// the compile does not handle without wrapping it.
func attributeOf(a aom14.CAttribute) (modelAttribute, error) {
	switch v := a.(type) {
	case *aom14.CSingleAttribute:
		if v != nil {
			return modelAttribute{
				key:       a,
				name:      trimmed(v.RMAttributeName),
				card:      template.Single,
				existence: &v.Existence,
				children:  v.Children,
			}, nil
		}
	case *aom14.CMultipleAttribute:
		if v != nil {
			return modelAttribute{
				key:         a,
				name:        trimmed(v.RMAttributeName),
				card:        template.Multiple,
				existence:   &v.Existence,
				cardinality: &v.Cardinality.Interval,
				children:    v.Children,
			}, nil
		}
	case nil:
	default:
		return modelAttribute{}, fmt.Errorf("templatecompile: attribute %T is of a kind Compile does not handle", a)
	}
	return modelAttribute{}, fmt.Errorf("%w: attribute missing (%T)", ErrInvalidInput, a)
}

// rawAttributeName returns the name the model gives the attribute a,
// trimmed, or "" for a nil attribute or one of another kind.
func rawAttributeName(a aom14.CAttribute) string {
	switch v := a.(type) {
	case *aom14.CSingleAttribute:
		if v != nil {
			return strings.TrimSpace(v.RMAttributeName)
		}
	case *aom14.CMultipleAttribute:
		if v != nil {
			return strings.TrimSpace(v.RMAttributeName)
		}
	}
	return ""
}

// attributeChildren returns the children of the attribute a, or nil for a
// nil attribute or one of another kind.
func attributeChildren(a aom14.CAttribute) []aom14.CObject {
	switch v := a.(type) {
	case *aom14.CSingleAttribute:
		if v != nil {
			return v.Children
		}
	case *aom14.CMultipleAttribute:
		if v != nil {
			return v.Children
		}
	}
	return nil
}

// attributeName returns the name the compile gives an attribute the model
// names name, trimmed, on a node of RM type rmType whose attributes are
// attrs. An ELEMENT's null_flavor, the US spelling some templates write, is
// read as the RM attribute null_flavour, so its constraint applies; that is
// the only name read otherwise than as written. Where the ELEMENT also
// declares null_flavour, null_flavor is kept as written, so the node does
// not declare null_flavour twice and each attribute keeps its own
// constraint.
func attributeName(rmType string, attrs []aom14.CAttribute, name string) string {
	if rmType != "ELEMENT" || name != "null_flavor" {
		return name
	}
	if slices.ContainsFunc(attrs, func(a aom14.CAttribute) bool { return rawAttributeName(a) == "null_flavour" }) {
		return name
	}
	return "null_flavour"
}

// nodeNameFromModel returns the template-level node name of a complex
// object whose attributes are attrs, or "" where it pins none: the archetype
// concept term is never substituted. Its name attribute pins a name in one
// of two ways:
//
//   - a DV_TEXT or DV_CODED_TEXT child whose value is a C_PRIMITIVE_OBJECT
//     whose C_STRING lists exactly one entry, not blank once trimmed, and is
//     not open, pins that entry, trimmed ([fixedValueString]). Anything
//     looser (a pattern, a list of several entries, an open list, a lone
//     blank entry) pins none. Where the name attribute has alternatives, the
//     first that pins a name gives it;
//   - failing that, a sole DV_CODED_TEXT child whose code is fixed pins the
//     term text of that code, which termText reads from the terms of the
//     object's own archetype ([codedNameText]).
func nodeNameFromModel(attrs []aom14.CAttribute, termText func(key string) string) string {
	for _, a := range attrs {
		if rawAttributeName(a) != "name" {
			continue
		}
		children := attributeChildren(a)
		for _, child := range children {
			if name := fixedValueString(child); name != "" {
				return name
			}
		}
		if len(children) == 1 {
			if name := codedNameText(children[0], termText); name != "" {
				return name
			}
		}
	}
	return ""
}

// fixedValueString returns the one string the name constraint o pins on its
// value attribute, trimmed, or "" when it pins none ([nodeNameFromModel]).
func fixedValueString(o aom14.CObject) string {
	for _, a := range complexAttributes(o) {
		if rawAttributeName(a) != "value" {
			continue
		}
		for _, child := range attributeChildren(a) {
			p, ok := child.(*aom14.CPrimitiveObject)
			if !ok || p == nil {
				continue
			}
			s, ok := p.Item.(*aom14.CString)
			if !ok || s == nil || len(s.List) != 1 || (s.ListOpen != nil && *s.ListOpen) {
				continue
			}
			if name := trimmed(s.List[0]); name != "" {
				return name
			}
		}
	}
	return ""
}

// codedNameText returns the term text of the one code the name constraint o
// fixes, trimmed, or "" where it fixes none ([nodeNameFromModel]). o fixes a
// code where its defining_code holds one child, a C_CODE_PHRASE or a
// C_CODE_REFERENCE read as the code phrase it carries, that names a
// terminology and lists exactly one code once blank entries are left out.
// The text is the code's term text ([codeTermText]), read through termText,
// and o fixes none where the code has no term text, or where o's value holds
// one C_STRING that rejects the text.
func codedNameText(o aom14.CObject, termText func(key string) string) string {
	attrs := complexAttributes(o)
	i := slices.IndexFunc(attrs, func(a aom14.CAttribute) bool { return rawAttributeName(a) == "defining_code" })
	if i < 0 {
		return ""
	}
	children := attributeChildren(attrs[i])
	if len(children) != 1 {
		return ""
	}
	var phrase constraints.CodePhrase
	switch v := children[0].(type) {
	case *aom14.CCodePhrase:
		if v != nil {
			phrase = codePhraseFromModel(v)
		}
	case *aom14.CCodeReference:
		if v != nil {
			phrase = codePhraseFromModel(&v.CCodePhrase)
		}
	}
	if phrase.Terminology == "" || len(phrase.CodeList) != 1 {
		return ""
	}
	text, ok := codeTermText(termText, phrase.Terminology, phrase.CodeList[0])
	if text = trimmed(text); !ok || text == "" {
		return ""
	}
	if value, ok := valueString(attrs); ok && len(value.Validate(text)) > 0 {
		return ""
	}
	return text
}

// valueString returns the C_STRING constraint of the value attribute among
// attrs, the attributes of a name constraint, where that attribute holds one
// child and the child is a C_PRIMITIVE_OBJECT whose item is a C_STRING; ok is
// false otherwise.
func valueString(attrs []aom14.CAttribute) (constraints.CString, bool) {
	i := slices.IndexFunc(attrs, func(a aom14.CAttribute) bool { return rawAttributeName(a) == "value" })
	if i < 0 {
		return constraints.CString{}, false
	}
	children := attributeChildren(attrs[i])
	if len(children) != 1 {
		return constraints.CString{}, false
	}
	p, ok := children[0].(*aom14.CPrimitiveObject)
	if !ok || p == nil {
		return constraints.CString{}, false
	}
	if item, ok := p.Item.(*aom14.CString); !ok || item == nil {
		return constraints.CString{}, false
	}
	c, err := primitiveItemFromModel(p.Item, false)
	if err != nil {
		return constraints.CString{}, false
	}
	s, ok := c.(constraints.CString)
	return s, ok
}

// complexAttributes returns the attributes of o where it is a complex
// object, an archetype root or a template's own complex object, and nil
// otherwise.
func complexAttributes(o aom14.CObject) []aom14.CAttribute {
	switch v := o.(type) {
	case *aom14.CComplexObject:
		if v != nil {
			return v.Attributes
		}
	case *aom14.CArchetypeRoot:
		if v != nil {
			return v.Attributes
		}
	case *aom14.TComplexObject:
		if v != nil {
			return v.Attributes
		}
	}
	return nil
}

// multiplicityOf returns the interval iv closed into a Multiplicity, as
// [template.NewMultiplicity] closes it, or nil when iv is nil or the zero
// interval. It returns an error for an excluded end on a bounded side whose
// closed end the model's 32-bit Integer cannot hold, on every platform,
// where NewMultiplicity refuses only a closed end Go's int cannot hold.
//
// The model holds each interval by value, so an interval the template leaves
// out reads as the zero interval, as an absent definition reads as the zero
// archetype root. The zero interval is read as absent. The decode gives each
// flag a written interval leaves out its default, a bounded side's included
// flag true, so a written interval reads as the zero one only where it writes
// both bounds 0 and all four flags false, an empty interval that closing
// refuses and the model cannot tell from an absent one.
func multiplicityOf(iv *rm.Interval[aom14.Integer]) (*template.Multiplicity, error) {
	if iv == nil || *iv == (rm.Interval[aom14.Integer]{}) {
		return nil, nil
	}
	// The closed end of ">2147483647" or "<-2147483648" is outside the
	// Integer range, so the result would depend on the width of int: kept
	// on a 64-bit build, refused on a 32-bit one. Refuse it on both.
	if !iv.LowerUnbounded && !iv.LowerIncluded && iv.Lower == math.MaxInt32 {
		return nil, fmt.Errorf("closed lower bound overflows the 32-bit Integer: excluded lower=%d", iv.Lower)
	}
	if !iv.UpperUnbounded && !iv.UpperIncluded && iv.Upper == math.MinInt32 {
		return nil, fmt.Errorf("closed upper bound overflows the 32-bit Integer: excluded upper=%d", iv.Upper)
	}
	return template.NewMultiplicity(template.IntervalBounds{
		Lower:          int(iv.Lower),
		Upper:          int(iv.Upper),
		LowerIncluded:  iv.LowerIncluded,
		UpperIncluded:  iv.UpperIncluded,
		LowerUnbounded: iv.LowerUnbounded,
		UpperUnbounded: iv.UpperUnbounded,
	})
}

// termsFromModel returns an archetype root's term definitions keyed by
// at-code: each code and each item key and text trimmed, an item with a
// blank key left out, and a term with a blank code or no item left out. A
// later term for a code replaces an earlier one. It returns nil when no term
// is left.
func termsFromModel(defs []aom14.ArchetypeTerm) map[string]template.ArchetypeTerm {
	var out map[string]template.ArchetypeTerm
	for _, d := range defs {
		code := trimmed(d.Code)
		if code == "" {
			continue
		}
		items := termItems(d.Items)
		if len(items) == 0 {
			continue
		}
		if out == nil {
			out = make(map[string]template.ArchetypeTerm, len(defs))
		}
		out[code] = template.ArchetypeTerm{Code: code, Items: items}
	}
	return out
}

// termItems returns the items of a term with each key and text trimmed and
// an item with a blank key left out. A key the model writes with white space
// around it gives way to the same key written without, and of two such
// spellings of one key the one that sorts first is kept, so the result does
// not depend on the order of the model's map.
func termItems(raw map[string]string) map[string]string {
	items := make(map[string]string, len(raw))
	padded := false
	for k, v := range raw {
		if id := strings.TrimSpace(k); id != k {
			padded = true
			continue
		}
		if k != "" {
			items[k] = trimmed(v)
		}
	}
	if !padded {
		return items
	}
	for _, k := range slices.Sorted(maps.Keys(raw)) {
		id := strings.TrimSpace(k)
		if id == k || id == "" {
			continue
		}
		if _, kept := items[id]; !kept {
			// Trimming k dropped a byte, so the key kept is a copy ([trimmed]).
			items[strings.Clone(id)] = trimmed(raw[k])
		}
	}
	return items
}

// trimmed returns s without its leading and trailing white space, as
// [strings.TrimSpace] does, in a string of its own where that drops a byte.
// The compiled template keeps the texts it reads trimmed, and a slice of s
// would keep all of s alive, padding included, once the model is dropped.
func trimmed(s string) string {
	t := strings.TrimSpace(s)
	if len(t) < len(s) {
		return strings.Clone(t)
	}
	return t
}

// appendTermBindings appends to out an archetype root's term bindings, one
// record per bound code: the terminology, the code and the target trimmed,
// and an item with a blank code left out.
func appendTermBindings(out []template.TermBinding, sets []aom14.TermBindingSet) []template.TermBinding {
	for _, set := range sets {
		terminology := trimmed(set.Terminology)
		for _, item := range set.Items {
			code := trimmed(item.Code)
			if code == "" {
				continue
			}
			out = append(out, template.TermBinding{
				Terminology: terminology,
				NodeOrPath:  code,
				Target: template.CodedTermRef{
					TerminologyID: trimmed(item.Value.TerminologyID.Value),
					CodeString:    trimmed(item.Value.CodeString),
				},
			})
		}
	}
	return out
}
