// Package composition is the generic, OPT-driven Composition builder:
// path-value assignment against a parsed template.OperationalTemplate,
// type-safe via generics where possible.
//
// Per-OPT generated structs (e.g. for a vital-signs operational template)
// are NOT part of this package — they belong in the consuming project.
// This package is the engine; the bound types are the application's.
package composition
