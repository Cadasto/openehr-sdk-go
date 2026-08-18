// Package aql provides:
//
//   - A struct-builder AQL builder and a verb-functions builder — both
//     produce identical AQL on the wire.
//   - AQL request and result models usable without an executor.
//   - Shared sentinels: ErrInvalidQuery, ErrPathResolution (execute-time),
//     ErrSyntax (parse-time), and ErrIncompleteAST (structured-AST residual
//     after a clean parse).
//
// The executor lives at openehr/client/query and wraps this package.
// Parsing and static lint of AQL strings (REQ-109) live in the building-block
// subpackages openehr/aql/parse (syntax → generated-type-free AST) and
// openehr/aql/lint (syntax + shape + template checks); validation.ValidateAQL
// bridges lint into the shared validation Issue model. Programs that only need
// to construct AQL (no execution) can import openehr/aql alone.
//
// The SDK grammar profile (ADR 0007) documents deltas from official QUERY
// 1.1.0 — notably SDK-AQL-001 (CONTAINS vs CONTAINS_STR) and SDK-AQL-002
// (SELECT *) — in resources/aql/grammar/DIVERGENCES.md. Parse success and
// lint-clean do not imply official-spec conformance.
package aql
