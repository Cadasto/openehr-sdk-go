package aql

import "errors"

// ErrInvalidQuery indicates a Query value failed validation before execution.
var ErrInvalidQuery = errors.New("aql: invalid query")

// ErrPathResolution indicates the backend could not resolve a path referenced
// by the query (a semantic, not syntactic, failure). The typed builders cannot
// emit a syntactically invalid query, so path resolution is the failure mode
// that survives to execution; the query executor maps the backend's AQL error
// envelope to this sentinel. Detect with errors.Is.
var ErrPathResolution = errors.New("aql: path resolution failed")

// ErrEngineCapability indicates a capability gap, not a client error: the
// query is valid, but this deployment does not implement it. Callers branch on
// it to retry elsewhere or degrade gracefully, never to correct the query. Bad
// AQL (syntax, semantically impossible containment, malformed path form)
// arrives as HTTP 400 instead. The query executor maps HTTP 501 to this
// sentinel from the status alone, with or without an openEHR error envelope,
// and never from message text. Detect with errors.Is.
var ErrEngineCapability = errors.New("aql: engine does not implement this AQL capability")

// ErrSyntax indicates AQL that does not parse against the SDK grammar profile
// (resources/aql/grammar/active). parse.Parse returns it wrapped, the lint
// layer surfaces it as code "aql_syntax", and (*parse.Query).Emit carries it
// beside the dominant [ErrInvalidQuery] when the emitted text does not
// re-parse. Detect with errors.Is.
var ErrSyntax = errors.New("aql: syntax error")

// ErrIncompleteAST indicates that the source AQL parsed cleanly but contains
// a shape outside the structured-AST catalogue, so the parser cannot surface
// it as a structured [parse.Query] without losing semantics.
// parse.ParseQuery returns it wrapped (and parse.Document.QueryErr surfaces
// it) so callers can branch on errors.Is.
//
// The catalogue models every SELECT / FROM / WHERE / ORDER BY / LIMIT
// construct the SDK grammar profile admits, including the deprecated
// `SELECT TOP` clause ([TopClause]). One residual condition remains:
//
//   - A numeric literal the value vocabulary cannot represent: a `LIMIT` /
//     `OFFSET` value beyond Go `int`, a `SELECT TOP` count beyond `int`, an
//     INTEGER beyond `int64`, or a REAL beyond `float64` in either direction
//     (an overflow, or an underflow collapsing to zero, which ParseFloat
//     reports as (0, nil) instead of a range error), in any value position (a
//     SELECT literal, a comparison operand, a MATCHES member). It is refused
//     so the precision loss is never emitted as canonical text.
//
// Every other extractor branch is defensive: it is unreachable against the
// current profile and records a gap if a widened grammar ever reaches it.
var ErrIncompleteAST = errors.New("aql: parsed query carries a shape outside the structured-AST catalogue")
