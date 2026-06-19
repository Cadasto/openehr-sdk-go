package instance

import (
	"fmt"
	"strings"

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

// parseBMMGeneric splits "BASE<PARAM>" OPT/BMM generic notation.
func parseBMMGeneric(declared string) (base, param string, ok bool) {
	i := strings.Index(declared, "<")
	if i < 0 {
		return "", "", false
	}
	j := strings.LastIndex(declared, ">")
	if j <= i {
		return "", "", false
	}
	return strings.TrimSpace(declared[:i]), strings.TrimSpace(declared[i+1 : j]), true
}

// newGenericRM materialises closed-set BMM generic RM types the OPT
// may declare with angle-bracket notation. Returns ok=false when
// declared is not a recognised generic form.
func newGenericRM(declared string) (v any, ok bool, err error) {
	base, param, ok := parseBMMGeneric(declared)
	if !ok {
		return nil, false, nil
	}
	switch base {
	case "DV_INTERVAL":
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
		case "DV_ORDERED":
			return &rm.DVInterval[rm.DVOrdered]{}, true, nil
		default:
			return nil, true, fmt.Errorf("%w: %q", rmwrite.ErrUnknownRMType, declared)
		}
	default:
		return nil, false, nil
	}
}
