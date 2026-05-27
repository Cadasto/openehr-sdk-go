// Package rm models the openEHR Reference Model.
//
// RM modeling rules (per the SDK Specification proposal):
//
//   - Concrete structs for concrete RM types (no abstract base struct
//     inheritance emulation).
//   - Embedded structs for shared fields: Locatable, Pathable,
//     Identified, ContentItem, …
//   - Interfaces for abstract RM categories: DataValue, ItemStructure,
//     Entry, …
//   - Central type registry (openehr/rm/typereg) decodes the _type
//     discriminator into concrete types — never via tag-magic alone.
//
// Generics carry typed responses through clients and validators
// without reflection.
//
// # Generated BMM function stubs
//
// Methods emitted from BMM `function` declarations in `*_gen.go` files
// have stub bodies that panic with `not implemented: <CLASS>.<fn>`.
// This is intentional (REQ-044): the generator never emits real
// function bodies. Implement behaviour in hand-written `*_ext.go`
// companions in the same package; run `make codegen` after BMM bumps
// but do not edit `*_gen.go` by hand.
//
// # Substitution slots and the `*Like` interfaces (SDK-GAP-11)
//
// The openEHR RM permits Liskov substitution at every property slot.
// The SDK exposes this on two surfaces, with two distinct call patterns:
//
//   - **Concrete-with-subtypes parents → `<Parent>Like` narrow
//     interfaces.** Where the BMM declares a property with a concrete
//     parent class that has registered subtypes (`LOCATABLE.name
//     DV_TEXT`, `EVENT_CONTEXT.health_care_facility PARTY_IDENTIFIED`,
//     audit envelopes, OBJECT_REFs, DV_URIs), the generated field type
//     is a narrow Go interface — `DVTextLike`, `PartyIdentifiedLike`,
//     `AuditDetailsLike`, `ObjectRefLike`, `DVURILike`. Callers reach
//     parent-shared attributes via Get-prefixed methods on the
//     interface (e.g. `c.Name.GetValue()`); subtype-specific fields
//     (e.g. `DVCodedText.DefiningCode`, `PartyRelated.Relationship`)
//     are reached via type assertion. The Get-prefix avoids the
//     field/method namespace collision Go enforces — BMM property
//     names like `value` are field identifiers and cannot also be
//     methods.
//
//   - **Abstract RM categories → existing Go interfaces.** `DataValue`,
//     `Item`, `ContentItem`, `UIDBasedID`, `PartyProxy`, `DVOrdered`,
//     `ItemStructure`, … stay marker-only interfaces. Callers type
//     assert to concrete types; helper functions land per-package as
//     needed.
//
// Closed type-switch helpers in like_accessors.go (`AsDVText`,
// `AuditDetailsBase`, `PartyIdentifiedBase`, `ObjectRefBase`) recover
// the parent struct from any subtype payload — useful at validation
// sites that consume the parent struct value. Prefer the interface
// methods for scalar field reads; reach for the helpers when you
// need the full parent record.
//
// BMM-bump audit on this surface lives in like_interfaces.go and
// like_accessors.go file comments + ADR-0001 step 10.
package rm
