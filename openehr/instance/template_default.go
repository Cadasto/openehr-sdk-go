package instance

// template_default.go: the default value a template's <constraints> set on
// a node, which the walk writes in place of the value a fill gives.

import (
	"slices"

	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// templateDefault returns the default value the template's <constraints>
// set on node (CompiledNode.DefaultValue), to stand in place of built, the
// value the walk built for node under either value fill, and nil where
// there is none to write: node carries no default, the default is not of
// built's RM type, or it does not validate under node's constraints
// (defaultFits). The template's <assumed_value>, a constraint's Default, is
// not read.
func templateDefault(node *tcimpl.CompiledNode, built any) any {
	def := node.DefaultValue()
	if def == nil || rm.IsTypedNil(def) || rmTypeOf(def) != rmTypeOf(built) || !defaultFits(node, def) {
		return nil
	}
	return def
}

// defaultFits reports whether v, a value set as the default of node,
// validates under the constraints node carries, as the walk holds them: the
// primitive constraint of node itself, where it has one, accepts v; and each
// attribute of node holds in v what the attribute allows. An attribute node
// requires by its existence must be there, one node prohibits must not, and
// each value v holds there must meet one of the attribute's allowed
// children (valueFits). An attribute of v that node does not name is not
// read.
func defaultFits(node *tcimpl.CompiledNode, v any) bool {
	if pc := node.PrimitiveConstraint(); pc != nil {
		in, ok := constraintInput(v)
		if !ok || len(pc.Validate(in)) != 0 {
			return false
		}
	}
	for _, attr := range tcimpl.NodeAttributes(node) {
		var values []any
		if attr.Cardinality() == template.Multiple {
			values, _ = rmaccess.Multiple(v, attr.Name())
		} else if value, present := rmaccess.Single(v, attr.Name()); present {
			values = []any{value}
		}
		switch {
		case len(values) == 0:
			if tcimpl.LowerAtLeastOne(attr.Existence()) {
				return false
			}
			continue
		case tcimpl.Prohibited(attr):
			return false
		}
		children := tcimpl.AllowedChildNodes(attr)
		if len(children) == 0 {
			continue
		}
		for _, value := range values {
			if !slices.ContainsFunc(children, func(child *tcimpl.CompiledNode) bool { return valueFits(child, value) }) {
				return false
			}
		}
	}
	return true
}

// valueFits reports whether value, held by an attribute of a default
// value, meets child, an allowed child of that attribute: a primitive the
// child's primitive constraint accepts, where the child is an AOM 1.4
// primitive such as STRING or INTEGER, and otherwise a value of the child's
// RM type, or of one that descends from it, that meets the child as
// defaultFits says.
func valueFits(child *tcimpl.CompiledNode, value any) bool {
	if tcimpl.IsAOMPrimitiveShortName(child.RMTypeName()) {
		return componentFits(child, primitiveInput(value))
	}
	built := builtRMType(child)
	if got := rmTypeOf(value); got != built && !descendsFrom(got, built) {
		return false
	}
	return defaultFits(child, value)
}

// constraintInput returns the value a primitive constraint's Validate takes
// for v, a data value or a code phrase, the shape the template validator
// gives it:
// a DV_QUANTITY's magnitude, units and precision (-1 where it has none), a
// coded text's or a code phrase's code, a DV_ORDINAL's value and the code of
// its symbol, the value of a DV_TEXT, a DV_BOOLEAN or a temporal value, and
// a DV_COUNT's magnitude. It reports false for any other value.
func constraintInput(v any) (any, bool) {
	switch v := v.(type) {
	case *rm.DVQuantity:
		precision := -1
		if v.Precision != nil {
			precision = int(*v.Precision)
		}
		return constraints.QuantityValue{Magnitude: float64(v.Magnitude), Units: v.Units, Precision: precision}, true
	case *rm.DVCodedText:
		return codedTerm(v.DefiningCode), true
	case *rm.CodePhrase:
		return codedTerm(*v), true
	case rm.CodePhrase:
		return codedTerm(v), true
	case *rm.DVOrdinal:
		return constraints.OrdinalSymbol{Value: int(v.Value), Symbol: codedTerm(v.Symbol.DefiningCode)}, true
	case *rm.DVText:
		return v.Value, true
	case *rm.DVBoolean:
		return v.Value, true
	case *rm.DVCount:
		return int64(v.Magnitude), true
	case *rm.DVDate:
		return v.Value, true
	case *rm.DVTime:
		return v.Value, true
	case *rm.DVDateTime:
		return v.Value, true
	case *rm.DVDuration:
		return v.Value, true
	}
	return nil, false
}

// codedTerm is the code of cp as a primitive constraint reads it.
func codedTerm(cp rm.CodePhrase) constraints.CodedTermRef {
	return constraints.CodedTermRef{Terminology: cp.TerminologyID.Value, CodeString: cp.CodeString}
}

// primitiveInput returns the value a primitive constraint's Validate takes
// for value, an RM primitive an attribute holds: an RM Integer, Integer64 or
// Real as the Go number it names, a Character as a string, and the string
// an optional String attribute points at. Any other value is returned as
// it is.
func primitiveInput(value any) any {
	switch v := value.(type) {
	case rm.Integer:
		return int64(v)
	case rm.Integer64:
		return int64(v)
	case rm.Real:
		return float64(v)
	case rm.Character:
		return string(v)
	case *string:
		if v == nil {
			return ""
		}
		return *v
	}
	return value
}
