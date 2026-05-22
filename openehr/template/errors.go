package template

import "errors"

// Sentinel errors per REQ-100 § Error taxonomy. Callers compare with
// errors.Is rather than equality, since parser internals wrap them
// with positional context via fmt.Errorf("...: %w", err).
var (
	// ErrInvalidOPT signals malformed XML, missing required wrapper
	// elements (template_id, definition), or an unsupported root
	// element.
	ErrInvalidOPT = errors.New("template: invalid OPT")

	// ErrNotOPTFile signals ParseFile was called with a path whose
	// suffix is not .opt (case-insensitive).
	ErrNotOPTFile = errors.New("template: not an .opt file")

	// ErrPathSyntax signals a path string failed the grammar subset
	// REQ-100 § Path syntax defines.
	ErrPathSyntax = errors.New("template: invalid path syntax")

	// ErrPathNotFound signals NodeAt traversed through an unknown
	// attribute or could not match a segment predicate.
	ErrPathNotFound = errors.New("template: path not found")

	// ErrUnsupportedNode signals the parser encountered an OPT XML
	// element shape outside the v1 node taxonomy. It is a
	// forward-compatible escape hatch — callers may inspect the
	// wrapped detail and decide whether to skip or fail.
	ErrUnsupportedNode = errors.New("template: unsupported node shape")
)
