package instance

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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
	switch base {
	case "DV_INTERVAL":
		// DV_INTERVAL takes one parameter; any other count is no known
		// instantiation, like an unknown parameter.
		param := ""
		if len(params) == 1 {
			param = params[0]
		}
		switch param {
		case "DV_QUANTITY":
			return &rm.DVInterval[rm.DVQuantity]{}, true
		case "DV_COUNT":
			return &rm.DVInterval[rm.DVCount]{}, true
		case "DV_DATE_TIME":
			return &rm.DVInterval[rm.DVDateTime]{}, true
		case "DV_DATE":
			return &rm.DVInterval[rm.DVDate]{}, true
		case "DV_TIME":
			return &rm.DVInterval[rm.DVTime]{}, true
		case "DV_PROPORTION":
			return &rm.DVInterval[rm.DVProportion]{}, true
		case "DV_DURATION":
			return &rm.DVInterval[rm.DVDuration]{}, true
		case "DV_ORDINAL":
			return &rm.DVInterval[rm.DVOrdinal]{}, true
		case "DV_SCALE":
			return &rm.DVInterval[rm.DVScale]{}, true
		case "DV_ORDERED":
			return &rm.DVInterval[rm.DVOrdered]{}, true
		default:
			return nil, true
		}
	default:
		return nil, false
	}
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
