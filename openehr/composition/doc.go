// Package composition is the generic, OPT-driven Composition builder.
// It produces a *rm.Composition graph in memory driven by
// a compiled operational template, exposing a path-first authoring
// surface (`Set(path, value)`, `SetText`, `SetQuantity`,
// `SetCodedText`) on top of an `openehr/instance` skeleton.
//
// Two entry points:
//
//	NewSkeleton(ctx, c, opts...) (*rm.Composition, error)
//	NewBuilder(ctx, c, opts...) (*Builder, error)
//
// This package is a thin layer over `openehr/instance`, which owns the
// OPT-driven recursive walk and primitive defaults. This package owns
// the composition-specific options (`WithLanguage`, `WithTerritory`,
// `WithComposer`, `WithCategory`, `WithNow`) and the path-assigning
// API. There is no second OPT walker here: `rmread` (read side) and
// `rmwrite` (write side) provide the closed-dispatch attribute access
// the path navigator needs.
//
// Per-template typed builders (vital-signs-shaped struct setters) are
// not provided.
//
// # Build lifecycle
//
// [Builder.Build] is repeatable: each invocation consumes the
// accumulated [Builder.Set] queue and any prior errors, so a
// subsequent Build with no intervening Set returns the same in-place
// skeleton with a nil error. Successful assignments from a failed
// Build are not replayed on the next invocation (the mutation has
// already happened in place). Pattern: Set → Build → inspect error
// → fix offending Sets → Build again.
//
// # Dependencies
//
// The exported NewBuilder / NewSkeleton signatures take the public
// compiled template openehr/templatecompile.Compiled; this
// package also imports openehr/rm, openehr/rm/typereg, openehr/template,
// openehr/template/constraints, openehr/instance, openehr/validation/rmread,
// internal/templatecompile (engine node types), and
// internal/templateinstance/rmwrite. It does not import
// openehr/serialize, openehr/client, transport, auth, or
// openehr/validation (callers run validation separately).
package composition
