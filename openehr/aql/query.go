// Package aql provides AQL request and result models usable without an
// HTTP executor (REQ-055). Full struct- and verb-function builders
// land in a later phase; today callers pass literal AQL strings.
package aql

import (
	"fmt"
	"strings"
)

// Query is the wire payload for ad-hoc and stored AQL execution per
// openEHR REST Query API (AdhocQueryExecute / Query schemas).
type Query struct {
	// Q is the AQL statement. Required for ad-hoc execution.
	Q string
	// Offset is the 0-based row offset into the result set.
	Offset int
	// Fetch limits the number of rows returned. Zero leaves the limit
	// to the deployment default.
	Fetch int
	// Parameters bind $name placeholders in Q. Keys MUST NOT include
	// the leading dollar sign (e.g. "ehr_id", not "$ehr_id").
	Parameters map[string]any
	// EHRID scopes execution to a single EHR when non-empty. The query
	// executor maps this to the `ehr_id` URL query parameter (distinct
	// from AQL placeholder keys inside Parameters).
	EHRID string
}

// NewQuery returns a Query with the given AQL string.
func NewQuery(q string) Query {
	return Query{Q: strings.TrimSpace(q)}
}

// String returns the trimmed AQL statement. It is the wire contract
// for the query text itself (REQ-055).
func (q Query) String() string {
	return strings.TrimSpace(q.Q)
}

// Validate reports whether q is suitable for execution.
func (q Query) Validate() error {
	if strings.TrimSpace(q.Q) == "" {
		return fmt.Errorf("%w: empty AQL", ErrInvalidQuery)
	}
	return nil
}
