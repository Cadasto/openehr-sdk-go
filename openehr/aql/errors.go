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
// Catalogue gaps that produce this error today: function-call LHS in WHERE
// (`LENGTH(x) > 5`), MATCHES with terminology-function / URI operand, path-vs-
// path comparisons (`a/x = b/y`), Primitive in SELECT projection, mixed
// `SELECT *, col` star plus columns, and a top-level boolean junction at the
// FROM root (`FROM A AND B`). Each gap is a forward-compatible extension —
// the v1 catalogue is the buildable grammar plus the parser-only shapes
// (Not / Exists / Like / Matches).
var ErrIncompleteAST = errors.New("aql: parsed query carries a shape outside the structured-AST catalogue")
