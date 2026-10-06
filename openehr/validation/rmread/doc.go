// Package rmread reads RM attribute values by name without
// reflection. It is the lookup half of the template-driven
// validator: the template walker drives traversal
// by walking the compiled OPT tree and asks this package "give me
// the RM value(s) at attribute `name` on this parent RM object".
//
// The API is intentionally small:
//
//	ReadSingle(parent any, parentType, attrName string) (val any, ok bool)
//	ReadMultiple(parent any, parentType, attrName string) (items []any, ok bool)
//
// Both functions dispatch on the concrete Go type of `parent` via
// a closed switch, with no reflection. `parentType` is the
// OPT-declared RM class name (e.g. "OBSERVATION", "DV_CODED_TEXT").
// It is currently unused for routing (type assertion on `parent`
// is authoritative), but is accepted as a parameter so callers
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
// Rows cover the attributes the validators walk on the types reachable
// from COMPOSITION through the supported content types (Observation,
// Evaluation, Instruction, Action, AdminEntry, Section,
// GenericEntry) plus the History / Event / ItemStructure / Item /
// DataValue paths below them. They also cover the party proxies
// (PARTY_SELF, PARTY_IDENTIFIED with its identifiers, PARTY_RELATED)
// wherever one sits, and PARTICIPATION in the two lists that hold it:
// an EVENT_CONTEXT's participations and an ENTRY's
// other_participations. Beyond COMPOSITION they cover the
// other LOCATABLE roots (FOLDER, EHR_STATUS, EHR_ACCESS and the
// demographic PARTY hierarchy with its parts), the ARCHETYPED node
// under every LOCATABLE's archetype_details, and every DV_ORDERED
// concrete with the attributes DV_ORDERED declares (normal_status,
// normal_range, other_reference_ranges and its REFERENCE_RANGE
// elements) and, where the RM declares it, accuracy. Every
// registered LOCATABLE and DV_ORDERED concrete is modelled. The
// closed taxonomy is asserted by table-driven and registry-driven
// tests in this package.
//
// A modelled type is not read in full. These optional attributes have
// no row, so reading one returns (nil, false), as for an unknown pair:
//
//   - uid, links and feeder_audit on every LOCATABLE;
//   - workflow_id on every ENTRY, and guideline_id on every CARE_ENTRY;
//   - hyperlink, language and encoding on DV_TEXT and DV_CODED_TEXT;
//   - every optional attribute of DV_MULTIMEDIA except alternate_text,
//     and charset and language on DV_PARSABLE;
//   - precision, units_system and units_display_name on DV_QUANTITY,
//     accuracy_is_percent on the amounts, and magnitude_status on every
//     quantified value except DV_QUANTITY;
//   - reason on ISM_TRANSITION, and time_validity on CONTACT and
//     PARTY_RELATIONSHIP.
//
// # Dependencies
//
// This package imports only the standard library and openehr/rm.
// It does not import openehr/template, internal/templatecompile,
// or any other validation-side package; the table is a pure
// lookup over RM values.
package rmread
