package parse

// predicate_structure.go: turn a bracketed `pathPredicate` parse context into
// the typed [aql.SegmentPredicate] model — REQ-113 § Structured node
// predicates. Populated from LEXER-LEVEL tokens, never by re-lexing the
// verbatim text downstream of the parser, which is the burden the REQ removes.
//
// The grammar position is THREE overlapping productions:
//
//	pathPredicate      : '[' (standardPredicate | archetypePredicate | nodePredicate) ']'
//	standardPredicate  : objectPath COMPARISON_OPERATOR pathPredicateOperand
//	archetypePredicate : ARCHETYPE_HRID | PARAMETER
//	nodePredicate      : (ID_CODE|AT_CODE) (',' name)? | ARCHETYPE_HRID (',' name)?
//	                   | PARAMETER | objectPath COMPARISON_OPERATOR pathPredicateOperand
//	                   | objectPath MATCHES CONTAINED_REGEX
//	                   | nodePredicate AND nodePredicate | nodePredicate OR nodePredicate
//
// so a comparison, a bare HRID and a bare `$param` each arrive through TWO
// contexts depending on nesting. Both routes below produce the same kind:
// structuredPathPredicate dispatches the three top-level alternatives, and
// structuredNodePredicate handles nodePredicate including its own recursion.

import (
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse/gen"
)

// structuredPathPredicate is the entry point for a whole bracket. Returns nil
// for a nil or empty context, and for the ENUMERATED unstructured forms
// (REQ-113 § Structured node predicates): a comparison whose operand the
// value vocabulary does not carry — an objectPath, a node code, or an
// out-of-range numeric, per pathPredicateOperandValue — and a junction
// containing one. Nil-not-partial: a hole never ships inside a populated
// carrier.
func structuredPathPredicate(pp gen.IPathPredicateContext) aql.SegmentPredicate {
	if pp == nil {
		return nil
	}
	// standardPredicate is pathPredicate's FIRST alternative, so a top-level
	// standing comparison arrives here rather than through nodePredicate's
	// structurally identical alternative.
	if sp := pp.StandardPredicate(); sp != nil {
		return structuredStandardPredicate(sp)
	}
	// archetypePredicate is `ARCHETYPE_HRID | PARAMETER` — the BARE forms
	// only. An HRID carrying a name is a nodePredicate.
	if ap := pp.ArchetypePredicate(); ap != nil {
		if hrid := ap.ARCHETYPE_HRID(); hrid != nil {
			return aql.ArchetypePredicate{HRID: hrid.GetText()}
		}
		if param := ap.PARAMETER(); param != nil {
			return aql.ParamPredicate{Name: strings.TrimPrefix(param.GetText(), "$")}
		}
		return nil
	}
	return structuredNodePredicate(pp.NodePredicate())
}

// structuredStandardPredicate lifts the standing comparison shared by
// standardPredicate and nodePredicate's comparison alternative.
func structuredStandardPredicate(sp gen.IStandardPredicateContext) aql.SegmentPredicate {
	cmp := standingComparison(sp)
	if cmp == nil {
		// The operand is outside the value vocabulary. Reporting a
		// ComparisonPredicate with a nil Val would be a PARTIAL structure,
		// which the REQ forbids — a reader could not tell it from a complete
		// one. Unstructured, with the verbatim text intact, is the honest
		// answer.
		return nil
	}
	return aql.ComparisonPredicate{Comparison: *cmp}
}

// structuredNodePredicate handles the nodePredicate production, including its
// own AND / OR recursion.
func structuredNodePredicate(np gen.INodePredicateContext) aql.SegmentPredicate {
	if np == nil {
		return nil
	}
	// Junction first: the recursive alternatives are the only ones with two
	// nodePredicate children, so testing for them disambiguates before any
	// token lookup, which on a junction would find the LEFT operand's tokens.
	if kids := np.AllNodePredicate(); len(kids) == 2 {
		op := aql.OpAnd
		if np.OR() != nil {
			op = aql.OpOr
		}
		left, right := structuredNodePredicate(kids[0]), structuredNodePredicate(kids[1])
		if left == nil || right == nil {
			// A junction missing an operand is a partial structure.
			return nil
		}
		return aql.JunctionPredicate{Op: op, Left: left, Right: right}
	}
	// `objectPath MATCHES CONTAINED_REGEX`
	if np.MATCHES() != nil {
		op, rx := np.ObjectPath(), np.CONTAINED_REGEX()
		if op == nil || rx == nil {
			return nil
		}
		parsed := &aql.IdentifiedPath{
			Segments: segmentsFromObjectPath(op),
			Raw:      sourceText(op),
		}
		pattern, label := splitContainedRegex(rx.GetText())
		return aql.MatchesPredicate{
			Path:       sourceText(op),
			ParsedPath: parsed,
			Regex:      pattern,
			Label:      label,
		}
	}
	// `objectPath COMPARISON_OPERATOR pathPredicateOperand` — the same shape
	// standardPredicate carries, reached here only inside a junction.
	if np.COMPARISON_OPERATOR() != nil {
		return structuredNodeComparison(np)
	}
	// `ARCHETYPE_HRID (',' name)?`
	if hrid := np.ARCHETYPE_HRID(); hrid != nil {
		return aql.ArchetypePredicate{HRID: hrid.GetText(), Name: predicateNameOf(np)}
	}
	// `(ID_CODE | AT_CODE) (',' name)?` — the node code is the FIRST of the
	// code tokens; a second one, when present, is the name.
	if code := firstNodeCode(np); code != "" {
		return aql.NodeIDPredicate{ID: code, Name: predicateNameOf(np)}
	}
	// `PARAMETER` — bare, reached here only inside a junction. A parameter
	// AFTER a comma is a name, not a whole predicate, and the grammar admits
	// at most one PARAMETER per nodePredicate, so the comma decides.
	if param := np.PARAMETER(); param != nil && np.SYM_COMMA() == nil {
		return aql.ParamPredicate{Name: strings.TrimPrefix(param.GetText(), "$")}
	}
	return nil
}

// structuredNodeComparison lifts nodePredicate's comparison alternative. It
// mirrors standingComparison, which is typed to the standardPredicate context.
func structuredNodeComparison(np gen.INodePredicateContext) aql.SegmentPredicate {
	op, cmp, operand := np.ObjectPath(), np.COMPARISON_OPERATOR(), np.PathPredicateOperand()
	if op == nil || cmp == nil || operand == nil {
		return nil
	}
	v := pathPredicateOperandValue(operand)
	if v == nil {
		return nil
	}
	path := sourceText(op)
	parsed := IdentifiedPath{}
	parsed.Segments = segmentsFromObjectPath(op)
	parsed.Raw = path
	return aql.ComparisonPredicate{Comparison: aql.Comparison{
		Path:       path,
		Op:         aql.Operator(cmp.GetText()),
		Val:        v,
		ParsedPath: &parsed.IdentifiedPath,
	}}
}

// firstNodeCode returns the leading ID_CODE / AT_CODE of a node-id predicate,
// or "" when the predicate does not start with one. Order matters: the name
// slot admits AT_CODE and ID_CODE too, so "the first code token in source
// order" is the node id and any later one is the name.
func firstNodeCode(np gen.INodePredicateContext) string {
	first, col := "", -1
	for _, t := range append(np.AllID_CODE(), np.AllAT_CODE()...) {
		c := t.GetSymbol().GetStart()
		if col == -1 || c < col {
			first, col = t.GetText(), c
		}
	}
	// A code that is not the predicate's first token is a NAME, not a node id
	// (`[openEHR-…v1, at0004]`), so it must not be lifted to the ID position.
	if col == -1 || col != np.GetStart().GetStart() {
		return ""
	}
	return first
}

// predicateNameOf reads the optional name after the comma. Returns nil when
// the predicate carries none.
//
// The slot is `SYM_COMMA (STRING | PARAMETER | TERM_CODE | AT_CODE | ID_CODE)`
// — FIVE spellings, and it hangs off the node-id and the archetype-HRID
// alternatives alike. A carrier modelling only a quoted string and a coded
// name would drop three of them, one of which (PARAMETER) is not a name at all
// but the name deferred to bind time.
func predicateNameOf(np gen.INodePredicateContext) *aql.PredicateName {
	if np.SYM_COMMA() == nil {
		return nil
	}
	after := np.SYM_COMMA().GetSymbol().GetStart()

	if s := np.STRING(); s != nil && s.GetSymbol().GetStart() > after {
		// REQ-119: normalise before deciding behaviour from a concrete shape —
		// unquoteAQLString may hand back either carrier of the shape.
		text := ""
		if v, ok := aql.DerefValue(unquoteAQLString(s.GetText())); ok {
			if sv, isString := v.(aql.StringValue); isString {
				text = sv.S
			}
		}
		return &aql.PredicateName{Kind: aql.NameString, Text: text}
	}
	if tc := np.TERM_CODE(); tc != nil && tc.GetSymbol().GetStart() > after {
		return termCodeName(tc.GetText())
	}
	// PARAMETER in the name slot. The grammar admits at most one PARAMETER per
	// nodePredicate, and this branch is only reached past a comma, so a
	// parameter here is the name rather than the bare-parameter alternative.
	if param := np.PARAMETER(); param != nil && param.GetSymbol().GetStart() > after {
		return &aql.PredicateName{
			Kind: aql.NameParam,
			Text: strings.TrimPrefix(param.GetText(), "$"),
		}
	}
	for _, c := range np.AllAT_CODE() {
		if c.GetSymbol().GetStart() > after {
			return &aql.PredicateName{Kind: aql.NameAtCode, Text: c.GetText()}
		}
	}
	for _, c := range np.AllID_CODE() {
		if c.GetSymbol().GetStart() > after {
			return &aql.PredicateName{Kind: aql.NameIDCode, Text: c.GetText()}
		}
	}
	return nil
}

// termCodeName decomposes a TERM_CODE token:
// `TERM_CODE_CHAR+ ('(' TERM_CODE_CHAR+ ')')? '::' TERM_CODE_CHAR+ ('|' ~[|[\]]+ '|')?`
// — terminology (with an optional parenthesised version), `::`, code, and an
// optional `|display|`. Decomposed rather than handed over whole, because a
// consumer that has to split it is back to re-lexing.
func termCodeName(text string) *aql.PredicateName {
	n := &aql.PredicateName{Kind: aql.NameTermCode}
	rest := text
	if i := strings.Index(rest, "|"); i >= 0 {
		n.Display = strings.TrimSuffix(rest[i+1:], "|")
		rest = rest[:i]
	}
	term, code, ok := strings.Cut(rest, "::")
	if !ok {
		// Defensive: the lexer cannot produce a TERM_CODE without `::`.
		n.Terminology = rest
		return n
	}
	n.Terminology, n.Code = term, code
	return n
}

// splitContainedRegex decomposes a CONTAINED_REGEX token —
// `'{' WS* SLASH_REGEX WS* (';' WS* STRING)? WS* '}'` — into its two values:
// the pattern between the SLASH_REGEX slashes (with the `\/` spelling, which
// exists only because `/` is the delimiter, resolved to `/`; every other
// backslash sequence is regex syntax and stays as written), and the optional
// label with quotes removed and escapes resolved. Handed the components
// rather than the token, a reader has nothing left to re-lex — the braces,
// the interior whitespace and the `;` are all delimiters, not value.
func splitContainedRegex(text string) (pattern, label string) {
	body := strings.TrimSuffix(strings.TrimPrefix(text, "{"), "}")
	body = strings.TrimSpace(body)
	if len(body) == 0 || body[0] != '/' {
		// Defensive: the lexer cannot produce a CONTAINED_REGEX whose first
		// non-WS byte is not the pattern's opening slash.
		return body, ""
	}
	// Scan the SLASH_REGEX body (the lexer fragment quoted above): a byte
	// after a backslash never closes the pattern.
	i, escaped := 1, false
	var b strings.Builder
	for ; i < len(body); i++ {
		c := body[i]
		if escaped {
			if c != '/' {
				// A regex escape — keep its backslash.
				b.WriteByte('\\')
			}
			b.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '/' {
			break
		}
		b.WriteByte(c)
	}
	pattern = b.String()
	rest := strings.TrimSpace(body[min(i+1, len(body)):])
	if after, ok := strings.CutPrefix(rest, ";"); ok {
		q := strings.TrimSpace(after)
		// REQ-119: normalise before deciding behaviour from a concrete shape.
		if v, okDeref := aql.DerefValue(unquoteAQLString(q)); okDeref {
			if sv, isString := v.(aql.StringValue); isString {
				label = sv.S
			}
		}
	}
	return pattern, label
}
