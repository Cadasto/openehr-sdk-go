package aql

// predicate_model.go: the typed model for a bracketed path predicate —
// REQ-113 § Structured node predicates. REQ-113 typed exactly one predicate
// (the standing comparison at the class position) and REQ-119 then made
// predicate text VERBATIM, so every other predicate is raw text that a reader
// must re-lex, now with trivia in it. [StripPredicateTrivia] exists so each
// such reader can hold the same line; this model removes the need for the
// line.
//
// Raw is the SPELLING and stays authoritative for emission; this model is the
// VALUE and is authoritative for comparison. Nothing here is read by a write
// path, so REQ-119's round-trip closure is untouched by construction.

import "strings"

// SegmentPredicate is the typed form of one bracketed path predicate, sealed
// over the forms the grammar's `pathPredicate` position admits:
// [NodeIDPredicate], [ArchetypePredicate], [ParamPredicate],
// [ComparisonPredicate], [MatchesPredicate], and [JunctionPredicate].
//
// The kind is a property of the FORM, not of the grammar production that
// carried it. `pathPredicate` is `standardPredicate | archetypePredicate |
// nodePredicate` and the three overlap — a comparison, a bare archetype HRID
// and a bare `$param` are each spelled twice, once as their own top-level
// alternative and once inside `nodePredicate` — so the same form reaches this
// model through different parse contexts depending on whether it sits at the
// top of the bracket or nested as a junction operand. It yields the same
// kind either way.
//
// Every component is the VALUE, not the spelling: brackets and quotes
// removed, escapes resolved, and trivia (whitespace, `--` comments) absent.
// [PathSegment.Raw] keeps the verbatim text for anyone who needs it.
//
// The set grows ADDITIVELY as further positions are structured, so a consumer
// type-switching over it MUST treat an unrecognised case as unstructured —
// refuse, skip, or report — and MUST NOT panic on it.
//
// # Comparability
//
// A SegmentPredicate is NOT safe to compare with `==` and MUST NOT be used as
// a map key. [ComparisonPredicate] carries an [aql.Comparison], whose Val is a
// [Value] that panics under `==` for its slice-bearing shapes (see [Value]
// § Comparability) — and [JunctionPredicate] can hold one transitively. The
// compiler admits both the comparison and the map key, so this is invisible
// at build time, exactly as it is for [Value]. Use [EqualPredicates].
type SegmentPredicate interface {
	// key is the canonical comparison form: a shape tag followed by each
	// component, NUL-separated so no component boundary is ambiguous. It is
	// deliberately NOT a wire form — emission reads Raw, never this model.
	key() string
}

// NodeIDPredicate is an archetype node id, with the node name the grammar
// optionally admits after it: `[at0001]`, `[id3]`, `[at0001, 'Systolic']`.
type NodeIDPredicate struct {
	// ID is the node code without its brackets — "at0001", "id3".
	ID string
	// Name is the node name when the predicate carries one, nil otherwise.
	Name *PredicateName
}

// ArchetypePredicate is an archetype HRID in a path segment, with the node
// name the grammar optionally admits after it:
// `[openEHR-EHR-OBSERVATION.blood_pressure.v1]` or the same with `, 'Systolic'`.
//
// The name slot is easy to miss: it hangs off the HRID alternative as well as
// the node-id one, so an HRID predicate can be named.
type ArchetypePredicate struct {
	// HRID is the archetype human-readable id, brackets removed.
	HRID string
	// Name is the node name when the predicate carries one, nil otherwise.
	Name *PredicateName
}

// ParamPredicate is a whole predicate deferred to query-bind time: `[$param]`.
type ParamPredicate struct {
	// Name is the parameter name WITHOUT its leading `$`.
	Name string
}

// ComparisonPredicate is the standing comparison form — `[ehr_id/value=$x]`,
// `[name/value='Systolic']`. It carries the landed [Comparison] so that a
// WHERE comparison and a predicate comparison share ONE vocabulary rather
// than two structurally-identical ones.
//
// NOT `==`-comparable — see [SegmentPredicate] § Comparability.
type ComparisonPredicate struct {
	Comparison Comparison
}

// MatchesPredicate is the regex form — `[name/value matches {/systolic/}]`.
type MatchesPredicate struct {
	// Path is the left-hand object path as written.
	Path string
	// Regex is the pattern with its `{` `}` delimiters removed. The inner
	// `/…/` slashes, when present, are part of the pattern the grammar
	// carries and are left as written.
	Regex string
}

// JunctionPredicate is an `AND` / `OR` over two structured predicates —
// `[at0001 and name/value=$name]`. Nesting is retained rather than flattened:
// a reader that needs the flat term list can walk it, and one that needs the
// source grouping still has it.
//
// NOT `==`-comparable — see [SegmentPredicate] § Comparability.
type JunctionPredicate struct {
	// Op is [OpAnd] or [OpOr], reusing the WHERE-side junction vocabulary.
	Op BoolOp
	// Left and Right are the operands. Either may itself be a junction.
	Left, Right SegmentPredicate
}

// PredicateNameKind discriminates the five spellings the grammar admits in a
// predicate's name slot: `SYM_COMMA (STRING | PARAMETER | TERM_CODE | AT_CODE
// | ID_CODE)`. Modelling fewer than five drops admissible names, and one of
// them is not a name at all — see [NameParam].
type PredicateNameKind uint8

const (
	// NameUnknown is the zero value: no name spelling was attributed. A
	// populated [PredicateName] never carries it, so a reader can fail closed
	// on it rather than mistaking it for a real spelling.
	NameUnknown PredicateNameKind = iota
	// NameString is a quoted string name — `'Systolic'`. Text carries it
	// unquoted with escapes resolved.
	NameString
	// NameParam is a `$param` in the name slot: NOT a name, but the name
	// DEFERRED to query-bind time. A consumer resolving names against a
	// template MUST distinguish it from a name it can resolve now. Text
	// carries the parameter name without its `$`.
	NameParam
	// NameTermCode is a terminology-coded name — `SNOMED-CT::271649006|systolic|`.
	// The parts are decomposed into Terminology / Code / Display.
	NameTermCode
	// NameAtCode is an `at`-code in the name slot — `at0004`.
	NameAtCode
	// NameIDCode is an `id`-code in the name slot — `id4`.
	NameIDCode
)

// String renders the spelling for diagnostics. Value-free: it names the KIND
// and never the name itself.
func (k PredicateNameKind) String() string {
	switch k {
	case NameString:
		return "string"
	case NameParam:
		return "parameter"
	case NameTermCode:
		return "term code"
	case NameAtCode:
		return "at-code"
	case NameIDCode:
		return "id-code"
	case NameUnknown:
		return "unknown"
	}
	return "unknown"
}

// PredicateName is the node name a predicate carries after its comma.
//
// Kind says which of the five grammar spellings it was, because they are not
// interchangeable: a [NameParam] cannot be resolved until bind time, and a
// [NameTermCode] is a coded concept rather than a display string.
type PredicateName struct {
	// Kind is which spelling the source used. Never [NameUnknown] as
	// produced by the parser.
	Kind PredicateNameKind
	// Text is the name's value for every spelling except [NameTermCode]: a
	// string with quotes removed and escapes resolved, a parameter name
	// without its `$`, or a node code as written. Empty for a term code,
	// which decomposes into the three fields below.
	Text string
	// Terminology is a [NameTermCode]'s terminology id — the part before
	// `::`, with any parenthesised version suffix kept as written.
	Terminology string
	// Code is a [NameTermCode]'s code — the part after `::`.
	Code string
	// Display is a [NameTermCode]'s optional display name — the part between
	// the trailing `|` delimiters, which the grammar makes optional. Empty
	// when the source carried none.
	Display string
}

// key is the canonical comparison form of a name; "" for a nil name, which no
// real spelling can produce (NameUnknown is 0 and every real kind is > 0).
func (n *PredicateName) key() string {
	if n == nil {
		return ""
	}
	return strings.Join([]string{
		string(rune('0' + n.Kind)), n.Text, n.Terminology, n.Code, n.Display,
	}, "\x00")
}

func (p NodeIDPredicate) key() string {
	return "id\x00" + p.ID + "\x00" + p.Name.key()
}

func (p ArchetypePredicate) key() string {
	return "arch\x00" + p.HRID + "\x00" + p.Name.key()
}

func (p ParamPredicate) key() string { return "param\x00" + p.Name }

func (p ComparisonPredicate) key() string {
	var val string
	if v, ok := derefValue(p.Comparison.Val); ok {
		val = v.token()
	}
	return "cmp\x00" + p.Comparison.Path + "\x00" + string(p.Comparison.Op) + "\x00" + val
}

func (p MatchesPredicate) key() string {
	return "match\x00" + p.Path + "\x00" + p.Regex
}

func (p JunctionPredicate) key() string {
	var l, r string
	if p.Left != nil {
		l = p.Left.key()
	}
	if p.Right != nil {
		r = p.Right.key()
	}
	return "junc\x00" + string(p.Op) + "\x00(" + l + ")\x00(" + r + ")"
}

// EqualPredicates reports whether two structured predicates say the same
// thing — the replacement for `==` on a [SegmentPredicate] (see
// § Comparability), panic-free over every shape including a nil.
//
// Equality is over the KIND and its components, not over the source spelling:
// `[at0001 , 'x']` and `[at0001,'x']` are equal, which is the point of the
// model. Two nil predicates are equal; a nil never equals a populated one.
// A junction is compared structurally, so `a AND (b AND c)` does NOT equal
// `(a AND b) AND c` — the model retains the source grouping, and collapsing it
// here would make equality disagree with what the predicate carries.
func EqualPredicates(a, b SegmentPredicate) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return samePredicateShape(a, b) && a.key() == b.key()
}

// samePredicateShape reports whether two predicates carry the same concrete
// shape. The type check is explicit because keys alone could conflate shapes
// if a tag were ever duplicated; exhaustive over the sealed set, with a
// default that fails closed on a shape added without a row here.
func samePredicateShape(a, b SegmentPredicate) bool {
	switch a.(type) {
	case NodeIDPredicate:
		_, ok := b.(NodeIDPredicate)
		return ok
	case ArchetypePredicate:
		_, ok := b.(ArchetypePredicate)
		return ok
	case ParamPredicate:
		_, ok := b.(ParamPredicate)
		return ok
	case ComparisonPredicate:
		_, ok := b.(ComparisonPredicate)
		return ok
	case MatchesPredicate:
		_, ok := b.(MatchesPredicate)
		return ok
	case JunctionPredicate:
		_, ok := b.(JunctionPredicate)
		return ok
	}
	return false
}
