// Package templatecompile turns an ADL 1.4 operational template, read into
// the operational template model (openehr/aom/aom14), into a walker-friendly
// compiled form that the composition builder, validator, and example
// generator consume.
//
// The model is a faithful decode of the template's XML; this package reads
// it as the compiled form needs it, values trimmed and intervals closed, and
// adds what downstream consumers repeatedly need:
//
//   - Stable AQL paths for every node (computed once, cached).
//   - Implicit RM attributes the OPT omits but the BMM declares
//     as mandatory (e.g. COMPOSITION.category, language, territory,
//     composer). Sourced from openehr/rm/rminfo.
//   - Typed primitive constraints and slot rules, built from the model's
//     primitive classes and slot assertions (openehr/template/constraints).
//   - The default values of the template's <constraints>, each on the node
//     it addresses ([CompiledNode.DefaultValue]).
//   - Per-archetype-root term definitions on [CompiledNode]
//     (scoped at-codes; not flattened to a single global map).
//     Term bindings flatten to [Compiled.TermBindings].
//   - Flat indexes by RM type name and by archetype node id for
//     O(1) lookup during walks.
//
// This is an internal package. Code outside the SDK module reaches the
// compiled form through the public openehr/templatecompile bridge.
package templatecompile
