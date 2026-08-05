package aql

// top.go: REQ-118 — the deprecated `SELECT TOP n [FORWARD|BACKWARD]` row
// limit, shared by the read side ([parse.SelectClause.Top]) and the write
// side ([Builder.Top] / [Builder.TopDirected]) so one model serves both
// (the REQ-113 rule).

import "strconv"

// TopDir is the optional direction of a [TopClause] — the grammar's
// `top : TOP INTEGER direction=(FORWARD|BACKWARD)?`. The zero value means
// the source wrote no direction, so a bare `TOP n` round-trips without
// acquiring one.
type TopDir int

const (
	// TopDirUnspecified means no direction keyword was written.
	TopDirUnspecified TopDir = iota
	// TopForward emits FORWARD.
	TopForward
	// TopBackward emits BACKWARD.
	TopBackward
)

// String renders the AQL keyword, or "" for [TopDirUnspecified]. An
// out-of-range value renders "" as well: a direction the vocabulary does
// not know is never emitted as text a parser would reject.
func (d TopDir) String() string {
	switch d {
	case TopForward:
		return "FORWARD"
	case TopBackward:
		return "BACKWARD"
	}
	return ""
}

// TopClause is a `SELECT TOP n [FORWARD|BACKWARD]` row limit.
//
// # Deprecated construct
//
// The `TOP` modifier is DEPRECATED by openEHR QUERY Release-1.1.0 § 4.4.3
// in favour of the `LIMIT` clause combined with `ORDER BY`, and the spec
// announces its removal in a future major release. It is modelled here
// because the SDK does not author the queries it is handed: a client, a
// stored query, or a conformance corpus may legitimately carry `TOP` until
// that removal, and a dropped row limit would silently turn a bounded
// query into an unbounded one. Prefer [Builder.LimitInline] (in-text
// `LIMIT`) or the request envelope ([Builder.Limit]) for new queries.
//
// § 4.4.3 also forbids `TOP` and `LIMIT` in the same query. The parser
// reports both as written and the lint gate diagnoses the combination
// (`aql_top_with_limit`); [Builder.Build] refuses to construct it.
//
// N is the row count; it MUST NOT be negative (the `top` production admits
// no sign). Dir is the optional direction.
type TopClause struct {
	N   int
	Dir TopDir
}

// token is the canonical wire form — `TOP 5`, `TOP 5 BACKWARD`. Keywords
// are upper-cased regardless of source casing, and the direction is
// omitted when unspecified.
func (t TopClause) token() string {
	out := "TOP " + strconv.Itoa(t.N)
	if d := t.Dir.String(); d != "" {
		out += " " + d
	}
	return out
}

// FormatTop renders a [TopClause] to canonical AQL text (`TOP 5 BACKWARD`).
// Returns "" for a nil clause. Mirrors [FormatWhere] / [FormatValue] for
// the SELECT-clause side of the shared vocabulary, so a consumer holding a
// parsed clause can render it without a package-local internal.
func FormatTop(t *TopClause) string {
	if t == nil {
		return ""
	}
	return t.token()
}
