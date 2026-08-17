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
// So the comparison is over an encoding-INDEPENDENT skeleton: the trees are
// FLATTENED into ordered leaf sequences, which is exactly the freedom the
// parser has (how it nests) factored out, and each leaf is reduced to the
// tokens `Emit` actually renders. A substitution cannot preserve that skeleton
// — to change the query's meaning it must add or remove a class expression, a
// projection, or a predicate leaf — and re-nesting cannot break it.
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
		if se, ok := errors.AsType[*SyntaxError](err); ok {
			return fmt.Errorf("%w: emitted AQL does not re-parse (syntax error at %d:%d); "+
				"the AST carries text no write-side guard covers: %w",
				aql.ErrInvalidQuery, se.Pos.Line, se.Pos.Col, aql.ErrSyntax)
		}
		if errors.Is(err, aql.ErrIncompleteAST) {
			return fmt.Errorf("%w: emitted AQL re-parses into an INCOMPLETE AST — the SDK "+
				"wrote a shape it cannot model back: %w", aql.ErrInvalidQuery, aql.ErrIncompleteAST)
		}
		return fmt.Errorf("%w: emitted AQL does not re-parse", aql.ErrInvalidQuery)
	}
	if at, ok := diffSkeleton(q, re); !ok {
		return fmt.Errorf("%w: emitted AQL re-parses as a DIFFERENT query (%s); a spliced "+
			"field terminated its own bracket or token early", aql.ErrInvalidQuery, at)
	}
	return nil
}

// diffSkeleton reports whether two queries carry the same encoding-independent
// skeleton, and on a difference the value-free coordinate at which they part.
func diffSkeleton(a, b *Query) (string, bool) {
	if a == nil || b == nil {
		return "query presence", a == b
	}
	sa, sb := skeletonOf(a), skeletonOf(b)
	for i, fa := range sa {
		if i >= len(sb) {
			return fa.at + " lost", false
		}
		if fb := sb[i]; fa.at != fb.at || fa.token != fb.token {
			return fa.at, false
		}
	}
	if len(sb) > len(sa) {
		return sb[len(sa)].at + " gained", false
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
	add("select.star", strconv.FormatBool(q.Select.Star))
	if t := q.Select.Top; t != nil {
		add("select.top", strconv.Itoa(t.N)+"/"+t.Dir.String())
	}
	// The projection is counted, not compared item by item: the SELECT
	// vocabulary re-encodes (a parsed function call over a literal argument is
	// not the shape a hand-built one carries), while the COUNT is what a
	// substitution has to change to add or drop a column. Per-item value
	// closure is REQ-119's value-position property, held by PROBE-090's own
	// suites rather than restated here.
	add("select item count", strconv.Itoa(len(q.Select.Items)))

	s = append(s, classSlots(q)...)
	s = append(s, whereSlots(q.Where, "where")...)

	add("order by term count", strconv.Itoa(len(q.OrderBy)))
	for i, t := range q.OrderBy {
		at := "orderBy[" + strconv.Itoa(i) + "]"
		add(at+".direction", t.Dir.String())
		add(at+".path text", t.Path.Raw)
	}
	add("limit", limitToken(q.Limit))
	add("offset", limitToken(q.Offset))
	return s
}

// classSlots flattens the FROM tree into its class expressions in source order.
//
// Flattening is the point: `A AND (B OR C)` and a hand-built tree that nests
// the same three classes differently emit the same text and MUST compare equal,
// while a swallowed `CONTAINS` term removes a class from the sequence and MUST
// NOT.
func classSlots(q *Query) []slot {
	var s []slot
	n := 0
	var walk func(c *Containment)
	walk = func(c *Containment) {
		if c == nil {
			return
		}
		// A junction node carries no class of its own — only operands — so it
		// contributes nothing and its children are spliced in at this level.
		if tok := classToken(c.Class); tok != "" {
			s = append(s, slot{"from.class[" + strconv.Itoa(n) + "]", tok})
			n++
		}
		for i := range c.Children {
			walk(&c.Children[i])
		}
	}
	s = append(s, slot{"from.root", classToken(q.From.Root)})
	n++
	walk(q.From.Contains)
	walk(q.From.Junction)
	return s
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
	bracket := c.Archetype
	if bracket == "" {
		bracket = c.Predicate
	}
	return rm + "|" + c.Alias + "|" + bracket
}

// whereSlots flattens a predicate tree into its LEAVES in order, so junction
// nesting and arity — where the parser has encoding freedom — do not enter the
// comparison, while a leaf added or removed does.
//
// Both sides route through [aql.DerefWhere] before any shape is read, so a
// pointer-carried twin reduces to the same token as its value form (REQ-119's
// pointer-twin invariant).
func whereSlots(w aql.WhereExpr, at string) []slot {
	d, ok := aql.DerefWhere(w)
	if !ok {
		// Absent WHERE and an unreadable one both render nothing; the emitter's
		// own validation refuses the latter before this runs.
		return nil
	}
	switch x := d.(type) {
	case aql.Junction:
		var s []slot
		for i, t := range x.Terms {
			s = append(s, whereSlots(t, at+".leaf["+strconv.Itoa(i)+"]")...)
		}
		return s
	case aql.NotExpr:
		return append([]slot{{at + ".not", "NOT"}}, whereSlots(x.Operand, at+".operand")...)
	case aql.Comparison:
		return []slot{{at, "cmp|" + x.Path + "|" + string(x.Op) + "|" +
			aql.FormatValue(x.Left) + "|" + aql.FormatValue(x.Val)}}
	case aql.ExistsExpr:
		return []slot{{at, "exists|" + x.Path}}
	case aql.LikeExpr:
		return []slot{{at, "like|" + x.Path + "|" + aql.FormatValue(x.Pattern)}}
	case aql.MatchesExpr:
		return []slot{{at, "matches|" + x.Path + "|" + matchesOperands(x)}}
	}
	return []slot{{at, "unknown"}}
}

func matchesOperands(m aql.MatchesExpr) string {
	if m.URI != "" {
		return "{" + m.URI + "}"
	}
	if m.Terminology != nil {
		args := make([]string, len(m.Terminology.Args))
		for i, a := range m.Terminology.Args {
			args[i] = aql.FormatValue(a)
		}
		return strings.ToUpper(m.Terminology.Name) + "(" + strings.Join(args, ",") + ")"
	}
	vals := make([]string, len(m.Values))
	for i, v := range m.Values {
		vals[i] = aql.FormatValue(v)
	}
	return "{" + strings.Join(vals, ",") + "}"
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
	return "unknown"
}
