// Package templatecompile turns a parsed OPT (openehr/template's
// wire representation) into a walker-friendly compiled form that
// the composition builder (REQ-101), validator (REQ-102), and
// example generator consume.
//
// The wire-side [template.OperationalTemplate] is a faithful
// decoder; this package augments it with information downstream
// consumers repeatedly need:
//
//   - **Stable AQL paths** for every node (computed once, cached).
//   - **Implicit RM attributes** the OPT omits but the BMM declares
//     as mandatory (e.g. COMPOSITION.category, language, territory,
//     composer). Sourced from openehr/rm/rminfo.
//   - **Per-node term lookup** keyed by archetype-node-id, flattened
//     from the per-C_ARCHETYPE_ROOT term_definitions blocks.
//   - **Flat indexes** by RM type name and by archetype node id for
//     O(1) lookup during walks.
//
// REQ-100 follow-up Phase 4. Internal package — kept out of the
// public API until REQ-101/REQ-102 confirm the stable shape. Once
// the surface stabilises, an aliased `template.Compile` /
// `template.Compiled` will be exposed.
package templatecompile
