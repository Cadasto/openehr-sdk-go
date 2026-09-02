package parse

// emit_verify.go — REQ-119, issue #103. The emission closure verified AFTER
// emission, over the whole query, rather than per spliced position.
//
// Every other REQ-119 guard decides one position from the text handed to it.
// One residual is not decidable that way: a `CONTAINED_REGEX` whose body
// completes a token inside the bracket text AND stays reachable beyond it
// (`{/a\/}` — `'\\/'` carries a `/` inside the body and `}` is an ordinary
// body character). Which token the lexer forms depends on text the predicate
// does not contain, so the escape condition is a property of the WHOLE QUERY.
//
// Two things this oracle is NOT, both established by trying them:
//
//   - **Text idempotence.** `ParseQuery(Emit(q)).Emit() == Emit(q)` HOLDS for
//     the residual and proves nothing: the re-parse reads the escaped text back
//     into the predicate field VERBATIM, so re-emitting reproduces the
//     byte-identical string while a whole `CONTAINS` term has vanished from the
//     tree. Verification has to be structural.
//   - **AST equality.** The AST admits several ENCODINGS of one emitted text,
//     and the parser picks one: a containment junction's operands nest
//     differently from a hand-built tree, a `VERSION` class ignores `RMType`,
//     `HasPredicate` is a read-side signal the emitter does not consult, and
//     `Pos` / `Raw` / `ParsedPath` / `PredicateComparison` are read-side
//     carriers a constructed AST leaves zero. Comparing fields refused most of
//     the hand-built corpus.
//
// So the comparison is over an encoding-INDEPENDENT skeleton: each tree is
// reduced to its structure with ONLY the freedom the parser legitimately has
// factored out — a same-operator junction chain is spliced flat and a one-term
// junction reduces to its term, because those are the encodings the emitted
// text erases — while nesting under a DIFFERENT operator stays in the
// coordinate, because `a AND (b OR c)` and `(a AND b) OR c` are different
// texts. Each leaf is reduced to the tokens `Emit` actually renders. A
// substitution cannot preserve that skeleton — to change the query's meaning
// it must add or remove a class expression, a projection, or a predicate
// leaf, or move one across an operator — and a legal re-encoding cannot break
// it.
//
// This does not settle the `WhereExpr` equality question § Out of scope defers
// ("is `a AND b` equal to `b AND a`?"). Leaves are compared POSITIONALLY, which
// answers no such question; that strand stays open for SEMANTIC equality.
//
// Diagnostics name the COORDINATE of the first difference and never the values
// at it. This bracket is where openEHR carries the identifiable root
// (`[ehr_id/value='…']`) — the same reason [aql.RedactPredicateValues] exists.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

// verifyEmitted confronts emitted text with the parser and reports a
// structure-changing difference as an error wrapping [aql.ErrInvalidQuery].
//
// A re-parse failure and a skeleton divergence are DISTINCT outcomes and are
// reported as such: the first is text this SDK wrote and cannot read (a defect
// in a guard, or an AST field carrying text no guard covers), the second is the
// silent substitution class — text that parses cleanly as a DIFFERENT query,
// which no downstream round-trip, golden or parser check would notice.
func (q *Query) verifyEmitted(out string) error {
	re, err := ParseQuery(out)
	if err != nil {
		// The parser's own message ECHOES the offending source text
		// ("no viable alternative at input 'a/b=''c''--'"), so wrapping it
		// verbatim would put predicate content — the identifiable root this
		// position carries — into every error string and log line downstream.
		// The position is reported instead, and both sentinels stay reachable
		// without the message riding along.
		if pos, ok := syntaxErrorPosition(err); ok {
			return fmt.Errorf("%w: emitted AQL does not re-parse (syntax error at %d:%d); "+
				"the AST carries text no write-side guard covers: %w",
				aql.ErrInvalidQuery, pos.Line, pos.Col, aql.ErrSyntax)
		}
		if errors.Is(err, aql.ErrIncompleteAST) {
			return fmt.Errorf("%w: emitted AQL re-parses into an INCOMPLETE AST — the SDK "+
				"wrote a shape it cannot model back: %w", aql.ErrInvalidQuery, aql.ErrIncompleteAST)
		}
		return fmt.Errorf("%w: emitted AQL does not re-parse", aql.ErrInvalidQuery)
	}
	sa, sb := skeletonOf(q), skeletonOf(re)
	// Fail CLOSED before comparing — see [checkModelled]. Two calls rather than
	// one over a concatenation, which aliased sa's backing array.
	if err := checkModelled(sa); err != nil {
		return err
	}
	if err := checkModelled(sb); err != nil {
		return err
	}
	if at, ok := diffSlots(sa, sb); !ok {
		return fmt.Errorf("%w: emitted AQL re-parses as a DIFFERENT query — the two differ at "+
			"%s (a spliced field may have terminated its bracket or token early, or the AST "+
			"carries a field emission drops)", aql.ErrInvalidQuery, at)
	}
	return nil
}

// syntaxErrorPosition extracts the position from a [ParseQuery] re-parse
// failure. A failed errors.AsType can leave a typed-nil *SyntaxError in a
// caller's hands (REQ-025 nil-receiver axis) — ok alone is not proof there is
// a position to read — so a typed-nil match reports false exactly as a
// non-match does, and verifyEmitted falls through to its generic refusal.
func syntaxErrorPosition(err error) (Position, bool) {
	se, ok := errors.AsType[*SyntaxError](err)
	if !ok || se == nil {
		return Position{}, false
	}
	return se.Pos, true
}

// diffSlots reports whether two skeletons agree, and on a difference the
// value-free coordinate at which they part.
//
// A length difference is reported at the first slot the shorter side lacks:
// [skeletonOf] emits a fixed set of slots unconditionally (the SELECT flags,
// `from.root`, the counts, `limit`, `offset`), so a divergence in the variable
// middle always shows up as a coordinate mismatch first, and a bare length
// difference means one side ran out of tree.
func diffSlots(sa, sb []slot) (string, bool) {
	for i, fa := range sa {
		if i >= len(sb) {
			return fa.at + " (lost)", false
		}
		if fb := sb[i]; fa.at != fb.at || fa.token != fb.token {
			return fa.at, false
		}
	}
	if len(sb) > len(sa) {
		return sb[len(sa)].at + " (gained)", false
	}
	return "", true
}

// slot is one skeleton coordinate and the token emission renders there. `at` is
// value-free and safe to surface; `token` may carry predicate or value text and
// MUST NOT be.
type slot struct {
	at    string
	token string
}

// skeletonOf reduces a query to its ordered skeleton.
func skeletonOf(q *Query) []slot {
	var s []slot
	add := func(at, token string) { s = append(s, slot{at, token}) }

	add("select.distinct", strconv.FormatBool(q.Select.Distinct))
	// The EFFECTIVE star, not the flag: the emitter renders `*` only for the
	// bare form and ignores `Star` once `Items` is populated (a mixed projection
	// carries its star as a [StarExpr] item instead). Comparing the raw flag
	// refused `{Star: true, Items: […]}`, whose emitted text says exactly what
	// the emitter chose — the same reasoning that keeps `HasPredicate` out of
	// [classToken]. A SOLE unaliased [StarExpr] item is the bare star's OTHER
	// encoding: it emits `SELECT *`, which re-parses as `{Star: true}` with no
	// items, so comparing the encodings refused a query whose text is legal —
	// both reduce to the flag form here.
	items := q.Select.Items
	star := q.Select.Star && len(items) == 0
	if len(items) == 1 && items[0].Alias == "" {
		if d, ok := DerefSelectExpr(items[0].Expr); ok {
			if _, isStar := d.(StarExpr); isStar {
				star, items = true, nil
			}
		}
	}
	add("select.star", strconv.FormatBool(star))
	if t := q.Select.Top; t != nil {
		add("select.top", strconv.Itoa(t.N)+"/"+t.Dir.String())
	}
	// The projection is counted, not compared item by item: the SELECT
	// vocabulary re-encodes (a parsed function call over a literal argument is
	// not the shape a hand-built one carries), while the COUNT is what a
	// substitution has to change to add or drop a column. Per-item value
	// closure is REQ-119's value-position property, held by PROBE-090's own
	// suites rather than restated here.
	add("select item count", strconv.Itoa(len(items)))
	// Each item's AS alias, which emission renders and which names a result
	// column an ORDER BY key may bind to. The item's EXPRESSION is deliberately
	// not tokenised: the SELECT vocabulary re-encodes (a parsed call over a
	// literal argument is not the shape a hand-built one carries), and an item
	// whose path TEXT changes its kind is the path-splice class § Out of scope
	// defers by REQ-055 rule 3, not this closure's business.
	for i, it := range items {
		add("select.items["+strconv.Itoa(i)+"].alias", it.Alias)
	}

	s = append(s, classSlots(q)...)
	// WHERE PRESENCE is its own slot, and it must be, because absence and
	// UNREADABILITY render the same nothing. `Emit` gates on `q.Where != nil` —
	// an INTERFACE nil — and [aql.FormatWhere] returns ("", nil) for a value it
	// cannot read, by documented design. So a typed-nil `*aql.Comparison` (the
	// classic Go trap: `func f() *aql.Comparison { return nil }` assigned to the
	// interface) emitted the query with its WHERE clause silently GONE. Without
	// this slot both sides reduce to no leaves and the skeletons compare equal —
	// the guard swallowing exactly the class it exists to catch, and at the
	// position that carries the `ehr_id` scoping filter.
	add("where.presence", wherePresence(q.Where))
	s = append(s, whereSlots(q.Where, "where")...)

	add("order by term count", strconv.Itoa(len(q.OrderBy)))
	for i, t := range q.OrderBy {
		at := "orderBy[" + strconv.Itoa(i) + "]"
		add(at+".direction", t.Dir.String())
		// Edge whitespace normalised — see [pathToken].
		add(at+".path text", strings.TrimSpace(t.Path.Raw))
	}
	add("limit", limitToken(q.Limit))
	add("offset", limitToken(q.Offset))
	return s
}

// classSlots reduces the FROM tree to its structural skeleton: the root class,
// then the containment tree with junction operators and negation kept AT their
// coordinate, exactly as [whereSlots] keeps the WHERE tree's.
//
// Only the encodings one text erases are factored out — a same-`ChildJoin`
// junction nested in its parent is spliced into the operand list, and a
// one-term junction reduces to its term (the keyword never reaches the text).
// Nesting under a DIFFERENT operator is structural and stays in the
// coordinate: `o AND (ev OR i)` is not `(o AND ev) OR i`. An IN-ORDER
// connective sequence was carried first and could not see that difference —
// both parenthesisations read [CONTAINS, AND, OR] — so a path-position splice
// re-grouped the operands with err == nil, the same soundness hole whose
// keyword-flip form the sequence had just closed.
func classSlots(q *Query) []slot {
	s := []slot{{"from.root", classToken(q.From.Root)}}
	if c := q.From.Contains; c != nil {
		s = append(s, containSlots(*c, "from.contains", edgeToken(*c))...)
	}
	if j := q.From.Junction; j != nil {
		s = append(s, containSlots(*j, "from", "")...)
	}
	return s
}

// containSlots reduces one containment node reached through edge — the
// spelled connective (`CONTAINS` / `NOT CONTAINS`), or "" for a junction
// operand, whose position carries no keyword of its own. The edge rides in
// the slot TOKEN rather than in a slot of its own so a dropped `NOT` and a
// re-grouped operand both surface at the node's coordinate.
//
// A class node's Children are a CHAIN, and the chain has its own erased
// encoding: `A CONTAINS B CONTAINS C` is one text whether the tree holds the
// tail flat (`A.Children = [B, C]`) or nested (`A.[B.[C]]`) — [emitContainment]
// renders every bracketing identically — so chain elements are flattened to
// sequential coordinates. A junction may only END a chain (validated), and a
// one-term junction's parentheses vanish from the text, so its term continues
// the SAME chain.
func containSlots(c Containment, at, edge string) []slot {
	if isContainmentJunction(c) {
		terms := flattenContainmentJunction(c)
		// A one-term junction renders as its term alone, so it must reduce to
		// that term's slots on the incoming edge — [whereSlots]'s own collapse.
		// The parser never builds one; a hand-built tree can. The junction's
		// own Negated (a legal `NOT CONTAINS` group) moves onto the term, whose
		// operand position could not carry one of its own.
		if len(terms) == 1 && !terms[0].Negated {
			t := terms[0]
			t.Negated = c.Negated
			return containSlots(t, at, edge)
		}
		s := []slot{{at + ".junction", edge + "|" + c.ChildJoin.String()}}
		for i, t := range terms {
			opEdge := ""
			if t.Negated {
				// No spelling negates a junction OPERAND (`NOT` belongs to
				// `CONTAINS`), and [validateContainmentTree] refuses the flag
				// before emission; carried anyway so the skeleton stays total.
				opEdge = "NOT"
			}
			s = append(s, containSlots(t, at+".op["+strconv.Itoa(i)+"]", opEdge)...)
		}
		return s
	}
	s := []slot{{at + ".class", edge + "|" + classToken(c.Class)}}
	j := 0
	var chain func(ch Containment)
	chain = func(ch Containment) {
		if isContainmentJunction(ch) {
			if terms := flattenContainmentJunction(ch); len(terms) == 1 && !terms[0].Negated {
				t := terms[0]
				t.Negated = ch.Negated
				chain(t)
				return
			}
			s = append(s, containSlots(ch, at+".chain["+strconv.Itoa(j)+"]", edgeToken(ch))...)
			j++
			return
		}
		s = append(s, slot{at + ".chain[" + strconv.Itoa(j) + "].class", edgeToken(ch) + "|" + classToken(ch.Class)})
		j++
		for i := range ch.Children {
			chain(ch.Children[i])
		}
	}
	for i := range c.Children {
		chain(c.Children[i])
	}
	return s
}

// edgeToken spells the connective through which a containment CHILD is
// reached — the reading [emitContainment] renders for the child's Negated flag.
func edgeToken(c Containment) string {
	if c.Negated {
		return "NOT CONTAINS"
	}
	return "CONTAINS"
}

// flattenContainmentJunction expands the operands of c that are themselves
// junctions on the SAME join, recursively — the associativity the emitted text
// already erases — mirroring [flattenJunction] on the WHERE side. A NEGATED
// operand junction never merges: its flag has no spelling and must stay at a
// coordinate of its own rather than vanish into the parent's operand list.
func flattenContainmentJunction(c Containment) []Containment {
	out := make([]Containment, 0, len(c.Children))
	for _, ch := range c.Children {
		if isContainmentJunction(ch) && ch.ChildJoin == c.ChildJoin && !ch.Negated {
			out = append(out, flattenContainmentJunction(ch)...)
			continue
		}
		out = append(out, ch)
	}
	return out
}

// classToken reduces a class expression to what emitClassExpr renders, and
// returns "" for a node that renders nothing (a junction operand holder).
//
// `HasPredicate` is deliberately absent: the emitter brackets on `Predicate` /
// `Archetype` being non-empty and never consults the flag, which the parser
// sets from source. A `VERSION` class renders the keyword and ignores `RMType`,
// so the token says so rather than carrying a field emission drops.
func classToken(c ClassExpr) string {
	rm := c.RMType
	if c.Version {
		rm = "VERSION"
	}
	if rm == "" && c.Alias == "" && c.Archetype == "" && c.Predicate == "" {
		return ""
	}
	// The VERSION branch renders `Predicate` and never `Archetype`, where the
	// class branch prefers `Archetype`. The divergence is unreachable only
	// because `checkClassOperands` refuses a VERSION class carrying an archetype
	// first; mirrored here so the token cannot drift if that guard moves.
	bracket := c.Predicate
	if !c.Version && c.Archetype != "" {
		bracket = c.Archetype
	}
	return rm + "|" + c.Alias + "|" + bracket
}

// whereSlots reduces a predicate tree to its operator and its LEAVES in order.
//
// A same-operator chain is FLATTENED first: `AND(AND(a,b),c)` and `AND(a,b,c)`
// emit the same text — [aql.FormatWhere] renders no parentheses for an
// associative chain, and `aql.And` / `aql.Or` do not flatten on construction,
// so incremental composition produces the nested form — so they MUST reduce to
// the same slots. Nesting under a DIFFERENT operator is structural and stays in
// the coordinate, since `a AND (b OR c)` is not `(a AND b) OR c`.
//
// The operator itself is a slot: `a AND b` and `a OR b` have identical leaves
// and are different queries.
//
// Both sides route through [aql.DerefWhere] before any shape is read, so a
// pointer-carried twin reduces to the same token as its value form (REQ-119's
// pointer-twin invariant).
func whereSlots(w aql.WhereExpr, at string) []slot {
	d, ok := aql.DerefWhere(w)
	if !ok {
		// A top-level absent WHERE renders nothing and reduces to no slots on
		// both sides, which is positional. Inside a composite, the emitter's own
		// validation refuses an unreadable term before this runs
		// ("AND junction term N carries no predicate").
		return nil
	}
	switch x := d.(type) {
	case aql.Junction:
		terms := flattenJunction(x)
		// A one-term junction renders as its term alone — no operator reaches the
		// text — so it must reduce to that term's slots. `Junction.validate`
		// accepts one term and only the `aql.And` / `aql.Or` constructors collapse
		// it, so a struct literal reaches here.
		if len(terms) == 1 {
			return whereSlots(terms[0], at)
		}
		s := []slot{{at + ".junction", string(x.Op)}}
		for i, t := range terms {
			s = append(s, whereSlots(t, at+".term["+strconv.Itoa(i)+"]")...)
		}
		return s
	case aql.NotExpr:
		return append([]slot{{at + ".not", "NOT"}}, whereSlots(x.Operand, at+".operand")...)
	case aql.Comparison:
		// The LHS has TWO carriers — `Path` for a path, `Left` for a call — and
		// the parser always reports a path LHS in `Path` while `aql.Compare` can
		// put one in `Left`. Both render the same text, so the carrier is
		// normalised away exactly as [classToken] does for Archetype/Predicate.
		lhs := x.Path
		if lhs == "" {
			lhs = aql.FormatValue(x.Left)
		}
		return []slot{{at, "cmp|" + pathToken(lhs) + "|" + string(x.Op) + "|" + pathToken(aql.FormatValue(x.Val))}}
	case aql.ExistsExpr:
		return []slot{{at, "exists|" + pathToken(x.Path)}}
	case aql.LikeExpr:
		return []slot{{at, "like|" + pathToken(x.Path) + "|" + aql.FormatValue(x.Pattern)}}
	case aql.MatchesExpr:
		return []slot{{at, "matches|" + pathToken(x.Path) + "|" + matchesOperands(x)}}
	}
	// Fail CLOSED — see [unmodelledPrefix]. The type name discriminates, so two
	// instances of an unlearned shape cannot compare equal.
	return []slot{{at, fmt.Sprintf("%s%T", unmodelledPrefix, d)}}
}

// unmodelledPrefix marks a slot the skeleton could not model. It exists so the
// verification fails CLOSED: a constant token would make two DIFFERENT instances
// of an unlearned shape compare equal, leaving the oracle blind at exactly the
// coordinate a newly added vocabulary shape occupies.
const unmodelledPrefix = "unmodelled:"

// pathToken normalises a path (or formatted operand) for a skeleton slot: the
// EDGE whitespace only. The emitters render a path verbatim (REQ-055 rule 3)
// while the parser's source span strips the padding, so an untrimmed carrier —
// `aql.Eq(" c/a ", …)`, a padded `OrderTerm.Path.Raw` — is one ENCODING of the
// trimmed text, not a different query, and comparing it raw refused legal
// queries built with the primary constructors (which do not trim). Interior
// bytes ride through untouched: trimming only the ends cannot mask a splice.
func pathToken(s string) string {
	return strings.TrimSpace(s)
}

// checkModelled refuses a skeleton carrying a slot the reduction could not
// model, BEFORE any comparison: an unmodelled slot compares equal to itself,
// so a vocabulary shape added without teaching this file would make the oracle
// silently blind at that coordinate rather than refuse.
func checkModelled(s []slot) error {
	for _, x := range s {
		if shape, found := strings.CutPrefix(x.token, unmodelledPrefix); found {
			return fmt.Errorf("%w: emission cannot be verified — %s carries %s, a shape the "+
				"closure check does not model; teach openehr/aql/parse/emit_verify.go",
				aql.ErrInvalidQuery, x.at, shape)
		}
	}
	return nil
}

// wherePresence distinguishes the three states of the WHERE slot that all render
// the same nothing, so absence cannot alias unreadability. See the call site.
func wherePresence(w aql.WhereExpr) string {
	if w == nil {
		return "absent"
	}
	if _, ok := aql.DerefWhere(w); !ok {
		return "unreadable"
	}
	return "present"
}

// flattenJunction expands the terms of j that are themselves junctions on the
// SAME operator, recursively — the associativity the emitted text already
// erases.
func flattenJunction(j aql.Junction) []aql.WhereExpr {
	out := make([]aql.WhereExpr, 0, len(j.Terms))
	for _, t := range j.Terms {
		if d, ok := aql.DerefWhere(t); ok {
			if inner, isJunction := d.(aql.Junction); isJunction && inner.Op == j.Op {
				out = append(out, flattenJunction(inner)...)
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

// matchesOperands reduces a MATCHES operand, and MUST mirror
// [aql.MatchesExpr]'s own renderer rather than re-deciding the same three-way
// choice: `Terminology` wins over a URI, and the URI's emptiness test is
// TrimSpace — not `!= ""` — because a whitespace-only URI is not an operand and
// the value list is what renders. Deciding it independently refused
// `MATCHES TERMINOLOGY(…)` and `MATCHES { uri://a }`, both of which validate and
// emit correctly.
//
// The operand is reduced through the SAME total formatter the emitter uses, so
// the skeleton cannot disagree with the text that was just written.
func matchesOperands(m aql.MatchesExpr) string {
	uri := strings.TrimSpace(m.URI)
	switch {
	case m.Terminology != nil:
		return aql.FormatValue(m.Terminology)
	case uri != "":
		return "{" + uri + "}"
	}
	vals := make([]string, len(m.Values))
	for i, v := range m.Values {
		vals[i] = aql.FormatValue(v)
	}
	return "{" + strings.Join(vals, ", ") + "}"
}

// limitToken reduces a LIMIT / OFFSET slot, routing through [DerefLimitExpr]
// so a pointer-carried twin reduces to the same token.
func limitToken(l LimitExpr) string {
	d, ok := DerefLimitExpr(l)
	if !ok {
		return ""
	}
	switch x := d.(type) {
	case IntLimit:
		return strconv.Itoa(x.N)
	case ParamLimit:
		return "$" + x.Name
	}
	return fmt.Sprintf("%s%T", unmodelledPrefix, d)
}
