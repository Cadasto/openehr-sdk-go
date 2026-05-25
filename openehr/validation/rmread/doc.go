// Package rmread reads RM attribute values by name without
// reflection. It is the lookup half of the REQ-102 v2
// template-driven validator: the template walker drives traversal
// by walking the compiled OPT tree and asks this package "give me
// the RM value(s) at attribute `name` on this parent RM object".
//
// The API is intentionally small:
//
//	ReadSingle(parent any, parentType, attrName string) (val any, ok bool)
//	ReadMultiple(parent any, parentType, attrName string) (items []any, ok bool)
//
// Both functions dispatch on the concrete Go type of `parent` via
// a closed switch (REQ-024 — no reflection). `parentType` is the
// OPT-declared RM class name (e.g. "OBSERVATION", "DV_CODED_TEXT");
// it is currently unused for routing — type assertion on `parent`
// is authoritative — but is accepted as a parameter so callers
// constructing transient parent values from generic interfaces
// (e.g. `ContentItem`) can pass through the compiled type without
// re-flattening it.
//
// # Semantics
//
//   - ReadSingle returns the attribute's RM value (interface- or
//     value-typed) plus `ok=true`. When the parent type carries the
//     attribute but the value is the RM zero (e.g. an empty
//     DVCodedText, a nil interface-typed slot, a typed-nil pointer
//     behind an interface such as Element.Value =
//     (*rm.DVQuantity)(nil), an empty CodePhrase), `ok` is `false`
//     so callers can flag the attribute as absent for existence
//     checks. See [IsTypedNilPointer]. Pointer-typed RM
//     attributes (e.g. `*EventContext`, `*History`) report
//     `ok=false` only when the pointer is nil; the underlying value
//     may still be the RM zero and the structural walker should
//     descend.
//
//   - ReadMultiple returns the attribute's slice (each element
//     boxed into `any`) and `ok=true` when the parent type carries
//     the attribute. A `nil` or empty slice still reports
//     `ok=true`, with `items` empty; callers that need "absent vs.
//     empty" semantics should use `len(items) == 0`.
//
//   - For unknown `(parentType, attrName)` pairs the functions
//     return `(nil, false)`. The structural validator treats this
//     as "attribute not addressable on this RM type": when the OPT
//     marks the attribute required (existence lower ≥ 1 or BMM-
//     mandatory), the walker emits a `required` issue; optional
//     attributes return silently with no issue.
//
// # Coverage
//
// Rows cover every (RMType, attr) pair reachable from COMPOSITION
// through the Phase 1 content-type closed set (Observation,
// Evaluation, Instruction, Action, AdminEntry, Section,
// GenericEntry) plus History / Event / ItemStructure / Item /
// DataValue paths the v1 walker exercised. The closed taxonomy is
// asserted by table-driven tests in this package.
//
// # REQ-013 building-block independence
//
// This package imports only the standard library and openehr/rm.
// It does NOT import openehr/template, internal/templatecompile,
// or any other validation-side package — the table is a pure
// lookup over RM values.
package rmread
