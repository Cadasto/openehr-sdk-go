package template

import "errors"

// Sentinel errors per REQ-100 § Error taxonomy. Callers compare with
// errors.Is rather than equality, since parser internals wrap them
// with positional context via fmt.Errorf("...: %w", err).
var (
	// ErrInvalidOPT signals malformed XML or missing required
	// wrapper elements (template_id, definition). encoding/xml
	// errors from the XML decoder are wrapped through this sentinel
	// — callers can match either with errors.Is(err, ErrInvalidOPT)
	// or unwrap to the inner decoder error.
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

	// ErrUnsupportedNode signals the parser encountered an
	// <attributes> element whose xsi:type is outside the v1
	// attribute taxonomy (C_SINGLE_ATTRIBUTE, C_MULTIPLE_ATTRIBUTE),
	// or — in strict mode — a <children> xsi:type that the parser
	// does not recognise AND that carries nested attributes. In
	// lenient mode, unknown <children> xsi:type values are admitted
	// as leaf *ComplexObject nodes (forward-compatible escape
	// hatch).
	ErrUnsupportedNode = errors.New("template: unsupported node shape")

	// ErrAmbiguousPath signals strict-mode resolution found multiple
	// candidate children for a predicate-less segment (the lenient
	// "first-child" rule per REQ-100 would arbitrate a pick; strict
	// mode forces the caller to disambiguate via predicate). Only
	// surfaced when NodeAt is called with WithStrictPaths().
	ErrAmbiguousPath = errors.New("template: ambiguous path resolution")
)
