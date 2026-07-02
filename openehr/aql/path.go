package aql

// path.go: the structured identified-path vocabulary shared by the read
// AST (openehr/aql/parse) and the WHERE comparison it decorates
// (SDK-GAP-19 / REQ-113). It lives in openehr/aql — the shared AQL
// vocabulary — so a WHERE [Comparison] can carry its parsed path without
// openehr/aql importing openehr/aql/parse (which would cycle, since parse
// imports aql). The parse package re-exports these as `parse.PathSegment`
// / `parse.IdentifiedPath` (the latter decorated with the parse-only
// Clause / source-Position fields), so existing consumers are unchanged.

// PathSegment is one step of an identified path: an attribute name and an
// optional predicate (the raw text inside `[...]`, brackets stripped —
// e.g. "at0001" or "name/value='Systolic'").
type PathSegment struct {
	Name      string
	Predicate string
}

// IdentifiedPath is an alias-qualified path referenced in a query (e.g.
// `o/data[at0001]/events[at0006]/value/magnitude`). The leading IDENTIFIER
// is the alias (root binding into the FROM / CONTAINS tree); the remaining
// steps are Segments. It is the structured form of a path a consumer would
// otherwise re-tokenize from raw text.
//
// Raw is authoritative for emission; Alias / Predicate / Segments are a
// read-only structured decomposition of it. The single producer (the
// parser) sets all fields consistently from one source node; a consumer
// mutating one without the others desynchronizes the value.
type IdentifiedPath struct {
	// Alias is the root binding (e.g. "o"); for a WHERE path it MUST
	// resolve to a FROM / CONTAINS class alias. "" when the path is
	// anonymous / relative.
	Alias string
	// Predicate is a predicate applied directly to the alias root
	// (`o[...]/...`), brackets stripped; "" in the common case.
	Predicate string
	// Segments are the path steps after the alias, in order.
	Segments []PathSegment
	// Raw is the whitespace-collapsed source text of the whole path.
	Raw string
}
