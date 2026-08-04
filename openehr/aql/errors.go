package aql

import "errors"

// ErrInvalidQuery indicates a Query value failed validation before execution.
var ErrInvalidQuery = errors.New("aql: invalid query")

// ErrPathResolution indicates the backend could not resolve a path referenced
// by the query (a semantic, not syntactic, failure). The typed builders cannot
// emit a syntactically invalid query, so path resolution is the failure mode
// that survives to execution; the query executor maps the backend's AQL error
// envelope to this sentinel (PROBE-021). Detect with errors.Is.
var ErrPathResolution = errors.New("aql: path resolution failed")

// ErrSyntax indicates AQL that does not parse against the SDK grammar profile
// (REQ-109; resources/aql/grammar/active, ADR 0007). Returned wrapped by
// parse.Parse and surfaced by the lint layer as code "aql_syntax". Detect with
// errors.Is.
var ErrSyntax = errors.New("aql: syntax error")

// ErrIncompleteAST indicates that the source AQL parsed cleanly but contains
// a shape outside the Tier-2 extraction catalogue (REQ-113) — the
// parser cannot surface it as a structured [parse.Query] without losing
// semantics. Returned wrapped by parse.ParseQuery (and surfaced via
// parse.Document.QueryErr) so callers can branch on errors.Is.
//
// Since REQ-117 the catalogue covers the whole SDK grammar profile, and the
// residual condition is a single class: an INTEGER literal the AST cannot
// represent — a `LIMIT` / `OFFSET` value beyond Go `int`, or a primitive
// beyond `int64` in a value position (a SELECT literal, a comparison
// operand, a MATCHES member). Such a literal is refused loudly rather than
// degraded to a float, so the loss is never emitted as canonical text.
var ErrIncompleteAST = errors.New("aql: parsed query carries a shape outside the structured-AST catalogue")
