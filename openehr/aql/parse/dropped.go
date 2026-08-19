package parse

// dropped.go: the value-free drop channel — REQ-113 § Value-free structured
// drop records. When the extractor cannot represent a construct it already
// records a human-readable reason (joined into the [aql.ErrIncompleteAST]
// [Document.QueryErr] returns). That text QUOTES the offending source, so a
// consumer whose refusal reasons may name structure but never a value has to
// discard it wholesale and refuse generically. The records here carry the
// same finding with no source text in any field: kind, clause, span.

import "strings"

// ConstructKind classifies one construct the extractor could not represent
// in the structured [Query] AST.
//
// The set is CLOSED and enumerates drop CLASSES, not extractor call sites:
// the extractor has many more sites than classes, and most sites are
// defensive arms that no legal query reaches while the extraction catalogue
// covers the whole grammar profile (REQ-117). A reader MUST be able to
// switch exhaustively and fail closed; adding a member is a consumer-visible
// change.
type ConstructKind int

const (
	// KindUnclassified is the zero value and means no kind was attributed.
	// The extractor never records it — it exists so that an unset field
	// cannot alias the first real member, making a zero value fail-closed
	// switch material rather than a plausible-looking misattribution.
	KindUnclassified ConstructKind = iota

	// KindNumericOutOfRange is a numeric literal the value vocabulary
	// cannot represent — an integer beyond Go's int64 / int in a value
	// position, in SELECT TOP, or in LIMIT / OFFSET. This is the one drop
	// class a legal query reaches (REQ-119 § Amends REQ-117): degrading it
	// to a float would silently change the query, so it is dropped instead.
	KindNumericOutOfRange

	// KindUnmodelledConstruct is a construct the extraction catalogue does
	// not model. It is UNREACHABLE by a legal query while the catalogue
	// covers the whole SDK grammar profile (REQ-117) — every site that
	// records it is a defensive arm, present so that a widened grammar
	// fails loudly here instead of dropping a clause silently. It is a
	// single member rather than one per arm precisely because no corpus can
	// reach it: a per-arm enumeration could never be shown complete.
	KindUnmodelledConstruct
)

// String renders the kind for diagnostics. It names the CLASS and carries no
// source text, so it is safe on a disclosure boundary.
func (k ConstructKind) String() string {
	switch k {
	case KindNumericOutOfRange:
		return "numeric literal out of range"
	case KindUnmodelledConstruct:
		return "construct outside the extraction catalogue"
	case KindUnclassified:
		return "unclassified"
	}
	return "unclassified"
}

// DroppedConstruct records one construct the extractor could not represent.
//
// No field carries source text. [DroppedConstruct.Span] points back into the
// input for a caller that holds it and may quote it under its own disclosure
// policy; the SDK does not quote it here. A caller may therefore report the
// finding by name and position — "numeric literal out of range in LIMIT" —
// without echoing a value.
type DroppedConstruct struct {
	// Kind is the drop class. Never [KindUnclassified] as recorded by the
	// extractor; a reader still MUST handle that member and fail closed.
	Kind ConstructKind
	// Clause is the enclosing top-level clause, or [ClauseUnknown] when it
	// could not be attributed.
	Clause Clause
	// Span locates the construct in the exact input handed to [Parse].
	// Zero when the position could not be attributed — it never falls back
	// to embedding source text.
	Span Span
}

// String renders the record as "<kind> in <clause>" — value-free, and the
// form a refusal reason can use verbatim.
func (d DroppedConstruct) String() string {
	if d.Clause == ClauseUnknown {
		return d.Kind.String()
	}
	return d.Kind.String() + " in " + strings.ToUpper(d.Clause.String())
}

// Dropped returns every construct this document's extraction could not
// represent, in source order (REQ-113 § Value-free structured drop records).
// Empty when [Document.QueryErr] is nil.
//
// Dropped triggers the same lazy, once-only extraction as [Document.Query] /
// [Document.QueryErr]: its claim is completeness, so a document on which
// nothing called Query must not read as "nothing dropped". The returned
// slice is a COPY — mutating it cannot corrupt the document, which is shared
// and re-readable.
//
// The existing [Document.QueryErr] text and its [aql.ErrIncompleteAST]
// sentinel are unchanged; these records are additive.
func (d *Document) Dropped() []DroppedConstruct {
	if d == nil || d.tree == nil {
		return nil
	}
	d.queryOnce.Do(func() {
		d.query, d.dropped, d.queryErr = extractQuery(d.tree)
	})
	if len(d.dropped) == 0 {
		return nil
	}
	out := make([]DroppedConstruct, len(d.dropped))
	copy(out, d.dropped)
	return out
}
