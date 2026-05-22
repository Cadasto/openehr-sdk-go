// Package rminfo answers "what does the openEHR Reference Model
// declare about RM class X?" — required attributes, attribute RM
// types, multi-cardinality flags, and the universe of known concrete
// class names.
//
// The data is generated from the pinned BMM under resources/bmm/
// via internal/bmmgen and lives in lookup_gen.go; this package
// contains only the Lookup interface and the [Default] accessor.
// No runtime BMM dependency — generated tables are pure Go strings.
//
// Consumed by [internal/templatecompile] (REQ-100 follow-up Phase 4)
// for implicit attribute injection on the compiled template, and
// by composition-builder / validator code that needs to enumerate
// the RM-mandatory fields the OPT does not model explicitly (e.g.
// COMPOSITION.category, COMPOSITION.language).
//
// Building-block weight: stdlib-only, single internal data table,
// no init-time work beyond a map literal. Safe to import from any
// SDK sub-package.
package rminfo
