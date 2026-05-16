// Package openehr is a namespace marker — generic openEHR primitives
// live in its sub-packages (rm, serialize, validation, template, aql,
// composition, client). This package itself intentionally exports
// nothing; consumers import the leaves they need.
//
// Each sub-package is independently usable: applications can import
// only openehr/rm for RM modeling, only openehr/serialize for
// canonicalization, etc., without constructing an authenticated client.
package openehr
