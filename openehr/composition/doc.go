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
// `WithComposer`, `WithCategory`, `WithNow`, `WithPolicy`,
// `WithValueFill`, `WithValueSource`, `WithUIDSource`) and the
// path-assigning API.
// With no WithPolicy, NewSkeleton builds instance.Medium, the instance
// package's default: a skeleton with at most one optional archetype under
// an attribute. `WithPolicy(instance.Required)` gives one with only what
// the template makes mandatory and a composition's context, and
// `WithPolicy(instance.Large)` one with a member of every optional
// archetype. NewBuilder builds on instance.Large when no WithPolicy is
// given, and refuses Medium and Required with [ErrUnsupportedPolicy],
// because Set needs a member of every optional archetype, which only Large
// builds. Both entry points refuse a Policy value the instance package does
// not define with an error wrapping [ErrUnsupportedPolicy] and
// [instance.ErrUnsupportedPolicy]. There is no second OPT walker here: the
// generated attribute access of `internal/rmaccess` (read side) and
// `rmwrite` (write side) provide the closed-dispatch attribute access the
// path navigator needs.
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
// package also imports openehr/rm, openehr/template, openehr/instance,
// openehr/internal/rmnames, internal/rmaccess, internal/templatecompile
// (engine node types), and internal/templateinstance/rmwrite. It does
// not import openehr/serialize, openehr/client, transport, auth, or
// openehr/validation (callers run validation separately).
package composition
