package aql

// projection_verify.go — REQ-163 § `Build()` verifies what it emitted.
//
// `Col` splices its argument into the projection list verbatim, so SELECT is a
// clause the builder writes text into that it did not itself shape. Two failure
// modes follow and they are opposite in direction, which is why REQ-163 settles
// them with two rules rather than one:
//
//   - Text that re-parses as ONE projected item and introduces no clause-level
//     flag the builder did not record — a function call, an aliased item, a path
//     — is LEGACY and stays tolerated. Refusing it would be a behaviour break
//     for no correctness gain (REQ-163 § rule 1, and [Col]'s own doc).
//   - Text that SPLITS the item list, spills out of SELECT into another clause,
//     or introduces a clause-level flag is the SILENT mode: it emits valid AQL
//     asking a different question, invisible to every round-trip, golden and
//     parser check downstream. Build refuses it (§ rule 2).
//
// The comparison runs over the REDUCED structure — REQ-119 § Emission verified
// after emission's own reduction, not a second one — so the encodings the
// emitted text erases are factored out first. Two of them decide rows this REQ
// requires:
//
//   - A SOLE unaliased star item and the bare `SELECT *` flag are ONE encoding:
//     `SELECT *` re-parses as the flag with ZERO items, so a raw carrier count
//     would refuse the legal sole-star query.
//   - The clause-level FLAGS — DISTINCT, and the deprecated TOP — are part of
//     the compared structure beside the item count and each RECORDED alias. A
//     flag the re-parse carries and the builder never recorded is the silent
//     mode exactly: the query asks for distinct rows the caller never asked
//     for, and nothing downstream can see it. This is what settles
//     `Col("DISTINCT c/uid/value")`.
//
// # The mechanism
//
// REQ-163 § rule 2 fixes the DECISION and leaves the mechanism to the
// implementing phase, under one constraint: openehr/aql cannot call
// openehr/aql/parse, because parse imports aql and the call would be an import
// CYCLE rather than a layering choice (imports_test.go pins the arrow).
//
// So the decision is obtained by a HAND-DERIVED clause-escape scan — the
// projection-slot analogue of [ValidatePathPredicate], written from the same
// vendored grammar profile (ADR 0007) and reusing its lexer-state helpers. It
// reads the emitted clause text back the way the parser will: the flags the
// clause consumes before the first item, the top-level commas that separate
// items, the first top-level `AS` that ends each item's operand, and any
// clause-opening keyword at the top level of an item — the place the parser
// would stop reading the projection the builder wrote.
//
// The literal re-parse stays the REFERENCE procedure, in the tests, where
// importing parse is legal: every accepted query is built, emitted, parsed and
// re-emitted to IDENTITY (containment_roundtrip_test.go), and each refusal
// shape is confronted with the parser to prove the refusal right. If the scan
// and the reference ever disagree, the scan is wrong.
//
// Diagnostics stay value-free: a refusal names the COORDINATE — the item index,
// the clause, and where a keyword is at fault its spelling from the SDK's own
// fixed vocabulary — and never reproduces the offending projection text
// (REQ-119 § the redaction rule).

import (
	"fmt"
	"strings"
)

// selectShape is the REDUCED structure of a SELECT clause: what a reader of the
// clause carries once the encodings its text erases are factored out. It is the
// write-side counterpart of the [parse.SelectClause] fields the comparison
// covers.
type selectShape struct {
	// distinct and top are the clause-level flags. top records the CLAUSE's
	// presence, not its count: a second TOP beside a recorded one is caught as
	// an escape, so the count never has to be re-derived from the text.
	distinct bool
	top      bool
	// star is the projection's star flag, true both for the bare `SELECT *`
	// form and for a star mixed with column items — exactly as
	// [parse.SelectClause.Star] is.
	star bool
	// items is the item count AFTER the reduction: zero for the bare star form,
	// which carries its projection in the flag alone.
	items int
	// aliases is the per-item AS alias, "" where an item carries none. It is
	// nil for the reduced bare-star form, which has no items.
	aliases []string
}

// recordedProjection is what the BUILDER recorded for the clause it emitted —
// the left-hand side of the comparison.
type recordedProjection struct {
	distinct bool
	top      bool
	items    []renderedItem
	// separators are the byte offsets of the commas the EMITTER itself wrote
	// between items, so a top-level comma at any other offset is one an item's
	// own text brought with it.
	separators map[int]bool
}

// shape reduces the recorded items to a [selectShape], applying the SAME
// reduction the derivation applies to the emitted text — a sole unaliased star
// item is the bare `SELECT *` form, which is how it re-parses.
//
// It runs over the builder's item LIST while [deriveSelectShape] runs over the
// emitted TEXT, so the two sides are computed by different code from different
// inputs. That is deliberate: a reduction dropped on either side alone makes the
// sole-star query refuse ITSELF, which is the failure
// TestSoleStarItemReducesToTheBareForm exists to catch.
func (r recordedProjection) shape() selectShape {
	sh := selectShape{distinct: r.distinct, top: r.top}
	if len(r.items) == 1 && r.items[0].expr == "*" && r.items[0].alias == "" {
		sh.star = true
		return sh
	}
	sh.items = len(r.items)
	sh.aliases = make([]string, len(r.items))
	for i, it := range r.items {
		sh.aliases[i] = it.alias
		if it.expr == "*" {
			sh.star = true
		}
	}
	return sh
}

// selectClause renders the SELECT clause payload — everything after the
// `SELECT ` keyword — and the structure the builder recorded for it.
//
// The clause is assembled into a string BEFORE it is written to the query so
// the verification below reads back exactly the bytes that will go on the wire,
// not a reconstruction of them.
func (a *ast) selectClause() (string, recordedProjection, error) {
	rec := recordedProjection{distinct: a.distinct, top: a.top != nil, separators: map[int]bool{}}
	var sb strings.Builder
	// Grammar order: `SELECT DISTINCT? top? selectExpr …`, so the flag precedes
	// the deprecated TOP clause (REQ-163 § Canonical spellings).
	//
	// The TOP operand is rendered AS RECORDED here, before validatePaging runs
	// later in build — a negative count, an unknown direction and the
	// forbidden channel combinations are all still refused there, and none of
	// them can change what the verification decides: [FormatTop] renders
	// `TOP <digits>` plus an optional direction keyword and can carry neither a
	// comma nor a clause word, so the clause reads back with the same item
	// structure either way.
	if a.distinct {
		sb.WriteString("DISTINCT ")
	}
	if a.top != nil {
		sb.WriteString(FormatTop(a.top))
		sb.WriteByte(' ')
	}
	for i, f := range a.sel {
		if i > 0 {
			rec.separators[sb.Len()] = true
			sb.WriteString(", ")
		}
		item, err := f.render(i)
		if err != nil {
			return "", rec, err
		}
		item.start = sb.Len()
		sb.WriteString(item.text)
		item.end = sb.Len()
		rec.items = append(rec.items, item)
	}
	return sb.String(), rec, nil
}

// verifySelectClause is REQ-163 § rule 2: the emitted clause is read back once
// and its REDUCED structure compared against what the builder recorded. A
// disagreement means the wire text asks a different question, so it is refused
// with an error wrapping [ErrInvalidQuery].
//
// The order of the checks is the order of diagnosis, from the most specific
// cause to the most general consequence: an opaque region left open explains
// everything after it, a clause-level flag explains a whole-clause change, a
// spilled keyword explains a truncated projection, and only then is the item
// count compared.
func verifySelectClause(clause string, rec recordedProjection) error {
	got := deriveSelectShape(clause)
	if got.flaw != "" {
		return fmt.Errorf("%w: %s %s; the emitted query continues past it, so the projection would "+
			"absorb the clauses after it and re-parse as a different query",
			ErrInvalidQuery, rec.coordinate(got.flawPos), got.flaw)
	}
	want := rec.shape()
	if got.shape.distinct != want.distinct {
		return fmt.Errorf("%w: the emitted SELECT clause reads back %s a clause-level DISTINCT, and the "+
			"builder recorded %s — `selectClause : SELECT DISTINCT? top? selectExpr …` consumes the "+
			"keyword into the CLAUSE and not into the item, so the query would ask for distinct rows "+
			"the caller never asked for; set the flag with (*aql.Builder).Distinct",
			ErrInvalidQuery, carrying(got.shape.distinct), carrying(want.distinct))
	}
	if got.shape.top != want.top {
		return fmt.Errorf("%w: the emitted SELECT clause reads back %s the deprecated TOP row limit, and "+
			"the builder recorded %s — `top` is a CLAUSE-level operand, so the query would carry a row "+
			"bound the caller never asked for; set it with (*aql.Builder).Top",
			ErrInvalidQuery, carrying(got.shape.top), carrying(want.top))
	}
	if len(got.escapes) > 0 {
		e := got.escapes[0]
		return fmt.Errorf("%w: %s spills out of the projection — its text carries the clause-level token %s "+
			"at the top level, so a reader stops reading the projection there and the emitted query "+
			"re-parses with a SELECT clause the builder did not write",
			ErrInvalidQuery, rec.coordinate(e.pos), e.word)
	}
	if got.shape.items != want.items || got.shape.star != want.star {
		return fmt.Errorf("%w: %s changes the projection's structure — the emitted clause reads back as %s, "+
			"and the builder recorded %s; text that re-shapes the item list is valid AQL asking a "+
			"different question",
			ErrInvalidQuery, rec.coordinate(rec.splitAt(got.commas)),
			describeItems(got.shape), describeItems(want))
	}
	// Aliases are compared only where the BUILDER fixed one: where the `AS`
	// boundary falls inside a legacy [Col]'s verbatim text is not part of the
	// recorded structure, which is what keeps `Col("COUNT(x) AS n")` tolerated
	// (REQ-163 § rule 1).
	if len(got.shape.aliases) != len(rec.items) {
		return nil // the reduced bare-star form; no items to align
	}
	for i, it := range rec.items {
		if !it.aliasRecorded || got.shape.aliases[i] == it.alias {
			continue
		}
		return fmt.Errorf("%w: SELECT item %d reads back with an AS alias the builder did not record; "+
			"`selectExpr : columnExpr (AS aliasName=IDENTIFIER)?` names the column, so the result set "+
			"would come back under a name the caller never asked for", ErrInvalidQuery, i)
	}
	return nil
}

// coordinate names the item a byte offset falls in, or the clause when it falls
// in none (the DISTINCT / TOP prefix, or a separator the emitter wrote). It
// reproduces no projection text — the coordinate is the whole diagnostic
// (REQ-119 § the redaction rule).
func (r recordedProjection) coordinate(pos int) string {
	for i, it := range r.items {
		if pos >= it.start && pos < it.end {
			return fmt.Sprintf("SELECT item %d", i)
		}
	}
	return "the SELECT clause"
}

// splitAt returns the offset of the first top-level comma the EMITTER did not
// write, i.e. the one an item's own text brought with it, or -1 when every
// comma is a separator the emitter wrote.
func (r recordedProjection) splitAt(commas []int) int {
	for _, p := range commas {
		if !r.separators[p] {
			return p
		}
	}
	return -1
}

// carrying and describeItems spell a shape difference in words. Both render
// STRUCTURE only — a count, a flag — so neither can leak projection text.
func carrying(v bool) string {
	if v {
		return "with"
	}
	return "without"
}

func describeItems(sh selectShape) string {
	if sh.items == 0 && sh.star {
		return "the bare `SELECT *` form"
	}
	return fmt.Sprintf("%d item(s)", sh.items)
}

// derivedProjection is what ONE pass over the emitted clause text says a reader
// of that text will carry.
type derivedProjection struct {
	shape selectShape
	// commas are the offsets of the top-level commas — the item separators the
	// reader sees, whoever wrote them.
	commas []int
	// escapes are the clause-level keywords found at the top level of an ITEM
	// (the DISTINCT / TOP prefix the clause itself consumes is not one). Each is
	// a place the reader stops reading the projection.
	escapes []projectionWord
	// flaw names an opaque region the text leaves open or a delimiter it leaves
	// unbalanced ("" when none), with the offset that proved it.
	flaw    string
	flawPos int
}

// deriveSelectShape reads the emitted SELECT-clause payload back the way the
// parser will. It is a lexical scan, not a parse: it decides only which
// characters are DELIMITERS and which are content, which is what makes it
// derivable by hand from the grammar profile.
func deriveSelectShape(clause string) derivedProjection {
	var d derivedProjection
	sc := scanProjection(clause)
	if sc.flaw != "" {
		d.flaw, d.flawPos = sc.flaw, sc.flawPos
		return d
	}
	d.commas = sc.commas

	// The clause-level prefix, `SELECT DISTINCT? top?`: these keywords are
	// consumed by the CLAUSE before the first item, so finding one here sets
	// the flag rather than reporting an escape. A DISTINCT the builder never
	// recorded is exactly how `Col("DISTINCT c/uid/value")` reads back.
	i := skipProjectionSpace(clause, 0)
	if projectionWordAt(clause, i) == "DISTINCT" {
		d.shape.distinct = true
		i = skipProjectionSpace(clause, i+len("DISTINCT"))
	}
	if projectionWordAt(clause, i) == "TOP" {
		d.shape.top = true
		i = skipProjectionSpace(clause, i+len("TOP"))
		for i < len(clause) && clause[i] >= '0' && clause[i] <= '9' {
			i++
		}
		i = skipProjectionSpace(clause, i)
		// `top : TOP INTEGER direction=(FORWARD|BACKWARD)?` — the direction
		// belongs to the clause too, so it is stepped over rather than left to
		// look like the first item's text.
		for _, dir := range []string{"FORWARD", "BACKWARD"} {
			if projectionKeywordIs(clause, i, dir) {
				i = skipProjectionSpace(clause, i+len(dir))
				break
			}
		}
	}

	// `selectExpr (SYM_COMMA selectExpr)*`: the top-level commas are the item
	// boundaries, whoever wrote them.
	type span struct{ start, end int }
	frags := []span{}
	start := i
	for _, p := range sc.commas {
		if p < i {
			continue
		}
		frags = append(frags, span{start, p})
		start = p + 1
	}
	frags = append(frags, span{start, len(clause)})

	exprs := make([]string, len(frags))
	aliases := make([]string, len(frags))
	for fi, fr := range frags {
		exprEnd := fr.end
		aliased := false
		for _, w := range sc.words {
			if w.pos < fr.start || w.pos >= fr.end {
				continue
			}
			// `columnExpr` cannot contain a bare `AS`, so the FIRST top-level
			// one ends the operand — and any keyword after it is an escape,
			// a second `AS` included.
			if w.word == "AS" && !aliased {
				aliased = true
				exprEnd = w.pos
				aliases[fi] = strings.TrimSpace(clause[w.pos+len("AS") : fr.end])
				continue
			}
			d.escapes = append(d.escapes, w)
		}
		exprs[fi] = strings.TrimSpace(clause[fr.start:exprEnd])
	}

	// The reduction: a SOLE unaliased star item and the bare `SELECT *` flag
	// are one encoding, so the star form carries ZERO items — which is how the
	// text re-parses, and the spelling REQ-163 § Canonical spellings calls
	// canonical.
	if len(frags) == 1 && exprs[0] == "*" && aliases[0] == "" {
		d.shape.star = true
		return d
	}
	d.shape.items = len(frags)
	d.shape.aliases = aliases
	for _, e := range exprs {
		if e == "*" {
			d.shape.star = true
		}
	}
	return d
}

// projectionWord is a clause-level keyword found in the clause text, carried
// with the offset that found it and the SDK's OWN canonical spelling — never
// the caller's bytes, so a diagnostic naming it reproduces no source.
type projectionWord struct {
	pos  int
	word string
}

// projectionScan is what one pass over projection text tells the derivation.
type projectionScan struct {
	// commas are the offsets of the commas outside every literal, regex,
	// comment, term code, bracket and paren — the item separators a reader
	// sees.
	commas []int
	// words are the clause-level keywords at that same top level, in offset
	// order.
	words []projectionWord
	// flaw names an opaque region left open or a delimiter left unbalanced
	// ("" when none), with the offset that proved it. Either one makes the
	// counts above a PREFIX count, so the derivation must refuse before it
	// reads them.
	flaw    string
	flawPos int
}

// scanProjection walks projection text once, tracking the states that make a
// character not the delimiter it looks like. The state list is
// [ValidatePathPredicate]'s — the same grammar, the same lexer — with the
// PARENTHESES of a function call added, since a projected `columnExpr` reaches
// `functionCall` and a predicate does not:
//
//   - a string literal: a comma inside `'…'` / `"…"` is content, and an
//     UNTERMINATED one swallows the emitter's own following clauses;
//   - a contained regex, matched as a WHOLE token or not at all;
//   - a comment, which the lexer SKIPS rather than channels, so it survives in
//     the text — and at the top level of a projection it comments out the rest
//     of the emitted query, so it is reported rather than stepped over;
//   - a TERM_CODE display name, transparent to quotes and dashes alike;
//   - a node predicate `[…]`, whose commas and keywords are its own;
//   - a function call's parentheses, whose commas separate ARGUMENTS rather
//     than items.
func scanProjection(text string) projectionScan {
	var sc projectionScan
	brackets, parens := 0, 0
	for i := 0; i < len(text); {
		switch c := text[i]; c {
		case '\'', '"':
			j, ok := skipPredicateString(text, i)
			if !ok {
				sc.flaw, sc.flawPos = "leaves a string literal unterminated", i
				return sc
			}
			i = j
		case '{':
			// A `{` that begins no COMPLETE contained regex is ordinary
			// content — ANTLR takes the longest match, so the token falls back
			// to SYM_LEFT_CURLY and every character after it is lexed on its
			// own terms. A body still open at the end of the text is refused:
			// a comma and a `)` are both ordinary body characters, so the run
			// continues into the rest of the emitted query.
			j, open := skipContainedRegex(text, i)
			if j > 0 {
				i = j
				continue
			}
			if open {
				sc.flaw, sc.flawPos = "leaves a contained regex unterminated", i
				return sc
			}
			i++
		case ':':
			// `TERM_CODE : TERM_CODE_CHAR+ '::' TERM_CODE_CHAR+ ('|' … '|')?`,
			// and TERM_CODE_CHAR admits '-', so a `--` inside a term code is
			// not a comment: maximal munch takes both dashes into the code.
			if i+1 < len(text) && text[i+1] == ':' {
				j := i + 2
				for j < len(text) && termCodeChar(text[j]) {
					j++
				}
				if j < len(text) && text[j] == '|' {
					if k, ok := skipTermCodeName(text, j); ok {
						j = k
					}
				}
				i = j
				continue
			}
			i++
		case '[':
			brackets++
			i++
		case ']':
			brackets--
			if brackets < 0 {
				sc.flaw, sc.flawPos = "closes a `[…]` predicate it never opened", i
				return sc
			}
			i++
		case '(':
			parens++
			i++
		case ')':
			parens--
			if parens < 0 {
				sc.flaw, sc.flawPos = "closes a `(…)` it never opened", i
				return sc
			}
			i++
		case '-':
			n, closed := commentRun(text, i)
			if n == 0 {
				i++
				continue
			}
			if !closed {
				sc.flaw, sc.flawPos = "leaves a comment unterminated", i
				return sc
			}
			if brackets == 0 && parens == 0 {
				// A comment at the top level of a projection comments out
				// everything after it on the line — the emitted FROM clause
				// included — so it is an escape, not content.
				sc.words = append(sc.words, projectionWord{pos: i, word: "--"})
			}
			i += n
		case ',':
			if brackets == 0 && parens == 0 {
				sc.commas = append(sc.commas, i)
			}
			i++
		default:
			if brackets == 0 && parens == 0 {
				if w := projectionWordAt(text, i); w != "" {
					sc.words = append(sc.words, projectionWord{pos: i, word: w})
					i += len(w)
					continue
				}
			}
			i++
		}
	}
	switch {
	case brackets != 0:
		sc.flaw, sc.flawPos = "leaves a `[…]` predicate unclosed", len(text)
	case parens != 0:
		sc.flaw, sc.flawPos = "leaves a `(…)` unclosed", len(text)
	}
	return sc
}

// projectionClauseWords are the tokens whose appearance at the TOP level of the
// SELECT clause text means the parser is no longer reading the projection the
// builder recorded: the clause OPENERS that end it, the two clause-level FLAGS
// it may carry before the first item, and `AS`, which ends one item's operand.
//
// Longest first, because the match is by prefix and the boundary check comes
// after — `ASC` must not read as `AS` (it does not: `C` is a word character),
// and `ORDER` must not read as `OR` (which is not in this set at all, since a
// junction keyword in a projection is a LOUD malformation the parser rejects
// rather than a silent structure change).
//
// Source: resources/aql/grammar/active/AqlParser.g4 (`selectQuery`,
// `selectClause`, `selectExpr`, `top`) and AqlLexer.g4 (§ Keywords). Held
// against the grammar by the round-trip corpus rather than by a second list.
var projectionClauseWords = []string{
	"CONTAINS", "DISTINCT", "OFFSET", "SELECT", "ORDER", "LIMIT", "WHERE", "FROM", "TOP", "AS",
}

// projectionWordAt returns the clause-level keyword beginning at i in the SDK's
// canonical spelling, or "".
//
// The match is case-insensitive, because the lexer builds its keywords from
// case-insensitive ASCII letter fragments, and bounded on BOTH sides by a
// non-word character, so a path segment merely CONTAINING the letters —
// `c/from_date`, `c/topic`, `c/as_of` — is not mistaken for one. The fold is
// ASCII by construction: the compared slice is exactly len(kw) BYTES, in which
// a multi-byte rune leaves too few runes to match (the argument
// [asciiKeyword] states).
func projectionWordAt(s string, i int) string {
	if i > 0 && wordChar(s[i-1]) {
		return ""
	}
	for _, kw := range projectionClauseWords {
		if projectionKeywordIs(s, i, kw) {
			return kw
		}
	}
	return ""
}

// projectionKeywordIs reports whether the token kw begins at i, bounded by a
// non-word character on the right. The left boundary is the caller's to check
// ([projectionWordAt] does; the prefix walk reaches i only after whitespace).
func projectionKeywordIs(s string, i int, kw string) bool {
	if i < 0 || len(s)-i < len(kw) || !strings.EqualFold(s[i:i+len(kw)], kw) {
		return false
	}
	j := i + len(kw)
	return j >= len(s) || !wordChar(s[j])
}

// skipProjectionSpace steps over the whitespace the lexer skips between clause
// tokens.
func skipProjectionSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}
	return i
}

// checkArgumentEscape holds ONE argument of a projected call to its own slot —
// the same scan the clause runs, one level down.
//
// A comma inside an argument re-parses as one more argument, so the call comes
// back with an ARITY the builder never recorded: the silent-substitution mode
// below the item list, where the clause-level scan cannot see it (the commas
// are inside the emitter's own parentheses).
func checkArgumentEscape(fn string, idx int, text string) error {
	sc := scanProjection(text)
	switch {
	case sc.flaw != "":
		return fmt.Errorf("%w: %s() argument %d %s; the emitted `)` would fall inside it and the call "+
			"would run on into the rest of the query", ErrInvalidQuery, fn, idx, sc.flaw)
	case len(sc.commas) > 0:
		return fmt.Errorf("%w: %s() argument %d carries a top-level comma, so the emitted call reads back "+
			"with more arguments than the builder recorded", ErrInvalidQuery, fn, idx)
	case len(sc.words) > 0:
		return fmt.Errorf("%w: %s() argument %d carries the clause-level token %s at the top level, so the "+
			"emitted text does not read back as the call the builder recorded",
			ErrInvalidQuery, fn, idx, sc.words[0].word)
	}
	return nil
}
