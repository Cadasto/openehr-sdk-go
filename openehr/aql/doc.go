// Package aql provides:
//
//   - A struct-builder AQL builder and a verb-functions builder — both
//     produce identical AQL on the wire.
//   - AQL request and result models usable without an executor.
//
// The executor lives at openehr/client/query and wraps this package.
// Programs that only need to construct, parse, or lint AQL strings
// (no execution) can import openehr/aql alone.
package aql
