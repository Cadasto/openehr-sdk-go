// Package composition is the generic, OPT-driven Composition builder
// (REQ-101). It produces a *rm.Composition graph in memory driven by
// a compiled operational template, exposing a path-first authoring
// surface (`Set(path, value)`, `SetText`, `SetQuantity`,
// `SetCodedText`) on top of an `openehr/instance` skeleton.
//
// Two entry points:
//
//	NewSkeleton(ctx, c, opts...) (*rm.Composition, error)
//	NewBuilder(ctx, c, opts...) (*Builder, error)
//
// REQ-101 is a thin shim over REQ-107: `openehr/instance` owns the
// OPT-driven recursive walk and primitive defaults; this package owns
// composition-specific options (`WithLanguage`, `WithTerritory`,
// `WithComposer`, `WithCategory`, `WithNow`) and the path-assigning
// API. There is no second OPT walker here — `rmread` (read side) and
// `rmwrite` (write side) provide the closed-dispatch attribute access
// the path navigator needs.
//
// Per-template typed builders (vital-signs-shaped struct setters) are
// out of v1 scope; they belong in the consuming project or behind an
// OET-driven authoring plan.
//
// # REQ-013 building-block independence
//
// This package imports openehr/rm, openehr/rm/typereg,
// openehr/template, openehr/template/constraints, openehr/instance,
// openehr/validation/rmread, internal/templatecompile,
// internal/templatecompile/walk, and internal/templateinstance/rmwrite.
// It does NOT import openehr/serialize, openehr/client, transport,
// auth, or openehr/validation (callers run validation separately).
package composition
