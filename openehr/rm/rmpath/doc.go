// Package rmpath provides read navigation of an in-memory openEHR RM
// instance by an openEHR path — the PATHABLE read operations
// (item_at_path, items_at_path, path_exists, path_unique) over the
// actual object tree (REQ-121).
//
// # Surface
//
// The operations are package functions rather than methods on the RM
// types. rmpath imports openehr/rm; a delegating rm.LOCATABLE method
// would create an import cycle, so the generated path-method stubs are
// suppressed (see ADR 0011) and these functions are the canonical
// surface:
//
//	v, err := rmpath.ItemAtPath(comp, "/content[at0001]/data[at0002]/events[at0003]/data/items[at0004]/value")
//
// # Path grammar
//
// A path is "/"-separated RM attribute-name segments, each with an
// optional predicate:
//
//	/attr                       — attribute, no filter
//	/attr[at0001]               — filter children by archetype_node_id
//	/attr['systolic']           — filter children by name/value
//	/attr[at0001,'systolic']    — both (comma- or " and "-separated;
//	/attr[at0001 and name/value='systolic']   AQL-style also accepted)
//
// Resolution is reflection-free: a typed walker dispatches attribute
// access per RM type. An empty path (or "/") denotes the root itself.
//
// # Coverage
//
// The walker covers the clinical composition spine — COMPOSITION,
// SECTION, the ENTRY types, HISTORY / EVENT, the ITEM_STRUCTURE
// variants, CLUSTER, ELEMENT — down to the ELEMENT leaf (its value, plus
// name and null_flavour). Data-value internals (e.g. DV_QUANTITY.units)
// are not traversed. Demographic (PARTY) and EHR/admin object navigation
// are not yet covered; an unresolvable attribute simply yields no match
// (path_exists = false). Convergence onto a single shared RM navigator
// (with openehr/validation/rmread) is a possible later step (ADR 0011).
//
// # Fallibility
//
// No function panics. ItemAtPath returns ErrPathNotFound when nothing
// matches and ErrPathAmbiguous when more than one item matches;
// malformed path syntax surfaces as ErrPathSyntax. PathExists /
// PathUnique report booleans and never error.
package rmpath
