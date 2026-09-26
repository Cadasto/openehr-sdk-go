// Package templatecompile turns a parsed OPT (openehr/template's
// wire representation) into a walker-friendly compiled form that
// the composition builder, validator, and example generator consume.
//
// The wire-side [template.OperationalTemplate] is a faithful
// decoder; this package augments it with information downstream
// consumers repeatedly need:
//
//   - Stable AQL paths for every node (computed once, cached).
//   - Implicit RM attributes the OPT omits but the BMM declares
//     as mandatory (e.g. COMPOSITION.category, language, territory,
//     composer). Sourced from openehr/rm/rminfo.
//   - Per-archetype-root term definitions on [CompiledNode]
//     (scoped at-codes; not flattened to a single global map).
//     Term bindings flatten to [Compiled.TermBindings].
//   - Flat indexes by RM type name and by archetype node id for
//     O(1) lookup during walks.
//
// This is an internal package. Code outside the SDK module reaches the
// compiled form through the public openehr/templatecompile bridge.
package templatecompile
