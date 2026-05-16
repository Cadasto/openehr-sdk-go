// Package bmm loads openEHR Basic Meta-Model (BMM) schemas — the
// P_BMM JSON files pinned under resources/bmm/ — into an in-memory model.
//
// # Building-block use (REQ-013, REQ-045)
//
// Consumers can import this package alone to introspect the openEHR
// domain model — class hierarchy, property cardinality, generic
// parameters, function signatures, documentation — without
// instantiating transport/, auth/, or any HTTP client. Validators,
// archetype tools, custom code generators and BMM-aware diff tools
// are the expected consumers. The package depends only on the
// standard library.
//
// # Load pipeline
//
// Two entry points:
//
//   - [Load] parses one P_BMM JSON document from an io.Reader into a
//     [*Schema]. It dispatches on the per-object _type discriminator
//     for every polymorphic node (properties, types, function
//     parameters, classes) via a small table of decoder functions —
//     no reflection-based polymorphism (per REQ-040 spirit). Required
//     fields are validated and missing/unknown values are surfaced as
//     wrapped sentinel errors ([ErrUnknownType], [ErrMissingField],
//     [ErrInvalidShape]).
//
//   - [LoadAll] resolves a schema's transitive `includes` via a
//     [Resolver] and merges the result. Descendant entries shadow
//     ancestor entries with the same name (matching observed openEHR
//     practice — e.g. RM refines TRANSLATION_DETAILS from BASE).
//     Sibling-ancestor name collisions return [ErrSchemaConflict].
//     Cycles return [ErrCircularIncludes].
//
// # Type registry (REQ-040)
//
// The decoder uses a switch statement (not reflection) on the
// _type string. The 14 known P_BMM discriminators are:
//
//   - P_BMM_SIMPLE_TYPE / P_BMM_GENERIC_TYPE / P_BMM_CONTAINER_TYPE
//   - P_BMM_SINGLE_PROPERTY / P_BMM_SINGLE_PROPERTY_OPEN /
//     P_BMM_GENERIC_PROPERTY / P_BMM_CONTAINER_PROPERTY
//   - P_BMM_SINGLE_FUNCTION_PARAMETER /
//     P_BMM_SINGLE_FUNCTION_PARAMETER_OPEN /
//     P_BMM_GENERIC_FUNCTION_PARAMETER /
//     P_BMM_CONTAINER_FUNCTION_PARAMETER
//   - P_BMM_INTERFACE / P_BMM_ENUMERATION_STRING /
//     P_BMM_ENUMERATION_INTEGER
//
// Class entries without a _type discriminator default to
// [*SimpleClass]. SimpleClass is also the carrier for generic-class
// declarations (distinguished by the presence of
// GenericParameterDefs).
//
// # Round-trip
//
// Every concrete polymorphic type implements MarshalJSON so that
// Load → MarshalJSON → Load produces a deeply-equal *Schema. The
// loader normalises the two observed CONTAINER_TYPE shapes
// (`"type":"X"` short form vs `"type_def":{...}` long form) to the
// long form; otherwise the on-wire shape is preserved.
//
// # Scope
//
// This package does NOT generate code. The code generator that emits
// openehr/rm/ and openehr/aom/aom14/ lives in internal/bmmgen and
// cmd/bmmgen and consumes the types declared here.
//
// See specs/bmm-conformance.md for the conformance contract and
// resources/bmm/README.md for the pinned BMM file inventory.
package bmm
