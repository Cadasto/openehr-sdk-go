package aom14

import (
	"encoding/xml"
	"errors"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/decodeopts"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// read_tokens.go: when [ReadOperationalTemplate] reads a document a second
// time, through [tolerate]. The first decode passes each token it reads to a
// [tokenFacts], once, as encoding/xml reads it: element names with their
// namespaces resolved, attribute values with their references resolved, and
// every attribute, the namespace declarations included. The facts cost no
// reading of their own, and decide, once that decode has read the document,
// whether a second reading may read it otherwise. Each reading tolerate makes
// applies to a document only where the first decode failed in a way this
// file names, or where the facts show what the reading changes, so the result
// never depends on whether the second reading ran.

// retryable reports whether err, the failure of the first decode, is one a
// reading of [tolerate] can lift: the text of a value, a type a slot cannot
// dispatch, which only the default mode reads but inside a C_DV_QUANTITY,
// whose assumed value every mode reads as absent where the model cannot hold
// it, or an element in the namespace the AM Release-1.4 schemas declare,
// which a strict decode refuses. A type a slot cannot dispatch covers a
// <children> of a primitive or an attribute class and an <item> of a class
// the default mode unwraps (isUnwrapClass); the text of a value covers a real
// number canonical XML does not read and an integer bound written as a
// decimal.
func retryable(err error, strict bool) bool {
	switch {
	case xmlunmatched.IsPrimitiveFailure(err), errors.Is(err, canxml.ErrNamespace), failsInsideQuantity(err):
		return true
	case strict:
		return false
	}
	return isTypeFailure(err)
}

// isTypeFailure reports whether err is the failure of a type a slot cannot
// dispatch.
func isTypeFailure(err error) bool {
	return errors.Is(err, typereg.ErrMissingType) || errors.Is(err, typereg.ErrUnknownType) || errors.Is(err, typereg.ErrTypeMismatch)
}

// failsInsideQuantity reports whether err is the failure of a type a slot
// inside a C_DV_QUANTITY cannot dispatch, such as a bound of the normal range
// of its assumed value.
func failsInsideQuantity(err error) bool {
	if !isTypeFailure(err) {
		return false
	}
	for {
		de, ok := errors.AsType[*canxml.DecodeError](err)
		if !ok || de == nil {
			return false
		}
		if de.Type == "C_DV_QUANTITY" {
			inner, ok := errors.AsType[*canxml.DecodeError](de.Inner)
			return ok && inner != nil
		}
		err = de.Inner
	}
}

// tokenFacts is what the tokens of a first decode show that [tolerate] may
// read otherwise than that decode. The zero value is ready to observe.
type tokenFacts struct {
	// v2 is a namespace declaration of the namespace the AM Release-1.4
	// schemas declare, which the reading rebinds to canonical XML's.
	v2 bool
	// bareType is an element that carries a type attribute in no namespace,
	// which the decode reads as an xsi:type.
	bareType bool
	// emptyValue is an element a number or a boolean is read from (list,
	// lower, upper, assumed_value, true_valid or false_valid) that holds no
	// text and no element, comments, processing instructions and empty CDATA
	// sections aside, which the decode reads as 0 or false.
	emptyValue bool
	// otherDefinition is a <definition> typed with a class it cannot hold.
	otherDefinition bool
	// booleanGap is an element an attribute value of which names C_BOOLEAN,
	// as the type of a C_BOOLEAN item does, without a true_valid and a
	// false_valid of its own in canonical XML's namespace or none, which the
	// decode reads as false.
	booleanGap bool
	// foreign is an element in a namespace of no openEHR schema
	// (isForeignNamespace), the namespace encoding/xml names after a prefix
	// no declaration binds included, or a default namespace declared as one,
	// which the default mode reads as canonical XML's.
	foreign bool
	// archetypeIDs counts the <archetype_id> elements.
	archetypeIDs int
	// open holds one frame per element opened and not yet closed.
	open []tokenFrame
}

// tokenFrame is an open element, as [tokenFacts] follows it.
type tokenFrame struct {
	value   bool  // an element a number or a boolean is read from
	content bool  // it holds text or an element
	boolean bool  // an attribute value of it names C_BOOLEAN
	seen    uint8 // the flags of a C_BOOLEAN it holds itself
}

// observeOption returns the decode option that passes the tokens of the
// decode to f.
func (f *tokenFacts) observeOption() canxml.DecoderOption {
	opt, _ := decodeopts.Observer(f.observe).(canxml.DecoderOption)
	return opt
}

// observe reads one token of the first decode.
func (f *tokenFacts) observe(tok xml.Token) {
	switch tk := tok.(type) {
	case xml.StartElement:
		f.start(tk)
	case xml.EndElement:
		n := len(f.open)
		if n == 0 {
			return
		}
		fr := f.open[n-1]
		f.open = f.open[:n-1]
		if fr.value && !fr.content {
			f.emptyValue = true
		}
		if fr.boolean && fr.seen != seenTrueValid|seenFalseValid {
			f.booleanGap = true
		}
	case xml.CharData:
		if n := len(f.open); n > 0 && len(tk) > 0 {
			f.open[n-1].content = true
		}
	}
}

// start reads the start tag of an element.
func (f *tokenFacts) start(tk xml.StartElement) {
	local, space := tk.Name.Local, tk.Name.Space
	depth := len(f.open)
	if depth > 0 {
		parent := &f.open[depth-1]
		parent.content = true
		if parent.boolean && (space == "" || space == canxml.NSDefault) {
			switch local {
			case "true_valid":
				parent.seen |= seenTrueValid
			case "false_valid":
				parent.seen |= seenFalseValid
			}
		}
	}
	if isForeignNamespace(space) {
		f.foreign = true
	}
	var fr tokenFrame
	switch local {
	case "list", "lower", "upper", "assumed_value", "true_valid", "false_valid":
		fr.value = true
	case "archetype_id":
		f.archetypeIDs++
	}
	for _, a := range tk.Attr {
		if prefix, ok := declared(a.Name); ok {
			if a.Value == nsOpenEHRv2 {
				f.v2 = true
			}
			if prefix == "" && isForeignNamespace(a.Value) {
				f.foreign = true
			}
			continue
		}
		if a.Name.Space == "" && a.Name.Local == "type" {
			f.bareType = true
		}
		if strings.Contains(a.Value, "C_BOOLEAN") {
			fr.boolean = true
		}
		if depth == 1 && local == "definition" && a.Name.Local == "type" {
			switch a.Value {
			case "C_ARCHETYPE_ROOT", "T_ARCHETYPE_ROOT", "C_COMPLEX_OBJECT":
			default:
				f.otherDefinition = true
			}
		}
	}
	f.open = append(f.open, fr)
}

// needsTolerance reports whether the document whose tokens f observed, which
// the first decode read into ot without a failure, may hold what [tolerate]
// reads otherwise than that decode: a C_BOOLEAN item without its true_valid
// or false_valid, or with an assumed value, which may be one the reader reads
// as none; a declaration of the namespace the AM Release-1.4 schemas declare;
// a bare type attribute; an element a number or a boolean is read from that
// holds nothing; or a <definition> typed with a class it cannot hold. Unless
// strict, it also looks for an element in a namespace the decode does not
// read, which it skips, and for an archetype id on a constraint object the
// decode read as a C_COMPLEX_OBJECT, which it skips: more archetype_id
// elements than archetype roots that hold one. A fact may show what tolerate
// then leaves as it is, which costs a second reading that changes nothing;
// none misses what it looks for.
func (f *tokenFacts) needsTolerance(ot *OperationalTemplate, strict bool) bool {
	var m modelFacts
	m.walk(ot.Definition.Attributes)
	if m.booleanAssumes || f.v2 || f.bareType || f.emptyValue || f.otherDefinition || f.booleanGap {
		return true
	}
	if strict {
		return false
	}
	if ot.Definition.ArchetypeID.Value != "" {
		m.roots++
	}
	return f.foreign || f.archetypeIDs > m.roots
}

// modelFacts is what [tokenFacts.needsTolerance] reads off the model the
// first decode gave.
type modelFacts struct {
	// booleanAssumes is true where a C_BOOLEAN item has an assumed value.
	booleanAssumes bool
	// roots counts the archetype roots that hold an archetype id.
	roots int
}

// walk reads the constraint objects under attrs into m.
func (m *modelFacts) walk(attrs []CAttribute) {
	for _, a := range attrs {
		var children []CObject
		switch v := a.(type) {
		case *CSingleAttribute:
			if v != nil {
				children = v.Children
			}
		case *CMultipleAttribute:
			if v != nil {
				children = v.Children
			}
		}
		for _, c := range children {
			switch v := c.(type) {
			case *CComplexObject:
				if v != nil {
					m.walk(v.Attributes)
				}
			case *CArchetypeRoot:
				if v != nil {
					if v.ArchetypeID.Value != "" {
						m.roots++
					}
					m.walk(v.Attributes)
				}
			case *CPrimitiveObject:
				if v == nil {
					continue
				}
				if b, ok := v.Item.(*CBoolean); ok && b != nil && b.AssumedValue != nil {
					m.booleanAssumes = true
				}
			}
		}
	}
}
