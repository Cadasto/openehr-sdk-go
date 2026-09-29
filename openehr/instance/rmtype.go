package instance

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// newRMForOPTType constructs a fresh RM value for an OPT-declared
// rm_type_name, including BMM generic instantiations such as
// DV_INTERVAL<DV_QUANTITY>. Abstract RM names are resolved via
// [concreteFor] before typereg lookup.
func newRMForOPTType(declared string) (any, error) {
	declared = strings.TrimSpace(declared)
	if v, ok, err := newGenericRM(declared); ok {
		if err != nil {
			return nil, err
		}
		return v, nil
	}
	return rmwrite.NewRM(concreteFor(declared))
}

// newGenericRM materialises closed-set BMM generic RM types the OPT
// may declare with angle-bracket notation. Returns ok=false when
// declared is not a recognised generic form.
func newGenericRM(declared string) (v any, ok bool, err error) {
	base, params, ok := bmmtype.Split(declared)
	if !ok || len(params) == 0 {
		return nil, false, nil
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
			return &rm.DVInterval[rm.DVQuantity]{}, true, nil
		case "DV_COUNT":
			return &rm.DVInterval[rm.DVCount]{}, true, nil
		case "DV_DATE_TIME":
			return &rm.DVInterval[rm.DVDateTime]{}, true, nil
		case "DV_DATE":
			return &rm.DVInterval[rm.DVDate]{}, true, nil
		case "DV_TIME":
			return &rm.DVInterval[rm.DVTime]{}, true, nil
		case "DV_PROPORTION":
			return &rm.DVInterval[rm.DVProportion]{}, true, nil
		case "DV_DURATION":
			return &rm.DVInterval[rm.DVDuration]{}, true, nil
		case "DV_ORDINAL":
			return &rm.DVInterval[rm.DVOrdinal]{}, true, nil
		case "DV_SCALE":
			return &rm.DVInterval[rm.DVScale]{}, true, nil
		case "DV_ORDERED":
			return &rm.DVInterval[rm.DVOrdered]{}, true, nil
		default:
			return nil, true, fmt.Errorf("%w: %q", rmwrite.ErrUnknownRMType, declared)
		}
	default:
		return nil, false, nil
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
