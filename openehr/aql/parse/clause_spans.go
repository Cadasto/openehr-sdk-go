package parse

import (
	"unicode/utf8"

	"github.com/antlr4-go/antlr/v4"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse/gen"
)

// Offsets is a half-open byte range [Start, End) into the source text;
// src[o.Start:o.End] is the construct's text. The zero value means absent.
//
// Offsets counts bytes, where [Span] counts lines and columns, so a caller
// can slice the source string or splice new text into it directly.
type Offsets struct{ Start, End int }

// IsZero reports whether o locates nothing.
func (o Offsets) IsZero() bool { return o == Offsets{} }

// ClauseSpans locates each clause of a parsed query in its source text.
//
// A clause runs from the first byte of its keyword to just past the last
// byte of its last token, so whitespace and comments around it, and a
// trailing `--`, fall outside every span. An absent clause leaves its field
// zero.
type ClauseSpans struct {
	// Select covers the SELECT clause, DISTINCT and TOP included.
	Select Offsets
	// From covers the FROM clause with its CONTAINS tree.
	From Offsets
	// Where covers the WHERE keyword and its condition.
	Where Offsets
	// WhereExpr covers the condition after WHERE. Text inserted at its End
	// extends the condition in front of any ORDER BY or LIMIT. An inserted
	// `AND` binds tighter than an `OR` already there, so wrap a condition
	// holding OR in parentheses before adding a conjunct to the whole of it.
	WhereExpr Offsets
	// OrderBy covers the ORDER BY clause.
	OrderBy Offsets
	// Limit covers LIMIT and its value, and stops before any OFFSET.
	Limit Offsets
	// Offset covers OFFSET and its value.
	Offset Offsets
}

// clauseSpans locates each clause of tree in src, the text tree was parsed
// from. The grammar's limitClause holds both LIMIT and OFFSET, so Limit ends
// at the limit value and Offset starts at its keyword.
func clauseSpans(tree gen.ISelectQueryContext, src string) ClauseSpans {
	at := newByteOffsets(src)
	var s ClauseSpans
	if c := tree.SelectClause(); c != nil {
		s.Select = at.span(c.GetStart(), c.GetStop())
	}
	if c := tree.FromClause(); c != nil {
		s.From = at.span(c.GetStart(), c.GetStop())
	}
	if c := tree.WhereClause(); c != nil {
		s.Where = at.span(c.GetStart(), c.GetStop())
		if e := c.WhereExpr(); e != nil {
			s.WhereExpr = at.span(e.GetStart(), e.GetStop())
		}
	}
	if c := tree.OrderByClause(); c != nil {
		s.OrderBy = at.span(c.GetStart(), c.GetStop())
	}
	if c := tree.LimitClause(); c != nil {
		if v := c.GetLimit(); v != nil {
			s.Limit = at.span(c.GetStart(), v.GetStop())
		}
		if kw, v := c.OFFSET(), c.GetOffset(); kw != nil && v != nil {
			s.Offset = at.span(kw.GetSymbol(), v.GetStop())
		}
	}
	return s
}

// byteOffsets turns the character indices of ANTLR tokens into byte offsets
// into the source. ANTLR's Go input stream holds the source as []rune, so a
// token's start and stop count runes, not bytes. The conversion decodes the
// source as that []rune conversion does, one rune per invalid byte, so both
// agree on every input.
type byteOffsets struct {
	// starts[r] is the byte offset of rune r. It is nil when every rune is
	// one byte wide, where a rune index is already a byte offset.
	starts []int
	runes  int
	size   int
}

func newByteOffsets(src string) byteOffsets {
	b := byteOffsets{runes: utf8.RuneCountInString(src), size: len(src)}
	if b.runes == b.size {
		return b
	}
	b.starts = make([]int, 0, b.runes)
	for i := range src {
		b.starts = append(b.starts, i)
	}
	return b
}

// offset is the byte offset of rune index r; the rune count stands for the
// end of the source. It reports false for an index outside the source.
func (b byteOffsets) offset(r int) (int, bool) {
	switch {
	case r < 0 || r > b.runes:
		return 0, false
	case r == b.runes:
		return b.size, true
	case b.starts == nil:
		return r, true
	}
	return b.starts[r], true
}

// span is the byte range from the first character of first to just past the
// last character of last, or zero when either token is missing or the two
// do not form a range inside the source.
func (b byteOffsets) span(first, last antlr.Token) Offsets {
	if first == nil || last == nil {
		return Offsets{}
	}
	start, okStart := b.offset(first.GetStart())
	end, okEnd := b.offset(last.GetStop() + 1)
	if !okStart || !okEnd || end <= start {
		return Offsets{}
	}
	return Offsets{Start: start, End: end}
}
