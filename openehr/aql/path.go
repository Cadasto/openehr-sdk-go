package aql

// path.go: the structured identified-path vocabulary shared by the read
// AST (openehr/aql/parse) and the WHERE comparison it decorates
// (REQ-113). It lives in openehr/aql — the shared AQL
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
	// Parsed is the typed form of Predicate — REQ-113 § Structured node
	// predicates. It is a READ-SIDE derivation: no write path reads it, so
	// mutating it cannot change what the emitter produces, and Predicate
	// stays authoritative for emission (REQ-119's verbatim round trip).
	//
	// nil means the SDK does not structure this form, and that is a
	// STATEMENT, not an accident: a reader may fail closed on it. There is
	// no partial structure — a Parsed that is non-nil is complete for its
	// kind. When Predicate is "" the segment carries no predicate at all;
	// when Predicate is populated and Parsed is nil, the predicate is one of
	// the ENUMERATED unstructured forms (REQ-113 § Structured node
	// predicates): a comparison whose right-hand operand the value
	// vocabulary does not carry — an object path, a node code, or an
	// out-of-range numeric — or a junction containing one.
	//
	// NOT `==`-comparable and not a map key — see [SegmentPredicate]
	// § Comparability. Compare with [EqualPredicates].
	Parsed SegmentPredicate
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
	// Raw is the VERBATIM source text of the whole path, whitespace
	// included. It was once whitespace-collapsed, which broke REQ-119
	// round-trip closure for any path carrying a predicate the grammar
	// separates with a keyword: `o/items[a/b='c' AND d/e='f']` collapsed to
	// `…'c'ANDd/e=…`, where `ANDd` re-lexes as one IDENTIFIER and the
	// emitted query no longer parses.
	Raw string
}
