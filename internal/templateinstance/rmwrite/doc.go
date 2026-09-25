// Package rmwrite writes RM attribute values by name without
// reflection. It is the inverse of openehr/validation/rmread: where
// rmread answers "give me the RM value at attribute `name` on this
// parent", rmwrite answers "attach `child` to attribute `name` on
// this parent".
//
// The API is intentionally small:
//
//	EnsureSingle(parent any, parentType, attrName string, child any) error
//	AppendMultiple(parent any, parentType, attrName string, child any) error
//	NewRM(rmTypeName string) (any, error)
//
// All three are pure structural setters. Dispatch is a closed type
// switch on the Go concrete type of `parent`, with no reflection.
// `parentType` is the OPT-declared RM class name
// (e.g. "OBSERVATION", "ITEM_LIST"); it is currently ignored for
// routing, but the parameter is kept so a future string-keyed dispatch
// can land without an API break, mirroring the rmread.ReadSingle
// signature.
//
// # Scope
//
// rmwrite is a low-level setter. It does not set LOCATABLE
// bookkeeping (`archetype_node_id`, `name`, `uid`,
// `archetype_details`); that is the caller's (`openehr/instance/`)
// responsibility. rmwrite binds one RM value into one named slot on
// a parent RM value. Higher-level identity,
// terminology, and template-id wiring live in the instance
// generator above.
//
// # Coverage
//
// Rows cover every (RMType, attr) pair the template-driven
// validator's rmread side handles, plus the inverse for write
// access. The closed taxonomy is asserted by table-driven tests in
// this package.
//
// # Dependencies
//
// This package imports only the standard library, openehr/rm, and
// openehr/rm/typereg. It does not import openehr/template,
// internal/templatecompile, openehr/validation, or any wire / auth /
// transport layer; rmwrite is infrastructure shared with validation
// and does not depend on it. Adding a new RM type means adding
// one switch case in each of the three exports and one rmread row
// in parallel.
package rmwrite
