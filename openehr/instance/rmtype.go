package instance

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// newRMForOPTType constructs a fresh RM value for an OPT-declared
// rm_type_name, including BMM generic instantiations such as
// DV_INTERVAL<DV_QUANTITY>. Abstract RM names are resolved via
// [concreteFor] before typereg lookup. A name it cannot build returns an
// error wrapping rmwrite.ErrUnknownRMType; a caller that drops that error
// calls [newRMIfKnown] instead.
func newRMForOPTType(declared string) (any, error) {
	declared = strings.TrimSpace(declared)
	if v, generic := newGenericRM(declared); generic {
		if v == nil {
			return nil, fmt.Errorf("%w: %q", rmwrite.ErrUnknownRMType, declared)
		}
		return v, nil
	}
	return rmwrite.NewRM(concreteFor(declared))
}

// newRMIfKnown builds what [newRMForOPTType] builds for declared, and
// reports false where that returns an error, without building one: a
// generic form with no instantiation here, or a name the type registry
// does not know, such as a BMM primitive (Boolean, Integer, Real), which an
// attribute the generator fills from the BMM alone can have.
func newRMIfKnown(declared string) (any, bool) {
	declared = strings.TrimSpace(declared)
	if v, generic := newGenericRM(declared); generic {
		return v, v != nil
	}
	ctor, ok := typereg.Default.Lookup(concreteFor(declared))
	if !ok {
		return nil, false
	}
	return ctor(), true
}

// newGenericRM materialises closed-set BMM generic RM types the OPT
// may declare with angle-bracket notation. generic is false when
// declared is not a recognised generic form; v is nil when it is one
// but names no instantiation built here.
func newGenericRM(declared string) (v any, generic bool) {
	base, params, ok := bmmtype.Split(declared)
	if !ok || len(params) == 0 {
		return nil, false
	}
	if base != "DV_INTERVAL" {
		return nil, false
	}
	// DV_INTERVAL takes one parameter; any other count is no known
	// instantiation, like an unknown parameter.
	if len(params) != 1 {
		return nil, true
	}
	build, ok := intervalOver[params[0]]
	if !ok {
		return nil, true
	}
	return build(), true
}

// intervalOver builds a DV_INTERVAL over each parameter the generator
// builds one over: every concrete DV_ORDERED class, and DV_ORDERED itself,
// as an interval whose bounds may be any ordered value. [intervalFlags]
// names the same instantiations.
var intervalOver = map[string]func() any{
	"DV_QUANTITY":   func() any { return &rm.DVInterval[rm.DVQuantity]{} },
	"DV_COUNT":      func() any { return &rm.DVInterval[rm.DVCount]{} },
	"DV_DATE_TIME":  func() any { return &rm.DVInterval[rm.DVDateTime]{} },
	"DV_DATE":       func() any { return &rm.DVInterval[rm.DVDate]{} },
	"DV_TIME":       func() any { return &rm.DVInterval[rm.DVTime]{} },
	"DV_PROPORTION": func() any { return &rm.DVInterval[rm.DVProportion]{} },
	"DV_DURATION":   func() any { return &rm.DVInterval[rm.DVDuration]{} },
	"DV_ORDINAL":    func() any { return &rm.DVInterval[rm.DVOrdinal]{} },
	"DV_SCALE":      func() any { return &rm.DVInterval[rm.DVScale]{} },
	"DV_ORDERED":    func() any { return &rm.DVInterval[rm.DVOrdered]{} },
}

// abstractIntervalParameter returns the parameter of declared, a
// DV_INTERVAL the generator builds no interval over as written because its
// parameter is an abstract DV_ORDERED class other than DV_ORDERED itself,
// such as DV_INTERVAL<DV_QUANTIFIED>. ok is false for any other declared
// type, an interval over an abstract class that is no DV_ORDERED, such as
// DATA_VALUE, included: the RM admits no such parameter.
func abstractIntervalParameter(declared string) (param string, ok bool) {
	if !strings.HasPrefix(declared, "DV_INTERVAL") {
		return "", false
	}
	base, params, split := bmmtype.Split(declared)
	if !split || base != "DV_INTERVAL" || len(params) != 1 {
		return "", false
	}
	if _, builds := intervalOver[params[0]]; builds {
		return "", false
	}
	h, isHierarchy := rminfo.Default.(rminfo.Hierarchy)
	if !isHierarchy {
		return "", false
	}
	abstract, known := h.IsAbstract(params[0])
	return params[0], known && abstract && descendsFrom(params[0], "DV_ORDERED")
}

// concreteIntervalParameter returns the concrete descendant of param, an
// abstract class, that an interval over param is built over: the first, in
// name order, that intervalOver builds an interval over, or "" for none.
// concreteFor names no substitute for such a class, so the pinned BMM's
// class hierarchy gives it, as it gives every concrete descendant.
func concreteIntervalParameter(param string) string {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return ""
	}
	descendants, _ := h.ConcreteDescendants(param)
	for _, d := range descendants {
		if _, builds := intervalOver[d]; builds {
			return d
		}
	}
	return ""
}

// firstConcreteDataValue returns the first concrete descendant, in name
// order, of rmType when the pinned BMM knows it as an abstract data-value
// class that concreteFor's table does not name, such as DV_QUANTIFIED,
// DV_ORDERED or DV_TEMPORAL, and "" otherwise. Other abstract classes keep
// failing as unknown RM types, as before.
func firstConcreteDataValue(rmType string) string {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return ""
	}
	if abstract, known := h.IsAbstract(rmType); !known || !abstract {
		return ""
	}
	if conforms, known := h.ConformsTo(rmType, "DATA_VALUE"); !known || !conforms {
		return ""
	}
	descendants, _ := h.ConcreteDescendants(rmType)
	if len(descendants) == 0 {
		return ""
	}
	return descendants[0]
}

// descendsFrom reports whether the pinned BMM knows sub as rmType or one
// of its descendants.
func descendsFrom(sub, rmType string) bool {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return false
	}
	conforms, known := h.ConformsTo(sub, rmType)
	return known && conforms
}

// boundaryFlags points at the four boundary flags of one interval.
type boundaryFlags struct {
	lowerUnbounded, upperUnbounded *bool
	lowerIncluded, upperIncluded   *bool
}

func flagsOf[T any](iv *rm.Interval[T]) boundaryFlags {
	return boundaryFlags{
		lowerUnbounded: &iv.LowerUnbounded,
		upperUnbounded: &iv.UpperUnbounded,
		lowerIncluded:  &iv.LowerIncluded,
		upperIncluded:  &iv.UpperIncluded,
	}
}

// intervalFlags returns the boundary flags of v when v is one of the
// DV_INTERVAL instantiations [newGenericRM] builds, and false otherwise.
// The two lists name the same instantiations.
func intervalFlags(v any) (boundaryFlags, bool) {
	switch iv := v.(type) {
	case *rm.DVInterval[rm.DVQuantity]:
		return flagsOf(&iv.Interval), true
	case *rm.DVInterval[rm.DVCount]:
		return flagsOf(&iv.Interval), true
	case *rm.DVInterval[rm.DVDateTime]:
		return flagsOf(&iv.Interval), true
	case *rm.DVInterval[rm.DVDate]:
		return flagsOf(&iv.Interval), true
	case *rm.DVInterval[rm.DVTime]:
		return flagsOf(&iv.Interval), true
	case *rm.DVInterval[rm.DVProportion]:
		return flagsOf(&iv.Interval), true
	case *rm.DVInterval[rm.DVDuration]:
		return flagsOf(&iv.Interval), true
	case *rm.DVInterval[rm.DVOrdinal]:
		return flagsOf(&iv.Interval), true
	case *rm.DVInterval[rm.DVScale]:
		return flagsOf(&iv.Interval), true
	case *rm.DVInterval[rm.DVOrdered]:
		return flagsOf(&iv.Interval), true
	}
	return boundaryFlags{}, false
}
