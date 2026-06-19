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
// function bodies. Implement behaviour in hand-written companions in the
// same package (`*_ext.go`, or `*_funcs.go` for the REQ-120..123
// behavioural-function set — see [ADR 0011]); run `make codegen` after
// BMM bumps but do not edit `*_gen.go` by hand.
//
// The pure/derived behavioural functions of REQ-120..123 are realised:
// identifier parsing/derivation in identification_funcs.go, version
// helpers in changecontrol_funcs.go, temporal DV_* helpers in
// temporal_funcs.go, and openEHR-path read access in the sibling
// [github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath] package. Their
// generated stubs are suppressed via the generator's
// manual-implementation skip set ([ADR 0002] § D7). Functions still left
// as fail-loud panic stubs (temporal arithmetic, PATHABLE.parent /
// path_of_item, VERSIONED_OBJECT container ops) are deferred by design.
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
//
// [ADR 0011]: https://github.com/cadasto/openehr-sdk-go/blob/main/docs/adr/0011-rm-behavioural-functions-surface.md
// [ADR 0002]: https://github.com/cadasto/openehr-sdk-go/blob/main/docs/adr/0002-bmm-codegen-decisions.md
package rm
