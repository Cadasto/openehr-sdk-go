// Package aql provides:
//
//   - A struct-builder AQL builder and a verb-functions builder — both
//     produce identical AQL on the wire.
//   - AQL request and result models usable without an executor.
//   - Shared sentinels: ErrInvalidQuery, ErrPathResolution (execute-time),
//     and ErrSyntax (parse-time).
//
// The executor lives at openehr/client/query and wraps this package.
// Parsing and static lint of AQL strings (REQ-109) live in the building-block
// subpackages openehr/aql/parse (syntax → generated-type-free AST) and
// openehr/aql/lint (syntax + shape + template checks); validation.ValidateAQL
// bridges lint into the shared validation Issue model. Programs that only need
// to construct AQL (no execution) can import openehr/aql alone.
package aql
