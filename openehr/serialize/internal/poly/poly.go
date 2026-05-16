// Package poly holds shared helpers for polymorphic discrimination
// across the canjson and canxml codecs. It is intentionally
// unexported (internal/) — only the sibling serialize/canjson and
// serialize/canxml packages depend on it.
//
// The [DecodeError] envelope itself lives in
// [github.com/cadasto/openehr-sdk-go/openehr/rm/typereg] so generator
// output under `openehr/rm/*_jsonunmar_gen.go` can construct it
// without forming an import cycle into a codec-specific package.
// This package re-exports the type via a Go type alias.
package poly

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// ResolveType looks up a constructor in [typereg.Default]. Returns
// [typereg.ErrUnknownType] (wrapped with the type name for context)
// when the discriminator is not registered.
func ResolveType(name string) (func() any, error) {
	ctor, ok := typereg.Default.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("poly: %q: %w", name, typereg.ErrUnknownType)
	}
	return ctor, nil
}

// DecodeError is re-exported from typereg so canjson / canxml have a
// codec-package handle without importing typereg directly at every
// site.
type DecodeError = typereg.DecodeError
