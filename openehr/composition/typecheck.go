package composition

import (
	"fmt"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
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
// DV_INTERVAL instantiations (absent from the old table) now
// type-check instead of failing as unrecognised, and an interface
// carrying a typed-nil pointer now fails as unrecognised instead of
// matching its type name — a typed-nil is not a usable value at a
// builder Set site.
func checkRMType(node *tcimpl.CompiledNode, v any) error {
	want := node.RMTypeName()
	got, ok := rm.RMTypeName(v)
	if !ok {
		return fmt.Errorf("%w: value type %T is not a recognised RM concrete", ErrTypeMismatch, v)
	}
	if got != want {
		return fmt.Errorf("%w: value type %T (RM %q) does not match path RM type %q",
			ErrTypeMismatch, v, got, want)
	}
	return nil
}
