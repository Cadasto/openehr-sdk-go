package template

import "errors"

// Sentinel errors returned by this package and by path lookup on a
// compiled template. Callers compare with errors.Is rather than equality,
// since each error that wraps one adds its context with
// fmt.Errorf("...: %w", err).
var (
	// ErrPathSyntax signals a path string failed the supported path
	// grammar (see [ParsePath]).
	ErrPathSyntax = errors.New("template: invalid path syntax")

	// ErrPathNotFound signals that path lookup, Compiled.Resolve in
	// package templatecompile, traversed through an unknown attribute or
	// could not match a segment predicate.
	ErrPathNotFound = errors.New("template: path not found")

	// ErrAmbiguousPath signals strict-mode resolution found multiple
	// candidate children for a predicate-less segment. Lenient mode
	// picks the first child; strict mode makes the caller disambiguate
	// with a predicate. Only returned when Compiled.Resolve is called
	// with templatecompile.WithStrictPaths.
	ErrAmbiguousPath = errors.New("template: ambiguous path resolution")
)
