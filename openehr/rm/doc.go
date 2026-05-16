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
package rm
