package composition

import (
	"fmt"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// checkRMType verifies v's Go concrete type matches the RM type the
// compiled node declares. Returns nil on match, ErrTypeMismatch
// (wrapped with diagnostic context) otherwise. The Go→RM-name reverse
// mapping is the generated rm.RMTypeName (ADR 0013) — the previous
// hand-written mirror of the typereg table is retired. REQ-024, no
// reflection.
//
// Two deliberate consequences of the generated mapping: typed
// DV_INTERVAL instantiations (absent from the old table, and rejected
// as unrecognised) now type-check bound-aware — a compiled node
// declares the ITS-JSON parameterised name (DV_INTERVAL<DV_QUANTITY>),
// so the comparison consults rmnames.TypedIntervalName when the bare
// registry name alone does not match. And an interface carrying a
// typed-nil pointer now fails as unrecognised instead of matching its
// type name — a typed-nil is not a usable value at a builder Set site.
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
	return fmt.Errorf("%w: value type %T (RM %q) does not match path RM type %q",
		ErrTypeMismatch, v, got, want)
}
