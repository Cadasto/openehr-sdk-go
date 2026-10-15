package composition

import (
	"fmt"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// checkRMType verifies that v's RM type conforms to the RM type the
// compiled node declares. Returns nil when it does, ErrTypeMismatch
// (wrapped with diagnostic context) otherwise. The Go→RM-name reverse
// mapping is the generated rm.RMTypeName (ADR 0013). REQ-024, no
// reflection.
//
// v conforms when its RM type is the node's type, or a class that
// descends from it in the pinned class hierarchy (REQ-101 Type
// enforcement, read the way the REQ-102 validator binds an alternative):
// a DV_CODED_TEXT conforms to a DV_TEXT node, and a concrete data value
// to a node the OPT types as an abstract class such as DATA_VALUE or
// EVENT. tcimpl.ConformsTo answers it from the classes the compile found
// to conform to the node's type, so this package imports no class
// hierarchy of its own (REQ-013).
//
// A typed DV_INTERVAL node declares the ITS-JSON parameterised name
// (DV_INTERVAL<DV_QUANTITY>), which is no class the hierarchy knows; the
// registry reverses only the bare name, so rmnames.TypedIntervalName
// supplies the parameterised one, and the interval passes when its
// parameter is the node's own. An interface carrying a typed-nil pointer
// fails as unrecognised: a typed-nil is not a usable value at a builder
// Set site.
func checkRMType(node *tcimpl.CompiledNode, v any) error {
	want := node.RMTypeName()
	got, ok := rm.RMTypeName(v)
	if !ok {
		return fmt.Errorf("%w: value type %T is not a recognised RM concrete", ErrTypeMismatch, v)
	}
	if got == want {
		return nil
	}
	if param, ok := rmnames.TypedIntervalName(v); ok && param == want {
		return nil
	}
	if conforms, known := tcimpl.ConformsTo(node, got); known && conforms {
		return nil
	}
	return fmt.Errorf("%w: value type %T (RM %q) does not conform to path RM type %q",
		ErrTypeMismatch, v, got, want)
}
