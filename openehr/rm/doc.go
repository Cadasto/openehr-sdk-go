// Package rm models the openEHR Reference Model.
//
// RM modeling rules:
//
//   - Concrete structs for concrete RM types (no emulation of
//     inheritance through an abstract base struct).
//   - Embedded structs for shared fields: Locatable, Pathable,
//     Identified, ContentItem, …
//   - Interfaces for abstract RM categories: DataValue, ItemStructure,
//     Entry, …
//   - A central type registry (openehr/rm/typereg) decodes the _type
//     discriminator into concrete types, never through struct tags alone.
//
// Generics carry typed responses through clients and validators
// without reflection.
//
// # Generated BMM function stubs
//
// Methods emitted from BMM `function` declarations in `*_gen.go` files
// have stub bodies that panic with `not implemented: <CLASS>.<fn>`.
// This is intentional: the generator never emits real function bodies.
// Behaviour is implemented in hand-written companions in the same
// package (`*_ext.go` and `*_funcs.go`). Run `make codegen` after BMM
// bumps; do not edit `*_gen.go` by hand.
//
// The pure and derived behavioural functions are implemented:
// identifier parsing and derivation in identification_funcs.go, version
// helpers in changecontrol_funcs.go, temporal DV_* helpers in
// temporal_funcs.go, and openEHR-path read access in the sibling
// [github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath] package. The
// generator skips the stubs for these functions. Functions still left
// as panicking stubs (temporal arithmetic, PATHABLE.parent and
// path_of_item, VERSIONED_OBJECT container operations) are deliberately
// out of scope.
//
// # Substitution slots and the `*Like` interfaces
//
// The openEHR RM permits Liskov substitution at every property slot.
// The SDK exposes this on two surfaces, with two distinct call patterns:
//
//   - Concrete parents with subtypes use `<Parent>Like` narrow
//     interfaces. Where the BMM declares a property with a concrete
//     parent class that has registered subtypes (`LOCATABLE.name
//     DV_TEXT`, `EVENT_CONTEXT.health_care_facility PARTY_IDENTIFIED`,
//     audit envelopes, OBJECT_REFs, DV_URIs), the generated field type
//     is a narrow Go interface: `DVTextLike`, `PartyIdentifiedLike`,
//     `AuditDetailsLike`, `ObjectRefLike`, `DVURILike`. Callers reach
//     parent-shared attributes through Get-prefixed methods on the
//     interface (e.g. `c.Name.GetValue()`); subtype-specific fields
//     (e.g. `DVCodedText.DefiningCode`, `PartyRelated.Relationship`)
//     need a type assertion. The Get prefix avoids a field/method name
//     collision: BMM property names like `value` are field identifiers
//     and cannot also be methods.
//
//   - Abstract RM categories use plain Go interfaces. `DataValue`,
//     `Item`, `ContentItem`, `UIDBasedID`, `PartyProxy`, `DVOrdered`,
//     `ItemStructure`, … are marker-only interfaces. Callers type
//     assert to concrete types.
//
// # Encoding substitution slots
//
// A concrete RM type may be stored in a `*Like` or abstract-category
// slot either by pointer (`&rm.DVCodedText{…}`) or by value
// (`rm.DVCodedText{…}`). Both forms encode correctly: each concrete type
// has a value-receiver `MarshalJSONTo`, so the method is in the method
// set of both the value and the pointer. This matters for callers that
// encode through the v1 `encoding/json` entry points, whose
// DefaultOptionsV1 sets `CallMethodsWithLegacySemantics` and skips a
// pointer-receiver marshal method on an unaddressable value (an
// interface or map element). The mandatory ITS-JSON `_type`
// discriminator is therefore emitted however the value was assigned,
// and callers need not remember to take a pointer.
//
// The generic bounds of `DV_INTERVAL[T]` (`lower` / `upper`) work the
// same way: a value bound in a concrete `DVInterval[DVQuantity]` runs its
// value-receiver `MarshalJSONTo`, and an interface bound in the
// `DVInterval[DVOrdered]` that typereg rebuilds on decode holds a pointer
// whose method set also carries it. The round trip keeps `_type` in
// both shapes.
//
// Closed type-switch helpers in like_accessors.go (`AsDVText`,
// `AuditDetailsBase`, `PartyIdentifiedBase`, `ObjectRefBase`) recover
// the parent struct from any subtype payload, for code that needs the
// parent struct value. Prefer the interface methods for scalar field
// reads; use the helpers when you need the full parent record.
package rm
