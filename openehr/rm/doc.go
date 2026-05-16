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
package rm
