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
